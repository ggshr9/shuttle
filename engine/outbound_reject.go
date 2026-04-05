package engine

import (
	"context"
	"fmt"
	"net"

	"github.com/shuttleX/shuttle/adapter"
)

// RejectOutbound refuses all connections.
type RejectOutbound struct {
	tag string
}

func (o *RejectOutbound) Tag() string  { return o.tag }
func (o *RejectOutbound) Type() string { return "reject" }

func (o *RejectOutbound) DialContext(_ context.Context, _, _ string) (net.Conn, error) {
	return nil, fmt.Errorf("connection rejected by outbound %q", o.tag)
}

func (o *RejectOutbound) Close() error { return nil }

// Compile-time interface check.
var _ adapter.Outbound = (*RejectOutbound)(nil)
