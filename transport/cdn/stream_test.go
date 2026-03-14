package cdn

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/shuttle-proxy/shuttle/transport"
)

// getFreePort finds a free TCP port by binding to :0 and releasing it.
func getFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("get free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// startServer creates and starts a CDN server on the given port.
func startServer(t *testing.T, port int, password string) *Server {
	t.Helper()
	cfg := &ServerConfig{
		ListenAddr: fmt.Sprintf(":%d", port),
		Password:   password,
		Path:       "/cdn/stream",
	}
	srv := NewServer(cfg, nil)
	if err := srv.Listen(context.Background()); err != nil {
		t.Fatalf("server listen: %v", err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv
}

// makeClient creates an H2Client configured for the given port and password.
func makeClient(t *testing.T, port int, password string) *H2Client {
	t.Helper()
	cfg := &H2Config{
		ServerAddr:         fmt.Sprintf("127.0.0.1:%d", port),
		CDNDomain:          fmt.Sprintf("127.0.0.1:%d", port),
		Path:               "/cdn/stream",
		Password:           password,
		InsecureSkipVerify: true,
	}
	return NewH2Client(cfg)
}

func TestCDNH2RoundTrip(t *testing.T) {
	port := getFreePort(t)
	srv := startServer(t, port, "test-password")
	client := makeClient(t, port, "test-password")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Accept server connection in background.
	type acceptResult struct {
		conn transport.Connection
		err  error
	}
	acceptCh := make(chan acceptResult, 1)
	go func() {
		conn, err := srv.Accept(ctx)
		acceptCh <- acceptResult{conn, err}
	}()

	// Client dials.
	clientConn, err := client.Dial(ctx, "")
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	defer clientConn.Close()

	// Wait for server to accept.
	ar := <-acceptCh
	if ar.err != nil {
		t.Fatalf("server accept: %v", ar.err)
	}
	serverConn := ar.conn
	defer serverConn.Close()

	// Server accepts stream in background while client opens one.
	type streamResult struct {
		stream transport.Stream
		err    error
	}
	streamCh := make(chan streamResult, 1)
	go func() {
		s, err := serverConn.AcceptStream(ctx)
		streamCh <- streamResult{s, err}
	}()

	clientStream, err := clientConn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer clientStream.Close()

	sr := <-streamCh
	if sr.err != nil {
		t.Fatalf("accept stream: %v", sr.err)
	}
	serverStream := sr.stream
	defer serverStream.Close()

	// Client -> Server: "hello"
	if _, err := clientStream.Write([]byte("hello")); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	buf := make([]byte, 64)
	n, err := serverStream.Read(buf)
	if err != nil {
		t.Fatalf("read hello: %v", err)
	}
	if got := string(buf[:n]); got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}

	// Server -> Client: "world"
	if _, err := serverStream.Write([]byte("world")); err != nil {
		t.Fatalf("write world: %v", err)
	}
	n, err = clientStream.Read(buf)
	if err != nil {
		t.Fatalf("read world: %v", err)
	}
	if got := string(buf[:n]); got != "world" {
		t.Fatalf("got %q, want %q", got, "world")
	}
}

func TestCDNH2MultipleStreams(t *testing.T) {
	port := getFreePort(t)
	srv := startServer(t, port, "test-password")
	client := makeClient(t, port, "test-password")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Accept server connection.
	acceptCh := make(chan transport.Connection, 1)
	go func() {
		conn, err := srv.Accept(ctx)
		if err == nil {
			acceptCh <- conn
		}
	}()

	clientConn, err := client.Dial(ctx, "")
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	defer clientConn.Close()

	serverConn := <-acceptCh
	defer serverConn.Close()

	const numStreams = 5
	errors := make(chan error, numStreams*2)

	// Server side: accept streams and echo data back.
	var serverWg sync.WaitGroup
	serverWg.Add(1)
	go func() {
		defer serverWg.Done()
		for i := 0; i < numStreams; i++ {
			s, err := serverConn.AcceptStream(ctx)
			if err != nil {
				errors <- fmt.Errorf("accept stream %d: %w", i, err)
				return
			}
			serverWg.Add(1)
			go func(stream transport.Stream) {
				defer serverWg.Done()
				defer stream.Close()
				buf := make([]byte, 256)
				n, err := stream.Read(buf)
				if err != nil {
					errors <- fmt.Errorf("server read: %w", err)
					return
				}
				if _, err := stream.Write(buf[:n]); err != nil {
					errors <- fmt.Errorf("server write: %w", err)
				}
			}(s)
		}
	}()

	// Client side: open streams concurrently and verify echo.
	var clientWg sync.WaitGroup
	for i := 0; i < numStreams; i++ {
		clientWg.Add(1)
		go func(idx int) {
			defer clientWg.Done()
			s, err := clientConn.OpenStream(ctx)
			if err != nil {
				errors <- fmt.Errorf("open stream %d: %w", idx, err)
				return
			}
			defer s.Close()

			msg := fmt.Sprintf("stream-%d-data", idx)
			if _, err := s.Write([]byte(msg)); err != nil {
				errors <- fmt.Errorf("client write %d: %w", idx, err)
				return
			}

			buf := make([]byte, 256)
			n, err := s.Read(buf)
			if err != nil {
				errors <- fmt.Errorf("client read %d: %w", idx, err)
				return
			}
			if got := string(buf[:n]); got != msg {
				errors <- fmt.Errorf("stream %d: got %q, want %q", idx, got, msg)
			}
		}(i)
	}

	clientWg.Wait()
	serverWg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

func TestCDNH2LargePayload(t *testing.T) {
	port := getFreePort(t)
	srv := startServer(t, port, "test-password")
	client := makeClient(t, port, "test-password")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	acceptCh := make(chan transport.Connection, 1)
	go func() {
		conn, err := srv.Accept(ctx)
		if err == nil {
			acceptCh <- conn
		}
	}()

	clientConn, err := client.Dial(ctx, "")
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	defer clientConn.Close()

	serverConn := <-acceptCh
	defer serverConn.Close()

	// Generate 1 MB of random data.
	const payloadSize = 1 << 20
	payload := make([]byte, payloadSize)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand read: %v", err)
	}

	streamCh := make(chan transport.Stream, 1)
	go func() {
		s, err := serverConn.AcceptStream(ctx)
		if err == nil {
			streamCh <- s
		}
	}()

	clientStream, err := clientConn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer clientStream.Close()

	serverStream := <-streamCh
	defer serverStream.Close()

	// Write payload from client concurrently while server reads.
	writeErr := make(chan error, 1)
	go func() {
		_, err := clientStream.Write(payload)
		writeErr <- err
	}()

	received := make([]byte, payloadSize)
	if _, err := io.ReadFull(serverStream, received); err != nil {
		t.Fatalf("read full payload: %v", err)
	}

	if err := <-writeErr; err != nil {
		t.Fatalf("write payload: %v", err)
	}

	if !bytes.Equal(payload, received) {
		t.Fatal("payload mismatch")
	}
}

func TestCDNH2AuthFailure(t *testing.T) {
	port := getFreePort(t)
	_ = startServer(t, port, "correct-password")

	client := makeClient(t, port, "wrong-password")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := client.Dial(ctx, "")
	if err != nil {
		// Dial failure is an acceptable outcome for wrong password.
		t.Logf("dial correctly failed with wrong password: %v", err)
		return
	}
	defer conn.Close()

	// If Dial returned a connection (yamux session was created before the
	// server dropped the HTTP/2 stream), opening or using a stream should
	// fail because the server closed the underlying transport.
	s, err := conn.OpenStream(ctx)
	if err != nil {
		t.Logf("open stream correctly failed: %v", err)
		return
	}
	defer s.Close()

	// Try to read -- the server never accepted, so this should fail.
	buf := make([]byte, 64)
	_, err = s.Read(buf)
	if err == nil {
		t.Fatal("expected error when using connection with wrong password")
	}
	t.Logf("read correctly failed: %v", err)
}

func TestCDNGRPCFrameRoundTrip(t *testing.T) {
	// Test gRPC framing directly using pipes.
	cr, sw := io.Pipe() // server writes -> client reads
	sr, cw := io.Pipe() // client writes -> server reads

	clientDuplex := &grpcDuplex{reader: cr, writer: cw}
	serverDuplex := &grpcDuplex{reader: sr, writer: sw}

	defer clientDuplex.Close()
	defer serverDuplex.Close()

	// Client writes data through gRPC framing.
	msg := []byte("hello gRPC framing test")
	writeErr := make(chan error, 1)
	go func() {
		_, err := clientDuplex.Write(msg)
		writeErr <- err
	}()

	// Server reads data through gRPC framing (strips frame header).
	buf := make([]byte, 256)
	n, err := serverDuplex.Read(buf)
	if err != nil {
		t.Fatalf("server read: %v", err)
	}
	if got := string(buf[:n]); got != string(msg) {
		t.Fatalf("got %q, want %q", got, string(msg))
	}
	if err := <-writeErr; err != nil {
		t.Fatalf("client write: %v", err)
	}

	// Server writes back through gRPC framing.
	reply := []byte("reply from server")
	go func() {
		_, err := serverDuplex.Write(reply)
		writeErr <- err
	}()

	n, err = clientDuplex.Read(buf)
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	if got := string(buf[:n]); got != string(reply) {
		t.Fatalf("got %q, want %q", got, string(reply))
	}
	if err := <-writeErr; err != nil {
		t.Fatalf("server write: %v", err)
	}
}

func TestCDNH2ServerClose(t *testing.T) {
	port := getFreePort(t)
	cfg := &ServerConfig{
		ListenAddr: fmt.Sprintf(":%d", port),
		Password:   "test-password",
		Path:       "/cdn/stream",
	}
	srv := NewServer(cfg, nil)
	if err := srv.Listen(context.Background()); err != nil {
		t.Fatalf("server listen: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	acceptCh := make(chan transport.Connection, 1)
	go func() {
		conn, err := srv.Accept(ctx)
		if err == nil {
			acceptCh <- conn
		}
	}()

	client := makeClient(t, port, "test-password")
	defer client.Close()

	clientConn, err := client.Dial(ctx, "")
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	defer clientConn.Close()

	serverConn := <-acceptCh
	defer serverConn.Close()

	// Open a stream so there is an active session.
	streamCh := make(chan transport.Stream, 1)
	go func() {
		s, err := serverConn.AcceptStream(ctx)
		if err == nil {
			streamCh <- s
		}
	}()

	clientStream, err := clientConn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer clientStream.Close()

	serverStream := <-streamCh
	defer serverStream.Close()

	// Verify the stream works before closing the server.
	if _, err := clientStream.Write([]byte("before-close")); err != nil {
		t.Fatalf("write before close: %v", err)
	}
	buf := make([]byte, 64)
	n, err := serverStream.Read(buf)
	if err != nil {
		t.Fatalf("read before close: %v", err)
	}
	if got := string(buf[:n]); got != "before-close" {
		t.Fatalf("got %q, want %q", got, "before-close")
	}

	// Close the server-side yamux session first (simulating server shutdown),
	// then close the server itself. The HTTP/2 server's graceful shutdown
	// would otherwise block waiting for the handler (which blocks on
	// session.CloseChan()) to return.
	serverConn.Close()

	if err := srv.Close(); err != nil {
		t.Fatalf("server close: %v", err)
	}

	// Give the shutdown a moment to propagate through the HTTP/2 layer.
	time.Sleep(200 * time.Millisecond)

	// The client stream should now return an error on read or write.
	_, readErr := clientStream.Read(buf)
	_, writeErr := clientStream.Write([]byte("after-close"))
	if readErr == nil && writeErr == nil {
		t.Fatal("expected error after server close, got nil on both read and write")
	}
}
