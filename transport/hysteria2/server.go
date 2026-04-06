package hysteria2

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/transport/shared"
)

const (
	defaultIdleTimeout = 30 * time.Second
	defaultKeepAlive   = 10 * time.Second
)

// ServerConfig holds Hysteria2 server configuration.
type ServerConfig struct {
	Password string                  `json:"password" yaml:"password"`
	TLS      shared.ServerTLSOptions `json:"tls" yaml:"tls"`
}

// Server accepts Hysteria2 connections over QUIC and dispatches
// authenticated streams to a handler.
type Server struct {
	password string
	tlsCfg   *tls.Config
	logger   *log.Logger
	ln       *quic.Listener
}

// NewServer creates a Hysteria2 server from config (loads TLS cert from files).
func NewServer(cfg ServerConfig, logger *log.Logger) (*Server, error) {
	if logger == nil {
		logger = log.Default()
	}

	tlsCfg, err := shared.BuildServerTLS(cfg.TLS)
	if err != nil {
		return nil, fmt.Errorf("hysteria2/server: build TLS: %w", err)
	}
	tlsCfg.NextProtos = []string{"h3"}

	return &Server{
		password: cfg.Password,
		tlsCfg:   tlsCfg,
		logger:   logger,
	}, nil
}

// NewServerWithTLS creates a Hysteria2 server with a pre-built TLS config.
func NewServerWithTLS(password string, tlsCfg *tls.Config, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	return &Server{
		password: password,
		tlsCfg:   tlsCfg,
		logger:   logger,
	}
}

// ServeUDP listens for QUIC connections on the given UDP connection and
// handles them until ctx is cancelled.
func (s *Server) ServeUDP(ctx context.Context, udpConn net.PacketConn, handler adapter.ConnHandler) error {
	tr := &quic.Transport{Conn: udpConn}

	ln, err := tr.Listen(s.tlsCfg, &quic.Config{
		MaxIdleTimeout:  defaultIdleTimeout,
		KeepAlivePeriod: defaultKeepAlive,
	})
	if err != nil {
		return fmt.Errorf("hysteria2/server: QUIC listen: %w", err)
	}
	s.ln = ln

	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		conn, err := ln.Accept(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("hysteria2/server: accept: %w", err)
			}
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handleConnection(ctx, conn, handler)
		}()
	}
}

// Serve implements adapter.InboundHandler-compatible serving over a net.Listener.
// Since Hysteria2 requires QUIC (UDP), callers should use ServeUDP directly.
func (s *Server) Serve(_ context.Context, _ net.Listener, _ adapter.ConnHandler) error {
	return fmt.Errorf("hysteria2/server: Serve(net.Listener) not supported; use ServeUDP with a UDP conn")
}

// Close shuts down the QUIC listener.
func (s *Server) Close() error {
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}

func (s *Server) handleConnection(ctx context.Context, conn *quic.Conn, handler adapter.ConnHandler) {
	defer func() { _ = conn.CloseWithError(0, "done") }()

	// First stream must be auth.
	authStream, err := conn.AcceptStream(ctx)
	if err != nil {
		s.logger.Printf("hysteria2/server: accept auth stream: %v", err)
		return
	}

	password, err := DecodeAuth(authStream)
	if err != nil {
		s.logger.Printf("hysteria2/server: decode auth: %v", err)
		WriteAuthResult(authStream, AuthFail) //nolint:errcheck
		return
	}

	if password != s.password {
		s.logger.Printf("hysteria2/server: auth failed (wrong password)")
		WriteAuthResult(authStream, AuthFail) //nolint:errcheck
		return
	}

	if err := WriteAuthResult(authStream, AuthOK); err != nil {
		s.logger.Printf("hysteria2/server: write auth OK: %v", err)
		return
	}

	// Accept data streams.
	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			// Connection closed or context done.
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handleStream(ctx, conn, stream, handler)
		}()
	}
}

func (s *Server) handleStream(ctx context.Context, conn *quic.Conn, stream *quic.Stream, handler adapter.ConnHandler) {
	_, address, err := DecodeStreamHeader(stream)
	if err != nil {
		s.logger.Printf("hysteria2/server: decode stream header: %v", err)
		stream.Close()
		return
	}

	sc := &streamConn{
		Stream: stream,
		local:  conn.LocalAddr(),
		remote: conn.RemoteAddr(),
	}

	handler(ctx, sc, adapter.ConnMetadata{
		Source:      conn.RemoteAddr(),
		Destination: address,
		Network:     "tcp",
		Protocol:    "hysteria2",
	})
}
