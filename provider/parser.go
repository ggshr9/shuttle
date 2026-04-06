package provider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Format represents the detected format of a proxy list.
type Format string

const (
	FormatClash    Format = "clash"
	FormatSingbox  Format = "singbox"
	FormatBase64URI Format = "base64-uri"
	FormatPlainURI Format = "plain-uri"
	FormatUnknown  Format = "unknown"
)

// ProxyNode represents a single proxy entry parsed from any format.
type ProxyNode struct {
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	Server  string         `json:"server"`
	Port    int            `json:"port"`
	Options map[string]any `json:"options"`
}

// uriSchemes lists URI schemes that indicate a proxy URI list.
var uriSchemes = []string{
	"ss://", "vless://", "trojan://", "vmess://",
	"hysteria2://", "hy2://", "tuic://", "naive://",
	"socks5://", "http://", "https://",
}

// DetectFormat inspects data and returns the most likely proxy list format.
// Detection order:
//  1. JSON with "outbounds" key → FormatSingbox
//  2. YAML with "proxies" key → FormatClash
//  3. Lines starting with a known URI scheme → FormatPlainURI
//  4. Base64-decode then re-check for URI scheme → FormatBase64URI
//  5. FormatUnknown
func DetectFormat(data []byte) Format {
	// 1. Try singbox JSON
	if isSingboxJSON(data) {
		return FormatSingbox
	}

	// 2. Try Clash YAML
	if isClashYAML(data) {
		return FormatClash
	}

	// 3. Try plain URI list
	if isPlainURIList(data) {
		return FormatPlainURI
	}

	// 4. Try base64-encoded URI list
	if decoded, ok := tryBase64Decode(data); ok && isPlainURIList(decoded) {
		return FormatBase64URI
	}

	return FormatUnknown
}

// ParseProxyList auto-detects format and returns a slice of ProxyNode.
func ParseProxyList(data []byte) ([]ProxyNode, error) {
	switch DetectFormat(data) {
	case FormatSingbox:
		return parseSingboxOutbounds(data)
	case FormatClash:
		return parseClashProxies(data)
	case FormatPlainURI:
		return parseURIList(data)
	case FormatBase64URI:
		decoded, _ := tryBase64Decode(data)
		return parseURIList(decoded)
	default:
		return nil, fmt.Errorf("unknown proxy list format")
	}
}

// ── format detection helpers ──────────────────────────────────────────────────

func isSingboxJSON(data []byte) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	_, ok := probe["outbounds"]
	return ok
}

func isClashYAML(data []byte) bool {
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if trimmed == "proxies:" || strings.HasPrefix(trimmed, "proxies:") {
			return true
		}
	}
	return false
}

func isPlainURIList(data []byte) bool {
	content := strings.TrimSpace(string(data))
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, scheme := range uriSchemes {
			if strings.HasPrefix(line, scheme) {
				return true
			}
		}
		return false // first non-empty line doesn't match any scheme
	}
	return false
}

func tryBase64Decode(data []byte) ([]byte, bool) {
	s := strings.TrimSpace(string(data))
	// Remove any whitespace (base64 sometimes has newlines)
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, " ", "")

	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(s)
	}
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(s)
	}
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(s)
	}
	if err != nil {
		return nil, false
	}
	return decoded, true
}

// ── format-specific parsers ───────────────────────────────────────────────────

// clashConfig is a minimal Clash YAML structure.
type clashConfig struct {
	Proxies []map[string]any `yaml:"proxies"`
}

// parseClashProxies unmarshals a Clash YAML subscription and maps each entry
// to a ProxyNode. The name, type, server, and port fields are promoted; all
// remaining fields are stored in Options.
func parseClashProxies(data []byte) ([]ProxyNode, error) {
	var cfg clashConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse clash yaml: %w", err)
	}
	if len(cfg.Proxies) == 0 {
		return nil, fmt.Errorf("no proxies found in clash config")
	}

	nodes := make([]ProxyNode, 0, len(cfg.Proxies))
	for _, p := range cfg.Proxies {
		node := ProxyNode{
			Options: make(map[string]any),
		}
		for k, v := range p {
			switch k {
			case "name":
				if s, ok := v.(string); ok {
					node.Name = s
				}
			case "type":
				if s, ok := v.(string); ok {
					node.Type = s
				}
			case "server":
				if s, ok := v.(string); ok {
					node.Server = s
				}
			case "port":
				switch pv := v.(type) {
				case int:
					node.Port = pv
				case float64:
					node.Port = int(pv)
				}
			default:
				node.Options[k] = v
			}
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// singboxConfig is a minimal sing-box JSON structure.
type singboxConfig struct {
	Outbounds []singboxOutbound `json:"outbounds"`
}

type singboxOutbound struct {
	Type       string         `json:"type"`
	Tag        string         `json:"tag"`
	Server     string         `json:"server"`
	ServerPort int            `json:"server_port"`
	Extra      map[string]any `json:"-"`
}

// parseSingboxOutbounds unmarshals a sing-box JSON config and maps each
// outbound entry to a ProxyNode. tag→Name, type→Type, server→Server,
// server_port→Port. Non-proxy types (direct, block, dns, selector, urltest)
// are skipped.
func parseSingboxOutbounds(data []byte) ([]ProxyNode, error) {
	var cfg singboxConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse singbox JSON: %w", err)
	}

	// Also unmarshal the raw outbounds for extra options
	var raw struct {
		Outbounds []map[string]json.RawMessage `json:"outbounds"`
	}
	_ = json.Unmarshal(data, &raw)

	skipTypes := map[string]bool{
		"direct": true, "block": true, "dns": true,
		"selector": true, "urltest": true,
	}

	nodes := make([]ProxyNode, 0, len(cfg.Outbounds))
	for i, ob := range cfg.Outbounds {
		if skipTypes[ob.Type] {
			continue
		}

		node := ProxyNode{
			Name:    ob.Tag,
			Type:    ob.Type,
			Server:  ob.Server,
			Port:    ob.ServerPort,
			Options: make(map[string]any),
		}

		// Populate Options from raw map, excluding promoted fields
		promoted := map[string]bool{
			"type": true, "tag": true, "server": true, "server_port": true,
		}
		if i < len(raw.Outbounds) {
			for k, v := range raw.Outbounds[i] {
				if promoted[k] {
					continue
				}
				var val any
				if err := json.Unmarshal(v, &val); err == nil {
					node.Options[k] = val
				}
			}
		}

		nodes = append(nodes, node)
	}
	return nodes, nil
}

// parseURIList splits data by newlines and stores each non-empty line as a
// ProxyNode with type "raw-uri" and Options["uri"] set to the line value.
// Full URI parsing (extracting server, port, etc.) is deferred to Phase 2.
func parseURIList(data []byte) ([]ProxyNode, error) {
	lines := strings.Split(string(data), "\n")
	nodes := make([]ProxyNode, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		nodes = append(nodes, ProxyNode{
			Type: "raw-uri",
			Options: map[string]any{
				"uri": line,
			},
		})
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no URI entries found")
	}
	return nodes, nil
}
