package engine

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/internal/netmon"
	"github.com/shuttleX/shuttle/internal/sysopt"
	"github.com/shuttleX/shuttle/mesh"
	"github.com/shuttleX/shuttle/plugin"
	"github.com/shuttleX/shuttle/proxy"
	"github.com/shuttleX/shuttle/stream"
	"github.com/shuttleX/shuttle/transport/selector"
)

const stopTimeout = 10 * time.Second

// Start initializes all subsystems and begins proxying.
func (e *Engine) Start(ctx context.Context) error {
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	return e.startInternal(ctx)
}

// startInternal is the lock-free core of Start; the caller must hold lifecycleMu.
func (e *Engine) startInternal(ctx context.Context) error {
	e.mu.Lock()
	if e.state == StateRunning || e.state == StateStarting {
		st := e.state
		e.mu.Unlock()
		return fmt.Errorf("engine already %s", st)
	}
	e.state = StateStarting
	e.parentCtx = ctx
	e.mu.Unlock()

	e.emit(Event{Type: EventLog, Message: "engine starting"})
	e.logger.Debug("engine state transition", "from", "stopped", "to", "starting")

	e.mu.Lock()
	e.streamTracker = stream.NewStreamTracker(0) // default 1000-entry ring
	e.mu.Unlock()

	sysopt.Apply(e.logger)

	e.mu.Lock()
	e.circuitBreaker = NewCircuitBreaker(CircuitBreakerConfig{
		OnStateChange: func(state CircuitState, cooldown time.Duration) {
			if state == CircuitOpen {
				e.emit(Event{Type: EventConnectionError, Error: "circuit breaker open", BackoffMs: cooldown.Milliseconds()})
			}
		},
	})
	e.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)

	// On any failure below, this helper resets state.
	fail := func(err error) error {
		cancel()
		e.mu.Lock()
		e.state = StateStopped
		e.cancel = nil
		e.sel = nil
		e.mu.Unlock()
		return err
	}

	e.mu.RLock()
	cfgSnap := e.cfg.DeepCopy()
	e.mu.RUnlock()

	e.logger.Debug("building congestion control", "mode", cfgSnap.Congestion.Mode)
	ccAdapter := e.buildCongestionControl(cfgSnap)
	e.logger.Debug("building transports")
	transports := e.buildTransports(cfgSnap, ccAdapter)
	if len(transports) == 0 {
		return fail(fmt.Errorf("no transports enabled; enable at least one in config (transport.h3, transport.reality, transport.cdn, or transport.webrtc)"))
	}

	strategy := selector.StrategyAuto
	switch cfgSnap.Transport.Preferred {
	case "multipath":
		strategy = selector.StrategyMultipath
	case "latency":
		strategy = selector.StrategyLatency
	case "priority":
		strategy = selector.StrategyPriority
	}

	e.logger.Debug("initializing transport selector", "strategy", strategy, "count", len(transports))
	sel := selector.New(transports, &selector.Config{
		Strategy:          strategy,
		ServerAddr:        cfgSnap.Server.Addr,
		MultipathSchedule: cfgSnap.Transport.MultipathSchedule,
		WarmUpConns:       cfgSnap.Transport.WarmUpConns,
	}, e.logger)
	sel.Start(ctx)

	e.mu.Lock()
	e.sel = sel
	e.cancel = cancel
	e.mu.Unlock()

	e.logger.Debug("building router and DNS resolver")
	rt, dnsResolver, prefetcher := e.buildRouter(cfgSnap)
	if prefetcher != nil {
		go prefetcher.Start(ctx)
	}
	if cfgSnap.Routing.GeoData.Enabled && cfgSnap.Routing.GeoData.AutoUpdate {
		if gm := e.GeoManager(); gm != nil {
			gm.Start(ctx)
		}
	}
	dialer := e.createDialer(cfgSnap, rt, dnsResolver)

	// Build plugin chain: metrics (byte counting + stats), connection tracker
	// (lifecycle events), and logger (debug logging).
	connTracker := plugin.NewConnTracker(e)
	chain := plugin.NewChain(
		e.metrics,
		connTracker,
		plugin.NewLogger(e.logger),
	)
	if err := chain.Init(ctx); err != nil {
		return fail(fmt.Errorf("plugin chain init: %w", err))
	}
	e.mu.Lock()
	e.chain = chain
	e.mu.Unlock()

	// Wrap dialer so every proxied connection flows through the plugin chain.
	dialer = e.wrapDialer(dialer, chain)

	e.logger.Debug("starting proxy listeners")
	closers, err := e.startProxies(ctx, cfgSnap, dialer, sel, cancel)
	if err != nil {
		sel.Close()
		chain.Close() // Clean up plugin chain on proxy start failure
		return fail(err)
	}

	e.mu.Lock()
	e.closers = closers
	e.currentRouter = rt
	e.state = StateRunning
	e.mu.Unlock()

	e.logger.Debug("engine state transition", "from", "starting", "to", "running")
	e.emit(Event{Type: EventConnected, Message: "engine started"})
	e.bgWg.Add(1)
	go func() {
		defer e.bgWg.Done()
		e.speedLoop(ctx)
	}()

	// Start network change monitor to detect WiFi/cellular switches.
	nm := netmon.New(5 * time.Second)
	nm.OnChange(func() {
		e.logger.Info("network change detected")
		e.emit(Event{Type: EventNetworkChange, Message: "network change detected"})
	})
	nm.Start(ctx)
	e.mu.Lock()
	e.netMon = nm
	e.mu.Unlock()

	return nil
}

// Stop shuts down the engine gracefully.
func (e *Engine) Stop() error {
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	return e.stopInternal()
}

// stopInternal is the lock-free core of Stop; the caller must hold lifecycleMu.
func (e *Engine) stopInternal() error {
	e.mu.Lock()
	if e.state != StateRunning {
		st := e.state
		e.mu.Unlock()
		return fmt.Errorf("engine not running (state: %s)", st)
	}
	e.state = StateStopping
	e.logger.Debug("engine state transition", "from", "running", "to", "stopping")
	cancel := e.cancel
	closers := e.closers
	sel := e.sel
	e.mu.Unlock()

	// Cancel context first so accept loops and in-flight dials unblock,
	// then close listeners/selector (whose wg.Wait won't hang).
	if cancel != nil {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, c := range closers {
			c()
		}
		if sel != nil {
			sel.Close()
		}
	}()
	select {
	case <-done:
	case <-time.After(stopTimeout):
		e.logger.Warn("engine stop timed out, forcing shutdown")
	}

	// Wait for background goroutines (speedLoop, MeshReceiveLoop) to exit.
	bgDone := make(chan struct{})
	go func() {
		e.bgWg.Wait()
		close(bgDone)
	}()
	select {
	case <-bgDone:
	case <-time.After(5 * time.Second):
		e.logger.Warn("background goroutines did not exit within timeout")
	}

	e.mu.Lock()
	if e.netMon != nil {
		e.netMon.Stop()
		e.netMon = nil
	}
	if e.geoManager != nil {
		e.geoManager.Stop()
		e.geoManager = nil
	}
	if e.chain != nil {
		e.chain.Close()
		e.chain = nil
	}
	e.state = StateStopped
	e.closers = nil
	e.sel = nil
	e.cancel = nil
	e.currentRouter = nil
	e.streamTracker = nil
	e.circuitBreaker = nil
	e.mu.Unlock()

	e.logger.Debug("engine state transition", "from", "stopping", "to", "stopped")
	e.emit(Event{Type: EventDisconnected, Message: "engine stopped"})
	return nil
}

// Reload stops and restarts the engine with a new config.
// The new config is validated before stopping; if invalid the engine keeps running.
func (e *Engine) Reload(cfg *config.ClientConfig) error {
	if err := ValidateConfig(cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()

	e.mu.RLock()
	oldCfg := e.cfg
	running := e.state == StateRunning
	parentCtx := e.parentCtx
	e.mu.RUnlock()

	if parentCtx == nil {
		parentCtx = context.Background()
	}

	e.logger.Debug("reload triggered", "was_running", running)
	if running {
		if err := e.stopInternal(); err != nil {
			return fmt.Errorf("stop for reload: %w", err)
		}
	}

	e.mu.Lock()
	e.cfg = cfg
	e.mu.Unlock()

	if err := e.startInternal(parentCtx); err != nil {
		// Rollback: restore old config and try to restart
		e.mu.Lock()
		e.cfg = oldCfg
		e.mu.Unlock()
		if running {
			if rollbackErr := e.startInternal(parentCtx); rollbackErr != nil {
				e.logger.Error("rollback restart failed", "err", rollbackErr)
				e.emit(Event{Type: EventError, Error: fmt.Sprintf("reload rollback failed: %v", rollbackErr)})
				return fmt.Errorf("new config: %w; rollback failed: %v", err, rollbackErr)
			}
		}
		return fmt.Errorf("start with new config: %w", err)
	}
	return nil
}

const meshMaxRetries = 3

// connectMesh attempts to establish a mesh connection with retries.
func (e *Engine) connectMesh(ctx context.Context, cfg *config.ClientConfig, tunServer *proxy.TUNServer) *mesh.MeshClient {
	serverAddr := cfg.Server.Addr
	var lastErr error
	for attempt := 1; attempt <= meshMaxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil
		}
		curSel := e.selector()
		if curSel == nil {
			e.logger.Warn("mesh: no active selector, skipping")
			return nil
		}
		conn, err := curSel.Dial(ctx, serverAddr)
		if err != nil {
			lastErr = err
			e.logger.Warn("mesh: dial failed, retrying", "attempt", attempt, "err", err)
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		mc, err := mesh.NewMeshClient(ctx, func(ctx context.Context) (io.ReadWriteCloser, error) {
			return conn.OpenStream(ctx)
		})
		if err != nil {
			conn.Close()
			lastErr = err
			e.logger.Warn("mesh: handshake failed, retrying", "attempt", attempt, "err", err)
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		tunServer.MeshClient = mc
		e.meshClient = mc // Store in engine for stats access
		if err := tunServer.AddMeshRoute(mc.MeshCIDR()); err != nil {
			e.logger.Warn("mesh: add route failed", "err", err)
		}
		e.bgWg.Add(1)
		go func() {
			defer e.bgWg.Done()
			tunServer.MeshReceiveLoop(ctx)
		}()
		e.logger.Info("mesh connected", "virtual_ip", mc.VirtualIP(), "cidr", mc.MeshCIDR())
		return mc
	}
	e.logger.Error("mesh: all attempts failed", "err", lastErr)
	return nil
}

// speedLoop periodically samples upload/download speed and emits events.
func (e *Engine) speedLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			up, down := e.metrics.SampleSpeed()
			e.emit(Event{
				Type:     EventSpeedTick,
				Upload:   up,
				Download: down,
			})
		}
	}
}
