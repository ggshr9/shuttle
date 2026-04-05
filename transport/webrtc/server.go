package webrtc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/shuttleX/shuttle/crypto"
	"github.com/shuttleX/shuttle/transport"
)

// ServerConfig holds configuration for a WebRTC server transport.
type ServerConfig struct {
	SignalListen string
	CertFile     string
	KeyFile      string
	Password     string
	STUNServers  []string
	TURNServers  []string
	TURNUser     string
	TURNPass     string
	ICEPolicy    string // "all", "relay", "public" (default "all")
	LoopbackOnly bool   // restrict ICE to 127.0.0.1, disable mDNS (for testing)
}

// Server implements transport.ServerTransport using WebRTC DataChannels.
type Server struct {
	config       *ServerConfig
	httpServer   *http.Server
	connCh       chan transport.Connection
	closed       atomic.Bool
	logger       *slog.Logger
	replayFilter *crypto.ReplayFilter
}

// NewServer creates a new WebRTC server transport.
func NewServer(cfg *ServerConfig, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	// Only fill defaults when STUNServers is nil (not explicitly set).
	// An explicit empty slice []string{} means "no STUN servers".
	if cfg.STUNServers == nil {
		cfg.STUNServers = []string{
			"stun:stun.l.google.com:19302",
			"stun:stun1.l.google.com:19302",
		}
	}
	return &Server{
		config:       cfg,
		connCh:       make(chan transport.Connection, 64),
		logger:       logger,
		replayFilter: crypto.NewReplayFilter(120 * time.Second),
	}
}

// Type returns the transport type identifier.
func (s *Server) Type() string { return "webrtc" }

// Listen starts the HTTPS signaling server.
func (s *Server) Listen(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webrtc/signal", s.handleSignal)
	mux.HandleFunc("GET /webrtc/ws", s.handleWebSocket)
	// Cover behavior: non-POST or wrong path returns 404
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	addr := s.config.SignalListen
	if addr == "" {
		addr = ":8443"
	}

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Load TLS config if certs provided
	if s.config.CertFile != "" && s.config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.config.CertFile, s.config.KeyFile)
		if err != nil {
			return fmt.Errorf("webrtc load tls: %w", err)
		}
		s.httpServer.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	}

	s.logger.Info("webrtc signaling server listening", "addr", addr)

	go func() {
		var err error
		if s.httpServer.TLSConfig != nil {
			err = s.httpServer.ListenAndServeTLS("", "")
		} else {
			err = s.httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			s.logger.Error("webrtc signal server error", "err", err)
		}
	}()

	return nil
}

// Accept returns the next authenticated WebRTC connection.
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
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// Compile-time interface check.
var _ transport.ServerTransport = (*Server)(nil)
