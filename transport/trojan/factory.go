package trojan

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

func (f *factory) Type() string { return "trojan" }

// NewDialer implements adapter.DialerFactory.
func (f *factory) NewDialer(cfg map[string]any, _ adapter.FactoryOptions) (adapter.Dialer, error) {
	server, _ := cfg["server"].(string)
	password, _ := cfg["password"].(string)

	if server == "" {
		return nil, fmt.Errorf("trojan factory: missing server")
	}
	if password == "" {
		return nil, fmt.Errorf("trojan factory: missing password")
	}

	tlsOpts := extractTLSOptions(cfg)

	d, err := NewDialer(&DialerConfig{
		Server:   server,
		Password: password,
		TLS:      tlsOpts,
	})
	if err != nil {
		return nil, err
	}
	return &trojanDialerAdapter{d: d}, nil
}

// NewInboundHandler implements adapter.DialerFactory.
func (f *factory) NewInboundHandler(cfg map[string]any, _ adapter.FactoryOptions) (adapter.InboundHandler, error) {
	// Build passwords map. Support either "passwords" (map[string]string) or single "password".
	passwords := make(map[string]string)

	if rawPasswords, ok := cfg["passwords"]; ok {
		if pm, ok := rawPasswords.(map[string]any); ok {
			for hash, tag := range pm {
				if tagStr, ok := tag.(string); ok {
					passwords[hash] = tagStr
				}
			}
		}
	} else if password, ok := cfg["password"].(string); ok && password != "" {
		passwords[HashPassword(password)] = ""
	}

	if len(passwords) == 0 {
		return nil, fmt.Errorf("trojan factory: at least one password is required")
	}

	fallback, _ := cfg["fallback"].(string)

	srv := NewServer(ServerConfig{
		Passwords: passwords,
		Fallback:  fallback,
	}, log.Default())

	return &trojanInboundAdapter{srv: srv}, nil
}

// trojanDialerAdapter adapts *Dialer to adapter.Dialer.
type trojanDialerAdapter struct {
	d *Dialer
}

func (a *trojanDialerAdapter) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return a.d.DialContext(ctx, network, address)
}

func (a *trojanDialerAdapter) Type() string { return "trojan" }
func (a *trojanDialerAdapter) Close() error { return nil }

// trojanInboundAdapter adapts *Server to adapter.InboundHandler.
type trojanInboundAdapter struct {
	srv *Server
}

func (a *trojanInboundAdapter) Type() string { return "trojan" }

func (a *trojanInboundAdapter) Serve(ctx context.Context, ln net.Listener, handler adapter.ConnHandler) error {
	return a.srv.Serve(ctx, ln, func(ctx context.Context, cmd byte, address string, conn net.Conn) {
		network := "tcp"
		if cmd == shared.CmdUDPAssociate {
			network = "udp"
		}
		handler(ctx, conn, adapter.ConnMetadata{
			Source:      conn.RemoteAddr(),
			Destination: address,
			Network:     network,
		})
	})
}

func (a *trojanInboundAdapter) Close() error { return nil }

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
