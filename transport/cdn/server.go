package cdn

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/crypto"
	"github.com/shuttleX/shuttle/transport"
	"github.com/shuttleX/shuttle/transport/auth"
)

// ServerConfig configures the CDN server transport.
type ServerConfig struct {
	ListenAddr string
	CertFile   string
	KeyFile    string
	Password   string
	Path       string // URL path for CDN endpoint (default "/cdn/stream")
}

// Server implements transport.ServerTransport for CDN (HTTP/2) connections.
//
// It listens on an HTTP/2+TLS endpoint and accepts POST requests at a configured
// path. Each request is authenticated via HMAC, then the HTTP connection is used
// as a bidirectional byte stream (request body = upload, response body = download)
// with yamux multiplexing on top — matching the client side in h2.go.
type Server struct {
	config     *ServerConfig
	httpServer *http.Server
	listener   net.Listener
	connCh     chan transport.Connection
	closed     atomic.Bool
	logger     *slog.Logger
	wg         sync.WaitGroup
}

// NewServer creates a new CDN server transport.
func NewServer(cfg *ServerConfig, logger *slog.Logger) *Server {
	if cfg.Path == "" {
		cfg.Path = "/cdn/stream"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		config: cfg,
		connCh: make(chan transport.Connection, 64),
		logger: logger,
	}
}

func (s *Server) Type() string { return "cdn" }

func (s *Server) Listen(ctx context.Context) error {
	tlsConf := &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
		MinVersion: tls.VersionTLS12,
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
		s.logger.Info("cdn: using auto-generated self-signed certificate")
	}

	addr := s.config.ListenAddr
	if addr == "" {
		addr = config.DefaultListenPort
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cdn listen: %w", err)
	}
	s.listener = ln

	mux := http.NewServeMux()
	mux.HandleFunc(s.config.Path, s.handleStream)

	s.httpServer = &http.Server{
		Handler:           mux,
		TLSConfig:         tlsConf,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       300 * time.Second,
	}

	s.logger.Info("cdn server listening (HTTP/2)", "addr", ln.Addr().String(), "path", s.config.Path)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		// ServeTLS with empty cert/key paths uses the TLSConfig.Certificates
		// already set above. This properly enables HTTP/2 via Go's built-in
		// h2 support (unlike tls.Listen + Serve which bypasses HTTP/2 setup).
		if err := s.httpServer.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
			if !s.closed.Load() {
				s.logger.Error("cdn http server error", "err", err)
			}
		}
	}()

	return nil
}

// handleStream processes an incoming CDN connection.
// The HTTP/2 POST body is the upload channel; the response body is the download channel.
// Together they form a bidirectional byte stream authenticated by HMAC and multiplexed by yamux.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Flush headers immediately to establish the bidirectional channel.
	// The client is waiting for a 200 OK before proceeding with auth.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.logger.Error("cdn: ResponseWriter does not implement http.Flusher")
		return
	}
	flusher.Flush()

	// Build a bidirectional duplex from the request body (read) and response writer (write).
	duplex := &serverH2Duplex{
		reader:  r.Body,
		writer:  &flushWriter{w: w, f: flusher},
		request: r,
	}
	defer duplex.Close()

	// Read and verify auth: [32-byte nonce][32-byte HMAC-SHA256(password, nonce)]
	var authBuf [64]byte
	if _, err := io.ReadFull(duplex.reader, authBuf[:]); err != nil {
		s.logger.Debug("cdn: failed to read auth payload", "err", err, "remote", r.RemoteAddr)
		return
	}

	nonce := authBuf[:32]
	clientMAC := authBuf[32:]

	if !auth.VerifyHMAC(nonce, clientMAC, s.config.Password) {
		s.logger.Debug("cdn: auth failed", "remote", r.RemoteAddr)
		return
	}

	// Auth OK — establish yamux session (server side).
	session, err := yamux.Server(duplex, yamux.DefaultConfig())
	if err != nil {
		s.logger.Error("cdn: yamux server setup failed", "err", err, "remote", r.RemoteAddr)
		return
	}

	remoteAddr := parseRemoteAddr(r.RemoteAddr)
	conn := &cdnServerConnection{
		session:    session,
		remoteAddr: remoteAddr,
		localAddr:  addrFromListener(s.listener),
	}

	s.logger.Debug("cdn: client authenticated", "remote", r.RemoteAddr)

	// Pass connection to Accept() via channel.
	select {
	case s.connCh <- conn:
	case <-r.Context().Done():
		conn.Close()
		return
	}

	// Block until the yamux session is closed. If we return from the handler,
	// the HTTP/2 stream (and its body) will be closed by the net/http server,
	// tearing down the duplex.
	<-session.CloseChan()
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
	var err error
	if s.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = s.httpServer.Shutdown(shutdownCtx)
		cancel()
	}
	s.wg.Wait()
	return err
}

// --- Duplex and helpers ---

// flushWriter wraps an http.ResponseWriter to flush after every write,
// ensuring data is sent immediately over the HTTP/2 stream.
type flushWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if n > 0 {
		fw.f.Flush()
	}
	return n, err
}

// serverH2Duplex implements io.ReadWriteCloser for the server side of an HTTP/2
// bidirectional stream. Reads come from the request body; writes go to the
// response body via a flush writer.
type serverH2Duplex struct {
	reader  io.ReadCloser // request body (upload from client)
	writer  io.Writer     // flush writer wrapping ResponseWriter (download to client)
	request *http.Request
}

func (d *serverH2Duplex) Read(p []byte) (int, error)  { return d.reader.Read(p) }
func (d *serverH2Duplex) Write(p []byte) (int, error) { return d.writer.Write(p) }
func (d *serverH2Duplex) Close() error                { return d.reader.Close() }

// cdnServerConnection wraps a yamux.Session for the server side.
type cdnServerConnection struct {
	session    *yamux.Session
	remoteAddr net.Addr
	localAddr  net.Addr
}

func (c *cdnServerConnection) OpenStream(ctx context.Context) (transport.Stream, error) {
	s, err := c.session.OpenStream()
	if err != nil {
		return nil, err
	}
	return &cdnH2Stream{stream: s}, nil
}

func (c *cdnServerConnection) AcceptStream(ctx context.Context) (transport.Stream, error) {
	s, err := c.session.AcceptStream()
	if err != nil {
		return nil, err
	}
	return &cdnH2Stream{stream: s}, nil
}

func (c *cdnServerConnection) Close() error        { return c.session.Close() }
func (c *cdnServerConnection) LocalAddr() net.Addr  { return c.localAddr }
func (c *cdnServerConnection) RemoteAddr() net.Addr { return c.remoteAddr }

// parseRemoteAddr converts an "ip:port" string to a net.TCPAddr.
func parseRemoteAddr(addr string) net.Addr {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return &net.TCPAddr{}
	}
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)
	return &net.TCPAddr{
		IP:   net.ParseIP(host),
		Port: port,
	}
}

// addrFromListener returns the listener's address or a zero-value TCPAddr.
func addrFromListener(ln net.Listener) net.Addr {
	if ln != nil {
		return ln.Addr()
	}
	return &net.TCPAddr{}
}

var _ transport.ServerTransport = (*Server)(nil)
