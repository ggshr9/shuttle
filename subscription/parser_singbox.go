package subscription

import (
	"encoding/json"
	"fmt"

	"github.com/ggshr9/shuttle/config"
)

// skipOutboundTypes lists sing-box outbound types that are not proxy servers.
var skipOutboundTypes = map[string]bool{
	"direct":   true,
	"block":    true,
	"dns":      true,
	"selector": true,
	"urltest":  true,
}

// singboxPromotedFields are fields extracted into ServerEndpoint top-level
// fields and must NOT be stored in Options.
// Note: "tls" is intentionally NOT promoted — the full TLS block must pass
// through in Options so factories can access enabled, insecure, alpn, etc.
// SNI is extracted separately from full["tls"]["server_name"] below.
var singboxPromotedFields = map[string]bool{
	"type":        true,
	"tag":         true,
	"server":      true,
	"server_port": true,
	"password":    true,
	"uuid":        true,
}

// singboxBaseOutbound holds only the fields we promote to ServerEndpoint.
type singboxBaseOutbound struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Password   string `json:"password"`
	UUID       string `json:"uuid"`
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

// parseSingbox parses a sing-box JSON configuration and returns proxy server
// endpoints. Non-proxy outbound types (direct, block, dns, selector, urltest)
// are skipped. All non-promoted fields are preserved in ServerEndpoint.Options.
func parseSingbox(data []byte) ([]config.ServerEndpoint, error) {
	// Unmarshal the outbounds array as raw messages so we can double-unmarshal.
	var wrapper struct {
		Outbounds []json.RawMessage `json:"outbounds"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("parse sing-box JSON: %w", err)
	}

	servers := make([]config.ServerEndpoint, 0, len(wrapper.Outbounds))
	for _, raw := range wrapper.Outbounds {
		// Unmarshal promoted fields into the typed base struct.
		var base singboxBaseOutbound
		if err := json.Unmarshal(raw, &base); err != nil {
			continue
		}

		// Skip non-proxy types.
		if skipOutboundTypes[base.Type] {
			continue
		}
		// Skip entries without a valid server address.
		if base.Server == "" || base.ServerPort == 0 {
			continue
		}

		// Unmarshal all fields into a generic map.
		var full map[string]any
		if err := json.Unmarshal(raw, &full); err != nil {
			continue
		}

		// Build the endpoint with promoted fields.
		ep := config.ServerEndpoint{
			Addr: fmt.Sprintf("%s:%d", base.Server, base.ServerPort),
			Name: base.Tag,
			Type: base.Type,
		}

		// Password: prefer "password", fall back to "uuid".
		ep.Password = base.Password
		if ep.Password == "" {
			ep.Password = base.UUID
		}

		// SNI: extract from tls.server_name in the full map.
		if tlsRaw, ok := full["tls"]; ok {
			if tlsMap, ok := tlsRaw.(map[string]any); ok {
				if sn, ok := tlsMap["server_name"].(string); ok {
					ep.SNI = sn
				}
			}
		}

		// Collect all non-promoted fields into Options.
		options := make(map[string]any)
		for k, v := range full {
			if !singboxPromotedFields[k] {
				options[k] = v
			}
		}
		if len(options) > 0 {
			ep.Options = options
		}

		servers = append(servers, ep)
	}
	return servers, nil
}
