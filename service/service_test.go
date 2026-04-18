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
	got := renderSystemdUnit(cfg, ScopeSystem)
	mustContain(t, got, "Description=Shuttle Server")
	mustContain(t, got, "ExecStart=/usr/local/bin/shuttled run -c /etc/shuttle/server.yaml")
	mustContain(t, got, "Restart=always")
	mustContain(t, got, "RestartSec=5")
	mustContain(t, got, "LimitNOFILE=65535")
	mustContain(t, got, "WantedBy=multi-user.target")

	user := renderSystemdUnit(cfg, ScopeUser)
	mustContain(t, user, "WantedBy=default.target")
}

func mustContain(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("rendered unit missing %q:\n%s", sub, s)
	}
}
