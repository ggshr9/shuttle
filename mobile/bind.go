// Package mobile provides gomobile-compatible bindings for the shuttle engine.
// Build with: gomobile bind -target=android -o mobile/android/shuttle.aar ./mobile
// Build with: gomobile bind -target=ios -o mobile/ios/Shuttle.xcframework ./mobile
package mobile

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/engine"
	"github.com/shuttle-proxy/shuttle/gui/api"
)

var (
	mu     sync.Mutex
	eng    *engine.Engine
	srv    *api.Server
	cancel context.CancelFunc
)

// Start initializes the engine and API server from a JSON config string.
// Returns the local API server address (e.g. "127.0.0.1:12345").
func Start(configJSON string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if eng != nil {
		return "", fmt.Errorf("already running")
	}

	var cfg config.ClientConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return "", fmt.Errorf("parse config: %w", err)
	}

	eng = engine.New(&cfg)

	ctx, c := context.WithCancel(context.Background())
	cancel = c

	if err := eng.Start(ctx); err != nil {
		eng = nil
		cancel = nil
		return "", fmt.Errorf("start engine: %w", err)
	}

	srv = api.NewServer(eng, nil)
	addr, err := srv.ListenAndServe("127.0.0.1:0")
	if err != nil {
		eng.Stop()
		eng = nil
		return "", fmt.Errorf("start api: %w", err)
	}

	return addr, nil
}

// StartWithTUN initializes the engine using an externally provided TUN file descriptor.
// This is intended for Android VpnService which creates the TUN device and passes its fd.
// Returns the local API server address.
func StartWithTUN(configJSON string, tunFD int) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if eng != nil {
		return "", fmt.Errorf("already running")
	}

	var cfg config.ClientConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return "", fmt.Errorf("parse config: %w", err)
	}

	// Enable TUN with the externally provided fd
	cfg.Proxy.TUN.Enabled = true
	cfg.Proxy.TUN.TunFD = tunFD

	eng = engine.New(&cfg)

	ctx, c := context.WithCancel(context.Background())
	cancel = c

	if err := eng.Start(ctx); err != nil {
		eng = nil
		cancel = nil
		return "", fmt.Errorf("start engine: %w", err)
	}

	srv = api.NewServer(eng, nil)
	addr, err := srv.ListenAndServe("127.0.0.1:0")
	if err != nil {
		eng.Stop()
		eng = nil
		return "", fmt.Errorf("start api: %w", err)
	}

	return addr, nil
}

// Stop shuts down the engine and API server.
func Stop() error {
	mu.Lock()
	defer mu.Unlock()

	if eng == nil {
		return fmt.Errorf("not running")
	}

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
