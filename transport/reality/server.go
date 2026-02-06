package reality

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/shuttle-proxy/shuttle/crypto"
	"github.com/shuttle-proxy/shuttle/transport"
	"golang.org/x/crypto/curve25519"
)

// ServerConfig holds configuration for a Reality server transport.
type ServerConfig struct {
	ListenAddr string
	PrivateKey string
	ShortIDs   []string
	TargetSNI  string
	TargetAddr string
	CertFile   string
	KeyFile    string
}

// Server implements transport.ServerTransport using Reality (TLS + Noise IK + yamux).
type Server struct {
	config   *ServerConfig
	listener net.Listener
	connCh   chan transport.Connection
	closed   atomic.Bool
	logger   *slog.Logger
	privKey  [32]byte
	pubKey   [32]byte
}

// NewServer creates a new Reality server transport.
// The private key is parsed from hex; the public key is derived from it.
func NewServer(cfg *ServerConfig, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		config: cfg,
		connCh: make(chan transport.Connection, 64),
		logger: logger,
	}
	if cfg.PrivateKey != "" {
		if keyBytes, err := hex.DecodeString(cfg.PrivateKey); err == nil && len(keyBytes) == 32 {
			copy(s.privKey[:], keyBytes)
			// Clamp for Curve25519
			s.privKey[0] &= 248
			s.privKey[31] &= 127
			s.privKey[31] |= 64
			pubSlice, err := curve25519.X25519(s.privKey[:], curve25519.Basepoint)
			if err == nil {
				copy(s.pubKey[:], pubSlice)
			}
		}
	}
	return s
}

// Type returns the transport type identifier.
func (s *Server) Type() string { return "reality" }

// Listen starts the TLS listener and begins accepting connections.
func (s *Server) Listen(ctx context.Context) error {
	addr := s.config.ListenAddr
	if addr == "" {
		addr = ":443"
	}
	tlsConf := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}
	if s.config.CertFile != "" && s.config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.config.CertFile, s.config.KeyFile)
		if err != nil {
			return fmt.Errorf("load tls cert: %w", err)
		}
		tlsConf.Certificates = []tls.Certificate{cert}
	}
	ln, err := tls.Listen("tcp", addr, tlsConf)
	if err != nil {
		return fmt.Errorf("reality listen: %w", err)
	}
	s.listener = ln
	s.logger.Info("reality server listening", "addr", addr)
	go s.acceptLoop(ctx)
	return nil
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		if s.closed.Load() {
			return
		}
		raw, err := s.listener.Accept()
		if err != nil {
			if s.closed.Load() {
				return
			}
			s.logger.Error("reality accept error", "err", err)
			continue
		}
		go s.handleConn(ctx, raw)
	}
}

func (s *Server) handleConn(ctx context.Context, raw net.Conn) {
	// Set a deadline for the Noise handshake phase
	raw.SetReadDeadline(time.Now().Add(10 * time.Second))

	hs, err := crypto.NewResponder(s.privKey, s.pubKey)
	if err != nil {
		s.logger.Error("noise responder init failed", "err", err)
		raw.Close()
		return
	}

	// Read handshake message 1 from client
	msg1, err := readFrame(raw)
	if err != nil {
		s.logger.Debug("noise read failed, forwarding to target", "err", err)
		raw.SetReadDeadline(time.Time{})
		s.forwardToTarget(raw)
		return
	}

	_, err = hs.ReadMessage(msg1)
	if err != nil {
		s.logger.Debug("noise auth failed, forwarding to target", "err", err)
		raw.SetReadDeadline(time.Time{})
		s.forwardToTarget(raw)
		return
	}

	// Write handshake message 2
	msg2, err := hs.WriteMessage(nil)
	if err != nil {
		s.logger.Debug("noise write failed, forwarding to target", "err", err)
		raw.SetReadDeadline(time.Time{})
		s.forwardToTarget(raw)
		return
	}
	if err := writeFrame(raw, msg2); err != nil {
		raw.Close()
		return
	}

	// Clear the handshake deadline
	raw.SetReadDeadline(time.Time{})

	if !hs.Completed() {
		s.forwardToTarget(raw)
		return
	}

	s.logger.Debug("reality auth success", "peer", fmt.Sprintf("%x", hs.PeerPublicKey()))

	// Create yamux server session
	sess, err := yamux.Server(raw, yamux.DefaultConfig())
	if err != nil {
		s.logger.Error("yamux server error", "err", err)
		raw.Close()
		return
	}

	conn := &realityConnection{rawConn: raw, session: sess}
	select {
	case s.connCh <- conn:
	case <-ctx.Done():
		conn.Close()
	}
}

// Accept returns the next authenticated Reality connection.
func (s *Server) Accept(ctx context.Context) (transport.Connection, error) {
	select {
	case conn := <-s.connCh:
		return conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close shuts down the server transport.
func (s *Server) Close() error {
	s.closed.Store(true)
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// forwardToTarget proxies the connection to the configured target address,
// making unauthenticated connections appear as normal HTTPS traffic.
func (s *Server) forwardToTarget(conn net.Conn) {
	if s.config.TargetAddr == "" {
		conn.Close()
		return
	}
	target, err := net.DialTimeout("tcp", s.config.TargetAddr, 10*time.Second)
	if err != nil {
		s.logger.Debug("forward to target failed", "err", err)
		conn.Close()
		return
	}
	done := make(chan struct{}, 2)
	cp := func(dst io.Writer, src io.Reader) {
		buf := make([]byte, 32*1024)
		io.CopyBuffer(dst, src, buf)
		done <- struct{}{}
	}
	go cp(target, conn)
	go cp(conn, target)
	<-done
	<-done
	target.Close()
	conn.Close()
}

// Compile-time interface check.
var _ transport.ServerTransport = (*Server)(nil)
