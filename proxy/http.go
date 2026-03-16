package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shuttleX/shuttle/qos"
)

// HTTPConfig configures the HTTP proxy server.
type HTTPConfig struct {
	ListenAddr string
	Username   string
	Password   string
}

// HTTPServer implements an HTTP CONNECT proxy server.
type HTTPServer struct {
	config        *HTTPConfig
	dialer        Dialer
	listener      net.Listener
	closed        atomic.Bool
	wg            sync.WaitGroup
	logger        *slog.Logger
	ProcResolver  ProcResolver
	QoSClassifier *qos.Classifier
}

// NewHTTPServer creates a new HTTP proxy server.
func NewHTTPServer(cfg *HTTPConfig, dialer Dialer, logger *slog.Logger) *HTTPServer {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:8080"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HTTPServer{
		config: cfg,
		dialer: dialer,
		logger: logger,
	}
}

// Start begins listening for HTTP proxy connections.
func (s *HTTPServer) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("http proxy listen: %w", err)
	}
	s.listener = ln
	s.logger.Info("http proxy listening", "addr", s.config.ListenAddr)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.acceptLoop(ctx)
	}()
	return nil
}

func (s *HTTPServer) acceptLoop(ctx context.Context) {
	for {
		if s.closed.Load() {
			return
		}
		conn, err := s.listener.Accept()
		if err != nil {
			if s.closed.Load() {
				return
			}
			s.logger.Error("http proxy accept error", "err", err)
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(ctx, conn)
		}()
	}
}

func (s *HTTPServer) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	// Set handshake deadline — prevent slow-loris attacks
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// Resolve source process
	if s.ProcResolver != nil {
		if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
			if procName := s.ProcResolver.Resolve(uint16(tcpAddr.Port)); procName != "" {
				ctx = WithProcess(ctx, procName)
				s.logger.Debug("http proxy process identified", "process", procName)
			}
		}
	}

	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		s.logger.Debug("http proxy read request failed", "err", err)
		return
	}

	// Clear handshake deadline before relay
	conn.SetDeadline(time.Time{})

	if req.Method == http.MethodConnect {
		s.handleConnect(ctx, conn, req)
	} else {
		s.handleHTTP(ctx, conn, req)
	}
}

func (s *HTTPServer) handleConnect(ctx context.Context, conn net.Conn, req *http.Request) {
	target := req.Host
	if _, _, err := net.SplitHostPort(target); err != nil {
		target = net.JoinHostPort(target, "443")
	}

	s.logger.Debug("http CONNECT", "target", target)

	remote, err := s.dialer(ctx, "tcp", target)
	if err != nil {
		fmt.Fprintf(conn, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
		return
	}
	defer remote.Close()

	fmt.Fprintf(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
	proxyRelay(conn, remote)
}

func (s *HTTPServer) handleHTTP(ctx context.Context, conn net.Conn, req *http.Request) {
	target := req.Host
	if _, _, err := net.SplitHostPort(target); err != nil {
		target = net.JoinHostPort(target, "80")
	}

	remote, err := s.dialer(ctx, "tcp", target)
	if err != nil {
		fmt.Fprintf(conn, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
		return
	}
	defer remote.Close()

	// Forward the original request
	if err := req.Write(remote); err != nil {
		s.logger.Debug("http request write failed", "target", target, "err", err)
		return
	}

	// Relay response back
	if _, err := io.Copy(conn, remote); err != nil {
		s.logger.Debug("http relay copy error", "target", target, "err", err)
	}
}

// Close shuts down the HTTP proxy server.
func (s *HTTPServer) Close() error {
	s.closed.Store(true)
	if s.listener != nil {
		s.listener.Close()
	}
	// Wait for active connections with a timeout so port is released promptly.
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
	return nil
}
