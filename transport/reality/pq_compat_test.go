package reality

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

// stubPeerPub satisfies the noisePeerPub interface for tests without
// running a real Noise handshake.
type stubPeerPub struct{}

func (stubPeerPub) PeerPublicKey() []byte { return []byte("stub") }

func newTestPQServer(t *testing.T) *Server {
	t.Helper()
	// Build a minimal server config with a valid private key.
	priv := make([]byte, 32)
	for i := range priv {
		priv[i] = byte(i + 1)
	}
	cfg := &ServerConfig{
		PostQuantum: true,
		PrivateKey:  bytesToHex(priv),
	}
	srv, err := NewServer(cfg, slog.Default())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

func bytesToHex(b []byte) string {
	const hexchars = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		out = append(out, hexchars[v>>4], hexchars[v&0x0f])
	}
	return string(out)
}

func TestDetectAndHandlePQ_ClassicalClient(t *testing.T) {
	srv := newTestPQServer(t)
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// Classical yamux bytes: protocol version 0, type 1 (window update),
	// flags 0x0000, stream id 0, length 0 → [0x00 0x01 0x00 0x00 0x00 0x00 0x00 0x00]
	yamuxPrefix := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	go func() {
		clientConn.Write(yamuxPrefix)
	}()

	next, err := srv.detectAndHandlePQ(serverConn, stubPeerPub{})
	if err != nil {
		t.Fatalf("detectAndHandlePQ classical: unexpected error %v", err)
	}
	if next == nil {
		t.Fatal("expected non-nil wrapped conn for classical client")
	}

	// The returned conn must replay the yamux bytes exactly.
	got := make([]byte, len(yamuxPrefix))
	next.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(next, got); err != nil {
		t.Fatalf("read replayed bytes: %v", err)
	}
	if !bytes.Equal(got, yamuxPrefix) {
		t.Fatalf("replayed bytes mismatch: got %x want %x", got, yamuxPrefix)
	}
}
