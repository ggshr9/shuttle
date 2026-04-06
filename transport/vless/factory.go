package vless

import (
	"context"
	"fmt"
	"net"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/transport/shared"
)

func init() {
	adapter.Register(&factory{})
}

type factory struct{}

func (f *factory) Type() string { return "vless" }

// VLESS doesn't use multiplexed transport — NewClient/NewServer return nil.
func (f *factory) NewClient(_ *config.ClientConfig, _ adapter.FactoryOptions) (adapter.ClientTransport, error) {
	return nil, nil
}

func (f *factory) NewServer(_ *config.ServerConfig, _ adapter.FactoryOptions) (adapter.ServerTransport, error) {
	return nil, nil
}

// NewDialer implements adapter.DialerFactory.
func (f *factory) NewDialer(cfg map[string]any, _ adapter.FactoryOptions) (adapter.Dialer, error) {
	server, _ := cfg["server"].(string)
	uuidStr, _ := cfg["uuid"].(string)

	if server == "" {
		return nil, fmt.Errorf("vless factory: missing server")
	}
	if uuidStr == "" {
		return nil, fmt.Errorf("vless factory: missing uuid")
	}

	uuid, err := parseUUID(uuidStr)
	if err != nil {
		return nil, fmt.Errorf("vless factory: %w", err)
	}

	tlsOpts := extractTLSOptions(cfg)

	d, err := NewDialer(&DialerConfig{
		Server: server,
		UUID:   uuid,
		TLS:    tlsOpts,
	})
	if err != nil {
		return nil, err
	}
	return &vlessDialerAdapter{d: d}, nil
}

// NewInboundHandler implements adapter.DialerFactory.
func (f *factory) NewInboundHandler(cfg map[string]any, _ adapter.FactoryOptions) (adapter.InboundHandler, error) {
	// Build users map from cfg["users"] or a single uuid entry.
	users := make(map[[16]byte]string)

	if rawUsers, ok := cfg["users"]; ok {
		if usersSlice, ok := rawUsers.([]any); ok {
			for _, u := range usersSlice {
				if m, ok := u.(map[string]any); ok {
					uuidStr, _ := m["uuid"].(string)
					tag, _ := m["tag"].(string)
					if uuidStr == "" {
						continue
					}
					uuid, err := parseUUID(uuidStr)
					if err != nil {
						return nil, fmt.Errorf("vless factory: invalid uuid %q: %w", uuidStr, err)
					}
					users[uuid] = tag
				}
			}
		}
	} else if uuidStr, ok := cfg["uuid"].(string); ok && uuidStr != "" {
		uuid, err := parseUUID(uuidStr)
		if err != nil {
			return nil, fmt.Errorf("vless factory: %w", err)
		}
		tag, _ := cfg["tag"].(string)
		users[uuid] = tag
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("vless factory: at least one user UUID is required")
	}

	srv, err := NewServer(ServerConfig{Users: users})
	if err != nil {
		return nil, err
	}
	return &vlessInboundAdapter{srv: srv}, nil
}

// vlessDialerAdapter adapts *Dialer to adapter.Dialer.
type vlessDialerAdapter struct {
	d *Dialer
}

func (a *vlessDialerAdapter) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return a.d.DialContext(ctx, network, address)
}

func (a *vlessDialerAdapter) Type() string { return "vless" }
func (a *vlessDialerAdapter) Close() error { return nil }

// vlessInboundAdapter adapts *Server to adapter.InboundHandler.
type vlessInboundAdapter struct {
	srv *Server
}

func (a *vlessInboundAdapter) Type() string { return "vless" }

func (a *vlessInboundAdapter) Serve(ctx context.Context, ln net.Listener, handler adapter.ConnHandler) error {
	return a.srv.Serve(ctx, ln, func(ctx context.Context, conn net.Conn, meta ConnMeta) {
		handler(ctx, conn, adapter.ConnMetadata{
			Source:      &addrString{meta.Source},
			Destination: meta.Destination,
			Network:     meta.Network,
		})
	})
}

func (a *vlessInboundAdapter) Close() error { return nil }

// addrString implements net.Addr for a plain string address.
type addrString struct{ s string }

func (a *addrString) Network() string { return "tcp" }
func (a *addrString) String() string  { return a.s }

// parseUUID parses a UUID string "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" into [16]byte.
func parseUUID(s string) ([16]byte, error) {
	var uuid [16]byte
	if len(s) != 36 {
		return uuid, fmt.Errorf("invalid UUID length: %q", s)
	}
	// Positions of hyphens: 8, 13, 18, 23
	for _, pos := range []int{8, 13, 18, 23} {
		if s[pos] != '-' {
			return uuid, fmt.Errorf("invalid UUID format (missing hyphen at pos %d): %q", pos, s)
		}
	}
	hex := s[:8] + s[9:13] + s[14:18] + s[19:23] + s[24:]
	if len(hex) != 32 {
		return uuid, fmt.Errorf("invalid UUID hex length: %q", s)
	}
	for i := 0; i < 16; i++ {
		b, err := hexByte(hex[i*2], hex[i*2+1])
		if err != nil {
			return uuid, fmt.Errorf("invalid UUID hex character: %w", err)
		}
		uuid[i] = b
	}
	return uuid, nil
}

func hexByte(hi, lo byte) (byte, error) {
	h, err := hexNibble(hi)
	if err != nil {
		return 0, err
	}
	l, err := hexNibble(lo)
	if err != nil {
		return 0, err
	}
	return h<<4 | l, nil
}

func hexNibble(c byte) (byte, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	default:
		return 0, fmt.Errorf("invalid hex character %q", c)
	}
}

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
