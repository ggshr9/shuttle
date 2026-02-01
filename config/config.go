package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ClientConfig is the top-level client configuration.
type ClientConfig struct {
	Server     ServerEndpoint   `yaml:"server" json:"server"`
	Servers    []ServerEndpoint `yaml:"servers,omitempty" json:"servers,omitempty"` // saved server list
	Transport  TransportConfig  `yaml:"transport" json:"transport"`
	Proxy      ProxyConfig      `yaml:"proxy" json:"proxy"`
	Routing    RoutingConfig    `yaml:"routing" json:"routing"`
	Congestion CongestionConfig `yaml:"congestion" json:"congestion"`
	Log        LogConfig        `yaml:"log" json:"log"`
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
	Reality   RealityConf `yaml:"reality" json:"reality"`
	CDN       CDNConfig   `yaml:"cdn" json:"cdn"`
}

// H3Config configures HTTP/3 transport.
type H3Config struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	PathPrefix string `yaml:"path_prefix" json:"path_prefix"`
}

// RealityConf configures Reality transport.
type RealityConf struct {
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
	SOCKS5 SOCKS5Conf `yaml:"socks5" json:"socks5"`
	HTTP   HTTPConf   `yaml:"http" json:"http"`
	TUN    TUNConf    `yaml:"tun" json:"tun"`
}

// SOCKS5Conf configures the SOCKS5 listener.
type SOCKS5Conf struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Listen  string `yaml:"listen" json:"listen"`
}

// HTTPConf configures the HTTP proxy listener.
type HTTPConf struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Listen  string `yaml:"listen" json:"listen"`
}

// TUNConf configures the TUN device.
type TUNConf struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	DeviceName string `yaml:"device_name" json:"device_name"`
	CIDR       string `yaml:"cidr" json:"cidr"`
	MTU        int    `yaml:"mtu" json:"mtu"`
	AutoRoute  bool   `yaml:"auto_route" json:"auto_route"`
}

// RoutingConfig configures routing rules.
type RoutingConfig struct {
	Rules   []RouteRule `yaml:"rules" json:"rules"`
	Default string      `yaml:"default" json:"default"` // "proxy", "direct"
	DNS     DNSConf     `yaml:"dns" json:"dns"`
}

// RouteRule defines a single routing rule.
type RouteRule struct {
	Domains  string   `yaml:"domains,omitempty" json:"domains,omitempty"`
	GeoIP    string   `yaml:"geoip,omitempty" json:"geoip,omitempty"`
	Process  []string `yaml:"process,omitempty" json:"process,omitempty"`
	Protocol string   `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	IPCIDR   []string `yaml:"ip_cidr,omitempty" json:"ip_cidr,omitempty"`
	Action   string   `yaml:"action" json:"action"`
}

// DNSConf configures DNS resolution.
type DNSConf struct {
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
	Listen    string          `yaml:"listen" json:"listen"`
	TLS       TLSConfig       `yaml:"tls" json:"tls"`
	Auth      AuthConfig      `yaml:"auth" json:"auth"`
	Cover     CoverSiteConfig `yaml:"cover" json:"cover"`
	Transport ServerTransConf `yaml:"transport" json:"transport"`
	Log       LogConfig       `yaml:"log" json:"log"`
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

// ServerTransConf configures server-side transports.
type ServerTransConf struct {
	H3      ServerH3Conf      `yaml:"h3" json:"h3"`
	Reality ServerRealityConf `yaml:"reality" json:"reality"`
}

// ServerH3Conf configures server H3.
type ServerH3Conf struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	PathPrefix string `yaml:"path_prefix" json:"path_prefix"`
}

// ServerRealityConf configures server Reality.
type ServerRealityConf struct {
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
}

// DeepCopy returns a fully independent copy of the config.
func (c *ClientConfig) DeepCopy() *ClientConfig {
	cp := *c
	// Copy slices that contain reference types
	if c.Servers != nil {
		cp.Servers = make([]ServerEndpoint, len(c.Servers))
		copy(cp.Servers, c.Servers)
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
}
