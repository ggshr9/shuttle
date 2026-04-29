package engine

import (
	"testing"
	"time"

	"github.com/shuttleX/shuttle/config"
)

// TestEngine_MetricsZeroValueSnapshot verifies a freshly-constructed engine
// returns a snapshot whose maps are non-nil (callers may safely range/index)
// and empty.
func TestEngine_MetricsZeroValueSnapshot(t *testing.T) {
	eng := New(config.DefaultClientConfig())
	snap := eng.Metrics()

	if snap.RoutingDecisions == nil {
		t.Error("RoutingDecisions should be a non-nil empty map")
	}
	if len(snap.RoutingDecisions) != 0 {
		t.Errorf("RoutingDecisions len = %d, want 0", len(snap.RoutingDecisions))
	}
	if snap.CircuitBreakers == nil {
		t.Error("CircuitBreakers should be non-nil")
	}
	if len(snap.CircuitBreakers) != 0 {
		t.Errorf("CircuitBreakers len = %d, want 0", len(snap.CircuitBreakers))
	}
	if snap.Subscriptions == nil {
		t.Error("Subscriptions should be non-nil")
	}
	if len(snap.Subscriptions) != 0 {
		t.Errorf("Subscriptions len = %d, want 0", len(snap.Subscriptions))
	}
	if snap.HandshakeDurationsNanos == nil {
		t.Error("HandshakeDurationsNanos should be non-nil")
	}
	if snap.HandshakeFailures == nil {
		t.Error("HandshakeFailures should be non-nil")
	}
	if snap.DNSQueryDurationsNanos == nil {
		t.Error("DNSQueryDurationsNanos should be non-nil")
	}
}

// TestEngine_MetricsSnapshotIsDeepCopy verifies that mutating the returned
// snapshot does not affect the engine's internal storage. This is essential
// for Tasks 9-13 which will populate the maps concurrently with readers.
func TestEngine_MetricsSnapshotIsDeepCopy(t *testing.T) {
	eng := New(config.DefaultClientConfig())

	// Seed the internal storage by direct access (we're in the same package).
	eng.metrics.mu.Lock()
	eng.metrics.routingDecisions["proxy/rule-1"] = 7
	eng.metrics.circuitBreakers["out-1"] = "closed"
	eng.metrics.subscriptions["sub-1"] = SubscriptionStats{OK: 3, Fail: 1, LastRefresh: time.Unix(1700000000, 0)}
	eng.metrics.handshakeDurations["h3"] = []int64{100, 200, 300}
	eng.metrics.handshakeFailures["h3/timeout"] = 2
	eng.metrics.dnsDurations["doh/cached"] = []int64{50}
	eng.metrics.mu.Unlock()

	snap := eng.Metrics()

	// Sanity: the seeded values appear in the snapshot.
	if snap.RoutingDecisions["proxy/rule-1"] != 7 {
		t.Errorf("RoutingDecisions[proxy/rule-1] = %d, want 7", snap.RoutingDecisions["proxy/rule-1"])
	}
	if snap.CircuitBreakers["out-1"] != "closed" {
		t.Errorf("CircuitBreakers[out-1] = %q, want closed", snap.CircuitBreakers["out-1"])
	}
	if got := snap.Subscriptions["sub-1"]; got.OK != 3 || got.Fail != 1 {
		t.Errorf("Subscriptions[sub-1] = %+v, want OK=3 Fail=1", got)
	}
	if len(snap.HandshakeDurationsNanos["h3"]) != 3 {
		t.Errorf("HandshakeDurationsNanos[h3] len = %d, want 3", len(snap.HandshakeDurationsNanos["h3"]))
	}

	// Mutate every field in the snapshot.
	snap.RoutingDecisions["proxy/rule-1"] = 999
	snap.RoutingDecisions["new-key"] = 1
	snap.CircuitBreakers["out-1"] = "open"
	snap.CircuitBreakers["new-key"] = "half-open"
	snap.Subscriptions["sub-1"] = SubscriptionStats{OK: 999}
	snap.Subscriptions["new-sub"] = SubscriptionStats{OK: 1}
	snap.HandshakeDurationsNanos["h3"][0] = 99999 // mutate slice element
	snap.HandshakeDurationsNanos["new-transport"] = []int64{1}
	snap.HandshakeFailures["h3/timeout"] = 999
	snap.DNSQueryDurationsNanos["doh/cached"][0] = 99999

	// Re-fetch and verify the engine's internal state was untouched.
	snap2 := eng.Metrics()

	if snap2.RoutingDecisions["proxy/rule-1"] != 7 {
		t.Errorf("after mutation, internal RoutingDecisions[proxy/rule-1] = %d, want 7", snap2.RoutingDecisions["proxy/rule-1"])
	}
	if _, ok := snap2.RoutingDecisions["new-key"]; ok {
		t.Error("after mutation, snapshot leaked new key into internal RoutingDecisions")
	}
	if snap2.CircuitBreakers["out-1"] != "closed" {
		t.Errorf("after mutation, internal CircuitBreakers[out-1] = %q, want closed", snap2.CircuitBreakers["out-1"])
	}
	if _, ok := snap2.CircuitBreakers["new-key"]; ok {
		t.Error("after mutation, snapshot leaked new key into internal CircuitBreakers")
	}
	if got := snap2.Subscriptions["sub-1"]; got.OK != 3 || got.Fail != 1 {
		t.Errorf("after mutation, internal Subscriptions[sub-1] = %+v, want OK=3 Fail=1", got)
	}
	if _, ok := snap2.Subscriptions["new-sub"]; ok {
		t.Error("after mutation, snapshot leaked new key into internal Subscriptions")
	}
	if snap2.HandshakeDurationsNanos["h3"][0] != 100 {
		t.Errorf("after mutation, internal HandshakeDurationsNanos[h3][0] = %d, want 100", snap2.HandshakeDurationsNanos["h3"][0])
	}
	if _, ok := snap2.HandshakeDurationsNanos["new-transport"]; ok {
		t.Error("after mutation, snapshot leaked new key into internal HandshakeDurationsNanos")
	}
	if snap2.HandshakeFailures["h3/timeout"] != 2 {
		t.Errorf("after mutation, internal HandshakeFailures[h3/timeout] = %d, want 2", snap2.HandshakeFailures["h3/timeout"])
	}
	if snap2.DNSQueryDurationsNanos["doh/cached"][0] != 50 {
		t.Errorf("after mutation, internal DNSQueryDurationsNanos[doh/cached][0] = %d, want 50", snap2.DNSQueryDurationsNanos["doh/cached"][0])
	}
}
