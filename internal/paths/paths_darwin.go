//go:build darwin && !ios

package paths

import (
	"os"
	"path/filepath"
)

func resolve(scope Scope) Paths {
	if scope == ScopeSystem {
		return Paths{
			ConfigDir: "/Library/Application Support/Shuttle",
			LogDir:    "/Library/Logs/Shuttle",
		}
	}
	home, _ := os.UserHomeDir()
	return Paths{
		ConfigDir: filepath.Join(home, "Library", "Application Support", "Shuttle"),
		LogDir:    filepath.Join(home, "Library", "Logs", "Shuttle"),
	}
}
