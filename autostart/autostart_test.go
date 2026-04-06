package autostart

import (
	"os"
	"path/filepath"
	"testing"
)

// Host-safe tests only — no system state modifications.
// System state tests live in autostart_sandbox_test.go (//go:build sandbox).

func TestDefaultConfig(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig() error = %v", err)
	}

	if cfg.AppName != "Shuttle" {
		t.Errorf("AppName = %q, want %q", cfg.AppName, "Shuttle")
	}

	if cfg.AppPath == "" {
		t.Error("AppPath is empty")
	}

	if !filepath.IsAbs(cfg.AppPath) {
		t.Errorf("AppPath %q is not absolute", cfg.AppPath)
	}

	if _, err := os.Stat(cfg.AppPath); err != nil {
		t.Errorf("AppPath %q does not exist: %v", cfg.AppPath, err)
	}

	if !cfg.Hidden {
		t.Error("Hidden = false, want true")
	}
}

func TestConfig(t *testing.T) {
	cfg := &Config{
		AppName: "TestApp",
		AppPath: "/usr/bin/test",
		Args:    []string{"--arg1", "--arg2"},
		Hidden:  true,
	}

	if cfg.AppName != "TestApp" {
		t.Errorf("AppName = %q, want %q", cfg.AppName, "TestApp")
	}
	if cfg.AppPath != "/usr/bin/test" {
		t.Errorf("AppPath = %q, want %q", cfg.AppPath, "/usr/bin/test")
	}
	if len(cfg.Args) != 2 {
		t.Errorf("len(Args) = %d, want 2", len(cfg.Args))
	}
	if !cfg.Hidden {
		t.Error("Hidden = false, want true")
	}
}

func TestIsEnabled(t *testing.T) {
	// Read-only check — doesn't modify system state
	_, err := IsEnabled()
	_ = err
}

func TestGetAutoStartArgs(t *testing.T) {
	args := GetAutoStartArgs()

	for _, arg := range args {
		if arg != "--hidden" {
			t.Errorf("unexpected arg %q", arg)
		}
	}
}
