package selector

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/transport"
)

// --- mock transport & connection ---

type mockConn struct {
	id     uint64
	closed atomic.Bool
}

func (c *mockConn) OpenStream(ctx context.Context) (transport.Stream, error) { return nil, nil }
func (c *mockConn) AcceptStream(ctx context.Context) (transport.Stream, error) {
	return nil, nil
}
func (c *mockConn) Close() error         { c.closed.Store(true); return nil }
func (c *mockConn) LocalAddr() net.Addr   { return &net.TCPAddr{} }
func (c *mockConn) RemoteAddr() net.Addr  { return &net.TCPAddr{} }
func (c *mockConn) IsClosed() bool        { return c.closed.Load() }

type mockTransport struct {
	dialCount atomic.Int64
	mu        sync.Mutex
	conns     []*mockConn
}

func (m *mockTransport) Dial(ctx context.Context, addr string) (transport.Connection, error) {
	m.dialCount.Add(1)
	c := &mockConn{id: uint64(m.dialCount.Load())}
	m.mu.Lock()
	m.conns = append(m.conns, c)
	m.mu.Unlock()
	return c, nil
}

func (m *mockTransport) Type() string  { return "mock" }
func (m *mockTransport) Close() error  { return nil }

// --- tests ---

func TestConnPoolGetPut(t *testing.T) {
	mt := &mockTransport{}
	pool := NewConnPool(mt, "127.0.0.1:443", 4, 0, nil)
	defer pool.Close()

	ctx := context.Background()

	// Get from empty pool should dial a new connection.
	conn1, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if mt.dialCount.Load() != 1 {
		t.Fatalf("expected 1 dial, got %d", mt.dialCount.Load())
	}

	// Put it back.
	pool.Put(conn1)

	// Get again should return the same connection (no new dial).
	conn2, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if mt.dialCount.Load() != 1 {
		t.Fatalf("expected still 1 dial, got %d", mt.dialCount.Load())
	}
	if conn2 != conn1 {
		t.Fatal("expected to get back the same connection from pool")
	}
}

func TestConnPoolWarmUp(t *testing.T) {
	mt := &mockTransport{}
	pool := NewConnPool(mt, "127.0.0.1:443", 8, 0, nil)
	defer pool.Close()

	ctx := context.Background()
	pool.WarmUp(ctx, 4)

	// Wait for goroutines to finish.
	time.Sleep(100 * time.Millisecond)

	if mt.dialCount.Load() != 4 {
		t.Fatalf("expected 4 dials from warm-up, got %d", mt.dialCount.Load())
	}

	// All 4 should be retrievable without new dials.
	for i := 0; i < 4; i++ {
		_, err := pool.Get(ctx)
		if err != nil {
			t.Fatalf("Get[%d]: %v", i, err)
		}
	}
	if mt.dialCount.Load() != 4 {
		t.Fatalf("expected no additional dials, got %d total", mt.dialCount.Load())
	}
}

func TestConnPoolEviction(t *testing.T) {
	mt := &mockTransport{}
	pool := NewConnPool(mt, "127.0.0.1:443", 4, 0, nil)
	pool.idleTTL = 50 * time.Millisecond // short TTL for testing
	defer pool.Close()

	ctx := context.Background()

	conn, err := pool.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pool.Put(conn)

	// Wait for TTL to expire.
	time.Sleep(100 * time.Millisecond)

	// evictExpired should remove it.
	pool.evictExpired()

	pool.mu.Lock()
	count := len(pool.idle)
	pool.mu.Unlock()

	if count != 0 {
		t.Fatalf("expected 0 idle conns after eviction, got %d", count)
	}

	// The evicted connection should have been closed.
	mc := conn.(*mockConn)
	if !mc.IsClosed() {
		t.Fatal("evicted connection was not closed")
	}
}

func TestConnPoolMaxIdle(t *testing.T) {
	mt := &mockTransport{}
	pool := NewConnPool(mt, "127.0.0.1:443", 2, 0, nil)
	defer pool.Close()

	ctx := context.Background()

	// Dial 3 connections.
	var conns []transport.Connection
	for i := 0; i < 3; i++ {
		c, err := pool.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}

	// Put all 3 back — only 2 should be kept, 3rd should be closed.
	for _, c := range conns {
		pool.Put(c)
	}

	pool.mu.Lock()
	idleCount := len(pool.idle)
	pool.mu.Unlock()

	if idleCount != 2 {
		t.Fatalf("expected 2 idle (maxIdle), got %d", idleCount)
	}

	// The third connection (first put, since it fills up at 2) should have been closed.
	// conns[2] is put last when pool is already full.
	mc := conns[2].(*mockConn)
	if !mc.IsClosed() {
		t.Fatal("excess connection should have been closed")
	}
}

func TestConnPoolClosed(t *testing.T) {
	mt := &mockTransport{}
	pool := NewConnPool(mt, "127.0.0.1:443", 4, 0, nil)

	ctx := context.Background()

	// Get a connection, put it back, then close the pool.
	conn, err := pool.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pool.Put(conn)
	pool.Close()

	// The idle connection should have been closed.
	mc := conn.(*mockConn)
	if !mc.IsClosed() {
		t.Fatal("idle connection should be closed after pool.Close()")
	}

	// Get after Close should return error.
	_, err = pool.Get(ctx)
	if err != errPoolClosed {
		t.Fatalf("expected errPoolClosed, got %v", err)
	}

	// Put after Close should close the connection.
	conn2, _ := mt.Dial(ctx, "x")
	pool.Put(conn2)
	mc2 := conn2.(*mockConn)
	if !mc2.IsClosed() {
		t.Fatal("put after close should close the conn")
	}
}
