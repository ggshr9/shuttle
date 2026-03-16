package conformance

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/transport"
)

// TransportFactory creates a matched client+server pair for testing.
// cleanup must close both and free resources.
type TransportFactory func(t testing.TB) (
	client transport.ClientTransport,
	server transport.ServerTransport,
	serverAddr string,
	cleanup func(),
)

// RunSuite runs the full conformance suite against a transport implementation.
func RunSuite(t *testing.T, factory TransportFactory) {
	t.Run("DialAndAccept", func(t *testing.T) {
		t.Parallel()
		testDialAndAccept(t, factory)
	})
	t.Run("StreamRoundTrip", func(t *testing.T) {
		t.Parallel()
		testStreamRoundTrip(t, factory)
	})
	t.Run("MultiplexStreams", func(t *testing.T) {
		t.Parallel()
		testMultiplexStreams(t, factory)
	})
	t.Run("HalfClose", func(t *testing.T) {
		t.Parallel()
		testHalfClose(t, factory)
	})
	t.Run("GracefulClose", func(t *testing.T) {
		t.Parallel()
		testGracefulClose(t, factory)
	})
	t.Run("ConcurrentStreamOps", func(t *testing.T) {
		t.Parallel()
		testConcurrentStreamOps(t, factory)
	})
	t.Run("CancelledContext", func(t *testing.T) {
		t.Parallel()
		testCancelledContext(t, factory)
	})
	t.Run("TypeNonEmpty", func(t *testing.T) {
		t.Parallel()
		testTypeNonEmpty(t, factory)
	})
	t.Run("StreamID", func(t *testing.T) {
		t.Parallel()
		testStreamID(t, factory)
	})
	t.Run("LargePayload", func(t *testing.T) {
		t.Parallel()
		testLargePayload(t, factory)
	})
}

// dial connects a client and accepts on the server, returning both connections.
func dial(t testing.TB, factory TransportFactory) (
	clientConn, serverConn transport.Connection, cleanup func(),
) {
	t.Helper()
	client, server, addr, factoryCleanup := factory(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start server listening.
	if err := server.Listen(ctx); err != nil {
		factoryCleanup()
		t.Fatalf("server.Listen: %v", err)
	}

	// Accept runs in background.
	type acceptResult struct {
		conn transport.Connection
		err  error
	}
	acceptCh := make(chan acceptResult, 1)
	go func() {
		c, err := server.Accept(ctx)
		acceptCh <- acceptResult{c, err}
	}()

	cc, err := client.Dial(ctx, addr)
	if err != nil {
		factoryCleanup()
		t.Fatalf("client.Dial: %v", err)
	}

	ar := <-acceptCh
	if ar.err != nil {
		cc.Close()
		factoryCleanup()
		t.Fatalf("server.Accept: %v", ar.err)
	}

	return cc, ar.conn, func() {
		cc.Close()
		ar.conn.Close()
		factoryCleanup()
	}
}

func testDialAndAccept(t *testing.T, factory TransportFactory) {
	t.Helper()
	clientConn, serverConn, cleanup := dial(t, factory)
	defer cleanup()

	if clientConn == nil {
		t.Fatal("client connection is nil")
	}
	if serverConn == nil {
		t.Fatal("server connection is nil")
	}
	if clientConn.LocalAddr() == nil {
		t.Error("client LocalAddr is nil")
	}
	if clientConn.RemoteAddr() == nil {
		t.Error("client RemoteAddr is nil")
	}
	if serverConn.LocalAddr() == nil {
		t.Error("server LocalAddr is nil")
	}
	if serverConn.RemoteAddr() == nil {
		t.Error("server RemoteAddr is nil")
	}
}

func testStreamRoundTrip(t *testing.T, factory TransportFactory) {
	t.Helper()
	clientConn, serverConn, cleanup := dial(t, factory)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Client opens stream, server accepts.
	type streamResult struct {
		s   transport.Stream
		err error
	}
	acceptCh := make(chan streamResult, 1)
	go func() {
		s, err := serverConn.AcceptStream(ctx)
		acceptCh <- streamResult{s, err}
	}()

	cs, err := clientConn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("AcceptStream: %v", sr.err)
	}
	ss := sr.s

	// Client -> Server
	want := []byte("hello from client")
	if _, err := cs.Write(want); err != nil {
		t.Fatalf("client write: %v", err)
	}
	got := make([]byte, len(want)+10)
	n, err := ss.Read(got)
	if err != nil {
		t.Fatalf("server read: %v", err)
	}
	if !bytes.Equal(got[:n], want) {
		t.Fatalf("server got %q, want %q", got[:n], want)
	}

	// Server -> Client
	want2 := []byte("hello from server")
	if _, err := ss.Write(want2); err != nil {
		t.Fatalf("server write: %v", err)
	}
	got2 := make([]byte, len(want2)+10)
	n2, err := cs.Read(got2)
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	if !bytes.Equal(got2[:n2], want2) {
		t.Fatalf("client got %q, want %q", got2[:n2], want2)
	}

	cs.Close()
	ss.Close()
}

func testMultiplexStreams(t *testing.T, factory TransportFactory) {
	t.Helper()
	clientConn, serverConn, cleanup := dial(t, factory)
	defer cleanup()

	const numStreams = 10
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Server: accept streams and echo back with a prefix.
	var serverWg sync.WaitGroup
	serverWg.Add(numStreams)
	go func() {
		for i := 0; i < numStreams; i++ {
			ss, err := serverConn.AcceptStream(ctx)
			if err != nil {
				t.Errorf("AcceptStream %d: %v", i, err)
				serverWg.Done()
				continue
			}
			go func(s transport.Stream) {
				defer serverWg.Done()
				defer s.Close()
				buf := make([]byte, 4096)
				n, err := s.Read(buf)
				if err != nil {
					t.Errorf("server read: %v", err)
					return
				}
				if _, err := s.Write(buf[:n]); err != nil {
					t.Errorf("server write: %v", err)
				}
			}(ss)
		}
	}()

	// Client: open streams concurrently, each sends unique data.
	var clientWg sync.WaitGroup
	for i := 0; i < numStreams; i++ {
		clientWg.Add(1)
		go func(id int) {
			defer clientWg.Done()
			cs, err := clientConn.OpenStream(ctx)
			if err != nil {
				t.Errorf("OpenStream %d: %v", id, err)
				return
			}
			defer cs.Close()

			msg := []byte(fmt.Sprintf("stream-%d-payload", id))
			if _, err := cs.Write(msg); err != nil {
				t.Errorf("client write %d: %v", id, err)
				return
			}
			buf := make([]byte, 4096)
			n, err := cs.Read(buf)
			if err != nil {
				t.Errorf("client read %d: %v", id, err)
				return
			}
			if !bytes.Equal(buf[:n], msg) {
				t.Errorf("stream %d: got %q, want %q", id, buf[:n], msg)
			}
		}(i)
	}

	clientWg.Wait()
	serverWg.Wait()
}

func testHalfClose(t *testing.T, factory TransportFactory) {
	t.Helper()
	clientConn, serverConn, cleanup := dial(t, factory)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	acceptCh := make(chan transport.Stream, 1)
	go func() {
		s, err := serverConn.AcceptStream(ctx)
		if err != nil {
			t.Errorf("AcceptStream: %v", err)
			return
		}
		acceptCh <- s
	}()

	cs, err := clientConn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}

	ss := <-acceptCh

	// Write data then close the writer side.
	payload := []byte("half-close-data")
	if _, err := cs.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	cs.Close()

	// Server should still be able to read the data, then get EOF.
	var received bytes.Buffer
	if _, err := io.Copy(&received, ss); err != nil {
		t.Fatalf("server read after half-close: %v", err)
	}
	if !bytes.Equal(received.Bytes(), payload) {
		t.Fatalf("got %q, want %q", received.Bytes(), payload)
	}

	ss.Close()
}

func testGracefulClose(t *testing.T, factory TransportFactory) {
	t.Helper()
	clientConn, serverConn, cleanup := dial(t, factory)
	defer cleanup()

	// Close both connections; should not panic or return unexpected errors.
	if err := clientConn.Close(); err != nil {
		t.Errorf("client close: %v", err)
	}
	if err := serverConn.Close(); err != nil {
		t.Errorf("server close: %v", err)
	}
}

func testConcurrentStreamOps(t *testing.T, factory TransportFactory) {
	t.Helper()
	clientConn, serverConn, cleanup := dial(t, factory)
	defer cleanup()

	const numGoroutines = 10
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Server: accept and echo.
	go func() {
		for {
			ss, err := serverConn.AcceptStream(ctx)
			if err != nil {
				return
			}
			go func(s transport.Stream) {
				defer s.Close()
				io.Copy(s, s) //nolint:errcheck
			}(ss)
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cs, err := clientConn.OpenStream(ctx)
			if err != nil {
				// Connection may be closed by another goroutine; that is acceptable.
				return
			}
			defer cs.Close()

			msg := []byte(fmt.Sprintf("concurrent-%d", id))
			cs.Write(msg) //nolint:errcheck
			buf := make([]byte, len(msg)+10)
			cs.Read(buf) //nolint:errcheck
		}(i)
	}
	wg.Wait()
}

func testCancelledContext(t *testing.T, factory TransportFactory) {
	t.Helper()
	client, _, _, cleanup := factory(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	start := time.Now()
	_, err := client.Dial(ctx, "127.0.0.1:1") // address doesn't matter
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from Dial with cancelled context")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Dial with cancelled context took %v, expected prompt return", elapsed)
	}
}

func testTypeNonEmpty(t *testing.T, factory TransportFactory) {
	t.Helper()
	client, server, _, cleanup := factory(t)
	defer cleanup()

	if client.Type() == "" {
		t.Error("client.Type() returned empty string")
	}
	if server.Type() == "" {
		t.Error("server.Type() returned empty string")
	}
	if client.Type() != server.Type() {
		t.Errorf("client.Type()=%q != server.Type()=%q", client.Type(), server.Type())
	}
}

func testStreamID(t *testing.T, factory TransportFactory) {
	t.Helper()
	clientConn, serverConn, cleanup := dial(t, factory)
	defer cleanup()

	const numStreams = 5
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Collect server-side stream IDs.
	serverIDs := make(chan uint64, numStreams)
	go func() {
		for i := 0; i < numStreams; i++ {
			ss, err := serverConn.AcceptStream(ctx)
			if err != nil {
				return
			}
			serverIDs <- ss.StreamID()
			// Read one byte so the stream doesn't block the muxer.
			buf := make([]byte, 1)
			ss.Read(buf) //nolint:errcheck
			ss.Close()
		}
		close(serverIDs)
	}()

	clientIDs := make(map[uint64]bool)
	for i := 0; i < numStreams; i++ {
		cs, err := clientConn.OpenStream(ctx)
		if err != nil {
			t.Fatalf("OpenStream %d: %v", i, err)
		}
		id := cs.StreamID()
		if clientIDs[id] {
			t.Errorf("duplicate client stream ID: %d", id)
		}
		clientIDs[id] = true
		cs.Write([]byte{0}) //nolint:errcheck
		cs.Close()
	}

	seen := make(map[uint64]bool)
	for id := range serverIDs {
		if seen[id] {
			t.Errorf("duplicate server stream ID: %d", id)
		}
		seen[id] = true
	}
}

func testLargePayload(t *testing.T, factory TransportFactory) {
	t.Helper()
	clientConn, serverConn, cleanup := dial(t, factory)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	acceptCh := make(chan transport.Stream, 1)
	go func() {
		s, err := serverConn.AcceptStream(ctx)
		if err != nil {
			t.Errorf("AcceptStream: %v", err)
			return
		}
		acceptCh <- s
	}()

	cs, err := clientConn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}

	ss := <-acceptCh

	// Generate 1MB of random data.
	const size = 1 << 20
	payload := make([]byte, size)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	// Write in a goroutine since it may block until the reader drains.
	errCh := make(chan error, 1)
	go func() {
		_, err := cs.Write(payload)
		cs.Close()
		errCh <- err
	}()

	var received bytes.Buffer
	if _, err := io.Copy(&received, ss); err != nil {
		t.Fatalf("server read: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("client write: %v", err)
	}

	if received.Len() != size {
		t.Fatalf("received %d bytes, want %d", received.Len(), size)
	}
	if !bytes.Equal(received.Bytes(), payload) {
		t.Fatal("large payload data mismatch")
	}

	ss.Close()
}
