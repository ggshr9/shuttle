package transport

import (
	"context"
	"io"
	"net"
	"testing"
)

// mockStream implements Stream for compile-time verification.
type mockStream struct {
	id uint64
}

func (m *mockStream) Read(p []byte) (int, error)  { return 0, io.EOF }
func (m *mockStream) Write(p []byte) (int, error) { return len(p), nil }
func (m *mockStream) Close() error                { return nil }
func (m *mockStream) StreamID() uint64             { return m.id }

// mockConnection implements Connection.
type mockConnection struct{}

func (m *mockConnection) OpenStream(ctx context.Context) (Stream, error) {
	return &mockStream{id: 1}, nil
}
func (m *mockConnection) AcceptStream(ctx context.Context) (Stream, error) {
	return &mockStream{id: 2}, nil
}
func (m *mockConnection) Close() error          { return nil }
func (m *mockConnection) LocalAddr() net.Addr   { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1234} }
func (m *mockConnection) RemoteAddr() net.Addr  { return &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 443} }

// mockClientTransport implements ClientTransport.
type mockClientTransport struct{}

func (m *mockClientTransport) Dial(ctx context.Context, addr string) (Connection, error) {
	return &mockConnection{}, nil
}
func (m *mockClientTransport) Type() string { return "mock" }
func (m *mockClientTransport) Close() error { return nil }

// mockServerTransport implements ServerTransport.
type mockServerTransport struct{}

func (m *mockServerTransport) Listen(ctx context.Context) error { return nil }
func (m *mockServerTransport) Accept(ctx context.Context) (Connection, error) {
	return &mockConnection{}, nil
}
func (m *mockServerTransport) Type() string { return "mock-server" }
func (m *mockServerTransport) Close() error { return nil }

// Compile-time interface compliance checks.
var _ Stream = (*mockStream)(nil)
var _ Connection = (*mockConnection)(nil)
var _ ClientTransport = (*mockClientTransport)(nil)
var _ ServerTransport = (*mockServerTransport)(nil)

func TestStreamInterface(t *testing.T) {
	s := &mockStream{id: 42}
	if s.StreamID() != 42 {
		t.Fatalf("StreamID = %d, want 42", s.StreamID())
	}
	n, err := s.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("Write: n=%d, err=%v", n, err)
	}
	_, err = s.Read(make([]byte, 10))
	if err != io.EOF {
		t.Fatalf("Read: expected io.EOF, got %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestConnectionInterface(t *testing.T) {
	conn := &mockConnection{}

	stream, err := conn.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	if stream.StreamID() != 1 {
		t.Fatalf("OpenStream ID = %d, want 1", stream.StreamID())
	}

	stream2, err := conn.AcceptStream(context.Background())
	if err != nil {
		t.Fatalf("AcceptStream: %v", err)
	}
	if stream2.StreamID() != 2 {
		t.Fatalf("AcceptStream ID = %d, want 2", stream2.StreamID())
	}

	if conn.LocalAddr().String() != "127.0.0.1:1234" {
		t.Fatalf("LocalAddr = %s", conn.LocalAddr())
	}
	if conn.RemoteAddr().String() != "10.0.0.1:443" {
		t.Fatalf("RemoteAddr = %s", conn.RemoteAddr())
	}
}

func TestClientTransportInterface(t *testing.T) {
	ct := &mockClientTransport{}
	if ct.Type() != "mock" {
		t.Fatalf("Type = %s, want mock", ct.Type())
	}
	conn, err := ct.Dial(context.Background(), "example.com:443")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if conn == nil {
		t.Fatal("Dial returned nil connection")
	}
	if err := ct.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestServerTransportInterface(t *testing.T) {
	st := &mockServerTransport{}
	if st.Type() != "mock-server" {
		t.Fatalf("Type = %s, want mock-server", st.Type())
	}
	if err := st.Listen(context.Background()); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	conn, err := st.Accept(context.Background())
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if conn == nil {
		t.Fatal("Accept returned nil connection")
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestTransportConfig(t *testing.T) {
	cfg := TransportConfig{
		ServerAddr:   "example.com:443",
		ServerName:   "example.com",
		Password:     "secret",
		InsecureSkip: true,
	}
	if cfg.ServerAddr != "example.com:443" {
		t.Fatal("ServerAddr mismatch")
	}
	if cfg.InsecureSkip != true {
		t.Fatal("InsecureSkip mismatch")
	}
}
