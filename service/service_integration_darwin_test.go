//go:build integration_darwin

package service

import (
	"os"
	"testing"
)

func TestLaunchdFullCycle(t *testing.T) {
	mgr, err := New("shuttle-ci-test", ScopeUser)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer mgr.Uninstall(false)

	cfg := Config{
		Name:        "shuttle-ci-test",
		DisplayName: "Shuttle CI Test",
		Description: "Integration test",
		BinaryPath:  "/bin/sleep",
		Args:        []string{"3600"},
		Scope:       ScopeUser,
		LogDir:      "/tmp/shuttle-ci-logs",
		Restart:     false,
	}
	_ = os.MkdirAll(cfg.LogDir, 0755)

	if err := mgr.Install(cfg); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := mgr.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	s, _ := mgr.Status()
	if s != StatusRunning && s != StatusStopped {
		t.Errorf("Status after Start = %v, want Running or Stopped", s)
	}

	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
