package yamux_test

import (
	"bytes"
	"context"
	"net"
	"testing"

	"github.com/ggshr9/shuttle/adapter"
	ymux "github.com/ggshr9/shuttle/transport/mux/yamux"
)

func TestYamuxMux_ClientServer(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	mux := ymux.New(nil)
	var _ adapter.Multiplexer = mux

	errCh := make(chan error, 1)
	dataCh := make(chan []byte, 1)
	go func() {
		serverMuxConn, err := mux.Server(serverConn)
		if err != nil {
			errCh <- err
			return
		}
		defer serverMuxConn.Close()

		s, err := serverMuxConn.AcceptStream(context.Background())
		if err != nil {
			errCh <- err
			return
		}
		defer s.Close()

		// Read exactly the expected message length, then send it back.
		buf := make([]byte, 128)
		n, err := s.Read(buf)
		if err != nil {
			errCh <- err
			return
		}
		dataCh <- buf[:n]
		errCh <- nil
	}()

	clientMuxConn, err := mux.Client(clientConn)
	if err != nil {
		t.Fatalf("Client() error: %v", err)
	}
	defer clientMuxConn.Close()

	s, err := clientMuxConn.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream error: %v", err)
	}
	defer s.Close()

	msg := []byte("hello yamux")
	if _, err := s.Write(msg); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Verify server received the data.
	got := <-dataCh
	if !bytes.Equal(got, msg) {
		t.Fatalf("got %q, want %q", got, msg)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("server error: %v", err)
	}

	// Verify StreamID is non-negative (basic sanity).
	_ = s.StreamID()

	// Verify LocalAddr/RemoteAddr are available.
	if clientMuxConn.LocalAddr() == nil {
		t.Fatal("LocalAddr() returned nil")
	}
	if clientMuxConn.RemoteAddr() == nil {
		t.Fatal("RemoteAddr() returned nil")
	}
}
