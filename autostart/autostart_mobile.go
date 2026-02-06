//go:build android || ios

package autostart

// Mobile platforms handle auto-start differently (system-level VPN always-on)
// These are no-op implementations.

func isEnabled(cfg *Config) (bool, error) {
	return false, nil
}

func enable(cfg *Config) error {
	return nil
}

func disable(cfg *Config) error {
	return nil
}

// GetAutoStartArgs returns nil on mobile.
func GetAutoStartArgs() []string {
	return nil
}
