package healthcheck

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestChecker_SingleCheck verifies that a successful HTTP check returns
// Available=true and a positive latency.
func TestChecker_SingleCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent) // 204
	}))
	defer srv.Close()

	checker := New(&Config{
		URL:       srv.URL,
		Timeout:   5 * time.Second,
		Tolerance: 3,
	})

	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		// Redirect all traffic to the test server.
		return net.Dial("tcp", srv.Listener.Addr().String())
	}

	result := checker.Check(context.Background(), "node-a", dial)

	if !result.Available {
		t.Errorf("expected Available=true, got false")
	}
	if result.Latency <= 0 {
		t.Errorf("expected Latency>0, got %v", result.Latency)
	}
	if result.UpdatedAt.IsZero() {
		t.Errorf("expected non-zero UpdatedAt")
	}
}

// TestChecker_FailedCheck verifies that a dial to an unreachable address
// marks the node as unavailable.
func TestChecker_FailedCheck(t *testing.T) {
	checker := New(&Config{
		URL:       "http://127.0.0.1:1/",
		Timeout:   2 * time.Second,
		Tolerance: 1, // one failure is enough to mark down
	})

	dial := DirectDialer()

	result := checker.Check(context.Background(), "node-b", dial)

	if result.Available {
		t.Errorf("expected Available=false for unreachable address")
	}

	// After 1 failure, Results() should also show unavailable (node never succeeded).
	results := checker.Results()
	r, ok := results["node-b"]
	if !ok {
		t.Fatal("expected node-b in results")
	}
	if r.Available {
		t.Errorf("Results() should show Available=false for a node that never succeeded")
	}
}

// TestChecker_ToleranceThreshold verifies that a node is only marked down
// after consecutiveFail >= tolerance.
func TestChecker_ToleranceThreshold(t *testing.T) {
	// Server state: first 2 requests fail (return 500), third succeeds (204).
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqCount.Add(1) <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	checker := New(&Config{
		URL:       srv.URL,
		Timeout:   5 * time.Second,
		Tolerance: 3, // must fail 3 times in a row to be marked down
	})

	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial("tcp", srv.Listener.Addr().String())
	}

	// First check: raw result is false (500), but node has never succeeded →
	// Results() should show Available=false (everSucceeded=false).
	r1 := checker.Check(context.Background(), "node-c", dial)
	if r1.Available {
		t.Errorf("check 1: raw result should be Available=false (500 response)")
	}
	got1, ok1 := checker.Result("node-c")
	if !ok1 {
		t.Fatal("node-c not found after first check")
	}
	if got1.Available {
		t.Errorf("Result() after check 1: expected Available=false (node never succeeded)")
	}

	// Second check: fails again (consecutiveFail=2, tolerance=3, everSucceeded=false).
	checker.Check(context.Background(), "node-c", dial)
	got2, _ := checker.Result("node-c")
	if got2.Available {
		t.Errorf("Result() after check 2: expected Available=false (still never succeeded)")
	}

	// Third check: succeeds (consecutiveFail resets to 0, everSucceeded=true).
	r3 := checker.Check(context.Background(), "node-c", dial)
	if !r3.Available {
		t.Errorf("check 3: expected raw Available=true after successful 204 response")
	}

	// After the success, Results() should show available.
	results := checker.Results()
	r, ok := results["node-c"]
	if !ok {
		t.Fatal("node-c not found in results")
	}
	if !r.Available {
		t.Errorf("Results(): expected Available=true after first success, got false")
	}
}

// TestChecker_Defaults verifies that zero-value Config fields get sensible defaults.
func TestChecker_Defaults(t *testing.T) {
	c := New(nil)
	cfg := c.Cfg()

	if cfg.URL != defaultURL {
		t.Errorf("URL: expected %q, got %q", defaultURL, cfg.URL)
	}
	if cfg.Interval != defaultInterval {
		t.Errorf("Interval: expected %v, got %v", defaultInterval, cfg.Interval)
	}
	if cfg.Timeout != defaultTimeout {
		t.Errorf("Timeout: expected %v, got %v", defaultTimeout, cfg.Timeout)
	}
	if cfg.Tolerance != defaultTolerance {
		t.Errorf("Tolerance: expected %d, got %d", defaultTolerance, cfg.Tolerance)
	}
}

// TestChecker_UnknownTag verifies that Result returns false for unknown tags.
func TestChecker_UnknownTag(t *testing.T) {
	c := New(nil)
	_, ok := c.Result("nonexistent")
	if ok {
		t.Errorf("expected ok=false for unknown tag")
	}
}
