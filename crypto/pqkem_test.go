package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestHybridKEMKeyGeneration(t *testing.T) {
	kem := NewHybridKEM()
	kp, err := kem.GenerateKeyPair(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	if kp.ClassicalPrivate == nil {
		t.Fatal("ClassicalPrivate is nil")
	}
	if kp.ClassicalPublic == nil {
		t.Fatal("ClassicalPublic is nil")
	}
	if len(kp.ClassicalPublic.Bytes()) != 32 {
		t.Fatalf("ClassicalPublic size = %d, want 32", len(kp.ClassicalPublic.Bytes()))
	}
	if len(kp.PQPublic) != mlkemPublicKeySize {
		t.Fatalf("PQPublic size = %d, want %d", len(kp.PQPublic), mlkemPublicKeySize)
	}
	if len(kp.PQPrivate) != mlkemPrivateKeySize {
		t.Fatalf("PQPrivate size = %d, want %d", len(kp.PQPrivate), mlkemPrivateKeySize)
	}
}

func TestHybridKEMEncapsulateDecapsulate(t *testing.T) {
	kem := NewHybridKEM()

	// Generate key pair for the "receiver"
	receiver, err := kem.GenerateKeyPair(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKeyPair (receiver) failed: %v", err)
	}

	// Sender encapsulates using receiver's public key
	senderSecret, ciphertext, err := kem.Encapsulate(receiver)
	if err != nil {
		t.Fatalf("Encapsulate failed: %v", err)
	}

	// Verify ciphertext size
	if len(ciphertext) != HybridCiphertextSize {
		t.Fatalf("ciphertext size = %d, want %d", len(ciphertext), HybridCiphertextSize)
	}

	// Receiver decapsulates
	receiverSecret, err := kem.Decapsulate(receiver, ciphertext)
	if err != nil {
		t.Fatalf("Decapsulate failed: %v", err)
	}

	// Shared secrets must match
	if senderSecret != receiverSecret {
		t.Fatalf("shared secrets do not match:\n  sender:   %x\n  receiver: %x", senderSecret, receiverSecret)
	}

	// Shared secret must not be all zeros
	var zero [32]byte
	if senderSecret == zero {
		t.Fatal("shared secret is all zeros")
	}
}

func TestHybridKEMDifferentKeys(t *testing.T) {
	kem := NewHybridKEM()

	kp1, err := kem.GenerateKeyPair(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKeyPair 1 failed: %v", err)
	}
	kp2, err := kem.GenerateKeyPair(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKeyPair 2 failed: %v", err)
	}

	// Encapsulate to kp1
	secret1, ct1, err := kem.Encapsulate(kp1)
	if err != nil {
		t.Fatalf("Encapsulate to kp1 failed: %v", err)
	}

	// Encapsulate to kp2
	secret2, ct2, err := kem.Encapsulate(kp2)
	if err != nil {
		t.Fatalf("Encapsulate to kp2 failed: %v", err)
	}

	// Secrets for different recipients should differ
	if secret1 == secret2 {
		t.Fatal("secrets for different key pairs should not be equal")
	}

	// Ciphertexts for different recipients should differ
	if bytes.Equal(ct1, ct2) {
		t.Fatal("ciphertexts for different key pairs should not be equal")
	}

	// Decapsulating ct1 with kp2 should produce a different secret
	wrongSecret, err := kem.Decapsulate(kp2, ct1)
	if err != nil {
		// Depending on implementation, this might error or produce wrong secret.
		// Either outcome is acceptable for security.
		return
	}
	if wrongSecret == secret1 {
		t.Fatal("decapsulating with wrong key should not produce matching secret")
	}
}

func TestHybridKEMPublicKeySerialization(t *testing.T) {
	kem := NewHybridKEM()
	kp, err := kem.GenerateKeyPair(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	// Serialize
	pubBytes := PublicKeyBytes(kp)
	if len(pubBytes) != HybridPublicKeySize {
		t.Fatalf("serialized public key size = %d, want %d", len(pubBytes), HybridPublicKeySize)
	}

	// Parse
	parsed, err := ParsePublicKey(pubBytes)
	if err != nil {
		t.Fatalf("ParsePublicKey failed: %v", err)
	}

	// Verify classical public key matches
	if !bytes.Equal(parsed.ClassicalPublic.Bytes(), kp.ClassicalPublic.Bytes()) {
		t.Fatal("parsed classical public key does not match original")
	}

	// Verify PQ public key matches
	if !bytes.Equal(parsed.PQPublic, kp.PQPublic) {
		t.Fatal("parsed PQ public key does not match original")
	}

	// Round-trip: encapsulate with parsed key, decapsulate with original private key
	secret1, ct, err := kem.Encapsulate(parsed)
	if err != nil {
		t.Fatalf("Encapsulate with parsed key failed: %v", err)
	}
	secret2, err := kem.Decapsulate(kp, ct)
	if err != nil {
		t.Fatalf("Decapsulate after round-trip failed: %v", err)
	}
	if secret1 != secret2 {
		t.Fatal("round-trip serialization broke key exchange")
	}
}

func TestHybridKEMBadCiphertext(t *testing.T) {
	kem := NewHybridKEM()
	kp, err := kem.GenerateKeyPair(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	// Generate valid ciphertext
	originalSecret, ciphertext, err := kem.Encapsulate(kp)
	if err != nil {
		t.Fatalf("Encapsulate failed: %v", err)
	}

	// Tamper with ciphertext (flip bits in the classical ephemeral key area)
	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[0] ^= 0xFF
	tampered[1] ^= 0xFF

	// Decapsulate with tampered ciphertext — should either error or produce wrong secret
	tamperedSecret, err := kem.Decapsulate(kp, tampered)
	if err != nil {
		// Error is acceptable
		return
	}
	if tamperedSecret == originalSecret {
		t.Fatal("tampered ciphertext should not produce the same shared secret")
	}
}

func TestHybridKEMShortCiphertext(t *testing.T) {
	kem := NewHybridKEM()
	kp, err := kem.GenerateKeyPair(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	// Try various truncated ciphertexts
	truncated := []int{0, 16, 31, 32, 33, 64, HybridCiphertextSize - 1}
	for _, size := range truncated {
		ct := make([]byte, size)
		_, err := kem.Decapsulate(kp, ct)
		if err == nil {
			t.Errorf("Decapsulate with %d-byte ciphertext should have failed", size)
		}
	}
}

func TestHybridKEMNilInputs(t *testing.T) {
	kem := NewHybridKEM()

	// Encapsulate with nil peer
	_, _, err := kem.Encapsulate(nil)
	if err == nil {
		t.Error("Encapsulate with nil peer should fail")
	}

	// Decapsulate with nil private key
	ct := make([]byte, HybridCiphertextSize)
	_, err = kem.Decapsulate(nil, ct)
	if err == nil {
		t.Error("Decapsulate with nil private key should fail")
	}

	// ParsePublicKey with short data
	_, err = ParsePublicKey([]byte{1, 2, 3})
	if err == nil {
		t.Error("ParsePublicKey with short data should fail")
	}
}
