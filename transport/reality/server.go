package reality

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"github.com/ggshr9/shuttle/config"
	shuttlecrypto "github.com/ggshr9/shuttle/crypto"
	"github.com/ggshr9/shuttle/internal/pool"
	"github.com/ggshr9/shuttle/transport"
	ymux "github.com/ggshr9/shuttle/transport/mux/yamux"
	"golang.org/x/crypto/curve25519"
)

// noisePeerPub is the minimal Noise state interface needed by
// detectAndHandlePQ/finishPQExchange for logging. Keeping this as an interface
// instead of *shuttlecrypto.NoiseHandshake lets tests pass lightweight stubs
// without running a real Noise handshake.
type noisePeerPub interface {
	PeerPublicKey() []byte
}

// ServerConfig holds configuration for a Reality server transport.
type ServerConfig struct {
	ListenAddr  string
	PrivateKey  string
	ShortIDs    []string
	TargetSNI   string
	TargetAddr  string
	CertFile    string
	KeyFile     string
	PostQuantum bool                // Enable hybrid X25519 + ML-KEM-768 key exchange
	Yamux       *config.YamuxConfig // optional yamux tuning
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
	metrics  *transport.HandshakeMetrics
}

// SetHandshakeMetrics installs the handshake metrics hook. Must be called
// before Listen; the hook is invoked once per completed or failed handshake.
func (s *Server) SetHandshakeMetrics(m *transport.HandshakeMetrics) {
	s.metrics = m
}

// NewServer creates a new Reality server transport.
// The private key is parsed from hex; the public key is derived from it.
// Returns an error if the private key is malformed or missing.
func NewServer(cfg *ServerConfig, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		config: cfg,
		connCh: make(chan transport.Connection, 64),
		logger: logger,
	}
	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("reality: private key is required")
	}
	privBytes, err := hex.DecodeString(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("reality: invalid private key hex: %w", err)
	}
	if len(privBytes) != 32 {
		return nil, fmt.Errorf("reality: private key must be 32 bytes, got %d", len(privBytes))
	}
	copy(s.privKey[:], privBytes)
	// Clamp for Curve25519
	shuttlecrypto.ClampPrivateKey(s.privKey[:])
	pubSlice, err := curve25519.X25519(s.privKey[:], curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("reality: derive public key: %w", err)
	}
	copy(s.pubKey[:], pubSlice)
	return s, nil
}

// Type returns the transport type identifier.
func (s *Server) Type() string { return "reality" }

// Listen starts the TLS listener and begins accepting connections.
func (s *Server) Listen(ctx context.Context) error {
	addr := s.config.ListenAddr
	if addr == "" {
		addr = config.DefaultListenPort
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
	} else {
		// Auto-generate self-signed cert for zero-config setup
		certPEM, keyPEM, err := shuttlecrypto.GenerateSelfSignedCert(nil, 365*24*time.Hour)
		if err != nil {
			return fmt.Errorf("generate self-signed cert: %w", err)
		}
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return fmt.Errorf("parse self-signed cert: %w", err)
		}
		tlsConf.Certificates = []tls.Certificate{cert}
		s.logger.Info("reality: using auto-generated self-signed certificate")
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
	start := time.Now()

	// Set a deadline for the Noise handshake phase
	_ = raw.SetReadDeadline(time.Now().Add(10 * time.Second))

	hs, err := shuttlecrypto.NewResponder(s.privKey, s.pubKey)
	if err != nil {
		s.logger.Error("noise responder init failed", "err", err)
		s.recordFailure("protocol")
		raw.Close()
		return
	}

	// Read handshake message 1 from client
	msg1, err := readFrame(raw)
	if err != nil {
		s.logger.Debug("noise read failed, forwarding to target", "err", err)
		s.recordFailure(classifyReason(err))
		_ = raw.SetReadDeadline(time.Time{})
		s.forwardToTarget(raw)
		return
	}

	_, err = hs.ReadMessage(msg1)
	if err != nil {
		s.logger.Debug("noise auth failed, forwarding to target", "err", err)
		s.recordFailure("auth")
		raw.SetReadDeadline(time.Time{})
		s.forwardToTarget(raw)
		return
	}

	// Write handshake message 2
	msg2, err := hs.WriteMessage(nil)
	if err != nil {
		s.logger.Debug("noise write failed, forwarding to target", "err", err)
		s.recordFailure("protocol")
		raw.SetReadDeadline(time.Time{})
		s.forwardToTarget(raw)
		return
	}
	if err := writeFrame(raw, msg2); err != nil {
		s.recordFailure(classifyReason(err))
		raw.Close()
		return
	}

	// Clear the handshake deadline
	raw.SetReadDeadline(time.Time{})

	if !hs.Completed() {
		s.recordFailure("auth")
		s.forwardToTarget(raw)
		return
	}

	s.logger.Debug("reality auth success", "peer", fmt.Sprintf("%x", hs.PeerPublicKey()))

	if s.config.PostQuantum {
		next, err := s.detectAndHandlePQ(raw, hs)
		if err != nil {
			s.logger.Warn("reality pq phase failed", "err", err)
			s.recordFailure("protocol")
			raw.Close()
			return
		}
		raw = next
	}

	// Create yamux server session
	mux := ymux.New(s.config.Yamux)
	conn, err := mux.Server(raw)
	if err != nil {
		s.logger.Error("yamux server error", "err", err)
		s.recordFailure("protocol")
		raw.Close()
		return
	}
	s.recordSuccess(time.Since(start))
	select {
	case s.connCh <- conn:
	case <-ctx.Done():
		conn.Close()
	}
}

func (s *Server) recordSuccess(d time.Duration) {
	if s.metrics != nil && s.metrics.OnSuccess != nil {
		s.metrics.OnSuccess("reality", d)
	}
}

func (s *Server) recordFailure(reason string) {
	if s.metrics != nil && s.metrics.OnFailure != nil {
		s.metrics.OnFailure("reality", reason)
	}
}

// detectAndHandlePQ handles the optional post-quantum KEM exchange phase.
// Before this refactor: strictly required a PQ frame from the client, closing
// classical clients. After this refactor (Task 4): detects PQ vs classical
// via byte peek and dispatches accordingly.
//
// Returns the net.Conn that subsequent yamux setup should use, or an error
// if the connection must be closed.
func (s *Server) detectAndHandlePQ(raw net.Conn, hs noisePeerPub) (net.Conn, error) {
	raw.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Peek 3 bytes to distinguish yamux (classical client) from PQ frame.
	//
	// Detection:
	//   peek[0] == 0x00  → yamux v0 header (classical client). Replay peek
	//                      into a peekConn and let yamux consume it.
	//   peek[2] == 0x02  → PQ frame: peek[0:2] is the 2-byte big-endian
	//                      length prefix, peek[2] is HandshakeVersionHybridPQ.
	//                      Read the rest of the payload and run the exchange.
	//   otherwise        → garbage. Close.
	//
	// The heuristic is unambiguous because the PQ payload is > 255 bytes
	// (enforced by pqPayloadMinSize in pq_compat.go), so the length prefix's
	// high byte is always non-zero for real PQ frames; and yamux v0 guarantees
	// the first byte is 0x00 (yamux protocol version).
	peek := make([]byte, 3)
	if _, err := io.ReadFull(raw, peek); err != nil {
		raw.SetReadDeadline(time.Time{})
		return nil, fmt.Errorf("pq peek read: %w", err)
	}

	switch {
	case peek[0] == 0x00:
		raw.SetReadDeadline(time.Time{})
		s.logger.Debug("reality pq-server accepting classical client",
			"peer", fmt.Sprintf("%x", hs.PeerPublicKey()))
		return &peekConn{Conn: raw, prefix: append([]byte(nil), peek...)}, nil

	case peek[2] == shuttlecrypto.HandshakeVersionHybridPQ:
		return s.finishPQExchange(raw, hs, peek)

	default:
		raw.SetReadDeadline(time.Time{})
		return nil, fmt.Errorf("pq detect: unexpected prefix %x", peek)
	}
}

// finishPQExchange completes the PQ handshake after detection has confirmed
// the client is sending a PQ frame. peek contains the 3 already-read bytes
// (2-byte length prefix + 1 byte of payload).
func (s *Server) finishPQExchange(raw net.Conn, hs noisePeerPub, peek []byte) (net.Conn, error) {
	defer raw.SetReadDeadline(time.Time{})

	payloadLen := int(binary.BigEndian.Uint16(peek[:2]))
	if payloadLen < 1 || payloadLen > 64*1024 {
		return nil, fmt.Errorf("pq frame length out of range: %d", payloadLen)
	}
	// We have peek[2] as the first payload byte (version 0x02).
	// Read the remaining payloadLen-1 bytes.
	rest := make([]byte, payloadLen-1)
	if _, err := io.ReadFull(raw, rest); err != nil {
		return nil, fmt.Errorf("pq payload read: %w", err)
	}

	// Reconstruct full payload: [version byte, pqPubKey...].
	pqFrame := make([]byte, 0, payloadLen)
	pqFrame = append(pqFrame, peek[2])
	pqFrame = append(pqFrame, rest...)

	pqPubBytes := pqFrame[1:]
	pq, err := shuttlecrypto.NewPQHandshake()
	if err != nil {
		return nil, fmt.Errorf("pq handshake init: %w", err)
	}

	pqSecret, ciphertext, err := pq.Encapsulate(pqPubBytes)
	if err != nil {
		return nil, fmt.Errorf("pq encapsulate: %w", err)
	}

	if err := writeFrame(raw, ciphertext); err != nil {
		return nil, fmt.Errorf("pq ciphertext send: %w", err)
	}

	wrapped, err := wrapConnWithPQ(raw, pqSecret)
	if err != nil {
		return nil, fmt.Errorf("pq wrap: %w", err)
	}
	s.logger.Debug("reality pq exchange complete", "peer", fmt.Sprintf("%x", hs.PeerPublicKey()))
	return wrapped, nil
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
	cp := func(dst net.Conn, src net.Conn) {
		buf := pool.GetMedLarge()
		_, _ = io.CopyBuffer(dst, src, buf)
		pool.PutMedLargeNoZero(buf)
		// Close write direction to signal EOF to peer.
		if tc, ok := dst.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
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
