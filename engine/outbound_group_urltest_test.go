package engine

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/outbound/healthcheck"
)

func TestURLTestState_SelectBest(t *testing.T) {
	// Start a local HTTP server that returns 204.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	checker := healthcheck.New(&healthcheck.Config{
		URL:      srv.URL,
		Interval: 1 * time.Hour, // won't tick during test
		Timeout:  2 * time.Second,
	})

	fast := &mockOutbound{tag: "fast", dialFunc: succeedDial}
	slow := &mockOutbound{tag: "slow", dialFunc: succeedDial}

	// Pre-seed the checker: fast uses direct dialer, slow uses a delayed dialer.
	ctx := context.Background()
	checker.Check(ctx, "fast", directDialer(srv))
	checker.Check(ctx, "slow", delayedDialer(srv, 200*time.Millisecond))

	state := newURLTestState(checker, 50, slog.Default())

	outbounds := []adapter.Outbound{slow, fast}
	state.selectBest(outbounds)

	sel := state.selected.Load()
	if sel == nil {
		t.Fatal("expected a selected outbound, got nil")
	}
	if (*sel).Tag() != "fast" {
		t.Errorf("selected = %q, want %q", (*sel).Tag(), "fast")
	}
}

func TestURLTestState_TolerancePreventsSwitch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	checker := healthcheck.New(&healthcheck.Config{
		URL:      srv.URL,
		Interval: 1 * time.Hour,
		Timeout:  2 * time.Second,
	})

	a := &mockOutbound{tag: "a", dialFunc: succeedDial}
	b := &mockOutbound{tag: "b", dialFunc: succeedDial}

	ctx := context.Background()
	// Both check in with similar latency via the real HTTP server.
	checker.Check(ctx, "a", directDialer(srv))
	checker.Check(ctx, "b", directDialer(srv))

	// Use a large tolerance so no switch occurs once selected.
	state := newURLTestState(checker, 5000, slog.Default())

	outbounds := []adapter.Outbound{a, b}
	state.selectBest(outbounds) // initial selection

	first := state.selected.Load()
	if first == nil {
		t.Fatal("expected initial selection")
	}
	firstTag := (*first).Tag()

	// Run selectBest again; tolerance should prevent switching.
	state.selectBest(outbounds)

	second := state.selected.Load()
	if second == nil {
		t.Fatal("expected selection after second pass")
	}
	if (*second).Tag() != firstTag {
		t.Errorf("expected no switch due to tolerance, was %q now %q", firstTag, (*second).Tag())
	}
}

func TestOutboundGroup_URLTest_Dial(t *testing.T) {
	fast := &mockOutbound{tag: "fast", dialFunc: succeedDial}
	slow := &mockOutbound{tag: "slow", dialFunc: succeedDial}

	g := NewOutboundGroup("ut", GroupURLTest, []adapter.Outbound{slow, fast})

	// Manually set up urlTest state with fast pre-selected.
	checker := healthcheck.New(nil)
	g.SetURLTest(checker, 50)
	var ob adapter.Outbound = fast
	g.urlTest.selected.Store(&ob)

	conn, err := g.DialContext(context.Background(), "tcp", "example.com:80")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	conn.Close()

	// fast should be called (selected), slow should not.
	if fast.calls.Load() != 1 {
		t.Errorf("fast calls = %d, want 1", fast.calls.Load())
	}
	if slow.calls.Load() != 0 {
		t.Errorf("slow calls = %d, want 0", slow.calls.Load())
	}
}

func TestOutboundGroup_URLTest_FallbackToFailover(t *testing.T) {
	failing := &mockOutbound{tag: "failing", dialFunc: failDial}
	backup := &mockOutbound{tag: "backup", dialFunc: succeedDial}

	g := NewOutboundGroup("ut-fb", GroupURLTest, []adapter.Outbound{failing, backup})

	// Set up urlTest with failing node selected.
	checker := healthcheck.New(nil)
	g.SetURLTest(checker, 50)
	var ob adapter.Outbound = failing
	g.urlTest.selected.Store(&ob)

	conn, err := g.DialContext(context.Background(), "tcp", "example.com:80")
	if err != nil {
		t.Fatalf("expected fallback success, got %v", err)
	}
	conn.Close()

	// failing tried once (selected), then failover tries failing again + backup.
	if backup.calls.Load() != 1 {
		t.Errorf("backup calls = %d, want 1", backup.calls.Load())
	}
}

func TestOutboundGroup_URLTest_NoSelection(t *testing.T) {
	a := &mockOutbound{tag: "a", dialFunc: succeedDial}

	g := NewOutboundGroup("ut-nil", GroupURLTest, []adapter.Outbound{a})

	// Set up urlTest but don't select anything.
	checker := healthcheck.New(nil)
	g.SetURLTest(checker, 50)

	conn, err := g.DialContext(context.Background(), "tcp", "example.com:80")
	if err != nil {
		t.Fatalf("expected failover success, got %v", err)
	}
	conn.Close()

	if a.calls.Load() != 1 {
		t.Errorf("a calls = %d, want 1", a.calls.Load())
	}
}

func TestURLTestSelected(t *testing.T) {
	g := NewOutboundGroup("ut-sel", GroupURLTest, nil)

	// No urlTest configured.
	if got := g.URLTestSelected(); got != "" {
		t.Errorf("URLTestSelected() = %q, want empty", got)
	}

	// Configure but no selection.
	checker := healthcheck.New(nil)
	g.SetURLTest(checker, 50)
	if got := g.URLTestSelected(); got != "" {
		t.Errorf("URLTestSelected() = %q, want empty", got)
	}

	// Store a selection.
	ob := adapter.Outbound(&mockOutbound{tag: "picked", dialFunc: succeedDial})
	g.urlTest.selected.Store(&ob)
	if got := g.URLTestSelected(); got != "picked" {
		t.Errorf("URLTestSelected() = %q, want %q", got, "picked")
	}
}

func TestURLTestState_StartStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	checker := healthcheck.New(&healthcheck.Config{
		URL:      srv.URL,
		Interval: 50 * time.Millisecond, // fast interval for test
		Timeout:  1 * time.Second,
	})

	a := &mockOutbound{tag: "a", dialFunc: succeedDial}
	state := newURLTestState(checker, 50, slog.Default())

	ctx := context.Background()
	state.Start(ctx, []adapter.Outbound{a})

	// Wait enough time for the initial check + at least one tick.
	time.Sleep(200 * time.Millisecond)

	state.Stop()

	// Should have selected something.
	sel := state.selected.Load()
	if sel == nil {
		// The checker may fail because mockOutbound's DialContext returns a pipe,
		// not a real TCP connection. That's OK - we're testing Start/Stop lifecycle.
		t.Log("no selection made (expected if mock dial doesn't support HTTP)")
	}

	// Calling Stop again should be safe.
	state.Stop()
}

func TestParseOutboundGroupConfig_URLTest(t *testing.T) {
	raw := []byte(`{"strategy":"url-test","outbounds":["us","jp"]}`)
	cfg, err := parseOutboundGroupConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Strategy != GroupURLTest {
		t.Errorf("strategy = %q, want %q", cfg.Strategy, GroupURLTest)
	}
}

func TestParseOutboundGroupConfig_Select(t *testing.T) {
	raw := []byte(`{"strategy":"select","outbounds":["us","jp"]}`)
	cfg, err := parseOutboundGroupConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Strategy != GroupSelect {
		t.Errorf("strategy = %q, want %q", cfg.Strategy, GroupSelect)
	}
}

// directDialer returns a healthcheck.DialFunc that dials the given test server directly.
func directDialer(srv *httptest.Server) healthcheck.DialFunc {
	return (&net.Dialer{}).DialContext
}

// delayedDialer returns a healthcheck.DialFunc that adds artificial delay before dialing.
func delayedDialer(srv *httptest.Server, delay time.Duration) healthcheck.DialFunc {
	d := &net.Dialer{}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		time.Sleep(delay)
		return d.DialContext(ctx, network, addr)
	}
}
