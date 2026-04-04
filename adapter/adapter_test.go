package adapter_test

import (
	"context"
	"net"
	"testing"

	"github.com/shuttleX/shuttle/adapter"
)

func TestInterfacesCompile(t *testing.T) {
	var _ adapter.Stream = (adapter.Stream)(nil)
	var _ adapter.Connection = (adapter.Connection)(nil)
	var _ adapter.ClientTransport = (adapter.ClientTransport)(nil)
	var _ adapter.ServerTransport = (adapter.ServerTransport)(nil)
	var _ adapter.SecureWrapper = (adapter.SecureWrapper)(nil)
	var _ adapter.Multiplexer = (adapter.Multiplexer)(nil)
	var _ adapter.Authenticator = (adapter.Authenticator)(nil)

	var d adapter.Dialer = adapter.DialerFunc(func(ctx context.Context, network, addr string) (net.Conn, error) {
		return nil, nil
	})
	_ = d
}
