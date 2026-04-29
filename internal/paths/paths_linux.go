//go:build linux && !android

package paths

import (
	"os"
	"path/filepath"
)

func resolve(scope Scope) Paths {
	if scope == ScopeSystem {
		return Paths{
			ConfigDir: "/etc/shuttle",
			LogDir:    "/var/log/shuttle",
		}
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, _ := os.UserHomeDir()
		configHome = filepath.Join(home, ".config")
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, _ := os.UserHomeDir()
		stateHome = filepath.Join(home, ".local", "state")
	}
	return Paths{
		ConfigDir: filepath.Join(configHome, "shuttle"),
		LogDir:    filepath.Join(stateHome, "shuttle", "logs"),
	}
}
