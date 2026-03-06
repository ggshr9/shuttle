package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"io"
)

// GenerateHMAC creates a [32-byte nonce][32-byte HMAC-SHA256(password, nonce)] payload.
func GenerateHMAC(password string) ([64]byte, error) {
	var payload [64]byte
	if _, err := io.ReadFull(rand.Reader, payload[:32]); err != nil {
		return payload, err
	}
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write(payload[:32])
	copy(payload[32:], mac.Sum(nil))
	return payload, nil
}

// VerifyHMAC checks that mac == HMAC-SHA256(password, nonce).
func VerifyHMAC(nonce, mac []byte, password string) bool {
	h := hmac.New(sha256.New, []byte(password))
	h.Write(nonce)
	expected := h.Sum(nil)
	return hmac.Equal(mac, expected)
}
