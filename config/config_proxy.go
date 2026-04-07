package config

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
	IPv6CIDR   string   `yaml:"ipv6_cidr,omitempty" json:"ipv6_cidr,omitempty"` // e.g. "fd00::1/64"
	MTU        int      `yaml:"mtu" json:"mtu"`
	AutoRoute  bool     `yaml:"auto_route" json:"auto_route"`
	TunFD      int      `yaml:"-" json:"-"`                        // externally provided fd (Android)
	PerAppMode string   `yaml:"per_app_mode" json:"per_app_mode"` // "allow" / "deny" / ""
	AppList    []string `yaml:"app_list" json:"app_list"`          // package names / bundle IDs
}
