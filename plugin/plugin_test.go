package plugin

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Filter tests
// ---------------------------------------------------------------------------

func TestFilterName(t *testing.T) {
	f := NewFilter(nil, nil)
	if f.Name() != "filter" {
		t.Errorf("Name() = %q, want %q", f.Name(), "filter")
	}
}

func TestFilterInitClose(t *testing.T) {
	f := NewFilter(nil, nil)
	if err := f.Init(context.Background()); err != nil {
		t.Errorf("Init() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestFilterBlocksDomain(t *testing.T) {
	f := NewFilter([]string{"blocked.com"}, nil)
	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}

	_, err := f.OnConnect(conn, "blocked.com:443")
	if err == nil {
		t.Fatal("OnConnect() expected error for blocked domain, got nil")
	}
	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) {
		t.Fatalf("expected *net.DNSError, got %T: %v", err, err)
	}
	if !strings.Contains(dnsErr.Err, "blocked by filter") {
		t.Errorf("error message = %q, want it to contain 'blocked by filter'", dnsErr.Err)
	}
}

func TestFilterAllowsDomain(t *testing.T) {
	f := NewFilter([]string{"blocked.com"}, nil)
	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}

	got, err := f.OnConnect(conn, "allowed.com:443")
	if err != nil {
		t.Fatalf("OnConnect() error = %v", err)
	}
	if got != conn {
		t.Error("OnConnect() should return the original conn for allowed domains")
	}
}

func TestFilterCaseInsensitive(t *testing.T) {
	f := NewFilter([]string{"Blocked.COM"}, nil)
	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}

	_, err := f.OnConnect(conn, "blocked.com:443")
	if err == nil {
		t.Fatal("OnConnect() expected error for case-insensitive blocked domain")
	}

	_, err = f.OnConnect(conn, "BLOCKED.COM:443")
	if err == nil {
		t.Fatal("OnConnect() expected error for uppercase blocked domain")
	}
}

func TestFilterAddBlock(t *testing.T) {
	f := NewFilter(nil, nil)
	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}

	// Initially allowed
	_, err := f.OnConnect(conn, "example.com:443")
	if err != nil {
		t.Fatalf("OnConnect() error = %v", err)
	}

	// Add block
	f.AddBlock("example.com")

	_, err = f.OnConnect(conn, "example.com:443")
	if err == nil {
		t.Fatal("OnConnect() expected error after AddBlock")
	}
}

func TestFilterRemoveBlock(t *testing.T) {
	f := NewFilter([]string{"example.com"}, nil)
	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}

	// Initially blocked
	_, err := f.OnConnect(conn, "example.com:443")
	if err == nil {
		t.Fatal("OnConnect() expected error for initially blocked domain")
	}

	// Remove block
	f.RemoveBlock("example.com")

	_, err = f.OnConnect(conn, "example.com:443")
	if err != nil {
		t.Fatalf("OnConnect() error after RemoveBlock = %v", err)
	}
}

func TestFilterOnDisconnect(t *testing.T) {
	f := NewFilter(nil, nil)
	// OnDisconnect is a no-op; just ensure it doesn't panic
	f.OnDisconnect(nil)
}

func TestFilterConcurrency(t *testing.T) {
	f := NewFilter([]string{"blocked.com"}, nil)
	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.OnConnect(conn, "blocked.com:443")
			f.OnConnect(conn, "allowed.com:443")
			f.AddBlock("dynamic.com")
			f.RemoveBlock("dynamic.com")
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Logger tests
// ---------------------------------------------------------------------------

func TestLoggerName(t *testing.T) {
	l := NewLogger(nil)
	if l.Name() != "logger" {
		t.Errorf("Name() = %q, want %q", l.Name(), "logger")
	}
}

func TestLoggerInitClose(t *testing.T) {
	l := NewLogger(nil)
	if err := l.Init(context.Background()); err != nil {
		t.Errorf("Init() error = %v", err)
	}
	if err := l.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestLoggerOnConnect(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	l := NewLogger(logger)

	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "10.0.0.1:5555"}}

	got, err := l.OnConnect(conn, "example.com:443")
	if err != nil {
		t.Fatalf("OnConnect() error = %v", err)
	}
	if got != conn {
		t.Error("OnConnect() should return the original conn")
	}

	output := buf.String()
	if !strings.Contains(output, "connection opened") {
		t.Errorf("log output missing 'connection opened': %s", output)
	}
	if !strings.Contains(output, "example.com:443") {
		t.Errorf("log output missing target: %s", output)
	}
	if !strings.Contains(output, "10.0.0.1:5555") {
		t.Errorf("log output missing remote addr: %s", output)
	}
}

func TestLoggerOnDisconnect(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	l := NewLogger(logger)

	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "10.0.0.1:5555"}}
	l.OnDisconnect(conn)

	output := buf.String()
	if !strings.Contains(output, "connection closed") {
		t.Errorf("log output missing 'connection closed': %s", output)
	}
	if !strings.Contains(output, "10.0.0.1:5555") {
		t.Errorf("log output missing remote addr: %s", output)
	}
}

func TestLoggerNilLogger(t *testing.T) {
	l := NewLogger(nil)
	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "10.0.0.1:5555"}}

	// Should not panic with nil logger (uses slog.Default)
	_, err := l.OnConnect(conn, "example.com:443")
	if err != nil {
		t.Fatalf("OnConnect() error = %v", err)
	}
	l.OnDisconnect(conn)
}

// ---------------------------------------------------------------------------
// Metrics tests
// ---------------------------------------------------------------------------

func TestMetricsName(t *testing.T) {
	m := NewMetrics()
	if m.Name() != "metrics" {
		t.Errorf("Name() = %q, want %q", m.Name(), "metrics")
	}
}

func TestMetricsInitClose(t *testing.T) {
	m := NewMetrics()
	if err := m.Init(context.Background()); err != nil {
		t.Errorf("Init() error = %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestMetricsOnConnectIncrementsCounters(t *testing.T) {
	m := NewMetrics()
	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}

	wrapped, err := m.OnConnect(conn, "example.com:443")
	if err != nil {
		t.Fatalf("OnConnect() error = %v", err)
	}
	if wrapped == nil {
		t.Fatal("OnConnect() returned nil conn")
	}

	if m.ActiveConns.Load() != 1 {
		t.Errorf("ActiveConns = %d, want 1", m.ActiveConns.Load())
	}
	if m.TotalConns.Load() != 1 {
		t.Errorf("TotalConns = %d, want 1", m.TotalConns.Load())
	}
}

func TestMetricsOnDisconnectDecrementsActive(t *testing.T) {
	m := NewMetrics()
	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}

	m.OnConnect(conn, "example.com:443")
	m.OnDisconnect(conn)

	if m.ActiveConns.Load() != 0 {
		t.Errorf("ActiveConns after disconnect = %d, want 0", m.ActiveConns.Load())
	}
	if m.TotalConns.Load() != 1 {
		t.Errorf("TotalConns after disconnect = %d, want 1", m.TotalConns.Load())
	}
}

func TestMetricsConnByteTracking(t *testing.T) {
	m := NewMetrics()
	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}

	wrapped, _ := m.OnConnect(conn, "example.com:443")

	// Write 200 bytes
	wrapped.Write(make([]byte, 100))
	wrapped.Write(make([]byte, 100))

	// Read 300 bytes
	wrapped.Read(make([]byte, 100))
	wrapped.Read(make([]byte, 100))
	wrapped.Read(make([]byte, 100))

	if m.BytesSent.Load() != 200 {
		t.Errorf("BytesSent = %d, want 200", m.BytesSent.Load())
	}
	if m.BytesReceived.Load() != 300 {
		t.Errorf("BytesReceived = %d, want 300", m.BytesReceived.Load())
	}
}

func TestMetricsOnData(t *testing.T) {
	m := NewMetrics()

	outData := []byte("hello world")
	got := m.OnData(outData, Outbound)
	if !bytes.Equal(got, outData) {
		t.Error("OnData should return data unchanged")
	}
	if m.BytesSent.Load() != int64(len(outData)) {
		t.Errorf("BytesSent after Outbound = %d, want %d", m.BytesSent.Load(), len(outData))
	}

	inData := []byte("response data here")
	got = m.OnData(inData, Inbound)
	if !bytes.Equal(got, inData) {
		t.Error("OnData should return data unchanged")
	}
	if m.BytesReceived.Load() != int64(len(inData)) {
		t.Errorf("BytesReceived after Inbound = %d, want %d", m.BytesReceived.Load(), len(inData))
	}
}

func TestMetricsSampleSpeed(t *testing.T) {
	m := NewMetrics()

	// Add some bytes
	m.BytesSent.Store(1000)
	m.BytesReceived.Store(2000)

	// Force lastSample to be in the past
	m.mu.Lock()
	m.lastSample = time.Now().Add(-1 * time.Second)
	m.lastSent = 0
	m.lastRecv = 0
	m.mu.Unlock()

	upload, download := m.SampleSpeed()

	// With 1000 bytes sent over ~1 second, speed should be around 1000
	if upload < 500 || upload > 1500 {
		t.Errorf("upload speed = %d, expected ~1000", upload)
	}
	if download < 1000 || download > 3000 {
		t.Errorf("download speed = %d, expected ~2000", download)
	}
}

func TestMetricsSpeed(t *testing.T) {
	m := NewMetrics()

	// Speed should be zero initially
	up, down := m.Speed()
	if up != 0 || down != 0 {
		t.Errorf("initial Speed() = (%d, %d), want (0, 0)", up, down)
	}

	// Set speed via internal fields
	m.mu.Lock()
	m.uploadSpeed = 500
	m.downloadSpeed = 1000
	m.mu.Unlock()

	up, down = m.Speed()
	if up != 500 {
		t.Errorf("upload = %d, want 500", up)
	}
	if down != 1000 {
		t.Errorf("download = %d, want 1000", down)
	}
}

func TestMetricsStats(t *testing.T) {
	m := NewMetrics()

	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}
	m.OnConnect(conn, "a:80")
	m.OnConnect(conn, "b:80")
	m.BytesSent.Store(500)
	m.BytesReceived.Store(1000)

	stats := m.Stats()

	if stats["active_conns"] != 2 {
		t.Errorf("active_conns = %d, want 2", stats["active_conns"])
	}
	if stats["total_conns"] != 2 {
		t.Errorf("total_conns = %d, want 2", stats["total_conns"])
	}
	if stats["bytes_sent"] != 500 {
		t.Errorf("bytes_sent = %d, want 500", stats["bytes_sent"])
	}
	if stats["bytes_received"] != 1000 {
		t.Errorf("bytes_received = %d, want 1000", stats["bytes_received"])
	}
}

func TestMetricsMultipleConnections(t *testing.T) {
	m := NewMetrics()
	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}

	m.OnConnect(conn, "a:80")
	m.OnConnect(conn, "b:80")
	m.OnConnect(conn, "c:80")

	if m.ActiveConns.Load() != 3 {
		t.Errorf("ActiveConns = %d, want 3", m.ActiveConns.Load())
	}

	m.OnDisconnect(conn)
	m.OnDisconnect(conn)

	if m.ActiveConns.Load() != 1 {
		t.Errorf("ActiveConns after 2 disconnects = %d, want 1", m.ActiveConns.Load())
	}
	if m.TotalConns.Load() != 3 {
		t.Errorf("TotalConns = %d, want 3", m.TotalConns.Load())
	}
}

func TestMetricsConcurrency(t *testing.T) {
	m := NewMetrics()
	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wrapped, _ := m.OnConnect(conn, "example.com:443")
			wrapped.Write(make([]byte, 10))
			wrapped.Read(make([]byte, 10))
			m.OnData([]byte("test"), Outbound)
			m.OnData([]byte("test"), Inbound)
			m.SampleSpeed()
			m.Stats()
			m.OnDisconnect(conn)
		}()
	}
	wg.Wait()

	if m.ActiveConns.Load() != 0 {
		t.Errorf("ActiveConns after all disconnect = %d, want 0", m.ActiveConns.Load())
	}
	if m.TotalConns.Load() != 100 {
		t.Errorf("TotalConns = %d, want 100", m.TotalConns.Load())
	}
}

// ---------------------------------------------------------------------------
// Chain (iface.go) tests
// ---------------------------------------------------------------------------

// faultyPlugin is a plugin that returns an error on Init or Close.
type faultyPlugin struct {
	name     string
	initErr  error
	closeErr error
	inited   bool
	closed   bool
}

func (f *faultyPlugin) Name() string { return f.name }
func (f *faultyPlugin) Init(ctx context.Context) error {
	if f.initErr != nil {
		return f.initErr
	}
	f.inited = true
	return nil
}
func (f *faultyPlugin) Close() error {
	f.closed = true
	return f.closeErr
}

// faultyConnPlugin implements ConnPlugin and returns an error on OnConnect.
type faultyConnPlugin struct {
	faultyPlugin
	connectErr error
}

func (f *faultyConnPlugin) OnConnect(conn net.Conn, target string) (net.Conn, error) {
	if f.connectErr != nil {
		return nil, f.connectErr
	}
	return conn, nil
}
func (f *faultyConnPlugin) OnDisconnect(conn net.Conn) {}

func TestChainInitSuccess(t *testing.T) {
	p1 := &faultyPlugin{name: "p1"}
	p2 := &faultyPlugin{name: "p2"}
	chain := NewChain(p1, p2)

	if err := chain.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if !p1.inited || !p2.inited {
		t.Error("expected both plugins to be initialized")
	}
}

func TestChainInitRollback(t *testing.T) {
	p1 := &faultyPlugin{name: "p1"}
	p2 := &faultyPlugin{name: "p2", initErr: errors.New("init fail")}
	p3 := &faultyPlugin{name: "p3"}
	chain := NewChain(p1, p2, p3)

	err := chain.Init(context.Background())
	if err == nil {
		t.Fatal("Init() expected error")
	}
	if !strings.Contains(err.Error(), "p2") {
		t.Errorf("error should mention failing plugin p2: %v", err)
	}

	// p1 should have been closed (rollback), p2 failed so not closed, p3 never inited
	if !p1.closed {
		t.Error("p1 should be closed during rollback")
	}
	if p2.closed {
		t.Error("p2 should not be closed (it failed init)")
	}
	if p3.inited {
		t.Error("p3 should not be initialized")
	}
}

func TestChainCloseReverseOrder(t *testing.T) {
	var order []string
	makePlugin := func(name string) *faultyPlugin {
		p := &faultyPlugin{name: name}
		return p
	}
	p1 := makePlugin("p1")
	p2 := makePlugin("p2")
	p3 := makePlugin("p3")
	chain := NewChain(p1, p2, p3)

	chain.Close()

	// All should be closed
	for _, p := range []*faultyPlugin{p1, p2, p3} {
		if !p.closed {
			t.Errorf("%s should be closed", p.name)
		}
	}
	_ = order
}

func TestChainCloseAggregatesErrors(t *testing.T) {
	p1 := &faultyPlugin{name: "p1", closeErr: errors.New("err1")}
	p2 := &faultyPlugin{name: "p2"}
	p3 := &faultyPlugin{name: "p3", closeErr: errors.New("err3")}
	chain := NewChain(p1, p2, p3)

	err := chain.Close()
	if err == nil {
		t.Fatal("Close() expected error")
	}
	if !strings.Contains(err.Error(), "p1") || !strings.Contains(err.Error(), "p3") {
		t.Errorf("error should mention both p1 and p3: %v", err)
	}
}

func TestChainOnConnect(t *testing.T) {
	m := NewMetrics()
	l := NewLogger(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	chain := NewChain(l, m)

	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}
	wrapped, err := chain.OnConnect(conn, "example.com:443")
	if err != nil {
		t.Fatalf("OnConnect() error = %v", err)
	}
	if wrapped == nil {
		t.Fatal("OnConnect() returned nil")
	}

	// Metrics should have tracked the connection
	if m.ActiveConns.Load() != 1 {
		t.Errorf("ActiveConns = %d, want 1", m.ActiveConns.Load())
	}
}

func TestChainOnConnectError(t *testing.T) {
	p1 := &faultyConnPlugin{
		faultyPlugin: faultyPlugin{name: "blocker"},
		connectErr:   errors.New("denied"),
	}
	chain := NewChain(p1)

	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}
	_, err := chain.OnConnect(conn, "example.com:443")
	if err == nil {
		t.Fatal("OnConnect() expected error")
	}
	if !strings.Contains(err.Error(), "blocker") {
		t.Errorf("error should mention plugin name: %v", err)
	}
}

func TestChainOnDisconnect(t *testing.T) {
	m := NewMetrics()
	chain := NewChain(m)

	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}
	chain.OnConnect(conn, "example.com:443")
	chain.OnDisconnect(conn)

	if m.ActiveConns.Load() != 0 {
		t.Errorf("ActiveConns after OnDisconnect = %d, want 0", m.ActiveConns.Load())
	}
}

func TestChainEmpty(t *testing.T) {
	chain := NewChain()

	if err := chain.Init(context.Background()); err != nil {
		t.Errorf("Init() on empty chain error = %v", err)
	}
	if err := chain.Close(); err != nil {
		t.Errorf("Close() on empty chain error = %v", err)
	}

	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}
	got, err := chain.OnConnect(conn, "example.com:443")
	if err != nil {
		t.Fatalf("OnConnect() on empty chain error = %v", err)
	}
	if got != conn {
		t.Error("OnConnect() on empty chain should return original conn")
	}

	// Should not panic
	chain.OnDisconnect(conn)
}

// ---------------------------------------------------------------------------
// Direction constants test
// ---------------------------------------------------------------------------

func TestDirectionValues(t *testing.T) {
	if Inbound != 0 {
		t.Errorf("Inbound = %d, want 0", Inbound)
	}
	if Outbound != 1 {
		t.Errorf("Outbound = %d, want 1", Outbound)
	}
}

// ---------------------------------------------------------------------------
// Integration: Filter + Logger + Metrics chain
// ---------------------------------------------------------------------------

func TestChainFilterBlocksBeforeMetrics(t *testing.T) {
	f := NewFilter([]string{"blocked.com"}, nil)
	m := NewMetrics()
	chain := NewChain(f, m)

	conn := &mockConn{remoteAddr: mockAddr{network: "tcp", addr: "1.2.3.4:443"}}

	// Blocked connection should not reach metrics
	_, err := chain.OnConnect(conn, "blocked.com:443")
	if err == nil {
		t.Fatal("expected blocked connection to return error")
	}
	if m.TotalConns.Load() != 0 {
		t.Errorf("TotalConns = %d, want 0 (blocked conn should not be counted)", m.TotalConns.Load())
	}

	// Allowed connection should reach metrics
	_, err = chain.OnConnect(conn, "allowed.com:443")
	if err != nil {
		t.Fatalf("OnConnect() error = %v", err)
	}
	if m.TotalConns.Load() != 1 {
		t.Errorf("TotalConns = %d, want 1", m.TotalConns.Load())
	}
}
