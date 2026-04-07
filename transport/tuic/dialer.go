package tuic

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"

	"github.com/quic-go/quic-go"
	"github.com/shuttleX/shuttle/transport/shared"
)

// DialerConfig configures a TUIC v5 client dialer.
type DialerConfig struct {
	Server   string            `json:"server" yaml:"server"`
	UUID     string            `json:"uuid" yaml:"uuid"`
	Password string            `json:"password" yaml:"password"`
	TLS      shared.TLSOptions `json:"tls" yaml:"tls"`
}

// Dialer creates connections through a TUIC v5 server using QUIC multiplexing.
type Dialer struct {
	server   string
	uuid     [UUIDLen]byte
	token    [TokenLen]byte
	tlsCfg   *tls.Config

	mu   sync.Mutex
	conn *quic.Conn // guarded by mu
}

// NewDialer creates a TUIC Dialer from the given config.
func NewDialer(cfg *DialerConfig) (*Dialer, error) {
	if cfg.Server == "" {
		return nil, fmt.Errorf("tuic/dialer: server address required")
	}
	if cfg.UUID == "" {
		return nil, fmt.Errorf("tuic/dialer: UUID required")
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("tuic/dialer: password required")
	}

	uuid, err := ParseUUID(cfg.UUID)
	if err != nil {
		return nil, fmt.Errorf("tuic/dialer: %w", err)
	}

	token := ComputeToken(uuid, []byte(cfg.Password))

	// Force TLS enabled with h3 ALPN for QUIC
	cfg.TLS.Enabled = true
	if len(cfg.TLS.ALPN) == 0 {
		cfg.TLS.ALPN = []string{"h3"}
	}

	tlsCfg, err := shared.BuildClientTLS(cfg.TLS)
	if err != nil {
		return nil, fmt.Errorf("tuic/dialer: build TLS config: %w", err)
	}

	return &Dialer{
		server: cfg.Server,
		uuid:   uuid,
		token:  token,
		tlsCfg: tlsCfg,
	}, nil
}

// DialContext connects to the target address through the TUIC server.
// Multiple calls share one underlying QUIC connection.
func (d *Dialer) DialContext(ctx context.Context, _, address string) (net.Conn, error) {
	conn, err := d.getOrDial(ctx)
	if err != nil {
		return nil, err
	}

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		// Connection may have been closed; clear it and retry once.
		d.clearConn(conn)
		conn, err = d.getOrDial(ctx)
		if err != nil {
			return nil, err
		}
		stream, err = conn.OpenStreamSync(ctx)
		if err != nil {
			return nil, fmt.Errorf("tuic/dialer: open stream: %w", err)
		}
	}

	if err := EncodeConnectHeader(stream, CmdConnect, address); err != nil {
		stream.Close()
		return nil, fmt.Errorf("tuic/dialer: write header: %w", err)
	}

	return &streamConn{
		Stream: stream,
		local:  conn.LocalAddr(),
		remote: conn.RemoteAddr(),
	}, nil
}

// Close shuts down the QUIC connection.
func (d *Dialer) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn != nil {
		err := d.conn.CloseWithError(0, "dialer closed")
		d.conn = nil
		return err
	}
	return nil
}

// getOrDial returns the existing QUIC connection or establishes a new one with auth.
func (d *Dialer) getOrDial(ctx context.Context) (*quic.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.conn != nil {
		return d.conn, nil
	}

	conn, err := quic.DialAddr(ctx, d.server, d.tlsCfg, &quic.Config{
		MaxIdleTimeout:  defaultIdleTimeout,
		KeepAlivePeriod: defaultKeepAlive,
		EnableDatagrams: true,
	})
	if err != nil {
		return nil, fmt.Errorf("tuic/dialer: QUIC dial: %w", err)
	}

	// Send auth as QUIC datagram: UUID(16) + Token(32)
	authReq := &AuthRequest{UUID: d.uuid, Token: d.token}
	if err := conn.SendDatagram(authReq.Encode()); err != nil {
		_ = conn.CloseWithError(1, "auth datagram failed")
		return nil, fmt.Errorf("tuic/dialer: send auth datagram: %w", err)
	}

	d.conn = conn
	return conn, nil
}

// clearConn clears the stored connection if it matches the given one.
func (d *Dialer) clearConn(c *quic.Conn) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn == c {
		d.conn = nil
	}
}
