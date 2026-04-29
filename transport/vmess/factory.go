package vmess

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net"

	"github.com/ggshr9/shuttle/adapter"
	"github.com/ggshr9/shuttle/transport/shared"
)

func init() {
	adapter.Register(&factory{})
}

type factory struct {
	adapter.BaseFactory
}

func (f *factory) Type() string { return "vmess" }

// NewDialer implements adapter.DialerFactory.
func (f *factory) NewDialer(cfg map[string]any, _ adapter.FactoryOptions) (adapter.Dialer, error) {
	server, _ := cfg["server"].(string)
	uuidStr, _ := cfg["uuid"].(string)

	if server == "" {
		return nil, fmt.Errorf("vmess factory: missing server")
	}
	if uuidStr == "" {
		return nil, fmt.Errorf("vmess factory: missing uuid")
	}

	uuid, err := parseUUID(uuidStr)
	if err != nil {
		return nil, fmt.Errorf("vmess factory: %w", err)
	}

	security := SecurityAES128GCM
	if secStr, ok := cfg["security"].(string); ok {
		switch secStr {
		case "aes-128-gcm":
			security = SecurityAES128GCM
		case "none":
			security = SecurityNone
		default:
			return nil, fmt.Errorf("vmess factory: unknown security %q", secStr)
		}
	}

	tlsOpts := extractTLSOptions(cfg)

	d, err := NewDialer(&DialerConfig{
		Server:   server,
		UUID:     uuid,
		Security: security,
		TLS:      tlsOpts,
	})
	if err != nil {
		return nil, err
	}
	return &vmessDialerAdapter{d: d}, nil
}

// NewInboundHandler implements adapter.DialerFactory.
func (f *factory) NewInboundHandler(cfg map[string]any, _ adapter.FactoryOptions) (adapter.InboundHandler, error) {
	users := make(map[[16]byte]string)

	// Support "users" as a list of {uuid, tag} or a map of uuid->tag.
	if rawUsers, ok := cfg["users"]; ok {
		switch v := rawUsers.(type) {
		case map[string]any:
			for uuidStr, tag := range v {
				uuid, err := parseUUID(uuidStr)
				if err != nil {
					return nil, fmt.Errorf("vmess factory: %w", err)
				}
				tagStr, _ := tag.(string)
				users[uuid] = tagStr
			}
		case []any:
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					uuidStr, _ := m["uuid"].(string)
					tag, _ := m["tag"].(string)
					uuid, err := parseUUID(uuidStr)
					if err != nil {
						return nil, fmt.Errorf("vmess factory: %w", err)
					}
					users[uuid] = tag
				}
			}
		}
	} else if uuidStr, ok := cfg["uuid"].(string); ok && uuidStr != "" {
		uuid, err := parseUUID(uuidStr)
		if err != nil {
			return nil, fmt.Errorf("vmess factory: %w", err)
		}
		users[uuid] = ""
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("vmess factory: at least one user UUID is required")
	}

	srv := NewServer(ServerConfig{Users: users}, log.Default())
	return &vmessInboundAdapter{srv: srv}, nil
}

// vmessDialerAdapter adapts *Dialer to adapter.Dialer.
type vmessDialerAdapter struct {
	d *Dialer
}

func (a *vmessDialerAdapter) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return a.d.DialContext(ctx, network, address)
}

func (a *vmessDialerAdapter) Type() string { return "vmess" }
func (a *vmessDialerAdapter) Close() error { return nil }

// vmessInboundAdapter adapts *Server to adapter.InboundHandler.
type vmessInboundAdapter struct {
	srv *Server
}

func (a *vmessInboundAdapter) Type() string { return "vmess" }

func (a *vmessInboundAdapter) Serve(ctx context.Context, ln net.Listener, handler adapter.ConnHandler) error {
	return a.srv.Serve(ctx, ln, func(ctx context.Context, cmd byte, address string, conn net.Conn) {
		network := "tcp"
		if cmd == CmdUDP {
			network = "udp"
		}
		handler(ctx, conn, adapter.ConnMetadata{
			Source:      conn.RemoteAddr(),
			Destination: address,
			Network:     network,
		})
	})
}

func (a *vmessInboundAdapter) Close() error { return nil }

// parseUUID parses a UUID string (with or without dashes) into a 16-byte array.
func parseUUID(s string) ([16]byte, error) {
	var uuid [16]byte

	// Strip dashes
	clean := make([]byte, 0, 32)
	for _, c := range []byte(s) {
		if c != '-' {
			clean = append(clean, c)
		}
	}

	if len(clean) != 32 {
		return uuid, fmt.Errorf("invalid UUID %q: expected 32 hex chars, got %d", s, len(clean))
	}

	decoded, err := hex.DecodeString(string(clean))
	if err != nil {
		return uuid, fmt.Errorf("invalid UUID %q: %w", s, err)
	}
	copy(uuid[:], decoded)
	return uuid, nil
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
