// Package shadowsocks implements the Shadowsocks protocol with AEAD encryption.
package shadowsocks

import (
	"fmt"
	"net"

	"github.com/shadowsocks/go-shadowsocks2/core"
)

// NewCipher creates an AEAD cipher for the given method and password.
// Supported methods: aes-128-gcm, aes-256-gcm, chacha20-ietf-poly1305.
func NewCipher(method, password string) (core.Cipher, error) {
	switch method {
	case "aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305":
		return core.PickCipher(method, nil, password)
	default:
		return nil, fmt.Errorf("shadowsocks: unsupported method: %s", method)
	}
}

// WrapConn wraps a net.Conn with AEAD stream encryption.
func WrapConn(conn net.Conn, ciph core.Cipher) net.Conn {
	return ciph.StreamConn(conn)
}
