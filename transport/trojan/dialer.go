package trojan

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/shuttleX/shuttle/transport/shared"
)

// DialerConfig configures a Trojan client dialer.
type DialerConfig struct {
	Server   string            `json:"server" yaml:"server"`
	Password string            `json:"password" yaml:"password"`
	TLS      shared.TLSOptions `json:"tls" yaml:"tls"`
}

// Dialer creates connections through a Trojan server.
type Dialer struct {
	server       string
	passwordHash string
	tlsConfig    *tls.Config
}

// NewDialer creates a Dialer from the given config.
func NewDialer(cfg *DialerConfig) (*Dialer, error) {
	if cfg.Server == "" {
		return nil, fmt.Errorf("trojan/dialer: server address required")
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("trojan/dialer: password required")
	}

	tlsCfg, err := shared.BuildClientTLS(cfg.TLS)
	if err != nil {
		return nil, fmt.Errorf("trojan/dialer: build TLS config: %w", err)
	}

	return &Dialer{
		server:       cfg.Server,
		passwordHash: HashPassword(cfg.Password),
		tlsConfig:    tlsCfg,
	}, nil
}

// DialContext connects to target through the Trojan server.
// The returned net.Conn is ready for application data immediately.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// 1. TCP dial to server
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", d.server)
	if err != nil {
		return nil, fmt.Errorf("trojan/dialer: TCP dial: %w", err)
	}

	// 2. Optional TLS handshake
	if d.tlsConfig != nil {
		tlsConn := tls.Client(conn, d.tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, fmt.Errorf("trojan/dialer: TLS handshake: %w", err)
		}
		conn = tlsConn
	}

	// 3. Send Trojan request header
	cmd := byte(shared.CmdConnect)
	if network == "udp" {
		cmd = shared.CmdUDPAssociate
	}
	if err := EncodeRequest(conn, d.passwordHash, cmd, address); err != nil {
		conn.Close()
		return nil, fmt.Errorf("trojan/dialer: encode request: %w", err)
	}

	// 4. No response header — data flows immediately
	return conn, nil
}
