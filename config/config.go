package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ClientConfig is the top-level client configuration.
type ClientConfig struct {
	Server     ServerEndpoint   `yaml:"server"`
	Transport  TransportConfig  `yaml:"transport"`
	Proxy      ProxyConfig      `yaml:"proxy"`
	Routing    RoutingConfig    `yaml:"routing"`
	Congestion CongestionConfig `yaml:"congestion"`
	Log        LogConfig        `yaml:"log"`
}

// CongestionConfig configures congestion control.
type CongestionConfig struct {
	Mode       string `yaml:"mode"`        // "adaptive", "bbr", "brutal"
	BrutalRate uint64 `yaml:"brutal_rate"` // bytes/sec for brutal mode (default 100MB/s)
}

// ServerEndpoint defines a remote server.
type ServerEndpoint struct {
	Addr     string `yaml:"addr"`
	Name     string `yaml:"name"`
	Password string `yaml:"password"`
	SNI      string `yaml:"sni"`
}

// TransportConfig configures available transports.
type TransportConfig struct {
	Preferred string      `yaml:"preferred"` // "h3", "reality", "cdn", "auto"
	H3        H3Config    `yaml:"h3"`
	Reality   RealityConf `yaml:"reality"`
	CDN       CDNConfig   `yaml:"cdn"`
}

// H3Config configures HTTP/3 transport.
type H3Config struct {
	Enabled    bool   `yaml:"enabled"`
	PathPrefix string `yaml:"path_prefix"`
}

// RealityConf configures Reality transport.
type RealityConf struct {
	Enabled    bool   `yaml:"enabled"`
	ServerName string `yaml:"server_name"`
	ShortID    string `yaml:"short_id"`
	PublicKey  string `yaml:"public_key"`
}

// CDNConfig configures CDN transport.
type CDNConfig struct {
	Enabled bool   `yaml:"enabled"`
	Domain  string `yaml:"domain"`
	Path    string `yaml:"path"`
	Mode    string `yaml:"mode"` // "h2", "grpc"
}

// ProxyConfig configures local proxy listeners.
type ProxyConfig struct {
	SOCKS5 SOCKS5Conf `yaml:"socks5"`
	HTTP   HTTPConf   `yaml:"http"`
	TUN    TUNConf    `yaml:"tun"`
}

// SOCKS5Conf configures the SOCKS5 listener.
type SOCKS5Conf struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
}

// HTTPConf configures the HTTP proxy listener.
type HTTPConf struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
}

// TUNConf configures the TUN device.
type TUNConf struct {
	Enabled    bool   `yaml:"enabled"`
	DeviceName string `yaml:"device_name"`
	CIDR       string `yaml:"cidr"`
	MTU        int    `yaml:"mtu"`
	AutoRoute  bool   `yaml:"auto_route"`
}

// RoutingConfig configures routing rules.
type RoutingConfig struct {
	Rules   []RouteRule `yaml:"rules"`
	Default string      `yaml:"default"` // "proxy", "direct"
	DNS     DNSConf     `yaml:"dns"`
}

// RouteRule defines a single routing rule.
type RouteRule struct {
	Domains  string   `yaml:"domains,omitempty"`
	GeoIP    string   `yaml:"geoip,omitempty"`
	Process  []string `yaml:"process,omitempty"`
	Protocol string   `yaml:"protocol,omitempty"`
	IPCIDR   []string `yaml:"ip_cidr,omitempty"`
	Action   string   `yaml:"action"`
}

// DNSConf configures DNS resolution.
type DNSConf struct {
	Domestic string    `yaml:"domestic"`
	Remote   DNSRemote `yaml:"remote"`
	Cache    bool      `yaml:"cache"`
	Prefetch bool      `yaml:"prefetch"`
}

// DNSRemote configures the remote DNS server.
type DNSRemote struct {
	Server string `yaml:"server"`
	Via    string `yaml:"via"` // "proxy" or "direct"
}

// LogConfig configures logging.
type LogConfig struct {
	Level  string `yaml:"level"`  // "debug", "info", "warn", "error"
	Output string `yaml:"output"` // "stdout", "stderr", or file path
}

// ServerConfig is the top-level server configuration.
type ServerConfig struct {
	Listen    string          `yaml:"listen"`
	TLS       TLSConfig       `yaml:"tls"`
	Auth      AuthConfig      `yaml:"auth"`
	Cover     CoverSiteConfig `yaml:"cover"`
	Transport ServerTransConf `yaml:"transport"`
	Log       LogConfig       `yaml:"log"`
}

// TLSConfig configures TLS certificates.
type TLSConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// AuthConfig configures authentication.
type AuthConfig struct {
	Password   string `yaml:"password"`
	PrivateKey string `yaml:"private_key"`
	PublicKey  string `yaml:"public_key"`
}

// CoverSiteConfig configures the cover website.
type CoverSiteConfig struct {
	Mode       string `yaml:"mode"` // "static", "reverse", "default"
	StaticDir  string `yaml:"static_dir"`
	ReverseURL string `yaml:"reverse_url"`
}

// ServerTransConf configures server-side transports.
type ServerTransConf struct {
	H3      ServerH3Conf      `yaml:"h3"`
	Reality ServerRealityConf `yaml:"reality"`
}

// ServerH3Conf configures server H3.
type ServerH3Conf struct {
	Enabled    bool   `yaml:"enabled"`
	PathPrefix string `yaml:"path_prefix"`
}

// ServerRealityConf configures server Reality.
type ServerRealityConf struct {
	Enabled    bool     `yaml:"enabled"`
	TargetSNI  string   `yaml:"target_sni"`
	TargetAddr string   `yaml:"target_addr"`
	ShortIDs   []string `yaml:"short_ids"`
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
