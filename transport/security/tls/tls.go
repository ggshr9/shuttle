// Package tls provides a SecureWrapper for TLS connections.
package tls

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/shuttleX/shuttle/adapter"
)

// Config configures the TLS wrapper.
type Config struct {
	ServerName         string
	InsecureSkipVerify bool
	NextProtos         []string
	MinVersion         uint16
	ServerCert         *tls.Certificate // Required for server side.
}

// Wrapper implements adapter.SecureWrapper using crypto/tls.
type Wrapper struct {
	cfg Config
}

// New creates a TLS SecureWrapper.
func New(cfg Config) *Wrapper {
	if cfg.MinVersion == 0 {
		cfg.MinVersion = tls.VersionTLS13
	}
	return &Wrapper{cfg: cfg}
}

func (w *Wrapper) WrapClient(ctx context.Context, conn net.Conn) (net.Conn, error) {
	tlsConf := &tls.Config{
		ServerName:         w.cfg.ServerName,
		InsecureSkipVerify: w.cfg.InsecureSkipVerify,
		NextProtos:         w.cfg.NextProtos,
		MinVersion:         w.cfg.MinVersion,
	}
	tlsConn := tls.Client(conn, tlsConf)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("tls client handshake: %w", err)
	}
	return tlsConn, nil
}

func (w *Wrapper) WrapServer(ctx context.Context, conn net.Conn) (net.Conn, error) {
	if w.cfg.ServerCert == nil {
		return nil, fmt.Errorf("tls server: no certificate configured")
	}
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{*w.cfg.ServerCert},
		NextProtos:   w.cfg.NextProtos,
		MinVersion:   w.cfg.MinVersion,
	}
	tlsConn := tls.Server(conn, tlsConf)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("tls server handshake: %w", err)
	}
	return tlsConn, nil
}

var _ adapter.SecureWrapper = (*Wrapper)(nil)
