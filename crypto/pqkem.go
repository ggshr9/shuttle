package crypto

import (
	"crypto/ecdh"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

// Key sizes for ML-KEM-768 wire compatibility.
const (
	mlkemPublicKeySize  = mlkem.EncapsulationKeySize768 // 1184 bytes
	mlkemPrivateKeySize = mlkem.SeedSize                // 64 bytes (seed for key derivation)
	mlkemCiphertextSize = mlkem.CiphertextSize768       // 1088 bytes
	mlkemSharedKeySize  = mlkem.SharedKeySize           // 32 bytes

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
	PQPrivate        []byte                  // ML-KEM-768 seed (64 bytes), used for serialization
	PQPublic         []byte                  // ML-KEM-768 encapsulation key (1184 bytes)
	pqDecapKey       *mlkem.DecapsulationKey768 // live decapsulation key (nil when parsed from pubkey-only)
}

// HybridKEM provides hybrid X25519 + ML-KEM-768 post-quantum key exchange.
//
// The shared secret is computed as:
//
//	SHA-256(x25519_shared || mlkem_shared)
//
// This provides classical security from X25519 and post-quantum security from
// ML-KEM-768 (NIST FIPS 203). An attacker must break both algorithms to recover
// the shared secret.
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

	// Generate classical X25519 key pair.
	classicalPriv, err := ecdh.X25519().GenerateKey(rng)
	if err != nil {
		return nil, fmt.Errorf("generate X25519 key: %w", err)
	}

	// Generate ML-KEM-768 key pair.
	pqDecapKey, err := mlkem.GenerateKey768()
	if err != nil {
		return nil, fmt.Errorf("generate ML-KEM-768 key: %w", err)
	}

	return &HybridKEMKeyPair{
		ClassicalPrivate: classicalPriv,
		ClassicalPublic:  classicalPriv.PublicKey(),
		PQPrivate:        pqDecapKey.Bytes(), // 64-byte seed
		PQPublic:         pqDecapKey.EncapsulationKey().Bytes(),
		pqDecapKey:       pqDecapKey,
	}, nil
}

// Encapsulate performs both X25519 DH and ML-KEM-768 encapsulation.
// Returns: (combined_shared_secret, encapsulated_ciphertext, error)
//
// combined_shared_secret = SHA-256(x25519_shared || mlkem_shared)
// ciphertext format: [32-byte x25519 ephemeral pubkey || 1088-byte mlkem ciphertext]
func (h *HybridKEM) Encapsulate(peerPublic *HybridKEMKeyPair) (sharedSecret [32]byte, ciphertext []byte, err error) {
	if peerPublic == nil || peerPublic.ClassicalPublic == nil {
		return sharedSecret, nil, errors.New("pqkem: nil peer public key")
	}
	if len(peerPublic.PQPublic) != mlkemPublicKeySize {
		return sharedSecret, nil, fmt.Errorf("pqkem: PQ public key wrong size: got %d, want %d", len(peerPublic.PQPublic), mlkemPublicKeySize)
	}

	// Classical X25519: generate ephemeral key pair and perform DH.
	ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return sharedSecret, nil, fmt.Errorf("pqkem: generate ephemeral key: %w", err)
	}

	x25519Shared, err := ephemeral.ECDH(peerPublic.ClassicalPublic)
	if err != nil {
		return sharedSecret, nil, fmt.Errorf("pqkem: X25519 ECDH: %w", err)
	}

	// ML-KEM-768 encapsulation.
	pqEncapKey, err := mlkem.NewEncapsulationKey768(peerPublic.PQPublic)
	if err != nil {
		return sharedSecret, nil, fmt.Errorf("pqkem: parse ML-KEM-768 encapsulation key: %w", err)
	}

	mlkemShared, mlkemCiphertext := pqEncapKey.Encapsulate()

	// Build ciphertext: [32-byte x25519 ephemeral pubkey || 1088-byte mlkem ciphertext]
	ciphertext = make([]byte, HybridCiphertextSize)
	copy(ciphertext[:x25519PublicKeySize], ephemeral.PublicKey().Bytes())
	copy(ciphertext[x25519PublicKeySize:], mlkemCiphertext)

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

	// Extract ephemeral X25519 public key from ciphertext.
	ephPubBytes := ciphertext[:x25519PublicKeySize]
	ephPub, err := ecdh.X25519().NewPublicKey(ephPubBytes)
	if err != nil {
		return sharedSecret, fmt.Errorf("pqkem: parse ephemeral public key: %w", err)
	}

	// Classical X25519 DH with ephemeral public key.
	x25519Shared, err := privKey.ClassicalPrivate.ECDH(ephPub)
	if err != nil {
		return sharedSecret, fmt.Errorf("pqkem: X25519 ECDH decapsulate: %w", err)
	}

	// ML-KEM-768 decapsulation.
	pqDecapKey, err := privKey.resolveDecapKey()
	if err != nil {
		return sharedSecret, fmt.Errorf("pqkem: resolve ML-KEM-768 decapsulation key: %w", err)
	}

	mlkemCiphertextBytes := ciphertext[x25519PublicKeySize : x25519PublicKeySize+mlkemCiphertextSize]
	mlkemShared, err := pqDecapKey.Decapsulate(mlkemCiphertextBytes)
	if err != nil {
		return sharedSecret, fmt.Errorf("pqkem: ML-KEM-768 decapsulate: %w", err)
	}

	// Combine shared secrets: SHA-256(x25519_shared || mlkem_shared)
	combined := sha256.New()
	combined.Write(x25519Shared)
	combined.Write(mlkemShared)
	copy(sharedSecret[:], combined.Sum(nil))

	return sharedSecret, nil
}

// resolveDecapKey returns the live DecapsulationKey768, reconstructing from
// the stored seed if the in-memory pointer was not set (e.g. after deserialization).
func (kp *HybridKEMKeyPair) resolveDecapKey() (*mlkem.DecapsulationKey768, error) {
	if kp.pqDecapKey != nil {
		return kp.pqDecapKey, nil
	}
	if len(kp.PQPrivate) != mlkemPrivateKeySize {
		return nil, fmt.Errorf("PQ private key seed wrong size: got %d, want %d", len(kp.PQPrivate), mlkemPrivateKeySize)
	}
	dk, err := mlkem.NewDecapsulationKey768(kp.PQPrivate)
	if err != nil {
		return nil, fmt.Errorf("reconstruct ML-KEM-768 key from seed: %w", err)
	}
	return dk, nil
}

// PublicKeyBytes serializes the public keys for transmission.
// Format: [32-byte x25519 pubkey || 1184-byte mlkem encapsulation key]
func PublicKeyBytes(kp *HybridKEMKeyPair) []byte {
	if kp == nil || kp.ClassicalPublic == nil {
		return nil
	}
	out := make([]byte, HybridPublicKeySize)
	copy(out[:x25519PublicKeySize], kp.ClassicalPublic.Bytes())
	copy(out[x25519PublicKeySize:], kp.PQPublic)
	return out
}

// GenerateHybridKEMKeypair is a package-level convenience function that generates
// a hybrid X25519 + ML-KEM-768 key pair.
// Returns (publicKey, privateKey, error) where both are *HybridKEMKeyPair.
// The public key contains only the public fields; the private key contains all fields.
func GenerateHybridKEMKeypair() (pub *HybridKEMKeyPair, priv *HybridKEMKeyPair, err error) {
	kem := NewHybridKEM()
	kp, err := kem.GenerateKeyPair(nil)
	if err != nil {
		return nil, nil, err
	}
	// Public key view: only public fields
	pub = &HybridKEMKeyPair{
		ClassicalPublic: kp.ClassicalPublic,
		PQPublic:        kp.PQPublic,
	}
	return pub, kp, nil
}

// HybridEncapsulate is a package-level convenience function.
// Returns (ciphertext, sharedSecret, error).
func HybridEncapsulate(peerPublic *HybridKEMKeyPair) ([]byte, []byte, error) {
	kem := NewHybridKEM()
	shared, ct, err := kem.Encapsulate(peerPublic)
	if err != nil {
		return nil, nil, err
	}
	return ct, shared[:], nil
}

// HybridDecapsulate is a package-level convenience function.
// Returns (sharedSecret, error).
func HybridDecapsulate(privKey *HybridKEMKeyPair, ciphertext []byte) ([]byte, error) {
	kem := NewHybridKEM()
	shared, err := kem.Decapsulate(privKey, ciphertext)
	if err != nil {
		return nil, err
	}
	return shared[:], nil
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
