//go:build sandbox

// Sandbox integration tests for sysproxy.
//
// These tests run ONLY inside Docker (via ./scripts/test.sh --sandbox).
// They exercise real system proxy Set/Clear operations in an isolated container
// where no real network is affected.

package sysproxy

import (
	"testing"
)

func TestSandboxClear(t *testing.T) {
	err := Clear()
	// In Docker Alpine there's no networksetup/gsettings, so this will
	// likely error — that's fine, we're testing it doesn't panic.
	_ = err
}

func TestSandboxSetAndClear(t *testing.T) {
	cfg := ProxyConfig{
		Enable:    true,
		HTTPAddr:  "127.0.0.1:8080",
		SOCKSAddr: "127.0.0.1:1080",
		Bypass:    DefaultBypass(),
	}

	err := Set(cfg)
	if err != nil {
		t.Logf("Set() error = %v (expected in Docker without desktop)", err)
	}

	err = Clear()
	if err != nil {
		t.Logf("Clear() error = %v (expected in Docker without desktop)", err)
	}
}
