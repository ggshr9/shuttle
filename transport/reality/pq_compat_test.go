package reality

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	shuttlecrypto "github.com/shuttleX/shuttle/crypto"
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

func TestDetectAndHandlePQ_PQClient(t *testing.T) {
	srv := newTestPQServer(t)
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// Simulate a PQ client: generate a PQ handshake, send the framed
	// pubkey, and expect ciphertext back.
	clientPQ, err := shuttlecrypto.NewPQHandshake()
	if err != nil {
		t.Fatalf("client pq init: %v", err)
	}

	// Build the wire frame: [2-byte length BE][version byte 0x02][pqPub]
	pqPub := clientPQ.PublicKeyBytes()
	payload := make([]byte, 0, 1+len(pqPub))
	payload = append(payload, shuttlecrypto.HandshakeVersionHybridPQ)
	payload = append(payload, pqPub...)

	frame := make([]byte, 2+len(payload))
	frame[0] = byte(len(payload) >> 8)
	frame[1] = byte(len(payload) & 0xff)
	copy(frame[2:], payload)

	// Write PQ frame in a goroutine so server-side read can proceed.
	go func() {
		clientConn.Write(frame)
	}()

	// Also prepare to read ciphertext (written by server) in another goroutine.
	ctCh := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		// Read the length-prefixed ciphertext frame.
		header := make([]byte, 2)
		if _, err := io.ReadFull(clientConn, header); err != nil {
			errCh <- err
			return
		}
		ctLen := int(header[0])<<8 | int(header[1])
		ct := make([]byte, ctLen)
		if _, err := io.ReadFull(clientConn, ct); err != nil {
			errCh <- err
			return
		}
		ctCh <- ct
	}()

	next, err := srv.detectAndHandlePQ(serverConn, stubPeerPub{})
	if err != nil {
		t.Fatalf("detectAndHandlePQ pq: %v", err)
	}
	if next == nil {
		t.Fatal("expected wrapped conn, got nil")
	}

	// Fetch ciphertext from client side.
	var ct []byte
	select {
	case ct = <-ctCh:
	case e := <-errCh:
		t.Fatalf("client read ciphertext: %v", e)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ciphertext")
	}

	clientSecret, err := clientPQ.Decapsulate(ct)
	if err != nil {
		t.Fatalf("client decap: %v", err)
	}
	clientWrapped, err := wrapConnWithPQ(clientConn, clientSecret)
	if err != nil {
		t.Fatalf("client wrapConnWithPQ: %v", err)
	}

	// Round-trip a short message through the AEAD tunnel.
	msg := []byte("hello-pq")
	go func() {
		clientWrapped.Write(msg)
	}()

	got := make([]byte, len(msg))
	next.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(next, got); err != nil {
		t.Fatalf("server read pq payload: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("pq round trip: got %q want %q", got, msg)
	}
}

func TestDetectAndHandlePQ_GarbageRejected(t *testing.T) {
	srv := newTestPQServer(t)
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// Prefix that is neither yamux (peek[0]==0) nor PQ (peek[2]==0x02).
	go func() {
		clientConn.Write([]byte{0x04, 0xC1, 0xFF, 0x00, 0x00})
	}()

	_, err := srv.detectAndHandlePQ(serverConn, stubPeerPub{})
	if err == nil {
		t.Fatal("expected error for garbage prefix, got nil")
	}
}
