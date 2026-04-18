package subscription

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/shuttleX/shuttle/config"
)

// clashTypeToAdapterType maps Clash/sing-box protocol type names to Shuttle adapter types.
var clashTypeToAdapterType = map[string]string{
	"ss":          "shadowsocks",
	"trojan":      "trojan",
	"vmess":       "vmess",
	"vless":       "vless",
	"hysteria2":   "hysteria2",
	"hysteria":    "hysteria2",
	"wireguard":   "wireguard",
	"shadowsocks": "shadowsocks", // sing-box already uses full names
}

// sanitizeTag converts a name to a valid outbound tag:
// lowercase, spaces and colons replaced with dashes.
func sanitizeTag(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, ":", "-")
	return s
}

// ToOutboundConfigs converts ServerEndpoints to OutboundConfigs.
// Each server becomes a "proxy" type outbound with server address in options.
// Tags are derived from the server name (sanitized), falling back to the addr.
// Duplicate tags are deduplicated by appending "-2", "-3", etc.
func ToOutboundConfigs(servers []config.ServerEndpoint) []config.OutboundConfig {
	out := make([]config.OutboundConfig, 0, len(servers))
	seen := make(map[string]int, len(servers))

	for _, s := range servers {
		base := sanitizeTag(s.Name)
		if base == "" {
			base = sanitizeTag(s.Addr)
		}

		tag := base
		if count, ok := seen[base]; ok {
			// count tracks how many times we have already emitted this base.
			// The first duplicate becomes "-2", the next "-3", etc.
			count++
			seen[base] = count
			tag = fmt.Sprintf("%s-%d", base, count)
		} else {
			seen[base] = 1
		}

		var outboundType string
		var opts json.RawMessage

		if adapterType, known := clashTypeToAdapterType[s.Type]; known {
			outboundType = adapterType
			opts = buildAdapterOptions(&s, adapterType)
		} else {
			outboundType = "proxy"
			raw, _ := json.Marshal(map[string]string{"server": s.Addr})
			opts = json.RawMessage(raw)
		}

		out = append(out, config.OutboundConfig{
			Tag:     tag,
			Type:    outboundType,
			Options: opts,
		})
	}

	return out
}

// buildAdapterOptions constructs the options map for a known-protocol adapter outbound.
// It splits Addr into server/server_port, includes password and SNI when present,
// then merges all entries from ServerEndpoint.Options (which take precedence).
//
// Key normalization is applied per adapterType to bridge Clash YAML field names
// to the keys expected by each transport factory:
//   - vless/vmess: Password is written as "uuid" (factories read cfg["uuid"])
//   - shadowsocks: "cipher" (from Clash YAML) is renamed to "method" after merging
func buildAdapterOptions(s *config.ServerEndpoint, adapterType string) json.RawMessage {
	m := make(map[string]any)

	host, portStr, err := net.SplitHostPort(s.Addr)
	if err == nil {
		m["server"] = host
		if port, err := strconv.Atoi(portStr); err == nil {
			m["server_port"] = port
		}
	} else {
		// Addr not in host:port form — store as-is so nothing is silently lost.
		m["server"] = s.Addr
	}

	// Key normalization: VLESS/VMess factories read "uuid", not "password".
	if s.Password != "" {
		switch adapterType {
		case "vless", "vmess":
			m["uuid"] = s.Password
		default:
			m["password"] = s.Password
		}
	}

	if s.SNI != "" {
		m["sni"] = s.SNI
	}

	// Merge extra options; keys from Options override the defaults above.
	for k, v := range s.Options {
		m[k] = v
	}

	// Key normalization: Shadowsocks factory reads "method"; Clash YAML uses "cipher".
	if adapterType == "shadowsocks" {
		if cipher, ok := m["cipher"]; ok {
			m["method"] = cipher
			delete(m, "cipher")
		}
	}

	raw, _ := json.Marshal(m)
	return json.RawMessage(raw)
}

// groupOptions is the JSON structure written into a group OutboundConfig.
type groupOptions struct {
	Strategy   string   `json:"strategy"`
	Outbounds  []string `json:"outbounds"`
	MaxLatency string   `json:"max_latency"`
	MaxLossRate float64  `json:"max_loss_rate"`
}

// ToGroupConfig creates a quality-group OutboundConfig wrapping all given outbounds.
func ToGroupConfig(tag string, outbounds []config.OutboundConfig) config.OutboundConfig {
	tags := make([]string, len(outbounds))
	for i, ob := range outbounds {
		tags[i] = ob.Tag
	}

	opts, _ := json.Marshal(groupOptions{
		Strategy:    "quality",
		Outbounds:   tags,
		MaxLatency:  "500ms",
		MaxLossRate: 0.05,
	})

	return config.OutboundConfig{
		Tag:     tag,
		Type:    "group",
		Options: json.RawMessage(opts),
	}
}
