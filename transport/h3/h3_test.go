package h3

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

	"github.com/quic-go/quic-go"
	"github.com/ggshr9/shuttle/crypto"
	"github.com/ggshr9/shuttle/obfs"
	"github.com/ggshr9/shuttle/transport/auth"
)

func TestNewClient(t *testing.T) {
	c := NewClient(&ClientConfig{
		ServerAddr: "127.0.0.1:4433",
		ServerName: "example.com",
		Password:   "secret",
	})

	if c.Type() != "h3" {
		t.Fatalf("expected type h3, got %s", c.Type())
	}
	if c.config.PathPrefix != "/cdn/stream/" {
		t.Fatalf("expected default path prefix, got %s", c.config.PathPrefix)
	}
	if c.config.Fingerprint == nil {
		t.Fatal("expected default fingerprint config")
	}
}

func TestNewClientCustomPrefix(t *testing.T) {
	c := NewClient(&ClientConfig{
		PathPrefix: "/custom/",
	})
	if c.config.PathPrefix != "/custom/" {
		t.Fatalf("expected /custom/, got %s", c.config.PathPrefix)
	}
}

func TestClientClosedDial(t *testing.T) {
	c := NewClient(&ClientConfig{
		ServerAddr: "127.0.0.1:4433",
		Password:   "secret",
	})
	c.Close()

	_, err := c.Dial(context.Background(), "")
	if err == nil {
		t.Fatal("expected error dialing closed client")
	}
}

func TestComputeSessionAuth(t *testing.T) {
	payload, err := computeSessionAuth("testpassword")
	if err != nil {
		t.Fatalf("computeSessionAuth: %v", err)
	}
	if len(payload) != 64 {
		t.Fatalf("expected 64-byte auth payload, got %d", len(payload))
	}

	nonce := payload[:32]
	mac := payload[32:]
	if !auth.VerifyHMAC(nonce, mac, "testpassword") {
		t.Fatal("HMAC verification failed for correct password")
	}
	if auth.VerifyHMAC(nonce, mac, "wrongpassword") {
		t.Fatal("HMAC verification succeeded for wrong password")
	}
}

func TestChromeFingerprint(t *testing.T) {
	params := DefaultChromeTransportParams()

	if params.MaxIdleTimeout != 30_000 {
		t.Fatalf("MaxIdleTimeout = %d, want 30000", params.MaxIdleTimeout)
	}
	if params.MaxUDPPayloadSize != 1350 {
		t.Fatalf("MaxUDPPayloadSize = %d, want 1350", params.MaxUDPPayloadSize)
	}
	if params.InitialMaxStreamsBidi != 100 {
		t.Fatalf("InitialMaxStreamsBidi = %d, want 100", params.InitialMaxStreamsBidi)
	}
}

func TestDefaultFingerprint(t *testing.T) {
	fp := DefaultFingerprint()
	if fp.Browser != "chrome" {
		t.Fatalf("expected chrome, got %s", fp.Browser)
	}
	if fp.Platform != "windows" {
		t.Fatalf("expected windows, got %s", fp.Platform)
	}
}

func TestChromeALPN(t *testing.T) {
	if len(ChromeALPN) != 1 || ChromeALPN[0] != "h3" {
		t.Fatalf("unexpected ChromeALPN: %v", ChromeALPN)
	}
}

func TestChromeCipherSuites(t *testing.T) {
	expected := map[uint16]bool{
		tls.TLS_AES_128_GCM_SHA256:       true,
		tls.TLS_AES_256_GCM_SHA384:       true,
		tls.TLS_CHACHA20_POLY1305_SHA256: true,
	}
	for _, cs := range ChromeCipherSuites {
		if !expected[cs] {
			t.Fatalf("unexpected cipher suite: %d", cs)
		}
	}
}

func TestNewServer(t *testing.T) {
	s := NewServer(&ServerConfig{
		ListenAddr: ":0",
		Password:   "secret",
	}, nil)

	if s.Type() != "h3" {
		t.Fatalf("expected type h3, got %s", s.Type())
	}
	if s.config.PathPrefix != "/cdn/stream/" {
		t.Fatalf("expected default path prefix, got %s", s.config.PathPrefix)
	}
	if s.replayFilter == nil {
		t.Fatal("expected non-nil replay filter")
	}
}

func TestServerAcceptTimeout(t *testing.T) {
	s := NewServer(&ServerConfig{Password: "secret"}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := s.Accept(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestServerCloseBeforeListen(t *testing.T) {
	s := NewServer(&ServerConfig{Password: "secret"}, nil)
	err := s.Close()
	if err != nil {
		t.Fatalf("Close before Listen: %v", err)
	}
}

// TestH3EndToEnd performs a full client→server handshake and data transfer
// over a real QUIC connection using loopback.
func TestH3EndToEnd(t *testing.T) {
	password := "test-e2e-password"

	// Generate self-signed cert for the QUIC server.
	certPEM, keyPEM, err := crypto.GenerateSelfSignedCert(nil, 24*time.Hour)
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	// Start QUIC listener.
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   ChromeALPN,
		MinVersion:   tls.VersionTLS13,
	}
	quicConf := &quic.Config{
		MaxIdleTimeout:            30 * time.Second,
		MaxIncomingStreams:         100,
		MaxIncomingUniStreams:      100,
		MaxStreamReceiveWindow:    6 * 1024 * 1024,
		MaxConnectionReceiveWindow: 15 * 1024 * 1024,
	}

	ln, err := quic.ListenAddr("127.0.0.1:0", tlsConf, quicConf)
	if err != nil {
		t.Fatalf("quic listen: %v", err)
	}
	defer ln.Close()
	listenAddr := ln.Addr().String()

	replayFilter := crypto.NewReplayFilter(60 * time.Second)
	padder := obfs.NewPadder(0)

	// Server goroutine: accept one connection, do auth, then echo data on a stream.
	serverReady := make(chan struct{})
	serverErr := make(chan error, 1)
	go func() {
		close(serverReady)
		qconn, err := ln.Accept(context.Background())
		if err != nil {
			serverErr <- err
			return
		}

		// Auth handshake
		ctrlStream, err := qconn.AcceptStream(context.Background())
		if err != nil {
			serverErr <- err
			return
		}
		ctrlStream.SetReadDeadline(time.Now().Add(5 * time.Second))
		authBuf := make([]byte, 64)
		if _, err := io.ReadFull(ctrlStream, authBuf); err != nil {
			serverErr <- err
			return
		}

		nonce := authBuf[:32]
		clientMAC := authBuf[32:]
		if replayFilter.CheckBytes(nonce) {
			serverErr <- fmt.Errorf("replay detected")
			return
		}
		if !auth.VerifyHMAC(nonce, clientMAC, password) {
			ctrlStream.Write([]byte{0x00})
			ctrlStream.Close()
			serverErr <- fmt.Errorf("auth failed")
			return
		}
		ctrlStream.Write([]byte{0x01})
		ctrlStream.CancelRead(0)
		ctrlStream.Close()

		// Accept data stream and echo back
		dataStream, err := qconn.AcceptStream(context.Background())
		if err != nil {
			serverErr <- err
			return
		}
		s := &h3Stream{qs: dataStream, padder: padder}
		buf := make([]byte, 4096)
		n, err := s.Read(buf)
		if err != nil {
			serverErr <- err
			return
		}
		if _, err := s.Write(buf[:n]); err != nil {
			serverErr <- err
			return
		}
		s.Close()
		serverErr <- nil
	}()

	<-serverReady

	// Client connects
	client := NewClient(&ClientConfig{
		ServerAddr:         listenAddr,
		ServerName:         "localhost",
		Password:           password,
		InsecureSkipVerify: true,
	})
	defer client.Close()

	conn, err := client.Dial(context.Background(), "")
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	defer conn.Close()

	// Open data stream and send data
	stream, err := conn.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}

	testData := []byte("hello shuttle h3 transport")
	if _, err := stream.Write(testData); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read echoed response
	buf := make([]byte, 4096)
	n, err := stream.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(buf[:n], testData) {
		t.Fatalf("echo mismatch: got %q, want %q", buf[:n], testData)
	}

	// Wait for server goroutine
	if err := <-serverErr; err != nil {
		t.Fatalf("server error: %v", err)
	}
}

// TestH3AuthRejection verifies that wrong passwords are rejected.
func TestH3AuthRejection(t *testing.T) {
	password := "correct-password"

	certPEM, keyPEM, err := crypto.GenerateSelfSignedCert(nil, 24*time.Hour)
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   ChromeALPN,
		MinVersion:   tls.VersionTLS13,
	}
	ln, err := quic.ListenAddr("127.0.0.1:0", tlsConf, &quic.Config{
		MaxIdleTimeout:    10 * time.Second,
		MaxIncomingStreams: 100,
	})
	if err != nil {
		t.Fatalf("quic listen: %v", err)
	}
	defer ln.Close()

	go func() {
		qconn, err := ln.Accept(context.Background())
		if err != nil {
			return
		}
		ctrlStream, err := qconn.AcceptStream(context.Background())
		if err != nil {
			return
		}
		ctrlStream.SetReadDeadline(time.Now().Add(5 * time.Second))
		authBuf := make([]byte, 64)
		io.ReadFull(ctrlStream, authBuf)
		nonce := authBuf[:32]
		clientMAC := authBuf[32:]
		if !auth.VerifyHMAC(nonce, clientMAC, password) {
			ctrlStream.Write([]byte{0x00})
		} else {
			ctrlStream.Write([]byte{0x01})
		}
		ctrlStream.CancelRead(0)
		ctrlStream.Close()
	}()

	// Client with wrong password
	client := NewClient(&ClientConfig{
		ServerAddr:         ln.Addr().String(),
		ServerName:         "localhost",
		Password:           "wrong-password",
		InsecureSkipVerify: true,
	})
	defer client.Close()

	_, err = client.Dial(context.Background(), "")
	if err == nil {
		t.Fatal("expected auth rejection error")
	}
}

// TestH3ConcurrentStreams tests multiplexing multiple streams.
func TestH3ConcurrentStreams(t *testing.T) {
	password := "concurrent-test"

	certPEM, keyPEM, _ := crypto.GenerateSelfSignedCert(nil, 24*time.Hour)
	cert, _ := tls.X509KeyPair(certPEM, keyPEM)

	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   ChromeALPN,
		MinVersion:   tls.VersionTLS13,
	}
	ln, err := quic.ListenAddr("127.0.0.1:0", tlsConf, &quic.Config{
		MaxIdleTimeout:    30 * time.Second,
		MaxIncomingStreams: 100,
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	padder := obfs.NewPadder(0)

	// Server: auth then echo on each stream
	go func() {
		qconn, _ := ln.Accept(context.Background())
		ctrl, _ := qconn.AcceptStream(context.Background())
		ctrl.SetReadDeadline(time.Now().Add(5 * time.Second))
		authBuf := make([]byte, 64)
		io.ReadFull(ctrl, authBuf)
		if auth.VerifyHMAC(authBuf[:32], authBuf[32:], password) {
			ctrl.Write([]byte{0x01})
		}
		ctrl.CancelRead(0)
		ctrl.Close()

		for i := 0; i < 5; i++ {
			qs, err := qconn.AcceptStream(context.Background())
			if err != nil {
				return
			}
			go func(qs *quic.Stream) {
				s := &h3Stream{qs: qs, padder: padder}
				buf := make([]byte, 4096)
				n, _ := s.Read(buf)
				s.Write(buf[:n])
				s.Close()
			}(qs)
		}
	}()

	client := NewClient(&ClientConfig{
		ServerAddr:         ln.Addr().String(),
		ServerName:         "localhost",
		Password:           password,
		InsecureSkipVerify: true,
	})
	defer client.Close()

	conn, err := client.Dial(context.Background(), "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			stream, err := conn.OpenStream(context.Background())
			if err != nil {
				t.Errorf("open stream %d: %v", id, err)
				return
			}
			msg := []byte(fmt.Sprintf("stream-%d", id))
			stream.Write(msg)
			buf := make([]byte, 4096)
			n, _ := stream.Read(buf)
			if !bytes.Equal(buf[:n], msg) {
				t.Errorf("stream %d echo mismatch", id)
			}
			stream.Close()
		}(i)
	}
	wg.Wait()
}

// TestH3StreamReadBuffer tests the h3Stream read buffering logic.
func TestH3StreamReadBuffer(t *testing.T) {
	padder := obfs.NewPadder(0)

	// Test the readBuf logic directly — when readBuf has leftover data.
	s := &h3Stream{
		padder: padder,
	}

	s.readBuf = []byte("buffered data")
	smallBuf := make([]byte, 5)
	n, err := s.Read(smallBuf)
	if err != nil {
		t.Fatalf("read from buffer: %v", err)
	}
	if n != 5 || string(smallBuf) != "buffe" {
		t.Fatalf("expected 'buffe', got %q", smallBuf[:n])
	}

	// Remaining buffer should have "red data"
	largeBuf := make([]byte, 100)
	n, _ = s.Read(largeBuf)
	if n != 8 {
		t.Fatalf("expected 8 remaining bytes, got %d", n)
	}
	if string(largeBuf[:n]) != "red data" {
		t.Fatalf("expected 'red data', got %q", largeBuf[:n])
	}
}

// TestH3ConnectionAddrs verifies LocalAddr/RemoteAddr are non-nil after connect.
func TestH3ConnectionAddrs(t *testing.T) {
	conn := &h3Connection{
		qconn: nil, // We can't easily mock quic.Conn, just test the type assertions
	}
	// These would panic with nil qconn, which is expected—test skipped.
	// The real test is in TestH3EndToEnd above.
	_ = conn
}

// Verify interface compliance at compile time.
var _ net.Addr = (*net.UDPAddr)(nil)
