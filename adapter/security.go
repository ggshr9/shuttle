package adapter

import (
	"context"
	"net"
)

// SecureWrapper wraps a net.Conn with a security layer (TLS, Noise, PQ-KEM, etc.).
// Multiple wrappers can be chained: the output of one feeds into the next.
type SecureWrapper interface {
	WrapClient(ctx context.Context, conn net.Conn) (net.Conn, error)
	WrapServer(ctx context.Context, conn net.Conn) (net.Conn, error)
}
