package test

import (
	"bytes"
	"testing"

	"github.com/shuttleX/shuttle/crypto"
)

func TestEncryptDecrypt(t *testing.T) {
	key, err := crypto.DeriveKeys([]byte("test-key-material"), 32)
	if err != nil {
		t.Fatalf("derive keys: %v", err)
	}
	plaintext := []byte("hello, shuttle!")

	ciphertext, err := crypto.Encrypt(key, nil, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	decrypted, err := crypto.Decrypt(key, nil, ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decrypted mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestStreamCipher(t *testing.T) {
	var key [32]byte
	derived, err := crypto.DeriveKeys([]byte("stream-key"), 32)
	if err != nil {
		t.Fatalf("derive keys: %v", err)
	}
	copy(key[:], derived)

	enc, err := crypto.NewStreamCipher(key, crypto.CipherChaChaPoly)
	if err != nil {
		t.Fatalf("new stream cipher: %v", err)
	}
	dec, err := crypto.NewStreamCipher(key, crypto.CipherChaChaPoly)
	if err != nil {
		t.Fatalf("new stream cipher: %v", err)
	}

	messages := []string{"hello", "world", "shuttle proxy"}
	for _, msg := range messages {
		ct := enc.Seal([]byte(msg))
		pt, err := dec.Open(ct)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if string(pt) != msg {
			t.Errorf("got %q, want %q", pt, msg)
		}
	}
}

func TestReplayFilter(t *testing.T) {
	rf := crypto.NewReplayFilter(0)

	// First use: not a replay
	if rf.Check(12345) {
		t.Error("first use should not be flagged as replay")
	}

	// Second use: replay detected
	if !rf.Check(12345) {
		t.Error("second use should be flagged as replay")
	}

	// Different nonce: not a replay
	if rf.Check(67890) {
		t.Error("different nonce should not be flagged as replay")
	}

	if rf.Size() != 2 {
		t.Errorf("expected size 2, got %d", rf.Size())
	}
}

func TestDeriveKeys(t *testing.T) {
	key1, err := crypto.DeriveKeys([]byte("material-a"), 32)
	if err != nil {
		t.Fatalf("derive keys: %v", err)
	}
	key2, err := crypto.DeriveKeys([]byte("material-b"), 32)
	if err != nil {
		t.Fatalf("derive keys: %v", err)
	}
	key3, err := crypto.DeriveKeys([]byte("material-a"), 32)
	if err != nil {
		t.Fatalf("derive keys: %v", err)
	}

	if bytes.Equal(key1, key2) {
		t.Error("different materials should produce different keys")
	}
	if !bytes.Equal(key1, key3) {
		t.Error("same material should produce same keys")
	}
}

func TestAutoSelectCipher(t *testing.T) {
	ct := crypto.AutoSelectCipher()
	// Should return a valid cipher type
	if ct != crypto.CipherChaChaPoly && ct != crypto.CipherAESGCM {
		t.Errorf("unexpected cipher type: %d", ct)
	}
	t.Logf("auto-selected cipher: %d", ct)
}
