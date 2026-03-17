package h3

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/crypto"
	"github.com/shuttleX/shuttle/obfs"
	"github.com/shuttleX/shuttle/transport"
	"github.com/shuttleX/shuttle/transport/auth"
)

type ServerConfig struct {
	ListenAddr        string
	CertFile          string
	KeyFile           string
	Password          string
	PathPrefix        string
	CoverSite         http.Handler
	CongestionControl quic.CongestionControl // optional custom CC
}

type Server struct {
	config       *ServerConfig
	listener     *quic.Listener
	connCh       chan transport.Connection
	closed       atomic.Bool
	logger       *slog.Logger
	replayFilter *crypto.ReplayFilter
	padder       *obfs.Padder
}

func NewServer(cfg *ServerConfig, logger *slog.Logger) *Server {
	if cfg.PathPrefix == "" {
		cfg.PathPrefix = "/cdn/stream/"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		config:       cfg,
		connCh:       make(chan transport.Connection, 64),
		logger:       logger,
		replayFilter: crypto.NewReplayFilter(120 * time.Second),
		padder:       obfs.NewPadder(0),
	}
}

func (s *Server) Type() string { return "h3" }

func (s *Server) Listen(ctx context.Context) error {
	tlsConf := &tls.Config{
		NextProtos:       ChromeALPN,
		MinVersion:       tls.VersionTLS13,
		CipherSuites:     ChromeCipherSuites,
		CurvePreferences: ChromeCurvePreferences,
	}
	if s.config.CertFile != "" && s.config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.config.CertFile, s.config.KeyFile)
		if err != nil {
			return fmt.Errorf("load tls cert: %w", err)
		}
		tlsConf.Certificates = []tls.Certificate{cert}
	} else {
		// Auto-generate self-signed cert for zero-config setup
		certPEM, keyPEM, err := crypto.GenerateSelfSignedCert(nil, 365*24*time.Hour)
		if err != nil {
			return fmt.Errorf("generate self-signed cert: %w", err)
		}
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return fmt.Errorf("parse self-signed cert: %w", err)
		}
		tlsConf.Certificates = []tls.Certificate{cert}
		s.logger.Info("using auto-generated self-signed certificate")
	}

	addr := s.config.ListenAddr
	if addr == "" {
		addr = config.DefaultListenPort
	}

	chromeParams := DefaultChromeTransportParams()
	quicConf := &quic.Config{
		MaxIdleTimeout:                 time.Duration(chromeParams.MaxIdleTimeout) * time.Millisecond,
		KeepAlivePeriod:                15 * time.Second,
		Allow0RTT:                      true,
		InitialStreamReceiveWindow:     chromeParams.InitialMaxStreamDataBidiLocal,
		MaxStreamReceiveWindow:         chromeParams.InitialMaxStreamDataBidiRemote,
		InitialConnectionReceiveWindow: chromeParams.InitialMaxData,
		MaxConnectionReceiveWindow:     chromeParams.InitialMaxData,
		MaxIncomingStreams:              int64(chromeParams.InitialMaxStreamsBidi),
		MaxIncomingUniStreams:           int64(chromeParams.InitialMaxStreamsUni),
		CongestionController:           s.config.CongestionControl,
	}
	ln, err := quic.ListenAddr(addr, tlsConf, quicConf)
	if err != nil {
		return fmt.Errorf("h3 listen: %w", err)
	}
	s.listener = ln
	s.logger.Info("h3 server listening (QUIC)", "addr", addr)

	go s.acceptLoop(ctx)
	return nil
}

// coverPage is a minimal HTTP/3-like response served to unauthenticated clients.
var coverPage = []byte("HTTP/3 200\r\n\r\n<!DOCTYPE html><html><body><h1>Welcome</h1></body></html>")

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		if s.closed.Load() {
			return
		}
		qconn, err := s.listener.Accept(ctx)
		if err != nil {
			if s.closed.Load() {
				return
			}
			s.logger.Error("quic accept error", "err", err)
			continue
		}
		go s.handleConn(ctx, qconn)
	}
}

func (s *Server) handleConn(ctx context.Context, qconn *quic.Conn) {
	// Accept the first stream as the control/auth stream.
	ctrlStream, err := qconn.AcceptStream(ctx)
	if err != nil {
		s.logger.Debug("failed to accept control stream", "err", err)
		qconn.CloseWithError(1, "no control stream")
		return
	}

	// Read the 64-byte auth payload: [32-byte nonce][32-byte HMAC].
	// Set a deadline so slow/malicious clients cannot hold resources indefinitely.
	_ = ctrlStream.SetReadDeadline(time.Now().Add(10 * time.Second))
	authBuf := make([]byte, 64)
	if _, err := io.ReadFull(ctrlStream, authBuf); err != nil {
		s.logger.Debug("failed to read auth payload", "err", err)
		s.serveCover(ctrlStream, qconn)
		return
	}

	nonce := authBuf[:32]
	clientMAC := authBuf[32:]

	// Check replay.
	if s.replayFilter.CheckBytes(nonce) {
		s.logger.Warn("replay detected", "remote", qconn.RemoteAddr())
		s.serveCover(ctrlStream, qconn)
		return
	}

	// Validate HMAC.
	if !auth.VerifyHMAC(nonce, clientMAC, s.config.Password) {
		s.logger.Debug("auth failed", "remote", qconn.RemoteAddr())
		s.serveCover(ctrlStream, qconn)
		return
	}

	// Auth OK.
	if _, err := ctrlStream.Write([]byte{0x01}); err != nil {
		s.logger.Debug("failed to send auth OK", "err", err)
		qconn.CloseWithError(1, "auth response failed")
		return
	}

	// Close control stream from server side.
	ctrlStream.CancelRead(0)
	ctrlStream.Close()

	conn := &h3Connection{qconn: qconn, padder: s.padder}
	select {
	case s.connCh <- conn:
	case <-ctx.Done():
		conn.Close()
	}
}

// serveCover sends a cover page and closes the connection, making the server
// appear as a normal web server to probes and unauthenticated clients.
func (s *Server) serveCover(ctrlStream *quic.Stream, qconn *quic.Conn) {
	// Send FAIL byte followed by cover content.
	_, _ = ctrlStream.Write([]byte{0x00})
	_, _ = ctrlStream.Write(coverPage)
	ctrlStream.CancelRead(0)
	ctrlStream.Close()
	qconn.CloseWithError(0, "")
}

func (s *Server) Accept(ctx context.Context) (transport.Connection, error) {
	select {
	case conn := <-s.connCh:
		return conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Server) Close() error {
	s.closed.Store(true)
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

var _ transport.ServerTransport = (*Server)(nil)
