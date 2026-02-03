package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"
)

// ImportResult holds the result of parsing an import string.
type ImportResult struct {
	Servers []ServerEndpoint `json:"servers"`
	Errors  []string         `json:"errors,omitempty"`
}

// ImportConfig parses various configuration formats and returns server endpoints.
// Supported formats:
// - base64 encoded JSON (single server or array)
// - JSON (single server or array)
// - shuttle:// URI scheme
// - Line-separated shuttle:// URIs
func ImportConfig(data string) (*ImportResult, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil, fmt.Errorf("empty input")
	}

	result := &ImportResult{}

	// Try shuttle:// URI first (single or multi-line)
	if strings.HasPrefix(data, "shuttle://") || strings.Contains(data, "\nshuttle://") {
		return parseShuttleURIs(data)
	}

	// Try base64 decode
	if decoded, err := tryBase64Decode(data); err == nil {
		data = decoded
	}

	// Try JSON parse
	servers, err := parseJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	result.Servers = servers
	return result, nil
}

// parseShuttleURIs parses one or more shuttle:// URIs.
// Format: shuttle://password@host:port?name=Name&sni=example.com
func parseShuttleURIs(data string) (*ImportResult, error) {
	result := &ImportResult{}
	lines := strings.Split(data, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if !strings.HasPrefix(line, "shuttle://") {
			result.Errors = append(result.Errors, fmt.Sprintf("invalid URI: %s", line))
			continue
		}

		srv, err := parseShuttleURI(line)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		result.Servers = append(result.Servers, *srv)
	}

	if len(result.Servers) == 0 {
		return nil, fmt.Errorf("no valid servers found")
	}

	return result, nil
}

// parseShuttleURI parses a single shuttle:// URI.
func parseShuttleURI(uri string) (*ServerEndpoint, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid URI %s: %w", uri, err)
	}

	if u.Scheme != "shuttle" {
		return nil, fmt.Errorf("invalid scheme: %s", u.Scheme)
	}

	srv := &ServerEndpoint{}

	// Host and port
	srv.Addr = u.Host
	if srv.Addr == "" {
		return nil, fmt.Errorf("missing host in URI")
	}

	// Password from userinfo
	if u.User != nil {
		srv.Password = u.User.Username()
		if pwd, set := u.User.Password(); set {
			srv.Password = pwd
		}
	}

	// Query parameters
	q := u.Query()
	srv.Name = q.Get("name")
	if srv.Name == "" {
		srv.Name = srv.Addr
	}
	srv.SNI = q.Get("sni")

	return srv, nil
}

// tryBase64Decode attempts to decode base64 data.
func tryBase64Decode(data string) (string, error) {
	// Try standard base64
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err == nil {
		return string(decoded), nil
	}

	// Try URL-safe base64
	decoded, err = base64.URLEncoding.DecodeString(data)
	if err == nil {
		return string(decoded), nil
	}

	// Try raw standard base64 (no padding)
	decoded, err = base64.RawStdEncoding.DecodeString(data)
	if err == nil {
		return string(decoded), nil
	}

	// Try raw URL-safe base64 (no padding)
	decoded, err = base64.RawURLEncoding.DecodeString(data)
	if err == nil {
		return string(decoded), nil
	}

	return "", fmt.Errorf("not base64 encoded")
}

// parseJSON parses JSON as a single server or array of servers.
func parseJSON(data string) ([]ServerEndpoint, error) {
	data = strings.TrimSpace(data)

	// Try as array first
	var servers []ServerEndpoint
	if err := json.Unmarshal([]byte(data), &servers); err == nil {
		return filterValidServers(servers), nil
	}

	// Try as single server
	var srv ServerEndpoint
	if err := json.Unmarshal([]byte(data), &srv); err == nil {
		if srv.Addr != "" {
			return []ServerEndpoint{srv}, nil
		}
	}

	// Try as wrapped object with "servers" field
	var wrapped struct {
		Servers []ServerEndpoint `json:"servers"`
		Server  ServerEndpoint   `json:"server"`
	}
	if err := json.Unmarshal([]byte(data), &wrapped); err == nil {
		if len(wrapped.Servers) > 0 {
			return filterValidServers(wrapped.Servers), nil
		}
		if wrapped.Server.Addr != "" {
			return []ServerEndpoint{wrapped.Server}, nil
		}
	}

	return nil, fmt.Errorf("invalid JSON format")
}

// filterValidServers removes servers without addresses.
func filterValidServers(servers []ServerEndpoint) []ServerEndpoint {
	valid := make([]ServerEndpoint, 0, len(servers))
	for _, s := range servers {
		if s.Addr != "" {
			valid = append(valid, s)
		}
	}
	return valid
}

// ExportConfig exports servers to the specified format.
func ExportConfig(cfg *ClientConfig, format string) ([]byte, error) {
	switch format {
	case "json":
		return json.MarshalIndent(cfg, "", "  ")
	case "yaml":
		return marshalYAML(cfg)
	case "uri":
		return exportAsURIs(cfg)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// marshalYAML marshals config to YAML format.
func marshalYAML(cfg *ClientConfig) ([]byte, error) {
	return yaml.Marshal(cfg)
}

// exportAsURIs exports servers as shuttle:// URIs.
func exportAsURIs(cfg *ClientConfig) ([]byte, error) {
	var lines []string

	addServer := func(srv ServerEndpoint) {
		u := url.URL{
			Scheme: "shuttle",
			Host:   srv.Addr,
		}
		if srv.Password != "" {
			u.User = url.User(srv.Password)
		}
		q := url.Values{}
		if srv.Name != "" && srv.Name != srv.Addr {
			q.Set("name", srv.Name)
		}
		if srv.SNI != "" {
			q.Set("sni", srv.SNI)
		}
		if len(q) > 0 {
			u.RawQuery = q.Encode()
		}
		lines = append(lines, u.String())
	}

	// Export active server
	if cfg.Server.Addr != "" {
		addServer(cfg.Server)
	}

	// Export saved servers
	for _, srv := range cfg.Servers {
		addServer(srv)
	}

	return []byte(strings.Join(lines, "\n")), nil
}
