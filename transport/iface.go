package transport

import (
	"context"
	"io"
	"net"
)

// Stream represents a multiplexed stream within a connection.
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

// TransportConfig holds common transport configuration.
type TransportConfig struct {
	ServerAddr   string
	ServerName   string
	Password     string
	InsecureSkip bool
}
