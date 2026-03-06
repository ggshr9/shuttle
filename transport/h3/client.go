package h3

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/shuttle-proxy/shuttle/obfs"
	"github.com/shuttle-proxy/shuttle/transport"
	"github.com/shuttle-proxy/shuttle/transport/auth"
)

type ClientConfig struct {
	ServerAddr        string
	ServerName        string
	Password          string
	Fingerprint       *FingerprintConfig
	PathPrefix        string
	CongestionControl quic.CongestionControl // optional custom CC (e.g., BBR/Brutal adaptive)
}

type Client struct {
	config *ClientConfig
	mu     sync.Mutex
	conn   *h3Connection
	closed atomic.Bool
	padder *obfs.Padder
}

func NewClient(cfg *ClientConfig) *Client {
	if cfg.PathPrefix == "" {
		cfg.PathPrefix = "/cdn/stream/"
	}
	if cfg.Fingerprint == nil {
		cfg.Fingerprint = DefaultFingerprint()
	}
	return &Client{
		config: cfg,
		padder: obfs.NewPadder(0),
	}
}

func (c *Client) Type() string { return "h3" }

// computeSessionAuth generates [32-byte nonce][32-byte HMAC-SHA256(password, nonce)].
func computeSessionAuth(password string) ([]byte, error) {
	payload, err := auth.GenerateHMAC(password)
	if err != nil {
		return nil, err
	}
	return payload[:], nil
}

func (c *Client) Dial(ctx context.Context, addr string) (transport.Connection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed.Load() {
		return nil, fmt.Errorf("h3 client closed")
	}
	if addr == "" {
		addr = c.config.ServerAddr
	}

	chromeParams := DefaultChromeTransportParams()

	tlsConf := &tls.Config{
		ServerName:       c.config.ServerName,
		NextProtos:       ChromeALPN,
		MinVersion:       tls.VersionTLS13,
		CipherSuites:     ChromeCipherSuites,
		CurvePreferences: ChromeCurvePreferences,
	}
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
		CongestionController:           c.config.CongestionControl,
	}

	qconn, err := quic.DialAddr(ctx, addr, tlsConf, quicConf)
	if err != nil {
		return nil, fmt.Errorf("h3 dial: %w", err)
	}

	// HTTP/3-style session establishment on control stream.
	ctrlStream, err := qconn.OpenStreamSync(ctx)
	if err != nil {
		qconn.CloseWithError(1, "control stream open failed")
		return nil, fmt.Errorf("h3 open control stream: %w", err)
	}
	defer func() { ctrlStream.CancelRead(0); ctrlStream.Close() }()

	authPayload, err := computeSessionAuth(c.config.Password)
	if err != nil {
		qconn.CloseWithError(1, "auth generation failed")
		return nil, fmt.Errorf("h3 auth: %w", err)
	}

	if _, err := ctrlStream.Write(authPayload); err != nil {
		qconn.CloseWithError(1, "auth write failed")
		return nil, fmt.Errorf("h3 write auth: %w", err)
	}

	// Read 1-byte server response.
	resp := make([]byte, 1)
	if _, err := io.ReadFull(ctrlStream, resp); err != nil {
		qconn.CloseWithError(1, "auth response read failed")
		return nil, fmt.Errorf("h3 read auth response: %w", err)
	}
	if resp[0] != 0x01 {
		qconn.CloseWithError(2, "auth rejected")
		return nil, fmt.Errorf("h3 auth rejected by server")
	}

	h3conn := &h3Connection{qconn: qconn, padder: c.padder}
	c.conn = h3conn
	return h3conn, nil
}

func (c *Client) Close() error {
	c.closed.Store(true)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// h3Connection wraps a quic.Conn with obfuscation padding.
type h3Connection struct {
	qconn  *quic.Conn
	padder *obfs.Padder
}

func (c *h3Connection) OpenStream(ctx context.Context) (transport.Stream, error) {
	qs, err := c.qconn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("open quic stream: %w", err)
	}
	return &h3Stream{qs: qs, padder: c.padder}, nil
}

func (c *h3Connection) AcceptStream(ctx context.Context) (transport.Stream, error) {
	qs, err := c.qconn.AcceptStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("accept quic stream: %w", err)
	}
	return &h3Stream{qs: qs, padder: c.padder}, nil
}

func (c *h3Connection) Close() error {
	return c.qconn.CloseWithError(0, "closing")
}

func (c *h3Connection) LocalAddr() net.Addr  { return c.qconn.LocalAddr() }
func (c *h3Connection) RemoteAddr() net.Addr { return c.qconn.RemoteAddr() }

// h3Stream wraps a quic.Stream with optional padding.
type h3Stream struct {
	qs     *quic.Stream
	padder *obfs.Padder

	// Read buffer: data from a frame that didn't fit in the caller's buffer.
	readBuf []byte
}

func (s *h3Stream) StreamID() uint64 { return uint64(s.qs.StreamID()) }

func (s *h3Stream) Read(p []byte) (int, error) {
	// Return buffered data from a previous read first.
	if len(s.readBuf) > 0 {
		n := copy(p, s.readBuf)
		s.readBuf = s.readBuf[n:]
		return n, nil
	}

	// Read one padded frame and extract original data.
	data, err := s.padder.ReadFrame(s.qs)
	if err != nil {
		return 0, err
	}

	n := copy(p, data)
	if n < len(data) {
		s.readBuf = data[n:]
	}
	return n, nil
}

func (s *h3Stream) Write(p []byte) (int, error) {
	padded, err := s.padder.Pad(p)
	if err != nil {
		return 0, err
	}
	_, err = s.qs.Write(padded)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *h3Stream) Close() error {
	s.qs.CancelRead(0)
	return s.qs.Close()
}

var _ transport.ClientTransport = (*Client)(nil)
var _ transport.Connection = (*h3Connection)(nil)
var _ transport.Stream = (*h3Stream)(nil)
