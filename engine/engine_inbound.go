package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/transport/selector"
)

// startInbounds starts all configured inbound listeners using the pluggable
// Inbound/Outbound abstraction layer. This is the new path activated when
// cfg.Inbounds is non-empty; the legacy startProxies path remains unchanged.
func (e *Engine) startInbounds(ctx context.Context, cfg *config.ClientConfig, sel *selector.Selector, cancel context.CancelFunc) ([]func() error, error) {
	// Build outbounds: always provide direct, reject, and proxy.
	outbounds := map[string]adapter.Outbound{
		"direct": &DirectOutbound{tag: "direct"},
		"reject": &RejectOutbound{tag: "reject"},
		"proxy": &ProxyOutbound{
			tag:       "proxy",
			engine:    e,
			cfg:       cfg,
			retryCfg:  e.buildRetryConfig(cfg.Retry),
			shaperCfg: e.buildShaperConfig(cfg.Obfs),
		},
	}

	// Add any explicitly configured outbounds from the registry.
	for _, outCfg := range cfg.Outbounds {
		ob, err := adapter.CreateOutbound(outCfg.Type, outCfg.Tag, outCfg.Options, adapter.OutboundDeps{Logger: e.logger})
		if err != nil {
			return nil, fmt.Errorf("create outbound %q: %w", outCfg.Tag, err)
		}
		outbounds[outCfg.Tag] = ob
	}

	// Build the InboundRouter using the already-built router and DNS resolver.
	// NOTE: buildRouter was already called in startInternal; reuse that router
	// by reading it from the engine, but we still need the DNS resolver which
	// isn't stored on the engine. So we build the router here; this is cheap
	// compared to transport setup.
	rt, dnsResolver, prefetcher := e.buildRouter(cfg)
	if prefetcher != nil {
		go prefetcher.Start(ctx)
	}

	ibRouter := &inboundRouter{
		routerEngine: rt,
		dnsResolver:  dnsResolver,
		outbounds:    outbounds,
		defaultOut:   outbounds["proxy"],
		logger:       e.logger,
	}

	// Store the router for PAC generation and other engine consumers.
	e.mu.Lock()
	e.currentRouter = rt
	e.mu.Unlock()

	var closers []func() error
	var started []adapter.Inbound

	cleanup := func(err error) ([]func() error, error) {
		for _, ib := range started {
			_ = ib.Close()
		}
		for _, ob := range outbounds {
			_ = ob.Close()
		}
		sel.Close()
		cancel()
		e.mu.Lock()
		e.state = StateStopped
		e.sel = nil
		e.cancel = nil
		e.mu.Unlock()
		return nil, err
	}

	for _, inCfg := range cfg.Inbounds {
		// Build options JSON: if Options is set use it, otherwise construct
		// from the Listen field for convenience.
		opts := inCfg.Options
		if opts == nil && inCfg.Listen != "" {
			opts, _ = json.Marshal(map[string]string{"listen": inCfg.Listen})
		}

		ib, err := adapter.CreateInbound(inCfg.Type, inCfg.Tag, opts, adapter.InboundDeps{Logger: e.logger})
		if err != nil {
			return cleanup(fmt.Errorf("create inbound %q: %w", inCfg.Tag, err))
		}
		if err := ib.Start(ctx, ibRouter); err != nil {
			return cleanup(fmt.Errorf("start inbound %q: %w", inCfg.Tag, err))
		}
		started = append(started, ib)
		closers = append(closers, ib.Close)
	}

	// Store inbounds/outbounds on the engine for lifecycle management.
	e.mu.Lock()
	e.inbounds = started
	e.outbounds = outbounds
	e.mu.Unlock()

	return closers, nil
}
