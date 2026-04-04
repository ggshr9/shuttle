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

// Conn wraps a yamux.Session as an adapter.Connection.
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
