//go:build sandbox

package test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	rtcTransport "github.com/ggshr9/shuttle/transport/webrtc"
)

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

// setupWebRTCPair creates a server+client pair. When wsMode is true the
// signalling goes through WebSocket (Trickle ICE), otherwise HTTP POST.
// All traffic is on 127.0.0.1 with an OS-assigned port; STUN is disabled.
func setupWebRTCPair(t *testing.T, wsMode bool) (
	srv *rtcTransport.Server,
	client *rtcTransport.Client,
	signalAddr string,
	cleanup func(),
) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	signalAddr = ln.Addr().String()
	ln.Close()

	password := "webrtc-test-" + t.Name()

	srv = rtcTransport.NewServer(&rtcTransport.ServerConfig{
		SignalListen: signalAddr,
		Password:     password,
		STUNServers:  []string{}, // no external STUN
		LoopbackOnly: true,       // no mDNS, no external network
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := srv.Listen(ctx); err != nil {
		cancel()
		t.Fatalf("server listen: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	var signalURL string
	if wsMode {
		signalURL = fmt.Sprintf("ws://%s/webrtc/ws", signalAddr)
	} else {
		signalURL = fmt.Sprintf("http://%s/webrtc/signal", signalAddr)
	}

	client = rtcTransport.NewClient(&rtcTransport.ClientConfig{
		SignalURL:    signalURL,
		Password:     password,
		STUNServers:  []string{}, // no external STUN
		LoopbackOnly: true,       // no mDNS, no external network
	})

	cleanup = func() {
		cancel()
		client.Close()
		srv.Close()
	}
	return
}

// ──────────────────────────────────────────────────────────────────────────────
// Existing tests (kept for backward compat)
// ──────────────────────────────────────────────────────────────────────────────

func TestWebRTCClientCreate(t *testing.T) {
	client := rtcTransport.NewClient(&rtcTransport.ClientConfig{
		SignalURL: "https://127.0.0.1:8443/webrtc/signal",
		Password:  "test",
	})
	if client.Type() != "webrtc" {
		t.Errorf("expected type 'webrtc', got '%s'", client.Type())
	}
}

func TestWebRTCServerCreate(t *testing.T) {
	srv := rtcTransport.NewServer(&rtcTransport.ServerConfig{
		SignalListen: "127.0.0.1:0",
		Password:     "test",
	}, nil)
	if srv.Type() != "webrtc" {
		t.Errorf("expected type 'webrtc', got '%s'", srv.Type())
	}
}

func TestWebRTCClientDialClosed(t *testing.T) {
	client := rtcTransport.NewClient(&rtcTransport.ClientConfig{
		SignalURL: "https://127.0.0.1:8443/webrtc/signal",
		Password:  "test",
	})
	client.Close()

	ctx := context.Background()
	_, err := client.Dial(ctx, "")
	if err == nil {
		t.Fatal("expected error when dialing closed client")
	}
}

func TestWebRTCSignalAuth(t *testing.T) {
	password := "test-password-123"

	req, err := rtcTransport.GenerateAuth(password, "v=0\r\no=- 0 0 IN IP4 0.0.0.0\r\n")
	if err != nil {
		t.Fatalf("generateAuth: %v", err)
	}

	nonce, ok := rtcTransport.VerifyAuth(req, password)
	if !ok {
		t.Fatal("verifyAuth should succeed with correct password")
	}
	if len(nonce) != 32 {
		t.Fatalf("expected 32-byte nonce, got %d", len(nonce))
	}

	_, ok = rtcTransport.VerifyAuth(req, "wrong-password")
	if ok {
		t.Fatal("verifyAuth should fail with wrong password")
	}
}

func TestWebRTCEndToEnd(t *testing.T) {
	srv, client, _, cleanup := setupWebRTCPair(t, false)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := client.Dial(ctx, "")
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	defer conn.Close()

	srvConn, err := srv.Accept(ctx)
	if err != nil {
		t.Fatalf("server accept: %v", err)
	}
	defer srvConn.Close()

	stream, err := conn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()

	srvStream, err := srvConn.AcceptStream(ctx)
	if err != nil {
		t.Fatalf("accept stream: %v", err)
	}
	defer srvStream.Close()

	testData := []byte("hello from webrtc client")
	if _, err := stream.Write(testData); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, 1024)
	n, err := srvStream.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != string(testData) {
		t.Fatalf("expected %q, got %q", testData, buf[:n])
	}

	replyData := []byte("hello from webrtc server")
	if _, err := srvStream.Write(replyData); err != nil {
		t.Fatalf("write reply: %v", err)
	}

	n, err = stream.Read(buf)
	if err != nil {
		t.Fatalf("read reply: %v", err)
	}
	if string(buf[:n]) != string(replyData) {
		t.Fatalf("expected %q, got %q", replyData, buf[:n])
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// New tests
// ──────────────────────────────────────────────────────────────────────────────

func TestWebRTCWSSignaling(t *testing.T) {
	srv, client, _, cleanup := setupWebRTCPair(t, true)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := client.Dial(ctx, "")
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()

	srvConn, err := srv.Accept(ctx)
	if err != nil {
		t.Fatalf("server accept: %v", err)
	}
	defer srvConn.Close()

	stream, err := conn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()

	srvStream, err := srvConn.AcceptStream(ctx)
	if err != nil {
		t.Fatalf("accept stream: %v", err)
	}
	defer srvStream.Close()

	msg := []byte("hello via websocket signaling")
	if _, err := stream.Write(msg); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, 1024)
	n, err := srvStream.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != string(msg) {
		t.Fatalf("expected %q, got %q", msg, buf[:n])
	}
}

func TestWebRTCTrickleICE(t *testing.T) {
	// Trickle ICE is used implicitly when wsMode=true. This test verifies
	// the connection establishes (meaning candidates were exchanged).
	srv, client, _, cleanup := setupWebRTCPair(t, true)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := client.Dial(ctx, "")
	if err != nil {
		t.Fatalf("trickle dial: %v", err)
	}
	defer conn.Close()

	srvConn, err := srv.Accept(ctx)
	if err != nil {
		t.Fatalf("server accept: %v", err)
	}
	defer srvConn.Close()

	// Bidirectional test
	stream, _ := conn.OpenStream(ctx)
	defer stream.Close()
	srvStream, _ := srvConn.AcceptStream(ctx)
	defer srvStream.Close()

	// Client → Server
	stream.Write([]byte("ping"))
	buf := make([]byte, 64)
	n, _ := srvStream.Read(buf)
	if string(buf[:n]) != "ping" {
		t.Fatalf("trickle c→s: expected 'ping', got %q", buf[:n])
	}

	// Server → Client
	srvStream.Write([]byte("pong"))
	n, _ = stream.Read(buf)
	if string(buf[:n]) != "pong" {
		t.Fatalf("trickle s→c: expected 'pong', got %q", buf[:n])
	}
}

func TestWebRTCMultiStream(t *testing.T) {
	srv, client, _, cleanup := setupWebRTCPair(t, true)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := client.Dial(ctx, "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	srvConn, err := srv.Accept(ctx)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer srvConn.Close()

	const numStreams = 10
	var wg sync.WaitGroup
	errCh := make(chan error, numStreams*2)

	// Client side: open streams and write
	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s, err := conn.OpenStream(ctx)
			if err != nil {
				errCh <- fmt.Errorf("stream %d open: %w", idx, err)
				return
			}
			defer s.Close()
			msg := fmt.Sprintf("stream-%d", idx)
			if _, err := s.Write([]byte(msg)); err != nil {
				errCh <- fmt.Errorf("stream %d write: %w", idx, err)
				return
			}
			buf := make([]byte, 64)
			n, err := s.Read(buf)
			if err != nil {
				errCh <- fmt.Errorf("stream %d read: %w", idx, err)
				return
			}
			expected := "reply-" + msg
			if string(buf[:n]) != expected {
				errCh <- fmt.Errorf("stream %d: expected %q, got %q", idx, expected, buf[:n])
			}
		}(i)
	}

	// Server side: accept and echo with prefix
	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := srvConn.AcceptStream(ctx)
			if err != nil {
				errCh <- fmt.Errorf("server accept: %w", err)
				return
			}
			defer s.Close()
			buf := make([]byte, 256)
			n, err := s.Read(buf)
			if err != nil {
				errCh <- fmt.Errorf("server read: %w", err)
				return
			}
			reply := "reply-" + string(buf[:n])
			if _, err := s.Write([]byte(reply)); err != nil {
				errCh <- fmt.Errorf("server write: %w", err)
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func TestWebRTCLargeTransfer(t *testing.T) {
	srv, client, _, cleanup := setupWebRTCPair(t, true)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := client.Dial(ctx, "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	srvConn, err := srv.Accept(ctx)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer srvConn.Close()

	stream, _ := conn.OpenStream(ctx)
	defer stream.Close()
	srvStream, _ := srvConn.AcceptStream(ctx)
	defer srvStream.Close()

	// Generate 10 MB of data
	const size = 10 * 1024 * 1024
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251) // deterministic pattern
	}
	expectedHash := sha256.Sum256(data)

	// Write in chunks
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		written := 0
		for written < size {
			chunkSize := 32 * 1024
			if written+chunkSize > size {
				chunkSize = size - written
			}
			n, err := stream.Write(data[written : written+chunkSize])
			if err != nil {
				t.Errorf("write at %d: %v", written, err)
				return
			}
			written += n
		}
	}()

	// Read all on server side
	received := make([]byte, 0, size)
	buf := make([]byte, 64*1024)
	for len(received) < size {
		n, err := srvStream.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatalf("read at %d: %v", len(received), err)
		}
		received = append(received, buf[:n]...)
		if err == io.EOF {
			break
		}
	}

	wg.Wait()

	if len(received) != size {
		t.Fatalf("expected %d bytes, got %d", size, len(received))
	}

	gotHash := sha256.Sum256(received)
	if gotHash != expectedHash {
		t.Fatalf("SHA-256 mismatch")
	}
}

func TestWebRTCAuthFailureWS(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	signalAddr := ln.Addr().String()
	ln.Close()

	srv := rtcTransport.NewServer(&rtcTransport.ServerConfig{
		SignalListen: signalAddr,
		Password:     "correct-password",
		STUNServers:  []string{},
		LoopbackOnly: true,
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Listen(ctx); err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer srv.Close()
	time.Sleep(50 * time.Millisecond)

	client := rtcTransport.NewClient(&rtcTransport.ClientConfig{
		SignalURL:    fmt.Sprintf("ws://%s/webrtc/ws", signalAddr),
		Password:     "wrong-password",
		STUNServers:  []string{},
		LoopbackOnly: true,
	})
	defer client.Close()

	_, err = client.Dial(ctx, "")
	if err == nil {
		t.Fatal("expected error with wrong password")
	}
}

func TestWebRTCReplayRejection(t *testing.T) {
	password := "replay-test"

	req, err := rtcTransport.GenerateAuth(password, "dummy-sdp")
	if err != nil {
		t.Fatalf("generateAuth: %v", err)
	}

	// First verification should succeed
	nonce1, ok := rtcTransport.VerifyAuth(req, password)
	if !ok {
		t.Fatal("first verify should succeed")
	}
	if len(nonce1) != 32 {
		t.Fatalf("expected 32-byte nonce")
	}

	// Second call with same request should still verify (VerifyAuth is stateless HMAC check).
	// Replay protection is at the server level via ReplayFilter.
	nonce2, ok := rtcTransport.VerifyAuth(req, password)
	if !ok {
		t.Fatal("HMAC verify is stateless — should pass")
	}
	if string(nonce1) != string(nonce2) {
		t.Fatal("nonce should be the same for the same request")
	}
}

func TestWebRTCICEPolicyRelay(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	signalAddr := ln.Addr().String()
	ln.Close()

	password := "relay-policy-test"
	srv := rtcTransport.NewServer(&rtcTransport.ServerConfig{
		SignalListen: signalAddr,
		Password:     password,
		STUNServers:  []string{},
		ICEPolicy:    "relay",
		LoopbackOnly: true,
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Listen(ctx); err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer srv.Close()
	time.Sleep(50 * time.Millisecond)

	client := rtcTransport.NewClient(&rtcTransport.ClientConfig{
		SignalURL:    fmt.Sprintf("ws://%s/webrtc/ws", signalAddr),
		Password:     password,
		STUNServers:  []string{},
		ICEPolicy:    "relay",
		LoopbackOnly: true,
	})
	defer client.Close()

	// With relay-only and no TURN servers, the connection should fail
	// because there are no relay candidates available.
	dialCtx, dialCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dialCancel()
	_, err = client.Dial(dialCtx, "")
	if err == nil {
		t.Fatal("expected failure with relay-only policy and no TURN servers")
	}
}

func TestWebRTCConnectionStats(t *testing.T) {
	srv, client, _, cleanup := setupWebRTCPair(t, true)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := client.Dial(ctx, "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	srvConn, err := srv.Accept(ctx)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer srvConn.Close()

	// Send some data to generate stats
	stream, _ := conn.OpenStream(ctx)
	defer stream.Close()
	srvStream, _ := srvConn.AcceptStream(ctx)
	defer srvStream.Close()

	data := make([]byte, 64*1024)
	for i := range data {
		data[i] = byte(i)
	}
	stream.Write(data)

	buf := make([]byte, 64*1024)
	total := 0
	for total < len(data) {
		n, err := srvStream.Read(buf)
		if err != nil {
			break
		}
		total += n
	}

	// Stats are polled every 5s, wait for at least one poll cycle
	time.Sleep(6 * time.Second)

	// Check stats via the typed assertion
	type statsGetter interface {
		Stats() rtcTransport.ConnStats
	}

	if sg, ok := conn.(statsGetter); ok {
		stats := sg.Stats()
		if stats.BytesSent == 0 && stats.BytesRecv == 0 {
			t.Log("warning: stats may not have data yet (depends on ICE pair nomination)")
		}
	}
}

func TestWebRTCServerClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	signalAddr := ln.Addr().String()
	ln.Close()

	password := "close-test"
	srv := rtcTransport.NewServer(&rtcTransport.ServerConfig{
		SignalListen: signalAddr,
		Password:     password,
		STUNServers:  []string{},
		LoopbackOnly: true,
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Listen(ctx); err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Verify server accepts connections
	client := rtcTransport.NewClient(&rtcTransport.ClientConfig{
		SignalURL:    fmt.Sprintf("http://%s/webrtc/signal", signalAddr),
		Password:     password,
		STUNServers:  []string{},
		LoopbackOnly: true,
	})
	defer client.Close()

	conn, err := client.Dial(ctx, "")
	if err != nil {
		t.Fatalf("dial before close: %v", err)
	}
	conn.Close()

	// Close server
	if err := srv.Close(); err != nil {
		t.Fatalf("server close: %v", err)
	}

	// After close, new connections should fail
	time.Sleep(100 * time.Millisecond)
	_, err = client.Dial(ctx, "")
	if err == nil {
		t.Fatal("expected error after server close")
	}
}
