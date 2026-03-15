package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config version constants.
const (
	CurrentClientConfigVersion = 1
	CurrentServerConfigVersion = 1
)

// YamuxConfig holds yamux multiplexer tuning parameters.
type YamuxConfig struct {
	MaxStreamWindowSize    uint32 `yaml:"max_stream_window_size" json:"max_stream_window_size"`       // default 256KB
	KeepAliveInterval      int    `yaml:"keep_alive_interval" json:"keep_alive_interval"`             // seconds, default 30
	ConnectionWriteTimeout int    `yaml:"connection_write_timeout" json:"connection_write_timeout"`    // seconds, default 10
}

// ClientConfig is the top-level client configuration.
type ClientConfig struct {
	Version       int                  `yaml:"version" json:"version"`
	Server        ServerEndpoint       `yaml:"server" json:"server"`
	Servers       []ServerEndpoint     `yaml:"servers,omitempty" json:"servers,omitempty"` // saved server list
	Subscriptions []SubscriptionConfig `yaml:"subscriptions,omitempty" json:"subscriptions,omitempty"`
	Transport     TransportConfig      `yaml:"transport" json:"transport"`
	Proxy         ProxyConfig          `yaml:"proxy" json:"proxy"`
	Routing       RoutingConfig        `yaml:"routing" json:"routing"`
	QoS           QoSConfig            `yaml:"qos" json:"qos"`
	Congestion    CongestionConfig     `yaml:"congestion" json:"congestion"`
	Retry         RetryConfig          `yaml:"retry" json:"retry"`
	Mesh          MeshConfig           `yaml:"mesh" json:"mesh"`
	Obfs          ObfsConfig           `yaml:"obfs" json:"obfs"`
	Yamux         YamuxConfig          `yaml:"yamux" json:"yamux"`
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
	Enabled    bool           `yaml:"enabled" json:"enabled"`
	P2PEnabled bool           `yaml:"p2p_enabled" json:"p2p_enabled"`
	P2P        P2PConfig      `yaml:"p2p" json:"p2p"`
	SplitRoutes []SplitRoute  `yaml:"split_routes,omitempty" json:"split_routes,omitempty"`
}

// SplitRoute defines a subnet-level routing policy for mesh traffic.
type SplitRoute struct {
	CIDR   string `yaml:"cidr" json:"cidr"`     // e.g. "10.7.0.128/25"
	Action string `yaml:"action" json:"action"` // "mesh", "direct", "proxy"
}

// P2PConfig configures peer-to-peer NAT traversal.
type P2PConfig struct {
	STUNServers         []string `yaml:"stun_servers" json:"stun_servers"`
	HolePunchTimeout    string   `yaml:"hole_punch_timeout" json:"hole_punch_timeout"`       // e.g. "10s"
	DirectRetryInterval string   `yaml:"direct_retry_interval" json:"direct_retry_interval"` // e.g. "60s"
	KeepAliveInterval   string   `yaml:"keep_alive_interval" json:"keep_alive_interval"`     // e.g. "30s"
	FallbackThreshold   float64  `yaml:"fallback_threshold" json:"fallback_threshold"`       // packet loss rate, e.g. 0.3

	// Port spoofing for bypassing restrictive firewalls
	// Values: "none", "dns" (port 53), "https" (port 443), "ike" (port 500), or a port number
	SpoofMode string `yaml:"spoof_mode" json:"spoof_mode"`
	SpoofPort int    `yaml:"spoof_port" json:"spoof_port"` // Custom port when spoof_mode is a number

	// UPnP/NAT-PMP configuration for automatic port mapping
	// By default, port mapping is auto-enabled for best NAT traversal experience
	// Set disable_upnp: true to disable automatic port mapping
	EnableUPnP    bool `yaml:"enable_upnp" json:"enable_upnp"`       // Deprecated: UPnP is auto-enabled
	DisableUPnP   bool `yaml:"disable_upnp" json:"disable_upnp"`     // Disable UPnP/NAT-PMP auto-detection
	PreferredPort int  `yaml:"preferred_port" json:"preferred_port"` // Preferred external port (0 = same as local)
}

// CongestionConfig configures congestion control.
type CongestionConfig struct {
	Mode       string `yaml:"mode" json:"mode"`               // "adaptive", "bbr", "brutal"
	BrutalRate uint64 `yaml:"brutal_rate" json:"brutal_rate"` // bytes/sec for brutal mode (default 100MB/s)
}

// QoSConfig configures Quality of Service marking.
type QoSConfig struct {
	Enabled bool      `yaml:"enabled" json:"enabled"`
	Rules   []QoSRule `yaml:"rules" json:"rules"`
}

// QoSRule defines a traffic classification rule for QoS marking.
// Priority levels: critical (0), high (1), normal (2), bulk (3), low (4)
// DSCP values: EF (46), AF41 (34), AF21 (18), AF11 (10), BE (0)
type QoSRule struct {
	// Match conditions (any match triggers the rule)
	Ports    []uint16 `yaml:"ports,omitempty" json:"ports,omitempty"`       // Destination ports
	Protocol string   `yaml:"protocol,omitempty" json:"protocol,omitempty"` // "tcp", "udp"
	Domains  []string `yaml:"domains,omitempty" json:"domains,omitempty"`   // Domain patterns
	Process  []string `yaml:"process,omitempty" json:"process,omitempty"`   // Process names

	// Priority assignment
	Priority string `yaml:"priority" json:"priority"` // "critical", "high", "normal", "bulk", "low"
}

// RetryConfig configures connection retry with exponential backoff.
type RetryConfig struct {
	MaxAttempts    int    `yaml:"max_attempts" json:"max_attempts"`
	InitialBackoff string `yaml:"initial_backoff" json:"initial_backoff"` // duration string, e.g. "100ms"
	MaxBackoff     string `yaml:"max_backoff" json:"max_backoff"`         // duration string, e.g. "5s"
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
	Preferred         string        `yaml:"preferred" json:"preferred"`                                     // "h3", "reality", "cdn", "webrtc", "auto", "multipath"
	MultipathSchedule string        `yaml:"multipath_schedule" json:"multipath_schedule"`                   // "weighted" (default), "min-latency", "load-balance"
	WarmUpConns       int           `yaml:"warm_up_conns,omitempty" json:"warm_up_conns,omitempty"`         // pre-dial N connections on startup to eliminate cold-start latency
	H3                H3Config      `yaml:"h3" json:"h3"`
	Reality           RealityConfig `yaml:"reality" json:"reality"`
	CDN               CDNConfig     `yaml:"cdn" json:"cdn"`
	WebRTC            WebRTCConfig  `yaml:"webrtc" json:"webrtc"`
}

// H3Config configures HTTP/3 transport.
type H3Config struct {
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	PathPrefix         string `yaml:"path_prefix" json:"path_prefix"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify,omitempty" json:"insecure_skip_verify,omitempty"`
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
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	Domain             string `yaml:"domain" json:"domain"`
	Path               string `yaml:"path" json:"path"`
	Mode               string `yaml:"mode" json:"mode"`                                                      // "h2", "grpc"
	FrontDomain        string `yaml:"front_domain" json:"front_domain"`                                       // SNI domain for domain fronting (different from actual server)
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify,omitempty" json:"insecure_skip_verify,omitempty"`
}

// WebRTCConfig configures WebRTC DataChannel transport.
type WebRTCConfig struct {
	Enabled     bool     `yaml:"enabled" json:"enabled"`
	SignalURL   string   `yaml:"signal_url" json:"signal_url"`
	STUNServers []string `yaml:"stun_servers" json:"stun_servers"`
	TURNServers []string `yaml:"turn_servers" json:"turn_servers"`
	TURNUser    string   `yaml:"turn_user" json:"turn_user"`
	TURNPass    string   `yaml:"turn_pass" json:"turn_pass"`
	ICEPolicy   string   `yaml:"ice_policy" json:"ice_policy"` // "all", "relay", "public" (default "all")
}

// ProxyConfig configures local proxy listeners.
type ProxyConfig struct {
	AllowLAN    bool              `yaml:"allow_lan" json:"allow_lan"` // Allow other devices on LAN to use this proxy (e.g., hotspot sharing)
	SOCKS5      SOCKS5Config      `yaml:"socks5" json:"socks5"`
	HTTP        HTTPConfig        `yaml:"http" json:"http"`
	TUN         TUNConfig         `yaml:"tun" json:"tun"`
	SystemProxy SystemProxyConfig `yaml:"system_proxy" json:"system_proxy"`
}

// SystemProxyConfig configures automatic system proxy setting.
type SystemProxyConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"` // Auto-set system proxy on connect
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
	Rules   []RouteRule   `yaml:"rules" json:"rules"`
	Default string        `yaml:"default" json:"default"` // "proxy", "direct"
	DNS     DNSConfig     `yaml:"dns" json:"dns"`
	GeoData GeoDataConfig `yaml:"geodata" json:"geodata"`
}

// GeoDataConfig configures automatic GeoIP/GeoSite data management.
type GeoDataConfig struct {
	Enabled        bool   `yaml:"enabled" json:"enabled"`
	DataDir        string `yaml:"data_dir,omitempty" json:"data_dir,omitempty"`
	AutoUpdate     bool   `yaml:"auto_update" json:"auto_update"`
	UpdateInterval string `yaml:"update_interval,omitempty" json:"update_interval,omitempty"` // e.g. "24h"
	// Source URLs (defaults to Loyalsoldier/v2ray-rules-dat)
	DirectListURL  string `yaml:"direct_list_url,omitempty" json:"direct_list_url,omitempty"`
	ProxyListURL   string `yaml:"proxy_list_url,omitempty" json:"proxy_list_url,omitempty"`
	RejectListURL  string `yaml:"reject_list_url,omitempty" json:"reject_list_url,omitempty"`
	GFWListURL     string `yaml:"gfw_list_url,omitempty" json:"gfw_list_url,omitempty"`
	CNCidrURL      string `yaml:"cn_cidr_url,omitempty" json:"cn_cidr_url,omitempty"`
	PrivateCidrURL string `yaml:"private_cidr_url,omitempty" json:"private_cidr_url,omitempty"`
}

// RouteRule defines a single routing rule.
type RouteRule struct {
	Domains     string   `yaml:"domains,omitempty" json:"domains,omitempty"`
	GeoSite     string   `yaml:"geosite,omitempty" json:"geosite,omitempty"`
	GeoIP       string   `yaml:"geoip,omitempty" json:"geoip,omitempty"`
	Process     []string `yaml:"process,omitempty" json:"process,omitempty"`
	Protocol    string   `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	IPCIDR      []string `yaml:"ip_cidr,omitempty" json:"ip_cidr,omitempty"`
	NetworkType string   `yaml:"network_type,omitempty" json:"network_type,omitempty"` // "wifi", "cellular", "ethernet"
	Action      string   `yaml:"action" json:"action"`
}

// DNSConfig configures DNS resolution.
type DNSConfig struct {
	Domestic       string    `yaml:"domestic" json:"domestic"`
	Remote         DNSRemote `yaml:"remote" json:"remote"`
	Cache          bool      `yaml:"cache" json:"cache"`
	Prefetch       bool      `yaml:"prefetch" json:"prefetch"`
	LeakPrevention bool      `yaml:"leak_prevention" json:"leak_prevention"` // Force all DNS through proxy
	DomesticDoH    string    `yaml:"domestic_doh" json:"domestic_doh"`       // DoH URL for domestic queries (e.g., "https://dns.alidns.com/dns-query")
	StripECS       bool      `yaml:"strip_ecs" json:"strip_ecs"`            // Strip EDNS Client Subnet
	PersistentConn *bool     `yaml:"persistent_conn" json:"persistent_conn"` // Persistent HTTP/2 connections for DoH (default true)
}

// DNSRemote configures the remote DNS server.
type DNSRemote struct {
	Server string `yaml:"server" json:"server"`
	Via    string `yaml:"via" json:"via"` // "proxy" or "direct"
}

// ObfsConfig configures traffic obfuscation.
type ObfsConfig struct {
	PaddingEnabled bool   `yaml:"padding_enabled" json:"padding_enabled"`
	ShapingEnabled bool   `yaml:"shaping_enabled" json:"shaping_enabled"`
	MinDelay       string `yaml:"min_delay" json:"min_delay"` // duration string, default "0s"
	MaxDelay       string `yaml:"max_delay" json:"max_delay"` // duration string, default "50ms"
	ChunkSize      int    `yaml:"chunk_size" json:"chunk_size"` // min chunk size for splitting (default 64)
}

// LogConfig configures logging.
type LogConfig struct {
	Level  string `yaml:"level" json:"level"`   // "debug", "info", "warn", "error"
	Format string `yaml:"format" json:"format"` // "text" (default) or "json"
	Output string `yaml:"output" json:"output"` // "stdout", "stderr", or file path
}

// AuditConfig configures server-side connection audit logging.
type AuditConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	LogDir     string `yaml:"log_dir" json:"log_dir"`
	MaxEntries int    `yaml:"max_entries" json:"max_entries"`
}

// DebugConfig configures debug/profiling endpoints.
type DebugConfig struct {
	PprofEnabled bool   `yaml:"pprof_enabled" json:"pprof_enabled"`
	PprofListen  string `yaml:"pprof_listen" json:"pprof_listen"`
}

// ReputationConfig configures IP reputation tracking for anti-probing defense.
type ReputationConfig struct {
	Enabled     bool `yaml:"enabled" json:"enabled"`
	MaxFailures int  `yaml:"max_failures" json:"max_failures"` // failures before ban (default 5)
}

// ServerConfig is the top-level server configuration.
type ServerConfig struct {
	Version      int                  `yaml:"version" json:"version"`
	Listen       string               `yaml:"listen" json:"listen"`
	DrainTimeout string               `yaml:"drain_timeout,omitempty" json:"drain_timeout,omitempty"` // graceful shutdown drain timeout, e.g. "30s"
	TLS          TLSConfig            `yaml:"tls" json:"tls"`
	Auth         AuthConfig           `yaml:"auth" json:"auth"`
	Cover        CoverSiteConfig      `yaml:"cover" json:"cover"`
	Transport    ServerTransportConfig `yaml:"transport" json:"transport"`
	Mesh         ServerMeshConfig     `yaml:"mesh" json:"mesh"`
	Admin        AdminConfig          `yaml:"admin" json:"admin"`
	Audit        AuditConfig          `yaml:"audit" json:"audit"`
	Reputation   ReputationConfig     `yaml:"reputation" json:"reputation"`
	Cluster      ClusterConfig        `yaml:"cluster" json:"cluster"`
	Debug        DebugConfig          `yaml:"debug" json:"debug"`
	Yamux        YamuxConfig          `yaml:"yamux" json:"yamux"`
	Log          LogConfig            `yaml:"log" json:"log"`
}

// ClusterConfig configures multi-instance server clustering.
type ClusterConfig struct {
	Enabled  bool              `yaml:"enabled" json:"enabled"`
	NodeName string            `yaml:"node_name" json:"node_name"`
	Secret   string            `yaml:"secret" json:"secret"`
	Peers    []ClusterPeer     `yaml:"peers,omitempty" json:"peers,omitempty"`
	Interval string            `yaml:"interval,omitempty" json:"interval,omitempty"` // default "15s"
	MaxConns int64             `yaml:"max_conns,omitempty" json:"max_conns,omitempty"`
}

// ClusterPeer defines a known peer node in the cluster.
type ClusterPeer struct {
	Name string `yaml:"name" json:"name"`
	Addr string `yaml:"addr" json:"addr"` // admin API address
}

// AdminConfig configures the server admin API.
type AdminConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Listen  string `yaml:"listen" json:"listen"` // default "127.0.0.1:9090"
	Token   string `yaml:"token" json:"token"`
	Users   []User `yaml:"users,omitempty" json:"users,omitempty"`
}

// User represents a client user with traffic quotas.
type User struct {
	Name     string `yaml:"name" json:"name"`
	Token    string `yaml:"token" json:"token"`         // Per-user auth token
	MaxBytes int64  `yaml:"max_bytes" json:"max_bytes"` // 0 = unlimited
	Enabled  bool   `yaml:"enabled" json:"enabled"`
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
	CDN     ServerCDNConfig     `yaml:"cdn" json:"cdn"`
	WebRTC  ServerWebRTCConfig  `yaml:"webrtc" json:"webrtc"`
}

// ServerCDNConfig configures server-side CDN (HTTP/2) transport.
type ServerCDNConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Path    string `yaml:"path" json:"path"`     // URL path for CDN endpoint (default "/cdn/stream")
	Listen  string `yaml:"listen" json:"listen"` // Listen address (default: same as main listen)
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

// ServerWebRTCConfig configures server-side WebRTC DataChannel transport.
type ServerWebRTCConfig struct {
	Enabled      bool     `yaml:"enabled" json:"enabled"`
	SignalListen string   `yaml:"signal_listen" json:"signal_listen"`
	STUNServers  []string `yaml:"stun_servers" json:"stun_servers"`
	TURNServers  []string `yaml:"turn_servers" json:"turn_servers"`
	TURNUser     string   `yaml:"turn_user" json:"turn_user"`
	TURNPass     string   `yaml:"turn_pass" json:"turn_pass"`
	ICEPolicy    string   `yaml:"ice_policy" json:"ice_policy"` // "all", "relay", "public" (default "all")
}

// validateHostPort checks that s is a valid host:port pair.
func validateHostPort(s, field string) error {
	if _, _, err := net.SplitHostPort(s); err != nil {
		return fmt.Errorf("invalid %s: %w", field, err)
	}
	return nil
}

// validateURL checks that s is a valid URL with an http or https scheme.
func validateURL(s, field string) error {
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", field, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid %s: scheme must be http or https, got %q", field, u.Scheme)
	}
	return nil
}

// validateDuration checks that s is a valid Go duration string.
func validateDuration(s, field string) error {
	if _, err := time.ParseDuration(s); err != nil {
		return fmt.Errorf("invalid %s: %w", field, err)
	}
	return nil
}

// validateCIDR checks that s is a valid CIDR notation.
func validateCIDR(s, field string) error {
	if _, _, err := net.ParseCIDR(s); err != nil {
		return fmt.Errorf("invalid %s: %w", field, err)
	}
	return nil
}

// Validate checks the client config for obviously wrong values.
func (c *ClientConfig) Validate() error {
	switch c.Transport.Preferred {
	case "auto", "h3", "reality", "cdn", "webrtc", "multipath":
	default:
		return fmt.Errorf("invalid transport.preferred: %q", c.Transport.Preferred)
	}
	switch c.Routing.Default {
	case "proxy", "direct":
	default:
		return fmt.Errorf("invalid routing.default: %q", c.Routing.Default)
	}
	if c.Proxy.SOCKS5.Listen != "" {
		if _, _, err := net.SplitHostPort(c.Proxy.SOCKS5.Listen); err != nil {
			return fmt.Errorf("invalid proxy.socks5.listen: %w", err)
		}
	}
	if c.Proxy.HTTP.Listen != "" {
		if _, _, err := net.SplitHostPort(c.Proxy.HTTP.Listen); err != nil {
			return fmt.Errorf("invalid proxy.http.listen: %w", err)
		}
	}
	switch c.Congestion.Mode {
	case "adaptive", "bbr", "brutal", "":
	default:
		return fmt.Errorf("invalid congestion.mode: %q", c.Congestion.Mode)
	}
	if c.Obfs.MaxDelay != "" {
		if _, err := time.ParseDuration(c.Obfs.MaxDelay); err != nil {
			return fmt.Errorf("invalid obfs.max_delay: %w", err)
		}
	}
	for _, sr := range c.Mesh.SplitRoutes {
		if _, _, err := net.ParseCIDR(sr.CIDR); err != nil {
			return fmt.Errorf("invalid mesh split_route CIDR %q: %w", sr.CIDR, err)
		}
		switch sr.Action {
		case "mesh", "direct", "proxy":
		default:
			return fmt.Errorf("invalid mesh split_route action: %q (must be mesh, direct, or proxy)", sr.Action)
		}
	}

	// Server.Addr validation
	if c.Server.Addr != "" {
		if err := validateHostPort(c.Server.Addr, "server.addr"); err != nil {
			return err
		}
	}

	// Servers[] validation
	for i, s := range c.Servers {
		if s.Addr != "" {
			if err := validateHostPort(s.Addr, fmt.Sprintf("servers[%d].addr", i)); err != nil {
				return err
			}
		}
	}

	// Subscriptions[].URL validation
	for i, sub := range c.Subscriptions {
		if sub.URL != "" {
			if err := validateURL(sub.URL, fmt.Sprintf("subscriptions[%d].url", i)); err != nil {
				return err
			}
		}
	}

	// Routing.DNS.Domestic validation
	if c.Routing.DNS.Domestic != "" {
		if strings.Contains(c.Routing.DNS.Domestic, "://") {
			if err := validateURL(c.Routing.DNS.Domestic, "routing.dns.domestic"); err != nil {
				return err
			}
		} else if strings.Contains(c.Routing.DNS.Domestic, ":") {
			// host:port form
			if err := validateHostPort(c.Routing.DNS.Domestic, "routing.dns.domestic"); err != nil {
				return err
			}
		} else {
			// plain IP — validate it parses
			if net.ParseIP(c.Routing.DNS.Domestic) == nil {
				return fmt.Errorf("invalid routing.dns.domestic: %q is not a valid IP address", c.Routing.DNS.Domestic)
			}
		}
	}

	// Routing.DNS.Remote.Server — must be valid DoH URL with https scheme
	if c.Routing.DNS.Remote.Server != "" {
		u, err := url.Parse(c.Routing.DNS.Remote.Server)
		if err != nil {
			return fmt.Errorf("invalid routing.dns.remote.server: %w", err)
		}
		if u.Scheme != "https" {
			return fmt.Errorf("invalid routing.dns.remote.server: scheme must be https, got %q", u.Scheme)
		}
	}

	// Routing.DNS.Remote.Via
	switch c.Routing.DNS.Remote.Via {
	case "proxy", "direct", "":
	default:
		return fmt.Errorf("invalid routing.dns.remote.via: %q (must be \"proxy\", \"direct\", or empty)", c.Routing.DNS.Remote.Via)
	}

	// Transport.CDN validation
	if c.Transport.CDN.Enabled {
		if c.Transport.CDN.Domain == "" {
			return fmt.Errorf("transport.cdn.domain is required when CDN is enabled")
		}
		switch c.Transport.CDN.Mode {
		case "", "h2", "grpc":
		default:
			return fmt.Errorf("invalid transport.cdn.mode: %q (must be \"h2\", \"grpc\", or empty)", c.Transport.CDN.Mode)
		}
	}

	// Transport.Reality validation
	if c.Transport.Reality.Enabled {
		if c.Transport.Reality.PublicKey == "" {
			return fmt.Errorf("transport.reality.public_key is required when Reality is enabled")
		}
	}

	// Transport.WebRTC validation
	if c.Transport.WebRTC.Enabled {
		if c.Transport.WebRTC.SignalURL == "" {
			return fmt.Errorf("transport.webrtc.signal_url is required when WebRTC is enabled")
		}
		if err := validateURL(c.Transport.WebRTC.SignalURL, "transport.webrtc.signal_url"); err != nil {
			return err
		}
	}

	// Log.Level validation
	switch c.Log.Level {
	case "debug", "info", "warn", "error", "":
	default:
		return fmt.Errorf("invalid log.level: %q", c.Log.Level)
	}

	// Log.Format validation
	switch c.Log.Format {
	case "text", "json", "":
	default:
		return fmt.Errorf("invalid log.format: %q", c.Log.Format)
	}

	return nil
}

// Validate checks the server config for obviously wrong values.
func (c *ServerConfig) Validate() error {
	if c.Listen != "" {
		if _, _, err := net.SplitHostPort(c.Listen); err != nil {
			return fmt.Errorf("invalid listen address: %w", err)
		}
	}
	switch c.Cover.Mode {
	case "static", "reverse", "default", "":
	default:
		return fmt.Errorf("invalid cover.mode: %q", c.Cover.Mode)
	}

	// Cover.Mode == "reverse" requires valid ReverseURL
	if c.Cover.Mode == "reverse" {
		if c.Cover.ReverseURL == "" {
			return fmt.Errorf("cover.reverse_url is required when cover.mode is \"reverse\"")
		}
		if err := validateURL(c.Cover.ReverseURL, "cover.reverse_url"); err != nil {
			return err
		}
	}

	// Cover.Mode == "static" requires StaticDir
	if c.Cover.Mode == "static" {
		if c.Cover.StaticDir == "" {
			return fmt.Errorf("cover.static_dir is required when cover.mode is \"static\"")
		}
	}

	// Mesh.CIDR validation when mesh is enabled
	if c.Mesh.Enabled {
		if err := validateCIDR(c.Mesh.CIDR, "mesh.cidr"); err != nil {
			return err
		}
	}

	// Admin.Listen validation when admin is enabled
	if c.Admin.Enabled {
		if c.Admin.Listen != "" {
			if err := validateHostPort(c.Admin.Listen, "admin.listen"); err != nil {
				return err
			}
		}
	}

	// Debug.PprofListen validation
	if c.Debug.PprofListen != "" {
		if err := validateHostPort(c.Debug.PprofListen, "debug.pprof_listen"); err != nil {
			return err
		}
	}

	if c.Cluster.Enabled {
		if c.Cluster.NodeName == "" {
			return fmt.Errorf("cluster.node_name is required when cluster is enabled")
		}
		if c.Cluster.Secret == "" {
			return fmt.Errorf("cluster.secret is required when cluster is enabled")
		}
		for _, p := range c.Cluster.Peers {
			if p.Name == "" {
				return fmt.Errorf("cluster peer name is required")
			}
			if _, _, err := net.SplitHostPort(p.Addr); err != nil {
				return fmt.Errorf("invalid cluster peer address %q: %w", p.Addr, err)
			}
		}
		if c.Cluster.Interval != "" {
			if _, err := time.ParseDuration(c.Cluster.Interval); err != nil {
				return fmt.Errorf("invalid cluster.interval: %w", err)
			}
		}
	}
	return nil
}

// LoadClientConfig loads client config from a YAML file.
// If the keystore is initialized, any "ENC:"-prefixed sensitive fields
// are automatically decrypted. Plaintext values pass through unchanged
// for backward compatibility.
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
	if cfg.Version == 0 {
		cfg.Version = CurrentClientConfigVersion
	}
	// Auto-decrypt sensitive fields if keystore is available.
	if key, err := GetKey(); err == nil {
		if err := decryptSensitiveFields(&cfg, key); err != nil {
			return nil, fmt.Errorf("decrypt config: %w", err)
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
	return &cfg, nil
}

// SaveClientConfig writes a client config to a YAML file.
// If the keystore is initialized, sensitive fields are encrypted before
// writing. The in-memory config is not modified.
func SaveClientConfig(path string, cfg *ClientConfig) error {
	// Work on a copy so we don't mutate the caller's config.
	cp := cfg.DeepCopy()

	// Encrypt sensitive fields if keystore is available.
	if key, err := GetKey(); err == nil {
		if err := encryptSensitiveFields(cp, key); err != nil {
			return fmt.Errorf("encrypt config: %w", err)
		}
	}

	data, err := yaml.Marshal(cp)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// LoadServerConfig loads server config from a YAML file.
// If the keystore is initialized, any "ENC:"-prefixed sensitive fields
// are automatically decrypted. Plaintext values pass through unchanged
// for backward compatibility.
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
	if cfg.Version == 0 {
		cfg.Version = CurrentServerConfigVersion
	}
	// Auto-decrypt sensitive fields if keystore is available.
	if key, err := GetKey(); err == nil {
		if err := decryptServerSensitiveFields(&cfg, key); err != nil {
			return nil, fmt.Errorf("decrypt config: %w", err)
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
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
	cfg.Routing.GeoData.Enabled = true
	cfg.Routing.GeoData.AutoUpdate = true
	return cfg
}

func applyClientDefaults(cfg *ClientConfig) {
	if cfg.Transport.Preferred == "" {
		cfg.Transport.Preferred = "auto"
	}
	// Determine bind address based on AllowLAN setting
	bindHost := "127.0.0.1"
	if cfg.Proxy.AllowLAN {
		bindHost = "0.0.0.0"
	}
	if cfg.Proxy.SOCKS5.Listen == "" {
		cfg.Proxy.SOCKS5.Listen = bindHost + ":1080"
	}
	if cfg.Proxy.HTTP.Listen == "" {
		cfg.Proxy.HTTP.Listen = bindHost + ":8080"
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
	applyGeoDataDefaults(&cfg.Routing.GeoData)
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
	// Obfuscation defaults
	if cfg.Obfs.MaxDelay == "" {
		cfg.Obfs.MaxDelay = "50ms"
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

const (
	defaultGeoBase     = "https://raw.githubusercontent.com/Loyalsoldier/v2ray-rules-dat/release/"
	defaultDirectList  = defaultGeoBase + "direct-list.txt"
	defaultProxyList   = defaultGeoBase + "proxy-list.txt"
	defaultRejectList  = defaultGeoBase + "reject-list.txt"
	defaultGFWList     = defaultGeoBase + "gfw.txt"
	defaultCNCidr      = "https://raw.githubusercontent.com/misakaio/chnroutes2/master/chnroutes.txt"
	defaultPrivateCidr = "" // not needed, private ranges handled by router directly
)

func applyGeoDataDefaults(g *GeoDataConfig) {
	if g.DataDir == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			g.DataDir = home + "/.shuttle/geodata"
		}
	}
	if g.UpdateInterval == "" {
		g.UpdateInterval = "24h"
	}
	if g.DirectListURL == "" {
		g.DirectListURL = defaultDirectList
	}
	if g.ProxyListURL == "" {
		g.ProxyListURL = defaultProxyList
	}
	if g.RejectListURL == "" {
		g.RejectListURL = defaultRejectList
	}
	if g.GFWListURL == "" {
		g.GFWListURL = defaultGFWList
	}
	if g.CNCidrURL == "" {
		g.CNCidrURL = defaultCNCidr
	}
	if g.PrivateCidrURL == "" {
		g.PrivateCidrURL = defaultPrivateCidr
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
	if c.Transport.WebRTC.STUNServers != nil {
		cp.Transport.WebRTC.STUNServers = make([]string, len(c.Transport.WebRTC.STUNServers))
		copy(cp.Transport.WebRTC.STUNServers, c.Transport.WebRTC.STUNServers)
	}
	if c.Transport.WebRTC.TURNServers != nil {
		cp.Transport.WebRTC.TURNServers = make([]string, len(c.Transport.WebRTC.TURNServers))
		copy(cp.Transport.WebRTC.TURNServers, c.Transport.WebRTC.TURNServers)
	}
	if c.QoS.Rules != nil {
		cp.QoS.Rules = make([]QoSRule, len(c.QoS.Rules))
		for i, r := range c.QoS.Rules {
			cp.QoS.Rules[i] = r
			if r.Ports != nil {
				cp.QoS.Rules[i].Ports = make([]uint16, len(r.Ports))
				copy(cp.QoS.Rules[i].Ports, r.Ports)
			}
			if r.Domains != nil {
				cp.QoS.Rules[i].Domains = make([]string, len(r.Domains))
				copy(cp.QoS.Rules[i].Domains, r.Domains)
			}
			if r.Process != nil {
				cp.QoS.Rules[i].Process = make([]string, len(r.Process))
				copy(cp.QoS.Rules[i].Process, r.Process)
			}
		}
	}
	return &cp
}

// DefaultServerConfig returns a config with sensible defaults.
func DefaultServerConfig() *ServerConfig {
	cfg := &ServerConfig{}
	applyServerDefaults(cfg)
	cfg.Transport.H3.Enabled = true
	cfg.Transport.H3.PathPrefix = "/h3"
	cfg.Transport.Reality.Enabled = true
	cfg.Transport.Reality.TargetSNI = "www.microsoft.com"
	cfg.Transport.Reality.TargetAddr = "www.microsoft.com:443"
	return cfg
}

// SaveServerConfig writes a server config to a YAML file.
// If the keystore is initialized, sensitive fields (password, private key,
// admin token, cluster secret) are encrypted before writing. The in-memory
// config is not modified.
//
// TODO: Add `shuttle encrypt-config` / `shuttled encrypt-config` CLI subcommand
// to encrypt an existing plaintext config file in place.
func SaveServerConfig(path string, cfg *ServerConfig) error {
	// Deep copy to avoid mutating the caller's config.
	cp := *cfg
	// Copy slices that may be modified.
	if cfg.Admin.Users != nil {
		cp.Admin.Users = make([]User, len(cfg.Admin.Users))
		copy(cp.Admin.Users, cfg.Admin.Users)
	}

	// Encrypt sensitive fields if keystore is available.
	if key, err := GetKey(); err == nil {
		if err := encryptServerSensitiveFields(&cp, key); err != nil {
			return fmt.Errorf("encrypt config: %w", err)
		}
	}

	data, err := yaml.Marshal(&cp)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0600)
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
