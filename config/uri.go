package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"strings"
)

const uriScheme = "shuttle://"

// ShareURI contains the fields needed to share a server connection.
type ShareURI struct {
	Addr      string `json:"addr"`
	Password  string `json:"password"`
	SNI       string `json:"sni,omitempty"`
	Transport string `json:"transport,omitempty"` // "h3", "reality", "both" (default "both")
	PublicKey string `json:"public_key,omitempty"`
	ShortID   string `json:"short_id,omitempty"`
	Name      string `json:"name,omitempty"`
}

// EncodeShareURI encodes a ShareURI into a shuttle:// URI string.
func EncodeShareURI(s *ShareURI) string {
	data, _ := json.Marshal(s)
	return uriScheme + base64.RawURLEncoding.EncodeToString(data)
}

// DecodeShareURI parses a shuttle:// URI string into a ShareURI.
func DecodeShareURI(uri string) (*ShareURI, error) {
	if !strings.HasPrefix(uri, uriScheme) {
		return nil, fmt.Errorf("invalid URI scheme: expected %s prefix", uriScheme)
	}
	encoded := strings.TrimPrefix(uri, uriScheme)
	encoded = strings.TrimSpace(encoded)
	// Support both standard base64 (from shell `base64` command) and
	// URL-safe base64 (from Go's RawURLEncoding). Try multiple decodings.
	var data []byte
	var err error
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	} {
		data, err = enc.DecodeString(encoded)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	var s ShareURI
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	if s.Addr == "" {
		return nil, fmt.Errorf("missing required field: addr")
	}
	if s.Password == "" {
		return nil, fmt.Errorf("missing required field: password")
	}
	return &s, nil
}

// ShareURIToClientConfig converts a ShareURI to a full ClientConfig with
// sensible defaults.
func ShareURIToClientConfig(s *ShareURI) *ClientConfig {
	transport := s.Transport
	if transport == "" {
		transport = "both"
	}

	cfg := DefaultClientConfig()
	cfg.Server.Addr = s.Addr
	cfg.Server.Password = s.Password
	cfg.Server.Name = s.Name
	if s.SNI != "" {
		cfg.Server.SNI = s.SNI
	}

	// Configure transports based on the share URI
	switch transport {
	case "h3":
		cfg.Transport.H3.Enabled = true
		cfg.Transport.Reality.Enabled = false
	case "reality":
		cfg.Transport.H3.Enabled = false
		cfg.Transport.Reality.Enabled = true
	default: // "both"
		cfg.Transport.H3.Enabled = true
		cfg.Transport.Reality.Enabled = true
	}

	// Reality-specific fields
	if s.PublicKey != "" {
		cfg.Transport.Reality.PublicKey = s.PublicKey
	}
	if s.ShortID != "" {
		cfg.Transport.Reality.ShortID = s.ShortID
	}
	if s.SNI != "" {
		cfg.Transport.Reality.ServerName = s.SNI
	}

	// Routing: geosite:cn direct
	cfg.Routing.Rules = []RouteRule{
		{GeoSite: "cn", Action: "direct"},
		{GeoIP: "cn", Action: "direct"},
		{Domains: "geosite:private", Action: "direct"},
	}

	return cfg
}

// RenderClientYAML generates a clean, human-readable YAML config from a ShareURI.
// Only includes relevant fields, with comments for guidance.
func RenderClientYAML(s *ShareURI) string {
	sni := s.SNI
	if sni == "" {
		// Try to extract hostname from addr
		if h, _, err := splitHostPort(s.Addr); err == nil && h != "" {
			sni = h
		}
	}

	transport := s.Transport
	if transport == "" {
		transport = "both"
	}

	name := s.Name
	if name == "" {
		name = sni
	}

	var b strings.Builder
	b.WriteString("# Shuttle Client Config — auto-generated via shuttle import\n\n")
	b.WriteString("server:\n")
	b.WriteString(fmt.Sprintf("  addr: %q\n", s.Addr))
	b.WriteString(fmt.Sprintf("  password: %q\n", s.Password))
	if name != "" {
		b.WriteString(fmt.Sprintf("  name: %q\n", name))
	}
	if sni != "" {
		b.WriteString(fmt.Sprintf("  sni: %q\n", sni))
	}

	b.WriteString("\ntransport:\n")
	b.WriteString("  preferred: auto\n")
	if transport == "h3" || transport == "both" {
		b.WriteString("  h3:\n    enabled: true\n")
	}
	if transport == "reality" || transport == "both" {
		b.WriteString("  reality:\n    enabled: true\n")
		if s.PublicKey != "" {
			b.WriteString(fmt.Sprintf("    public_key: %q\n", s.PublicKey))
		}
		if s.ShortID != "" {
			b.WriteString(fmt.Sprintf("    short_id: %q\n", s.ShortID))
		}
		if sni != "" {
			b.WriteString(fmt.Sprintf("    server_name: %q\n", sni))
		}
	}

	b.WriteString("\nproxy:\n")
	b.WriteString("  socks5:\n    enabled: true\n    listen: \"127.0.0.1:1080\"\n")
	b.WriteString("  http:\n    enabled: true\n    listen: \"127.0.0.1:8080\"\n")

	b.WriteString("\nrouting:\n")
	b.WriteString("  default: proxy\n")
	b.WriteString("  rules:\n")
	b.WriteString("    - domains: \"geosite:cn\"\n      action: direct\n")
	b.WriteString("    - geoip: CN\n      action: direct\n")
	b.WriteString("  dns:\n")
	b.WriteString("    domestic: \"223.5.5.5\"\n")
	b.WriteString("    remote:\n      server: \"https://1.1.1.1/dns-query\"\n      via: proxy\n")

	b.WriteString("\ncongestion:\n  mode: adaptive\n")
	b.WriteString("\nlog:\n  level: info\n")

	return b.String()
}

func splitHostPort(addr string) (string, string, error) {
	// Simple wrapper — net.SplitHostPort requires port
	host, port, err := net.SplitHostPort(addr)
	return host, port, err
}
