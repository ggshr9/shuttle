// Package engine — internal storage and accessor for engine-side metrics.
//
// The storage backs Engine.Metrics(), which returns a deep-copied
// MetricsSnapshot (see metrics_snapshot.go). Hooks that populate the maps
// are wired up in subsequent tasks (router decision hook, circuit breaker
// callback, subscription refresh hook, client transport / DNS hooks); this
// file only defines the storage type, lifecycle, and snapshot accessor.
package engine

import "sync"

// engineMetrics is the internal mutable storage backing Engine.Metrics().
// Counters here are monotonically increasing across reloads — Reload()
// must NOT reset them.
type engineMetrics struct {
	mu sync.Mutex

	routingDecisions   map[string]int64
	circuitBreakers    map[string]string
	subscriptions      map[string]SubscriptionStats
	handshakeDurations map[string][]int64
	handshakeFailures  map[string]int64
	dnsDurations       map[string][]int64
}

// newEngineMetrics constructs an engineMetrics with all sub-maps allocated.
// Callers can therefore read or write any field without a nil-map check.
func newEngineMetrics() *engineMetrics {
	return &engineMetrics{
		routingDecisions:   make(map[string]int64),
		circuitBreakers:    make(map[string]string),
		subscriptions:      make(map[string]SubscriptionStats),
		handshakeDurations: make(map[string][]int64),
		handshakeFailures:  make(map[string]int64),
		dnsDurations:       make(map[string][]int64),
	}
}

// Metrics returns a snapshot of engine-side metrics. Cheap, lock-protected.
// Used by gui/api/routes_prometheus.go to render Prometheus exposition.
//
// The returned snapshot is a deep copy: mutating any map or slice on the
// returned value does not affect the engine's internal storage.
func (e *Engine) Metrics() MetricsSnapshot {
	e.metrics.mu.Lock()
	defer e.metrics.mu.Unlock()
	return MetricsSnapshot{
		RoutingDecisions:        copyInt64Map(e.metrics.routingDecisions),
		CircuitBreakers:         copyStringMap(e.metrics.circuitBreakers),
		Subscriptions:           copySubscriptionStats(e.metrics.subscriptions),
		HandshakeDurationsNanos: copyInt64SliceMap(e.metrics.handshakeDurations),
		HandshakeFailures:       copyInt64Map(e.metrics.handshakeFailures),
		DNSQueryDurationsNanos:  copyInt64SliceMap(e.metrics.dnsDurations),
	}
}

func copyInt64Map(m map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyStringMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copySubscriptionStats(m map[string]SubscriptionStats) map[string]SubscriptionStats {
	out := make(map[string]SubscriptionStats, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyInt64SliceMap(m map[string][]int64) map[string][]int64 {
	out := make(map[string][]int64, len(m))
	for k, v := range m {
		dup := make([]int64, len(v))
		copy(dup, v)
		out[k] = dup
	}
	return out
}

// SeedMetricsForTest overwrites the engine's internal metrics storage with
// the values from the given snapshot. This is intended for unit tests in
// other packages (e.g. gui/api) that need to render a populated metrics
// view without driving real handshakes / routing / refreshes. Production
// code should not call this — metrics are populated by hooks installed at
// engine startup. The snapshot is deep-copied so the caller's maps are
// not aliased.
func (e *Engine) SeedMetricsForTest(seed MetricsSnapshot) {
	e.metrics.mu.Lock()
	defer e.metrics.mu.Unlock()
	if seed.RoutingDecisions != nil {
		e.metrics.routingDecisions = copyInt64Map(seed.RoutingDecisions)
	}
	if seed.CircuitBreakers != nil {
		e.metrics.circuitBreakers = copyStringMap(seed.CircuitBreakers)
	}
	if seed.Subscriptions != nil {
		e.metrics.subscriptions = copySubscriptionStats(seed.Subscriptions)
	}
	if seed.HandshakeDurationsNanos != nil {
		e.metrics.handshakeDurations = copyInt64SliceMap(seed.HandshakeDurationsNanos)
	}
	if seed.HandshakeFailures != nil {
		e.metrics.handshakeFailures = copyInt64Map(seed.HandshakeFailures)
	}
	if seed.DNSQueryDurationsNanos != nil {
		e.metrics.dnsDurations = copyInt64SliceMap(seed.DNSQueryDurationsNanos)
	}
}
