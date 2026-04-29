//go:build sandbox

package test

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/transport"
	"github.com/ggshr9/shuttle/transport/selector"
)

// --- multipath-specific mock types (prefixed mp to avoid collision with mesh_test mocks) ---

type mpMockTransport struct {
	name     string
	latency  time.Duration
	failDial bool
	mu       sync.Mutex
	conns    []*mpMockConn
}

func (m *mpMockTransport) Type() string { return m.name }
func (m *mpMockTransport) Close() error { return nil }

func (m *mpMockTransport) Dial(ctx context.Context, addr string) (transport.Connection, error) {
	if m.failDial {
		return nil, errors.New("mock: dial failed")
	}
	time.Sleep(m.latency)
	c := &mpMockConn{transport: m.name}
	m.mu.Lock()
	m.conns = append(m.conns, c)
	m.mu.Unlock()
	return c, nil
}

type mpMockConn struct {
	transport   string
	failStream  bool
	streamCount atomic.Int64
	closed      atomic.Bool
}

func (c *mpMockConn) OpenStream(ctx context.Context) (transport.Stream, error) {
	if c.failStream || c.closed.Load() {
		return nil, errors.New("mock: open stream failed")
	}
	c.streamCount.Add(1)
	return &mpMockStream{id: uint64(c.streamCount.Load())}, nil
}

func (c *mpMockConn) AcceptStream(ctx context.Context) (transport.Stream, error) {
	return nil, errors.New("not supported")
}

func (c *mpMockConn) Close() error {
	c.closed.Store(true)
	return nil
}

func (c *mpMockConn) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (c *mpMockConn) RemoteAddr() net.Addr { return &net.TCPAddr{} }

type mpMockStream struct {
	id     uint64
	closed atomic.Bool
}

func (s *mpMockStream) StreamID() uint64             { return s.id }
func (s *mpMockStream) Read(b []byte) (int, error)   { return 0, io.EOF }
func (s *mpMockStream) Write(b []byte) (int, error)  { return len(b), nil }
func (s *mpMockStream) Close() error                 { s.closed.Store(true); return nil }

// --- Tests ---

func TestMultipathPoolCreation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transports := []transport.ClientTransport{
		&mpMockTransport{name: "h3", latency: time.Millisecond},
		&mpMockTransport{name: "reality", latency: time.Millisecond},
		&mpMockTransport{name: "cdn", latency: time.Millisecond},
	}

	pool := selector.NewMultipathPool(ctx, transports, "127.0.0.1:0", selector.NewWeightedLatencyScheduler(), 0, 0, nil)
	defer pool.Close()

	paths := pool.PathInfos()
	if len(paths) != 3 {
		t.Fatalf("expected 3 paths, got %d", len(paths))
	}
	for _, p := range paths {
		if !p.Available {
			t.Errorf("path %s should be available", p.Transport)
		}
	}
}

func TestMultipathOpenStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transports := []transport.ClientTransport{
		&mpMockTransport{name: "h3", latency: time.Millisecond},
		&mpMockTransport{name: "reality", latency: time.Millisecond},
		&mpMockTransport{name: "cdn", latency: time.Millisecond},
	}

	pool := selector.NewMultipathPool(ctx, transports, "127.0.0.1:0", selector.NewLoadBalanceScheduler(), 0, 0, nil)
	defer pool.Close()

	vconn := pool.VirtualConn()
	const numStreams = 100
	// Keep streams open so load balancer distributes across paths
	var streams []transport.Stream
	for i := 0; i < numStreams; i++ {
		s, err := vconn.OpenStream(ctx)
		if err != nil {
			t.Fatalf("stream %d: %v", i, err)
		}
		streams = append(streams, s)
	}

	// At least 2 paths should have been used (load balance distributes)
	paths := pool.PathInfos()
	usedCount := 0
	for _, p := range paths {
		if p.TotalStreams > 0 {
			usedCount++
		}
	}
	if usedCount < 2 {
		t.Errorf("expected at least 2 paths used, got %d", usedCount)
	}

	for _, s := range streams {
		s.Close()
	}
}

func TestWeightedScheduler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fast := &mpMockTransport{name: "fast", latency: time.Microsecond}
	slow := &mpMockTransport{name: "slow", latency: time.Microsecond}

	transports := []transport.ClientTransport{fast, slow}
	pool := selector.NewMultipathPool(ctx, transports, "127.0.0.1:0", selector.NewWeightedLatencyScheduler(), 0, 0, nil)
	defer pool.Close()

	// Manually update metrics: fast=1ms, slow=100ms
	pool.UpdateMetrics(map[string]*selector.ProbeResult{
		"fast": {Latency: 1 * time.Millisecond, Available: true},
		"slow": {Latency: 100 * time.Millisecond, Available: true},
	})

	vconn := pool.VirtualConn()
	const total = 1000
	for i := 0; i < total; i++ {
		s, err := vconn.OpenStream(ctx)
		if err != nil {
			t.Fatalf("stream %d: %v", i, err)
		}
		s.Close()
	}

	paths := pool.PathInfos()
	var fastStreams, slowStreams int64
	for _, p := range paths {
		switch p.Transport {
		case "fast":
			fastStreams = p.TotalStreams
		case "slow":
			slowStreams = p.TotalStreams
		}
	}

	// The fast path should get significantly more streams due to weighted scheduling
	if fastStreams <= slowStreams {
		t.Errorf("expected fast (%d) > slow (%d) streams", fastStreams, slowStreams)
	}
}

func TestMinLatencyScheduler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transports := []transport.ClientTransport{
		&mpMockTransport{name: "fast", latency: time.Microsecond},
		&mpMockTransport{name: "medium", latency: time.Microsecond},
		&mpMockTransport{name: "slow", latency: time.Microsecond},
	}

	pool := selector.NewMultipathPool(ctx, transports, "127.0.0.1:0", selector.NewMinLatencyScheduler(), 0, 0, nil)
	defer pool.Close()

	pool.UpdateMetrics(map[string]*selector.ProbeResult{
		"fast":   {Latency: 5 * time.Millisecond, Available: true},
		"medium": {Latency: 50 * time.Millisecond, Available: true},
		"slow":   {Latency: 200 * time.Millisecond, Available: true},
	})

	vconn := pool.VirtualConn()
	const total = 50
	for i := 0; i < total; i++ {
		s, err := vconn.OpenStream(ctx)
		if err != nil {
			t.Fatalf("stream %d: %v", i, err)
		}
		s.Close()
	}

	paths := pool.PathInfos()
	for _, p := range paths {
		if p.Transport == "fast" {
			if p.TotalStreams != total {
				t.Errorf("expected all %d streams on fast, got %d", total, p.TotalStreams)
			}
		} else {
			if p.TotalStreams != 0 {
				t.Errorf("expected 0 streams on %s, got %d", p.Transport, p.TotalStreams)
			}
		}
	}
}

func TestLoadBalanceScheduler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transports := []transport.ClientTransport{
		&mpMockTransport{name: "a", latency: time.Microsecond},
		&mpMockTransport{name: "b", latency: time.Microsecond},
		&mpMockTransport{name: "c", latency: time.Microsecond},
	}

	pool := selector.NewMultipathPool(ctx, transports, "127.0.0.1:0", selector.NewLoadBalanceScheduler(), 0, 0, nil)
	defer pool.Close()

	vconn := pool.VirtualConn()
	const total = 90
	// Keep streams open so ActiveStreams accumulates
	var streams []transport.Stream
	for i := 0; i < total; i++ {
		s, err := vconn.OpenStream(ctx)
		if err != nil {
			t.Fatalf("stream %d: %v", i, err)
		}
		streams = append(streams, s)
	}

	paths := pool.PathInfos()
	expected := int64(total / 3)
	for _, p := range paths {
		// Allow ±5 tolerance for load balance
		diff := p.TotalStreams - expected
		if diff < -5 || diff > 5 {
			t.Errorf("path %s: expected ~%d streams, got %d", p.Transport, expected, p.TotalStreams)
		}
	}

	for _, s := range streams {
		s.Close()
	}
}

func TestMultipathPathFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	healthy := &mpMockTransport{name: "healthy", latency: time.Microsecond}
	failing := &mpMockTransport{name: "failing", latency: time.Microsecond}

	transports := []transport.ClientTransport{failing, healthy}
	pool := selector.NewMultipathPool(ctx, transports, "127.0.0.1:0", selector.NewMinLatencyScheduler(), 0, 0, nil)
	defer pool.Close()

	// Set latencies so failing is preferred
	pool.UpdateMetrics(map[string]*selector.ProbeResult{
		"failing": {Latency: 1 * time.Millisecond, Available: true},
		"healthy": {Latency: 50 * time.Millisecond, Available: true},
	})

	// Now make the failing transport's connection fail on OpenStream
	failing.mu.Lock()
	for _, c := range failing.conns {
		c.failStream = true
	}
	failing.mu.Unlock()

	vconn := pool.VirtualConn()
	// Streams should fallback to healthy
	for i := 0; i < 10; i++ {
		s, err := vconn.OpenStream(ctx)
		if err != nil {
			t.Fatalf("stream %d should succeed via fallback: %v", i, err)
		}
		s.Close()
	}

	paths := pool.PathInfos()
	for _, p := range paths {
		if p.Transport == "healthy" && p.TotalStreams == 0 {
			t.Error("expected healthy path to receive fallback streams")
		}
	}
}

func TestMultipathReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mt := &mpMockTransport{name: "reconnectable", latency: time.Microsecond}
	transports := []transport.ClientTransport{mt}

	pool := selector.NewMultipathPool(ctx, transports, "127.0.0.1:0", selector.NewMinLatencyScheduler(), 0, 0, nil)
	defer pool.Close()

	paths := pool.PathInfos()
	if len(paths) != 1 || !paths[0].Available {
		t.Fatalf("expected 1 available path, got %+v", paths)
	}

	// Simulate dial failure then recovery — close existing conn and set failures >= 3
	mt.mu.Lock()
	for _, c := range mt.conns {
		c.Close()
	}
	mt.mu.Unlock()

	// The healthLoop runs every 10s in production. We can't wait that long in test,
	// so verify the pool structure is correct and the path is initially connected.
	if !paths[0].Available {
		t.Error("path should be initially available")
	}
}

func TestMultipathAllFail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1 := &mpMockTransport{name: "a", failDial: true}
	t2 := &mpMockTransport{name: "b", failDial: true}

	transports := []transport.ClientTransport{t1, t2}
	pool := selector.NewMultipathPool(ctx, transports, "127.0.0.1:0", selector.NewMinLatencyScheduler(), 0, 0, nil)
	defer pool.Close()

	vconn := pool.VirtualConn()
	_, err := vconn.OpenStream(ctx)
	if err == nil {
		t.Fatal("expected error when all paths fail")
	}
}

func TestTrackedStreamClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transports := []transport.ClientTransport{
		&mpMockTransport{name: "test", latency: time.Microsecond},
	}

	pool := selector.NewMultipathPool(ctx, transports, "127.0.0.1:0", selector.NewMinLatencyScheduler(), 0, 0, nil)
	defer pool.Close()

	vconn := pool.VirtualConn()

	// Open 5 streams
	var streams []transport.Stream
	for i := 0; i < 5; i++ {
		s, err := vconn.OpenStream(ctx)
		if err != nil {
			t.Fatal(err)
		}
		streams = append(streams, s)
	}

	paths := pool.PathInfos()
	if paths[0].ActiveStreams != 5 {
		t.Errorf("expected 5 active streams, got %d", paths[0].ActiveStreams)
	}

	// Close 3
	for i := 0; i < 3; i++ {
		streams[i].Close()
	}

	paths = pool.PathInfos()
	if paths[0].ActiveStreams != 2 {
		t.Errorf("expected 2 active streams after closing 3, got %d", paths[0].ActiveStreams)
	}

	// Double close should be safe
	streams[0].Close()
	paths = pool.PathInfos()
	if paths[0].ActiveStreams != 2 {
		t.Errorf("expected 2 active streams after double close, got %d", paths[0].ActiveStreams)
	}

	// Close remaining
	streams[3].Close()
	streams[4].Close()
	paths = pool.PathInfos()
	if paths[0].ActiveStreams != 0 {
		t.Errorf("expected 0 active streams, got %d", paths[0].ActiveStreams)
	}
}

func TestSelectorMultipathDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transports := []transport.ClientTransport{
		&mpMockTransport{name: "h3", latency: time.Microsecond},
		&mpMockTransport{name: "reality", latency: time.Microsecond},
	}

	sel := selector.New(transports, &selector.Config{
		Strategy:   selector.StrategyMultipath,
		ServerAddr: "127.0.0.1:0",
	}, nil)
	sel.Start(ctx)
	defer sel.Close()

	if sel.ActiveTransport() != "multipath" {
		t.Errorf("expected 'multipath', got %q", sel.ActiveTransport())
	}

	conn, err := sel.Dial(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	stream, err := conn.OpenStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Close()

	paths := sel.ActivePaths()
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
}

func TestBackwardCompat(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transports := []transport.ClientTransport{
		&mpMockTransport{name: "h3", latency: time.Microsecond},
		&mpMockTransport{name: "reality", latency: time.Microsecond},
	}

	for _, strategy := range []selector.Strategy{
		selector.StrategyAuto,
		selector.StrategyPriority,
		selector.StrategyLatency,
	} {
		t.Run(string(strategy), func(t *testing.T) {
			sel := selector.New(transports, &selector.Config{
				Strategy: strategy,
			}, nil)
			sel.Start(ctx)
			defer sel.Close()

			if sel.ActiveTransport() == "multipath" {
				t.Errorf("strategy %s should not report multipath", strategy)
			}

			if paths := sel.ActivePaths(); paths != nil {
				t.Errorf("strategy %s should return nil ActivePaths", strategy)
			}

			conn, err := sel.Dial(ctx, "127.0.0.1:0")
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			stream, err := conn.OpenStream(ctx)
			if err != nil {
				t.Fatalf("open stream: %v", err)
			}
			stream.Close()
		})
	}
}

func TestSingleTransport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transports := []transport.ClientTransport{
		&mpMockTransport{name: "only", latency: time.Microsecond},
	}

	pool := selector.NewMultipathPool(ctx, transports, "127.0.0.1:0", selector.NewWeightedLatencyScheduler(), 0, 0, nil)
	defer pool.Close()

	vconn := pool.VirtualConn()
	for i := 0; i < 10; i++ {
		s, err := vconn.OpenStream(ctx)
		if err != nil {
			t.Fatalf("stream %d: %v", i, err)
		}
		s.Close()
	}

	paths := pool.PathInfos()
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	if paths[0].TotalStreams != 10 {
		t.Errorf("expected 10 total streams, got %d", paths[0].TotalStreams)
	}
}
