package adapter_test

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/shuttleX/shuttle/adapter"
)

// startEchoServer starts a TCP echo server and returns its address and a stop function.
func startEchoServer(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start echo server: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go io.Copy(conn, conn) //nolint:errcheck
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

// mockDialer wraps net.Dial as a Dialer.
type mockDialer struct {
	network string
}

func (m *mockDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, m.network, address)
}
func (m *mockDialer) Type() string { return "mock" }
func (m *mockDialer) Close() error { return nil }

// TestDialerAsTransport verifies that DialerAsTransport wraps a Dialer into a
// ClientTransport where Dial→OpenStream→Write→Read round-trips correctly.
func TestDialerAsTransport(t *testing.T) {
	addr, stop := startEchoServer(t)
	defer stop()

	d := &mockDialer{network: "tcp"}
	transport := adapter.DialerAsTransport(d)

	ctx := context.Background()
	conn, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	stream, err := conn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	defer stream.Close()

	const msg = "hello bridge"
	if _, err := stream.Write([]byte(msg)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(stream, buf); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(buf) != msg {
		t.Fatalf("echo mismatch: got %q, want %q", buf, msg)
	}
}

// TestDialerAsTransport_OpenStreamOnce verifies that OpenStream can only be
// called once on a singleStreamConn.
func TestDialerAsTransport_OpenStreamOnce(t *testing.T) {
	addr, stop := startEchoServer(t)
	defer stop()

	d := &mockDialer{network: "tcp"}
	transport := adapter.DialerAsTransport(d)

	ctx := context.Background()
	conn, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.OpenStream(ctx); err != nil {
		t.Fatalf("first OpenStream: %v", err)
	}
	if _, err := conn.OpenStream(ctx); err == nil {
		t.Fatal("expected error on second OpenStream, got nil")
	}
}

// TestDialerAsTransport_AcceptStreamBlocksOnCancel verifies that AcceptStream
// blocks and returns when the context is cancelled.
func TestDialerAsTransport_AcceptStreamBlocksOnCancel(t *testing.T) {
	addr, stop := startEchoServer(t)
	defer stop()

	d := &mockDialer{network: "tcp"}
	transport := adapter.DialerAsTransport(d)

	ctx, cancel := context.WithCancel(context.Background())

	conn, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	done := make(chan error, 1)
	go func() {
		_, err := conn.AcceptStream(ctx)
		done <- err
	}()

	cancel()
	err = <-done
	if err == nil {
		t.Fatal("expected non-nil error from AcceptStream after cancel")
	}
}

// TestTransportAsDialer wraps a DialerAsTransport result back into a Dialer and
// verifies that DialContext→Write→Read round-trips correctly.
func TestTransportAsDialer(t *testing.T) {
	addr, stop := startEchoServer(t)
	defer stop()

	d := &mockDialer{network: "tcp"}
	transport := adapter.DialerAsTransport(d)
	dialer := adapter.TransportAsDialer(transport, addr)

	ctx := context.Background()
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	defer conn.Close()

	const msg = "round trip"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(buf) != msg {
		t.Fatalf("echo mismatch: got %q, want %q", buf, msg)
	}
}

// TestBridgeType verifies that Type() propagates correctly through both bridge adapters.
func TestBridgeType(t *testing.T) {
	d := &mockDialer{network: "tcp"}
	transport := adapter.DialerAsTransport(d)
	if transport.Type() != "mock" {
		t.Errorf("DialerAsTransport.Type() = %q, want %q", transport.Type(), "mock")
	}

	dialer := adapter.TransportAsDialer(transport, "127.0.0.1:0")
	if dialer.Type() != "mock" {
		t.Errorf("TransportAsDialer.Type() = %q, want %q", dialer.Type(), "mock")
	}
}

// TestDialerAsTransport_StreamID verifies that the stream's StreamID is 0.
func TestDialerAsTransport_StreamID(t *testing.T) {
	addr, stop := startEchoServer(t)
	defer stop()

	d := &mockDialer{network: "tcp"}
	transport := adapter.DialerAsTransport(d)

	ctx := context.Background()
	conn, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	stream, err := conn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	defer stream.Close()

	if stream.StreamID() != 0 {
		t.Errorf("StreamID() = %d, want 0", stream.StreamID())
	}
}
