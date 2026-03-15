package crypto

import (
	"testing"
)

// TestNoiseHandshake performs a full initiator/responder round-trip using Noise IK.
func TestNoiseHandshake(t *testing.T) {
	// Generate key pairs for both sides.
	iPub, iPriv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate initiator keys: %v", err)
	}
	rPub, rPriv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate responder keys: %v", err)
	}

	// Create handshake states. Initiator knows the responder's public key (IK pattern).
	initiator, err := NewInitiator(iPriv, iPub, rPub)
	if err != nil {
		t.Fatalf("NewInitiator: %v", err)
	}
	responder, err := NewResponder(rPriv, rPub)
	if err != nil {
		t.Fatalf("NewResponder: %v", err)
	}

	// Step 1: Initiator writes first message (-> e, es, s, ss)
	msg1, err := initiator.WriteMessage(nil)
	if err != nil {
		t.Fatalf("initiator WriteMessage: %v", err)
	}
	if len(msg1) == 0 {
		t.Fatal("msg1 is empty")
	}

	// Step 2: Responder reads first message
	_, err = responder.ReadMessage(msg1)
	if err != nil {
		t.Fatalf("responder ReadMessage(msg1): %v", err)
	}

	// Responder writes second message (<- e, ee, se)
	msg2, err := responder.WriteMessage(nil)
	if err != nil {
		t.Fatalf("responder WriteMessage: %v", err)
	}
	if len(msg2) == 0 {
		t.Fatal("msg2 is empty")
	}

	// Step 3: Initiator reads second message
	_, err = initiator.ReadMessage(msg2)
	if err != nil {
		t.Fatalf("initiator ReadMessage(msg2): %v", err)
	}

	// Both sides should be completed.
	if !initiator.Completed() {
		t.Fatal("initiator handshake not completed")
	}
	if !responder.Completed() {
		t.Fatal("responder handshake not completed")
	}

	// Both sides should have non-nil cipher states.
	if initiator.SendCipher() == nil {
		t.Fatal("initiator SendCipher is nil")
	}
	if initiator.RecvCipher() == nil {
		t.Fatal("initiator RecvCipher is nil")
	}
	if responder.SendCipher() == nil {
		t.Fatal("responder SendCipher is nil")
	}
	if responder.RecvCipher() == nil {
		t.Fatal("responder RecvCipher is nil")
	}

	// Verify encrypted data round-trip: initiator encrypts, responder decrypts.
	plaintext := []byte("hello noise")
	encrypted, err := initiator.SendCipher().Encrypt(nil, nil, plaintext)
	if err != nil {
		t.Fatalf("initiator encrypt failed: %v", err)
	}
	decrypted, err := responder.RecvCipher().Decrypt(nil, nil, encrypted)
	if err != nil {
		t.Fatalf("responder decrypt failed: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted mismatch: got %q, want %q", decrypted, plaintext)
	}

	// Reverse direction: responder encrypts, initiator decrypts.
	plaintext2 := []byte("hello back")
	encrypted2, err := responder.SendCipher().Encrypt(nil, nil, plaintext2)
	if err != nil {
		t.Fatalf("responder encrypt failed: %v", err)
	}
	decrypted2, err := initiator.RecvCipher().Decrypt(nil, nil, encrypted2)
	if err != nil {
		t.Fatalf("initiator decrypt failed: %v", err)
	}
	if string(decrypted2) != string(plaintext2) {
		t.Fatalf("decrypted2 mismatch: got %q, want %q", decrypted2, plaintext2)
	}

	// Responder should see the initiator's public key.
	peerKey := responder.PeerPublicKey()
	if len(peerKey) == 0 {
		t.Fatal("responder PeerPublicKey is empty")
	}
}

// TestNoiseHandshakeWrongKey verifies that an initiator with the wrong remote
// public key fails during the handshake.
func TestNoiseHandshakeWrongKey(t *testing.T) {
	_, iPriv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate initiator keys: %v", err)
	}
	iPub, _, err := GenerateKeyPair() // use a different pub for initiator identity
	if err != nil {
		t.Fatalf("generate initiator pub: %v", err)
	}
	rPub, rPriv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate responder keys: %v", err)
	}
	wrongPub, _, err := GenerateKeyPair() // wrong key the initiator thinks is the responder
	if err != nil {
		t.Fatalf("generate wrong keys: %v", err)
	}

	// Initiator uses the wrong remote public key.
	initiator, err := NewInitiator(iPriv, iPub, wrongPub)
	if err != nil {
		t.Fatalf("NewInitiator: %v", err)
	}
	responder, err := NewResponder(rPriv, rPub)
	if err != nil {
		t.Fatalf("NewResponder: %v", err)
	}

	msg1, err := initiator.WriteMessage(nil)
	if err != nil {
		t.Fatalf("initiator WriteMessage: %v", err)
	}

	// Responder should fail to read because the initiator encrypted to the wrong static key.
	_, err = responder.ReadMessage(msg1)
	if err == nil {
		t.Fatal("expected error when initiator uses wrong remote public key, but got nil")
	}
}

// TestDeriveKeysFromPassword verifies that the same password produces
// the same keys deterministically.
func TestDeriveKeysFromPassword(t *testing.T) {
	password := "test-password-123"

	pub1, priv1, err := DeriveKeysFromPassword(password)
	if err != nil {
		t.Fatalf("DeriveKeysFromPassword(1): %v", err)
	}

	pub2, priv2, err := DeriveKeysFromPassword(password)
	if err != nil {
		t.Fatalf("DeriveKeysFromPassword(2): %v", err)
	}

	if pub1 != pub2 {
		t.Fatal("same password should produce same public key")
	}
	if priv1 != priv2 {
		t.Fatal("same password should produce same private key")
	}

	// Keys should not be all zeros.
	var zero [32]byte
	if pub1 == zero {
		t.Fatal("public key is all zeros")
	}
	if priv1 == zero {
		t.Fatal("private key is all zeros")
	}
}

// TestDeriveKeysFromPasswordDifferent verifies that different passwords
// produce different keys.
func TestDeriveKeysFromPasswordDifferent(t *testing.T) {
	pub1, priv1, err := DeriveKeysFromPassword("password-alpha")
	if err != nil {
		t.Fatalf("DeriveKeysFromPassword(alpha): %v", err)
	}

	pub2, priv2, err := DeriveKeysFromPassword("password-beta")
	if err != nil {
		t.Fatalf("DeriveKeysFromPassword(beta): %v", err)
	}

	if pub1 == pub2 {
		t.Fatal("different passwords should produce different public keys")
	}
	if priv1 == priv2 {
		t.Fatal("different passwords should produce different private keys")
	}
}
