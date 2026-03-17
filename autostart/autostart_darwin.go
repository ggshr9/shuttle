//go:build darwin && !ios

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>%s
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
</dict>
</plist>
`

func getLaunchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", "com.shuttle.proxy.plist"), nil
}

func isEnabled(cfg *Config) (bool, error) {
	path, err := getLaunchAgentPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	return err == nil, nil
}

func enable(cfg *Config) error {
	path, err := getLaunchAgentPath()
	if err != nil {
		return err
	}

	// Ensure LaunchAgents directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}

	// Build arguments
	var argsXML string
	for _, arg := range cfg.Args {
		argsXML += fmt.Sprintf("\n        <string>%s</string>", arg)
	}
	if cfg.Hidden {
		argsXML += "\n        <string>--hidden</string>"
	}

	// Generate plist content
	content := fmt.Sprintf(plistTemplate, "com.shuttle.proxy", cfg.AppPath, argsXML)

	// Write plist file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	return nil
}

func disable(cfg *Config) error {
	path, err := getLaunchAgentPath()
	if err != nil {
		return err
	}

	// Remove plist file
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
