package config

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

// ReputationConfig configures IP reputation tracking for anti-probing defense.
type ReputationConfig struct {
	Enabled     bool `yaml:"enabled" json:"enabled"`
	MaxFailures int  `yaml:"max_failures" json:"max_failures"` // failures before ban (default 5)
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
