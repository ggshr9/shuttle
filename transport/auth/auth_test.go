package auth

import (
	"testing"
)

func TestHMACGenerateVerify(t *testing.T) {
	password := "test-password-123"

	payload, err := GenerateHMAC(password)
	if err != nil {
		t.Fatalf("GenerateHMAC: %v", err)
	}

	nonce := payload[:32]
	mac := payload[32:]

	if !VerifyHMAC(nonce, mac, password) {
		t.Fatal("VerifyHMAC should return true for correct password")
	}
}

func TestHMACVerifyWrongPassword(t *testing.T) {
	payload, err := GenerateHMAC("correct-password")
	if err != nil {
		t.Fatalf("GenerateHMAC: %v", err)
	}

	nonce := payload[:32]
	mac := payload[32:]

	if VerifyHMAC(nonce, mac, "wrong-password") {
		t.Fatal("VerifyHMAC should return false for wrong password")
	}
}

func TestHMACVerifyTampered(t *testing.T) {
	password := "my-password"
	payload, err := GenerateHMAC(password)
	if err != nil {
		t.Fatalf("GenerateHMAC: %v", err)
	}

	nonce := payload[:32]
	mac := make([]byte, 32)
	copy(mac, payload[32:])

	// Flip a bit in the MAC
	mac[0] ^= 0xFF

	if VerifyHMAC(nonce, mac, password) {
		t.Fatal("VerifyHMAC should return false for tampered MAC")
	}

	// Also tamper the nonce
	nonce2 := make([]byte, 32)
	copy(nonce2, payload[:32])
	nonce2[0] ^= 0xFF

	if VerifyHMAC(nonce2, payload[32:], password) {
		t.Fatal("VerifyHMAC should return false for tampered nonce")
	}
}
