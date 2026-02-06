package autostart

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

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

	// Verify AppPath is absolute
	if !filepath.IsAbs(cfg.AppPath) {
		t.Errorf("AppPath %q is not absolute", cfg.AppPath)
	}

	// Verify AppPath exists
	if _, err := os.Stat(cfg.AppPath); err != nil {
		t.Errorf("AppPath %q does not exist: %v", cfg.AppPath, err)
	}

	// Hidden should default to true
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
	// This test just verifies the function doesn't panic
	// We don't test the actual result as it depends on system state
	_, err := IsEnabled()
	// On mobile platforms or test environments, this may return an error
	// but shouldn't panic
	_ = err
}

func TestGetAutoStartArgs(t *testing.T) {
	// GetAutoStartArgs should return nil when --hidden is not in args
	args := GetAutoStartArgs()

	// During testing, os.Args won't have --hidden, so expect nil
	// unless the test runner is using it
	if args != nil {
		for _, arg := range args {
			if arg != "--hidden" {
				t.Errorf("unexpected arg %q", arg)
			}
		}
	}
}

// TestEnableDisable tests the enable/disable cycle
// This test is skipped on CI as it modifies system state
func TestEnableDisable(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping on CI - modifies system state")
	}

	// Skip on mobile platforms
	if runtime.GOOS == "android" || runtime.GOOS == "ios" {
		t.Skip("Skipping on mobile platforms")
	}

	// First check current state
	wasEnabled, err := IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled() error = %v", err)
	}

	// If currently disabled, test enable then disable
	if !wasEnabled {
		if err := Enable(); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}

		enabled, err := IsEnabled()
		if err != nil {
			t.Fatalf("IsEnabled() after Enable error = %v", err)
		}
		if !enabled {
			t.Error("IsEnabled() = false after Enable(), want true")
		}

		if err := Disable(); err != nil {
			t.Fatalf("Disable() error = %v", err)
		}

		enabled, err = IsEnabled()
		if err != nil {
			t.Fatalf("IsEnabled() after Disable error = %v", err)
		}
		if enabled {
			t.Error("IsEnabled() = true after Disable(), want false")
		}
	} else {
		// Currently enabled, test disable then re-enable
		if err := Disable(); err != nil {
			t.Fatalf("Disable() error = %v", err)
		}

		enabled, err := IsEnabled()
		if err != nil {
			t.Fatalf("IsEnabled() after Disable error = %v", err)
		}
		if enabled {
			t.Error("IsEnabled() = true after Disable(), want false")
		}

		// Re-enable to restore original state
		if err := Enable(); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}
	}
}

func TestToggle(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping on CI - modifies system state")
	}

	// Skip on mobile platforms
	if runtime.GOOS == "android" || runtime.GOOS == "ios" {
		t.Skip("Skipping on mobile platforms")
	}

	// Get initial state
	initial, err := IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled() error = %v", err)
	}

	// Toggle once
	newState, err := Toggle()
	if err != nil {
		t.Fatalf("Toggle() error = %v", err)
	}

	if newState == initial {
		t.Errorf("Toggle() returned %v, same as initial state", newState)
	}

	// Toggle back to restore state
	finalState, err := Toggle()
	if err != nil {
		t.Fatalf("Toggle() restore error = %v", err)
	}

	if finalState != initial {
		t.Errorf("After two toggles, state = %v, want %v", finalState, initial)
	}
}
