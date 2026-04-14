# Reality PQ Backward Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a Reality server with `post_quantum: true` accept both classical and PQ-capable clients by peek-detecting the first 3 bytes of the post-Noise stream and replaying them via a `peekConn` wrapper when the client turns out to be classical.

**Architecture:** Add `transport/reality/peekconn.go` (net.Conn wrapper with prefix-replay). Refactor the `handleConn` PQ branch into a helper `detectAndHandlePQ` that reads 3 bytes, inspects `peek[0]` and `peek[2]`, and dispatches to yamux-replay or PQ-exchange. Add a `pqPayloadMinSize` init-time guard to keep the detection heuristic honest.

**Tech Stack:** Go 1.24, existing `shuttlecrypto` PQ primitives, existing `wrapConnWithPQ` AEAD.

**Spec:** `docs/superpowers/specs/2026-04-14-reality-pq-compat.md`

---

## File Structure

**Create:**
- `transport/reality/peekconn.go` — `peekConn` wrapper
- `transport/reality/peekconn_test.go` — `peekConn` unit tests
- `transport/reality/pq_compat.go` — `pqPayloadMinSize` guard + `init()` assertion
- `transport/reality/pq_compat_test.go` — PQ-vs-classical detection tests (in-memory `net.Pipe`)
- `transport/reality/reality_pq_compat_sandbox_test.go` — sandbox integration test

**Modify:**
- `transport/reality/server.go` — replace the `if s.config.PostQuantum { ... }` block (lines 188–245) with a call to new helper `detectAndHandlePQ`

---

## Task 1: Add `peekConn` wrapper

**Files:**
- Create: `transport/reality/peekconn.go`
- Create: `transport/reality/peekconn_test.go`

- [ ] **Step 1: Write the failing test**

Create `transport/reality/peekconn_test.go`:

```go
package reality

import (
	"io"
	"net"
	"testing"
	"time"
)

func TestPeekConn_ReplaysPrefix(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	// Write "world" from the far side.
	go func() {
		c2.Write([]byte("world"))
	}()

	pc := &peekConn{Conn: c1, prefix: []byte("hello")}
	buf := make([]byte, 10)
	n, err := io.ReadFull(pc, buf)
	if err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if n != 10 {
		t.Fatalf("expected 10 bytes, got %d", n)
	}
	if string(buf) != "helloworld" {
		t.Fatalf("expected helloworld, got %q", string(buf))
	}
}

func TestPeekConn_PartialReads(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		c2.Write([]byte("XY"))
	}()

	pc := &peekConn{Conn: c1, prefix: []byte("AB")}
	for _, want := range []byte{'A', 'B', 'X', 'Y'} {
		b := make([]byte, 1)
		if _, err := io.ReadFull(pc, b); err != nil {
			t.Fatalf("read: %v", err)
		}
		if b[0] != want {
			t.Fatalf("expected %c, got %c", want, b[0])
		}
	}
}

func TestPeekConn_NilPrefix(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		c2.Write([]byte("direct"))
	}()

	pc := &peekConn{Conn: c1}
	buf := make([]byte, 6)
	if _, err := io.ReadFull(pc, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "direct" {
		t.Fatalf("expected direct, got %q", string(buf))
	}
}

func TestPeekConn_ForwardsDeadlines(t *testing.T) {
	c1, _ := net.Pipe()
	defer c1.Close()
	pc := &peekConn{Conn: c1, prefix: []byte{1, 2, 3}}
	if err := pc.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
}
```

- [ ] **Step 2: Run test — expect compile failure**

Run: `./scripts/test.sh --pkg ./transport/reality/ --run TestPeekConn`
Expected: FAIL — `undefined: peekConn`.

- [ ] **Step 3: Implement `peekConn`**

Create `transport/reality/peekconn.go`:

```go
package reality

import "net"

// peekConn wraps a net.Conn with a prepended byte buffer that is replayed
// on Read before reads fall through to the underlying conn. Used by the
// Reality server to "un-read" bytes consumed during PQ-vs-yamux detection.
//
// peekConn is not safe for concurrent use beyond what the underlying
// net.Conn already permits; callers must not Read from the conn outside
// the peekConn wrapper once wrapping is in effect.
type peekConn struct {
	net.Conn
	prefix []byte // bytes to replay first; emptied as Reads consume them
}

// Read drains the prefix buffer first, then falls through to the embedded
// net.Conn once the prefix is exhausted. Each Read call returns either
// prefix bytes or underlying-conn bytes, never a mix, matching
// net.Conn.Read semantics.
func (p *peekConn) Read(b []byte) (int, error) {
	if len(p.prefix) > 0 {
		n := copy(b, p.prefix)
		p.prefix = p.prefix[n:]
		if len(p.prefix) == 0 {
			p.prefix = nil
		}
		return n, nil
	}
	return p.Conn.Read(b)
}
```

All other `net.Conn` methods (`Write`, `Close`, `SetDeadline`, `SetReadDeadline`, `SetWriteDeadline`, `LocalAddr`, `RemoteAddr`) are inherited from the embedded `net.Conn`.

- [ ] **Step 4: Run tests — expect pass**

Run: `./scripts/test.sh --pkg ./transport/reality/ --run TestPeekConn`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add transport/reality/peekconn.go transport/reality/peekconn_test.go
git commit -m "feat(reality): add peekConn wrapper for protocol detection replay"
```

---

## Task 2: Add `pqPayloadMinSize` guard

**Files:**
- Create: `transport/reality/pq_compat.go`

- [ ] **Step 1: Write the guard**

Create `transport/reality/pq_compat.go`:

```go
package reality

import (
	shuttlecrypto "github.com/shuttleX/shuttle/crypto"
)

// pqPayloadMinSize guards the PQ-vs-yamux detection heuristic in
// detectAndHandlePQ. The detection assumes that a PQ frame's 2-byte
// big-endian length prefix has a non-zero high byte, which is true as
// long as the framed payload (version byte + pqPubKey) is > 255 bytes.
//
// X25519 (32) + ML-KEM-768 public key (1184) + 1 version byte = 1217 bytes,
// well above the threshold. If the PQ handshake ever shrinks the public
// key below 255 bytes, the detection becomes ambiguous and must be
// redesigned (e.g., by prepending an explicit magic marker).
const pqPayloadMinSize = 256

func init() {
	kp, err := shuttlecrypto.NewPQHandshake()
	if err != nil {
		// Crypto unavailable at init time (e.g., during test-only builds
		// that swap the provider). The real runtime will fail at first
		// use; no panic here to avoid breaking ephemeral environments.
		return
	}
	size := len(kp.PublicKeyBytes()) + 1 // +1 for the version byte in the payload
	if size < pqPayloadMinSize {
		panic("reality: PQ payload size shrank below detection threshold; revisit detectAndHandlePQ")
	}
}
```

- [ ] **Step 2: Run tests — expect pass (init runs, no panic)**

Run: `./scripts/test.sh --pkg ./transport/reality/`
Expected: PASS (package compiles, init passes silently).

- [ ] **Step 3: Commit**

```bash
git add transport/reality/pq_compat.go
git commit -m "feat(reality): add pqPayloadMinSize init guard for PQ detection"
```

---

## Task 3: Extract `detectAndHandlePQ` helper (non-behavior change)

**Files:**
- Modify: `transport/reality/server.go`

This task extracts the existing PQ branch into a standalone function WITHOUT changing behavior yet. The goal is to isolate it so the behavior change in Task 4 is surgical.

- [ ] **Step 1: Read `server.go:188-245` carefully**

Note the exact flow: deadline set, `readFrame`, deadline clear, version byte check, branch.

- [ ] **Step 2: Extract into `detectAndHandlePQ` method**

Add a new method to `Server` in `transport/reality/server.go` (place after `handleConn`):

```go
// detectAndHandlePQ handles the optional post-quantum KEM exchange phase.
// Before this refactor: strictly required a PQ frame from the client, closing
// classical clients. After this refactor (Task 4): detects PQ vs classical
// via byte peek and dispatches accordingly.
//
// Returns the net.Conn that subsequent yamux setup should use, or nil + error
// if the connection must be closed.
func (s *Server) detectAndHandlePQ(raw net.Conn, hs *shuttlecrypto.NoiseState) (net.Conn, error) {
	raw.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer raw.SetReadDeadline(time.Time{})

	pqFrame, err := readFrame(raw)
	if err != nil {
		return nil, fmt.Errorf("pq frame read: %w", err)
	}

	if len(pqFrame) == 0 || pqFrame[0] != shuttlecrypto.HandshakeVersionHybridPQ {
		return nil, fmt.Errorf("pq enabled but client sent classical frame")
	}

	pqPubBytes := pqFrame[1:]
	pq, err := shuttlecrypto.NewPQHandshake()
	if err != nil {
		return nil, fmt.Errorf("pq handshake init: %w", err)
	}

	pqSecret, ciphertext, err := pq.Encapsulate(pqPubBytes)
	if err != nil {
		return nil, fmt.Errorf("pq encapsulate: %w", err)
	}

	if err := writeFrame(raw, ciphertext); err != nil {
		return nil, fmt.Errorf("pq ciphertext send: %w", err)
	}

	wrapped, err := wrapConnWithPQ(raw, pqSecret)
	if err != nil {
		return nil, fmt.Errorf("pq wrap: %w", err)
	}
	s.logger.Debug("reality pq exchange complete", "peer", fmt.Sprintf("%x", hs.PeerPublicKey()))
	return wrapped, nil
}
```

**Note on the `hs` parameter type:** The Noise state type is `shuttlecrypto.NoiseState` (verify by reading `crypto/noise.go`). If the actual type differs, adjust the signature. The hs is only used for the `PeerPublicKey()` log string.

- [ ] **Step 3: Replace the inline block in `handleConn` with a call**

In `transport/reality/server.go`, replace lines 188–245 (the `if s.config.PostQuantum { ... }` block) with:

```go
	if s.config.PostQuantum {
		next, err := s.detectAndHandlePQ(raw, hs)
		if err != nil {
			s.logger.Warn("reality pq phase failed", "err", err)
			raw.Close()
			return
		}
		raw = next
	}
```

- [ ] **Step 4: Run the existing reality tests — all must still pass**

Run: `./scripts/test.sh --pkg ./transport/reality/`
Expected: PASS. No behavior change, only refactor. If `pq_test.go` or `reality_test.go` fails, the extraction introduced a regression — revert and try again.

- [ ] **Step 5: Verify the noise state type**

If Step 2's `*shuttlecrypto.NoiseState` type is wrong, grep for the actual type used at `handleConn`:
`rg 'NewResponder' crypto/` (via Grep tool).
Adjust the helper signature accordingly.

- [ ] **Step 6: Commit**

```bash
git add transport/reality/server.go
git commit -m "refactor(reality): extract detectAndHandlePQ helper (no behavior change)"
```

---

## Task 4: Add peek-based detection and classical fallback

**Files:**
- Modify: `transport/reality/server.go`

This is the core behavior change.

- [ ] **Step 1: Rewrite `detectAndHandlePQ` to peek 3 bytes**

Replace the body of `detectAndHandlePQ` in `transport/reality/server.go`:

```go
func (s *Server) detectAndHandlePQ(raw net.Conn, hs *shuttlecrypto.NoiseState) (net.Conn, error) {
	raw.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Peek 3 bytes to distinguish yamux (classical client) from PQ frame.
	//
	// Detection:
	//   peek[0] == 0x00  → yamux v0 header (classical client). Replay peek
	//                      into a peekConn and let yamux consume it.
	//   peek[2] == 0x02  → PQ frame: peek[0:2] is the 2-byte big-endian
	//                      length prefix, peek[2] is HandshakeVersionHybridPQ.
	//                      Read the rest of the payload and run the exchange.
	//   otherwise        → garbage. Close.
	//
	// The heuristic is unambiguous because the PQ payload is > 255 bytes
	// (enforced by pqPayloadMinSize in pq_compat.go), so the length prefix's
	// high byte is always non-zero for real PQ frames; and yamux v0 guarantees
	// the first byte is 0x00 (yamux protocol version).
	peek := make([]byte, 3)
	if _, err := io.ReadFull(raw, peek); err != nil {
		raw.SetReadDeadline(time.Time{})
		return nil, fmt.Errorf("pq peek read: %w", err)
	}

	switch {
	case peek[0] == 0x00:
		raw.SetReadDeadline(time.Time{})
		s.logger.Debug("reality pq-server accepting classical client",
			"peer", fmt.Sprintf("%x", hs.PeerPublicKey()))
		return &peekConn{Conn: raw, prefix: append([]byte(nil), peek...)}, nil

	case peek[2] == shuttlecrypto.HandshakeVersionHybridPQ:
		return s.finishPQExchange(raw, hs, peek)

	default:
		raw.SetReadDeadline(time.Time{})
		return nil, fmt.Errorf("pq detect: unexpected prefix %x", peek)
	}
}

// finishPQExchange completes the PQ handshake after detection has confirmed
// the client is sending a PQ frame. peek contains the 3 already-read bytes
// (2-byte length prefix + 1 byte of payload).
func (s *Server) finishPQExchange(raw net.Conn, hs *shuttlecrypto.NoiseState, peek []byte) (net.Conn, error) {
	defer raw.SetReadDeadline(time.Time{})

	payloadLen := int(binary.BigEndian.Uint16(peek[:2]))
	if payloadLen < 1 || payloadLen > 64*1024 {
		return nil, fmt.Errorf("pq frame length out of range: %d", payloadLen)
	}
	// We have peek[2] as the first payload byte (version 0x02).
	// Read the remaining payloadLen-1 bytes.
	rest := make([]byte, payloadLen-1)
	if _, err := io.ReadFull(raw, rest); err != nil {
		return nil, fmt.Errorf("pq payload read: %w", err)
	}

	// Reconstruct full payload: [version byte, pqPubKey...].
	pqFrame := make([]byte, 0, payloadLen)
	pqFrame = append(pqFrame, peek[2])
	pqFrame = append(pqFrame, rest...)

	pqPubBytes := pqFrame[1:]
	pq, err := shuttlecrypto.NewPQHandshake()
	if err != nil {
		return nil, fmt.Errorf("pq handshake init: %w", err)
	}

	pqSecret, ciphertext, err := pq.Encapsulate(pqPubBytes)
	if err != nil {
		return nil, fmt.Errorf("pq encapsulate: %w", err)
	}

	if err := writeFrame(raw, ciphertext); err != nil {
		return nil, fmt.Errorf("pq ciphertext send: %w", err)
	}

	wrapped, err := wrapConnWithPQ(raw, pqSecret)
	if err != nil {
		return nil, fmt.Errorf("pq wrap: %w", err)
	}
	s.logger.Debug("reality pq exchange complete", "peer", fmt.Sprintf("%x", hs.PeerPublicKey()))
	return wrapped, nil
}
```

Ensure `encoding/binary` is imported at the top of `server.go`.

- [ ] **Step 2: Run reality tests — existing PQ test must still pass**

Run: `./scripts/test.sh --pkg ./transport/reality/`
Expected: PASS for existing PQ tests (they use real PQ clients, which hit the `peek[2] == 0x02` branch).

- [ ] **Step 3: Commit**

```bash
git add transport/reality/server.go
git commit -m "feat(reality): peek-detect classical clients under PQ-enabled server"
```

---

## Task 5: In-memory test for classical client acceptance

**Files:**
- Create: `transport/reality/pq_compat_test.go`

- [ ] **Step 1: Write the failing test**

Create `transport/reality/pq_compat_test.go`:

```go
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

// fakeNoiseHS satisfies the subset of NoiseState used by detectAndHandlePQ
// (only PeerPublicKey for logging).
// Replaced with the real type once we confirm the type name.

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &ServerConfig{
		PostQuantum: true,
	}
	// A Server zero-value with a logger is enough for detectAndHandlePQ,
	// which doesn't use the listener. If NewServer requires a private key,
	// generate one via shuttlecrypto.
	priv := make([]byte, 32)
	for i := range priv {
		priv[i] = byte(i + 1)
	}
	cfg.PrivateKey = toHex(priv)
	srv, err := NewServer(cfg, slog.Default())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

func toHex(b []byte) string {
	const hexchars = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		out = append(out, hexchars[v>>4], hexchars[v&0x0f])
	}
	return string(out)
}

func TestDetectAndHandlePQ_ClassicalClient(t *testing.T) {
	srv := newTestServer(t)
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// Classical yamux bytes: protocol version 0, type 1 (window update),
	// flags 0x0000, stream id 0, length 0 → [0x00 0x01 0x00 0x00 0x00 0x00 ...]
	yamuxPrefix := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	go func() {
		clientConn.Write(yamuxPrefix)
	}()

	// We pass a nil NoiseState because our detect path only calls
	// PeerPublicKey for logging, which the Debug logger won't invoke
	// unless debug is on. If the signature requires non-nil, construct
	// a real NoiseState via shuttlecrypto.NewResponder.
	_ = shuttlecrypto.NewPQHandshake
	hs := newFakeNoiseHS(t)

	next, err := srv.detectAndHandlePQ(serverConn, hs)
	if err != nil {
		t.Fatalf("detectAndHandlePQ classical: unexpected error %v", err)
	}

	// The returned conn must replay the yamux bytes exactly.
	got := make([]byte, len(yamuxPrefix))
	serverConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(next, got); err != nil {
		t.Fatalf("read replayed bytes: %v", err)
	}
	if !bytes.Equal(got, yamuxPrefix) {
		t.Fatalf("replayed bytes mismatch: got %x want %x", got, yamuxPrefix)
	}
}

// newFakeNoiseHS constructs a NoiseState usable only for its PeerPublicKey
// method in debug logs. Adjust implementation once the real type is confirmed.
func newFakeNoiseHS(t *testing.T) *shuttlecrypto.NoiseState {
	t.Helper()
	// TODO: the test needs to construct whatever minimal NoiseState makes
	// detectAndHandlePQ's log line work. If NoiseState is opaque, either:
	//   (a) refactor detectAndHandlePQ to take an interface { PeerPublicKey() []byte }
	//       and pass a stub here, or
	//   (b) build a real NoiseState by running a full Noise handshake
	//       against a dummy responder in-memory.
	// Option (a) is cleaner; do that in this step.
	return nil
}
```

**Note to executor:** The `NoiseState` type's constructor is not trivial to stub. The cleanest fix is to **refactor `detectAndHandlePQ` to accept an interface** like `noiseHS interface { PeerPublicKey() []byte }`. Do this refactor in this task (it's a 2-line signature change).

- [ ] **Step 2: Refactor `detectAndHandlePQ` to take an interface**

In `transport/reality/server.go`, change the signature:

```go
type noisePeerPub interface {
	PeerPublicKey() []byte
}

func (s *Server) detectAndHandlePQ(raw net.Conn, hs noisePeerPub) (net.Conn, error) {
	...
}

func (s *Server) finishPQExchange(raw net.Conn, hs noisePeerPub, peek []byte) (net.Conn, error) {
	...
}
```

The caller in `handleConn` passes the real Noise state, which already has `PeerPublicKey() []byte` — verify by reading `crypto/noise.go` for the method. If the method name differs, adjust both sides.

- [ ] **Step 3: Replace the fake HS in the test with a stub**

Replace `newFakeNoiseHS` with:

```go
type stubPeerPub struct{}

func (stubPeerPub) PeerPublicKey() []byte { return []byte("stub") }
```

And in the test:

```go
next, err := srv.detectAndHandlePQ(serverConn, stubPeerPub{})
```

- [ ] **Step 4: Run the test — expect pass**

Run: `./scripts/test.sh --pkg ./transport/reality/ --run TestDetectAndHandlePQ_ClassicalClient`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add transport/reality/server.go transport/reality/pq_compat_test.go
git commit -m "test(reality): verify PQ server accepts classical yamux client"
```

---

## Task 6: In-memory test for PQ client on PQ server

**Files:**
- Modify: `transport/reality/pq_compat_test.go`

- [ ] **Step 1: Add PQ-client-path test**

Append to `transport/reality/pq_compat_test.go`:

```go
func TestDetectAndHandlePQ_PQClient(t *testing.T) {
	srv := newTestServer(t)
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
```

- [ ] **Step 2: Run test**

Run: `./scripts/test.sh --pkg ./transport/reality/ --run TestDetectAndHandlePQ_PQClient`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add transport/reality/pq_compat_test.go
git commit -m "test(reality): verify PQ server completes exchange with PQ client"
```

---

## Task 7: Test garbage-prefix rejection

**Files:**
- Modify: `transport/reality/pq_compat_test.go`

- [ ] **Step 1: Add garbage test**

Append to `transport/reality/pq_compat_test.go`:

```go
func TestDetectAndHandlePQ_GarbageRejected(t *testing.T) {
	srv := newTestServer(t)
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
```

- [ ] **Step 2: Run test**

Run: `./scripts/test.sh --pkg ./transport/reality/ --run TestDetectAndHandlePQ_GarbageRejected`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add transport/reality/pq_compat_test.go
git commit -m "test(reality): verify PQ detection rejects garbage prefix"
```

---

## Task 8: Sandbox integration test

**Files:**
- Create: `transport/reality/reality_pq_compat_sandbox_test.go`

- [ ] **Step 1: Write sandbox test**

Create `transport/reality/reality_pq_compat_sandbox_test.go`:

```go
//go:build sandbox

package reality

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	shuttlecrypto "github.com/shuttleX/shuttle/crypto"
)

// TestRealityPQServer_AcceptsClassicalAndPQ spins up a Reality server with
// PostQuantum enabled and verifies that both a classical and a PQ client
// can complete a full handshake + yamux stream + round-trip.
func TestRealityPQServer_AcceptsBothClients(t *testing.T) {
	priv, pub, err := shuttlecrypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	srvCfg := &ServerConfig{
		ListenAddr:  "127.0.0.1:0",
		PrivateKey:  toHex(priv[:]),
		PostQuantum: true,
	}
	srv, err := NewServer(srvCfg, slog.Default())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Listen(ctx); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer srv.Close()

	// Accept loop — echoes any stream data back.
	go func() {
		for {
			conn, err := srv.Accept(ctx)
			if err != nil {
				return
			}
			go func() {
				stream, err := conn.AcceptStream(ctx)
				if err != nil {
					return
				}
				io.Copy(stream, stream)
			}()
		}
	}()

	addr := srv.Addr() // assumes Server exposes Addr(); if not, read from listener
	pubHex := toHex(pub[:])

	// --- Classical client ---
	classicalCfg := &ClientConfig{
		ServerAddr:  addr,
		ServerName:  "example.com",
		PublicKey:   pubHex,
		Password:    "test",
		PostQuantum: false,
	}
	classical := NewClient(classicalCfg)
	cConn, err := classical.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("classical dial: %v", err)
	}
	defer cConn.Close()

	cStream, err := cConn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("classical open stream: %v", err)
	}
	cStream.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := cStream.Write([]byte("classical")); err != nil {
		t.Fatalf("classical write: %v", err)
	}
	buf := make([]byte, 9)
	if _, err := io.ReadFull(cStream, buf); err != nil {
		t.Fatalf("classical read: %v", err)
	}
	if string(buf) != "classical" {
		t.Fatalf("classical echo mismatch: %q", string(buf))
	}

	// --- PQ client ---
	pqCfg := &ClientConfig{
		ServerAddr:  addr,
		ServerName:  "example.com",
		PublicKey:   pubHex,
		Password:    "test",
		PostQuantum: true,
	}
	pqc := NewClient(pqCfg)
	pqConn, err := pqc.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("pq dial: %v", err)
	}
	defer pqConn.Close()

	pqStream, err := pqConn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("pq open stream: %v", err)
	}
	pqStream.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := pqStream.Write([]byte("pq-client")); err != nil {
		t.Fatalf("pq write: %v", err)
	}
	buf = make([]byte, 9)
	if _, err := io.ReadFull(pqStream, buf); err != nil {
		t.Fatalf("pq read: %v", err)
	}
	if string(buf) != "pq-client" {
		t.Fatalf("pq echo mismatch: %q", string(buf))
	}
}
```

**Executor notes:**
- `Server.Addr()` may not exist — if so, expose the listener address via a new getter or read from `s.listener.Addr().String()`.
- `GenerateKeyPair` may have a different name in `shuttlecrypto` — verify and adjust.
- `OpenStream` and `AcceptStream` method names come from the yamux adapter; verify via `transport/mux/yamux/`.

- [ ] **Step 2: Run sandbox test**

Run: `./scripts/test.sh --sandbox --pkg ./transport/reality/ --run TestRealityPQServer_AcceptsBothClients`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add transport/reality/reality_pq_compat_sandbox_test.go
git commit -m "test(reality): sandbox integration for PQ server + classical/PQ clients"
```

---

## Task 9: Full build and regression sweep

- [ ] **Step 1: Build both binaries**

Run:
```bash
CGO_ENABLED=0 go build -o /tmp/shuttle ./cmd/shuttle
CGO_ENABLED=0 go build -o /tmp/shuttled ./cmd/shuttled
```
Expected: both succeed.

- [ ] **Step 2: Run host-safe tests**

Run: `./scripts/test.sh`
Expected: PASS across all packages.

- [ ] **Step 3: Run sandbox tests for reality**

Run: `./scripts/test.sh --sandbox --pkg ./transport/reality/`
Expected: PASS — including existing `reality_test.go`, `pq_test.go`, `pqwrap_test.go`, and new `reality_pq_compat_sandbox_test.go`.

- [ ] **Step 4: If any regression surfaces, fix and commit separately**

---

## Self-Review Checklist

- [ ] `peekConn` replays prefix before falling through (unit test).
- [ ] `detectAndHandlePQ` returns the unwrapped peek-conn for classical clients.
- [ ] `detectAndHandlePQ` completes PQ exchange and returns AEAD-wrapped conn for PQ clients.
- [ ] Garbage prefixes are rejected cleanly.
- [ ] `pqPayloadMinSize` init guard present.
- [ ] Classical client against a PQ server successfully completes yamux handshake + round-trip (sandbox).
- [ ] PQ client against a PQ server successfully completes PQ exchange + yamux + round-trip (sandbox).
- [ ] No wire-protocol changes — classical clients untouched.
- [ ] Existing `reality_test.go`, `pq_test.go`, `pqwrap_test.go` all still pass.
- [ ] Both `shuttle` and `shuttled` binaries build with `CGO_ENABLED=0`.
