package admin

import (
	"testing"
	"time"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(1, 2)

	// First two requests should be allowed (burst=2).
	if !rl.Allow("1.2.3.4") {
		t.Fatal("expected first request to be allowed")
	}
	if !rl.Allow("1.2.3.4") {
		t.Fatal("expected second request to be allowed")
	}
	// Third request should be denied (no tokens left).
	if rl.Allow("1.2.3.4") {
		t.Fatal("expected third request to be denied")
	}
}

func TestRateLimiterBurst(t *testing.T) {
	rl := NewRateLimiter(100, 5)

	// Should allow exactly burst-count requests immediately.
	for i := 0; i < 5; i++ {
		if !rl.Allow("10.0.0.1") {
			t.Fatalf("expected request %d to be allowed", i+1)
		}
	}
	if rl.Allow("10.0.0.1") {
		t.Fatal("expected request beyond burst to be denied")
	}
}

func TestRateLimiterRefill(t *testing.T) {
	rl := NewRateLimiter(10, 2) // 10 tokens/sec, burst 2

	// Exhaust tokens.
	rl.Allow("5.5.5.5")
	rl.Allow("5.5.5.5")
	if rl.Allow("5.5.5.5") {
		t.Fatal("expected denial after exhaustion")
	}

	// Simulate time passing by adjusting the bucket's last timestamp.
	rl.mu.Lock()
	rl.ips["5.5.5.5"].last = time.Now().Add(-200 * time.Millisecond) // should refill ~2 tokens
	rl.mu.Unlock()

	if !rl.Allow("5.5.5.5") {
		t.Fatal("expected allow after refill")
	}
}

func TestRateLimiterDifferentIPs(t *testing.T) {
	rl := NewRateLimiter(1, 1)

	if !rl.Allow("1.1.1.1") {
		t.Fatal("expected first IP first request allowed")
	}
	if rl.Allow("1.1.1.1") {
		t.Fatal("expected first IP second request denied")
	}

	// Different IP should have its own bucket.
	if !rl.Allow("2.2.2.2") {
		t.Fatal("expected second IP first request allowed")
	}
	if rl.Allow("2.2.2.2") {
		t.Fatal("expected second IP second request denied")
	}
}

func TestRateLimiterCleanup(t *testing.T) {
	rl := NewRateLimiter(1, 5)

	rl.Allow("old.ip")
	rl.Allow("fresh.ip")

	// Age the "old.ip" entry beyond 10 minutes.
	rl.mu.Lock()
	rl.ips["old.ip"].last = time.Now().Add(-11 * time.Minute)
	rl.mu.Unlock()

	rl.cleanup()

	rl.mu.Lock()
	_, hasOld := rl.ips["old.ip"]
	_, hasFresh := rl.ips["fresh.ip"]
	rl.mu.Unlock()

	if hasOld {
		t.Fatal("expected old.ip to be cleaned up")
	}
	if !hasFresh {
		t.Fatal("expected fresh.ip to remain")
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"192.168.1.1:8080", "192.168.1.1"},
		{"[::1]:443", "::1"},
		{"10.0.0.1", "10.0.0.1"},
	}
	for _, tt := range tests {
		got := extractIP(tt.input)
		if got != tt.want {
			t.Errorf("extractIP(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
