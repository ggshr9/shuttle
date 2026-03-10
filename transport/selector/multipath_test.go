package selector

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shuttle-proxy/shuttle/transport"
)

// --- fakes for testing ---

type fakeStream struct {
	id     uint64
	closed atomic.Bool
}

func (s *fakeStream) Read(b []byte) (int, error)  { return 0, errors.New("fake") }
func (s *fakeStream) Write(b []byte) (int, error) { return len(b), nil }
func (s *fakeStream) Close() error                { s.closed.Store(true); return nil }
func (s *fakeStream) StreamID() uint64             { return s.id }

type fakeConn struct {
	streamErr  error
	streamID   uint64
	openCount  atomic.Int64
	closeCalled atomic.Bool
}

func (c *fakeConn) OpenStream(ctx context.Context) (transport.Stream, error) {
	if c.streamErr != nil {
		return nil, c.streamErr
	}
	id := c.openCount.Add(1)
	return &fakeStream{id: uint64(id)}, nil
}
func (c *fakeConn) AcceptStream(ctx context.Context) (transport.Stream, error) {
	return nil, errors.New("not implemented")
}
func (c *fakeConn) Close() error       { c.closeCalled.Store(true); return nil }
func (c *fakeConn) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (c *fakeConn) RemoteAddr() net.Addr { return &net.TCPAddr{} }

type fakeTransport struct {
	typeName string
	conn     *fakeConn
	dialErr  error
}

func (t *fakeTransport) Dial(ctx context.Context, addr string) (transport.Connection, error) {
	if t.dialErr != nil {
		return nil, t.dialErr
	}
	return t.conn, nil
}
func (t *fakeTransport) Type() string { return t.typeName }
func (t *fakeTransport) Close() error { return nil }

// --- tests ---

func TestMultipathPoolVirtualConnOpenStream(t *testing.T) {
	conn := &fakeConn{}
	ft := &fakeTransport{typeName: "test", conn: conn}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewMultipathPool(ctx, []transport.ClientTransport{ft}, "localhost:443", &MinLatencyScheduler{}, nil)
	defer pool.Close()

	vc := pool.VirtualConn()
	stream, err := vc.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}
	defer stream.Close()

	// Verify tracking
	infos := pool.PathInfos()
	if len(infos) != 1 {
		t.Fatalf("expected 1 path, got %d", len(infos))
	}
	if infos[0].ActiveStreams != 1 {
		t.Fatalf("expected 1 active stream, got %d", infos[0].ActiveStreams)
	}
	if infos[0].TotalStreams != 1 {
		t.Fatalf("expected 1 total stream, got %d", infos[0].TotalStreams)
	}
}

func TestMultipathPoolStreamCloseDecrementsActive(t *testing.T) {
	conn := &fakeConn{}
	ft := &fakeTransport{typeName: "test", conn: conn}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewMultipathPool(ctx, []transport.ClientTransport{ft}, "localhost:443", &MinLatencyScheduler{}, nil)
	defer pool.Close()

	vc := pool.VirtualConn()
	stream, err := vc.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}

	stream.Close()

	infos := pool.PathInfos()
	if infos[0].ActiveStreams != 0 {
		t.Fatalf("expected 0 active streams after close, got %d", infos[0].ActiveStreams)
	}
	if infos[0].TotalStreams != 1 {
		t.Fatalf("expected 1 total stream (cumulative), got %d", infos[0].TotalStreams)
	}
}

func TestMultipathPoolFallbackOnStreamError(t *testing.T) {
	failConn := &fakeConn{streamErr: errors.New("broken")}
	goodConn := &fakeConn{}

	ft1 := &fakeTransport{typeName: "broken", conn: failConn}
	ft2 := &fakeTransport{typeName: "good", conn: goodConn}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use MinLatency scheduler; set latencies so broken is preferred first
	pool := NewMultipathPool(ctx, []transport.ClientTransport{ft1, ft2}, "localhost:443", &MinLatencyScheduler{}, nil)
	defer pool.Close()

	pool.mu.RLock()
	pool.paths[0].Latency = 1 * time.Millisecond  // broken preferred
	pool.paths[1].Latency = 10 * time.Millisecond // fallback
	pool.mu.RUnlock()

	vc := pool.VirtualConn()
	stream, err := vc.OpenStream(ctx)
	if err != nil {
		t.Fatalf("expected fallback to work, got: %v", err)
	}
	stream.Close()

	// Verify broken path got a failure count
	infos := pool.PathInfos()
	if infos[0].Failures == 0 {
		t.Fatal("expected failures > 0 on broken path")
	}
}

func TestMultipathPoolAllPathsFail(t *testing.T) {
	failConn := &fakeConn{streamErr: errors.New("broken")}
	ft := &fakeTransport{typeName: "broken", conn: failConn}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewMultipathPool(ctx, []transport.ClientTransport{ft}, "localhost:443", &MinLatencyScheduler{}, nil)
	defer pool.Close()

	vc := pool.VirtualConn()
	_, err := vc.OpenStream(ctx)
	if err == nil {
		t.Fatal("expected error when all paths fail")
	}
}

func TestMultipathPoolNoAvailablePaths(t *testing.T) {
	ft := &fakeTransport{typeName: "bad", dialErr: errors.New("dial failed")}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewMultipathPool(ctx, []transport.ClientTransport{ft}, "localhost:443", &MinLatencyScheduler{}, nil)
	defer pool.Close()

	vc := pool.VirtualConn()
	_, err := vc.OpenStream(ctx)
	if err == nil {
		t.Fatal("expected error when no paths available")
	}
}

func TestMultipathPoolClose(t *testing.T) {
	conn := &fakeConn{}
	ft := &fakeTransport{typeName: "test", conn: conn}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewMultipathPool(ctx, []transport.ClientTransport{ft}, "localhost:443", &MinLatencyScheduler{}, nil)
	pool.Close()

	if !conn.closeCalled.Load() {
		t.Fatal("expected connection to be closed")
	}

	infos := pool.PathInfos()
	for _, info := range infos {
		if info.Available {
			t.Fatal("expected all paths unavailable after close")
		}
	}
}

func TestMultipathPoolUpdateMetrics(t *testing.T) {
	conn := &fakeConn{}
	ft := &fakeTransport{typeName: "h3", conn: conn}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewMultipathPool(ctx, []transport.ClientTransport{ft}, "localhost:443", &MinLatencyScheduler{}, nil)
	defer pool.Close()

	pool.UpdateMetrics(map[string]*ProbeResult{
		"h3": {Available: true, Latency: 42 * time.Millisecond},
	})

	infos := pool.PathInfos()
	if infos[0].Latency != 42 {
		t.Fatalf("expected latency 42ms after UpdateMetrics, got %d", infos[0].Latency)
	}
}

func TestMultipathPoolMultipleStreams(t *testing.T) {
	conn := &fakeConn{}
	ft := &fakeTransport{typeName: "test", conn: conn}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewMultipathPool(ctx, []transport.ClientTransport{ft}, "localhost:443", &MinLatencyScheduler{}, nil)
	defer pool.Close()

	vc := pool.VirtualConn()

	// Open 10 streams
	var streams []transport.Stream
	for i := 0; i < 10; i++ {
		s, err := vc.OpenStream(ctx)
		if err != nil {
			t.Fatalf("OpenStream %d failed: %v", i, err)
		}
		streams = append(streams, s)
	}

	infos := pool.PathInfos()
	if infos[0].ActiveStreams != 10 {
		t.Fatalf("expected 10 active streams, got %d", infos[0].ActiveStreams)
	}

	// Close half
	for i := 0; i < 5; i++ {
		streams[i].Close()
	}

	infos = pool.PathInfos()
	if infos[0].ActiveStreams != 5 {
		t.Fatalf("expected 5 active streams after closing half, got %d", infos[0].ActiveStreams)
	}
	if infos[0].TotalStreams != 10 {
		t.Fatalf("expected 10 total streams, got %d", infos[0].TotalStreams)
	}

	// Close remaining
	for i := 5; i < 10; i++ {
		streams[i].Close()
	}

	infos = pool.PathInfos()
	if infos[0].ActiveStreams != 0 {
		t.Fatalf("expected 0 active streams, got %d", infos[0].ActiveStreams)
	}
}

func TestTrackedStreamDoubleClose(t *testing.T) {
	conn := &fakeConn{}
	ft := &fakeTransport{typeName: "test", conn: conn}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewMultipathPool(ctx, []transport.ClientTransport{ft}, "localhost:443", &MinLatencyScheduler{}, nil)
	defer pool.Close()

	vc := pool.VirtualConn()
	stream, err := vc.OpenStream(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Double close should not decrement below 0
	stream.Close()
	stream.Close()

	infos := pool.PathInfos()
	if infos[0].ActiveStreams != 0 {
		t.Fatalf("expected 0 after double close, got %d", infos[0].ActiveStreams)
	}
}
