package selector

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shuttle-proxy/shuttle/transport"
)

func TestSelectorAutoStrategy(t *testing.T) {
	h3 := &fakeTransport{typeName: "h3", conn: &fakeConn{}}
	reality := &fakeTransport{typeName: "reality", conn: &fakeConn{}}
	cdn := &fakeTransport{typeName: "cdn", conn: &fakeConn{}}
	transports := []transport.ClientTransport{h3, reality, cdn}

	s := New(transports, &Config{Strategy: StrategyAuto}, nil)

	conn, err := s.Dial(context.Background(), "localhost:443")
	if err != nil {
		t.Fatalf("Dial with auto strategy failed: %v", err)
	}
	if conn == nil {
		t.Fatal("Dial returned nil connection")
	}
	// Auto strategy behaves like priority: picks first available.
	if got := s.ActiveTransport(); got != "h3" {
		t.Fatalf("ActiveTransport = %s, want h3 (first in priority order)", got)
	}
}

func TestSelectorLatencyDynamic(t *testing.T) {
	h3 := &fakeTransport{typeName: "h3", conn: &fakeConn{}}
	reality := &fakeTransport{typeName: "reality", conn: &fakeConn{}}
	transports := []transport.ClientTransport{h3, reality}

	s := New(transports, &Config{Strategy: StrategyLatency}, nil)

	// Set h3 as fastest initially.
	s.mu.Lock()
	s.probes["h3"].Latency = 10 * time.Millisecond
	s.probes["h3"].Available = true
	s.probes["reality"].Latency = 100 * time.Millisecond
	s.probes["reality"].Available = true
	s.mu.Unlock()

	s.maybeSwitch()

	if got := s.ActiveTransport(); got != "h3" {
		t.Fatalf("after initial probes: ActiveTransport = %s, want h3", got)
	}

	// Now make reality fastest.
	s.mu.Lock()
	s.probes["h3"].Latency = 200 * time.Millisecond
	s.probes["reality"].Latency = 5 * time.Millisecond
	s.mu.Unlock()

	s.maybeSwitch()

	if got := s.ActiveTransport(); got != "reality" {
		t.Fatalf("after updated probes: ActiveTransport = %s, want reality", got)
	}
}

func TestSelectorMultipathDial(t *testing.T) {
	h3 := &fakeTransport{typeName: "h3", conn: &fakeConn{}}
	reality := &fakeTransport{typeName: "reality", conn: &fakeConn{}}
	transports := []transport.ClientTransport{h3, reality}

	s := New(transports, &Config{
		Strategy:   StrategyMultipath,
		ServerAddr: "localhost:443",
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	// Allow pool initialization to complete.
	time.Sleep(50 * time.Millisecond)

	conn, err := s.Dial(ctx, "localhost:443")
	if err != nil {
		t.Fatalf("Dial with multipath strategy failed: %v", err)
	}
	if conn == nil {
		t.Fatal("Dial returned nil connection")
	}

	// Verify we can open a stream on the multipath connection.
	stream, err := conn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream on multipath conn failed: %v", err)
	}
	stream.Close()

	paths := s.ActivePaths()
	if paths == nil {
		t.Fatal("ActivePaths returned nil in multipath mode")
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 active paths, got %d", len(paths))
	}

	if got := s.ActiveTransport(); got != "multipath" {
		t.Fatalf("ActiveTransport = %s, want multipath", got)
	}

	s.Close()
}

func TestSelectorConcurrentDial(t *testing.T) {
	h3 := &fakeTransport{typeName: "h3", conn: &fakeConn{}}
	reality := &fakeTransport{typeName: "reality", conn: &fakeConn{}}
	transports := []transport.ClientTransport{h3, reality}

	s := New(transports, &Config{Strategy: StrategyPriority}, nil)

	const goroutines = 10
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			conn, err := s.Dial(context.Background(), "localhost:443")
			if err != nil {
				errs <- err
				return
			}
			if conn == nil {
				errs <- errNilConn
				return
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent Dial failed: %v", err)
	}

	// After all dials complete, active transport should be set.
	if got := s.ActiveTransport(); got == "none" {
		t.Fatal("expected active transport to be set after concurrent dials")
	}
}

var errNilConn = errorString("Dial returned nil connection")

type errorString string

func (e errorString) Error() string { return string(e) }

func TestSelectorProbeUpdatesAvailability(t *testing.T) {
	h3 := &fakeTransport{typeName: "h3", conn: &fakeConn{}}
	reality := &fakeTransport{typeName: "reality", conn: &fakeConn{}}
	transports := []transport.ClientTransport{h3, reality}

	s := New(transports, &Config{Strategy: StrategyPriority}, nil)

	// Mark h3 as unavailable.
	s.mu.Lock()
	s.probes["h3"].Available = false
	s.mu.Unlock()

	s.maybeSwitch()

	// Priority strategy should skip h3 and pick reality.
	if got := s.ActiveTransport(); got != "reality" {
		t.Fatalf("ActiveTransport = %s, want reality (h3 unavailable)", got)
	}

	// Now also mark reality as unavailable; re-enable h3.
	s.mu.Lock()
	s.probes["h3"].Available = true
	s.probes["reality"].Available = false
	s.mu.Unlock()

	s.maybeSwitch()

	if got := s.ActiveTransport(); got != "h3" {
		t.Fatalf("ActiveTransport = %s, want h3 (reality now unavailable)", got)
	}
}

func TestSelectorMigratePreservesStreams(t *testing.T) {
	h3Conn := &fakeConn{}
	realityConn := &fakeConn{}
	h3 := &fakeTransport{typeName: "h3", conn: h3Conn}
	reality := &fakeTransport{typeName: "reality", conn: realityConn}
	transports := []transport.ClientTransport{h3, reality}

	s := New(transports, &Config{Strategy: StrategyPriority}, nil)

	// Dial to establish initial connection on h3.
	conn, err := s.Dial(context.Background(), "localhost:443")
	if err != nil {
		t.Fatalf("initial Dial failed: %v", err)
	}

	// Open a stream on the h3 connection.
	stream, err := conn.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}

	if s.ActiveTransport() != "h3" {
		t.Fatalf("expected h3 active, got %s", s.ActiveTransport())
	}

	// Migrate to reality via the migrator.
	newConn, err := s.migrator.Migrate(context.Background(), reality, "localhost:443")
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if newConn == nil {
		t.Fatal("Migrate returned nil connection")
	}

	// The old stream should still be usable (Write should succeed on fakeStream).
	data := []byte("hello")
	n, err := stream.Write(data)
	if err != nil {
		t.Fatalf("Write on old stream after migration failed: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Write returned %d, want %d", n, len(data))
	}

	// Old connection should not be closed yet (cleanup keeps latest).
	if h3Conn.closeCalled.Load() {
		t.Fatal("old h3 connection should not be closed before cleanup")
	}

	// Open a stream on the new connection to verify it works.
	newStream, err := newConn.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream on new connection failed: %v", err)
	}
	newStream.Close()

	// Now close the old stream.
	stream.Close()
}
