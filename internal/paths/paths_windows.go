//go:build windows

package paths

import (
	"os"
	"path/filepath"
)

func resolve(scope Scope) Paths {
	if scope == ScopeSystem {
		root := os.Getenv("ProgramData")
		if root == "" {
			root = `C:\ProgramData`
		}
		return Paths{
			ConfigDir: filepath.Join(root, "Shuttle"),
			LogDir:    filepath.Join(root, "Shuttle", "logs"),
		}
	}
	root := os.Getenv("AppData")
	if root == "" {
		home, _ := os.UserHomeDir()
		root = filepath.Join(home, "AppData", "Roaming")
	}
	return Paths{
		ConfigDir: filepath.Join(root, "Shuttle"),
		LogDir:    filepath.Join(root, "Shuttle", "logs"),
	}
}
