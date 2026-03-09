package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBootstrap(t *testing.T) {
	dir := t.TempDir()

	result, err := Bootstrap(&InitOptions{
		ConfigDir:  dir,
		Listen:     ":8443",
		Domain:     "test.example.com",
		Password:   "testpass123",
		Transports: []string{"h3", "reality"},
		Force:      false,
	})
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	// Check result fields
	if result.ConfigPath != filepath.Join(dir, "server.yaml") {
		t.Errorf("ConfigPath = %q, want %q", result.ConfigPath, filepath.Join(dir, "server.yaml"))
	}
	if result.Password != "testpass123" {
		t.Errorf("Password = %q, want testpass123", result.Password)
	}
	if result.PublicKey == "" {
		t.Error("PublicKey is empty")
	}
	if !strings.HasPrefix(result.ShareURI, "shuttle://") {
		t.Errorf("ShareURI = %q, doesn't start with shuttle://", result.ShareURI)
	}
	if result.AdminToken == "" {
		t.Error("AdminToken is empty")
	}
	if len(result.AdminToken) != 64 { // 32 bytes hex
		t.Errorf("AdminToken length = %d, want 64", len(result.AdminToken))
	}

	// Check files were created
	for _, name := range []string{"server.yaml", "cert.pem", "key.pem"} {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("file %s not created: %v", name, err)
			continue
		}
		// Check permissions (0600)
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("file %s permissions = %o, want 0600", name, perm)
		}
	}

	// Load and validate config
	cfg, err := LoadServerConfig(filepath.Join(dir, "server.yaml"))
	if err != nil {
		t.Fatalf("LoadServerConfig() error = %v", err)
	}
	if cfg.Listen != ":8443" {
		t.Errorf("Listen = %q, want :8443", cfg.Listen)
	}
	if cfg.Auth.Password != "testpass123" {
		t.Errorf("Password = %q, want testpass123", cfg.Auth.Password)
	}
	if !cfg.Transport.H3.Enabled {
		t.Error("H3 should be enabled")
	}
	if !cfg.Transport.Reality.Enabled {
		t.Error("Reality should be enabled")
	}
	if !cfg.Admin.Enabled {
		t.Error("Admin should be enabled")
	}
}

func TestBootstrapAutoPassword(t *testing.T) {
	dir := t.TempDir()

	result, err := Bootstrap(&InitOptions{
		ConfigDir: dir,
		Domain:    "test.example.com",
	})
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	// Auto-generated password should be 32 hex chars (16 bytes)
	if len(result.Password) != 32 {
		t.Errorf("auto password length = %d, want 32", len(result.Password))
	}
}

func TestBootstrapNoOverwrite(t *testing.T) {
	dir := t.TempDir()

	// First init
	_, err := Bootstrap(&InitOptions{
		ConfigDir: dir,
		Domain:    "test.example.com",
	})
	if err != nil {
		t.Fatalf("first Bootstrap() error = %v", err)
	}

	// Second init without force should fail
	_, err = Bootstrap(&InitOptions{
		ConfigDir: dir,
		Domain:    "test.example.com",
	})
	if err == nil {
		t.Fatal("second Bootstrap() should fail without --force")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want 'already exists'", err.Error())
	}
}

func TestBootstrapForce(t *testing.T) {
	dir := t.TempDir()

	// First init
	_, err := Bootstrap(&InitOptions{
		ConfigDir: dir,
		Domain:    "test.example.com",
		Password:  "first",
	})
	if err != nil {
		t.Fatalf("first Bootstrap() error = %v", err)
	}

	// Second init with force should succeed
	result, err := Bootstrap(&InitOptions{
		ConfigDir: dir,
		Domain:    "test.example.com",
		Password:  "second",
		Force:     true,
	})
	if err != nil {
		t.Fatalf("forced Bootstrap() error = %v", err)
	}
	if result.Password != "second" {
		t.Errorf("Password = %q, want second", result.Password)
	}
}

func TestBootstrapH3Only(t *testing.T) {
	dir := t.TempDir()

	result, err := Bootstrap(&InitOptions{
		ConfigDir:  dir,
		Domain:     "test.example.com",
		Transports: []string{"h3"},
	})
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	cfg, err := LoadServerConfig(result.ConfigPath)
	if err != nil {
		t.Fatalf("LoadServerConfig() error = %v", err)
	}
	if !cfg.Transport.H3.Enabled {
		t.Error("H3 should be enabled")
	}
	if cfg.Transport.Reality.Enabled {
		t.Error("Reality should be disabled")
	}
}

func TestFindDefaultConfig(t *testing.T) {
	// With no config files, should return empty
	result := FindDefaultConfig()
	// Can't assert empty since /etc/shuttle/server.yaml might exist on this machine
	// Just verify it doesn't panic
	_ = result
}

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()

	if cfg.Listen != ":443" {
		t.Errorf("Listen = %q, want :443", cfg.Listen)
	}
	if !cfg.Transport.H3.Enabled {
		t.Error("H3 should be enabled")
	}
	if !cfg.Transport.Reality.Enabled {
		t.Error("Reality should be enabled")
	}
	if cfg.Transport.Reality.TargetSNI != "www.microsoft.com" {
		t.Errorf("TargetSNI = %q, want www.microsoft.com", cfg.Transport.Reality.TargetSNI)
	}
}
