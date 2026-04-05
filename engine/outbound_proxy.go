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
// transport selector, circuit breaker, retry logic, and stream metrics.
type ProxyOutbound struct {
	tag       string
	engine    *Engine
	cfg       *config.ClientConfig
	shaperCfg obfs.ShaperConfig
	retryCfg  RetryConfig
}

func (o *ProxyOutbound) Tag() string  { return o.tag }
func (o *ProxyOutbound) Type() string { return "proxy" }

func (o *ProxyOutbound) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var classifier *qos.Classifier
	if o.cfg.QoS.Enabled {
		classifier = qos.NewClassifier(&o.cfg.QoS)
	}
	return o.engine.dialProxyStream(ctx, o.cfg.Server.Addr, address, network, o.retryCfg, o.shaperCfg, classifier)
}

func (o *ProxyOutbound) Close() error { return nil }

// Compile-time interface check.
var _ adapter.Outbound = (*ProxyOutbound)(nil)
