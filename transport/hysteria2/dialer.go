package hysteria2

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/quic-go/quic-go"
	"github.com/ggshr9/shuttle/transport/shared"
)

// DialerConfig configures a Hysteria2 client dialer.
type DialerConfig struct {
	Server   string            `json:"server" yaml:"server"`
	Password string            `json:"password" yaml:"password"`
	TLS      shared.TLSOptions `json:"tls" yaml:"tls"`
}

// Dialer creates connections through a Hysteria2 server using QUIC multiplexing.
type Dialer struct {
	server   string
	password string
	tlsCfg   *tls.Config

	mu   sync.Mutex
	conn *quic.Conn // guarded by mu

	reqID atomic.Uint32
}

// NewDialer creates a Hysteria2 Dialer from the given config.
func NewDialer(cfg *DialerConfig) (*Dialer, error) {
	if cfg.Server == "" {
		return nil, fmt.Errorf("hysteria2/dialer: server address required")
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("hysteria2/dialer: password required")
	}

	// Force TLS enabled with h3 ALPN for QUIC
	cfg.TLS.Enabled = true
	if len(cfg.TLS.ALPN) == 0 {
		cfg.TLS.ALPN = []string{"h3"}
	}

	tlsCfg, err := shared.BuildClientTLS(cfg.TLS)
	if err != nil {
		return nil, fmt.Errorf("hysteria2/dialer: build TLS config: %w", err)
	}

	return &Dialer{
		server:   cfg.Server,
		password: cfg.Password,
		tlsCfg:   tlsCfg,
	}, nil
}

// DialContext connects to the target address through the Hysteria2 server.
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
			return nil, fmt.Errorf("hysteria2/dialer: open stream: %w", err)
		}
	}

	reqID := d.reqID.Add(1)
	if err := EncodeStreamHeader(stream, reqID, address); err != nil {
		stream.Close()
		return nil, fmt.Errorf("hysteria2/dialer: write header: %w", err)
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
	})
	if err != nil {
		return nil, fmt.Errorf("hysteria2/dialer: QUIC dial: %w", err)
	}

	// Authenticate on the first stream.
	authStream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		_ = conn.CloseWithError(1, "auth stream failed")
		return nil, fmt.Errorf("hysteria2/dialer: open auth stream: %w", err)
	}

	if err := EncodeAuth(authStream, d.password); err != nil {
		authStream.Close()
		_ = conn.CloseWithError(1, "auth write failed")
		return nil, fmt.Errorf("hysteria2/dialer: write auth: %w", err)
	}

	// Read auth result. The server writes the result after reading auth,
	// so we don't need to close our write side first — the framing is
	// length-prefixed and the server knows exactly how many bytes to read.
	status, err := ReadAuthResult(authStream)
	if err != nil {
		_ = conn.CloseWithError(1, "auth read failed")
		return nil, fmt.Errorf("hysteria2/dialer: read auth result: %w", err)
	}

	// Done with auth stream — close it.
	authStream.CancelRead(0)
	authStream.Close()
	if status != AuthOK {
		_ = conn.CloseWithError(1, "auth rejected")
		return nil, fmt.Errorf("hysteria2/dialer: authentication failed (status=%d)", status)
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
