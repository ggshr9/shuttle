//go:build sandbox

// Sandbox integration tests for autostart.
//
// These tests run ONLY inside Docker (via ./scripts/test.sh --sandbox).
// They exercise real Enable/Disable/Toggle operations in an isolated container.

package autostart

import (
	"testing"
)

func TestSandboxEnableDisable(t *testing.T) {
	// In Docker there's no LaunchAgent/systemd user session,
	// so these may error — we're testing they don't panic.
	err := Enable()
	if err != nil {
		t.Logf("Enable() error = %v (expected in Docker)", err)
	}

	err = Disable()
	if err != nil {
		t.Logf("Disable() error = %v (expected in Docker)", err)
	}
}

func TestSandboxToggle(t *testing.T) {
	_, err := Toggle()
	if err != nil {
		t.Logf("Toggle() error = %v (expected in Docker)", err)
	}
}
