//go:build integration_windows

package service

import (
	"os"
	"testing"
)

func TestWindowsSCMFullCycle(t *testing.T) {
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

	if err := mgr.Install(cfg); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := mgr.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	s, _ := mgr.Status()
	if s != StatusRunning && s != StatusStopped {
		t.Errorf("Status after Start = %v, want Running/Stopped", s)
	}

	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
