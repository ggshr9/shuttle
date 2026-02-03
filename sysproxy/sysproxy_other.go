//go:build !darwin && !windows && !linux

package sysproxy

// set is a no-op on unsupported platforms (mobile, etc.)
func set(cfg ProxyConfig) error {
	return nil
}
