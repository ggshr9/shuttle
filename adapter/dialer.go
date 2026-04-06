package adapter

import (
	"context"
	"net"
)

// Dialer is the interface for per-request protocols (Shadowsocks, VLESS, Trojan, etc.)
// that create a new connection for each request instead of multiplexing.
// The DialContext signature matches Outbound.DialContext for seamless integration.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
	Type() string
	Close() error
}

// InboundHandler handles incoming connections for per-request protocol servers.
type InboundHandler interface {
	Type() string
	Serve(ctx context.Context, listener net.Listener, handler ConnHandler) error
	Close() error
}

// ConnHandler is invoked by InboundHandler when a new proxied connection arrives.
type ConnHandler func(ctx context.Context, conn net.Conn, metadata ConnMetadata)
