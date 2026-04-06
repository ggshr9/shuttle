package subscription

import (
	"fmt"
	"strings"

	"github.com/shuttleX/shuttle/config"
	"gopkg.in/yaml.v3"
)

// clashConfig is used only for format detection via isClashFormat.
type clashConfig struct {
	Proxies []clashProxy `yaml:"proxies"`
}

// clashProxy is kept for backward-compat with isClashFormat detection.
type clashProxy struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

// isClashFormat returns true if data looks like a Clash YAML subscription
// (contains a "proxies:" key with at least one entry).
func isClashFormat(data []byte) bool {
	// Quick heuristic: must contain "proxies:" at the start of a line.
	if !containsProxiesKey(data) {
		return false
	}

	var cfg clashConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return false
	}
	return len(cfg.Proxies) > 0
}

// containsProxiesKey checks if the raw YAML bytes contain a top-level
// "proxies:" key (line-start match to avoid false positives).
func containsProxiesKey(data []byte) bool {
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if trimmed == "proxies:" || strings.HasPrefix(trimmed, "proxies:") {
			return true
		}
	}
	return false
}

// promotedFields are fields extracted into ServerEndpoint top-level fields
// and must NOT be stored in Options.
var promotedFields = map[string]bool{
	"name":      true,
	"type":      true,
	"server":    true,
	"port":      true,
	"password":  true,
	"uuid":      true,
	"sni":       true,
	"servername": true,
	"servname":  true,
}

// parseClash parses a Clash YAML subscription and returns a slice of
// ServerEndpoint values. Supported proxy types: ss, trojan, vmess, vless,
// hysteria2, hysteria, wireguard. All transport options are preserved in
// ServerEndpoint.Options. Returns an error if data is invalid or contains no proxies.
func parseClash(data []byte) ([]config.ServerEndpoint, error) {
	// Parse the raw YAML into a map to capture all fields generically.
	var raw struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse clash yaml: %w", err)
	}

	if len(raw.Proxies) == 0 {
		return nil, fmt.Errorf("no proxies found in clash config")
	}

	var endpoints []config.ServerEndpoint
	for _, p := range raw.Proxies {
		proxyType := strings.ToLower(stringField(p, "type"))
		switch proxyType {
		case "ss", "trojan", "vmess", "vless", "hysteria2", "hysteria", "wireguard":
			// Build the endpoint.
			server := stringField(p, "server")
			port := intField(p, "port")

			ep := config.ServerEndpoint{
				Name: stringField(p, "name"),
				Addr: fmt.Sprintf("%s:%d", server, port),
				Type: proxyType,
			}

			// Password: vmess/vless use "uuid", all others use "password".
			switch proxyType {
			case "vmess", "vless":
				ep.Password = stringField(p, "uuid")
			default:
				ep.Password = stringField(p, "password")
			}

			// SNI: try "sni" first, then "servername", then "servname".
			ep.SNI = stringField(p, "sni")
			if ep.SNI == "" {
				ep.SNI = stringField(p, "servername")
			}
			if ep.SNI == "" {
				ep.SNI = stringField(p, "servname")
			}

			// Collect all remaining fields into Options.
			options := make(map[string]any)
			for k, v := range p {
				if !promotedFields[k] {
					options[k] = v
				}
			}
			if len(options) > 0 {
				ep.Options = options
			}

			endpoints = append(endpoints, ep)
		}
	}

	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no supported proxies found in clash config")
	}
	return endpoints, nil
}

// stringField extracts a string value from a map, returning "" if absent or wrong type.
func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// intField extracts an integer value from a map, returning 0 if absent or wrong type.
func intField(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case int64:
		return int(n)
	}
	return 0
}
