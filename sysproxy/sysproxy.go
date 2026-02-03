// Package sysproxy provides system proxy configuration for different platforms.
package sysproxy

// ProxyConfig represents the proxy configuration to set.
type ProxyConfig struct {
	// Enable turns on system proxy when true, restores original when false
	Enable bool

	// HTTP proxy address (e.g., "127.0.0.1:8080")
	HTTPAddr string

	// SOCKS proxy address (e.g., "127.0.0.1:1080")
	SOCKSAddr string

	// Bypass list - addresses that should not go through proxy
	Bypass []string
}

// DefaultBypass returns the default bypass list for the current platform.
func DefaultBypass() []string {
	return []string{
		"localhost",
		"127.0.0.1",
		"10.*",
		"172.16.*",
		"172.17.*",
		"172.18.*",
		"172.19.*",
		"172.20.*",
		"172.21.*",
		"172.22.*",
		"172.23.*",
		"172.24.*",
		"172.25.*",
		"172.26.*",
		"172.27.*",
		"172.28.*",
		"172.29.*",
		"172.30.*",
		"172.31.*",
		"192.168.*",
		"<local>",
	}
}

// Set configures the system proxy according to the provided config.
// This is implemented per-platform in sysproxy_*.go files.
func Set(cfg ProxyConfig) error {
	return set(cfg)
}

// Clear disables the system proxy and restores original settings.
func Clear() error {
	return Set(ProxyConfig{Enable: false})
}
