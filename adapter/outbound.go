package adapter

import (
	"context"
	"net"
)

// Outbound dials remote destinations.
type Outbound interface {
	Tag() string
	Type() string
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
	Close() error
}
