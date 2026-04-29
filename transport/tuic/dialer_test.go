package tuic_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/adapter"
	shuttlecrypto "github.com/ggshr9/shuttle/crypto"
	"github.com/ggshr9/shuttle/transport/shared"
	"github.com/ggshr9/shuttle/transport/tuic"
)

func generateTestTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	certPEM, keyPEM, err := shuttlecrypto.GenerateSelfSignedCert(
		[]string{"127.0.0.1", "localhost"},
		time.Hour,
	)
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("parse key pair: %v", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h3"},
	}
}

func testTLSOptions() shared.TLSOptions {
	return shared.TLSOptions{
		Enabled:            true,
		InsecureSkipVerify: true,
		ALPN:               []string{"h3"},
	}
}

func TestNewDialer_Validation(t *testing.T) {
	_, err := tuic.NewDialer(&tuic.DialerConfig{})
	if err == nil {
		t.Fatal("expected error for empty config")
	}

	_, err = tuic.NewDialer(&tuic.DialerConfig{Server: "x:1"})
	if err == nil {
		t.Fatal("expected error for missing UUID")
	}

	_, err = tuic.NewDialer(&tuic.DialerConfig{Server: "x:1", UUID: "550e8400-e29b-41d4-a716-446655440000"})
	if err == nil {
		t.Fatal("expected error for missing password")
	}

	_, err = tuic.NewDialer(&tuic.DialerConfig{Server: "x:1", UUID: "bad-uuid", Password: "pw"})
	if err == nil {
		t.Fatal("expected error for invalid UUID")
	}
}

// ---------- Integration test: echo through QUIC server ----------

const testUUID = "550e8400-e29b-41d4-a716-446655440000"
const testPassword = "test-tuic-password"

func TestTUIC_EchoThroughServer(t *testing.T) {
	// 1. Start TCP echo backend.
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoLn.Close()
	go echoServer(t, echoLn)

	echoAddr := echoLn.Addr().String()

	// 2. Start TUIC QUIC server.
	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()

	serverAddr := udpConn.LocalAddr().String()
	tlsCfg := generateTestTLSConfig(t)

	srv, err := tuic.NewServerWithTLS(
		map[string]string{testUUID: testPassword},
		tlsCfg, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.ServeUDP(ctx, udpConn, func(_ context.Context, conn net.Conn, meta adapter.ConnMetadata) {
			defer conn.Close()

			if meta.Destination != echoAddr {
				t.Errorf("expected destination %s, got %s", echoAddr, meta.Destination)
				return
			}
			if meta.Protocol != "tuic" {
				t.Errorf("expected protocol tuic, got %s", meta.Protocol)
				return
			}

			// Dial the echo backend and relay.
			backend, dialErr := net.Dial("tcp", meta.Destination)
			if dialErr != nil {
				t.Errorf("handler dial: %v", dialErr)
				return
			}
			defer backend.Close()

			var relayWg sync.WaitGroup
			relayWg.Add(2)
			go func() {
				defer relayWg.Done()
				io.Copy(backend, conn) //nolint:errcheck
			}()
			go func() {
				defer relayWg.Done()
				io.Copy(conn, backend) //nolint:errcheck
			}()
			relayWg.Wait()
		})
	}()

	// Small delay for QUIC listener to start.
	time.Sleep(50 * time.Millisecond)

	// 3. Create TUIC client dialer.
	dialer, err := tuic.NewDialer(&tuic.DialerConfig{
		Server:   serverAddr,
		UUID:     testUUID,
		Password: testPassword,
		TLS:      testTLSOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer dialer.Close()

	// 4. Dial through TUIC to the echo backend.
	conn, err := dialer.DialContext(ctx, "tcp", echoAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// 5. Echo test.
	msg := "hello tuic v5"
	if _, err := fmt.Fprint(conn, msg); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, len(msg))
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}

	if got := string(buf); got != msg {
		t.Fatalf("echo mismatch: got %q, want %q", got, msg)
	}
}

func TestTUIC_BadAuth(t *testing.T) {
	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()

	serverAddr := udpConn.LocalAddr().String()
	tlsCfg := generateTestTLSConfig(t)
	srv, err := tuic.NewServerWithTLS(
		map[string]string{testUUID: "correct-password"},
		tlsCfg, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go srv.ServeUDP(ctx, udpConn, func(_ context.Context, conn net.Conn, _ adapter.ConnMetadata) {
		conn.Close()
	})

	time.Sleep(50 * time.Millisecond)

	// Connect with wrong password — the server should close the connection.
	dialer, err := tuic.NewDialer(&tuic.DialerConfig{
		Server:   serverAddr,
		UUID:     testUUID,
		Password: "wrong-password",
		TLS:      testTLSOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer dialer.Close()

	// The connection will establish (QUIC handshake succeeds) but the server
	// will close it after verifying the auth datagram fails.
	// The dial may succeed, but opening a stream should eventually fail.
	conn, err := dialer.DialContext(ctx, "tcp", "127.0.0.1:9999")
	if err != nil {
		// Expected: auth or stream failure
		return
	}
	// If dial succeeds, a read should fail because server closes the connection.
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatal("expected error reading from auth-rejected connection")
	}
	conn.Close()
}

func TestTUIC_MultiplexedStreams(t *testing.T) {
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoLn.Close()
	go echoServer(t, echoLn)

	echoAddr := echoLn.Addr().String()

	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()

	serverAddr := udpConn.LocalAddr().String()
	tlsCfg := generateTestTLSConfig(t)
	srv, err := tuic.NewServerWithTLS(
		map[string]string{testUUID: testPassword},
		tlsCfg, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go srv.ServeUDP(ctx, udpConn, func(_ context.Context, conn net.Conn, meta adapter.ConnMetadata) {
		defer conn.Close()
		backend, dialErr := net.Dial("tcp", meta.Destination)
		if dialErr != nil {
			return
		}
		defer backend.Close()
		var relayWg sync.WaitGroup
		relayWg.Add(2)
		go func() {
			defer relayWg.Done()
			io.Copy(backend, conn) //nolint:errcheck
		}()
		go func() {
			defer relayWg.Done()
			io.Copy(conn, backend) //nolint:errcheck
		}()
		relayWg.Wait()
	})

	time.Sleep(50 * time.Millisecond)

	dialer, err := tuic.NewDialer(&tuic.DialerConfig{
		Server:   serverAddr,
		UUID:     testUUID,
		Password: testPassword,
		TLS:      testTLSOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer dialer.Close()

	const n = 5
	var mu sync.Mutex
	var errs []error

	var streamWg sync.WaitGroup
	for i := 0; i < n; i++ {
		streamWg.Add(1)
		go func(idx int) {
			defer streamWg.Done()
			conn, dialErr := dialer.DialContext(ctx, "tcp", echoAddr)
			if dialErr != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("stream %d dial: %w", idx, dialErr))
				mu.Unlock()
				return
			}
			defer conn.Close()

			msg := fmt.Sprintf("stream-%d", idx)
			if _, writeErr := fmt.Fprint(conn, msg); writeErr != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("stream %d write: %w", idx, writeErr))
				mu.Unlock()
				return
			}

			buf := make([]byte, len(msg))
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			if _, readErr := io.ReadFull(conn, buf); readErr != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("stream %d read: %w", idx, readErr))
				mu.Unlock()
				return
			}

			if string(buf) != msg {
				mu.Lock()
				errs = append(errs, fmt.Errorf("stream %d: got %q, want %q", idx, buf, msg))
				mu.Unlock()
			}
		}(i)
	}

	streamWg.Wait()

	for _, e := range errs {
		t.Error(e)
	}
}

// ---------- helpers ----------

func echoServer(t *testing.T, ln net.Listener) {
	t.Helper()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			io.Copy(conn, conn) //nolint:errcheck
		}()
	}
}
