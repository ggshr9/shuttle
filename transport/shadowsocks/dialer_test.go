package shadowsocks_test

import (
	"context"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/transport/shadowsocks"
)

func TestMain(m *testing.M) {
	// Disable the go-shadowsocks2 salt replay filter: when client and server
	// share the same process the filter incorrectly rejects the client's salt.
	os.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")
	os.Exit(m.Run())
}

// TestDialer_EchoThroughServer starts a plain TCP echo server, a Shadowsocks
// server that relays to it, then uses a Shadowsocks Dialer to send data
// through the chain and verify the echo.
func TestDialer_EchoThroughServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// ---------- 1. Start TCP echo server ----------
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoLn.Close()

	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(c)
		}
	}()

	echoAddr := echoLn.Addr().String()

	// ---------- 2. Start SS server ----------
	const method = "aes-256-gcm"
	const password = "test-password-1234"

	ssServer, err := shadowsocks.NewServer(shadowsocks.ServerConfig{
		Method:   method,
		Password: password,
	})
	if err != nil {
		t.Fatal(err)
	}

	ssLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ssLn.Close()

	// Handler: relay decrypted conn to the echo server.
	handler := func(_ context.Context, conn net.Conn, meta shadowsocks.ConnMeta) {
		upstream, err := net.Dial("tcp", meta.Destination)
		if err != nil {
			conn.Close()
			return
		}

		// Bidirectional relay; close both when either direction ends.
		go func() {
			io.Copy(conn, upstream)
			conn.Close()
		}()
		go func() {
			io.Copy(upstream, conn)
			upstream.Close()
		}()
	}

	go ssServer.Serve(ctx, ssLn, handler)

	ssAddr := ssLn.Addr().String()

	// ---------- 3. Dial through SS to echo ----------
	dialer, err := shadowsocks.NewDialer(shadowsocks.DialerConfig{
		Server:   ssAddr,
		Method:   method,
		Password: password,
	})
	if err != nil {
		t.Fatal(err)
	}

	conn, err := dialer.DialContext(ctx, "tcp", echoAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// ---------- 4. Echo test ----------
	msg := "hello shadowsocks"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}

	if string(buf) != msg {
		t.Fatalf("echo mismatch: got %q, want %q", string(buf), msg)
	}
}

// TestDialer_Chacha20 verifies chacha20-ietf-poly1305 works end-to-end.
func TestDialer_Chacha20(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Echo server
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoLn.Close()

	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(c)
		}
	}()

	const method = "chacha20-ietf-poly1305"
	const password = "chacha-test-pw"

	ssServer, err := shadowsocks.NewServer(shadowsocks.ServerConfig{
		Method: method, Password: password,
	})
	if err != nil {
		t.Fatal(err)
	}

	ssLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ssLn.Close()

	handler := func(_ context.Context, conn net.Conn, meta shadowsocks.ConnMeta) {
		upstream, err := net.Dial("tcp", meta.Destination)
		if err != nil {
			conn.Close()
			return
		}
		go func() {
			io.Copy(conn, upstream)
			conn.Close()
		}()
		go func() {
			io.Copy(upstream, conn)
			upstream.Close()
		}()
	}

	go ssServer.Serve(ctx, ssLn, handler)

	dialer, err := shadowsocks.NewDialer(shadowsocks.DialerConfig{
		Server:   ssLn.Addr().String(),
		Method:   method,
		Password: password,
	})
	if err != nil {
		t.Fatal(err)
	}

	conn, err := dialer.DialContext(ctx, "tcp", echoLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	msg := "hello chacha20"
	conn.Write([]byte(msg))
	buf := make([]byte, len(msg))
	io.ReadFull(conn, buf)

	if string(buf) != msg {
		t.Fatalf("echo mismatch: got %q, want %q", string(buf), msg)
	}
}
