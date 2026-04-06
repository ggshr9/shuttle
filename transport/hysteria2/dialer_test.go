package hysteria2_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/adapter"
	shuttlecrypto "github.com/shuttleX/shuttle/crypto"
	"github.com/shuttleX/shuttle/transport/hysteria2"
	"github.com/shuttleX/shuttle/transport/shared"
)

// ---------- Protocol codec tests ----------

func TestEncodeDecodeStreamHeader(t *testing.T) {
	tests := []struct {
		reqID   uint32
		address string
	}{
		{1, "127.0.0.1:8080"},
		{42, "example.com:443"},
		{0xFFFFFFFF, "[::1]:80"},
		{0, "a:1"},
	}

	for _, tc := range tests {
		var buf bytes.Buffer
		if err := hysteria2.EncodeStreamHeader(&buf, tc.reqID, tc.address); err != nil {
			t.Fatalf("encode(%d, %q): %v", tc.reqID, tc.address, err)
		}

		gotID, gotAddr, err := hysteria2.DecodeStreamHeader(&buf)
		if err != nil {
			t.Fatalf("decode(%d, %q): %v", tc.reqID, tc.address, err)
		}
		if gotID != tc.reqID {
			t.Errorf("requestID: got %d, want %d", gotID, tc.reqID)
		}
		if gotAddr != tc.address {
			t.Errorf("address: got %q, want %q", gotAddr, tc.address)
		}
	}
}

func TestEncodeDecodeAuth(t *testing.T) {
	passwords := []string{"", "secret", "a-very-long-password-with-unicode-\u00e9\u00e8\u00ea"}

	for _, pw := range passwords {
		var buf bytes.Buffer
		if err := hysteria2.EncodeAuth(&buf, pw); err != nil {
			t.Fatalf("encode auth %q: %v", pw, err)
		}

		got, err := hysteria2.DecodeAuth(&buf)
		if err != nil {
			t.Fatalf("decode auth %q: %v", pw, err)
		}
		if got != pw {
			t.Errorf("password: got %q, want %q", got, pw)
		}
	}
}

func TestAuthResult_RoundTrip(t *testing.T) {
	for _, status := range []byte{hysteria2.AuthOK, hysteria2.AuthFail} {
		var buf bytes.Buffer
		if err := hysteria2.WriteAuthResult(&buf, status); err != nil {
			t.Fatal(err)
		}
		got, err := hysteria2.ReadAuthResult(&buf)
		if err != nil {
			t.Fatal(err)
		}
		if got != status {
			t.Errorf("status: got %d, want %d", got, status)
		}
	}
}

func TestDecodeStreamHeader_Truncated(t *testing.T) {
	var buf bytes.Buffer
	if err := hysteria2.EncodeStreamHeader(&buf, 1, "x:1"); err != nil {
		t.Fatal(err)
	}
	truncated := buf.Bytes()[:4] // Only the request ID
	_, _, err := hysteria2.DecodeStreamHeader(bytes.NewReader(truncated))
	if err == nil {
		t.Fatal("expected error on truncated header")
	}
}

func TestNewDialer_Validation(t *testing.T) {
	_, err := hysteria2.NewDialer(hysteria2.DialerConfig{})
	if err == nil {
		t.Fatal("expected error for empty config")
	}

	_, err = hysteria2.NewDialer(hysteria2.DialerConfig{Server: "x:1"})
	if err == nil {
		t.Fatal("expected error for missing password")
	}
}

// ---------- Integration test: echo through QUIC server ----------

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

func TestHysteria2_EchoThroughServer(t *testing.T) {
	// 1. Start TCP echo backend.
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoLn.Close()
	go echoServer(t, echoLn)

	echoAddr := echoLn.Addr().String()

	// 2. Start Hysteria2 QUIC server.
	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()

	serverAddr := udpConn.LocalAddr().String()
	password := "test-hy2-password"
	tlsCfg := generateTestTLSConfig(t)

	srv := hysteria2.NewServerWithTLS(password, tlsCfg, nil)

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

	// 3. Create Hysteria2 client dialer.
	dialer, err := hysteria2.NewDialer(hysteria2.DialerConfig{
		Server:   serverAddr,
		Password: password,
		TLS:      testTLSOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer dialer.Close()

	// 4. Dial through Hysteria2 to the echo backend.
	conn, err := dialer.DialContext(ctx, "tcp", echoAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// 5. Echo test.
	msg := "hello hysteria2"
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

func TestHysteria2_BadPassword(t *testing.T) {
	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()

	serverAddr := udpConn.LocalAddr().String()
	tlsCfg := generateTestTLSConfig(t)
	srv := hysteria2.NewServerWithTLS("correct-password", tlsCfg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go srv.ServeUDP(ctx, udpConn, func(_ context.Context, conn net.Conn, _ adapter.ConnMetadata) {
		conn.Close()
	})

	time.Sleep(50 * time.Millisecond)

	dialer, err := hysteria2.NewDialer(hysteria2.DialerConfig{
		Server:   serverAddr,
		Password: "wrong-password",
		TLS:      testTLSOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer dialer.Close()

	_, err = dialer.DialContext(ctx, "tcp", "127.0.0.1:9999")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestHysteria2_MultiplexedStreams(t *testing.T) {
	// Verify multiple streams through a single QUIC connection.
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
	password := "mux-test"
	tlsCfg := generateTestTLSConfig(t)
	srv := hysteria2.NewServerWithTLS(password, tlsCfg, nil)

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

	dialer, err := hysteria2.NewDialer(hysteria2.DialerConfig{
		Server:   serverAddr,
		Password: password,
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
