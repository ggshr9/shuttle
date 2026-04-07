package config

// RoutingConfig configures routing rules.
type RoutingConfig struct {
	RuleChain []RuleChainEntry `yaml:"rule_chain,omitempty" json:"rule_chain,omitempty"` // ordered rules evaluated before legacy
	Rules     []RouteRule      `yaml:"rules" json:"rules"`
	Default   string           `yaml:"default" json:"default"` // "proxy", "direct"
	DNS       DNSConfig        `yaml:"dns" json:"dns"`
	GeoData   GeoDataConfig    `yaml:"geodata" json:"geodata"`
}

// RuleChainEntry defines a single rule in the ordered rule chain.
type RuleChainEntry struct {
	Match  RuleMatch `yaml:"match" json:"match"`
	Logic  string    `yaml:"logic,omitempty" json:"logic,omitempty"`   // "and" (default) | "or"
	Negate bool      `yaml:"negate,omitempty" json:"negate,omitempty"` // invert the match result
	Action string    `yaml:"action" json:"action"`
}

// RuleMatch defines match conditions for a rule chain entry.
type RuleMatch struct {
	Domain        []string `yaml:"domain,omitempty" json:"domain,omitempty"`
	DomainSuffix  []string `yaml:"domain_suffix,omitempty" json:"domain_suffix,omitempty"`
	DomainKeyword []string `yaml:"domain_keyword,omitempty" json:"domain_keyword,omitempty"`
	GeoSite       []string `yaml:"geosite,omitempty" json:"geosite,omitempty"`
	IPCIDR        []string `yaml:"ip_cidr,omitempty" json:"ip_cidr,omitempty"`
	GeoIP         []string `yaml:"geoip,omitempty" json:"geoip,omitempty"`
	Port          []string `yaml:"port,omitempty" json:"port,omitempty"`
	SrcIP         []string `yaml:"src_ip,omitempty" json:"src_ip,omitempty"`
	Process       []string `yaml:"process,omitempty" json:"process,omitempty"`
	Protocol      []string `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	NetworkType   []string `yaml:"network_type,omitempty" json:"network_type,omitempty"`
	RuleProvider  []string `yaml:"rule_provider,omitempty" json:"rule_provider,omitempty"`
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
	Mode           string   `yaml:"mode,omitempty" json:"mode,omitempty"`                       // "normal" or "fake-ip"
	FakeIPRange    string   `yaml:"fake_ip_range,omitempty" json:"fake_ip_range,omitempty"`     // CIDR for fake-ip pool (default "198.18.0.0/15")
	FakeIPFilter   []string `yaml:"fake_ip_filter,omitempty" json:"fake_ip_filter,omitempty"`   // domains to bypass fake-ip
	Persist        bool     `yaml:"persist,omitempty" json:"persist,omitempty"`                  // persist fake-ip mappings across restarts
	Hosts          map[string]string  `yaml:"hosts,omitempty" json:"hosts,omitempty"`            // static hostname → IP mappings (supports *.example.com wildcards)
	DomainPolicy   []DomainPolicyEntry `yaml:"domain_policy,omitempty" json:"domain_policy,omitempty"` // per-domain nameserver policy
}

// DomainPolicyEntry maps a domain pattern to a specific DNS server.
// Domain supports "+.example.com" (matches example.com and all subdomains)
// or exact match like "corp.internal".
type DomainPolicyEntry struct {
	Domain string `yaml:"domain" json:"domain"`
	Server string `yaml:"server" json:"server"` // DoH URL (https://...) or plain UDP (host:port)
}

// DNSRemote configures the remote DNS server.
type DNSRemote struct {
	Server string `yaml:"server" json:"server"`
	Via    string `yaml:"via" json:"via"` // "proxy" or "direct"
}
