package engine

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDialer struct {
	dialCalled bool
	typ        string
}

func (m *mockDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	m.dialCalled = true
	return nil, net.ErrClosed
}
func (m *mockDialer) Type() string { return m.typ }
func (m *mockDialer) Close() error { return nil }

func TestDialerOutbound_ForwardsDialAndType(t *testing.T) {
	md := &mockDialer{typ: "shadowsocks"}
	ob := NewDialerOutbound("my-ss", md)

	assert.Equal(t, "my-ss", ob.Tag())
	assert.Equal(t, "shadowsocks", ob.Type())

	_, err := ob.DialContext(context.Background(), "tcp", "example.com:80")
	require.Error(t, err)
	assert.True(t, md.dialCalled)
}
