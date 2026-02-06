// Package autostart provides cross-platform auto-start functionality.
package autostart

import (
	"os"
	"path/filepath"
)

// Config holds auto-start configuration.
type Config struct {
	// AppName is the display name of the application
	AppName string

	// AppPath is the full path to the executable
	AppPath string

	// Args are additional command line arguments
	Args []string

	// Hidden starts the app minimized/hidden (where supported)
	Hidden bool
}

// DefaultConfig returns a config with the current executable.
func DefaultConfig() (*Config, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return nil, err
	}

	return &Config{
		AppName: "Shuttle",
		AppPath: exe,
		Hidden:  true,
	}, nil
}

// IsEnabled checks if auto-start is currently enabled.
func IsEnabled() (bool, error) {
	cfg, err := DefaultConfig()
	if err != nil {
		return false, err
	}
	return isEnabled(cfg)
}

// Enable enables auto-start for the application.
func Enable() error {
	cfg, err := DefaultConfig()
	if err != nil {
		return err
	}
	return enable(cfg)
}

// Disable disables auto-start for the application.
func Disable() error {
	cfg, err := DefaultConfig()
	if err != nil {
		return err
	}
	return disable(cfg)
}

// Toggle toggles auto-start state and returns the new state.
func Toggle() (bool, error) {
	enabled, err := IsEnabled()
	if err != nil {
		return false, err
	}

	if enabled {
		return false, Disable()
	}
	return true, Enable()
}
