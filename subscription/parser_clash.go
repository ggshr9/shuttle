package subscription

import (
	"fmt"
	"strings"

	"github.com/shuttleX/shuttle/config"
	"gopkg.in/yaml.v3"
)

// clashConfig is a minimal representation of a Clash YAML config.
type clashConfig struct {
	Proxies []clashProxy `yaml:"proxies"`
}

// clashProxy represents a single proxy entry in Clash format.
type clashProxy struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Server   string `yaml:"server"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	UUID     string `yaml:"uuid"`     // vmess/vless
	SNI      string `yaml:"sni"`      // trojan, vless, hysteria2
	ServName string `yaml:"servname"` // hysteria2 alternate SNI field
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

// parseClash parses a Clash YAML subscription and returns a slice of
// ServerEndpoint values. Supported proxy types: ss, trojan, vmess, vless,
// hysteria2. Returns an error if data is invalid or contains no proxies.
func parseClash(data []byte) ([]config.ServerEndpoint, error) {
	var cfg clashConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse clash yaml: %w", err)
	}

	if len(cfg.Proxies) == 0 {
		return nil, fmt.Errorf("no proxies found in clash config")
	}

	var endpoints []config.ServerEndpoint
	for _, p := range cfg.Proxies {
		switch strings.ToLower(p.Type) {
		case "ss", "trojan", "vmess", "vless", "hysteria2":
			ep := config.ServerEndpoint{
				Name: p.Name,
				Addr: fmt.Sprintf("%s:%d", p.Server, p.Port),
				SNI:  p.SNI,
			}
			// Pick password: ss/trojan/hysteria2 use "password", vmess/vless use "uuid".
			switch strings.ToLower(p.Type) {
			case "vmess", "vless":
				ep.Password = p.UUID
			default:
				ep.Password = p.Password
			}
			// hysteria2 may also carry servname as SNI.
			if ep.SNI == "" && p.ServName != "" {
				ep.SNI = p.ServName
			}
			endpoints = append(endpoints, ep)
		}
	}

	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no supported proxies found in clash config")
	}
	return endpoints, nil
}
