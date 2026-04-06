package vless

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/shuttleX/shuttle/transport/shared"
)

// DialerConfig holds configuration for a VLESS client dialer.
type DialerConfig struct {
	Server string       // host:port of the VLESS server
	UUID   [16]byte     // client UUID for authentication
	TLS    shared.TLSOptions
}

// Dialer connects to a VLESS server and tunnels traffic through it.
type Dialer struct {
	server    string
	uuid      [16]byte
	tlsConfig *tls.Config
}

// NewDialer creates a Dialer from the given config.
func NewDialer(cfg DialerConfig) (*Dialer, error) {
	tlsCfg, err := shared.BuildClientTLS(cfg.TLS)
	if err != nil {
		return nil, fmt.Errorf("vless/dialer: build TLS: %w", err)
	}
	return &Dialer{
		server:    cfg.Server,
		uuid:      cfg.UUID,
		tlsConfig: tlsCfg,
	}, nil
}

// DialContext dials through the VLESS server to the given target address.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// 1. TCP dial to server.
	var dialer net.Dialer
	raw, err := dialer.DialContext(ctx, "tcp", d.server)
	if err != nil {
		return nil, fmt.Errorf("vless/dialer: connect server: %w", err)
	}

	conn := raw

	// 2. TLS handshake if configured.
	if d.tlsConfig != nil {
		tlsConn := tls.Client(raw, d.tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			raw.Close()
			return nil, fmt.Errorf("vless/dialer: TLS handshake: %w", err)
		}
		conn = tlsConn
	}

	// 3. Send VLESS request header.
	cmd := byte(CmdTCP)
	if network == "udp" {
		cmd = CmdUDP
	}

	h := &RequestHeader{
		UUID:    d.uuid,
		Cmd:     cmd,
		Network: network,
		Address: address,
	}

	if err := EncodeRequest(conn, h); err != nil {
		conn.Close()
		return nil, fmt.Errorf("vless/dialer: encode request: %w", err)
	}

	// 4. Read response header.
	if err := DecodeResponse(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("vless/dialer: decode response: %w", err)
	}

	return conn, nil
}

// Type returns the transport type name.
func (d *Dialer) Type() string { return "vless" }

// Close is a no-op for the VLESS dialer (connections are independent).
func (d *Dialer) Close() error { return nil }
