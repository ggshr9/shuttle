package engine

import (
	"context"
	"net"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/obfs"
	"github.com/shuttleX/shuttle/qos"
)

// ProxyOutbound routes connections through the proxy server via the engine's
// transport selector and stream metrics. It is a pure dialer with no embedded
// resilience logic; wrap with ResilientOutbound to add retry and circuit breaker.
type ProxyOutbound struct {
	tag        string
	engine     *Engine
	serverAddr string
	shaperCfg  obfs.ShaperConfig
	classifier *qos.Classifier // pre-built at construction, nil if QoS disabled
}

func (o *ProxyOutbound) Tag() string  { return o.tag }
func (o *ProxyOutbound) Type() string { return "proxy" }

func (o *ProxyOutbound) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return o.engine.dialProxyStream(ctx, o.serverAddr, address, network, o.shaperCfg, o.classifier)
}

func (o *ProxyOutbound) Close() error { return nil }

// newProxyOutbound creates the default proxy outbound tagged "proxy".
func newProxyOutbound(e *Engine, cfg *config.ClientConfig) *ProxyOutbound {
	return newProxyOutboundWithTag("proxy", e, cfg)
}

// newProxyOutboundWithTag creates a proxy outbound with a custom tag.
// This allows multiple proxy outbounds pointing to different servers.
func newProxyOutboundWithTag(tag string, e *Engine, cfg *config.ClientConfig) *ProxyOutbound {
	var classifier *qos.Classifier
	if cfg.QoS.Enabled {
		classifier = qos.NewClassifier(&cfg.QoS)
	}
	return &ProxyOutbound{
		tag:        tag,
		engine:     e,
		serverAddr: cfg.Server.Addr,
		shaperCfg:  e.buildShaperConfig(cfg.Obfs),
		classifier: classifier,
	}
}

// Compile-time interface check.
var _ adapter.Outbound = (*ProxyOutbound)(nil)
