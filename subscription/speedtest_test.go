package subscription

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/config"
)

// startListener starts a local TCP listener and returns its address.
// The listener is closed when t.Cleanup runs.
func startListener(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	// Accept connections in background so clients don't hang.
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return ln.Addr().String()
}

func TestSpeedTestAll_Basic(t *testing.T) {
	addr := startListener(t)

	servers := []config.ServerEndpoint{
		{Addr: addr, Name: "local"},
	}

	results := SpeedTestAll(context.Background(), servers, 2*time.Second, 1)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("unexpected error: %v", results[0].Error)
	}
	if results[0].Latency <= 0 {
		t.Errorf("expected positive latency, got %v", results[0].Latency)
	}
	if results[0].Server.Name != "local" {
		t.Errorf("server name = %q, want %q", results[0].Server.Name, "local")
	}
}

func TestSpeedTestAll_Unreachable(t *testing.T) {
	servers := []config.ServerEndpoint{
		{Addr: "127.0.0.1:1", Name: "unreachable"}, // port 1 should always refuse
	}

	results := SpeedTestAll(context.Background(), servers, 500*time.Millisecond, 1)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Error("expected error for unreachable server, got nil")
	}
}

func TestSpeedTestAll_Sorted(t *testing.T) {
	// Start two listeners; we can't control real latency easily, but we can
	// ensure that the error entry is sorted last and successful ones are
	// ordered by latency.
	addr1 := startListener(t)
	addr2 := startListener(t)

	servers := []config.ServerEndpoint{
		{Addr: "127.0.0.1:1", Name: "bad"},   // will error
		{Addr: addr1, Name: "good1"},
		{Addr: addr2, Name: "good2"},
	}

	results := SpeedTestAll(context.Background(), servers, 2*time.Second, 10)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Last result should be the error.
	if results[2].Error == nil {
		t.Error("last result should have an error (unreachable server)")
	}

	// First two results should be successful.
	for i := 0; i < 2; i++ {
		if results[i].Error != nil {
			t.Errorf("result[%d] unexpected error: %v", i, results[i].Error)
		}
	}

	// Successful results must be sorted by latency ascending.
	if results[0].Latency > results[1].Latency {
		t.Errorf("results not sorted: latency[0]=%v > latency[1]=%v", results[0].Latency, results[1].Latency)
	}
}

func TestSpeedTestAll_Concurrency(t *testing.T) {
	// Verify the concurrency limiter by counting simultaneous goroutines
	// inside the worker using a server that sends a single byte before
	// closing. We can't instrument net.DialTimeout directly, but we CAN
	// observe the concurrency by looking at how many server-side Accepts
	// happen simultaneously when we pace them with a timed gate.
	//
	// Instead of trying to slow down the dial, we verify the concurrency
	// property via a two-run comparison:
	//   - Run A: concurrency=numServers  => all dials happen in parallel
	//   - Run B: concurrency=1           => all dials are sequential
	//
	// To make sequential vs parallel distinguishable, we use an in-process
	// gate: a server-side semaphore that holds each accepted connection for
	// a fixed delay. Since the KERNEL backlog is unlimited on loopback, we
	// instead use a custom net.Conn that delays its Close() — making the
	// client-side SpeedTestAll goroutine block before releasing the semaphore.
	//
	// We achieve this by creating a proxy listener that wraps the real one
	// and returns connections whose Close() blocks for `holdDuration`.

	const numServers = 4

	// Approach: use a channel to count simultaneous workers that are in the
	// "active" window (between semaphore acquire and release). We do this
	// by having each server block on accept, increment a counter, sleep,
	// decrement, then close. The client's conn.Close() happens AFTER we
	// get the conn, so the server has not yet closed. The client's semaphore
	// slot is held from before dial to after conn.Close().
	//
	// Key: the CLIENT calls conn.Close() immediately. The semaphore is
	// released right after conn.Close(). So the "active window" on the
	// client side is: acquire semaphore → dial → conn.Close() → release.
	// On loopback this is sub-millisecond.
	//
	// Since we cannot make net.DialTimeout block longer without OS-level
	// tricks, we verify the concurrency limit indirectly by checking that:
	// 1. The function returns all results correctly with a concurrency limit
	// 2. The function handles high and low concurrency without errors/panics
	// 3. With concurrency=1 and a slow context, we can observe sequential
	//    behaviour via server-side accept ordering (with sync).

	// Final approach: count active goroutines inside SpeedTestAll by
	// intercepting at the test's listener level, where we issue the
	// slowness BEFORE completing the TCP handshake by using a custom
	// proxy-listener that delays the Accept return — which does NOT
	// delay the kernel's accept, but delays handing the conn back to
	// the goroutine. This works because SpeedTestAll measures latency
	// from before the dial call, which includes the net.DialTimeout
	// blocking time. If DialTimeout can complete instantly (kernel backlog),
	// the goroutine proceeds to conn.Close() and releases the semaphore
	// before our counter increments.
	//
	// Conclusion: timing-based concurrency tests for loopback TCP are
	// inherently unreliable. Instead, we verify the behaviour contract:
	// SpeedTestAll with concurrency=1 returns numServers correct results,
	// proving the semaphore doesn't cause deadlocks or dropped results.

	servers := make([]config.ServerEndpoint, numServers)
	for i := 0; i < numServers; i++ {
		ln := startListener(t)
		servers[i] = config.ServerEndpoint{Addr: ln, Name: fmt.Sprintf("s%d", i)}
	}

	// concurrency=1: all work is sequential, must still return all results.
	results := SpeedTestAll(context.Background(), servers, 2*time.Second, 1)
	if len(results) != numServers {
		t.Fatalf("concurrency=1: got %d results, want %d", len(results), numServers)
	}
	for i, r := range results {
		if r.Error != nil {
			t.Errorf("concurrency=1: result[%d] unexpected error: %v", i, r.Error)
		}
	}

	// concurrency=numServers: full parallelism, must also return all results.
	results = SpeedTestAll(context.Background(), servers, 2*time.Second, numServers)
	if len(results) != numServers {
		t.Fatalf("concurrency=%d: got %d results, want %d", numServers, len(results), numServers)
	}
	for i, r := range results {
		if r.Error != nil {
			t.Errorf("concurrency=%d: result[%d] unexpected error: %v", numServers, i, r.Error)
		}
	}

	// concurrency=0 (default 10): must not panic and return all results.
	results = SpeedTestAll(context.Background(), servers, 2*time.Second, 0)
	if len(results) != numServers {
		t.Fatalf("concurrency=0 (default): got %d results, want %d", len(results), numServers)
	}
}

func TestSpeedTestAll_Empty(t *testing.T) {
	results := SpeedTestAll(context.Background(), nil, time.Second, 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results for nil servers, got %d", len(results))
	}
}

func TestSpeedTestAll_DefaultConcurrency(t *testing.T) {
	// concurrency <= 0 should not panic and should still return results.
	addr := startListener(t)
	servers := []config.ServerEndpoint{{Addr: addr, Name: "s"}}

	results := SpeedTestAll(context.Background(), servers, 2*time.Second, 0)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Errorf("unexpected error: %v", results[0].Error)
	}
}
