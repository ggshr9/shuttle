// Package yamux provides a Multiplexer adapter for hashicorp/yamux.
package yamux

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
)

// Mux implements adapter.Multiplexer using yamux.
type Mux struct {
	cfg *yamux.Config
}

// New creates a yamux Multiplexer. If cfg is nil, yamux defaults are used.
func New(userCfg *config.YamuxConfig) *Mux {
	c := yamux.DefaultConfig()
	c.LogOutput = io.Discard
	if userCfg != nil {
		if userCfg.MaxStreamWindowSize > 0 {
			c.MaxStreamWindowSize = userCfg.MaxStreamWindowSize
		}
		if userCfg.KeepAliveInterval > 0 {
			c.KeepAliveInterval = time.Duration(userCfg.KeepAliveInterval) * time.Second
		}
		if userCfg.ConnectionWriteTimeout > 0 {
			c.ConnectionWriteTimeout = time.Duration(userCfg.ConnectionWriteTimeout) * time.Second
		}
	}
	return &Mux{cfg: c}
}

func (m *Mux) Client(conn net.Conn) (adapter.Connection, error) {
	sess, err := yamux.Client(conn, m.cfg)
	if err != nil {
		return nil, fmt.Errorf("yamux client: %w", err)
	}
	return &Conn{session: sess, raw: conn}, nil
}

func (m *Mux) Server(conn net.Conn) (adapter.Connection, error) {
	sess, err := yamux.Server(conn, m.cfg)
	if err != nil {
		return nil, fmt.Errorf("yamux server: %w", err)
	}
	return &Conn{session: sess, raw: conn}, nil
}

var _ adapter.Multiplexer = (*Mux)(nil)

// ClientRWC creates a client yamux session over an io.ReadWriteCloser.
// Use this for transports where the underlying connection is not a net.Conn
// (e.g., WebRTC DataChannel, HTTP/2 duplex).
// LocalAddr/RemoteAddr will return zero-value TCPAddr.
func (m *Mux) ClientRWC(rwc io.ReadWriteCloser) (adapter.Connection, error) {
	sess, err := yamux.Client(rwc, m.cfg)
	if err != nil {
		return nil, fmt.Errorf("yamux client: %w", err)
	}
	return &RWCConn{session: sess, rwc: rwc}, nil
}

// ServerRWC creates a server yamux session over an io.ReadWriteCloser.
func (m *Mux) ServerRWC(rwc io.ReadWriteCloser) (adapter.Connection, error) {
	sess, err := yamux.Server(rwc, m.cfg)
	if err != nil {
		return nil, fmt.Errorf("yamux server: %w", err)
	}
	return &RWCConn{session: sess, rwc: rwc}, nil
}

// Conn wraps a yamux.Session as an adapter.Connection (over net.Conn).
type Conn struct {
	session *yamux.Session
	raw     net.Conn
}

func (c *Conn) OpenStream(ctx context.Context) (adapter.Stream, error) {
	s, err := c.session.OpenStream()
	if err != nil {
		return nil, fmt.Errorf("yamux open: %w", err)
	}
	return &Stream{ys: s}, nil
}

func (c *Conn) AcceptStream(ctx context.Context) (adapter.Stream, error) {
	s, err := c.session.AcceptStream()
	if err != nil {
		return nil, fmt.Errorf("yamux accept: %w", err)
	}
	return &Stream{ys: s}, nil
}

func (c *Conn) Close() error {
	c.session.Close()
	return c.raw.Close()
}

func (c *Conn) LocalAddr() net.Addr  { return c.raw.LocalAddr() }
func (c *Conn) RemoteAddr() net.Addr { return c.raw.RemoteAddr() }

var _ adapter.Connection = (*Conn)(nil)

// Stream wraps a yamux.Stream as an adapter.Stream.
type Stream struct {
	ys *yamux.Stream
}

func (s *Stream) StreamID() uint64            { return uint64(s.ys.StreamID()) }
func (s *Stream) Read(p []byte) (int, error)  { return s.ys.Read(p) }
func (s *Stream) Write(p []byte) (int, error) { return s.ys.Write(p) }
func (s *Stream) Close() error                { return s.ys.Close() }

var _ adapter.Stream = (*Stream)(nil)

// RWCConn wraps a yamux.Session over an io.ReadWriteCloser (no net.Conn).
// Used for WebRTC DataChannels, HTTP/2 duplex, and similar non-socket transports.
type RWCConn struct {
	session *yamux.Session
	rwc     io.ReadWriteCloser
}

func (c *RWCConn) OpenStream(ctx context.Context) (adapter.Stream, error) {
	s, err := c.session.OpenStream()
	if err != nil {
		return nil, fmt.Errorf("yamux open: %w", err)
	}
	return &Stream{ys: s}, nil
}

func (c *RWCConn) AcceptStream(ctx context.Context) (adapter.Stream, error) {
	s, err := c.session.AcceptStream()
	if err != nil {
		return nil, fmt.Errorf("yamux accept: %w", err)
	}
	return &Stream{ys: s}, nil
}

func (c *RWCConn) Close() error {
	c.session.Close()
	return c.rwc.Close()
}

func (c *RWCConn) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (c *RWCConn) RemoteAddr() net.Addr { return &net.TCPAddr{} }

var _ adapter.Connection = (*RWCConn)(nil)
