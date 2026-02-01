package cdn

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"io"
)

// generateAuthPayload creates a [32-byte nonce][32-byte HMAC-SHA256(password, nonce)] payload.
func generateAuthPayload(password string) ([64]byte, error) {
	var payload [64]byte
	if _, err := io.ReadFull(rand.Reader, payload[:32]); err != nil {
		return payload, err
	}
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write(payload[:32])
	copy(payload[32:], mac.Sum(nil))
	return payload, nil
}
