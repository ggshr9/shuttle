//go:build !darwin && !windows && !linux

package sysproxy

import (
	"log/slog"
	"runtime"
)

// set is a no-op on unsupported platforms (mobile, etc.)
func set(cfg ProxyConfig) error {
	slog.Warn("system proxy not supported on this platform", "platform", runtime.GOOS)
	return nil
}
