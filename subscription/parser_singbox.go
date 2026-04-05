package subscription

import (
	"encoding/json"
	"fmt"

	"github.com/shuttleX/shuttle/config"
)

// skipOutboundTypes lists sing-box outbound types that are not proxy servers.
var skipOutboundTypes = map[string]bool{
	"direct":   true,
	"block":    true,
	"dns":      true,
	"selector": true,
	"urltest":  true,
}

// singboxConfig is a minimal representation of a sing-box configuration file.
type singboxConfig struct {
	Outbounds []singboxOutbound `json:"outbounds"`
}

// singboxOutbound represents a single outbound entry in a sing-box config.
type singboxOutbound struct {
	Type       string      `json:"type"`
	Tag        string      `json:"tag"`
	Server     string      `json:"server"`
	ServerPort int         `json:"server_port"`
	Password   string      `json:"password"`
	Method     string      `json:"method"`
	TLS        *singboxTLS `json:"tls,omitempty"`
}

// singboxTLS holds TLS-related fields for a sing-box outbound.
type singboxTLS struct {
	ServerName string `json:"server_name"`
}

// isSingboxFormat reports whether data looks like a sing-box JSON config.
// It checks for the presence of an "outbounds" key in the top-level JSON object.
func isSingboxFormat(data []byte) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	_, ok := probe["outbounds"]
	return ok
}

// parseSingbox parses a sing-box JSON configuration and returns proxy server endpoints.
// Non-proxy outbound types (direct, block, dns, selector, urltest) are skipped.
func parseSingbox(data []byte) ([]config.ServerEndpoint, error) {
	var cfg singboxConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse sing-box JSON: %w", err)
	}

	servers := make([]config.ServerEndpoint, 0, len(cfg.Outbounds))
	for _, ob := range cfg.Outbounds {
		if skipOutboundTypes[ob.Type] {
			continue
		}
		if ob.Server == "" || ob.ServerPort == 0 {
			continue
		}

		ep := config.ServerEndpoint{
			Addr:     fmt.Sprintf("%s:%d", ob.Server, ob.ServerPort),
			Name:     ob.Tag,
			Password: ob.Password,
		}
		if ob.TLS != nil {
			ep.SNI = ob.TLS.ServerName
		}
		servers = append(servers, ep)
	}
	return servers, nil
}
