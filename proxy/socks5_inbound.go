package proxy

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"

	"github.com/shuttleX/shuttle/adapter"
)

// SOCKS5InboundConfig configures a SOCKS5 inbound listener.
type SOCKS5InboundConfig struct {
	Listen   string `json:"listen"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// SOCKS5Inbound wraps SOCKS5Server as an adapter.Inbound.
type SOCKS5Inbound struct {
	tag    string
	config SOCKS5InboundConfig
	server *SOCKS5Server
	logger *slog.Logger
}

func (s *SOCKS5Inbound) Tag() string  { return s.tag }
func (s *SOCKS5Inbound) Type() string { return "socks5" }

func (s *SOCKS5Inbound) Start(ctx context.Context, router adapter.InboundRouter) error {
	dialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return router.RouteConnection(ctx, &adapter.ConnMetadata{
			Destination: addr,
			Network:     network,
			Process:     ProcessFromContext(ctx),
			Protocol:    "socks5",
			InboundTag:  s.tag,
		})
	}
	s.server = NewSOCKS5Server(&SOCKS5Config{
		ListenAddr: s.config.Listen,
		Username:   s.config.Username,
		Password:   s.config.Password,
	}, dialer, s.logger)
	return s.server.Start(ctx)
}

func (s *SOCKS5Inbound) Close() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// SOCKS5InboundFactory creates SOCKS5Inbound instances.
type SOCKS5InboundFactory struct{}

func (f *SOCKS5InboundFactory) Type() string { return "socks5" }

func (f *SOCKS5InboundFactory) Create(tag string, options json.RawMessage, deps adapter.InboundDeps) (adapter.Inbound, error) {
	var cfg SOCKS5InboundConfig
	if options != nil {
		if err := json.Unmarshal(options, &cfg); err != nil {
			return nil, err
		}
	}
	return &SOCKS5Inbound{tag: tag, config: cfg, logger: deps.Logger}, nil
}

// Compile-time interface checks.
var _ adapter.Inbound = (*SOCKS5Inbound)(nil)
var _ adapter.InboundFactory = (*SOCKS5InboundFactory)(nil)

func init() {
	adapter.RegisterInbound(&SOCKS5InboundFactory{})
}
