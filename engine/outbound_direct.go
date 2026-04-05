package engine

import (
	"context"
	"net"

	"github.com/shuttleX/shuttle/adapter"
)

// DirectOutbound dials destinations directly without a proxy.
type DirectOutbound struct {
	tag string
}

func (o *DirectOutbound) Tag() string  { return o.tag }
func (o *DirectOutbound) Type() string { return "direct" }

func (o *DirectOutbound) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, address)
}

func (o *DirectOutbound) Close() error { return nil }

// Compile-time interface check.
var _ adapter.Outbound = (*DirectOutbound)(nil)
