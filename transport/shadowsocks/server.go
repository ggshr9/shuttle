package shadowsocks

import (
	"context"
	"fmt"
	"net"

	"github.com/shadowsocks/go-shadowsocks2/core"

	"github.com/ggshr9/shuttle/transport/shared"
)

// ConnMeta carries metadata about an accepted connection.
type ConnMeta struct {
	Network     string // "tcp"
	Destination string // target "host:port"
	Source      string // client remote address
}

// ConnHandler is called for each accepted Shadowsocks connection with the
// decrypted stream and parsed target metadata.
type ConnHandler func(ctx context.Context, conn net.Conn, meta ConnMeta)

// ServerConfig holds configuration for a Shadowsocks inbound server.
type ServerConfig struct {
	Method   string
	Password string
}

// Server is a Shadowsocks inbound handler that accepts encrypted connections,
// decrypts them, reads the SOCKS5-style target address, and hands them off.
type Server struct {
	cipher core.Cipher
}

// NewServer creates a Server from the given config.
func NewServer(cfg ServerConfig) (*Server, error) {
	ciph, err := NewCipher(cfg.Method, cfg.Password)
	if err != nil {
		return nil, fmt.Errorf("shadowsocks/server: %w", err)
	}
	return &Server{cipher: ciph}, nil
}

// Serve runs the accept loop on the provided listener. For each connection it
// decrypts the stream, reads the target address, and invokes handler in a new
// goroutine. Serve blocks until ctx is cancelled or the listener is closed.
func (s *Server) Serve(ctx context.Context, ln net.Listener, handler ConnHandler) error {
	for {
		raw, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("shadowsocks/server: accept: %w", err)
			}
		}

		go s.handle(ctx, raw, handler)
	}
}

func (s *Server) handle(ctx context.Context, raw net.Conn, handler ConnHandler) {
	conn := WrapConn(raw, s.cipher)

	network, addr, err := shared.DecodeAddr(conn)
	if err != nil {
		conn.Close()
		return
	}

	meta := ConnMeta{
		Network:     network,
		Destination: addr,
		Source:      raw.RemoteAddr().String(),
	}

	handler(ctx, conn, meta)
}
