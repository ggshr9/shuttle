package config

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

// ObfsConfig configures traffic obfuscation.
type ObfsConfig struct {
	PaddingEnabled bool   `yaml:"padding_enabled" json:"padding_enabled"`
	ShapingEnabled bool   `yaml:"shaping_enabled" json:"shaping_enabled"`
	MinDelay       string `yaml:"min_delay" json:"min_delay"` // duration string, default "0s"
	MaxDelay       string `yaml:"max_delay" json:"max_delay"` // duration string, default "50ms"
	ChunkSize      int    `yaml:"chunk_size" json:"chunk_size"` // min chunk size for splitting (default 64)
}
