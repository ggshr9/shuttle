package plugin

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

// mockEmitter records emitted events for testing.
type mockEmitter struct {
	mu     sync.Mutex
	events []mockEvent
}

type mockEvent struct {
	connID, state, target, rule, protocol, processName string
	bytesIn, bytesOut, durationMs                      int64
}

func (m *mockEmitter) EmitConnectionEvent(connID, state, target, rule, protocol, processName string, bytesIn, bytesOut, durationMs int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, mockEvent{connID, state, target, rule, protocol, processName, bytesIn, bytesOut, durationMs})
}

func (m *mockEmitter) getEvents() []mockEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mockEvent{}, m.events...)
}

// mockConn is a minimal net.Conn for testing.
type mockConn struct {
	net.Conn
	remoteAddr net.Addr
	closed     bool
}

func (m *mockConn) RemoteAddr() net.Addr {
	return m.remoteAddr
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	return len(b), nil
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	return len(b), nil
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

type mockAddr struct {
	network, addr string
}

func (m mockAddr) Network() string { return m.network }
func (m mockAddr) String() string  { return m.addr }

func TestConnTrackerName(t *testing.T) {
	ct := NewConnTracker(nil)
	if ct.Name() != "conntrack" {
		t.Errorf("Name() = %q, want %q", ct.Name(), "conntrack")
	}
}

func TestConnTrackerInit(t *testing.T) {
	ct := NewConnTracker(nil)
	if err := ct.Init(context.Background()); err != nil {
		t.Errorf("Init() error = %v", err)
	}
}

func TestConnTrackerOnConnect(t *testing.T) {
	emitter := &mockEmitter{}
	ct := NewConnTracker(emitter)

	conn := &mockConn{
		remoteAddr: mockAddr{network: "tcp", addr: "192.168.1.1:443"},
	}

	wrapped, err := ct.OnConnect(conn, "example.com:443")
	if err != nil {
		t.Fatalf("OnConnect() error = %v", err)
	}
	if wrapped == nil {
		t.Fatal("OnConnect() returned nil")
	}

	// Verify event emitted
	events := emitter.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].state != "opened" {
		t.Errorf("Event state = %q, want %q", events[0].state, "opened")
	}
	if events[0].target != "example.com:443" {
		t.Errorf("Event target = %q, want %q", events[0].target, "example.com:443")
	}
}

func TestConnTrackerByteCounting(t *testing.T) {
	emitter := &mockEmitter{}
	ct := NewConnTracker(emitter)

	conn := &mockConn{
		remoteAddr: mockAddr{network: "tcp", addr: "192.168.1.1:443"},
	}

	wrapped, _ := ct.OnConnect(conn, "example.com:443")

	// Write some data
	buf := make([]byte, 100)
	_, _ = wrapped.Write(buf)
	_, _ = wrapped.Write(buf) // 200 bytes out

	// Read some data
	_, _ = wrapped.Read(buf)
	_, _ = wrapped.Read(buf)
	_, _ = wrapped.Read(buf) // 300 bytes in

	// Close connection
	wrapped.Close()

	// Verify close event has correct byte counts
	events := emitter.getEvents()
	if len(events) != 2 {
		t.Fatalf("Expected 2 events (open + close), got %d", len(events))
	}

	closeEvent := events[1]
	if closeEvent.state != "closed" {
		t.Errorf("Close event state = %q, want %q", closeEvent.state, "closed")
	}
	if closeEvent.bytesOut != 200 {
		t.Errorf("bytesOut = %d, want 200", closeEvent.bytesOut)
	}
	if closeEvent.bytesIn != 300 {
		t.Errorf("bytesIn = %d, want 300", closeEvent.bytesIn)
	}
}

func TestConnTrackerSetRule(t *testing.T) {
	emitter := &mockEmitter{}
	ct := NewConnTracker(emitter)

	conn := &mockConn{
		remoteAddr: mockAddr{network: "tcp", addr: "192.168.1.1:443"},
	}

	wrapped, _ := ct.OnConnect(conn, "example.com:443")
	tc := wrapped.(*trackingConn)

	// Set rule
	ct.SetConnRule(tc.tracked.id, "geosite:google")

	// Close and verify rule in event
	wrapped.Close()

	events := emitter.getEvents()
	closeEvent := events[1]
	if closeEvent.rule != "geosite:google" {
		t.Errorf("rule = %q, want %q", closeEvent.rule, "geosite:google")
	}
}

func TestConnTrackerSetProcess(t *testing.T) {
	emitter := &mockEmitter{}
	ct := NewConnTracker(emitter)

	conn := &mockConn{
		remoteAddr: mockAddr{network: "tcp", addr: "192.168.1.1:443"},
	}

	wrapped, _ := ct.OnConnect(conn, "example.com:443")
	tc := wrapped.(*trackingConn)

	// Set process
	ct.SetConnProcess(tc.tracked.id, "chrome")

	// Close and verify process in event
	wrapped.Close()

	events := emitter.getEvents()
	closeEvent := events[1]
	if closeEvent.processName != "chrome" {
		t.Errorf("processName = %q, want %q", closeEvent.processName, "chrome")
	}
}

func TestConnTrackerDoubleClose(t *testing.T) {
	emitter := &mockEmitter{}
	ct := NewConnTracker(emitter)

	conn := &mockConn{
		remoteAddr: mockAddr{network: "tcp", addr: "192.168.1.1:443"},
	}

	wrapped, _ := ct.OnConnect(conn, "example.com:443")

	// Close twice
	wrapped.Close()
	wrapped.Close()

	// Should only emit one close event
	events := emitter.getEvents()
	closeCount := 0
	for _, e := range events {
		if e.state == "closed" {
			closeCount++
		}
	}
	if closeCount != 1 {
		t.Errorf("Expected 1 close event, got %d", closeCount)
	}
}

func TestDetectProtocol(t *testing.T) {
	tests := []struct {
		network string
		want    string
	}{
		{"tcp", "tcp"},
		{"tcp4", "tcp"},
		{"tcp6", "tcp"},
		{"udp", "udp"},
		{"udp4", "udp"},
		{"udp6", "udp"},
		{"unix", "unix"},  // returns network as-is for unknown
		{"ip", "ip"},      // returns network as-is for unknown
	}

	for _, tt := range tests {
		conn := &mockConn{remoteAddr: mockAddr{network: tt.network}}
		got := detectProtocol(conn)
		if got != tt.want {
			t.Errorf("detectProtocol(%q) = %q, want %q", tt.network, got, tt.want)
		}
	}
}

func TestDetectProtocolNilAddr(t *testing.T) {
	conn := &mockConn{remoteAddr: nil}
	got := detectProtocol(conn)
	if got != "unknown" {
		t.Errorf("detectProtocol(nil) = %q, want %q", got, "unknown")
	}
}

func TestFormatConnID(t *testing.T) {
	tests := []struct {
		id   uint64
		want string
	}{
		{0, "00000000"},
		{1, "00000001"},
		{255, "000000ff"},
		{256, "00000100"},
		{0xdeadbeef, "deadbeef"},
	}

	for _, tt := range tests {
		got := formatConnID(tt.id)
		if got != tt.want {
			t.Errorf("formatConnID(%d) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestConnTrackerConcurrency(t *testing.T) {
	emitter := &mockEmitter{}
	ct := NewConnTracker(emitter)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn := &mockConn{
				remoteAddr: mockAddr{network: "tcp", addr: "192.168.1.1:443"},
			}
			wrapped, _ := ct.OnConnect(conn, "example.com:443")
			tc := wrapped.(*trackingConn)

			// Concurrent operations
			ct.SetConnRule(tc.tracked.id, "rule")
			ct.SetConnProcess(tc.tracked.id, "proc")
			wrapped.Write([]byte("test"))
			wrapped.Read(make([]byte, 10))
			wrapped.Close()
		}()
	}
	wg.Wait()

	// All connections should be tracked and closed
	events := emitter.getEvents()
	openCount, closeCount := 0, 0
	for _, e := range events {
		if e.state == "opened" {
			openCount++
		} else if e.state == "closed" {
			closeCount++
		}
	}
	if openCount != 100 {
		t.Errorf("Expected 100 open events, got %d", openCount)
	}
	if closeCount != 100 {
		t.Errorf("Expected 100 close events, got %d", closeCount)
	}
}

func TestConnTrackerDuration(t *testing.T) {
	emitter := &mockEmitter{}
	ct := NewConnTracker(emitter)

	conn := &mockConn{
		remoteAddr: mockAddr{network: "tcp", addr: "192.168.1.1:443"},
	}

	wrapped, _ := ct.OnConnect(conn, "example.com:443")

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	wrapped.Close()

	events := emitter.getEvents()
	closeEvent := events[1]
	if closeEvent.durationMs < 50 {
		t.Errorf("durationMs = %d, expected >= 50", closeEvent.durationMs)
	}
}
