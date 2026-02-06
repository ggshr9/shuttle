//go:build linux

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const desktopTemplate = `[Desktop Entry]
Type=Application
Name=%s
Exec=%s
Icon=shuttle
Terminal=false
Categories=Network;
StartupNotify=false
X-GNOME-Autostart-enabled=true
`

func getAutostartPath() (string, error) {
	// Try XDG_CONFIG_HOME first
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "autostart", "shuttle.desktop"), nil
}

func isEnabled(cfg *Config) (bool, error) {
	path, err := getAutostartPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	return err == nil, nil
}

func enable(cfg *Config) error {
	path, err := getAutostartPath()
	if err != nil {
		return err
	}

	// Ensure autostart directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create autostart dir: %w", err)
	}

	// Build exec command
	exec := cfg.AppPath
	for _, arg := range cfg.Args {
		exec += fmt.Sprintf(" %s", arg)
	}
	if cfg.Hidden {
		exec += " --hidden"
	}

	// Generate desktop file content
	content := fmt.Sprintf(desktopTemplate, cfg.AppName, exec)

	// Write desktop file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write desktop file: %w", err)
	}

	return nil
}

func disable(cfg *Config) error {
	path, err := getAutostartPath()
	if err != nil {
		return err
	}

	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil // Already disabled
	}
	return err
}

// GetAutoStartArgs returns args that indicate the app was auto-started.
func GetAutoStartArgs() []string {
	for _, arg := range os.Args[1:] {
		if strings.Contains(arg, "--hidden") {
			return []string{"--hidden"}
		}
	}
	return nil
}
