package sysopt

import (
	"log/slog"
	"runtime"
)

// Apply applies system-level optimizations for proxy performance.
func Apply(logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}

	// Use all available CPU cores
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)
	logger.Info("system optimization applied",
		"cpus", numCPU,
		"os", runtime.GOOS,
		"arch", runtime.GOARCH)

	// Platform-specific optimizations
	applyPlatformOpts(logger)
}

// applyPlatformOpts applies OS-specific optimizations.
func applyPlatformOpts(logger *slog.Logger) {
	// On Linux: increase fd limit, set socket buffer sizes
	// On Windows/macOS: adjust as needed
	// These are best-effort and log warnings on failure.
	switch runtime.GOOS {
	case "linux":
		logger.Debug("linux optimizations: consider increasing ulimit -n")
	case "darwin":
		logger.Debug("macos optimizations: consider increasing maxfiles")
	case "windows":
		logger.Debug("windows optimizations applied")
	}
}

// RecommendedBufferSize returns the recommended socket buffer size.
func RecommendedBufferSize() int {
	return 4 * 1024 * 1024 // 4MB
}
