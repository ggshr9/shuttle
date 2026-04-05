package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync/atomic"

	"github.com/shuttleX/shuttle/adapter"
)

// GroupStrategy determines how an OutboundGroup selects members.
type GroupStrategy string

const (
	// GroupFailover tries outbounds in order; uses the first that succeeds.
	GroupFailover GroupStrategy = "failover"
	// GroupLoadBalance distributes connections round-robin across outbounds.
	GroupLoadBalance GroupStrategy = "loadbalance"
)

// OutboundGroupConfig is the JSON options schema for a group outbound.
type OutboundGroupConfig struct {
	Strategy  GroupStrategy `json:"strategy"`
	Outbounds []string      `json:"outbounds"`
}

// OutboundGroup wraps multiple outbounds with failover or load-balance strategy.
type OutboundGroup struct {
	tag       string
	strategy  GroupStrategy
	outbounds []adapter.Outbound
	counter   atomic.Uint64 // for round-robin
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

// Close closes all member outbounds.
func (g *OutboundGroup) Close() error {
	var firstErr error
	for _, ob := range g.outbounds {
		if err := ob.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
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
	return cfg, nil
}

// Compile-time interface check.
var _ adapter.Outbound = (*OutboundGroup)(nil)
