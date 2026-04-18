package service

import (
	"fmt"
	"path/filepath"
	"strings"
)

// renderSystemdUnit returns the text of a systemd unit file for the given
// config and scope.
func renderSystemdUnit(cfg *Config, scope Scope) string {
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
	fmt.Fprintf(&sb, "[Unit]\nDescription=%s\nAfter=network-online.target\nWants=network-online.target\n\n",
		sanitizeUnitValue(cfg.Description))

	sb.WriteString("[Service]\n")
	execStart := cfg.BinaryPath
	if len(cfg.Args) > 0 {
		execStart += " " + joinUnitArgs(cfg.Args)
	}
	fmt.Fprintf(&sb, "ExecStart=%s\n", execStart)
	fmt.Fprintf(&sb, "Restart=%s\nRestartSec=%d\n", restart, sec)
	if cfg.LimitNOFILE > 0 {
		fmt.Fprintf(&sb, "LimitNOFILE=%d\n", cfg.LimitNOFILE)
	}
	if cfg.User != "" && scope == ScopeSystem {
		fmt.Fprintf(&sb, "User=%s\n", sanitizeUnitValue(cfg.User))
	}
	fmt.Fprintf(&sb, "\n[Install]\nWantedBy=%s\n", wantedBy)
	return sb.String()
}

// joinUnitArgs joins arguments for use in a systemd ExecStart= directive.
// Arguments containing whitespace, quotes, or backslashes are double-quoted
// with embedded quotes/backslashes escaped per systemd's quoting rules.
func joinUnitArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\n\"\\") {
			a = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(a) + `"`
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}

// sanitizeUnitValue collapses newlines in a single-line unit directive value
// to prevent injection of additional directives.
func sanitizeUnitValue(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")
}

// renderLaunchdPlist returns the text of a launchd property-list file for the
// given service config. Log paths are derived from cfg.LogDir.
func renderLaunchdPlist(cfg *Config) string {
	var args strings.Builder
	args.WriteString("\t\t<string>" + xmlEscape(cfg.BinaryPath) + "</string>\n")
	for _, a := range cfg.Args {
		args.WriteString("\t\t<string>" + xmlEscape(a) + "</string>\n")
	}
	keepAlive := "<false/>"
	if cfg.Restart {
		keepAlive = "<true/>"
	}
	logOut := filepath.Join(cfg.LogDir, cfg.Name+".log")
	logErr := filepath.Join(cfg.LogDir, cfg.Name+".err.log")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.shuttle.%s</string>
	<key>ProgramArguments</key>
	<array>
%s	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	%s
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, cfg.Name, args.String(), keepAlive, xmlEscape(logOut), xmlEscape(logErr))
}

func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return r.Replace(s)
}
