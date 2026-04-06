package selector

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/transport"
)

// --- mocks for migrate tests ---

type migMockStream struct {
	id     uint64
	closed atomic.Bool
}

func (s *migMockStream) Read(b []byte) (int, error)  { return 0, errors.New("mock") }
func (s *migMockStream) Write(b []byte) (int, error) { return len(b), nil }
func (s *migMockStream) Close() error                { s.closed.Store(true); return nil }
func (s *migMockStream) StreamID() uint64             { return s.id }

type migMockConn struct {
	streamCounter atomic.Uint64
	closeCalled   atomic.Bool
}

func (c *migMockConn) OpenStream(_ context.Context) (transport.Stream, error) {
	id := c.streamCounter.Add(1)
	return &migMockStream{id: id}, nil
}
func (c *migMockConn) AcceptStream(_ context.Context) (transport.Stream, error) {
	return nil, errors.New("not implemented")
}
func (c *migMockConn) Close() error        { c.closeCalled.Store(true); return nil }
func (c *migMockConn) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (c *migMockConn) RemoteAddr() net.Addr { return &net.TCPAddr{} }

// --- tests ---

func TestMigratorTrackAndWrap(t *testing.T) {
	m := NewMigrator(nil)
	defer m.Close()

	conn := &migMockConn{}
	tc := m.Track(conn, "h3")

	if tc.transportName != "h3" {
		t.Fatalf("transportName = %s, want h3", tc.transportName)
	}

	// Open 3 streams via WrapStream
	for i := 0; i < 3; i++ {
		s, err := conn.OpenStream(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		_, err = m.WrapStream(tc, s)
		if err != nil {
			t.Fatal(err)
		}
	}

	if got := tc.activeStreams.Load(); got != 3 {
		t.Fatalf("activeStreams = %d, want 3", got)
	}
}

func TestMigratorStreamClose(t *testing.T) {
	m := NewMigrator(nil)
	defer m.Close()

	conn := &migMockConn{}
	tc := m.Track(conn, "reality")

	s, _ := conn.OpenStream(context.Background())
	ts, _ := m.WrapStream(tc, s)

	if tc.activeStreams.Load() != 1 {
		t.Fatal("expected 1 active stream")
	}

	ts.Close()

	if tc.activeStreams.Load() != 0 {
		t.Fatalf("activeStreams = %d after close, want 0", tc.activeStreams.Load())
	}
}

func TestMigratorStreamDoubleClose(t *testing.T) {
	m := NewMigrator(nil)
	defer m.Close()

	conn := &migMockConn{}
	tc := m.Track(conn, "h3")

	s, _ := conn.OpenStream(context.Background())
	ts, _ := m.WrapStream(tc, s)

	ts.Close()
	ts.Close() // second close should be a no-op for the counter

	if got := tc.activeStreams.Load(); got != 0 {
		t.Fatalf("activeStreams = %d after double close, want 0", got)
	}
}

func TestMigratorMigrate(t *testing.T) {
	m := NewMigrator(nil)
	defer m.Close()

	conn := &migMockConn{}
	tc := m.Track(conn, "h3")

	// Before Migrate, WrapStream should work.
	s1, _ := conn.OpenStream(context.Background())
	_, err := m.WrapStream(tc, s1)
	if err != nil {
		t.Fatalf("WrapStream before Migrate: %v", err)
	}

	m.Migrate()

	// After Migrate, WrapStream on the old conn should fail.
	s2, _ := conn.OpenStream(context.Background())
	_, err = m.WrapStream(tc, s2)
	if !errors.Is(err, ErrConnectionDraining) {
		t.Fatalf("expected ErrConnectionDraining, got %v", err)
	}
}

func TestMigratorDrainLoop(t *testing.T) {
	m := NewMigrator(nil)

	conn := &migMockConn{}
	tc := m.Track(conn, "cdn")

	// Mark draining with 0 active streams.
	tc.draining.Store(true)

	// Manually trigger drainIdle (instead of waiting for ticker).
	m.drainIdle()

	if !conn.closeCalled.Load() {
		t.Fatal("expected draining conn with 0 streams to be closed")
	}

	// Verify it was removed from tracked list.
	stats := m.Stats()
	if len(stats) != 0 {
		t.Fatalf("expected 0 tracked connections after drain, got %d", len(stats))
	}

	m.Close()
}

func TestMigratorDrainWithActiveStreams(t *testing.T) {
	m := NewMigrator(nil)

	conn := &migMockConn{}
	tc := m.Track(conn, "h3")

	// Open a stream, then mark draining.
	s, _ := conn.OpenStream(context.Background())
	ts, _ := m.WrapStream(tc, s)

	tc.draining.Store(true)

	// drainIdle should NOT close it because there's 1 active stream.
	m.drainIdle()

	if conn.closeCalled.Load() {
		t.Fatal("should not close conn with active streams")
	}

	stats := m.Stats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 tracked connection, got %d", len(stats))
	}

	// Close the stream, then drain again.
	ts.Close()
	m.drainIdle()

	if !conn.closeCalled.Load() {
		t.Fatal("expected conn to be closed after all streams closed")
	}

	stats = m.Stats()
	if len(stats) != 0 {
		t.Fatalf("expected 0 tracked connections, got %d", len(stats))
	}

	m.Close()
}

func TestMigratorClose(t *testing.T) {
	m := NewMigrator(nil)

	conn1 := &migMockConn{}
	conn2 := &migMockConn{}
	m.Track(conn1, "h3")
	m.Track(conn2, "reality")

	m.Close()

	if !conn1.closeCalled.Load() {
		t.Fatal("conn1 not closed")
	}
	if !conn2.closeCalled.Load() {
		t.Fatal("conn2 not closed")
	}

	stats := m.Stats()
	if len(stats) != 0 {
		t.Fatalf("expected 0 connections after Close, got %d", len(stats))
	}
}

func TestMigrator_DrainTimeout(t *testing.T) {
	// Use a very short timeout and interval so the test runs fast.
	m := NewMigrator(nil, MigratorConfig{
		DrainInterval: 10 * time.Millisecond,
		DrainTimeout:  50 * time.Millisecond,
	})
	defer m.Close()

	conn := &migMockConn{}
	tc := m.Track(conn, "h3")

	// Open a stream but don't close it — this prevents idle-drain.
	s, _ := conn.OpenStream(context.Background())
	_, err := m.WrapStream(tc, s)
	if err != nil {
		t.Fatalf("WrapStream: %v", err)
	}

	// Migrate marks the connection as draining.
	m.Migrate()

	if !tc.draining.Load() {
		t.Fatal("expected connection to be draining after Migrate")
	}

	// drainIdle should NOT close it immediately — stream is still open.
	m.drainIdle()
	if conn.closeCalled.Load() {
		t.Fatal("connection should not be closed before drain timeout")
	}

	// Wait for the drain timeout to expire, then trigger drainIdle manually.
	time.Sleep(60 * time.Millisecond)
	m.drainIdle()

	if !conn.closeCalled.Load() {
		t.Fatal("expected connection to be force-closed after drain timeout")
	}

	// Verify it was removed from tracked list.
	stats := m.Stats()
	if len(stats) != 0 {
		t.Fatalf("expected 0 tracked connections after force-close, got %d", len(stats))
	}
}

// countingTransport tracks how many times Dial is called, returning a fresh
// fakeConn each time. Used to detect connection leaks in concurrent tests.
type countingTransport struct {
	typeName  string
	dialCount atomic.Int64
}

func (t *countingTransport) Dial(_ context.Context, _ string) (transport.Connection, error) {
	t.dialCount.Add(1)
	return &migMockConn{}, nil
}
func (t *countingTransport) Type() string { return t.typeName }
func (t *countingTransport) Close() error { return nil }

// TestMigratedConnConcurrentDialNewAndOpen verifies that when multiple
// goroutines call dialNewAndOpen concurrently on a draining connection, only
// one new connection is dialed (no connection leak).
func TestMigratedConnConcurrentDialNewAndOpen(t *testing.T) {
	const goroutines = 20

	ct := &countingTransport{typeName: "h3"}

	// Build a minimal Selector with ct as the active transport.
	sel := &Selector{
		active:  ct,
		logger:  slog.Default(),
		migrator: NewMigrator(nil),
	}

	// Create an initial draining connection.
	initialConn := &migMockConn{}
	tc := sel.migrator.Track(initialConn, "h3")
	tc.draining.Store(true) // simulate draining state

	mc := &migratedConn{
		sel:      sel,
		migrator: sel.migrator,
		tc:       tc,
		addr:     "127.0.0.1:443",
	}

	// Use a WaitGroup barrier so all goroutines start at the same time,
	// maximising the chance of exposing a concurrent-dial race.
	var ready sync.WaitGroup
	ready.Add(goroutines)
	start := make(chan struct{})

	// Fire goroutines that all call dialNewAndOpen concurrently.
	type result struct {
		stream transport.Stream
		err    error
	}
	results := make(chan result, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			ready.Done()
			<-start
			s, err := mc.dialNewAndOpen(context.Background())
			results <- result{s, err}
		}()
	}
	ready.Wait()
	close(start) // release all goroutines simultaneously

	// Collect results.
	var successes int
	for i := 0; i < goroutines; i++ {
		r := <-results
		if r.err != nil {
			t.Errorf("dialNewAndOpen error: %v", r.err)
			continue
		}
		successes++
		r.stream.Close()
	}

	if successes != goroutines {
		t.Fatalf("expected %d successful streams, got %d", goroutines, successes)
	}

	// The fix: only ONE new connection should have been dialed regardless of
	// how many goroutines raced. Without the mutex, every goroutine would dial
	// its own connection, causing a leak.
	if got := ct.dialCount.Load(); got != 1 {
		t.Fatalf("Dial called %d times, want 1 (connection leak detected)", got)
	}
}

func TestMigratorStats(t *testing.T) {
	m := NewMigrator(nil)
	defer m.Close()

	conn1 := &migMockConn{}
	conn2 := &migMockConn{}
	tc1 := m.Track(conn1, "h3")
	tc2 := m.Track(conn2, "reality")

	// Open 2 streams on tc1
	for i := 0; i < 2; i++ {
		s, _ := conn1.OpenStream(context.Background())
		m.WrapStream(tc1, s)
	}

	// Mark tc2 as draining
	tc2.draining.Store(true)

	stats := m.Stats()
	if len(stats) != 2 {
		t.Fatalf("expected 2 stats entries, got %d", len(stats))
	}

	// Find h3 stats
	var h3Stats, realityStats ConnMigrationStats
	for _, s := range stats {
		switch s.Transport {
		case "h3":
			h3Stats = s
		case "reality":
			realityStats = s
		}
	}

	if h3Stats.ActiveStreams != 2 {
		t.Fatalf("h3 active streams = %d, want 2", h3Stats.ActiveStreams)
	}
	if h3Stats.Draining {
		t.Fatal("h3 should not be draining")
	}
	if h3Stats.Created.IsZero() {
		t.Fatal("h3 created time should not be zero")
	}

	if !realityStats.Draining {
		t.Fatal("reality should be draining")
	}
	if realityStats.ActiveStreams != 0 {
		t.Fatalf("reality active streams = %d, want 0", realityStats.ActiveStreams)
	}

	// Verify Created is recent (within last second).
	if time.Since(realityStats.Created) > time.Second {
		t.Fatal("reality created time too old")
	}
}
