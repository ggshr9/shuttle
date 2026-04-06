package subscription

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/shuttleX/shuttle/provider"
)

// ParseURI parses a single proxy URI and returns a ProxyNode.
// Supported schemes: ss://, vless://, trojan://
func ParseURI(uri string) (*provider.ProxyNode, error) {
	switch {
	case strings.HasPrefix(uri, "ss://"):
		return parseShadowsocksURI(uri)
	case strings.HasPrefix(uri, "vless://"):
		return parseVLESSURI(uri)
	case strings.HasPrefix(uri, "trojan://"):
		return parseTrojanURI(uri)
	default:
		return nil, fmt.Errorf("unsupported URI scheme: %q", uri)
	}
}

// parseShadowsocksURI parses a Shadowsocks URI.
// Supports SIP002 format: ss://method:password@host:port[#name]
// And legacy format:      ss://base64(method:password)@host:port[#name]
// Also handles:           ss://base64(method:password:host:port)[#name]
func parseShadowsocksURI(raw string) (*provider.ProxyNode, error) {
	// Strip the fragment (name) before parsing, to handle it separately.
	name := ""
	if idx := strings.Index(raw, "#"); idx >= 0 {
		name, _ = url.PathUnescape(raw[idx+1:])
		raw = raw[:idx]
	}

	// Try SIP002 first: ss://method:password@host:port
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" && u.User != nil {
		// Check if userinfo is base64-encoded (legacy with @) or plain (SIP002).
		method, password, host, port, parseErr := extractSIPUserinfo(u)
		if parseErr == nil {
			node := &provider.ProxyNode{
				Name:   name,
				Type:   "ss",
				Server: host,
				Port:   port,
				Options: map[string]any{
					"method":   method,
					"password": password,
				},
			}
			return node, nil
		}
	}

	// Legacy format: ss://base64(method:password@host:port)[#name]
	// or ss://base64(method:password)[#name] (already stripped)
	b64Part := strings.TrimPrefix(raw, "ss://")
	decoded, decErr := base64Decode(b64Part)
	if decErr != nil {
		return nil, fmt.Errorf("ss:// URI parse error: %w", decErr)
	}

	// decoded should be "method:password@host:port"
	atIdx := strings.LastIndex(decoded, "@")
	if atIdx < 0 {
		return nil, fmt.Errorf("ss:// legacy URI: missing @ in decoded payload: %q", decoded)
	}
	methodPass := decoded[:atIdx]
	hostPort := decoded[atIdx+1:]

	colonIdx := strings.Index(methodPass, ":")
	if colonIdx < 0 {
		return nil, fmt.Errorf("ss:// legacy URI: missing : between method and password")
	}
	method := methodPass[:colonIdx]
	password := methodPass[colonIdx+1:]

	host, portStr, err := splitHostPort(hostPort)
	if err != nil {
		return nil, fmt.Errorf("ss:// legacy URI: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("ss:// legacy URI: invalid port %q", portStr)
	}

	return &provider.ProxyNode{
		Name:   name,
		Type:   "ss",
		Server: host,
		Port:   port,
		Options: map[string]any{
			"method":   method,
			"password": password,
		},
	}, nil
}

// extractSIPUserinfo extracts method, password, host and port from a parsed SIP002 URL.
// Tries plain userinfo first; if the password contains colons it may be base64-encoded.
func extractSIPUserinfo(u *url.URL) (method, password, host string, port int, err error) {
	user := u.User.Username()
	pass, hasPass := u.User.Password()

	host, portStr, splitErr := splitHostPort(u.Host)
	if splitErr != nil {
		return "", "", "", 0, splitErr
	}
	p, portErr := strconv.Atoi(portStr)
	if portErr != nil {
		return "", "", "", 0, portErr
	}

	if hasPass {
		// SIP002: userinfo = method:password (plain text)
		return user, pass, host, p, nil
	}

	// userinfo may be base64(method:password) without @
	decoded, decErr := base64Decode(user)
	if decErr != nil {
		return "", "", "", 0, fmt.Errorf("cannot decode userinfo as base64: %w", decErr)
	}
	colonIdx := strings.Index(decoded, ":")
	if colonIdx < 0 {
		return "", "", "", 0, fmt.Errorf("decoded userinfo missing colon: %q", decoded)
	}
	return decoded[:colonIdx], decoded[colonIdx+1:], host, p, nil
}

// parseVLESSURI parses a VLESS URI.
// Format: vless://uuid@host:port?type=tcp&security=tls&sni=xxx#name
func parseVLESSURI(raw string) (*provider.ProxyNode, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("vless:// URI parse error: %w", err)
	}

	name, _ := url.PathUnescape(u.Fragment)

	uuid := u.User.Username()
	if uuid == "" {
		return nil, fmt.Errorf("vless:// URI: missing UUID")
	}

	host, portStr, err := splitHostPort(u.Host)
	if err != nil {
		return nil, fmt.Errorf("vless:// URI: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("vless:// URI: invalid port %q", portStr)
	}

	opts := map[string]any{
		"uuid": uuid,
	}

	// Copy all query parameters into options.
	for k, vs := range u.Query() {
		if len(vs) > 0 {
			opts[k] = vs[0]
		}
	}

	return &provider.ProxyNode{
		Name:    name,
		Type:    "vless",
		Server:  host,
		Port:    port,
		Options: opts,
	}, nil
}

// parseTrojanURI parses a Trojan URI.
// Format: trojan://password@host:port?sni=xxx#name
func parseTrojanURI(raw string) (*provider.ProxyNode, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("trojan:// URI parse error: %w", err)
	}

	name, _ := url.PathUnescape(u.Fragment)

	password := u.User.Username()
	if password == "" {
		return nil, fmt.Errorf("trojan:// URI: missing password")
	}

	host, portStr, err := splitHostPort(u.Host)
	if err != nil {
		return nil, fmt.Errorf("trojan:// URI: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("trojan:// URI: invalid port %q", portStr)
	}

	opts := map[string]any{
		"password": password,
	}

	// Copy all query parameters into options.
	for k, vs := range u.Query() {
		if len(vs) > 0 {
			opts[k] = vs[0]
		}
	}

	return &provider.ProxyNode{
		Name:    name,
		Type:    "trojan",
		Server:  host,
		Port:    port,
		Options: opts,
	}, nil
}

// base64Decode tries multiple base64 encodings (standard, raw, URL, raw-URL).
func base64Decode(s string) (string, error) {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if b, err := enc.DecodeString(s); err == nil {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("cannot base64-decode %q", s)
}

// splitHostPort splits "host:port", handling IPv6 addresses in brackets.
func splitHostPort(hostPort string) (host, port string, err error) {
	if hostPort == "" {
		return "", "", fmt.Errorf("empty host:port")
	}
	// Use the last colon for IPv6 addresses not wrapped in brackets.
	if hostPort[0] == '[' {
		// IPv6 with brackets: [::1]:port
		end := strings.Index(hostPort, "]")
		if end < 0 {
			return "", "", fmt.Errorf("invalid IPv6 address: %q", hostPort)
		}
		host = hostPort[1:end]
		rest := hostPort[end+1:]
		if len(rest) == 0 || rest[0] != ':' {
			return "", "", fmt.Errorf("missing port in %q", hostPort)
		}
		return host, rest[1:], nil
	}
	idx := strings.LastIndex(hostPort, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("missing port in %q", hostPort)
	}
	return hostPort[:idx], hostPort[idx+1:], nil
}
