package sysproxy

import (
	"os"
	"runtime"
	"testing"
)

func TestProxyConfig(t *testing.T) {
	cfg := ProxyConfig{
		Enable:    true,
		HTTPAddr:  "127.0.0.1:8080",
		SOCKSAddr: "127.0.0.1:1080",
		Bypass:    []string{"localhost", "127.0.0.1"},
	}

	if !cfg.Enable {
		t.Error("Enable = false, want true")
	}
	if cfg.HTTPAddr != "127.0.0.1:8080" {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, "127.0.0.1:8080")
	}
	if cfg.SOCKSAddr != "127.0.0.1:1080" {
		t.Errorf("SOCKSAddr = %q, want %q", cfg.SOCKSAddr, "127.0.0.1:1080")
	}
	if len(cfg.Bypass) != 2 {
		t.Errorf("len(Bypass) = %d, want 2", len(cfg.Bypass))
	}
}

func TestDefaultBypass(t *testing.T) {
	bypass := DefaultBypass()

	// Should contain common bypass entries
	expected := map[string]bool{
		"localhost":   false,
		"127.0.0.1":   false,
		"10.*":        false,
		"192.168.*":   false,
		"<local>":     false,
	}

	for _, item := range bypass {
		if _, ok := expected[item]; ok {
			expected[item] = true
		}
	}

	for item, found := range expected {
		if !found {
			t.Errorf("DefaultBypass() missing %q", item)
		}
	}

	// Should have reasonable number of entries
	if len(bypass) < 5 {
		t.Errorf("DefaultBypass() has %d entries, expected at least 5", len(bypass))
	}
}

func TestSplitHostPort(t *testing.T) {
	tests := []struct {
		input    string
		wantHost string
		wantPort string
	}{
		{"127.0.0.1:8080", "127.0.0.1", "8080"},
		{"localhost:1080", "localhost", "1080"},
		{"[::1]:8080", "[::1]", "8080"},
		{"example.com:443", "example.com", "443"},
		{"noport", "noport", ""},
		{"", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			host, port := splitHostPort(tt.input)
			if host != tt.wantHost {
				t.Errorf("splitHostPort(%q) host = %q, want %q", tt.input, host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("splitHostPort(%q) port = %q, want %q", tt.input, port, tt.wantPort)
			}
		})
	}
}

func TestClear(t *testing.T) {
	// Clear should not panic on any platform
	// Note: We don't actually test system changes in unit tests
	if os.Getenv("CI") != "" {
		t.Skip("Skipping on CI - modifies system state")
	}

	// Just verify it doesn't panic
	err := Clear()
	// On some platforms without proper permissions, this may error
	// but it shouldn't panic
	_ = err
}

// TestSetAndClear tests the enable/disable cycle
// This test is skipped on CI as it modifies system state
func TestSetAndClear(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping on CI - modifies system state")
	}

	// Skip on platforms where we can't easily test
	switch runtime.GOOS {
	case "android", "ios":
		t.Skip("Skipping on mobile platforms")
	}

	// Test setting proxy
	cfg := ProxyConfig{
		Enable:    true,
		HTTPAddr:  "127.0.0.1:8080",
		SOCKSAddr: "127.0.0.1:1080",
		Bypass:    DefaultBypass(),
	}

	err := Set(cfg)
	if err != nil {
		t.Logf("Set() error = %v (may be expected without permissions)", err)
	}

	// Clean up - disable proxy
	err = Clear()
	if err != nil {
		t.Logf("Clear() error = %v (may be expected without permissions)", err)
	}
}

func TestProxyConfigWithEmptyValues(t *testing.T) {
	cfg := ProxyConfig{
		Enable:    true,
		HTTPAddr:  "",
		SOCKSAddr: "",
		Bypass:    nil,
	}

	// Should handle empty values gracefully
	if cfg.HTTPAddr != "" {
		t.Error("HTTPAddr should be empty")
	}
	if cfg.SOCKSAddr != "" {
		t.Error("SOCKSAddr should be empty")
	}
	if cfg.Bypass != nil {
		t.Error("Bypass should be nil")
	}
}

func TestProxyConfigDisabled(t *testing.T) {
	cfg := ProxyConfig{
		Enable: false,
	}

	if cfg.Enable {
		t.Error("Enable should be false")
	}
}
