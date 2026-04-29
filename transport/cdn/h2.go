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
	"time"

	"github.com/shuttleX/shuttle/transport"
	ymux "github.com/shuttleX/shuttle/transport/mux/yamux"
)

// H2Config configures the CDN HTTP/2 transport.
type H2Config struct {
	ServerAddr         string
	CDNDomain          string // CDN domain (e.g., "cdn.example.com")
	Path               string // HTTP/2 stream path
	Host               string // Host header
	Password           string
	FrontDomain        string // Domain fronting: use as TLS SNI instead of CDNDomain
	InsecureSkipVerify bool   // Skip TLS certificate verification (for testing)
}

// H2Client implements transport.ClientTransport over HTTP/2 through CDN.
//
// Note: CDN H2 uses HTTP/2 POST duplex (io.ReadWriteCloser) as its underlying
// transport, not net.Conn. This means it cannot use ByteStreamClient directly.
// The HMAC auth is inline; yamux multiplexing uses the shared Mux via ClientRWC.
type H2Client struct {
	config  *H2Config
	client  *http.Client
	logger  *slog.Logger
	closed  atomic.Bool
	metrics *transport.HandshakeMetrics
}

// SetHandshakeMetrics installs the handshake metrics hook on this client.
// Must be called before Dial; the hook fires once per Dial — OnSuccess
// after the HMAC auth and yamux setup, OnFailure on any handshake error.
func (c *H2Client) SetHandshakeMetrics(m *transport.HandshakeMetrics) {
	c.metrics = m
}

func (c *H2Client) recordSuccess(d time.Duration) {
	if c.metrics != nil && c.metrics.OnSuccess != nil {
		c.metrics.OnSuccess("cdn-h2", d)
	}
}

func (c *H2Client) recordFailure(reason string) {
	if c.metrics != nil && c.metrics.OnFailure != nil {
		c.metrics.OnFailure("cdn-h2", reason)
	}
}

func NewH2Client(cfg *H2Config, opts ...H2Option) *H2Client {
	if cfg.Path == "" {
		cfg.Path = "/ws"
	}
	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		NextProtos:         []string{"h2", "http/1.1"},
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // configurable option for CDN fronting where cert may not match
	}
	if cfg.FrontDomain != "" {
		tlsCfg.ServerName = cfg.FrontDomain
	}
	c := &H2Client{
		config: cfg,
		logger: slog.Default(),
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsCfg,
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

	start := time.Now()

	// Create a bidirectional channel over HTTP/2:
	// - Client writes into io.Pipe -> becomes request body (upload)
	// - Server response body -> client reads (download)
	pr, pw := io.Pipe()

	url := fmt.Sprintf("https://%s%s", c.config.CDNDomain, c.config.Path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		c.recordFailure("protocol")
		pw.Close()
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	if c.config.Host != "" {
		req.Host = c.config.Host
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.recordFailure(classifyReason(err))
		pw.Close()
		return nil, fmt.Errorf("http2 dial: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		c.recordFailure("auth")
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
		c.recordFailure(classifyReason(err))
		duplex.Close()
		return nil, fmt.Errorf("auth handshake: %w", err)
	}

	// Establish yamux session over the duplex channel.
	mux := ymux.New(nil)
	muxConn, err := mux.ClientRWC(duplex)
	if err != nil {
		c.recordFailure("protocol")
		duplex.Close()
		return nil, fmt.Errorf("yamux client: %w", err)
	}

	c.recordSuccess(time.Since(start))

	cdnAddr := &net.TCPAddr{}
	if host, port, e := net.SplitHostPort(c.config.CDNDomain + ":443"); e == nil {
		cdnAddr.IP = net.ParseIP(host)
		fmt.Sscanf(port, "%d", &cdnAddr.Port)
	}

	c.logger.Debug("cdn-h2: connected", "addr", addr)
	return &cdnH2Connection{
		Connection: muxConn,
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

// cdnH2Connection wraps a shared yamux Connection, overriding RemoteAddr
// to return the CDN address instead of a zero-value TCPAddr.
type cdnH2Connection struct {
	transport.Connection
	remoteAddr net.Addr
}

func (c *cdnH2Connection) RemoteAddr() net.Addr { return c.remoteAddr }

var _ transport.ClientTransport = (*H2Client)(nil)
