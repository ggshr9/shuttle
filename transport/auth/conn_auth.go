package auth

import (
	"fmt"
	"io"
	"net"

	"github.com/ggshr9/shuttle/adapter"
)

// HMACAuthenticator implements adapter.Authenticator using HMAC-SHA256.
// Wire format: [32-byte nonce][32-byte HMAC-SHA256(password, nonce)].
type HMACAuthenticator struct {
	password string
}

// NewHMACAuthenticator creates an HMAC-based authenticator.
func NewHMACAuthenticator(password string) *HMACAuthenticator {
	return &HMACAuthenticator{password: password}
}

func (a *HMACAuthenticator) AuthClient(conn net.Conn) error {
	payload, err := GenerateHMAC(a.password)
	if err != nil {
		return fmt.Errorf("hmac auth generate: %w", err)
	}
	if _, err := conn.Write(payload[:]); err != nil {
		return fmt.Errorf("hmac auth write: %w", err)
	}
	return nil
}

func (a *HMACAuthenticator) AuthServer(conn net.Conn) (string, error) {
	var payload [64]byte
	if _, err := io.ReadFull(conn, payload[:]); err != nil {
		return "", fmt.Errorf("hmac auth read: %w", err)
	}
	nonce := payload[:32]
	mac := payload[32:]
	if !VerifyHMAC(nonce, mac, a.password) {
		return "", fmt.Errorf("hmac auth: verification failed")
	}
	return "", nil
}

var _ adapter.Authenticator = (*HMACAuthenticator)(nil)
