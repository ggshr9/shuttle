package shadowsocks

import (
	"context"
	"fmt"
	"net"

	"github.com/shadowsocks/go-shadowsocks2/core"

	"github.com/shuttleX/shuttle/transport/shared"
)

// DialerConfig holds configuration for a Shadowsocks client dialer.
type DialerConfig struct {
	Server   string // host:port of the SS server
	Method   string // AEAD method (aes-256-gcm, chacha20-ietf-poly1305, etc.)
	Password string
}

// Dialer connects to a Shadowsocks server and tunnels traffic through it.
type Dialer struct {
	server string
	cipher core.Cipher
}

// NewDialer creates a Dialer from the given config.
func NewDialer(cfg DialerConfig) (*Dialer, error) {
	ciph, err := NewCipher(cfg.Method, cfg.Password)
	if err != nil {
		return nil, fmt.Errorf("shadowsocks/dialer: %w", err)
	}
	return &Dialer{server: cfg.Server, cipher: ciph}, nil
}

// DialContext dials through the Shadowsocks server to the given target address.
// The returned net.Conn is already AEAD-encrypted; callers read/write plaintext.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// 1. TCP connect to the SS server.
	var dialer net.Dialer
	raw, err := dialer.DialContext(ctx, "tcp", d.server)
	if err != nil {
		return nil, fmt.Errorf("shadowsocks/dialer: connect server: %w", err)
	}

	// 2. Wrap with AEAD encryption.
	conn := WrapConn(raw, d.cipher)

	// 3. Write the SOCKS5-style target address header.
	if err := shared.EncodeAddr(conn, network, address); err != nil {
		conn.Close()
		return nil, fmt.Errorf("shadowsocks/dialer: encode addr: %w", err)
	}

	return conn, nil
}
