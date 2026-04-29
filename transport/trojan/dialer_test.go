package trojan_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/transport/shared"
	"github.com/ggshr9/shuttle/transport/trojan"
)

// echoServer accepts connections and echoes all data back.
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

func TestTrojan_EchoThroughServer(t *testing.T) {
	// 1. Start echo backend
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoLn.Close()
	go echoServer(t, echoLn)

	echoAddr := echoLn.Addr().String()

	// 2. Start Trojan server (no TLS)
	password := "test-password-123"
	hash := trojan.HashPassword(password)

	serverLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer serverLn.Close()

	srv := trojan.NewServer(trojan.ServerConfig{
		Passwords: map[string]string{hash: "testuser"},
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.Serve(ctx, serverLn, func(_ context.Context, cmd byte, address string, conn net.Conn) {
			defer conn.Close()

			if cmd != shared.CmdConnect {
				t.Errorf("expected CmdConnect, got 0x%02x", cmd)
				return
			}

			// Dial the echo backend and relay
			backend, err := net.Dial("tcp", address)
			if err != nil {
				t.Errorf("handler dial: %v", err)
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

	// 3. Create Trojan client dialer (no TLS)
	dialer, err := trojan.NewDialer(&trojan.DialerConfig{
		Server:   serverLn.Addr().String(),
		Password: password,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 4. Dial through Trojan to the echo backend
	conn, err := dialer.DialContext(ctx, "tcp", echoAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// 5. Echo test
	msg := "hello trojan"
	if _, err := fmt.Fprint(conn, msg); err != nil {
		t.Fatal(err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}

	if got := string(buf); got != msg {
		t.Fatalf("echo mismatch: got %q, want %q", got, msg)
	}
}

func TestTrojan_BadPasswordFallback(t *testing.T) {
	// 1. Start fallback HTTP server returning "fallback"
	fallbackLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer fallbackLn.Close()

	fallbackServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "fallback")
		}),
	}
	go fallbackServer.Serve(fallbackLn) //nolint:errcheck
	defer fallbackServer.Close()

	// 2. Start Trojan server with fallback configured
	password := "real-password"
	hash := trojan.HashPassword(password)

	serverLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer serverLn.Close()

	srv := trojan.NewServer(trojan.ServerConfig{
		Passwords: map[string]string{hash: "testuser"},
		Fallback:  fallbackLn.Addr().String(),
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.Serve(ctx, serverLn, func(_ context.Context, _ byte, _ string, conn net.Conn) {
			defer conn.Close()
			// Should not be reached for bad password
			t.Error("handler should not be called for bad password")
		})
	}()

	// 3. Connect with a raw HTTP request (not Trojan protocol)
	conn, err := net.Dial("tcp", serverLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send an HTTP GET — this is not a valid Trojan header, so server should fallback
	httpReq := "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
	if _, err := fmt.Fprint(conn, httpReq); err != nil {
		t.Fatal(err)
	}

	// 4. Read response — should come from fallback HTTP server.
	// Use a generous deadline: the server needs ~1s to detect a non-Trojan client.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	body := string(bodyBytes)
	// The response should contain "fallback" from our HTTP handler
	if !contains(body, "fallback") {
		t.Fatalf("expected fallback content in response, got: %q", body)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
