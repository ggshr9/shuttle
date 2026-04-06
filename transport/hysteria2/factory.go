package hysteria2

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/transport/shared"
)

func init() {
	adapter.Register(&factory{})
}

type factory struct{}

func (f *factory) Type() string { return "hysteria2" }

// Hysteria2 uses per-request dialer model — multiplexed NewClient/NewServer return nil.
func (f *factory) NewClient(_ *config.ClientConfig, _ adapter.FactoryOptions) (adapter.ClientTransport, error) {
	return nil, nil
}

func (f *factory) NewServer(_ *config.ServerConfig, _ adapter.FactoryOptions) (adapter.ServerTransport, error) {
	return nil, nil
}

// NewDialer implements adapter.DialerFactory.
func (f *factory) NewDialer(cfg map[string]any, _ adapter.FactoryOptions) (adapter.Dialer, error) {
	server, _ := cfg["server"].(string)
	password, _ := cfg["password"].(string)

	if server == "" {
		return nil, fmt.Errorf("hysteria2 factory: missing server")
	}
	if password == "" {
		return nil, fmt.Errorf("hysteria2 factory: missing password")
	}

	tlsOpts := extractTLSOptions(cfg)

	d, err := NewDialer(DialerConfig{
		Server:   server,
		Password: password,
		TLS:      tlsOpts,
	})
	if err != nil {
		return nil, err
	}
	return &hy2DialerAdapter{d: d}, nil
}

// NewInboundHandler implements adapter.DialerFactory.
func (f *factory) NewInboundHandler(cfg map[string]any, _ adapter.FactoryOptions) (adapter.InboundHandler, error) {
	password, _ := cfg["password"].(string)
	if password == "" {
		return nil, fmt.Errorf("hysteria2 factory: missing password")
	}

	tlsOpts := shared.ServerTLSOptions{}
	if v, ok := cfg["cert_file"].(string); ok {
		tlsOpts.CertFile = v
	}
	if v, ok := cfg["key_file"].(string); ok {
		tlsOpts.KeyFile = v
	}

	srv, err := NewServer(ServerConfig{
		Password: password,
		TLS:      tlsOpts,
	}, log.Default())
	if err != nil {
		return nil, err
	}
	return &hy2InboundAdapter{srv: srv}, nil
}

// hy2DialerAdapter adapts *Dialer to adapter.Dialer.
type hy2DialerAdapter struct {
	d *Dialer
}

func (a *hy2DialerAdapter) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return a.d.DialContext(ctx, network, address)
}

func (a *hy2DialerAdapter) Type() string { return "hysteria2" }
func (a *hy2DialerAdapter) Close() error { return a.d.Close() }

// hy2InboundAdapter adapts *Server to adapter.InboundHandler.
type hy2InboundAdapter struct {
	srv *Server
}

func (a *hy2InboundAdapter) Type() string { return "hysteria2" }

func (a *hy2InboundAdapter) Serve(ctx context.Context, ln net.Listener, handler adapter.ConnHandler) error {
	return a.srv.Serve(ctx, ln, handler)
}

func (a *hy2InboundAdapter) Close() error { return a.srv.Close() }

// extractTLSOptions reads TLS options from the config map.
func extractTLSOptions(cfg map[string]any) shared.TLSOptions {
	opts := shared.TLSOptions{}
	if v, ok := cfg["tls"].(bool); ok {
		opts.Enabled = v
	}
	if v, ok := cfg["sni"].(string); ok {
		opts.ServerName = v
	}
	if v, ok := cfg["insecure"].(bool); ok {
		opts.InsecureSkipVerify = v
	}
	return opts
}
