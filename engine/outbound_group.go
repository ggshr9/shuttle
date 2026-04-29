package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"github.com/ggshr9/shuttle/adapter"
)

// GroupStrategy determines how an OutboundGroup selects members.
type GroupStrategy string

const (
	// GroupFailover tries outbounds in order; uses the first that succeeds.
	GroupFailover GroupStrategy = "failover"
	// GroupLoadBalance distributes connections round-robin across outbounds.
	GroupLoadBalance GroupStrategy = "loadbalance"
	// GroupQuality ranks outbounds by probe-measured latency and loss.
	GroupQuality GroupStrategy = "quality"
	// GroupURLTest auto-selects the lowest-latency available node via periodic health checks.
	GroupURLTest GroupStrategy = "url-test"
	// GroupSelect allows manual selection of the active outbound.
	GroupSelect GroupStrategy = "select"
)

// ProbeSnapshot is a point-in-time quality reading for an outbound.
type ProbeSnapshot struct {
	Latency   time.Duration
	Loss      float64
	Available bool
}

// OutboundGroupConfig is the JSON options schema for a group outbound.
type OutboundGroupConfig struct {
	Strategy             GroupStrategy     `json:"strategy"`
	Outbounds            []string          `json:"outbounds"`
	MaxLatency           string            `json:"max_latency,omitempty"`   // duration string, e.g. "200ms"
	MaxLossRate          float64           `json:"max_loss_rate,omitempty"` // 0-1 range
	QualityToleranceMS   int               `json:"quality_tolerance_ms,omitempty"` // for quality strategy; 0 -> default
	HealthCheck          *GroupHealthCheck `json:"health_check,omitempty"`         // for url-test strategy
}

// GroupHealthCheck configures the health checker for url-test groups.
type GroupHealthCheck struct {
	URL         string `json:"url,omitempty"`
	Interval    string `json:"interval,omitempty"`
	Timeout     string `json:"timeout,omitempty"`
	ToleranceMS int    `json:"tolerance_ms,omitempty"`
}

// OutboundGroup wraps multiple outbounds with failover, load-balance, or quality strategy.
type OutboundGroup struct {
	tag         string
	strategy    GroupStrategy
	outbounds   []adapter.Outbound
	counter     atomic.Uint64                    // for round-robin
	qualityCfg  QualityConfig                    // thresholds for quality strategy
	probeGetter func() map[string]ProbeSnapshot // returns latest probe data; nil when not quality
	urlTest     *urlTestState                    // for url-test strategy
	selectState *selectState                     // for select strategy
	logger      *slog.Logger
}

// NewOutboundGroup creates a new OutboundGroup.
func NewOutboundGroup(tag string, strategy GroupStrategy, outbounds []adapter.Outbound) *OutboundGroup {
	return &OutboundGroup{
		tag:       tag,
		strategy:  strategy,
		outbounds: outbounds,
	}
}

func (g *OutboundGroup) Tag() string  { return g.tag }
func (g *OutboundGroup) Type() string { return "group" }

// DialContext dials using the group's strategy.
//
// Failover: try each outbound in order; return first success or last error.
// LoadBalance: round-robin via atomic counter; if selected fails, try remaining in order.
func (g *OutboundGroup) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if len(g.outbounds) == 0 {
		return nil, fmt.Errorf("outbound group %q has no members", g.tag)
	}

	switch g.strategy {
	case GroupLoadBalance:
		return g.dialLoadBalance(ctx, network, address)
	case GroupQuality:
		return g.dialQuality(ctx, network, address)
	case GroupURLTest:
		return g.dialURLTest(ctx, network, address)
	case GroupSelect:
		return g.dialSelect(ctx, network, address)
	default: // failover is the default
		return g.dialFailover(ctx, network, address)
	}
}

func (g *OutboundGroup) dialFailover(ctx context.Context, network, address string) (net.Conn, error) {
	var lastErr error
	for _, ob := range g.outbounds {
		conn, err := ob.DialContext(ctx, network, address)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("outbound group %q: all %d members failed, last error: %w", g.tag, len(g.outbounds), lastErr)
}

func (g *OutboundGroup) dialLoadBalance(ctx context.Context, network, address string) (net.Conn, error) {
	n := uint64(len(g.outbounds))
	idx := g.counter.Add(1) - 1 // 0-based index for this call
	start := idx % n

	// Try the selected outbound first, then wrap around.
	var lastErr error
	for i := uint64(0); i < n; i++ {
		ob := g.outbounds[(start+i)%n]
		conn, err := ob.DialContext(ctx, network, address)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("outbound group %q: all %d members failed, last error: %w", g.tag, len(g.outbounds), lastErr)
}

func (g *OutboundGroup) dialQuality(ctx context.Context, network, address string) (net.Conn, error) {
	var probes map[string]ProbeSnapshot
	if g.probeGetter != nil {
		probes = g.probeGetter()
	}

	// Build quality entries from probe data.
	entries := make([]qualityEntry, len(g.outbounds))
	for i, ob := range g.outbounds {
		entry := qualityEntry{tag: ob.Tag(), index: i, latency: time.Second, loss: 1.0}
		if ps, ok := probes[ob.Tag()]; ok && ps.Available {
			entry.latency = ps.Latency
			entry.loss = ps.Loss
		}
		entries[i] = entry
	}

	// Filter and rank. rankByQuality shuffles entries within the
	// tolerance bucket per call so concurrent dials don't pile on #1.
	entries = filterByQuality(entries, g.qualityCfg)
	entries = rankByQuality(entries, g.qualityCfg)

	// Try ranked outbounds in order (failover among qualified).
	var lastErr error
	for _, entry := range entries {
		conn, err := g.outbounds[entry.index].DialContext(ctx, network, address)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, fmt.Errorf("outbound group %q: all %d quality-ranked members failed, last error: %w",
			g.tag, len(entries), lastErr)
	}
	return nil, fmt.Errorf("outbound group %q: no outbounds available", g.tag)
}

// Close stops any background loops (e.g. url-test health checks). Member outbounds
// are owned by the engine's outbound map and closed there; the group does not own
// their lifecycle.
func (g *OutboundGroup) Close() error {
	if g.urlTest != nil {
		g.urlTest.Stop()
	}
	return nil
}

// parseOutboundGroupConfig unmarshals OutboundGroupConfig from raw JSON.
func parseOutboundGroupConfig(raw json.RawMessage) (OutboundGroupConfig, error) {
	var cfg OutboundGroupConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parse group options: %w", err)
	}
	if len(cfg.Outbounds) == 0 {
		return cfg, fmt.Errorf("group must have at least one outbound member")
	}
	switch cfg.Strategy {
	case GroupFailover, GroupLoadBalance, GroupQuality, GroupURLTest, GroupSelect, "":
		// valid (empty defaults to failover)
	default:
		return cfg, fmt.Errorf("unknown group strategy: %q", cfg.Strategy)
	}
	if cfg.MaxLatency != "" {
		if _, err := time.ParseDuration(cfg.MaxLatency); err != nil {
			return cfg, fmt.Errorf("invalid max_latency %q: %w", cfg.MaxLatency, err)
		}
	}
	if cfg.MaxLossRate < 0 || cfg.MaxLossRate > 1 {
		return cfg, fmt.Errorf("max_loss_rate must be between 0 and 1, got %v", cfg.MaxLossRate)
	}
	return cfg, nil
}

// QualityConfigFromGroupConfig extracts a QualityConfig from the parsed group config.
func QualityConfigFromGroupConfig(cfg OutboundGroupConfig) QualityConfig {
	var qc QualityConfig
	if cfg.MaxLatency != "" {
		qc.MaxLatency, _ = time.ParseDuration(cfg.MaxLatency) // already validated
	}
	qc.MaxLossRate = cfg.MaxLossRate
	if cfg.QualityToleranceMS > 0 {
		qc.Tolerance = time.Duration(cfg.QualityToleranceMS) * time.Millisecond
	}
	return qc
}

// Compile-time interface check.
var _ adapter.Outbound = (*OutboundGroup)(nil)
