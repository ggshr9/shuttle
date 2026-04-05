package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
)

// startInbounds starts all configured inbound listeners using the pluggable
// Inbound/Outbound abstraction layer. This is the unified path for all proxy
// listeners; legacy proxy.* config is converted by adaptLegacyConfig before
// this function is called.
//
// It reuses the router and DNS resolver already built by startInternal
// (stored on e.currentRouter and e.dnsResolver), avoiding duplicate work.
func (e *Engine) startInbounds(ctx context.Context, cfg *config.ClientConfig) ([]func() error, error) {
	// Build outbounds: always provide direct, reject, and proxy.
	outbounds := e.traffic.BuildBuiltinOutbounds(cfg, e)

	// Add mesh outbound if mesh is enabled.
	if cfg.Mesh.Enabled {
		outbounds["mesh"] = NewMeshOutbound("mesh", e.meshManager)
	}

	// First pass: build individual outbounds (skip groups).
	for _, outCfg := range cfg.Outbounds {
		if outCfg.Type == "group" {
			continue // handled in second pass
		}
		if outCfg.Type == "proxy" {
			// Custom proxy outbound — create a ProxyOutbound pointing to
			// a different server address, reusing the engine's transport
			// selector and stream metrics.
			ob, err := e.createCustomProxyOutbound(outCfg.Tag, outCfg.Options, cfg)
			if err != nil {
				return nil, err
			}
			outbounds[outCfg.Tag] = ob
		} else {
			ob, err := adapter.CreateOutbound(outCfg.Type, outCfg.Tag, outCfg.Options, adapter.OutboundDeps{Logger: e.logger})
			if err != nil {
				return nil, fmt.Errorf("create outbound %q: %w", outCfg.Tag, err)
			}
			outbounds[outCfg.Tag] = ob
		}
	}

	// Wrap all proxy outbounds with resilience (circuit breaker + retry)
	// then plugin chain (metrics, conntrack, logger).
	chain := e.obs.Chain()
	for tag, ob := range outbounds {
		if ob.Type() == "proxy" {
			tagName := tag // capture for closure
			wrapped := NewResilientOutbound(ob, ResilientOutboundConfig{
				CircuitBreaker: e.circuitBreaker,
				RetryConfig:    e.buildRetryConfig(cfg.Retry),
				OnRetry: func(attempt int, err error) {
					e.obs.Emit(Event{
						Type:      EventRetry,
						Timestamp: time.Now(),
						Message:   fmt.Sprintf("retry attempt %d for %s: %v", attempt, tagName, err),
					})
				},
			})
			if chain != nil {
				outbounds[tag] = NewChainOutbound(wrapped, chain)
			} else {
				outbounds[tag] = wrapped
			}
		}
	}

	// Second pass: build groups that reference other outbounds by tag.
	for _, outCfg := range cfg.Outbounds {
		if outCfg.Type != "group" {
			continue
		}
		groupCfg, err := parseOutboundGroupConfig(outCfg.Options)
		if err != nil {
			return nil, fmt.Errorf("outbound group %q: %w", outCfg.Tag, err)
		}
		members := make([]adapter.Outbound, 0, len(groupCfg.Outbounds))
		for _, memberTag := range groupCfg.Outbounds {
			ob, ok := outbounds[memberTag]
			if !ok {
				return nil, fmt.Errorf("outbound group %q: member %q not found", outCfg.Tag, memberTag)
			}
			members = append(members, ob)
		}
		grp := NewOutboundGroup(outCfg.Tag, groupCfg.Strategy, members)
		grp.qualityCfg = QualityConfigFromGroupConfig(groupCfg)
		grp.probeGetter = e.ProbeSnapshots
		outbounds[outCfg.Tag] = grp
	}

	// Reuse the router and DNS resolver built by startInternal.
	e.mu.RLock()
	rt := e.currentRouter
	dnsResolver := e.dnsResolver
	e.mu.RUnlock()

	ibRouter := e.traffic.BuildInboundRouter(rt, dnsResolver, outbounds, outbounds["proxy"])

	var closers []func() error
	var started []adapter.Inbound

	// On failure, only clean up what startInbounds created (inbounds + outbounds).
	// The caller (startInternal) handles sel, cancel, and state.
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
