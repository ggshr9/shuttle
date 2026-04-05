package engine

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/proxy"
	"github.com/shuttleX/shuttle/router"
)

// inboundRouter implements adapter.InboundRouter, bridging inbound traffic
// through the routing engine to the appropriate outbound.
type inboundRouter struct {
	routerEngine *router.Router
	dnsResolver  *router.DNSResolver
	outbounds    map[string]adapter.Outbound // tag → outbound
	defaultOut   adapter.Outbound
	logger       *slog.Logger
}

func (r *inboundRouter) RouteConnection(ctx context.Context, meta *adapter.ConnMetadata) (net.Conn, error) {
	host, port, err := net.SplitHostPort(meta.Destination)
	if err != nil {
		host = meta.Destination
		port = ""
	}

	ip, err := resolveTarget(ctx, host, r.dnsResolver)
	if err != nil {
		return nil, err
	}

	procName := meta.Process
	if procName == "" {
		procName = proxy.ProcessFromContext(ctx)
	}

	action := r.routerEngine.Match(host, ip, procName, meta.Protocol)

	var out adapter.Outbound
	switch action {
	case router.ActionDirect:
		out = r.outbounds["direct"]
	case router.ActionReject:
		out = r.outbounds["reject"]
	default:
		out = r.defaultOut
	}
	if out == nil {
		return nil, fmt.Errorf("no outbound for action %v", action)
	}

	// For direct outbound, dial the resolved IP so DNS results are used.
	// For proxy outbound, dial the original hostname (server resolves it).
	if out.Type() == "direct" && ip != nil && port != "" {
		return out.DialContext(ctx, meta.Network, net.JoinHostPort(ip.String(), port))
	}
	return out.DialContext(ctx, meta.Network, meta.Destination)
}

// Compile-time interface check.
var _ adapter.InboundRouter = (*inboundRouter)(nil)
