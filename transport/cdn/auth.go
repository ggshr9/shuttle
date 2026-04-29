package cdn

import (
	"github.com/ggshr9/shuttle/transport/auth"
)

// generateAuthPayload creates a [32-byte nonce][32-byte HMAC-SHA256(password, nonce)] payload.
func generateAuthPayload(password string) ([64]byte, error) {
	return auth.GenerateHMAC(password)
}
