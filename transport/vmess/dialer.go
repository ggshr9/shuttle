package vmess

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/ggshr9/shuttle/transport/shared"
)

// DialerConfig configures a VMess client dialer.
type DialerConfig struct {
	Server   string            `json:"server" yaml:"server"`
	UUID     [16]byte          `json:"uuid" yaml:"uuid"`
	Security byte              `json:"security" yaml:"security"` // SecurityAES128GCM or SecurityNone
	TLS      shared.TLSOptions `json:"tls" yaml:"tls"`
}

// Dialer creates connections through a VMess server.
type Dialer struct {
	server    string
	uuid      [16]byte
	security  byte
	tlsConfig *tls.Config
}

// NewDialer creates a Dialer from the given config.
func NewDialer(cfg *DialerConfig) (*Dialer, error) {
	if cfg.Server == "" {
		return nil, fmt.Errorf("vmess/dialer: server address required")
	}
	if cfg.UUID == [16]byte{} {
		return nil, fmt.Errorf("vmess/dialer: UUID required")
	}
	if cfg.Security == 0 {
		cfg.Security = SecurityAES128GCM
	}

	tlsCfg, err := shared.BuildClientTLS(cfg.TLS)
	if err != nil {
		return nil, fmt.Errorf("vmess/dialer: build TLS config: %w", err)
	}

	return &Dialer{
		server:    cfg.Server,
		uuid:      cfg.UUID,
		security:  cfg.Security,
		tlsConfig: tlsCfg,
	}, nil
}

// DialContext connects to target through the VMess server.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// 1. TCP dial to server
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", d.server)
	if err != nil {
		return nil, fmt.Errorf("vmess/dialer: TCP dial: %w", err)
	}

	// 2. Optional TLS handshake
	if d.tlsConfig != nil {
		tlsConn := tls.Client(conn, d.tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, fmt.Errorf("vmess/dialer: TLS handshake: %w", err)
		}
		conn = tlsConn
	}

	// 3. Build and send VMess request header
	cmd := CmdTCP
	if network == "udp" {
		cmd = CmdUDP
	}

	var dataIV [8]byte
	var dataKey [8]byte
	if _, err := rand.Read(dataIV[:]); err != nil {
		conn.Close()
		return nil, fmt.Errorf("vmess/dialer: random IV: %w", err)
	}
	if _, err := rand.Read(dataKey[:]); err != nil {
		conn.Close()
		return nil, fmt.Errorf("vmess/dialer: random key: %w", err)
	}

	hdr := &RequestHeader{
		Version:  Version,
		Command:  cmd,
		Security: d.security,
		Address:  address,
		DataIV:   dataIV,
		DataKey:  dataKey,
	}

	timestamp := time.Now().Unix()
	if err := EncodeRequest(conn, d.uuid, timestamp, hdr); err != nil {
		conn.Close()
		return nil, fmt.Errorf("vmess/dialer: encode request: %w", err)
	}

	// 4. Read server response byte
	var resp [1]byte
	if _, err := conn.Read(resp[:]); err != nil {
		conn.Close()
		return nil, fmt.Errorf("vmess/dialer: read response: %w", err)
	}
	if resp[0] != ResponseOK {
		conn.Close()
		return nil, fmt.Errorf("vmess/dialer: server returned error code 0x%02x", resp[0])
	}

	// 5. Body flows directly (security=None relies on outer TLS; AES-GCM chunked
	//    encryption can be added in a follow-up for full wire compatibility).
	return conn, nil
}
