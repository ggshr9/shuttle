package transport

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/transport/auth"
	yamuxmux "github.com/shuttleX/shuttle/transport/mux/yamux"
	tlswrap "github.com/shuttleX/shuttle/transport/security/tls"
)

// tcpDialer returns an adapter.DialerFunc that wraps net.Dial.
func tcpDialer() adapter.DialerFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
}

func TestByteStreamClient_DialAndStream(t *testing.T) {
	const password = "test-secret"

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	mux := yamuxmux.New(nil)
	authenticator := auth.NewHMACAuthenticator(password)

	// Server goroutine.
	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		// Auth server side.
		if _, err := authenticator.AuthServer(conn); err != nil {
			serverErr <- err
			return
		}

		// Mux server side.
		muxConn, err := mux.Server(conn)
		if err != nil {
			serverErr <- err
			return
		}
		defer muxConn.Close()

		// Accept a stream and echo.
		stream, err := muxConn.AcceptStream(context.Background())
		if err != nil {
			serverErr <- err
			return
		}
		defer stream.Close()

		_, err = io.Copy(stream, stream)
		if err != nil {
			// yamux may return EOF on copy; that's fine.
			serverErr <- nil
			return
		}
		serverErr <- nil
	}()

	// Client side.
	client := NewByteStreamClient(ByteStreamConfig{
		Addr:     ln.Addr().String(),
		Dialer:   tcpDialer(),
		Auth:     authenticator,
		Mux:      mux,
		TypeName: "test-plain",
	})
	defer client.Close()

	if client.Type() != "test-plain" {
		t.Fatalf("Type() = %q, want %q", client.Type(), "test-plain")
	}

	conn, err := client.Dial(context.Background(), "")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	stream, err := conn.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}

	msg := []byte("hello bytestream")
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

// generateSelfSignedCert creates a self-signed TLS certificate for testing.
func generateSelfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}
}

func TestByteStreamClient_WithTLS(t *testing.T) {
	const password = "tls-secret"

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

	// Server goroutine.
	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		// TLS server wrap.
		tlsConn, err := tlsServer.WrapServer(context.Background(), conn)
		if err != nil {
			serverErr <- err
			return
		}

		// Auth server side.
		if _, err := authenticator.AuthServer(tlsConn); err != nil {
			serverErr <- err
			return
		}

		// Mux server side.
		muxConn, err := mux.Server(tlsConn)
		if err != nil {
			serverErr <- err
			return
		}
		defer muxConn.Close()

		// Accept a stream and echo.
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

	// Client side.
	client := NewByteStreamClient(ByteStreamConfig{
		Addr:     ln.Addr().String(),
		Dialer:   tcpDialer(),
		Security: []adapter.SecureWrapper{tlsClient},
		Auth:     authenticator,
		Mux:      mux,
		TypeName: "test-tls",
	})
	defer client.Close()

	if client.Type() != "test-tls" {
		t.Fatalf("Type() = %q, want %q", client.Type(), "test-tls")
	}

	conn, err := client.Dial(context.Background(), "")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	stream, err := conn.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}

	msg := []byte("hello tls bytestream")
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
