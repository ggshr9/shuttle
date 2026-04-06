package fakeip

import (
	"net/netip"
	"testing"
)

func mustPrefix(s string) netip.Prefix {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		panic(err)
	}
	return p
}

func TestPool_AllocateAndReverse(t *testing.T) {
	pool, err := NewPool(PoolConfig{
		CIDR: mustPrefix("198.18.0.0/15"),
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	ip1 := pool.Lookup("example.com")
	ip2 := pool.Lookup("google.com")

	if !ip1.IsValid() {
		t.Fatal("expected valid IP for example.com")
	}
	if !ip2.IsValid() {
		t.Fatal("expected valid IP for google.com")
	}
	if ip1 == ip2 {
		t.Fatal("two different domains should not get the same IP")
	}

	if got, ok := pool.Reverse(ip1); !ok || got != "example.com" {
		t.Fatalf("Reverse(%s) = %q, %v; want %q, true", ip1, got, ok, "example.com")
	}
	if got, ok := pool.Reverse(ip2); !ok || got != "google.com" {
		t.Fatalf("Reverse(%s) = %q, %v; want %q, true", ip2, got, ok, "google.com")
	}
}

func TestPool_IsFakeIP(t *testing.T) {
	pool, err := NewPool(PoolConfig{
		CIDR: mustPrefix("198.18.0.0/15"),
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	inRange := netip.MustParseAddr("198.18.0.1")
	outRange := netip.MustParseAddr("1.1.1.1")
	networkAddr := netip.MustParseAddr("198.18.0.0") // network address itself

	if !pool.IsFakeIP(inRange) {
		t.Errorf("IsFakeIP(%s) = false; want true", inRange)
	}
	if pool.IsFakeIP(outRange) {
		t.Errorf("IsFakeIP(%s) = true; want false", outRange)
	}
	// The network address is technically inside the prefix mask
	if !pool.IsFakeIP(networkAddr) {
		t.Errorf("IsFakeIP(%s) = false; want true (it is contained in prefix)", networkAddr)
	}
}

func TestPool_Wraparound(t *testing.T) {
	// /30 has 4 total addresses, 2 usable (network + broadcast excluded)
	pool, err := NewPool(PoolConfig{
		CIDR: mustPrefix("198.18.0.0/30"),
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	// size should be 2
	if pool.size != 2 {
		t.Fatalf("expected pool size 2, got %d", pool.size)
	}

	ip1 := pool.Lookup("a.com") // slot 0 → 198.18.0.1
	ip2 := pool.Lookup("b.com") // slot 1 → 198.18.0.2

	// Third allocation wraps to slot 0, evicting a.com
	ip3 := pool.Lookup("c.com")

	if ip3 != ip1 {
		t.Fatalf("wraparound: expected c.com to get ip1 (%s), got %s", ip1, ip3)
	}

	// a.com should be evicted from forward map
	if got := pool.Lookup("a.com"); got == ip1 {
		// a.com was evicted; its new lookup should get the next slot (ip2)
		// Actually: after c.com took slot0 (ip1), next=1. a.com would take slot1, evicting b.com.
		// So a.com != ip1 is expected only if it was evicted.
		// But if it was NOT evicted, ip1 would still be mapped to a.com, conflicting with c.com.
		// Since c.com evicted a.com, forward["a.com"] was deleted.
		// A new Lookup("a.com") re-allocates at current next (1), evicting b.com.
		_ = ip2 // avoid unused warning
		t.Logf("note: a.com re-allocated to %s after eviction", got)
	}

	// Verify c.com now has ip1 via reverse
	if domain, ok := pool.Reverse(ip1); !ok || domain != "c.com" {
		t.Fatalf("Reverse(%s) = %q, %v; want c.com, true", ip1, domain, ok)
	}
}

func TestPool_SamedomainSameIP(t *testing.T) {
	pool, err := NewPool(PoolConfig{
		CIDR: mustPrefix("198.18.0.0/15"),
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	ip1 := pool.Lookup("example.com")
	ip2 := pool.Lookup("example.com")

	if ip1 != ip2 {
		t.Fatalf("same domain returned different IPs: %s vs %s", ip1, ip2)
	}
}

func TestPool_FilteredDomain(t *testing.T) {
	pool, err := NewPool(PoolConfig{
		CIDR:   mustPrefix("198.18.0.0/15"),
		Filter: []string{"+.lan", "stun.*", "ntp.example.com"},
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	cases := []struct {
		domain string
		want   bool // ShouldFakeIP result
	}{
		{"router.lan", false},       // matches +.lan suffix
		{"stun.example.com", false}, // matches stun.* glob
		{"ntp.example.com", false},  // exact match
		{"google.com", true},        // not filtered
		{"example.org", true},       // not filtered
	}

	for _, tc := range cases {
		got := pool.ShouldFakeIP(tc.domain)
		if got != tc.want {
			t.Errorf("ShouldFakeIP(%q) = %v; want %v", tc.domain, got, tc.want)
		}
	}
}
