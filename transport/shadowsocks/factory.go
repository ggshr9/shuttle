package shadowsocks

import (
	"context"
	"fmt"
	"net"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
)

func init() {
	adapter.Register(&factory{})
}

type factory struct{}

func (f *factory) Type() string { return "shadowsocks" }

// Shadowsocks doesn't use multiplexed transport — NewClient/NewServer return nil.
func (f *factory) NewClient(_ *config.ClientConfig, _ adapter.FactoryOptions) (adapter.ClientTransport, error) {
	return nil, nil
}

func (f *factory) NewServer(_ *config.ServerConfig, _ adapter.FactoryOptions) (adapter.ServerTransport, error) {
	return nil, nil
}

// NewDialer implements adapter.DialerFactory.
func (f *factory) NewDialer(cfg map[string]any, _ adapter.FactoryOptions) (adapter.Dialer, error) {
	server, _ := cfg["server"].(string)
	method, _ := cfg["method"].(string)
	password, _ := cfg["password"].(string)

	if server == "" {
		return nil, fmt.Errorf("shadowsocks factory: missing server")
	}
	if method == "" {
		return nil, fmt.Errorf("shadowsocks factory: missing method")
	}
	if password == "" {
		return nil, fmt.Errorf("shadowsocks factory: missing password")
	}

	d, err := NewDialer(DialerConfig{
		Server:   server,
		Method:   method,
		Password: password,
	})
	if err != nil {
		return nil, err
	}
	return &ssDialerAdapter{d: d}, nil
}

// NewInboundHandler implements adapter.DialerFactory.
func (f *factory) NewInboundHandler(cfg map[string]any, _ adapter.FactoryOptions) (adapter.InboundHandler, error) {
	method, _ := cfg["method"].(string)
	password, _ := cfg["password"].(string)

	if method == "" {
		return nil, fmt.Errorf("shadowsocks factory: missing method")
	}
	if password == "" {
		return nil, fmt.Errorf("shadowsocks factory: missing password")
	}

	srv, err := NewServer(ServerConfig{
		Method:   method,
		Password: password,
	})
	if err != nil {
		return nil, err
	}
	return &ssInboundAdapter{srv: srv}, nil
}

// ssDialerAdapter adapts *Dialer to adapter.Dialer.
type ssDialerAdapter struct {
	d *Dialer
}

func (a *ssDialerAdapter) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return a.d.DialContext(ctx, network, address)
}

func (a *ssDialerAdapter) Type() string  { return "shadowsocks" }
func (a *ssDialerAdapter) Close() error  { return nil }

// ssInboundAdapter adapts *Server to adapter.InboundHandler.
type ssInboundAdapter struct {
	srv *Server
}

func (a *ssInboundAdapter) Type() string { return "shadowsocks" }

func (a *ssInboundAdapter) Serve(ctx context.Context, ln net.Listener, handler adapter.ConnHandler) error {
	return a.srv.Serve(ctx, ln, func(ctx context.Context, conn net.Conn, meta ConnMeta) {
		handler(ctx, conn, adapter.ConnMetadata{
			Source:      &addrString{meta.Source},
			Destination: meta.Destination,
			Network:     meta.Network,
		})
	})
}

func (a *ssInboundAdapter) Close() error { return nil }

// addrString implements net.Addr for a plain string address.
type addrString struct{ s string }

func (a *addrString) Network() string { return "tcp" }
func (a *addrString) String() string  { return a.s }
