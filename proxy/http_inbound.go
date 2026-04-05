package proxy

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"

	"github.com/shuttleX/shuttle/adapter"
)

// HTTPInboundConfig configures an HTTP CONNECT inbound listener.
type HTTPInboundConfig struct {
	Listen   string `json:"listen"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// HTTPInbound wraps HTTPServer as an adapter.Inbound.
type HTTPInbound struct {
	tag    string
	config HTTPInboundConfig
	server *HTTPServer
	logger *slog.Logger
}

func (h *HTTPInbound) Tag() string  { return h.tag }
func (h *HTTPInbound) Type() string { return "http" }

func (h *HTTPInbound) Start(ctx context.Context, router adapter.InboundRouter) error {
	dialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return router.RouteConnection(ctx, &adapter.ConnMetadata{
			Destination: addr,
			Network:     network,
			Process:     ProcessFromContext(ctx),
			Protocol:    "http",
			InboundTag:  h.tag,
		})
	}
	h.server = NewHTTPServer(&HTTPConfig{
		ListenAddr: h.config.Listen,
		Username:   h.config.Username,
		Password:   h.config.Password,
	}, dialer, h.logger)
	return h.server.Start(ctx)
}

func (h *HTTPInbound) Close() error {
	if h.server != nil {
		return h.server.Close()
	}
	return nil
}

// HTTPInboundFactory creates HTTPInbound instances.
type HTTPInboundFactory struct{}

func (f *HTTPInboundFactory) Type() string { return "http" }

func (f *HTTPInboundFactory) Create(tag string, options json.RawMessage, deps adapter.InboundDeps) (adapter.Inbound, error) {
	var cfg HTTPInboundConfig
	if options != nil {
		if err := json.Unmarshal(options, &cfg); err != nil {
			return nil, err
		}
	}
	return &HTTPInbound{tag: tag, config: cfg, logger: deps.Logger}, nil
}

// Compile-time interface checks.
var _ adapter.Inbound = (*HTTPInbound)(nil)
var _ adapter.InboundFactory = (*HTTPInboundFactory)(nil)

func init() {
	adapter.RegisterInbound(&HTTPInboundFactory{})
}
