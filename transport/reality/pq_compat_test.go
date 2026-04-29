package reality

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/ggshr9/shuttle/transport"
	"golang.org/x/crypto/curve25519"

	shuttlecrypto "github.com/ggshr9/shuttle/crypto"
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

// TestPQServer_HandleConnIntegrationClassicalClient exercises the full
// production integration path on a PQ-enabled server when a classical
// (non-PQ) client connects:
//
//	handleConn → Noise IK → detectAndHandlePQ → peekConn → yamux.Server
//
// A regression where handleConn fails to substitute the wrapped conn
// returned by detectAndHandlePQ (e.g. removing `raw = next`) would cause
// yamux.Server to read the post-peek byte stream directly — losing the 3
// peeked bytes — and the yamux handshake would fail, failing this test.
//
// The test uses net.Pipe() (host-safe, no real listeners) and drives
// srv.handleConn directly in a goroutine; srv.Accept delivers the resulting
// server-side Connection. A background echo loop on the server conn echoes
// bytes back so the client can verify a successful round-trip.
func TestPQServer_HandleConnIntegrationClassicalClient(t *testing.T) {
	srv := newTestPQServer(t)

	// Derive the server's public key from the same deterministic private
	// key that newTestPQServer constructs (bytes 1..32). The server stores
	// these internally via NewServer; we recompute pub here so the client
	// can run Noise IK as the initiator.
	var priv [32]byte
	for i := range priv {
		priv[i] = byte(i + 1)
	}
	pubSlice, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		t.Fatalf("derive server pub: %v", err)
	}
	var serverPub [32]byte
	copy(serverPub[:], pubSlice)

	// Client static keys (arbitrary, not shared with server — Noise IK
	// only authenticates the server on the first flight).
	var clientPriv [32]byte
	for i := range clientPriv {
		clientPriv[i] = byte(0x80 + i)
	}
	clientPubSlice, err := curve25519.X25519(clientPriv[:], curve25519.Basepoint)
	if err != nil {
		t.Fatalf("derive client pub: %v", err)
	}
	var clientPub [32]byte
	copy(clientPub[:], clientPubSlice)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Drive handleConn directly on the server-side pipe end.
	handleDone := make(chan struct{})
	go func() {
		defer close(handleDone)
		srv.handleConn(ctx, serverConn)
	}()

	// Accept the transport.Connection that handleConn will push into connCh,
	// then run an echo loop on the first inbound stream.
	echoErrCh := make(chan error, 1)
	go func() {
		conn, err := srv.Accept(ctx)
		if err != nil {
			echoErrCh <- err
			return
		}
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			echoErrCh <- err
			return
		}
		buf := make([]byte, 4096)
		n, err := stream.Read(buf)
		if err != nil {
			echoErrCh <- err
			return
		}
		if _, err := stream.Write(buf[:n]); err != nil {
			echoErrCh <- err
			return
		}
		echoErrCh <- nil
	}()

	// Client side: Noise IK handshake (initiator).
	hs, err := shuttlecrypto.NewInitiator(clientPriv, clientPub, serverPub)
	if err != nil {
		t.Fatalf("noise initiator: %v", err)
	}
	msg1, err := hs.WriteMessage(nil)
	if err != nil {
		t.Fatalf("noise write msg1: %v", err)
	}
	if err := writeFrame(clientConn, msg1); err != nil {
		t.Fatalf("send msg1: %v", err)
	}
	msg2, err := readFrame(clientConn)
	if err != nil {
		t.Fatalf("read msg2: %v", err)
	}
	if _, err := hs.ReadMessage(msg2); err != nil {
		t.Fatalf("noise read msg2: %v", err)
	}
	if !hs.Completed() {
		t.Fatal("noise handshake incomplete")
	}

	// Classical path: skip the PQ frame and go straight to yamux. The
	// server's detectAndHandlePQ will peek the first 3 yamux header bytes,
	// wrap serverConn in a peekConn, and hand it back to handleConn which
	// must use it for yamux.Server().
	yamuxSess, err := yamux.Client(clientConn, yamux.DefaultConfig())
	if err != nil {
		t.Fatalf("yamux client: %v", err)
	}
	defer yamuxSess.Close()

	stream, err := yamuxSess.OpenStream()
	if err != nil {
		t.Fatalf("yamux open: %v", err)
	}

	payload := []byte("classical-handleconn-test")
	if _, err := stream.Write(payload); err != nil {
		t.Fatalf("stream write: %v", err)
	}

	got := make([]byte, len(payload))
	if err := stream.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if _, err := io.ReadFull(stream, got); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("echo mismatch: got %q want %q", got, payload)
	}

	// Cleanly verify the server echo goroutine finished without error.
	select {
	case err := <-echoErrCh:
		if err != nil {
			t.Fatalf("server echo: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for server echo goroutine")
	}

	// Sanity: confirm we used the transport.Connection type path (not a
	// compile-time-only reference). This line is also a guard that the
	// transport package stays imported even if the interface shape changes.
	var _ transport.Connection = (transport.Connection)(nil)
}
