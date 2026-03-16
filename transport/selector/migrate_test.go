package selector

import (
	"context"
	"errors"
	"net"
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
