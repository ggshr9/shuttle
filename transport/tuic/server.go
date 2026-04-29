package tuic

import (
	"context"
	"crypto/hmac"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/ggshr9/shuttle/adapter"
	"github.com/ggshr9/shuttle/transport/shared"
)

const (
	defaultIdleTimeout = 30 * time.Second
	defaultKeepAlive   = 10 * time.Second
)

// ServerConfig holds TUIC v5 server configuration.
type ServerConfig struct {
	// Users maps UUID strings to passwords for multi-user support.
	Users map[string]string               `json:"users" yaml:"users"`
	TLS   shared.ServerTLSOptions         `json:"tls" yaml:"tls"`
}

// userEntry stores precomputed auth material for a user.
type userEntry struct {
	uuid  [UUIDLen]byte
	token [TokenLen]byte
}

// Server accepts TUIC v5 connections over QUIC and dispatches
// authenticated streams to a handler.
type Server struct {
	users  []userEntry
	tlsCfg *tls.Config
	logger *log.Logger
	ln     *quic.Listener
}

// NewServer creates a TUIC server from config (loads TLS cert from files).
func NewServer(cfg ServerConfig, logger *log.Logger) (*Server, error) {
	if logger == nil {
		logger = log.Default()
	}

	users, err := buildUserEntries(cfg.Users)
	if err != nil {
		return nil, fmt.Errorf("tuic/server: %w", err)
	}

	tlsCfg, err := shared.BuildServerTLS(cfg.TLS)
	if err != nil {
		return nil, fmt.Errorf("tuic/server: build TLS: %w", err)
	}
	tlsCfg.NextProtos = []string{"h3"}

	return &Server{
		users:  users,
		tlsCfg: tlsCfg,
		logger: logger,
	}, nil
}

// NewServerWithTLS creates a TUIC server with a pre-built TLS config.
// For single-user mode, pass one uuid+password pair.
func NewServerWithTLS(users map[string]string, tlsCfg *tls.Config, logger *log.Logger) (*Server, error) {
	if logger == nil {
		logger = log.Default()
	}
	entries, err := buildUserEntries(users)
	if err != nil {
		return nil, err
	}
	return &Server{
		users:  entries,
		tlsCfg: tlsCfg,
		logger: logger,
	}, nil
}

func buildUserEntries(users map[string]string) ([]userEntry, error) {
	if len(users) == 0 {
		return nil, fmt.Errorf("at least one user (uuid:password) is required")
	}
	entries := make([]userEntry, 0, len(users))
	for uuidStr, password := range users {
		uuid, err := ParseUUID(uuidStr)
		if err != nil {
			return nil, err
		}
		token := ComputeToken(uuid, []byte(password))
		entries = append(entries, userEntry{uuid: uuid, token: token})
	}
	return entries, nil
}

// ServeUDP listens for QUIC connections on the given UDP connection and
// handles them until ctx is cancelled.
func (s *Server) ServeUDP(ctx context.Context, udpConn net.PacketConn, handler adapter.ConnHandler) error {
	tr := &quic.Transport{Conn: udpConn}

	ln, err := tr.Listen(s.tlsCfg, &quic.Config{
		MaxIdleTimeout:  defaultIdleTimeout,
		KeepAlivePeriod: defaultKeepAlive,
		EnableDatagrams: true,
	})
	if err != nil {
		return fmt.Errorf("tuic/server: QUIC listen: %w", err)
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
				return fmt.Errorf("tuic/server: accept: %w", err)
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
// Since TUIC requires QUIC (UDP), callers should use ServeUDP directly.
func (s *Server) Serve(_ context.Context, _ net.Listener, _ adapter.ConnHandler) error {
	return fmt.Errorf("tuic/server: Serve(net.Listener) not supported; use ServeUDP with a UDP conn")
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

	// Read auth datagram (UUID + Token).
	authData, err := conn.ReceiveDatagram(ctx)
	if err != nil {
		s.logger.Printf("tuic/server: receive auth datagram: %v", err)
		return
	}

	authReq, err := DecodeAuth(authData)
	if err != nil {
		s.logger.Printf("tuic/server: decode auth: %v", err)
		return
	}

	if !s.verifyAuth(authReq) {
		s.logger.Printf("tuic/server: auth failed for UUID")
		_ = conn.CloseWithError(1, "auth failed")
		return
	}

	// Accept data streams.
	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handleStream(ctx, conn, stream, handler)
		}()
	}
}

func (s *Server) verifyAuth(req *AuthRequest) bool {
	for _, u := range s.users {
		if u.uuid == req.UUID && hmac.Equal(u.token[:], req.Token[:]) {
			return true
		}
	}
	return false
}

func (s *Server) handleStream(ctx context.Context, conn *quic.Conn, stream *quic.Stream, handler adapter.ConnHandler) {
	cmd, address, err := DecodeConnectHeader(stream)
	if err != nil {
		s.logger.Printf("tuic/server: decode connect header: %v", err)
		stream.Close()
		return
	}

	network := "tcp"
	if cmd == CmdUDPAssoc {
		network = "udp"
	}

	sc := &streamConn{
		Stream: stream,
		local:  conn.LocalAddr(),
		remote: conn.RemoteAddr(),
	}

	handler(ctx, sc, adapter.ConnMetadata{
		Source:      conn.RemoteAddr(),
		Destination: address,
		Network:     network,
		Protocol:    "tuic",
	})
}
