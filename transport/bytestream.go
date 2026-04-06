package transport

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/shuttleX/shuttle/adapter"
)

// ByteStreamConfig configures a generic byte-stream transport pipeline.
type ByteStreamConfig struct {
	Addr     string                  // Server address (host:port).
	Dialer   adapter.NetDialer        // Establishes the raw connection.
	Security []adapter.SecureWrapper // Security chain applied in order. May be nil.
	Auth     adapter.Authenticator   // Authentication after security. May be nil.
	Mux      adapter.Multiplexer     // Stream multiplexer (required).
	TypeName string                  // Transport type identifier.
}

type byteStreamClient struct {
	cfg    ByteStreamConfig
	closed atomic.Bool
}

// NewByteStreamClient creates a ClientTransport by composing
// dialer → security chain → authenticator → multiplexer.
func NewByteStreamClient(cfg *ByteStreamConfig) adapter.ClientTransport {
	return &byteStreamClient{cfg: *cfg}
}

func (c *byteStreamClient) Dial(ctx context.Context, addr string) (adapter.Connection, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("bytestream %s: client closed", c.cfg.TypeName)
	}
	if addr == "" {
		addr = c.cfg.Addr
	}

	raw, err := c.cfg.Dialer.Dial(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("bytestream %s dial: %w", c.cfg.TypeName, err)
	}

	success := false
	defer func() {
		if !success {
			raw.Close()
		}
	}()

	conn := raw
	for _, wrapper := range c.cfg.Security {
		conn, err = wrapper.WrapClient(ctx, conn)
		if err != nil {
			return nil, fmt.Errorf("bytestream %s security: %w", c.cfg.TypeName, err)
		}
	}

	if c.cfg.Auth != nil {
		if err := c.cfg.Auth.AuthClient(conn); err != nil {
			return nil, fmt.Errorf("bytestream %s auth: %w", c.cfg.TypeName, err)
		}
	}

	muxConn, err := c.cfg.Mux.Client(conn)
	if err != nil {
		return nil, fmt.Errorf("bytestream %s mux: %w", c.cfg.TypeName, err)
	}

	success = true
	return muxConn, nil
}

func (c *byteStreamClient) Type() string { return c.cfg.TypeName }

func (c *byteStreamClient) Close() error {
	c.closed.Store(true)
	return nil
}

var _ adapter.ClientTransport = (*byteStreamClient)(nil)
