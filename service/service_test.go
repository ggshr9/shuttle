package service

import (
	"strings"
	"testing"
)

func TestStatusString(t *testing.T) {
	tests := []struct {
		s    Status
		want string
	}{
		{StatusRunning, "running"},
		{StatusStopped, "stopped"},
		{StatusNotInstalled, "not-installed"},
		{StatusUnknown, "unknown"},
	}
	for _, tc := range tests {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("Status(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}

func TestRenderSystemdUnit(t *testing.T) {
	cfg := Config{
		Name:        "shuttled",
		Description: "Shuttle Server",
		BinaryPath:  "/usr/local/bin/shuttled",
		Args:        []string{"run", "-c", "/etc/shuttle/server.yaml"},
		Restart:     true,
		RestartSec:  5,
		LimitNOFILE: 65535,
	}
	got := renderSystemdUnit(&cfg, ScopeSystem)
	mustContain(t, got, "Description=Shuttle Server")
	mustContain(t, got, "ExecStart=/usr/local/bin/shuttled run -c /etc/shuttle/server.yaml")
	mustContain(t, got, "Restart=always")
	mustContain(t, got, "RestartSec=5")
	mustContain(t, got, "LimitNOFILE=65535")
	mustContain(t, got, "WantedBy=multi-user.target")

	user := renderSystemdUnit(&cfg, ScopeUser)
	mustContain(t, user, "WantedBy=default.target")
}

func mustContain(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("rendered unit missing %q:\n%s", sub, s)
	}
}

func TestRenderSystemdUnitRestartFalse(t *testing.T) {
	got := renderSystemdUnit(&Config{
		Name:        "shuttled",
		Description: "Shuttle Server",
		BinaryPath:  "/usr/local/bin/shuttled",
		Args:        []string{"run"},
		Restart:     false,
	}, ScopeSystem)
	mustContain(t, got, "Restart=no")
}

func TestRenderSystemdUnitUserDirective(t *testing.T) {
	got := renderSystemdUnit(&Config{
		Name:        "shuttled",
		Description: "Shuttle Server",
		BinaryPath:  "/usr/local/bin/shuttled",
		Args:        []string{"run"},
		User:        "shuttle",
	}, ScopeSystem)
	mustContain(t, got, "User=shuttle")

	// ScopeUser must not emit User directive even if cfg.User is set.
	userScope := renderSystemdUnit(&Config{
		Name:        "shuttled",
		Description: "Shuttle Server",
		BinaryPath:  "/usr/local/bin/shuttled",
		Args:        []string{"run"},
		User:        "shuttle",
	}, ScopeUser)
	if strings.Contains(userScope, "User=shuttle") {
		t.Errorf("user-scope unit should not emit User= directive, got:\n%s", userScope)
	}
}

func TestRenderSystemdUnitArgsWithSpaces(t *testing.T) {
	got := renderSystemdUnit(&Config{
		Name:        "shuttled",
		Description: "Shuttle Server",
		BinaryPath:  "/usr/local/bin/shuttled",
		Args:        []string{"run", "-c", "/Library/App Support/config.yaml"},
	}, ScopeSystem)
	// Path with space must be quoted so systemd parses it as one arg.
	mustContain(t, got, `ExecStart=/usr/local/bin/shuttled run -c "/Library/App Support/config.yaml"`)
}

func TestRenderSystemdUnitSanitizesNewlines(t *testing.T) {
	got := renderSystemdUnit(&Config{
		Name:        "shuttled",
		Description: "Shuttle\nExecStart=/evil",
		BinaryPath:  "/usr/local/bin/shuttled",
		Args:        []string{"run"},
	}, ScopeSystem)
	if strings.Contains(got, "Description=Shuttle\n") {
		t.Errorf("newline in Description must be collapsed, got:\n%s", got)
	}
	// The injected ExecStart line should not appear as a standalone directive.
	lines := strings.Split(got, "\n")
	execStartCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "ExecStart=") {
			execStartCount++
		}
	}
	if execStartCount != 1 {
		t.Errorf("expected exactly one ExecStart= line, got %d:\n%s", execStartCount, got)
	}
}

func TestRenderLaunchdPlist(t *testing.T) {
	cfg := Config{
		Name:        "shuttled",
		Description: "Shuttle Server",
		BinaryPath:  "/usr/local/bin/shuttled",
		Args:        []string{"run", "-c", "/Library/Application Support/Shuttle/server.yaml"},
		LogDir:      "/Library/Logs/Shuttle",
		Restart:     true,
	}
	got := renderLaunchdPlist(&cfg)
	mustContain(t, got, "<key>Label</key>\n\t<string>com.shuttle.shuttled</string>")
	mustContain(t, got, "<key>KeepAlive</key>\n\t<true/>")
	mustContain(t, got, "<key>RunAtLoad</key>\n\t<true/>")
	mustContain(t, got, "<string>/usr/local/bin/shuttled</string>")
	mustContain(t, got, "<string>run</string>")
	mustContain(t, got, "/Library/Logs/Shuttle/shuttled.log")
	mustContain(t, got, "/Library/Logs/Shuttle/shuttled.err.log")
}
