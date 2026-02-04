package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyClientDefaults(t *testing.T) {
	cfg := &ClientConfig{}
	applyClientDefaults(cfg)

	if cfg.Transport.Preferred != "auto" {
		t.Errorf("Transport.Preferred = %q, want 'auto'", cfg.Transport.Preferred)
	}
	if cfg.Proxy.SOCKS5.Listen != "127.0.0.1:1080" {
		t.Errorf("SOCKS5 listen = %q", cfg.Proxy.SOCKS5.Listen)
	}
	if cfg.Proxy.HTTP.Listen != "127.0.0.1:8080" {
		t.Errorf("HTTP listen = %q", cfg.Proxy.HTTP.Listen)
	}
	if cfg.Routing.Default != "proxy" {
		t.Errorf("Routing.Default = %q", cfg.Routing.Default)
	}
	if cfg.Congestion.Mode != "adaptive" {
		t.Errorf("Congestion.Mode = %q", cfg.Congestion.Mode)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q", cfg.Log.Level)
	}
}

func TestAllowLAN(t *testing.T) {
	// Default: AllowLAN false -> 127.0.0.1
	cfg := &ClientConfig{}
	applyClientDefaults(cfg)
	if cfg.Proxy.SOCKS5.Listen != "127.0.0.1:1080" {
		t.Errorf("Without AllowLAN, SOCKS5 listen = %q, want '127.0.0.1:1080'", cfg.Proxy.SOCKS5.Listen)
	}
	if cfg.Proxy.HTTP.Listen != "127.0.0.1:8080" {
		t.Errorf("Without AllowLAN, HTTP listen = %q, want '127.0.0.1:8080'", cfg.Proxy.HTTP.Listen)
	}

	// AllowLAN true -> 0.0.0.0
	cfg2 := &ClientConfig{
		Proxy: ProxyConfig{AllowLAN: true},
	}
	applyClientDefaults(cfg2)
	if cfg2.Proxy.SOCKS5.Listen != "0.0.0.0:1080" {
		t.Errorf("With AllowLAN, SOCKS5 listen = %q, want '0.0.0.0:1080'", cfg2.Proxy.SOCKS5.Listen)
	}
	if cfg2.Proxy.HTTP.Listen != "0.0.0.0:8080" {
		t.Errorf("With AllowLAN, HTTP listen = %q, want '0.0.0.0:8080'", cfg2.Proxy.HTTP.Listen)
	}

	// Custom listen should not be overwritten
	cfg3 := &ClientConfig{
		Proxy: ProxyConfig{
			AllowLAN: true,
			SOCKS5:   SOCKS5Config{Listen: "192.168.1.1:1081"},
		},
	}
	applyClientDefaults(cfg3)
	if cfg3.Proxy.SOCKS5.Listen != "192.168.1.1:1081" {
		t.Errorf("Custom listen should be preserved, got %q", cfg3.Proxy.SOCKS5.Listen)
	}
}

func TestApplyServerDefaults(t *testing.T) {
	cfg := &ServerConfig{}
	applyServerDefaults(cfg)

	if cfg.Listen != ":443" {
		t.Errorf("Listen = %q, want ':443'", cfg.Listen)
	}
	if cfg.Cover.Mode != "default" {
		t.Errorf("Cover.Mode = %q", cfg.Cover.Mode)
	}
	if cfg.Mesh.CIDR != "10.7.0.0/24" {
		t.Errorf("Mesh.CIDR = %q, want '10.7.0.0/24'", cfg.Mesh.CIDR)
	}
}

func TestSNIAutoFill(t *testing.T) {
	cfg := &ClientConfig{
		Server: ServerEndpoint{Addr: "example.com:443"},
	}
	applyClientDefaults(cfg)
	if cfg.Server.SNI != "example.com" {
		t.Errorf("SNI = %q, want 'example.com'", cfg.Server.SNI)
	}

	// IP address should not fill SNI
	cfg2 := &ClientConfig{
		Server: ServerEndpoint{Addr: "1.2.3.4:443"},
	}
	applyClientDefaults(cfg2)
	if cfg2.Server.SNI != "" {
		t.Errorf("SNI should be empty for IP, got %q", cfg2.Server.SNI)
	}
}

func TestDeepCopy(t *testing.T) {
	cfg := &ClientConfig{
		Server: ServerEndpoint{Addr: "a.com:443"},
		Routing: RoutingConfig{
			Rules: []RouteRule{
				{Domains: "google.com", Action: "proxy", Process: []string{"chrome"}, IPCIDR: []string{"10.0.0.0/8"}},
			},
		},
		Proxy: ProxyConfig{
			TUN: TUNConfig{AppList: []string{"com.app.test"}},
		},
		Mesh: MeshConfig{Enabled: true},
	}

	cp := cfg.DeepCopy()

	// Modify original
	cfg.Server.Addr = "b.com:443"
	cfg.Routing.Rules[0].Domains = "changed.com"
	cfg.Routing.Rules[0].Process[0] = "firefox"
	cfg.Routing.Rules[0].IPCIDR[0] = "192.168.0.0/16"
	cfg.Proxy.TUN.AppList[0] = "com.app.changed"
	cfg.Mesh.Enabled = false

	// Copy should be unaffected
	if cp.Server.Addr != "a.com:443" {
		t.Errorf("deep copy addr mutated: %q", cp.Server.Addr)
	}
	if cp.Routing.Rules[0].Domains != "google.com" {
		t.Errorf("deep copy domain mutated: %q", cp.Routing.Rules[0].Domains)
	}
	if cp.Routing.Rules[0].Process[0] != "chrome" {
		t.Errorf("deep copy process mutated: %q", cp.Routing.Rules[0].Process[0])
	}
	if cp.Routing.Rules[0].IPCIDR[0] != "10.0.0.0/8" {
		t.Errorf("deep copy ipcidr mutated: %q", cp.Routing.Rules[0].IPCIDR[0])
	}
	if cp.Proxy.TUN.AppList[0] != "com.app.test" {
		t.Errorf("deep copy applist mutated: %q", cp.Proxy.TUN.AppList[0])
	}
	if !cp.Mesh.Enabled {
		t.Error("deep copy mesh.enabled mutated")
	}
}

func TestLoadClientConfig(t *testing.T) {
	yaml := `
server:
  addr: "example.com:443"
  password: "secret"
transport:
  h3:
    enabled: true
proxy:
  socks5:
    enabled: true
mesh:
  enabled: true
`
	dir := t.TempDir()
	path := filepath.Join(dir, "client.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg.Server.Addr != "example.com:443" {
		t.Errorf("Server.Addr = %q", cfg.Server.Addr)
	}
	if !cfg.Transport.H3.Enabled {
		t.Error("H3 should be enabled")
	}
	if !cfg.Mesh.Enabled {
		t.Error("Mesh should be enabled")
	}
	if cfg.Server.SNI != "example.com" {
		t.Errorf("SNI auto-fill failed: %q", cfg.Server.SNI)
	}
}

func TestLoadServerConfig(t *testing.T) {
	yaml := `
listen: ":8443"
mesh:
  enabled: true
  cidr: "10.8.0.0/24"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatalf("LoadServerConfig: %v", err)
	}
	if cfg.Listen != ":8443" {
		t.Errorf("Listen = %q", cfg.Listen)
	}
	if !cfg.Mesh.Enabled {
		t.Error("Mesh should be enabled")
	}
	if cfg.Mesh.CIDR != "10.8.0.0/24" {
		t.Errorf("Mesh.CIDR = %q", cfg.Mesh.CIDR)
	}
}

func TestLoadConfigInvalidPath(t *testing.T) {
	_, err := LoadClientConfig("/nonexistent/path.yaml")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	os.WriteFile(path, []byte("{{invalid yaml"), 0644)

	_, err := LoadClientConfig(path)
	if err == nil {
		t.Error("expected error for invalid yaml")
	}
}

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()
	if !cfg.Proxy.SOCKS5.Enabled {
		t.Error("SOCKS5 should be enabled by default")
	}
	if !cfg.Proxy.HTTP.Enabled {
		t.Error("HTTP should be enabled by default")
	}
	if cfg.Congestion.Mode != "adaptive" {
		t.Errorf("Congestion.Mode = %q", cfg.Congestion.Mode)
	}
}
