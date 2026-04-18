package service

import (
	"fmt"
	"strings"
)

// renderSystemdUnit returns the text of a systemd unit file for the given
// config and scope.
func renderSystemdUnit(cfg Config, scope Scope) string {
	restart := "no"
	if cfg.Restart {
		restart = "always"
	}
	sec := cfg.RestartSec
	if sec == 0 {
		sec = 5
	}
	wantedBy := "multi-user.target"
	if scope == ScopeUser {
		wantedBy = "default.target"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "[Unit]\nDescription=%s\nAfter=network-online.target\nWants=network-online.target\n\n", cfg.Description)
	sb.WriteString("[Service]\n")
	fmt.Fprintf(&sb, "ExecStart=%s %s\n", cfg.BinaryPath, strings.Join(cfg.Args, " "))
	fmt.Fprintf(&sb, "Restart=%s\nRestartSec=%d\n", restart, sec)
	if cfg.LimitNOFILE > 0 {
		fmt.Fprintf(&sb, "LimitNOFILE=%d\n", cfg.LimitNOFILE)
	}
	if cfg.User != "" && scope == ScopeSystem {
		fmt.Fprintf(&sb, "User=%s\n", cfg.User)
	}
	fmt.Fprintf(&sb, "\n[Install]\nWantedBy=%s\n", wantedBy)
	return sb.String()
}
