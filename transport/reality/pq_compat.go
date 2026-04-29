package reality

import (
	shuttlecrypto "github.com/ggshr9/shuttle/crypto"
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
