package proxy

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"

	"github.com/shuttleX/shuttle/adapter"
)

// TUNInboundConfig configures a TUN inbound device.
type TUNInboundConfig struct {
	DeviceName string `json:"device_name,omitempty"`
	CIDR       string `json:"cidr,omitempty"`
	IPv6CIDR   string `json:"ipv6_cidr,omitempty"` // e.g. "fd00::1/64"
	MTU        int    `json:"mtu,omitempty"`
	AutoRoute  bool   `json:"auto_route,omitempty"`
	TunFD      int    `json:"tun_fd,omitempty"`
}

// TUNInbound wraps TUNServer as an adapter.Inbound.
type TUNInbound struct {
	tag    string
	config TUNInboundConfig
	server *TUNServer
	logger *slog.Logger
}

func (t *TUNInbound) Tag() string  { return t.tag }
func (t *TUNInbound) Type() string { return "tun" }

func (t *TUNInbound) Start(ctx context.Context, router adapter.InboundRouter) error {
	dialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return router.RouteConnection(ctx, &adapter.ConnMetadata{
			Destination: addr,
			Network:     network,
			Process:     ProcessFromContext(ctx),
			Protocol:    "tun",
			InboundTag:  t.tag,
		})
	}
	t.server = NewTUNServer(&TUNConfig{
		DeviceName: t.config.DeviceName,
		CIDR:       t.config.CIDR,
		IPv6CIDR:   t.config.IPv6CIDR,
		MTU:        t.config.MTU,
		AutoRoute:  t.config.AutoRoute,
		TunFD:      t.config.TunFD,
	}, dialer, t.logger)
	return t.server.Start(ctx)
}

func (t *TUNInbound) Close() error {
	if t.server != nil {
		return t.server.Close()
	}
	return nil
}

// Server returns the underlying TUNServer. MeshManager needs access
// to the TUN device for packet injection.
func (t *TUNInbound) Server() *TUNServer {
	return t.server
}

// TUNInboundFactory creates TUNInbound instances.
type TUNInboundFactory struct{}

func (f *TUNInboundFactory) Type() string { return "tun" }

func (f *TUNInboundFactory) Create(tag string, options json.RawMessage, deps adapter.InboundDeps) (adapter.Inbound, error) {
	var cfg TUNInboundConfig
	if options != nil {
		if err := json.Unmarshal(options, &cfg); err != nil {
			return nil, err
		}
	}
	return &TUNInbound{tag: tag, config: cfg, logger: deps.Logger}, nil
}

// Compile-time interface checks.
var _ adapter.Inbound = (*TUNInbound)(nil)
var _ adapter.InboundFactory = (*TUNInboundFactory)(nil)

func init() {
	adapter.RegisterInbound(&TUNInboundFactory{})
}
