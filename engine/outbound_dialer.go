package engine

import (
	"context"
	"net"

	"github.com/shuttleX/shuttle/adapter"
)

// DialerOutbound wraps an adapter.Dialer (per-protocol: SS, VLESS, Trojan, etc.)
// to satisfy the adapter.Outbound interface used by the engine's outbound table.
type DialerOutbound struct {
	tag    string
	dialer adapter.Dialer
}

// NewDialerOutbound creates a DialerOutbound with the given tag and underlying dialer.
func NewDialerOutbound(tag string, d adapter.Dialer) *DialerOutbound {
	return &DialerOutbound{tag: tag, dialer: d}
}

func (o *DialerOutbound) Tag() string  { return o.tag }
func (o *DialerOutbound) Type() string { return o.dialer.Type() }

func (o *DialerOutbound) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return o.dialer.DialContext(ctx, network, address)
}

func (o *DialerOutbound) Close() error {
	return o.dialer.Close()
}

// Compile-time interface check.
var _ adapter.Outbound = (*DialerOutbound)(nil)
