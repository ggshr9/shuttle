package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

// ML-KEM-768 key sizes (used for API shape even with placeholder implementation).
const (
	mlkemPublicKeySize  = 1184 // ML-KEM-768 encapsulation key
	mlkemPrivateKeySize = 2400 // ML-KEM-768 decapsulation key
	mlkemCiphertextSize = 1088 // ML-KEM-768 ciphertext
	mlkemSharedKeySize  = 32   // ML-KEM-768 shared secret

	// x25519PublicKeySize is the size of an X25519 public key.
	x25519PublicKeySize = 32

	// HybridPublicKeySize is the total serialized public key size:
	// 32-byte X25519 + 1184-byte ML-KEM-768 encapsulation key.
	HybridPublicKeySize = x25519PublicKeySize + mlkemPublicKeySize

	// HybridCiphertextSize is the total ciphertext size:
	// 32-byte X25519 ephemeral public key + 1088-byte ML-KEM-768 ciphertext.
	HybridCiphertextSize = x25519PublicKeySize + mlkemCiphertextSize
)

// HybridKEMKeyPair holds both classical and PQ key material.
type HybridKEMKeyPair struct {
	ClassicalPrivate *ecdh.PrivateKey
	ClassicalPublic  *ecdh.PublicKey
	PQPrivate        []byte // ML-KEM-768 decapsulation key (2400 bytes)
	PQPublic         []byte // ML-KEM-768 encapsulation key (1184 bytes)
}

// HybridKEM provides hybrid X25519 + ML-KEM-768 key exchange.
//
// NOTE: This uses a second X25519 exchange as a PQ placeholder.
// Replace with ML-KEM-768 (cloudflare/circl) for production PQ security.
// The API and wire format are designed for the real ML-KEM-768 key sizes.
type HybridKEM struct{}

// NewHybridKEM creates a new HybridKEM instance.
func NewHybridKEM() *HybridKEM {
	return &HybridKEM{}
}

// GenerateKeyPair generates both X25519 and ML-KEM-768 key pairs.
func (h *HybridKEM) GenerateKeyPair(rng io.Reader) (*HybridKEMKeyPair, error) {
	if rng == nil {
		rng = rand.Reader
	}

	// Generate classical X25519 key pair
	classicalPriv, err := ecdh.X25519().GenerateKey(rng)
	if err != nil {
		return nil, fmt.Errorf("generate X25519 key: %w", err)
	}

	// Generate PQ key pair (placeholder: X25519 padded to ML-KEM-768 sizes)
	pqPrivECDH, err := ecdh.X25519().GenerateKey(rng)
	if err != nil {
		return nil, fmt.Errorf("generate PQ placeholder key: %w", err)
	}

	// Pad the X25519 keys to ML-KEM-768 sizes for API compatibility.
	// The real X25519 key bytes are stored at the start; the rest is zero-padded.
	pqPublic := make([]byte, mlkemPublicKeySize)
	copy(pqPublic, pqPrivECDH.PublicKey().Bytes())

	pqPrivate := make([]byte, mlkemPrivateKeySize)
	copy(pqPrivate, pqPrivECDH.Bytes())

	return &HybridKEMKeyPair{
		ClassicalPrivate: classicalPriv,
		ClassicalPublic:  classicalPriv.PublicKey(),
		PQPrivate:        pqPrivate,
		PQPublic:         pqPublic,
	}, nil
}

// Encapsulate performs both X25519 DH and ML-KEM encapsulation.
// Returns: (combined_shared_secret, encapsulated_ciphertext, error)
//
// combined_shared_secret = SHA-256(x25519_shared || mlkem_shared)
// ciphertext format: [32-byte x25519 ephemeral pubkey || mlkem ciphertext]
func (h *HybridKEM) Encapsulate(peerPublic *HybridKEMKeyPair) (sharedSecret [32]byte, ciphertext []byte, err error) {
	if peerPublic == nil || peerPublic.ClassicalPublic == nil {
		return sharedSecret, nil, errors.New("pqkem: nil peer public key")
	}
	if len(peerPublic.PQPublic) < x25519PublicKeySize {
		return sharedSecret, nil, errors.New("pqkem: PQ public key too short")
	}

	// Classical X25519: generate ephemeral key pair and perform DH
	ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return sharedSecret, nil, fmt.Errorf("pqkem: generate ephemeral key: %w", err)
	}

	x25519Shared, err := ephemeral.ECDH(peerPublic.ClassicalPublic)
	if err != nil {
		return sharedSecret, nil, fmt.Errorf("pqkem: X25519 ECDH: %w", err)
	}

	// PQ ML-KEM encapsulation (placeholder: X25519 DH with peer's PQ public key)
	pqEphemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return sharedSecret, nil, fmt.Errorf("pqkem: generate PQ ephemeral key: %w", err)
	}

	// Extract the real X25519 public key from the padded PQ public key
	peerPQPub, err := ecdh.X25519().NewPublicKey(peerPublic.PQPublic[:x25519PublicKeySize])
	if err != nil {
		return sharedSecret, nil, fmt.Errorf("pqkem: parse PQ public key: %w", err)
	}

	mlkemShared, err := pqEphemeral.ECDH(peerPQPub)
	if err != nil {
		return sharedSecret, nil, fmt.Errorf("pqkem: PQ ECDH: %w", err)
	}

	// Build ciphertext: [32-byte x25519 ephemeral pubkey || mlkem ciphertext]
	// ML-KEM ciphertext placeholder: [32-byte pq ephemeral pubkey || zero padding to 1088 bytes]
	ciphertext = make([]byte, HybridCiphertextSize)
	copy(ciphertext[:x25519PublicKeySize], ephemeral.PublicKey().Bytes())
	copy(ciphertext[x25519PublicKeySize:], pqEphemeral.PublicKey().Bytes())
	// Remaining bytes in the ML-KEM ciphertext area are zero (padding)

	// Combine shared secrets: SHA-256(x25519_shared || mlkem_shared)
	combined := sha256.New()
	combined.Write(x25519Shared)
	combined.Write(mlkemShared)
	copy(sharedSecret[:], combined.Sum(nil))

	return sharedSecret, ciphertext, nil
}

// Decapsulate recovers the shared secret using private keys.
func (h *HybridKEM) Decapsulate(privKey *HybridKEMKeyPair, ciphertext []byte) (sharedSecret [32]byte, err error) {
	if privKey == nil || privKey.ClassicalPrivate == nil {
		return sharedSecret, errors.New("pqkem: nil private key")
	}
	if len(ciphertext) < HybridCiphertextSize {
		return sharedSecret, fmt.Errorf("pqkem: ciphertext too short: got %d, want %d", len(ciphertext), HybridCiphertextSize)
	}
	if len(privKey.PQPrivate) < x25519PublicKeySize {
		return sharedSecret, errors.New("pqkem: PQ private key too short")
	}

	// Extract ephemeral X25519 public key from ciphertext
	ephPubBytes := ciphertext[:x25519PublicKeySize]
	ephPub, err := ecdh.X25519().NewPublicKey(ephPubBytes)
	if err != nil {
		return sharedSecret, fmt.Errorf("pqkem: parse ephemeral public key: %w", err)
	}

	// Classical X25519 DH with ephemeral public key
	x25519Shared, err := privKey.ClassicalPrivate.ECDH(ephPub)
	if err != nil {
		return sharedSecret, fmt.Errorf("pqkem: X25519 ECDH decapsulate: %w", err)
	}

	// PQ ML-KEM decapsulation (placeholder: X25519 DH)
	// Extract PQ ephemeral public key from the ML-KEM ciphertext portion
	pqEphPubBytes := ciphertext[x25519PublicKeySize : x25519PublicKeySize+x25519PublicKeySize]
	pqEphPub, err := ecdh.X25519().NewPublicKey(pqEphPubBytes)
	if err != nil {
		return sharedSecret, fmt.Errorf("pqkem: parse PQ ephemeral public key: %w", err)
	}

	// Reconstruct the PQ private key from stored bytes
	pqPrivKey, err := ecdh.X25519().NewPrivateKey(privKey.PQPrivate[:x25519PublicKeySize])
	if err != nil {
		return sharedSecret, fmt.Errorf("pqkem: parse PQ private key: %w", err)
	}

	mlkemShared, err := pqPrivKey.ECDH(pqEphPub)
	if err != nil {
		return sharedSecret, fmt.Errorf("pqkem: PQ ECDH decapsulate: %w", err)
	}

	// Combine shared secrets: SHA-256(x25519_shared || mlkem_shared)
	combined := sha256.New()
	combined.Write(x25519Shared)
	combined.Write(mlkemShared)
	copy(sharedSecret[:], combined.Sum(nil))

	return sharedSecret, nil
}

// PublicKeyBytes serializes the public keys for transmission.
// Format: [32-byte x25519 pubkey || 1184-byte mlkem pubkey]
func PublicKeyBytes(kp *HybridKEMKeyPair) []byte {
	if kp == nil || kp.ClassicalPublic == nil {
		return nil
	}
	out := make([]byte, HybridPublicKeySize)
	copy(out[:x25519PublicKeySize], kp.ClassicalPublic.Bytes())
	copy(out[x25519PublicKeySize:], kp.PQPublic)
	return out
}

// ParsePublicKey deserializes public keys from the wire format.
func ParsePublicKey(data []byte) (*HybridKEMKeyPair, error) {
	if len(data) < HybridPublicKeySize {
		return nil, fmt.Errorf("pqkem: public key data too short: got %d, want %d", len(data), HybridPublicKeySize)
	}

	classicalPub, err := ecdh.X25519().NewPublicKey(data[:x25519PublicKeySize])
	if err != nil {
		return nil, fmt.Errorf("pqkem: parse X25519 public key: %w", err)
	}

	pqPublic := make([]byte, mlkemPublicKeySize)
	copy(pqPublic, data[x25519PublicKeySize:HybridPublicKeySize])

	return &HybridKEMKeyPair{
		ClassicalPublic: classicalPub,
		PQPublic:        pqPublic,
	}, nil
}
