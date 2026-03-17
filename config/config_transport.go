package config

// YamuxConfig holds yamux multiplexer tuning parameters.
type YamuxConfig struct {
	MaxStreamWindowSize    uint32 `yaml:"max_stream_window_size" json:"max_stream_window_size"`       // default 256KB
	KeepAliveInterval      int    `yaml:"keep_alive_interval" json:"keep_alive_interval"`             // seconds, default 30
	ConnectionWriteTimeout int    `yaml:"connection_write_timeout" json:"connection_write_timeout"`    // seconds, default 10
}

// CongestionConfig configures congestion control.
type CongestionConfig struct {
	Mode       string `yaml:"mode" json:"mode"`               // "adaptive", "bbr", "brutal"
	BrutalRate uint64 `yaml:"brutal_rate" json:"brutal_rate"` // bytes/sec for brutal mode (default 100MB/s)
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

// MultipathConfig configures QUIC multipath behavior for H3 transport.
type MultipathConfig struct {
	Enabled       bool     `yaml:"enabled" json:"enabled"`
	Interfaces    []string `yaml:"interfaces,omitempty" json:"interfaces,omitempty"` // bind to specific interfaces, empty = auto-detect
	Mode          string   `yaml:"mode" json:"mode"`                                 // "redundant" | "aggregate" | "failover"
	ProbeInterval string   `yaml:"probe_interval,omitempty" json:"probe_interval,omitempty"` // duration string, e.g. "5s"
}

// H3Config configures HTTP/3 transport.
type H3Config struct {
	Enabled            bool            `yaml:"enabled" json:"enabled"`
	PathPrefix         string          `yaml:"path_prefix" json:"path_prefix"`
	InsecureSkipVerify bool            `yaml:"insecure_skip_verify,omitempty" json:"insecure_skip_verify,omitempty"`
	Multipath          MultipathConfig `yaml:"multipath,omitempty" json:"multipath,omitempty"`
}

// RealityConfig configures Reality transport.
type RealityConfig struct {
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	ServerName   string `yaml:"server_name" json:"server_name"`
	ShortID      string `yaml:"short_id" json:"short_id"`
	PublicKey    string `yaml:"public_key" json:"public_key"`
	PostQuantum  bool   `yaml:"post_quantum,omitempty" json:"post_quantum,omitempty"` // Enable hybrid X25519 + ML-KEM-768 key exchange
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
	Enabled     bool     `yaml:"enabled" json:"enabled"`
	TargetSNI   string   `yaml:"target_sni" json:"target_sni"`
	TargetAddr  string   `yaml:"target_addr" json:"target_addr"`
	ShortIDs    []string `yaml:"short_ids" json:"short_ids"`
	PostQuantum bool     `yaml:"post_quantum,omitempty" json:"post_quantum,omitempty"` // Enable hybrid X25519 + ML-KEM-768 key exchange
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
