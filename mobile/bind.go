// Package mobile provides gomobile-compatible bindings for the shuttle engine.
// Build with: gomobile bind -target=android -o mobile/android/shuttle.aar ./mobile
// Build with: gomobile bind -target=ios -o mobile/ios/Shuttle.xcframework ./mobile
package mobile

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/engine"
	"github.com/shuttle-proxy/shuttle/gui/api"
	"github.com/shuttle-proxy/shuttle/internal/netmon"
)

var (
	mu     sync.Mutex
	eng    *engine.Engine
	srv    *api.Server
	cancel context.CancelFunc

	// Callback system
	cbMu     sync.RWMutex
	callback Callback

	// Auto-reconnect
	autoReconnect bool
	netMon        *netmon.Monitor

	// Log buffer
	logs *logBuffer

	// Event subscription goroutine cancel
	eventCancel context.CancelFunc

	// Logger for mobile-layer events
	mobileLogger *slog.Logger
)

func init() {
	logs = newLogBuffer(defaultLogBufferSize)
	mobileLogger = slog.New(newLogHandler(logs))
}

// SetCallback registers a native callback receiver. Pass nil to unregister.
// The callback methods are invoked from background goroutines; native code
// must handle thread safety (e.g. dispatch to main thread on iOS/Android).
func SetCallback(cb Callback) {
	cbMu.Lock()
	callback = cb
	cbMu.Unlock()
}

func getCallback() Callback {
	cbMu.RLock()
	cb := callback
	cbMu.RUnlock()
	return cb
}

// notifyStatus sends a status change to the registered callback, if any.
func notifyStatus(state string) {
	if cb := getCallback(); cb != nil {
		cb.OnStatusChange(state)
	}
}

// notifyError sends an error to the registered callback, if any.
func notifyError(code int, message string) {
	if cb := getCallback(); cb != nil {
		cb.OnError(code, message)
	}
}

// SetAutoReconnect enables or disables automatic transport reconnection
// when a network change (WiFi/cellular switch) is detected.
func SetAutoReconnect(enabled bool) {
	mu.Lock()
	autoReconnect = enabled
	mu.Unlock()
}

// ValidateConfig checks a JSON config string for required fields.
// Returns an empty string on success or an error message describing the problem.
func ValidateConfig(configJSON string) string {
	var cfg config.ClientConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Sprintf("invalid JSON: %v", err)
	}
	if cfg.Server.Addr == "" {
		return "server address is required"
	}
	if cfg.Server.Password == "" {
		return "server password is required"
	}
	if err := engine.ValidateConfig(&cfg); err != nil {
		return err.Error()
	}
	return ""
}

// GetRecentLogs returns the most recent log lines from the in-memory ring buffer,
// separated by newlines. maxLines controls how many lines to return (0 = all available).
func GetRecentLogs(maxLines int) string {
	return logs.recent(maxLines)
}

// Start initializes the engine and API server from a JSON config string.
// Returns the local API server address (e.g. "127.0.0.1:12345").
func Start(configJSON string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if eng != nil {
		return "", NewMobileError(ErrAlreadyRunning, "already running")
	}

	var cfg config.ClientConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return "", NewMobileError(ErrInvalidConfig, fmt.Sprintf("parse config: %v", err))
	}

	mobileLogger.Info("starting engine", "server", cfg.Server.Addr)
	notifyStatus("starting")

	eng = engine.New(&cfg)

	ctx, c := context.WithCancel(context.Background())
	cancel = c

	if err := eng.Start(ctx); err != nil {
		eng = nil
		cancel = nil
		mobileLogger.Error("start failed", "err", err)
		notifyStatus("stopped")
		return "", NewMobileError(ErrStartFailed, fmt.Sprintf("start engine: %v", err))
	}

	srv = api.NewServer(eng, nil)
	addr, err := srv.ListenAndServe("127.0.0.1:0")
	if err != nil {
		eng.Stop()
		eng = nil
		mobileLogger.Error("api server start failed", "err", err)
		notifyStatus("stopped")
		return "", NewMobileError(ErrStartFailed, fmt.Sprintf("start api: %v", err))
	}

	// Subscribe to engine events and forward to callback.
	startEventForwarder(eng)

	// Start network monitor for auto-reconnect.
	startNetworkMonitor(ctx)

	mobileLogger.Info("engine started", "api_addr", addr)
	notifyStatus("running")

	return addr, nil
}

// StartWithTUN initializes the engine using an externally provided TUN file descriptor.
// This is intended for Android VpnService which creates the TUN device and passes its fd.
// Returns the local API server address.
func StartWithTUN(configJSON string, tunFD int) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if eng != nil {
		return "", NewMobileError(ErrAlreadyRunning, "already running")
	}

	var cfg config.ClientConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return "", NewMobileError(ErrInvalidConfig, fmt.Sprintf("parse config: %v", err))
	}

	// Enable TUN with the externally provided fd
	cfg.Proxy.TUN.Enabled = true
	cfg.Proxy.TUN.TunFD = tunFD

	mobileLogger.Info("starting engine with TUN", "server", cfg.Server.Addr, "tun_fd", tunFD)
	notifyStatus("starting")

	eng = engine.New(&cfg)

	ctx, c := context.WithCancel(context.Background())
	cancel = c

	if err := eng.Start(ctx); err != nil {
		eng = nil
		cancel = nil
		mobileLogger.Error("start failed", "err", err)
		notifyStatus("stopped")
		return "", NewMobileError(ErrStartFailed, fmt.Sprintf("start engine: %v", err))
	}

	srv = api.NewServer(eng, nil)
	addr, err := srv.ListenAndServe("127.0.0.1:0")
	if err != nil {
		eng.Stop()
		eng = nil
		mobileLogger.Error("api server start failed", "err", err)
		notifyStatus("stopped")
		return "", NewMobileError(ErrStartFailed, fmt.Sprintf("start api: %v", err))
	}

	// Subscribe to engine events and forward to callback.
	startEventForwarder(eng)

	// Start network monitor for auto-reconnect.
	startNetworkMonitor(ctx)

	mobileLogger.Info("engine started with TUN", "api_addr", addr)
	notifyStatus("running")

	return addr, nil
}

// Stop shuts down the engine and API server.
func Stop() error {
	mu.Lock()
	defer mu.Unlock()

	if eng == nil {
		return NewMobileError(ErrNotRunning, "not running")
	}

	mobileLogger.Info("stopping engine")
	notifyStatus("stopping")

	stopEventForwarder()
	stopNetworkMonitor()

	if srv != nil {
		srv.Close()
		srv = nil
	}
	eng.Stop()
	if cancel != nil {
		cancel()
		cancel = nil
	}
	eng = nil

	mobileLogger.Info("engine stopped")
	notifyStatus("stopped")

	return nil
}

// Status returns the engine status as JSON.
func Status() string {
	mu.Lock()
	e := eng
	mu.Unlock()

	if e == nil {
		return `{"state":"stopped"}`
	}
	data, _ := json.Marshal(e.Status())
	return string(data)
}

// Reload updates the configuration and restarts the engine if needed.
func Reload(configJSON string) error {
	mu.Lock()
	defer mu.Unlock()

	if eng == nil {
		return NewMobileError(ErrNotRunning, "not running")
	}

	var cfg config.ClientConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return NewMobileError(ErrInvalidConfig, fmt.Sprintf("parse config: %v", err))
	}

	mobileLogger.Info("reloading config")

	if err := eng.Reload(&cfg); err != nil {
		mobileLogger.Error("reload failed", "err", err)
		notifyError(ErrReloadFailed, fmt.Sprintf("reload failed: %v", err))
		return NewMobileError(ErrReloadFailed, fmt.Sprintf("reload: %v", err))
	}

	mobileLogger.Info("config reloaded")
	return nil
}

// IsRunning returns whether the engine is currently running.
func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return eng != nil
}

// GetNetworkType returns the current detected network type.
// Returns one of: "wifi", "cellular", "ethernet", "unknown"
func GetNetworkType() string {
	mu.Lock()
	nm := netMon
	mu.Unlock()

	if nm == nil {
		return "unknown"
	}
	return nm.CurrentType().String()
}

// GetRecommendedPreset returns the name of the recommended network preset
// based on the current detected network type and power state.
// Native apps can use this to automatically apply optimal settings.
func GetRecommendedPreset() string {
	netType := GetNetworkType()
	power := currentPowerState()

	// Power-critical always uses data_saver
	if power == PowerCritical {
		return "data_saver"
	}

	switch netType {
	case "wifi":
		return "wifi"
	case "cellular":
		if power == PowerLow {
			return "data_saver"
		}
		return "lte"
	default:
		return "lte" // safe default
	}
}

// startEventForwarder subscribes to engine events and forwards them
// to the registered native callback. Must be called with mu held.
func startEventForwarder(e *engine.Engine) {
	ch := e.Subscribe()
	ctx, c := context.WithCancel(context.Background())
	eventCancel = c

	go func() {
		defer e.Unsubscribe(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				forwardEvent(ev)
			}
		}
	}()
}

// stopEventForwarder cancels the event forwarding goroutine. Must be called with mu held.
func stopEventForwarder() {
	if eventCancel != nil {
		eventCancel()
		eventCancel = nil
	}
}

// forwardEvent dispatches an engine event to the registered callback.
func forwardEvent(ev engine.Event) {
	cb := getCallback()
	if cb == nil {
		return
	}

	switch ev.Type {
	case engine.EventConnected:
		cb.OnStatusChange("running")
	case engine.EventDisconnected:
		cb.OnStatusChange("stopped")
	case engine.EventSpeedTick:
		cb.OnSpeedUpdate(ev.Upload, ev.Download)
	case engine.EventError:
		cb.OnError(0, ev.Error)
	case engine.EventNetworkChange:
		cb.OnNetworkChange()
	}
}

// debounceInterval is the minimum time between consecutive auto-reconnects
// to avoid thrashing on flaky network transitions.
const debounceInterval = 2 * time.Second

// lastReconnect tracks the time of the last auto-reconnect attempt.
var lastReconnect time.Time

// startNetworkMonitor creates a network change monitor that triggers
// auto-reconnect when enabled. Must be called with mu held.
func startNetworkMonitor(ctx context.Context) {
	nm := netmon.New(5 * time.Second)
	nm.OnChange(func() {
		mobileLogger.Info("network change detected by mobile monitor")

		if cb := getCallback(); cb != nil {
			cb.OnNetworkChange()
		}

		mu.Lock()
		shouldReconnect := autoReconnect && eng != nil
		e := eng

		// Debounce: skip if we reconnected very recently
		now := time.Now()
		if shouldReconnect && now.Sub(lastReconnect) < debounceInterval {
			mobileLogger.Info("auto-reconnect: debounced, skipping (too soon after last reconnect)")
			mu.Unlock()
			return
		}
		if shouldReconnect {
			lastReconnect = now
		}
		mu.Unlock()

		if shouldReconnect {
			// Brief delay to let the network interface stabilize
			time.Sleep(500 * time.Millisecond)

			mobileLogger.Info("auto-reconnect: reloading engine after network change")
			cfg := e.Config()
			if err := e.Reload(&cfg); err != nil {
				mobileLogger.Error("auto-reconnect reload failed", "err", err)
				notifyError(ErrReloadFailed, fmt.Sprintf("auto-reconnect failed: %v", err))
			} else {
				mobileLogger.Info("auto-reconnect: engine reloaded successfully")
			}
		}
	})

	// Also register network type callback for smarter reconnect
	nm.OnChangeWithType(func(netType netmon.NetworkType) {
		mobileLogger.Info("network type changed", "type", netType.String())

		// Apply appropriate preset based on detected network type
		mu.Lock()
		e := eng
		mu.Unlock()

		if e == nil {
			return
		}

		// Notify native layer with network type info
		if cb := getCallback(); cb != nil {
			cb.OnNetworkChange()
		}
	})

	nm.Start(ctx)
	netMon = nm
}

// stopNetworkMonitor stops the mobile network monitor. Must be called with mu held.
func stopNetworkMonitor() {
	if netMon != nil {
		netMon.Stop()
		netMon = nil
	}
}
