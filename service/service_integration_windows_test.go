//go:build integration_windows

package service

import (
	"os"
	"testing"
)

// TestWindowsSCMInstallUninstall exercises Install/Status/Uninstall via the
// Windows Service Control Manager. It intentionally skips Start/Stop because
// driving those requires a binary that implements StartServiceCtrlDispatcher;
// the broader lifecycle is covered by the Linux sandbox test which uses
// systemd (which does not impose that constraint).
func TestWindowsSCMInstallUninstall(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("Windows service tests require CI administrator context")
	}
	mgr, err := New("shuttle-ci-test", ScopeSystem)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer mgr.Uninstall(false)

	cfg := Config{
		Name:        "shuttle-ci-test",
		DisplayName: "Shuttle CI Test",
		Description: "Integration test",
		BinaryPath:  `C:\Windows\System32\cmd.exe`,
		Args:        []string{"/c", "timeout /t 3600"},
		Scope:       ScopeSystem,
		LogDir:      `C:\Windows\Temp\shuttle-ci`,
		Restart:     false,
	}

	if err := mgr.Install(&cfg); err != nil {
		t.Fatalf("Install: %v", err)
	}
	s, err := mgr.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if s != StatusStopped && s != StatusRunning {
		t.Errorf("Status after Install = %v, want Stopped or Running (not NotInstalled)", s)
	}
	if err := mgr.Uninstall(false); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	s, err = mgr.Status()
	if err != nil {
		t.Fatalf("Status after Uninstall: %v", err)
	}
	if s != StatusNotInstalled {
		t.Errorf("Status after Uninstall = %v, want NotInstalled", s)
	}
}
