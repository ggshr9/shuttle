package config

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

// ServerMeshConfig configures the server-side mesh virtual LAN.
type ServerMeshConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	CIDR       string `yaml:"cidr" json:"cidr"`             // e.g. "10.7.0.0/24"
	P2PEnabled bool   `yaml:"p2p_enabled" json:"p2p_enabled"` // Enable P2P signaling
}
