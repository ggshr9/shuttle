package engine

import (
	"encoding/json"
	"fmt"

	"github.com/ggshr9/shuttle/config"
)

// ProxyOutboundConfig is the JSON options schema for a custom proxy outbound.
// Users specify this in the outbounds config to route traffic through different
// proxy servers by tag.
//
// Example YAML:
//
//	outbounds:
//	  - tag: "us-server"
//	    type: "proxy"
//	    options:
//	      server: "us.example.com:443"
type ProxyOutboundConfig struct {
	Server string `json:"server"` // server address (host:port)
}

// createCustomProxyOutbound builds a ProxyOutbound for a user-defined server
// address. It deep-copies the client config and overrides the server address
// so the outbound dials the specified server while reusing the engine's
// transport selector and stream metrics.
func (e *Engine) createCustomProxyOutbound(tag string, options json.RawMessage, baseCfg *config.ClientConfig) (*ProxyOutbound, error) {
	var proxyCfg ProxyOutboundConfig
	if err := json.Unmarshal(options, &proxyCfg); err != nil {
		return nil, fmt.Errorf("proxy outbound %q: invalid options: %w", tag, err)
	}
	if proxyCfg.Server == "" {
		return nil, fmt.Errorf("proxy outbound %q: server address required", tag)
	}

	// Deep-copy the base config and override the server address so transport
	// setup uses the correct endpoint.
	customCfg := baseCfg.DeepCopy()
	customCfg.Server.Addr = proxyCfg.Server

	return newProxyOutboundWithTag(tag, e, customCfg), nil
}
