package cdn

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/hashicorp/yamux"
	"github.com/shuttle-proxy/shuttle/transport"
)

// H2Config configures the CDN HTTP/2 transport.
type H2Config struct {
	ServerAddr string
	CDNDomain  string // CDN domain (e.g., "cdn.example.com")
	Path       string // HTTP/2 stream path
	Host       string // Host header
	Password   string
}

// H2Client implements transport.ClientTransport over HTTP/2 through CDN.
type H2Client struct {
	config *H2Config
	client *http.Client
	logger *slog.Logger
	closed atomic.Bool
}

func NewH2Client(cfg *H2Config, opts ...H2Option) *H2Client {
	if cfg.Path == "" {
		cfg.Path = "/ws"
	}
	c := &H2Client{
		config: cfg,
		logger: slog.Default(),
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					NextProtos: []string{"h2", "http/1.1"},
				},
				ForceAttemptHTTP2: true,
			},
		},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// H2Option configures an H2Client.
type H2Option func(*H2Client)

// WithH2Logger sets the logger for the H2 CDN client.
func WithH2Logger(l *slog.Logger) H2Option {
	return func(c *H2Client) { c.logger = l }
}

func (c *H2Client) Type() string { return "cdn-h2" }

func (c *H2Client) Dial(ctx context.Context, addr string) (transport.Connection, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("cdn client closed")
	}
	c.logger.Debug("cdn-h2: dialing", "addr", addr)

	// Create a bidirectional channel over HTTP/2:
	// - Client writes into io.Pipe -> becomes request body (upload)
	// - Server response body -> client reads (download)
	pr, pw := io.Pipe()

	url := fmt.Sprintf("https://%s%s", c.config.CDNDomain, c.config.Path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		pw.Close()
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	if c.config.Host != "" {
		req.Host = c.config.Host
	}

	resp, err := c.client.Do(req)
	if err != nil {
		pw.Close()
		return nil, fmt.Errorf("http2 dial: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		pw.Close()
		resp.Body.Close()
		return nil, fmt.Errorf("http2 dial: status %d", resp.StatusCode)
	}

	// The duplex channel: write to pw, read from resp.Body.
	duplex := &h2Duplex{
		reader: resp.Body,
		writer: pw,
		resp:   resp,
	}

	// Send auth: [32-byte nonce][32-byte HMAC-SHA256(password, nonce)]
	if err := sendAuth(duplex, c.config.Password); err != nil {
		duplex.Close()
		return nil, fmt.Errorf("auth handshake: %w", err)
	}

	// Establish yamux session over the duplex channel.
	session, err := yamux.Client(duplex, yamux.DefaultConfig())
	if err != nil {
		duplex.Close()
		return nil, fmt.Errorf("yamux client: %w", err)
	}

	cdnAddr := &net.TCPAddr{}
	if host, port, e := net.SplitHostPort(c.config.CDNDomain + ":443"); e == nil {
		cdnAddr.IP = net.ParseIP(host)
		fmt.Sscanf(port, "%d", &cdnAddr.Port)
	}

	c.logger.Debug("cdn-h2: connected", "addr", addr)
	return &cdnH2Connection{
		session:    session,
		remoteAddr: cdnAddr,
	}, nil
}

func (c *H2Client) Close() error {
	c.logger.Debug("cdn-h2: closing")
	c.closed.Store(true)
	c.client.CloseIdleConnections()
	return nil
}

// sendAuth writes [32-byte nonce][32-byte HMAC-SHA256(password, nonce)] to w.
func sendAuth(w io.Writer, password string) error {
	payload, err := generateAuthPayload(password)
	if err != nil {
		return err
	}
	_, err = w.Write(payload[:])
	return err
}

// h2Duplex implements io.ReadWriteCloser over an HTTP/2 POST request/response pair.
type h2Duplex struct {
	reader io.ReadCloser // response body (download)
	writer io.WriteCloser // pipe writer (upload via request body)
	resp   *http.Response
}

func (d *h2Duplex) Read(p []byte) (int, error)  { return d.reader.Read(p) }
func (d *h2Duplex) Write(p []byte) (int, error) { return d.writer.Write(p) }
func (d *h2Duplex) Close() error {
	d.writer.Close()
	return d.reader.Close()
}

// cdnH2Connection wraps a yamux.Session.
type cdnH2Connection struct {
	session    *yamux.Session
	remoteAddr net.Addr
}

func (c *cdnH2Connection) OpenStream(ctx context.Context) (transport.Stream, error) {
	s, err := c.session.OpenStream()
	if err != nil {
		return nil, err
	}
	return &cdnH2Stream{stream: s}, nil
}

func (c *cdnH2Connection) AcceptStream(ctx context.Context) (transport.Stream, error) {
	s, err := c.session.AcceptStream()
	if err != nil {
		return nil, err
	}
	return &cdnH2Stream{stream: s}, nil
}

func (c *cdnH2Connection) Close() error           { return c.session.Close() }
func (c *cdnH2Connection) LocalAddr() net.Addr     { return c.session.LocalAddr() }
func (c *cdnH2Connection) RemoteAddr() net.Addr    { return c.remoteAddr }

// cdnH2Stream wraps a yamux.Stream.
type cdnH2Stream struct {
	stream *yamux.Stream
}

func (s *cdnH2Stream) StreamID() uint64            { return uint64(s.stream.StreamID()) }
func (s *cdnH2Stream) Read(p []byte) (int, error)  { return s.stream.Read(p) }
func (s *cdnH2Stream) Write(p []byte) (int, error) { return s.stream.Write(p) }
func (s *cdnH2Stream) Close() error                { return s.stream.Close() }

var _ transport.ClientTransport = (*H2Client)(nil)
