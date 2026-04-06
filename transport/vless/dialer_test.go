package vless_test

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/transport/vless"
)

// TestVLESS_EchoThroughServer starts a plain TCP echo server, a VLESS server
// that relays to it, then uses a VLESS Dialer to send data through the chain
// and verify the echo.
func TestVLESS_EchoThroughServer(t *testing.T) {
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

	// ---------- 2. Start VLESS server ----------
	testUUID := [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}

	srv, err := vless.NewServer(vless.ServerConfig{
		Users: map[[16]byte]string{
			testUUID: "test-user",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	vlessLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer vlessLn.Close()

	// Handler: relay decrypted conn to the echo server.
	handler := func(_ context.Context, conn net.Conn, meta vless.ConnMeta) {
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

	go srv.Serve(ctx, vlessLn, handler)

	vlessAddr := vlessLn.Addr().String()

	// ---------- 3. Dial through VLESS to echo ----------
	dialer, err := vless.NewDialer(&vless.DialerConfig{
		Server: vlessAddr,
		UUID:   testUUID,
		// TLS disabled (plain TCP for unit test)
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
	msg := "hello vless"
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

// TestVLESS_BadUUID verifies that a client with the wrong UUID is rejected.
func TestVLESS_BadUUID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	goodUUID := [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	badUUID := [16]byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
		0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0}

	srv, err := vless.NewServer(vless.ServerConfig{
		Users: map[[16]byte]string{
			goodUUID: "good-user",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	vlessLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer vlessLn.Close()

	handler := func(_ context.Context, conn net.Conn, meta vless.ConnMeta) {
		// Should never be called for bad UUID.
		t.Error("handler called for bad UUID")
		conn.Close()
	}

	go srv.Serve(ctx, vlessLn, handler)

	// Dial with bad UUID.
	dialer, err := vless.NewDialer(&vless.DialerConfig{
		Server: vlessLn.Addr().String(),
		UUID:   badUUID,
	})
	if err != nil {
		t.Fatal(err)
	}

	conn, err := dialer.DialContext(ctx, "tcp", "127.0.0.1:9999")
	if err != nil {
		// Connection error during dial is acceptable — server closed the conn
		// before/after we could read the response.
		return
	}
	defer conn.Close()

	// If we somehow got a conn, a write or read should fail because the
	// server closed the connection.
	_, err = conn.Write([]byte("should fail"))
	if err != nil {
		return
	}

	buf := make([]byte, 16)
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatal("expected error reading from connection with bad UUID")
	}
}
