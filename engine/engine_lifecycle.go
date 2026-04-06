package engine

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/internal/netmon"
	"github.com/shuttleX/shuttle/internal/sysopt"
	"github.com/shuttleX/shuttle/provider"
	"github.com/shuttleX/shuttle/proxy"
	"github.com/shuttleX/shuttle/stream"
	"github.com/shuttleX/shuttle/subscription"
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

	if err := cfgSnap.Validate(); err != nil {
		return fail(fmt.Errorf("config validation: %w", err))
	}

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
	poolIdleTTL, _ := time.ParseDuration(cfgSnap.Transport.PoolIdleTTL)
	sel := selector.New(transports, &selector.Config{
		Strategy:          strategy,
		ServerAddr:        cfgSnap.Server.Addr,
		MultipathSchedule: cfgSnap.Transport.MultipathSchedule,
		WarmUpConns:       cfgSnap.Transport.WarmUpConns,
		PoolMaxIdle:       cfgSnap.Transport.PoolMaxIdle,
		PoolIdleTTL:       poolIdleTTL,
	}, e.logger)
	sel.Start(ctx)

	e.mu.Lock()
	e.sel = sel
	e.cancel = cancel
	e.mu.Unlock()

	// Initialize proxy providers from config.
	proxyProviders := make([]*provider.ProxyProvider, 0, len(cfgSnap.ProxyProviders))
	for _, ppCfg := range cfgSnap.ProxyProviders {
		interval := time.Hour
		if ppCfg.Interval != "" {
			if d, err := time.ParseDuration(ppCfg.Interval); err == nil {
				interval = d
			}
		}
		pp, err := provider.NewProxyProvider(provider.ProxyProviderConfig{
			Name:     ppCfg.Name,
			URL:      ppCfg.URL,
			Path:     ppCfg.Path,
			Interval: interval,
			Filter:   ppCfg.Filter,
		})
		if err != nil {
			e.logger.Warn("failed to create proxy provider", "name", ppCfg.Name, "err", err)
			continue
		}
		pp.Start(ctx)
		proxyProviders = append(proxyProviders, pp)
		e.logger.Info("proxy provider started", "name", ppCfg.Name)
	}
	e.mu.Lock()
	e.proxyProviders = proxyProviders
	e.mu.Unlock()

	// Initialize rule providers from config.
	ruleProviders := make([]*provider.RuleProvider, 0, len(cfgSnap.RuleProviders))
	ruleProviderMap := make(map[string]*provider.RuleProvider, len(cfgSnap.RuleProviders))
	for _, rpCfg := range cfgSnap.RuleProviders {
		interval := time.Hour
		if rpCfg.Interval != "" {
			if d, err := time.ParseDuration(rpCfg.Interval); err == nil {
				interval = d
			}
		}
		rp, err := provider.NewRuleProvider(provider.RuleProviderConfig{
			Name:     rpCfg.Name,
			URL:      rpCfg.URL,
			Path:     rpCfg.Path,
			Behavior: rpCfg.Behavior,
			Interval: interval,
		})
		if err != nil {
			e.logger.Warn("failed to create rule provider", "name", rpCfg.Name, "err", err)
			continue
		}
		rp.Start(ctx)
		ruleProviders = append(ruleProviders, rp)
		ruleProviderMap[rpCfg.Name] = rp
		e.logger.Info("rule provider started", "name", rpCfg.Name)
	}
	e.mu.Lock()
	e.ruleProviders = ruleProviders
	e.mu.Unlock()

	e.logger.Debug("building router and DNS resolver")
	rt, dnsResolver, prefetcher := e.buildRouter(cfgSnap, ruleProviderMap)
	if prefetcher != nil {
		e.bgWg.Add(1)
		go func() {
			defer e.bgWg.Done()
			prefetcher.Start(ctx)
		}()
	}
	if cfgSnap.Routing.GeoData.Enabled && cfgSnap.Routing.GeoData.AutoUpdate {
		if gm := e.GeoManager(); gm != nil {
			e.bgWg.Add(1)
			go func() {
				defer e.bgWg.Done()
				gm.Start(ctx)
			}()
		}
	}
	// Store router and DNS resolver on engine so startInbounds can access them.
	e.mu.Lock()
	e.currentRouter = rt
	e.dnsResolver = dnsResolver
	e.mu.Unlock()

	// Build plugin chain: metrics (byte counting + stats), connection tracker
	// (lifecycle events), and logger (debug logging).
	if err := e.obs.BuildChain(ctx, e); err != nil {
		return fail(fmt.Errorf("plugin chain init: %w", err))
	}

	// Convert legacy proxy.* config to inbound entries so all listeners
	// flow through the unified inbound path.
	adaptLegacyConfig(cfgSnap)

	e.logger.Debug("starting inbound listeners")
	closers, err := e.startInbounds(ctx, cfgSnap)
	if err != nil {
		sel.Close()
		e.obs.CloseChain()
		return fail(err)
	}

	// Start mesh if configured.
	if cfgSnap.Mesh.Enabled {
		var tunIB *proxy.TUNInbound
		e.mu.RLock()
		for _, ib := range e.inbounds {
			if tun, ok := ib.(*proxy.TUNInbound); ok {
				tunIB = tun
				break
			}
		}
		e.mu.RUnlock()
		if err := e.meshManager.Start(ctx, cfgSnap, sel, tunIB); err != nil {
			e.logger.Warn("mesh start failed", "err", err)
			// Non-fatal: engine continues without mesh
		}
	}

	e.mu.Lock()
	e.closers = closers
	e.state = StateRunning
	e.mu.Unlock()

	e.logger.Debug("engine state transition", "from", "starting", "to", "running")
	e.emit(Event{Type: EventConnected, Message: "engine started"})
	e.obs.StartSpeedLoop(ctx)

	// Parse migration probe timeout from config.
	migrateTimeout := 3 * time.Second
	if cfgSnap.Transport.MigrationProbeTimeout != "" {
		if d, err := time.ParseDuration(cfgSnap.Transport.MigrationProbeTimeout); err == nil {
			migrateTimeout = d
		}
	}

	// Build a simple TCP probe function: try to dial the server address.
	serverAddr := cfgSnap.Server.Addr
	probeFn := func(ctx context.Context) error {
		var dialer net.Dialer
		c, err := dialer.DialContext(ctx, "tcp", serverAddr)
		if err != nil {
			return err
		}
		return c.Close()
	}

	pm := NewProactiveMigrator(
		ProactiveMigratorConfig{
			Enabled:    cfgSnap.Transport.ProactiveMigration,
			ServerAddr: serverAddr,
			Timeout:    migrateTimeout,
		},
		probeFn,
		func() { sel.Migrate() },
		e.logger,
		e.obs.Emit,
	)
	e.mu.Lock()
	e.proactiveMigrator = pm
	e.mu.Unlock()

	// Start network change monitor to detect WiFi/cellular switches.
	nm := netmon.New(5 * time.Second)
	nm.OnChange(func() {
		e.logger.Info("network change detected")
		e.obs.Emit(Event{Type: EventNetworkChange, Message: "network change detected"})
		go pm.OnNetworkChange(ctx) // non-blocking: avoid stalling netmon callback
	})
	e.bgWg.Add(1)
	go func() {
		defer e.bgWg.Done()
		nm.Start(ctx)
	}()
	e.mu.Lock()
	e.netMon = nm
	e.mu.Unlock()

	// Start subscription manager if subscriptions are configured.
	if len(cfgSnap.Subscriptions) > 0 {
		sm := subscription.NewManager()
		sm.LoadFromConfig(cfgSnap.Subscriptions)
		sm.StartAutoRefresh(ctx, 24*time.Hour)
		e.mu.Lock()
		e.subscriptionManager = sm
		e.mu.Unlock()
		e.logger.Info("subscription manager started", "count", len(cfgSnap.Subscriptions))
	}

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

	// Close mesh before waiting for background goroutines — this closes
	// the MeshClient (unblocking MeshReceiveLoop) and waits for its goroutines.
	if e.meshManager != nil {
		_ = e.meshManager.Close()
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, c := range closers {
			_ = c()
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

	// Wait for background goroutines (MeshReceiveLoop, etc.) to exit.
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

	// Wait for observability background goroutines (speed loop).
	obsDone := make(chan struct{})
	go func() {
		e.obs.WaitBackground()
		close(obsDone)
	}()
	select {
	case <-obsDone:
	case <-time.After(5 * time.Second):
		e.logger.Warn("observability goroutines did not exit within timeout")
	}

	// Stop providers before taking the final lock.
	e.mu.RLock()
	pps := e.proxyProviders
	rps := e.ruleProviders
	sm := e.subscriptionManager
	e.mu.RUnlock()
	for _, pp := range pps {
		pp.Stop()
	}
	for _, rp := range rps {
		rp.Stop()
	}
	if sm != nil {
		sm.StopAutoRefresh()
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
	e.obs.CloseChain()
	// Close inbounds and outbounds from the pluggable abstraction layer.
	for _, ib := range e.inbounds {
		_ = ib.Close()
	}
	e.inbounds = nil
	for _, ob := range e.outbounds {
		_ = ob.Close()
	}
	e.outbounds = nil
	e.state = StateStopped
	e.closers = nil
	e.sel = nil
	e.cancel = nil
	e.currentRouter = nil
	e.streamTracker = nil
	e.circuitBreaker = nil
	e.proactiveMigrator = nil
	e.subscriptionManager = nil
	e.proxyProviders = nil
	e.ruleProviders = nil
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


