package h3

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/transport"
)

// mockAddr implements net.Addr for testing.
type mockAddr struct {
	network string
	addr    string
}

func (a *mockAddr) Network() string { return a.network }
func (a *mockAddr) String() string  { return a.addr }

// mockStream implements transport.Stream for testing.
type mockStream struct {
	id     uint64
	closed atomic.Bool
}

func (s *mockStream) Read(p []byte) (int, error) {
	if s.closed.Load() {
		return 0, io.EOF
	}
	return 0, io.EOF
}

func (s *mockStream) Write(p []byte) (int, error) {
	if s.closed.Load() {
		return 0, fmt.Errorf("stream closed")
	}
	return len(p), nil
}

func (s *mockStream) Close() error {
	s.closed.Store(true)
	return nil
}

func (s *mockStream) StreamID() uint64 { return s.id }

// mockConnection implements transport.Connection for testing.
type mockConnection struct {
	localAddr  net.Addr
	remoteAddr net.Addr
	closed     atomic.Bool
	failOpen   atomic.Bool
	streamID   atomic.Uint64
}

func newMockConnection(local, remote string) *mockConnection {
	return &mockConnection{
		localAddr:  &mockAddr{network: "udp", addr: local},
		remoteAddr: &mockAddr{network: "udp", addr: remote},
	}
}

func (c *mockConnection) OpenStream(ctx context.Context) (transport.Stream, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("connection closed")
	}
	if c.failOpen.Load() {
		return nil, fmt.Errorf("open stream failed")
	}
	id := c.streamID.Add(1)
	return &mockStream{id: id}, nil
}

func (c *mockConnection) AcceptStream(ctx context.Context) (transport.Stream, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *mockConnection) Close() error {
	c.closed.Store(true)
	return nil
}

func (c *mockConnection) LocalAddr() net.Addr  { return c.localAddr }
func (c *mockConnection) RemoteAddr() net.Addr { return c.remoteAddr }

func TestMultipathManagerDiscoverPaths(t *testing.T) {
	cfg := &MultipathConfig{
		Enabled: true,
		Mode:    "failover",
	}
	mgr := NewMultipathManager(cfg, slog.Default())
	defer mgr.Close()

	// DiscoverPaths uses net.Interfaces() which should work on any host.
	paths, err := mgr.DiscoverPaths()
	if err != nil {
		t.Skipf("no usable network interfaces on this host: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("expected at least one interface")
	}

	// All returned interfaces should be non-empty strings.
	for _, p := range paths {
		if p == "" {
			t.Error("discovered empty interface name")
		}
	}
}

func TestMultipathManagerSelectPath_Failover(t *testing.T) {
	cfg := &MultipathConfig{Enabled: true, Mode: "failover"}
	mgr := NewMultipathManager(cfg, slog.Default())
	defer mgr.Close()

	conn1 := newMockConnection("10.0.0.1:1234", "1.2.3.4:443")
	conn2 := newMockConnection("10.0.1.1:1234", "1.2.3.4:443")

	mgr.AddPath("eth0", conn1)
	mgr.AddPath("wlan0", conn2)

	// Primary (index 0) should be selected when available.
	selected := mgr.SelectPath()
	if selected == nil {
		t.Fatal("expected a path to be selected")
		return
	}
	if selected.iface != "eth0" {
		t.Errorf("expected eth0, got %s", selected.iface)
	}

	// Mark primary unavailable — should switch to secondary.
	mgr.paths[0].available.Store(false)
	selected = mgr.SelectPath()
	if selected == nil {
		t.Fatal("expected a path after failover")
		return
	}
	if selected.iface != "wlan0" {
		t.Errorf("expected wlan0 after failover, got %s", selected.iface)
	}

	// Mark all unavailable — should return nil.
	mgr.paths[1].available.Store(false)
	selected = mgr.SelectPath()
	if selected != nil {
		t.Error("expected nil when all paths unavailable")
	}
}

func TestMultipathManagerSelectPath_Aggregate(t *testing.T) {
	cfg := &MultipathConfig{Enabled: true, Mode: "aggregate"}
	mgr := NewMultipathManager(cfg, slog.Default())
	defer mgr.Close()

	conn1 := newMockConnection("10.0.0.1:1234", "1.2.3.4:443")
	conn2 := newMockConnection("10.0.1.1:1234", "1.2.3.4:443")
	conn3 := newMockConnection("10.0.2.1:1234", "1.2.3.4:443")

	mgr.AddPath("eth0", conn1)
	mgr.AddPath("wlan0", conn2)
	mgr.AddPath("eth1", conn3)

	// Round-robin should distribute across all paths.
	seen := make(map[string]int)
	for i := 0; i < 9; i++ {
		p := mgr.SelectPath()
		if p == nil {
			t.Fatal("expected a path")
			return
		}
		seen[p.iface]++
	}

	// Each interface should be selected 3 times (9 / 3 = 3).
	for _, iface := range []string{"eth0", "wlan0", "eth1"} {
		if seen[iface] != 3 {
			t.Errorf("expected %s selected 3 times, got %d", iface, seen[iface])
		}
	}
}

func TestMultipathManagerSelectPath_Redundant(t *testing.T) {
	cfg := &MultipathConfig{Enabled: true, Mode: "redundant"}
	mgr := NewMultipathManager(cfg, slog.Default())
	defer mgr.Close()

	conn1 := newMockConnection("10.0.0.1:1234", "1.2.3.4:443")
	conn2 := newMockConnection("10.0.1.1:1234", "1.2.3.4:443")

	mgr.AddPath("eth0", conn1)
	mgr.AddPath("wlan0", conn2)

	// Set RTTs: eth0 = 50ms, wlan0 = 20ms — wlan0 should be preferred.
	mgr.paths[0].rtt.Store(int64(50 * time.Millisecond))
	mgr.paths[1].rtt.Store(int64(20 * time.Millisecond))

	selected := mgr.SelectPath()
	if selected == nil {
		t.Fatal("expected a path")
		return
	}
	if selected.iface != "wlan0" {
		t.Errorf("expected wlan0 (lower RTT), got %s", selected.iface)
	}

	// If wlan0 goes down, eth0 should be selected.
	mgr.paths[1].available.Store(false)
	selected = mgr.SelectPath()
	if selected == nil {
		t.Fatal("expected a path")
		return
	}
	if selected.iface != "eth0" {
		t.Errorf("expected eth0 as fallback, got %s", selected.iface)
	}
}

func TestMultipathManagerStats(t *testing.T) {
	cfg := &MultipathConfig{Enabled: true, Mode: "failover"}
	mgr := NewMultipathManager(cfg, slog.Default())
	defer mgr.Close()

	conn := newMockConnection("10.0.0.1:1234", "1.2.3.4:443")
	mgr.AddPath("eth0", conn)

	// Set some values.
	mgr.paths[0].rtt.Store(int64(30 * time.Millisecond))
	mgr.paths[0].loss.Store(50) // 5% = 50 permille
	mgr.paths[0].bytesSent.Store(1024)
	mgr.paths[0].bytesRecv.Store(2048)

	stats := mgr.Stats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 path stat, got %d", len(stats))
	}

	s := stats[0]
	if s.Interface != "eth0" {
		t.Errorf("expected eth0, got %s", s.Interface)
	}
	if s.RTT != 30 {
		t.Errorf("expected RTT 30ms, got %d", s.RTT)
	}
	if s.LossRate != 0.05 {
		t.Errorf("expected loss rate 0.05, got %f", s.LossRate)
	}
	if s.BytesSent != 1024 {
		t.Errorf("expected 1024 bytes sent, got %d", s.BytesSent)
	}
	if s.BytesRecv != 2048 {
		t.Errorf("expected 2048 bytes recv, got %d", s.BytesRecv)
	}
	if !s.Available {
		t.Error("expected path to be available")
	}
	if s.LocalAddr != "10.0.0.1:1234" {
		t.Errorf("expected 10.0.0.1:1234, got %s", s.LocalAddr)
	}
}

func TestMultipathManagerClose(t *testing.T) {
	cfg := &MultipathConfig{Enabled: true, Mode: "failover"}
	mgr := NewMultipathManager(cfg, slog.Default())

	conn1 := newMockConnection("10.0.0.1:1234", "1.2.3.4:443")
	conn2 := newMockConnection("10.0.1.1:1234", "1.2.3.4:443")

	mgr.AddPath("eth0", conn1)
	mgr.AddPath("wlan0", conn2)

	// Start probes so we can verify cancel works.
	mgr.StartProbes(context.Background())

	mgr.Close()

	// After close, paths should be nil and connections closed.
	if mgr.paths != nil {
		t.Error("expected paths to be nil after close")
	}
	if !conn1.closed.Load() {
		t.Error("expected conn1 to be closed")
	}
	if !conn2.closed.Load() {
		t.Error("expected conn2 to be closed")
	}
}

func TestMultipathConnOpenStream(t *testing.T) {
	cfg := &MultipathConfig{Enabled: true, Mode: "failover"}
	mgr := NewMultipathManager(cfg, slog.Default())

	primary := newMockConnection("10.0.0.1:1234", "1.2.3.4:443")
	secondary := newMockConnection("10.0.1.1:1234", "1.2.3.4:443")

	mgr.AddPath("eth0", primary)
	mgr.AddPath("wlan0", secondary)

	mc := &multipathConn{manager: mgr, primary: primary}

	// Should use primary path (failover mode, primary available).
	stream, err := mc.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}
	stream.Close()

	// Mark primary path as unavailable — should failover to secondary.
	mgr.paths[0].available.Store(false)
	stream, err = mc.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream failed after failover: %v", err)
	}
	stream.Close()

	// Verify byte tracking works via multipathStream.
	// After failover, activePath is now 1 (wlan0). Re-mark path 0 as available
	// but selectFailover still prefers the current activePath (1) since it's available.
	mgr.paths[0].available.Store(true)
	stream, err = mc.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}

	data := []byte("hello multipath")
	n, err := stream.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected %d bytes written, got %d", len(data), n)
	}
	// activePath is 1 (wlan0) after failover, so bytes are tracked on path[1].
	if mgr.paths[1].bytesSent.Load() != int64(len(data)) {
		t.Errorf("expected %d bytes tracked on path[1], got %d", len(data), mgr.paths[1].bytesSent.Load())
	}

	stream.Close()
	mc.Close()
}

func TestMultipathConnOpenStream_FallbackToPrimary(t *testing.T) {
	cfg := &MultipathConfig{Enabled: true, Mode: "failover"}
	mgr := NewMultipathManager(cfg, slog.Default())

	primary := newMockConnection("10.0.0.1:1234", "1.2.3.4:443")
	secondary := newMockConnection("10.0.1.1:1234", "1.2.3.4:443")

	mgr.AddPath("eth0", primary)
	mgr.AddPath("wlan0", secondary)

	// Make the selected path (eth0) fail to open streams.
	primary.failOpen.Store(true)

	mc := &multipathConn{manager: mgr, primary: primary}

	// SelectPath returns eth0 (it's "available" by flag), but OpenStream fails.
	// multipathConn should then fall back to primary (which is also eth0 here, so it will fail).
	_, err := mc.OpenStream(context.Background())
	if err == nil {
		t.Fatal("expected error when primary conn fails to open stream")
	}

	mc.Close()
}

func TestNewMultipathManager_DefaultMode(t *testing.T) {
	cfg := &MultipathConfig{Enabled: true}
	mgr := NewMultipathManager(cfg, nil)
	defer mgr.Close()

	if mgr.mode != "failover" {
		t.Errorf("expected default mode 'failover', got %q", mgr.mode)
	}
}

func TestMultipathManagerSelectPath_Empty(t *testing.T) {
	cfg := &MultipathConfig{Enabled: true, Mode: "failover"}
	mgr := NewMultipathManager(cfg, slog.Default())
	defer mgr.Close()

	p := mgr.SelectPath()
	if p != nil {
		t.Error("expected nil when no paths added")
	}
}
