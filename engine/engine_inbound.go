package engine

import (
	"encoding/json"
	"fmt"

	"context"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
)

// startInbounds starts all configured inbound listeners using the pluggable
// Inbound/Outbound abstraction layer. This is the new path activated when
// cfg.Inbounds is non-empty; the legacy startProxies path remains unchanged.
//
// It reuses the router and DNS resolver already built by startInternal
// (stored on e.currentRouter and e.dnsResolver), avoiding duplicate work.
func (e *Engine) startInbounds(ctx context.Context, cfg *config.ClientConfig) ([]func() error, error) {
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

	// Reuse the router and DNS resolver built by startInternal.
	e.mu.RLock()
	rt := e.currentRouter
	dnsResolver := e.dnsResolver
	e.mu.RUnlock()

	ibRouter := &inboundRouter{
		routerEngine: rt,
		dnsResolver:  dnsResolver,
		outbounds:    outbounds,
		defaultOut:   outbounds["proxy"],
		logger:       e.logger,
	}

	var closers []func() error
	var started []adapter.Inbound

	// On failure, only clean up what startInbounds created (inbounds + outbounds).
	// The caller (startInternal via startProxies) handles sel, cancel, and state.
	cleanup := func(err error) ([]func() error, error) {
		for _, ib := range started {
			_ = ib.Close()
		}
		for _, ob := range outbounds {
			_ = ob.Close()
		}
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
