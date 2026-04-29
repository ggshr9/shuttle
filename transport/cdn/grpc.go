package cdn

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ggshr9/shuttle/transport"
	ymux "github.com/ggshr9/shuttle/transport/mux/yamux"
)

// GRPCConfig configures the gRPC CDN transport.
type GRPCConfig struct {
	ServerAddr  string
	CDNDomain   string // CDN domain
	ServiceName string // gRPC service name (e.g., "tunnel.Relay")
	Password    string
	Host        string // Optional Host header override
	FrontDomain string // Domain fronting: use as TLS SNI instead of CDNDomain
}

// GRPCClient implements transport.ClientTransport over gRPC through CDN.
type GRPCClient struct {
	config  *GRPCConfig
	client  *http.Client
	logger  *slog.Logger
	closed  atomic.Bool
	metrics *transport.HandshakeMetrics
}

// SetHandshakeMetrics installs the handshake metrics hook on this client.
// Must be called before Dial; the hook fires once per Dial — OnSuccess
// after the gRPC auth frame and yamux setup, OnFailure on any handshake
// error.
func (c *GRPCClient) SetHandshakeMetrics(m *transport.HandshakeMetrics) {
	c.metrics = m
}

func (c *GRPCClient) recordSuccess(d time.Duration) {
	if c.metrics != nil && c.metrics.OnSuccess != nil {
		c.metrics.OnSuccess("cdn-grpc", d)
	}
}

func (c *GRPCClient) recordFailure(reason string) {
	if c.metrics != nil && c.metrics.OnFailure != nil {
		c.metrics.OnFailure("cdn-grpc", reason)
	}
}

func NewGRPCClient(cfg *GRPCConfig, opts ...GRPCOption) *GRPCClient {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "tunnel.Relay"
	}
	if cfg.CDNDomain == "" {
		cfg.CDNDomain = cfg.ServerAddr
	}
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"h2"},
	}
	if cfg.FrontDomain != "" {
		tlsCfg.ServerName = cfg.FrontDomain
	}
	c := &GRPCClient{
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

// GRPCOption configures a GRPCClient.
type GRPCOption func(*GRPCClient)

// WithGRPCLogger sets the logger for the gRPC CDN client.
func WithGRPCLogger(l *slog.Logger) GRPCOption {
	return func(c *GRPCClient) { c.logger = l }
}

func (c *GRPCClient) Type() string { return "cdn-grpc" }

func (c *GRPCClient) Dial(ctx context.Context, addr string) (transport.Connection, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("grpc client closed")
	}
	c.logger.Debug("cdn-grpc: dialing", "addr", addr)

	start := time.Now()

	pr, pw := io.Pipe()

	// gRPC uses POST to /{ServiceName}/Tunnel
	path := fmt.Sprintf("/%s/Tunnel", c.config.ServiceName)
	url := fmt.Sprintf("https://%s%s", c.config.CDNDomain, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		c.recordFailure("protocol")
		pw.Close()
		return nil, fmt.Errorf("create grpc request: %w", err)
	}

	// gRPC required headers
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("TE", "trailers")
	if c.config.Host != "" {
		req.Host = c.config.Host
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.recordFailure(classifyReason(err))
		pw.Close()
		return nil, fmt.Errorf("grpc dial: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		c.recordFailure("auth")
		pw.Close()
		resp.Body.Close()
		return nil, fmt.Errorf("grpc dial: status %d", resp.StatusCode)
	}

	// Wrap the HTTP/2 duplex in gRPC framing
	duplex := &grpcDuplex{
		reader: resp.Body,
		writer: pw,
		resp:   resp,
	}

	// Send auth: [32-byte nonce][32-byte HMAC-SHA256(password, nonce)]
	if err := sendGRPCAuth(duplex, c.config.Password); err != nil {
		c.recordFailure(classifyReason(err))
		duplex.Close()
		return nil, fmt.Errorf("grpc auth: %w", err)
	}

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

	c.logger.Debug("cdn-grpc: connected", "addr", addr)
	return &cdnGRPCConnection{
		Connection: muxConn,
		remoteAddr: cdnAddr,
	}, nil
}

func (c *GRPCClient) Close() error {
	c.logger.Debug("cdn-grpc: closing")
	c.closed.Store(true)
	c.client.CloseIdleConnections()
	return nil
}

// sendGRPCAuth sends auth wrapped in a gRPC frame.
func sendGRPCAuth(w io.Writer, password string) error {
	payload, err := generateAuthPayload(password)
	if err != nil {
		return err
	}

	// Write as gRPC frame: [0x00][4-byte length][payload]
	var frame [5 + 64]byte
	frame[0] = 0 // not compressed
	binary.BigEndian.PutUint32(frame[1:5], 64)
	copy(frame[5:], payload[:])
	_, err = w.Write(frame[:])
	return err
}

// grpcDuplex wraps HTTP/2 request/response with gRPC framing.
// Writes are wrapped in gRPC frames; reads strip gRPC frame headers.
type grpcDuplex struct {
	reader io.ReadCloser
	writer io.WriteCloser
	resp   *http.Response

	// Read state: buffered payload from current gRPC frame
	rmu       sync.Mutex
	remaining []byte
}

func (d *grpcDuplex) Read(p []byte) (int, error) {
	d.rmu.Lock()
	defer d.rmu.Unlock()

	// If we have leftover data from a previous frame, return it first.
	if len(d.remaining) > 0 {
		n := copy(p, d.remaining)
		d.remaining = d.remaining[n:]
		return n, nil
	}

	// Read next gRPC frame header: [1-byte flag][4-byte length]
	var hdr [5]byte
	if _, err := io.ReadFull(d.reader, hdr[:]); err != nil {
		return 0, err
	}
	length := binary.BigEndian.Uint32(hdr[1:5])
	if length == 0 {
		return 0, nil
	}
	const maxFrameSize = 4 << 20 // 4 MB
	if length > maxFrameSize {
		return 0, fmt.Errorf("grpc frame too large: %d bytes (max %d)", length, maxFrameSize)
	}

	// Read the frame payload.
	buf := make([]byte, length)
	if _, err := io.ReadFull(d.reader, buf); err != nil {
		return 0, err
	}

	n := copy(p, buf)
	if n < len(buf) {
		d.remaining = buf[n:]
	}
	return n, nil
}

func (d *grpcDuplex) Write(p []byte) (int, error) {
	// Wrap in gRPC frame: [0x00][4-byte big-endian length][payload]
	hdr := [5]byte{0}
	binary.BigEndian.PutUint32(hdr[1:5], uint32(len(p)))
	if _, err := d.writer.Write(hdr[:]); err != nil {
		return 0, err
	}
	return d.writer.Write(p)
}

func (d *grpcDuplex) Close() error {
	d.writer.Close()
	return d.reader.Close()
}

// cdnGRPCConnection wraps a shared yamux Connection, overriding RemoteAddr
// to return the CDN address instead of a zero-value TCPAddr.
type cdnGRPCConnection struct {
	transport.Connection
	remoteAddr net.Addr
}

func (c *cdnGRPCConnection) RemoteAddr() net.Addr { return c.remoteAddr }

var _ transport.ClientTransport = (*GRPCClient)(nil)
