// Package adapter defines the core transport abstractions for Shuttle.
// All subsystems import these interfaces instead of concrete transport types.
package adapter

import (
	"context"
	"io"
	"net"
)

// Stream represents a multiplexed bidirectional byte stream.
type Stream interface {
	io.ReadWriteCloser
	StreamID() uint64
}

// Connection represents a multiplexed connection that can open/accept streams.
type Connection interface {
	OpenStream(ctx context.Context) (Stream, error)
	AcceptStream(ctx context.Context) (Stream, error)
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

// ClientTransport dials servers and returns multiplexed connections.
type ClientTransport interface {
	Dial(ctx context.Context, addr string) (Connection, error)
	Type() string
	Close() error
}

// ServerTransport listens for incoming multiplexed connections.
type ServerTransport interface {
	Listen(ctx context.Context) error
	Accept(ctx context.Context) (Connection, error)
	Type() string
	Close() error
}

// NetDialer establishes a raw network connection (TCP/UDP).
// This is distinct from Dialer, which wraps per-request proxy protocols.
type NetDialer interface {
	Dial(ctx context.Context, network, addr string) (net.Conn, error)
}

// NetDialerFunc is a convenience adapter for functions that implement NetDialer.
type NetDialerFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func (f NetDialerFunc) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	return f(ctx, network, addr)
}
