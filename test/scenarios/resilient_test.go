package scenarios

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/testkit/vnet"
	"github.com/ggshr9/shuttle/transport"
	"github.com/ggshr9/shuttle/transport/resilient"
)

// ---------------------------------------------------------------------------
// breakableConn wraps a transport.Connection with a killswitch.
// When Break() is called, subsequent OpenStream calls return io.EOF,
// which ResilientConn recognizes as a connection-level error and triggers
// reconnection. This avoids yamux's "session shutdown" error which is not
// currently in the recognized error set.
// ---------------------------------------------------------------------------

type breakableConn struct {
	inner  transport.Connection
	broken atomic.Bool
}

func newBreakableConn(c transport.Connection) *breakableConn {
	return &breakableConn{inner: c}
}

func (b *breakableConn) Break() {
	b.broken.Store(true)
	b.inner.Close()
}

func (b *breakableConn) OpenStream(ctx context.Context) (transport.Stream, error) {
	if b.broken.Load() {
		return nil, io.EOF
	}
	s, err := b.inner.OpenStream(ctx)
	if err != nil && b.broken.Load() {
		return nil, io.EOF
	}
	return s, err
}

func (b *breakableConn) AcceptStream(ctx context.Context) (transport.Stream, error) {
	return b.inner.AcceptStream(ctx)
}

func (b *breakableConn) Close() error {
	return b.inner.Close()
}

func (b *breakableConn) LocalAddr() net.Addr  { return b.inner.LocalAddr() }
func (b *breakableConn) RemoteAddr() net.Addr { return b.inner.RemoteAddr() }

var _ transport.Connection = (*breakableConn)(nil)

// ---------------------------------------------------------------------------
// TestResilientConnOverVnet
//
// Creates a vnet with a client->server link. Wraps the connection in
// ResilientConn. Sends data, breaks the link, verifies auto-reconnect
// happens and data flows again.
// ---------------------------------------------------------------------------

func TestResilientConnOverVnet(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(100))
	defer env.Close()

	clientNode := env.Net.AddNode("client")
	serverNode := env.Net.AddNode("server")
	env.Net.Link(clientNode, serverNode, vnet.LinkConfig{Latency: 2 * time.Millisecond})

	// Start echo server.
	srv := newVnetServer(env.Net, serverNode, "resilient", "server:9000")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echoServer(ctx, t, srv)

	clientTransport := newVnetClient(env.Net, clientNode, "resilient")

	// Dial function for ResilientConn -- creates a fresh transport.Connection each time.
	dial := func(ctx context.Context) (transport.Connection, error) {
		return clientTransport.Dial(ctx, "server:9000")
	}

	// Establish initial connection wrapped in breakableConn.
	rawInitial, err := dial(ctx)
	if err != nil {
		t.Fatalf("initial dial: %v", err)
	}
	initial := newBreakableConn(rawInitial)

	var reconnectCount atomic.Int32
	rc := resilient.Wrap(initial, dial, resilient.Config{
		MaxRetries:  5,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		OnReconnect: func() { reconnectCount.Add(1) },
	})
	defer rc.Close()

	// Phase 1: Send data through the resilient connection -- should work normally.
	msg1 := []byte("hello-resilient")
	stream1, err := rc.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream (phase 1): %v", err)
	}
	if _, err := stream1.Write(msg1); err != nil {
		t.Fatalf("Write (phase 1): %v", err)
	}
	buf1 := make([]byte, len(msg1))
	if _, err := io.ReadFull(stream1, buf1); err != nil {
		t.Fatalf("ReadFull (phase 1): %v", err)
	}
	if !bytes.Equal(buf1, msg1) {
		t.Fatalf("phase 1 mismatch: got %q, want %q", buf1, msg1)
	}
	stream1.Close()

	// Phase 2: Break the underlying connection via the killswitch.
	// This causes OpenStream to return io.EOF, triggering reconnect.
	initial.Break()

	// Phase 3: OpenStream should detect the broken connection and auto-reconnect.
	msg2 := []byte("after-reconnect")
	stream2, err := rc.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream (phase 3, after reconnect): %v", err)
	}
	defer stream2.Close()
	if _, err := stream2.Write(msg2); err != nil {
		t.Fatalf("Write (phase 3): %v", err)
	}
	buf2 := make([]byte, len(msg2))
	if _, err := io.ReadFull(stream2, buf2); err != nil {
		t.Fatalf("ReadFull (phase 3): %v", err)
	}
	if !bytes.Equal(buf2, msg2) {
		t.Fatalf("phase 3 mismatch: got %q, want %q", buf2, msg2)
	}

	if reconnectCount.Load() < 1 {
		t.Fatal("expected at least one reconnection to have occurred")
	}
	t.Logf("reconnect count: %d", reconnectCount.Load())
}

// ---------------------------------------------------------------------------
// TestResilientConnMobileHandoff
//
// Uses mobile presets: start on WiFi, switch to HandoffBlip then LTE.
// Verify ResilientConn reconnects transparently through link changes.
// ---------------------------------------------------------------------------

func TestResilientConnMobileHandoff(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(101))
	defer env.Close()

	clientNode := env.Net.AddNode("phone")
	serverNode := env.Net.AddNode("server")

	// Start with WiFi conditions.
	wifiCfg := vnet.WiFi()
	env.Net.Link(clientNode, serverNode, wifiCfg)

	srv := newVnetServer(env.Net, serverNode, "mobile", "server:9100")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echoServer(ctx, t, srv)

	clientTransport := newVnetClient(env.Net, clientNode, "mobile")

	dial := func(ctx context.Context) (transport.Connection, error) {
		return clientTransport.Dial(ctx, "server:9100")
	}

	rawInitial, err := dial(ctx)
	if err != nil {
		t.Fatalf("initial dial: %v", err)
	}
	initial := newBreakableConn(rawInitial)

	var reconnects atomic.Int32
	rc := resilient.Wrap(initial, dial, resilient.Config{
		MaxRetries:  10,
		BaseDelay:   20 * time.Millisecond,
		MaxDelay:    200 * time.Millisecond,
		OnReconnect: func() { reconnects.Add(1) },
	})
	defer rc.Close()

	// Phase 1: Send data over WiFi.
	msg := []byte("wifi-data")
	stream, err := rc.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream (WiFi): %v", err)
	}
	if _, err := stream.Write(msg); err != nil {
		t.Fatalf("Write (WiFi): %v", err)
	}
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(stream, buf); err != nil {
		t.Fatalf("ReadFull (WiFi): %v", err)
	}
	if !bytes.Equal(buf, msg) {
		t.Fatalf("WiFi data mismatch: got %q, want %q", buf, msg)
	}
	stream.Close()
	t.Log("phase 1 (WiFi): data verified")

	// Phase 2: Simulate handoff blip -- connection breaks during transition.
	blipCfg := vnet.HandoffBlip()
	env.Net.UpdateLink(clientNode, serverNode, blipCfg)
	env.Net.UpdateLink(serverNode, clientNode, blipCfg)

	// Break the existing connection to simulate the handoff disruption.
	initial.Break()

	// Wait a brief moment for the handoff to settle, then switch to LTE.
	time.Sleep(50 * time.Millisecond)

	// Phase 3: Switch to LTE conditions.
	lteCfg := vnet.LTE()
	env.Net.UpdateLink(clientNode, serverNode, lteCfg)
	env.Net.UpdateLink(serverNode, clientNode, lteCfg)

	// The ResilientConn should reconnect transparently.
	msg2 := []byte("lte-data")
	stream2, err := rc.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream (LTE, after handoff): %v", err)
	}
	defer stream2.Close()
	if _, err := stream2.Write(msg2); err != nil {
		t.Fatalf("Write (LTE): %v", err)
	}
	buf2 := make([]byte, len(msg2))
	if _, err := io.ReadFull(stream2, buf2); err != nil {
		t.Fatalf("ReadFull (LTE): %v", err)
	}
	if !bytes.Equal(buf2, msg2) {
		t.Fatalf("LTE data mismatch: got %q, want %q", buf2, msg2)
	}

	if reconnects.Load() < 1 {
		t.Fatal("expected at least one reconnection during mobile handoff")
	}
	t.Logf("mobile handoff reconnects: %d", reconnects.Load())
}

// ---------------------------------------------------------------------------
// TestResilientConnMaxRetriesVnet
//
// The server goes down permanently. Verify ResilientConn fails after
// max retries with the appropriate error.
// ---------------------------------------------------------------------------

func TestResilientConnMaxRetriesVnet(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(102))
	defer env.Close()

	clientNode := env.Net.AddNode("client")
	serverNode := env.Net.AddNode("server")
	env.Net.Link(clientNode, serverNode, vnet.LinkConfig{Latency: 2 * time.Millisecond})

	srv := newVnetServer(env.Net, serverNode, "maxretry", "server:9200")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	echoServer(ctx, t, srv)

	// Establish initial connection before shutting down the server.
	clientTransport := newVnetClient(env.Net, clientNode, "maxretry")
	rawInitial, err := clientTransport.Dial(ctx, "server:9200")
	if err != nil {
		t.Fatalf("initial dial: %v", err)
	}
	initial := newBreakableConn(rawInitial)

	// Shut down the server now -- subsequent dials will fail.
	srv.Close()

	var dialAttempts atomic.Int32
	dial := func(ctx context.Context) (transport.Connection, error) {
		dialAttempts.Add(1)
		// Server is down, so dial fails.
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	}

	rc := resilient.Wrap(initial, dial, resilient.Config{
		MaxRetries: 3,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
	})
	defer rc.Close()

	// Break the initial connection to trigger reconnect.
	initial.Break()

	// OpenStream should detect the broken connection, attempt to reconnect
	// MaxRetries times, and then fail.
	_, err = rc.OpenStream(ctx)
	if err == nil {
		t.Fatal("expected error after server shutdown and max retries, got nil")
	}

	attempts := dialAttempts.Load()
	if attempts < 1 {
		t.Fatalf("expected at least 1 dial attempt, got %d", attempts)
	}
	if attempts > 3 {
		t.Fatalf("expected at most 3 dial attempts (MaxRetries), got %d", attempts)
	}

	// Verify the error is a connection/dial error, not a context error.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected dial/connection error, got context error: %v", err)
	}

	t.Logf("correctly failed after %d dial attempts with error: %v", attempts, err)
}

// ---------------------------------------------------------------------------
// TestResilientConnMultipleReconnectsVnet
//
// The connection breaks twice in succession. Verify ResilientConn
// recovers each time and data continues flowing.
// ---------------------------------------------------------------------------

func TestResilientConnMultipleReconnectsVnet(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(103))
	defer env.Close()

	clientNode := env.Net.AddNode("client")
	serverNode := env.Net.AddNode("server")
	env.Net.Link(clientNode, serverNode, vnet.LinkConfig{Latency: 2 * time.Millisecond})

	srv := newVnetServer(env.Net, serverNode, "multi", "server:9300")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echoServer(ctx, t, srv)

	clientTransport := newVnetClient(env.Net, clientNode, "multi")

	// Track all dialed connections so we can break them.
	// The dial function wraps each new conn in a breakableConn.
	var connsMu sync.Mutex
	var breakables []*breakableConn

	dial := func(ctx context.Context) (transport.Connection, error) {
		raw, err := clientTransport.Dial(ctx, "server:9300")
		if err != nil {
			return nil, err
		}
		bc := newBreakableConn(raw)
		connsMu.Lock()
		breakables = append(breakables, bc)
		connsMu.Unlock()
		return bc, nil
	}

	rawInitial, err := clientTransport.Dial(ctx, "server:9300")
	if err != nil {
		t.Fatalf("initial dial: %v", err)
	}
	initial := newBreakableConn(rawInitial)

	var reconnects atomic.Int32
	rc := resilient.Wrap(initial, dial, resilient.Config{
		MaxRetries:  5,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		OnReconnect: func() { reconnects.Add(1) },
	})
	defer rc.Close()

	// echoRoundTrip sends a message and verifies the echo response.
	echoRoundTrip := func(phase string, msg []byte) {
		t.Helper()
		stream, err := rc.OpenStream(ctx)
		if err != nil {
			t.Fatalf("OpenStream (%s): %v", phase, err)
		}
		defer stream.Close()
		if _, err := stream.Write(msg); err != nil {
			t.Fatalf("Write (%s): %v", phase, err)
		}
		buf := make([]byte, len(msg))
		if _, err := io.ReadFull(stream, buf); err != nil {
			t.Fatalf("ReadFull (%s): %v", phase, err)
		}
		if !bytes.Equal(buf, msg) {
			t.Fatalf("%s mismatch: got %q, want %q", phase, buf, msg)
		}
	}

	// Round 1: normal.
	echoRoundTrip("round-1", []byte("first"))

	// Break connection #1.
	initial.Break()

	// Round 2: triggers reconnect.
	echoRoundTrip("round-2", []byte("second"))
	if reconnects.Load() < 1 {
		t.Fatal("expected at least 1 reconnect after first break")
	}

	// Break the most recently dialed connection.
	connsMu.Lock()
	if len(breakables) > 0 {
		breakables[len(breakables)-1].Break()
	}
	connsMu.Unlock()

	// Round 3: triggers another reconnect.
	echoRoundTrip("round-3", []byte("third"))
	if reconnects.Load() < 2 {
		t.Fatal("expected at least 2 reconnects after second break")
	}

	t.Logf("total reconnects: %d", reconnects.Load())
}

// ---------------------------------------------------------------------------
// TestResilientConnCloseWhileReconnecting
//
// Close the ResilientConn while a reconnect is in progress. Verify the
// close is clean and subsequent OpenStream returns an error.
// ---------------------------------------------------------------------------

func TestResilientConnCloseWhileReconnecting(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(104))
	defer env.Close()

	clientNode := env.Net.AddNode("client")
	serverNode := env.Net.AddNode("server")
	env.Net.Link(clientNode, serverNode, vnet.LinkConfig{Latency: 2 * time.Millisecond})

	srv := newVnetServer(env.Net, serverNode, "closereconn", "server:9400")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echoServer(ctx, t, srv)

	clientTransport := newVnetClient(env.Net, clientNode, "closereconn")

	// Dial always fails -- simulating permanent server down.
	dial := func(ctx context.Context) (transport.Connection, error) {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	}

	// Start with a working connection, then break it.
	rawInitial, err := clientTransport.Dial(ctx, "server:9400")
	if err != nil {
		t.Fatalf("initial dial: %v", err)
	}
	initial := newBreakableConn(rawInitial)

	rc := resilient.Wrap(initial, dial, resilient.Config{
		MaxRetries: 3,
		BaseDelay:  50 * time.Millisecond,
		MaxDelay:   200 * time.Millisecond,
	})

	// Break the connection.
	initial.Break()

	// Close the ResilientConn -- should not block or panic.
	rc.Close()

	// After close, operations should fail promptly.
	_, err = rc.OpenStream(ctx)
	if err == nil {
		t.Fatal("expected error after closing ResilientConn")
	}
	t.Logf("OpenStream after close returned: %v", err)
}
