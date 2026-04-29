// Package engine — metrics snapshot type.
package engine

import "time"

// MetricsSnapshot is a frozen view of engine-side metrics suitable for
// rendering by gui/api/routes_prometheus.go. Returned by Engine.Metrics().
type MetricsSnapshot struct {
	// Routing decisions: map["<decision>/<rule>"] -> count.
	RoutingDecisions map[string]int64

	// Per-outbound circuit breaker state: map["<outbound>"] -> "closed"|"open"|"half-open".
	CircuitBreakers map[string]string

	// Subscription refresh stats: map["<subscription_id>"] -> stats.
	Subscriptions map[string]SubscriptionStats

	// Per-transport handshake durations and failures (client-side dial).
	HandshakeDurationsNanos map[string][]int64 // map["<transport>"] -> observed nanos
	HandshakeFailures       map[string]int64   // map["<transport>/<reason>"] -> count

	// DNS query histogram (client-side resolver).
	DNSQueryDurationsNanos map[string][]int64 // map["<protocol>/<cached>"] -> observed nanos
}

// SubscriptionStats records refresh outcomes for a single subscription.
type SubscriptionStats struct {
	OK          int64
	Fail        int64
	LastRefresh time.Time
}
