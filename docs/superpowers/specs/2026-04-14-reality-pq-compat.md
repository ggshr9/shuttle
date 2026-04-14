# Spec: Reality PQ Backward Compatibility

**Date:** 2026-04-14
**Status:** Draft
**Owner:** TBD
**Type:** Protocol compatibility fix

## Problem

`transport/reality/server.go:188-245` has a TODO gap: when a Reality server has `PostQuantum: true` enabled, it **strictly requires** the connecting client to also perform the PQ exchange. Classical (non-PQ) clients are closed with a debug log:

```go
// Client sent a non-PQ frame (likely yamux). We can't un-read it,
// so for a fully production implementation we would need framing
// that distinguishes PQ from yamux. For now, log and close.
s.logger.Debug("pq enabled but client sent classical frame, closing")
raw.Close()
return
```

This breaks **mixed-version fleets**: the moment an operator flips `post_quantum: true` on their server, every existing classical client on the network loses connectivity until re-configured with PQ enabled. There is no graceful-degradation path.

### Why the Current Implementation Is Strict

The Reality handshake is:

```
Client → Server:    TLS 1.3 (SNI impersonation)
Client ↔ Server:    Noise IK (Noise_IK_25519_ChaChaPoly_SHA256)
                    [2 messages, length-prefixed via writeFrame/readFrame]
                    ↓
               [optional PQ exchange if both sides enabled]
                    ↓
Client ↔ Server:    yamux multiplex
```

When PQ is disabled, immediately after Noise IK completes, both sides start speaking yamux on the raw TLS connection. When PQ is enabled, the **client** proactively sends a PQ public-key frame (`writeFrame(raw, [0x02, pqPubKey...])`) and the **server** reads it. Because yamux framing and PQ framing share the same underlying stream, the server has no way to distinguish the two *before* consuming bytes:

- `readFrame(raw)` consumes 2 bytes of length prefix, then `length` bytes of payload.
- If the client is actually speaking yamux, those first bytes are yamux's spec-header (`0x00 0x00` is typical — version byte `0x00` followed by type byte `0x00` for data frames).
- Once `readFrame` consumes those bytes, the server **cannot un-read them**, so falling back to `mux.Server(raw)` would see a truncated yamux stream and fail.

The current code "solves" this by closing the connection. That's safe but user-hostile.

## Goals

1. A PQ-enabled Reality server **accepts both** PQ clients (wrapping the connection with the derived AEAD) and classical clients (proceeding straight to yamux), without requiring client-side coordination.
2. Detection is deterministic — no timing-based heuristics, no ambiguous branches.
3. Classical client behavior is **unchanged** — no wire protocol modification, no forced upgrades.
4. PQ client behavior is unchanged — same framing on the wire.
5. Failed detection (garbage bytes) is logged and closed cleanly; no security downgrade.

## Non-Goals

- Introducing a Reality protocol version 2 with explicit capability negotiation (too invasive, breaks classical clients).
- Forcing classical clients to upgrade (the operator controls this via `post_quantum: false`).
- Changing client-side behavior at all. This is a server-only fix.
- IPv6 / TUN / non-Reality transports — out of scope.

## Design

### Observation: Byte-Level Distinguishability

The PQ frame and yamux protocol happen to be **byte-level distinguishable** if we peek at the first 3 bytes:

| Protocol | First 3 bytes | Why |
|---|---|---|
| Yamux v0 | `0x00 0xHH 0xHH` (first byte is protocol version = 0) | yamux spec: header[0] = version = 0 |
| PQ frame | `0xLL 0xLL 0x02` (2-byte big-endian length prefix, then version byte `HandshakeVersionHybridPQ=0x02`) | `pqMsg[0] = 0x02` at payload offset 0, which is wire offset 2 |

For a PQ frame, the 2-byte length prefix encodes `1 + len(pqPubKey)`. ML-KEM-768 + X25519 hybrid public key is a fixed 1216 bytes (`crypto/pqkem.go` — `PublicKeyBytes` returns X25519 32 bytes + ML-KEM-768 1184 bytes = 1216). Total payload = 1217 bytes = `0x04C1`. So wire bytes are: `0x04 0xC1 0x02 <x25519_32><mlkem_1184>`.

For yamux: `0x00 <type> <flags_hi>` — first byte is always `0x00` for yamux v0.

**Detection rule:** peek 3 bytes on the raw connection:

- If `peek[0] == 0x00`: classical client (yamux). Do not consume any bytes — replay the peek bytes via a wrapper and hand to `mux.Server`.
- If `peek[0] != 0x00` AND `peek[2] == HandshakeVersionHybridPQ (0x02)`: PQ client. Continue reading the length-prefixed frame and do the PQ exchange.
- Otherwise: garbage. Close with error.

This is **unambiguous** given the current PQ payload size (always 1217 bytes → first byte 0x04). If in the future the PQ payload size changes to something with `0x00` as its high byte (i.e., total frame < 256 bytes), detection becomes ambiguous. **Mitigation**: add a static assertion that the PQ payload size is > 255. ML-KEM-768 guarantees this for the foreseeable future.

### Implementation

#### New File: `transport/reality/peekconn.go`

A `peekConn` wraps a `net.Conn` with a prepended byte buffer:

```go
// peekConn is a net.Conn that replays a prefix buffer on Read before
// falling through to the underlying conn. Used to "un-read" bytes that
// were consumed during protocol detection.
type peekConn struct {
    net.Conn
    prefix []byte // bytes to replay first; emptied as Reads consume them
}

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

`Write`, `Close`, `LocalAddr`, `RemoteAddr`, `SetDeadline`, `SetReadDeadline`, `SetWriteDeadline` are inherited via the embedded `net.Conn`.

#### Server Branch Refactor: `transport/reality/server.go`

Replace lines 188–245 (`if s.config.PostQuantum { ... }` block) with:

```go
	// Post-quantum hybrid KEM exchange (optional, after Noise IK).
	// Detects whether the client is speaking PQ or yamux by peeking the
	// first 3 bytes of the stream without consuming them irrevocably.
	//
	// Detection key (see peekPQOrYamux for details):
	//   peek[0]==0x00 → yamux (classical client)
	//   peek[2]==HandshakeVersionHybridPQ → PQ client
	//   otherwise   → garbage, close.
	if s.config.PostQuantum {
		raw.SetReadDeadline(time.Now().Add(5 * time.Second))
		peek := make([]byte, 3)
		if _, err := io.ReadFull(raw, peek); err != nil {
			s.logger.Warn("pq peek read failed", "err", err)
			raw.Close()
			return
		}
		raw.SetReadDeadline(time.Time{})

		switch {
		case peek[0] == 0x00:
			// Classical client; replay the 3 peeked bytes into the yamux stream.
			s.logger.Debug("reality pq-server accepting classical client")
			raw = &peekConn{Conn: raw, prefix: append([]byte(nil), peek...)}

		case peek[2] == shuttlecrypto.HandshakeVersionHybridPQ:
			// PQ frame. peek[0:2] is the 2-byte big-endian length prefix of
			// the pqMsg payload (version byte + pqPubKey). Read the rest.
			payloadLen := int(binary.BigEndian.Uint16(peek[:2]))
			if payloadLen < 1 || payloadLen > 64*1024 {
				s.logger.Warn("pq frame length out of range", "len", payloadLen)
				raw.Close()
				return
			}
			// We've already read 1 byte of the payload (peek[2] == 0x02 version).
			// Read the remaining payloadLen-1 bytes.
			rest := make([]byte, payloadLen-1)
			raw.SetReadDeadline(time.Now().Add(5 * time.Second))
			if _, err := io.ReadFull(raw, rest); err != nil {
				s.logger.Warn("pq payload read failed", "err", err)
				raw.Close()
				return
			}
			raw.SetReadDeadline(time.Time{})

			// Reconstruct the full PQ payload (version byte + pqPubKey).
			pqFrame := make([]byte, 0, payloadLen)
			pqFrame = append(pqFrame, peek[2])
			pqFrame = append(pqFrame, rest...)

			pqPubBytes := pqFrame[1:]
			pq, pqErr := shuttlecrypto.NewPQHandshake()
			if pqErr != nil {
				s.logger.Error("pq handshake init failed", "err", pqErr)
				raw.Close()
				return
			}

			pqSecret, ciphertext, pqErr := pq.Encapsulate(pqPubBytes)
			if pqErr != nil {
				s.logger.Error("pq encapsulate failed", "err", pqErr)
				raw.Close()
				return
			}

			if pqErr := writeFrame(raw, ciphertext); pqErr != nil {
				s.logger.Error("pq ciphertext send failed", "err", pqErr)
				raw.Close()
				return
			}

			var wrapErr error
			raw, wrapErr = wrapConnWithPQ(raw, pqSecret)
			if wrapErr != nil {
				s.logger.Error("pq wrap failed", "err", wrapErr)
				raw.Close()
				return
			}
			s.logger.Debug("reality pq exchange complete", "peer", fmt.Sprintf("%x", hs.PeerPublicKey()))

		default:
			s.logger.Warn("reality pq detect: unexpected prefix", "peek", fmt.Sprintf("%x", peek))
			raw.Close()
			return
		}
	}
```

Add `"encoding/binary"` import if not already present.

#### Static Assertion: PQ Payload Size > 255

Add to `transport/reality/pq_compat.go` (new file):

```go
package reality

import (
	shuttlecrypto "github.com/shuttleX/shuttle/crypto"
)

// pqPayloadMinSize guards the PQ-vs-yamux detection heuristic in the server.
// The detection relies on the PQ frame's 2-byte length prefix having a
// non-zero high byte, which is true as long as the PQ payload is > 255 bytes.
// X25519 (32) + ML-KEM-768 public key (1184) + 1 version byte = 1217 bytes,
// so the high byte is 0x04. If this ever drops to ≤ 255, the detection
// ambiguity must be resolved with explicit framing.
const pqPayloadMinSize = 256

func init() {
	// A compile-time-ish check that the PQ public key is large enough.
	// We use a runtime panic in init() because Go doesn't have const-generic
	// assertions. This runs on package import; if it ever fails, it fails
	// loudly on startup.
	kp, err := shuttlecrypto.NewPQHandshake()
	if err != nil {
		return // crypto unavailable; skip (test environment)
	}
	size := len(kp.PublicKeyBytes())
	if size+1 < pqPayloadMinSize {
		panic("reality: PQ payload size shrank below detection threshold; revisit peek-detect logic")
	}
}
```

## Testing Strategy

### Unit Tests: `peekConn`

`transport/reality/peekconn_test.go`:

- `TestPeekConn_ReplaysPrefix` — wrap a `net.Pipe` conn with `peekConn{prefix: [0xAA, 0xBB]}`, read 4 bytes, verify first 2 are prefix and next 2 come from the pipe.
- `TestPeekConn_PartialRead` — read 1 byte at a time, verify correct order.
- `TestPeekConn_NilPrefixFallsThrough` — empty prefix, reads hit underlying conn directly.

### Unit Test: PQ vs Yamux Detection (in-memory)

`transport/reality/pq_compat_test.go`:

- `TestPQServerAcceptsClassicalClient`
  - Use `net.Pipe()` to construct a pair of in-memory conns.
  - Spin up a goroutine that acts as the "classical client": writes yamux-shaped bytes (`0x00 0x00 0x00 ...`) directly into its side of the pipe.
  - On the other side, manually invoke the PQ-detect logic (extract into a helper `detectAndHandlePQ(raw, pqEnabled bool) (net.Conn, error)` for testability).
  - Verify: returned conn's first read returns the exact yamux bytes the client wrote, in order.

- `TestPQServerAcceptsPQClient`
  - Same setup. Client writes a real PQ frame (use `shuttlecrypto.NewPQHandshake()` to get a real pub key, wrap in length prefix).
  - Server-side `detectAndHandlePQ` runs the full exchange, writes ciphertext back, wraps with `wrapConnWithPQ`.
  - Client reads ciphertext, decapsulates, wraps its side with `wrapConnWithPQ`, does a round-trip "hello" message, verifies decryption.

- `TestPQServerRejectsGarbage`
  - Client writes `[0x01, 0x02, 0x03]` (not yamux, not PQ).
  - Server closes with error. Verify the conn is closed.

### Refactor for Testability

Extract the detect-and-handle block into a helper:

```go
// detectAndHandlePQ peeks the first 3 bytes of raw to distinguish a
// classical yamux client from a PQ-enabled client. It returns a net.Conn
// that yamux should use. For PQ clients it performs the full exchange
// and wraps the connection. For classical clients it returns raw with the
// peeked bytes replayed via peekConn.
func (s *Server) detectAndHandlePQ(raw net.Conn, hs *shuttlecrypto.NoiseState) (net.Conn, error) {
    // ... implementation as above
}
```

This keeps `handleConn` readable and makes the PQ branch unit-testable in isolation.

### Sandbox Integration Test

`transport/reality/reality_pq_compat_sandbox_test.go` (`//go:build sandbox`):

- Start a Reality server with `PostQuantum: true`.
- Connect a classical client (`PostQuantum: false`) — expect a successful yamux handshake and successful bidirectional stream.
- Connect a PQ client (`PostQuantum: true`) — expect a successful PQ exchange + yamux and successful bidirectional stream.
- Both clients running simultaneously — verify no interference.

### Regression: Existing Tests

Run the full reality test suite:

- `transport/reality/reality_test.go`
- `transport/reality/pq_test.go`
- `transport/reality/pqwrap_test.go`

All must continue to pass.

## Rollout

Single PR, server-only change. No config changes. No client-side changes. No wire protocol modification.

After merge:
- Users with `post_quantum: false` (default) see no behavior change.
- Users with `post_quantum: true` who previously had PQ-capable clients see no change — PQ exchange still runs.
- Users with `post_quantum: true` who had classical clients getting dropped now see their clients connect successfully — the classical clients do NOT get PQ protection, but they stay connected.

**Logging**: Classical connections to a PQ-enabled server log at Debug level "reality pq-server accepting classical client" so operators can audit how many clients are falling through the classical path and decide whether to nudge them to upgrade.

## Risks

- **Detection false positives**: A corrupt or truncated yamux stream might by chance have `peek[2] == 0x02`. Probability: 1/256 for random noise. Impact: server attempts PQ decapsulation with garbage, fails, closes. No security implication. Client retries.
- **Detection false negatives**: A PQ frame whose 2-byte length prefix has high byte `0x00` would be misread as yamux. This requires PQ payload < 256 bytes, impossible with ML-KEM-768. Guarded by the `pqPayloadMinSize` init-time check.
- **Read deadline**: 5 seconds for PQ peek is identical to the old timeout. No change.
- **Peek byte ordering**: yamux v0 guarantees `header[0] == 0` (protocol version). This is part of the yamux spec, not an implementation detail. Stable dependency.
- **Backward-future compat**: if the Reality handshake ever adds more post-Noise pre-yamux stages, the peek-detection needs extension. Document in the comment block.

## Open Questions

1. **Should classical clients against a PQ server be refused via config opt-in?** — Operators who want strict PQ-only mode (no downgrade) could set `post_quantum: true, require_pq: true`. Out of scope for this spec; would be a follow-up. The default is graceful fallback.
2. **Should we emit a metric for classical fallbacks?** — Not in this spec. Add if operators ask for it.
3. **Why not use a magic prefix (`0x52 0x51` "RQ") to eliminate the length-based heuristic?** — That's a wire protocol change requiring client updates. This spec deliberately avoids client changes to make rollout zero-risk for existing deployments.
