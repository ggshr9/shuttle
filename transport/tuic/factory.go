package tuic

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/transport/shared"
)

func init() {
	adapter.Register(&factory{})
}

type factory struct {
	adapter.BaseFactory
}

func (f *factory) Type() string { return "tuic" }

// NewDialer implements adapter.DialerFactory.
func (f *factory) NewDialer(cfg map[string]any, _ adapter.FactoryOptions) (adapter.Dialer, error) {
	server, _ := cfg["server"].(string)
	uuid, _ := cfg["uuid"].(string)
	password, _ := cfg["password"].(string)

	if server == "" {
		return nil, fmt.Errorf("tuic factory: missing server")
	}
	if uuid == "" {
		return nil, fmt.Errorf("tuic factory: missing uuid")
	}
	if password == "" {
		return nil, fmt.Errorf("tuic factory: missing password")
	}

	tlsOpts := extractTLSOptions(cfg)

	d, err := NewDialer(&DialerConfig{
		Server:   server,
		UUID:     uuid,
		Password: password,
		TLS:      tlsOpts,
	})
	if err != nil {
		return nil, err
	}
	return &tuicDialerAdapter{d: d}, nil
}

// NewInboundHandler implements adapter.DialerFactory.
func (f *factory) NewInboundHandler(cfg map[string]any, _ adapter.FactoryOptions) (adapter.InboundHandler, error) {
	users := make(map[string]string)

	// Support "users" as a map[string]any of uuid->password
	if usersRaw, ok := cfg["users"].(map[string]any); ok {
		for uuid, pw := range usersRaw {
			if pwStr, ok2 := pw.(string); ok2 {
				users[uuid] = pwStr
			}
		}
	}

	// Also support single-user via uuid+password fields
	if uuid, ok := cfg["uuid"].(string); ok {
		if pw, ok2 := cfg["password"].(string); ok2 {
			users[uuid] = pw
		}
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("tuic factory: missing users (uuid:password pairs)")
	}

	tlsOpts := shared.ServerTLSOptions{}
	if v, ok := cfg["cert_file"].(string); ok {
		tlsOpts.CertFile = v
	}
	if v, ok := cfg["key_file"].(string); ok {
		tlsOpts.KeyFile = v
	}

	srv, err := NewServer(ServerConfig{
		Users: users,
		TLS:   tlsOpts,
	}, log.Default())
	if err != nil {
		return nil, err
	}
	return &tuicInboundAdapter{srv: srv}, nil
}

// tuicDialerAdapter adapts *Dialer to adapter.Dialer.
type tuicDialerAdapter struct {
	d *Dialer
}

func (a *tuicDialerAdapter) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return a.d.DialContext(ctx, network, address)
}

func (a *tuicDialerAdapter) Type() string { return "tuic" }
func (a *tuicDialerAdapter) Close() error { return a.d.Close() }

// tuicInboundAdapter adapts *Server to adapter.InboundHandler.
type tuicInboundAdapter struct {
	srv *Server
}

func (a *tuicInboundAdapter) Type() string { return "tuic" }

func (a *tuicInboundAdapter) Serve(ctx context.Context, ln net.Listener, handler adapter.ConnHandler) error {
	return a.srv.Serve(ctx, ln, handler)
}

func (a *tuicInboundAdapter) Close() error { return a.srv.Close() }

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
	if v, ok := cfg["alpn"].([]string); ok {
		opts.ALPN = v
	}
	return opts
}
