package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/transport/auth"
	yamuxmux "github.com/shuttleX/shuttle/transport/mux/yamux"
	tlswrap "github.com/shuttleX/shuttle/transport/security/tls"
)

func TestByteStreamPipeline_ConcurrentStreams(t *testing.T) {
	cert := generateSelfSignedCert(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	password := "integration-test"
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

	// Server: accept connections, echo each stream.
	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			go func(raw net.Conn) {
				secured, err := tlsServer.WrapServer(context.Background(), raw)
				if err != nil {
					raw.Close()
					return
				}
				if _, err := authenticator.AuthServer(secured); err != nil {
					secured.Close()
					return
				}
				serverConn, err := mux.Server(secured)
				if err != nil {
					secured.Close()
					return
				}
				for {
					s, err := serverConn.AcceptStream(context.Background())
					if err != nil {
						serverConn.Close()
						return
					}
					go func(s adapter.Stream) {
						io.Copy(s, s)
						s.Close()
					}(s)
				}
			}(raw)
		}
	}()

	// Client: ByteStreamClient with TLS + HMAC + yamux.
	client := NewByteStreamClient(ByteStreamConfig{
		Addr:     ln.Addr().String(),
		Dialer:   tcpDialer(),
		Security: []adapter.SecureWrapper{tlsClient},
		Auth:     authenticator,
		Mux:      mux,
		TypeName: "integration",
	})
	defer client.Close()

	conn, err := client.Dial(context.Background(), "")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// Open 10 concurrent streams, each sends unique data and verifies echo.
	const numStreams = 10
	var wg sync.WaitGroup
	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			s, err := conn.OpenStream(context.Background())
			if err != nil {
				t.Errorf("stream %d OpenStream: %v", id, err)
				return
			}
			defer s.Close()

			msg := []byte(fmt.Sprintf("stream-%d-payload", id))
			if _, err := s.Write(msg); err != nil {
				t.Errorf("stream %d Write: %v", id, err)
				return
			}

			// Read echo (server echoes as it receives, no half-close needed).
			buf := make([]byte, len(msg))
			if _, err := io.ReadFull(s, buf); err != nil {
				t.Errorf("stream %d ReadFull: %v", id, err)
				return
			}
			if string(buf) != string(msg) {
				t.Errorf("stream %d: got %q, want %q", id, buf, msg)
			}
		}(i)
	}
	wg.Wait()
}
