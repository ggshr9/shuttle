package engine

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"

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

	// Fake-ip reverse lookup: if the destination IP is a fake IP, recover the
	// original domain so routing rules can match on it and the proxy outbound
	// sends the real hostname to the remote server.
	if ip != nil && r.dnsResolver.IsFakeIP(ip) {
		if domain, ok := r.dnsResolver.ReverseFakeIP(ip); ok {
			host = domain
			if port != "" {
				meta.Destination = net.JoinHostPort(domain, port)
			} else {
				meta.Destination = domain
			}
			// Clear resolved IP so the direct outbound path below will
			// re-resolve the real domain to a real IP.
			ip = nil
		}
	}

	procName := meta.Process
	if procName == "" {
		procName = proxy.ProcessFromContext(ctx)
	}

	// Extract destination port for port-based routing.
	var dstPort uint16
	if port != "" {
		if pn, err := strconv.ParseUint(port, 10, 16); err == nil {
			dstPort = uint16(pn)
		}
	}

	// Extract source IP for source-IP-based routing.
	var srcIP net.IP
	if meta.Source != nil {
		if tcpAddr, ok := meta.Source.(*net.TCPAddr); ok {
			srcIP = tcpAddr.IP
		}
	}

	action := r.routerEngine.Match(host, ip, procName, meta.Protocol, dstPort, srcIP)

	// Look up outbound: try well-known actions first, then treat the action
	// string as a custom outbound tag, falling back to the default outbound.
	var out adapter.Outbound
	switch action {
	case router.ActionDirect:
		out = r.outbounds["direct"]
	case router.ActionReject:
		out = r.outbounds["reject"]
	case router.ActionProxy:
		out = r.defaultOut
	default:
		// Custom outbound tag (e.g., action: "my-outbound")
		if ob, ok := r.outbounds[string(action)]; ok {
			out = ob
		} else {
			out = r.defaultOut
		}
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
