package reality

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/yamux"
	"github.com/shuttle-proxy/shuttle/crypto"
	"github.com/shuttle-proxy/shuttle/transport"
)

// ClientConfig holds configuration for a Reality client transport.
type ClientConfig struct {
	ServerAddr string
	ServerName string
	ShortID    string
	PublicKey  string
	Password   string
}

// Client implements transport.ClientTransport using Reality (TLS + Noise IK + yamux).
type Client struct {
	config *ClientConfig
	mu     sync.Mutex
	closed atomic.Bool
}

// NewClient creates a new Reality client transport.
func NewClient(cfg *ClientConfig) *Client {
	return &Client{config: cfg}
}

// Type returns the transport type identifier.
func (c *Client) Type() string { return "reality" }

// Dial establishes a Reality connection to the given address.
func (c *Client) Dial(ctx context.Context, addr string) (transport.Connection, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("reality client closed")
	}
	if addr == "" {
		addr = c.config.ServerAddr
	}

	// Step 1: TLS dial with SNI impersonation
	tlsConf := &tls.Config{
		ServerName:         c.config.ServerName,
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2", "http/1.1"},
		MinVersion:         tls.VersionTLS13,
	}
	dialer := &tls.Dialer{Config: tlsConf}
	raw, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("reality dial: %w", err)
	}

	// Step 2: Noise IK handshake
	localPub, localPriv, err := crypto.DeriveKeysFromPassword(c.config.Password)
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("derive keys: %w", err)
	}
	var remotePub [32]byte
	if c.config.PublicKey != "" {
		pubBytes, err := hex.DecodeString(c.config.PublicKey)
		if err != nil || len(pubBytes) != 32 {
			raw.Close()
			return nil, fmt.Errorf("invalid server public key")
		}
		copy(remotePub[:], pubBytes)
	}

	hs, err := crypto.NewInitiator(localPriv, localPub, remotePub)
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("noise init: %w", err)
	}

	// Send handshake message 1 (-> e, es, s, ss)
	msg1, err := hs.WriteMessage(nil)
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("noise write msg1: %w", err)
	}
	if err := writeFrame(raw, msg1); err != nil {
		raw.Close()
		return nil, fmt.Errorf("send msg1: %w", err)
	}

	// Read handshake message 2 (<- e, ee, se)
	msg2, err := readFrame(raw)
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("read msg2: %w", err)
	}
	_, err = hs.ReadMessage(msg2)
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("noise read msg2: %w", err)
	}

	if !hs.Completed() {
		raw.Close()
		return nil, fmt.Errorf("noise handshake incomplete")
	}

	// Step 3: yamux multiplexed session over the TLS connection
	sess, err := yamux.Client(raw, yamux.DefaultConfig())
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("yamux client: %w", err)
	}

	return &realityConnection{rawConn: raw, session: sess}, nil
}

// Close shuts down the client transport.
func (c *Client) Close() error {
	c.closed.Store(true)
	return nil
}

// writeFrame writes a length-prefixed frame: [2-byte big-endian length][payload].
func writeFrame(w io.Writer, data []byte) error {
	header := make([]byte, 2)
	binary.BigEndian.PutUint16(header, uint16(len(data)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// readFrame reads a length-prefixed frame from the reader.
func readFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint16(header)
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return data, nil
}

// realityConnection wraps a yamux session as a transport.Connection.
type realityConnection struct {
	rawConn net.Conn
	session *yamux.Session
}

func (c *realityConnection) OpenStream(ctx context.Context) (transport.Stream, error) {
	s, err := c.session.OpenStream()
	if err != nil {
		return nil, fmt.Errorf("yamux open: %w", err)
	}
	return &realityStream{ys: s}, nil
}

func (c *realityConnection) AcceptStream(ctx context.Context) (transport.Stream, error) {
	s, err := c.session.AcceptStream()
	if err != nil {
		return nil, fmt.Errorf("yamux accept: %w", err)
	}
	return &realityStream{ys: s}, nil
}

func (c *realityConnection) Close() error {
	c.session.Close()
	return c.rawConn.Close()
}

func (c *realityConnection) LocalAddr() net.Addr  { return c.rawConn.LocalAddr() }
func (c *realityConnection) RemoteAddr() net.Addr { return c.rawConn.RemoteAddr() }

// realityStream wraps a yamux.Stream as a transport.Stream.
type realityStream struct {
	ys *yamux.Stream
}

func (s *realityStream) StreamID() uint64            { return uint64(s.ys.StreamID()) }
func (s *realityStream) Read(p []byte) (int, error)  { return s.ys.Read(p) }
func (s *realityStream) Write(p []byte) (int, error) { return s.ys.Write(p) }
func (s *realityStream) Close() error                { return s.ys.Close() }

// Compile-time interface checks.
var _ transport.ClientTransport = (*Client)(nil)
var _ transport.Connection = (*realityConnection)(nil)
var _ transport.Stream = (*realityStream)(nil)
