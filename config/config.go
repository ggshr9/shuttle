package config

import (
	"fmt"
	"net"
	"os"

	"gopkg.in/yaml.v3"
)

// ClientConfig is the top-level client configuration.
type ClientConfig struct {
	Server        ServerEndpoint       `yaml:"server" json:"server"`
	Servers       []ServerEndpoint     `yaml:"servers,omitempty" json:"servers,omitempty"` // saved server list
	Subscriptions []SubscriptionConfig `yaml:"subscriptions,omitempty" json:"subscriptions,omitempty"`
	Transport     TransportConfig      `yaml:"transport" json:"transport"`
	Proxy         ProxyConfig          `yaml:"proxy" json:"proxy"`
	Routing       RoutingConfig        `yaml:"routing" json:"routing"`
	Congestion    CongestionConfig     `yaml:"congestion" json:"congestion"`
	Mesh          MeshConfig           `yaml:"mesh" json:"mesh"`
	Log           LogConfig            `yaml:"log" json:"log"`
}

// SubscriptionConfig represents a subscription source.
type SubscriptionConfig struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url"`
}

// MeshConfig configures the client-side mesh virtual LAN.
type MeshConfig struct {
	Enabled    bool      `yaml:"enabled" json:"enabled"`
	P2PEnabled bool      `yaml:"p2p_enabled" json:"p2p_enabled"`
	P2P        P2PConfig `yaml:"p2p" json:"p2p"`
}

// P2PConfig configures peer-to-peer NAT traversal.
type P2PConfig struct {
	STUNServers         []string `yaml:"stun_servers" json:"stun_servers"`
	HolePunchTimeout    string   `yaml:"hole_punch_timeout" json:"hole_punch_timeout"`       // e.g. "10s"
	DirectRetryInterval string   `yaml:"direct_retry_interval" json:"direct_retry_interval"` // e.g. "60s"
	KeepAliveInterval   string   `yaml:"keep_alive_interval" json:"keep_alive_interval"`     // e.g. "30s"
	FallbackThreshold   float64  `yaml:"fallback_threshold" json:"fallback_threshold"`       // packet loss rate, e.g. 0.3
}

// CongestionConfig configures congestion control.
type CongestionConfig struct {
	Mode       string `yaml:"mode" json:"mode"`               // "adaptive", "bbr", "brutal"
	BrutalRate uint64 `yaml:"brutal_rate" json:"brutal_rate"` // bytes/sec for brutal mode (default 100MB/s)
}

// ServerEndpoint defines a remote server.
type ServerEndpoint struct {
	Addr     string `yaml:"addr" json:"addr"`
	Name     string `yaml:"name" json:"name"`
	Password string `yaml:"password" json:"password"`
	SNI      string `yaml:"sni" json:"sni"`
}

// TransportConfig configures available transports.
type TransportConfig struct {
	Preferred string      `yaml:"preferred" json:"preferred"` // "h3", "reality", "cdn", "auto"
	H3        H3Config    `yaml:"h3" json:"h3"`
	Reality   RealityConfig `yaml:"reality" json:"reality"`
	CDN       CDNConfig   `yaml:"cdn" json:"cdn"`
}

// H3Config configures HTTP/3 transport.
type H3Config struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	PathPrefix string `yaml:"path_prefix" json:"path_prefix"`
}

// RealityConfig configures Reality transport.
type RealityConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	ServerName string `yaml:"server_name" json:"server_name"`
	ShortID    string `yaml:"short_id" json:"short_id"`
	PublicKey  string `yaml:"public_key" json:"public_key"`
}

// CDNConfig configures CDN transport.
type CDNConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Domain  string `yaml:"domain" json:"domain"`
	Path    string `yaml:"path" json:"path"`
	Mode    string `yaml:"mode" json:"mode"` // "h2", "grpc"
}

// ProxyConfig configures local proxy listeners.
type ProxyConfig struct {
	SOCKS5 SOCKS5Config `yaml:"socks5" json:"socks5"`
	HTTP   HTTPConfig  `yaml:"http" json:"http"`
	TUN    TUNConfig   `yaml:"tun" json:"tun"`
}

// SOCKS5Config configures the SOCKS5 listener.
type SOCKS5Config struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Listen  string `yaml:"listen" json:"listen"`
}

// HTTPConfig configures the HTTP proxy listener.
type HTTPConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Listen  string `yaml:"listen" json:"listen"`
}

// TUNConfig configures the TUN device.
type TUNConfig struct {
	Enabled    bool     `yaml:"enabled" json:"enabled"`
	DeviceName string   `yaml:"device_name" json:"device_name"`
	CIDR       string   `yaml:"cidr" json:"cidr"`
	MTU        int      `yaml:"mtu" json:"mtu"`
	AutoRoute  bool     `yaml:"auto_route" json:"auto_route"`
	TunFD      int      `yaml:"-" json:"-"`                        // externally provided fd (Android)
	PerAppMode string   `yaml:"per_app_mode" json:"per_app_mode"` // "allow" / "deny" / ""
	AppList    []string `yaml:"app_list" json:"app_list"`          // package names / bundle IDs
}

// RoutingConfig configures routing rules.
type RoutingConfig struct {
	Rules   []RouteRule `yaml:"rules" json:"rules"`
	Default string      `yaml:"default" json:"default"` // "proxy", "direct"
	DNS     DNSConfig   `yaml:"dns" json:"dns"`
}

// RouteRule defines a single routing rule.
type RouteRule struct {
	Domains  string   `yaml:"domains,omitempty" json:"domains,omitempty"`
	GeoSite  string   `yaml:"geosite,omitempty" json:"geosite,omitempty"`
	GeoIP    string   `yaml:"geoip,omitempty" json:"geoip,omitempty"`
	Process  []string `yaml:"process,omitempty" json:"process,omitempty"`
	Protocol string   `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	IPCIDR   []string `yaml:"ip_cidr,omitempty" json:"ip_cidr,omitempty"`
	Action   string   `yaml:"action" json:"action"`
}

// DNSConfig configures DNS resolution.
type DNSConfig struct {
	Domestic string    `yaml:"domestic" json:"domestic"`
	Remote   DNSRemote `yaml:"remote" json:"remote"`
	Cache    bool      `yaml:"cache" json:"cache"`
	Prefetch bool      `yaml:"prefetch" json:"prefetch"`
}

// DNSRemote configures the remote DNS server.
type DNSRemote struct {
	Server string `yaml:"server" json:"server"`
	Via    string `yaml:"via" json:"via"` // "proxy" or "direct"
}

// LogConfig configures logging.
type LogConfig struct {
	Level  string `yaml:"level" json:"level"`   // "debug", "info", "warn", "error"
	Output string `yaml:"output" json:"output"` // "stdout", "stderr", or file path
}

// ServerConfig is the top-level server configuration.
type ServerConfig struct {
	Listen    string               `yaml:"listen" json:"listen"`
	TLS       TLSConfig            `yaml:"tls" json:"tls"`
	Auth      AuthConfig           `yaml:"auth" json:"auth"`
	Cover     CoverSiteConfig      `yaml:"cover" json:"cover"`
	Transport ServerTransportConfig `yaml:"transport" json:"transport"`
	Mesh      ServerMeshConfig     `yaml:"mesh" json:"mesh"`
	Log       LogConfig            `yaml:"log" json:"log"`
}

// ServerMeshConfig configures the server-side mesh virtual LAN.
type ServerMeshConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	CIDR       string `yaml:"cidr" json:"cidr"`             // e.g. "10.7.0.0/24"
	P2PEnabled bool   `yaml:"p2p_enabled" json:"p2p_enabled"` // Enable P2P signaling
}

// TLSConfig configures TLS certificates.
type TLSConfig struct {
	CertFile string `yaml:"cert_file" json:"cert_file"`
	KeyFile  string `yaml:"key_file" json:"key_file"`
}

// AuthConfig configures authentication.
type AuthConfig struct {
	Password   string `yaml:"password" json:"password"`
	PrivateKey string `yaml:"private_key" json:"private_key"`
	PublicKey  string `yaml:"public_key" json:"public_key"`
}

// CoverSiteConfig configures the cover website.
type CoverSiteConfig struct {
	Mode       string `yaml:"mode" json:"mode"` // "static", "reverse", "default"
	StaticDir  string `yaml:"static_dir" json:"static_dir"`
	ReverseURL string `yaml:"reverse_url" json:"reverse_url"`
}

// ServerTransportConfig configures server-side transports.
type ServerTransportConfig struct {
	H3      ServerH3Config      `yaml:"h3" json:"h3"`
	Reality ServerRealityConfig `yaml:"reality" json:"reality"`
}

// ServerH3Config configures server H3.
type ServerH3Config struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	PathPrefix string `yaml:"path_prefix" json:"path_prefix"`
}

// ServerRealityConfig configures server Reality.
type ServerRealityConfig struct {
	Enabled    bool     `yaml:"enabled" json:"enabled"`
	TargetSNI  string   `yaml:"target_sni" json:"target_sni"`
	TargetAddr string   `yaml:"target_addr" json:"target_addr"`
	ShortIDs   []string `yaml:"short_ids" json:"short_ids"`
}

// LoadClientConfig loads client config from a YAML file.
func LoadClientConfig(path string) (*ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg ClientConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	applyClientDefaults(&cfg)
	return &cfg, nil
}

// LoadServerConfig loads server config from a YAML file.
func LoadServerConfig(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg ServerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	applyServerDefaults(&cfg)
	return &cfg, nil
}

// DefaultClientConfig returns a config with sensible defaults, ready for the GUI
// to display and let the user fill in server details.
func DefaultClientConfig() *ClientConfig {
	cfg := &ClientConfig{}
	applyClientDefaults(cfg)
	cfg.Proxy.SOCKS5.Enabled = true
	cfg.Proxy.HTTP.Enabled = true
	cfg.Transport.H3.Enabled = true
	cfg.Transport.Reality.Enabled = true
	cfg.Transport.CDN.Enabled = true
	cfg.Congestion.Mode = "adaptive"
	return cfg
}

func applyClientDefaults(cfg *ClientConfig) {
	if cfg.Transport.Preferred == "" {
		cfg.Transport.Preferred = "auto"
	}
	if cfg.Proxy.SOCKS5.Listen == "" {
		cfg.Proxy.SOCKS5.Listen = "127.0.0.1:1080"
	}
	if cfg.Proxy.HTTP.Listen == "" {
		cfg.Proxy.HTTP.Listen = "127.0.0.1:8080"
	}
	if cfg.Routing.Default == "" {
		cfg.Routing.Default = "proxy"
	}
	if cfg.Routing.DNS.Domestic == "" {
		cfg.Routing.DNS.Domestic = "223.5.5.5"
	}
	if cfg.Routing.DNS.Remote.Server == "" {
		cfg.Routing.DNS.Remote.Server = "https://1.1.1.1/dns-query"
	}
	if cfg.Routing.DNS.Remote.Via == "" {
		cfg.Routing.DNS.Remote.Via = "proxy"
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Congestion.Mode == "" {
		cfg.Congestion.Mode = "adaptive"
	}
	// Auto-fill SNI from server address hostname
	if cfg.Server.SNI == "" && cfg.Server.Addr != "" {
		host := cfg.Server.Addr
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		// Only set SNI if it looks like a hostname (not an IP)
		if net.ParseIP(host) == nil {
			cfg.Server.SNI = host
		}
	}
	// P2P defaults
	if len(cfg.Mesh.P2P.STUNServers) == 0 {
		cfg.Mesh.P2P.STUNServers = []string{
			"stun.l.google.com:19302",
			"stun1.l.google.com:19302",
			"stun.cloudflare.com:3478",
		}
	}
	if cfg.Mesh.P2P.HolePunchTimeout == "" {
		cfg.Mesh.P2P.HolePunchTimeout = "10s"
	}
	if cfg.Mesh.P2P.DirectRetryInterval == "" {
		cfg.Mesh.P2P.DirectRetryInterval = "60s"
	}
	if cfg.Mesh.P2P.KeepAliveInterval == "" {
		cfg.Mesh.P2P.KeepAliveInterval = "30s"
	}
	if cfg.Mesh.P2P.FallbackThreshold == 0 {
		cfg.Mesh.P2P.FallbackThreshold = 0.3
	}
}

// DeepCopy returns a fully independent copy of the config.
func (c *ClientConfig) DeepCopy() *ClientConfig {
	cp := *c
	// Copy slices that contain reference types
	if c.Servers != nil {
		cp.Servers = make([]ServerEndpoint, len(c.Servers))
		copy(cp.Servers, c.Servers)
	}
	if c.Subscriptions != nil {
		cp.Subscriptions = make([]SubscriptionConfig, len(c.Subscriptions))
		copy(cp.Subscriptions, c.Subscriptions)
	}
	if c.Routing.Rules != nil {
		cp.Routing.Rules = make([]RouteRule, len(c.Routing.Rules))
		for i, r := range c.Routing.Rules {
			cp.Routing.Rules[i] = r
			if r.Process != nil {
				cp.Routing.Rules[i].Process = make([]string, len(r.Process))
				copy(cp.Routing.Rules[i].Process, r.Process)
			}
			if r.IPCIDR != nil {
				cp.Routing.Rules[i].IPCIDR = make([]string, len(r.IPCIDR))
				copy(cp.Routing.Rules[i].IPCIDR, r.IPCIDR)
			}
		}
	}
	if c.Proxy.TUN.AppList != nil {
		cp.Proxy.TUN.AppList = make([]string, len(c.Proxy.TUN.AppList))
		copy(cp.Proxy.TUN.AppList, c.Proxy.TUN.AppList)
	}
	if c.Mesh.P2P.STUNServers != nil {
		cp.Mesh.P2P.STUNServers = make([]string, len(c.Mesh.P2P.STUNServers))
		copy(cp.Mesh.P2P.STUNServers, c.Mesh.P2P.STUNServers)
	}
	return &cp
}

func applyServerDefaults(cfg *ServerConfig) {
	if cfg.Listen == "" {
		cfg.Listen = ":443"
	}
	if cfg.Cover.Mode == "" {
		cfg.Cover.Mode = "default"
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Mesh.CIDR == "" {
		cfg.Mesh.CIDR = "10.7.0.0/24"
	}
}
