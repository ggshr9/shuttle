package vless

import (
	"context"
	"fmt"
	"net"
)

// ConnMeta carries metadata about an accepted VLESS connection.
type ConnMeta struct {
	Network     string // "tcp" or "udp"
	Destination string // target "host:port"
	Source      string // client remote address
	UserTag     string // user tag from UUID lookup
}

// ConnHandler is called for each accepted VLESS connection with the
// unwrapped stream and parsed target metadata.
type ConnHandler func(ctx context.Context, conn net.Conn, meta ConnMeta)

// ServerConfig holds configuration for a VLESS inbound server.
type ServerConfig struct {
	Users map[[16]byte]string // uuid → user tag
}

// Server is a VLESS inbound handler that accepts connections,
// validates the UUID, reads the target address, and hands them off.
type Server struct {
	users map[[16]byte]string
}

// NewServer creates a Server from the given config.
func NewServer(cfg ServerConfig) (*Server, error) {
	if len(cfg.Users) == 0 {
		return nil, fmt.Errorf("vless/server: at least one user UUID is required")
	}
	return &Server{users: cfg.Users}, nil
}

// Serve runs the accept loop on the provided listener. For each connection it
// reads the VLESS request header, validates the UUID, sends a response header,
// and invokes handler in a new goroutine. Serve blocks until ctx is cancelled
// or the listener is closed.
//
// The listener should already be TLS-wrapped externally if TLS is desired;
// this server handles only the VLESS protocol layer.
func (s *Server) Serve(ctx context.Context, ln net.Listener, handler ConnHandler) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("vless/server: accept: %w", err)
			}
		}

		go s.handle(ctx, conn, handler)
	}
}

func (s *Server) handle(ctx context.Context, conn net.Conn, handler ConnHandler) {
	h, err := DecodeRequest(conn)
	if err != nil {
		conn.Close()
		return
	}

	// Validate UUID.
	tag, ok := s.users[h.UUID]
	if !ok {
		conn.Close()
		return
	}

	// Send response header.
	if err := EncodeResponse(conn); err != nil {
		conn.Close()
		return
	}

	meta := ConnMeta{
		Network:     h.Network,
		Destination: h.Address,
		Source:      conn.RemoteAddr().String(),
		UserTag:     tag,
	}

	handler(ctx, conn, meta)
}

// Type returns the transport type name.
func (s *Server) Type() string { return "vless" }

// Close is a no-op; close the listener to stop serving.
func (s *Server) Close() error { return nil }
