package adapter

import (
	"context"
	"net"
)

// ConnMetadata carries per-connection routing context.
type ConnMetadata struct {
	Source      net.Addr
	Destination string
	Network     string
	Process     string
	Protocol    string
	InboundTag  string
}

// InboundRouter is how inbounds request outbound connections.
type InboundRouter interface {
	RouteConnection(ctx context.Context, metadata *ConnMetadata) (net.Conn, error)
}

// Inbound accepts local connections and routes them via InboundRouter.
type Inbound interface {
	Tag() string
	Type() string
	Start(ctx context.Context, router InboundRouter) error
	Close() error
}
