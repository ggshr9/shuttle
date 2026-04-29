package transport

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/adapter"
	"github.com/ggshr9/shuttle/transport/auth"
	yamuxmux "github.com/ggshr9/shuttle/transport/mux/yamux"
	tlswrap "github.com/ggshr9/shuttle/transport/security/tls"
)

func TestByteStreamServer_FullPipeline(t *testing.T) {
	const password = "server-test-secret"

	cert := generateSelfSignedCert(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	mux := yamuxmux.New(nil)
	authenticator := auth.NewHMACAuthenticator(password)

	tlsServer := tlswrap.New(tlswrap.Config{
		ServerCert: &cert,
		MinVersion: tls.VersionTLS13,
	})
	tlsClient := tlswrap.New(tlswrap.Config{
		ServerName:         "localhost",
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	})

	serverCfg := ByteStreamServerConfig{
		Security: []adapter.SecureWrapper{tlsServer},
		Auth:     authenticator,
		Mux:      mux,
	}

	// Server goroutine: accept → ByteStreamServerProcess → accept stream → echo.
	serverErr := make(chan error, 1)
	go func() {
		raw, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}

		muxConn, user, err := ByteStreamServerProcess(context.Background(), raw, serverCfg)
		if err != nil {
			serverErr <- err
			return
		}
		defer muxConn.Close()

		if user == "" {
			serverErr <- nil // HMAC auth doesn't return a user name
		}

		stream, err := muxConn.AcceptStream(context.Background())
		if err != nil {
			serverErr <- err
			return
		}
		defer stream.Close()

		_, err = io.Copy(stream, stream)
		if err != nil {
			serverErr <- nil
			return
		}
		serverErr <- nil
	}()

	// Client side: ByteStreamClient with TLS + HMAC + yamux.
	client := NewByteStreamClient(&ByteStreamConfig{
		Addr:     ln.Addr().String(),
		Dialer:   tcpDialer(),
		Security: []adapter.SecureWrapper{tlsClient},
		Auth:     authenticator,
		Mux:      mux,
		TypeName: "test-server",
	})
	defer client.Close()

	conn, err := client.Dial(context.Background(), "")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	stream, err := conn.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}

	msg := []byte("hello bytestream server")
	if _, err := stream.Write(msg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(stream, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	stream.Close()

	if !bytes.Equal(buf, msg) {
		t.Fatalf("echo mismatch: got %q, want %q", buf, msg)
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server timed out")
	}
}

func TestByteStreamServer_WrongPassword(t *testing.T) {
	const serverPassword = "correct-password"
	const clientPassword = "wrong-password"

	cert := generateSelfSignedCert(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	mux := yamuxmux.New(nil)

	tlsServer := tlswrap.New(tlswrap.Config{
		ServerCert: &cert,
		MinVersion: tls.VersionTLS13,
	})
	tlsClient := tlswrap.New(tlswrap.Config{
		ServerName:         "localhost",
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	})

	serverCfg := ByteStreamServerConfig{
		Security: []adapter.SecureWrapper{tlsServer},
		Auth:     auth.NewHMACAuthenticator(serverPassword),
		Mux:      mux,
	}

	// Server goroutine: expect auth failure.
	serverErr := make(chan error, 1)
	go func() {
		raw, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}

		_, _, err = ByteStreamServerProcess(context.Background(), raw, serverCfg)
		serverErr <- err
	}()

	// Client side with wrong password.
	client := NewByteStreamClient(&ByteStreamConfig{
		Addr:     ln.Addr().String(),
		Dialer:   tcpDialer(),
		Security: []adapter.SecureWrapper{tlsClient},
		Auth:     auth.NewHMACAuthenticator(clientPassword),
		Mux:      mux,
		TypeName: "test-wrong-pw",
	})
	defer client.Close()

	_, dialErr := client.Dial(context.Background(), "")
	// Either the client or server (or both) should see an auth error.

	select {
	case srvErr := <-serverErr:
		// At least one side must report an auth-related error.
		if srvErr == nil && dialErr == nil {
			t.Fatal("expected auth error from server or client, got none")
		}
		if srvErr != nil && !strings.Contains(srvErr.Error(), "auth") {
			t.Fatalf("server error not auth-related: %v", srvErr)
		}
		if dialErr != nil && !strings.Contains(dialErr.Error(), "auth") {
			t.Fatalf("client error not auth-related: %v", dialErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server timed out")
	}
}
