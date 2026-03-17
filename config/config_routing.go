package config

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
