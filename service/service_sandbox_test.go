//go:build sandbox && linux

package service

import (
	"os"
	"os/exec"
	"testing"
)

func TestSandboxSystemdFullCycle(t *testing.T) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		t.Skip("systemctl not found")
	}
	if os.Geteuid() != 0 {
		t.Skip("requires root for system-scope systemd")
	}
	if out, err := exec.Command("systemctl", "is-system-running").CombinedOutput(); err != nil && len(out) > 0 {
		// "offline" / "maintenance" / stopped indicates systemd is present but not usable.
		t.Skipf("systemd not ready: %s", string(out))
	}

	mgr, err := New("shuttle-test", ScopeSystem)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cfg := Config{
		Name:        "shuttle-test",
		Description: "Shuttle Test Service",
		BinaryPath:  "/bin/sleep",
		Args:        []string{"3600"},
		Scope:       ScopeSystem,
		Restart:     false,
	}
	defer mgr.Uninstall(false)

	if err := mgr.Install(&cfg); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := mgr.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	s, _ := mgr.Status()
	if s != StatusRunning {
		t.Errorf("Status after Start = %v, want Running", s)
	}
	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	s, _ = mgr.Status()
	if s != StatusStopped {
		t.Errorf("Status after Stop = %v, want Stopped", s)
	}
}
