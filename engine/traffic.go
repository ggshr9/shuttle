package engine

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/ggshr9/shuttle/adapter"
	"github.com/ggshr9/shuttle/config"
	"github.com/ggshr9/shuttle/plugin"
	"github.com/ggshr9/shuttle/proxy"
	"github.com/ggshr9/shuttle/router"
)

// TrafficManager owns data-plane wiring: dialer construction, inbound/outbound
// setup, and the routing bridge between inbounds and outbounds.
type TrafficManager struct {
	logger *slog.Logger
}

// NewTrafficManager creates a TrafficManager with the given logger.
func NewTrafficManager(logger *slog.Logger) *TrafficManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &TrafficManager{logger: logger}
}

// BuildBuiltinOutbounds creates the always-present direct/reject/proxy outbounds.
// If eng is nil, the proxy outbound is omitted (useful for testing).
func (tm *TrafficManager) BuildBuiltinOutbounds(cfg *config.ClientConfig, eng *Engine) map[string]adapter.Outbound {
	outbounds := map[string]adapter.Outbound{
		"direct": &DirectOutbound{tag: "direct"},
		"reject": &RejectOutbound{tag: "reject"},
	}
	if eng != nil {
		outbounds["proxy"] = newProxyOutbound(eng, cfg)
	}
	return outbounds
}

// BuildInboundRouter creates an inboundRouter that bridges routing decisions
// to outbound selection.
func (tm *TrafficManager) BuildInboundRouter(
	rt *router.Router,
	dnsResolver *router.DNSResolver,
	outbounds map[string]adapter.Outbound,
	defaultOut adapter.Outbound,
) adapter.InboundRouter {
	return &inboundRouter{
		routerEngine: rt,
		dnsResolver:  dnsResolver,
		outbounds:    outbounds,
		defaultOut:   defaultOut,
		logger:       tm.logger,
	}
}

// CreateDialer builds a dialer function that routes connections through the
// router. Connections matching ActionDirect are dialled directly, ActionReject
// returns an error, and all others use the provided dialProxy callback.
func (tm *TrafficManager) CreateDialer(
	cfg *config.ClientConfig,
	rt *router.Router,
	dnsResolver *router.DNSResolver,
	dialProxy func(ctx context.Context, serverAddr, addr, network string) (net.Conn, error),
) func(context.Context, string, string) (net.Conn, error) {
	serverAddr := cfg.Server.Addr
	return func(dialCtx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
			port = ""
		}

		ip, err := resolveTarget(dialCtx, host, dnsResolver)
		if err != nil {
			return nil, err
		}

		procName := proxy.ProcessFromContext(dialCtx)
		action := rt.Match(host, ip, procName, "", 0, nil)

		switch action {
		case router.ActionDirect:
			return (&net.Dialer{}).DialContext(dialCtx, network, net.JoinHostPort(ip.String(), port))
		case router.ActionReject:
			return nil, fmt.Errorf("rejected: %s", addr)
		default:
			return dialProxy(dialCtx, serverAddr, addr, network)
		}
	}
}

// WrapDialerWithChain wraps a dialer so that every successfully dialled
// connection is run through the plugin chain's OnConnect hooks. When the
// connection is closed, OnDisconnect is called via the chainConn wrapper.
func (tm *TrafficManager) WrapDialerWithChain(
	dialer func(context.Context, string, string) (net.Conn, error),
	chain *plugin.Chain,
) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dialer(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		wrapped, err := chain.OnConnect(conn, addr)
		if err != nil {
			conn.Close()
			return nil, err
		}
		return &chainConn{Conn: wrapped, chain: chain}, nil
	}
}
