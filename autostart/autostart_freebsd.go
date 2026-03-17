//go:build freebsd

package autostart

// FreeBSD autostart is not yet supported — no-op stubs.

func isEnabled(cfg *Config) (bool, error) {
	return false, nil
}

func enable(cfg *Config) error {
	return nil
}

func disable(cfg *Config) error {
	return nil
}

func GetAutoStartArgs() []string {
	return nil
}
