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
	os.WriteFile(path, []byte(yaml), 0600)

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
	os.WriteFile(path, []byte(yaml), 0600)

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
	os.WriteFile(path, []byte("{{invalid yaml"), 0600)

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

func TestServerEndpoint_TypeAndOptions(t *testing.T) {
	ep := ServerEndpoint{
		Addr:     "1.2.3.4:443",
		Name:     "my-ss-server",
		Password: "secret",
		Type:     "shadowsocks",
		Options: map[string]any{
			"method": "aes-256-gcm",
		},
	}
	if ep.Type != "shadowsocks" {
		t.Errorf("Type = %q, want %q", ep.Type, "shadowsocks")
	}
	if ep.Options["method"] != "aes-256-gcm" {
		t.Errorf("Options[method] = %v, want %q", ep.Options["method"], "aes-256-gcm")
	}
}

func TestServerEndpoint_DeepCopyOptions(t *testing.T) {
	cfg := &ClientConfig{
		Server: ServerEndpoint{
			Addr: "1.2.3.4:443",
			Type: "shadowsocks",
			Options: map[string]any{
				"method": "aes-256-gcm",
			},
		},
		Servers: []ServerEndpoint{
			{
				Addr: "5.6.7.8:443",
				Type: "trojan",
				Options: map[string]any{
					"sni": "example.com",
				},
			},
		},
	}

	cp := cfg.DeepCopy()

	// Mutate originals
	cfg.Server.Options["method"] = "chacha20-ietf-poly1305"
	cfg.Servers[0].Options["sni"] = "changed.com"

	// Copy should be unaffected
	if cp.Server.Options["method"] != "aes-256-gcm" {
		t.Errorf("DeepCopy Server.Options mutated: got %v", cp.Server.Options["method"])
	}
	if cp.Servers[0].Options["sni"] != "example.com" {
		t.Errorf("DeepCopy Servers[0].Options mutated: got %v", cp.Servers[0].Options["sni"])
	}
}

// validClientConfig returns a DefaultClientConfig with required fields filled
// in so that it passes all validation checks. DefaultClientConfig() is a GUI
// template and intentionally omits server-specific values like public keys.
func validClientConfig() *ClientConfig {
	cfg := DefaultClientConfig()
	cfg.Transport.Reality.PublicKey = "testkey"
	cfg.Transport.CDN.Domain = "cdn.example.com"
	return cfg
}

func TestClientConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*ClientConfig)
		wantErr bool
	}{
		{"valid default", func(c *ClientConfig) {}, false},
		{"invalid transport", func(c *ClientConfig) { c.Transport.Preferred = "invalid" }, true},
		{"invalid routing default", func(c *ClientConfig) { c.Routing.Default = "invalid" }, true},
		{"invalid socks5 listen", func(c *ClientConfig) { c.Proxy.SOCKS5.Listen = "not-valid" }, true},
		{"invalid http listen", func(c *ClientConfig) { c.Proxy.HTTP.Listen = "not-valid" }, true},
		{"invalid congestion mode", func(c *ClientConfig) { c.Congestion.Mode = "invalid" }, true},
		{"valid congestion bbr", func(c *ClientConfig) { c.Congestion.Mode = "bbr" }, false},
		{"valid congestion brutal", func(c *ClientConfig) { c.Congestion.Mode = "brutal" }, false},
		{"invalid obfs max_delay", func(c *ClientConfig) { c.Obfs.MaxDelay = "not-a-duration" }, true},
		{"invalid split route CIDR", func(c *ClientConfig) {
			c.Mesh.SplitRoutes = []SplitRoute{{CIDR: "invalid", Action: "mesh"}}
		}, true},
		{"invalid split route action", func(c *ClientConfig) {
			c.Mesh.SplitRoutes = []SplitRoute{{CIDR: "10.0.0.0/24", Action: "invalid"}}
		}, true},

		// server.addr
		{"valid server addr", func(c *ClientConfig) { c.Server.Addr = "example.com:443" }, false},
		{"invalid server addr", func(c *ClientConfig) { c.Server.Addr = "no-port" }, true},

		// servers[]
		{"valid servers list", func(c *ClientConfig) {
			c.Servers = []ServerEndpoint{{Addr: "a.com:443"}, {Addr: "b.com:8443"}}
		}, false},
		{"invalid servers entry", func(c *ClientConfig) {
			c.Servers = []ServerEndpoint{{Addr: "bad-addr"}}
		}, true},

		// subscriptions[].url
		{"valid subscription url", func(c *ClientConfig) {
			c.Subscriptions = []SubscriptionConfig{{URL: "https://example.com/sub"}}
		}, false},
		{"invalid subscription url scheme", func(c *ClientConfig) {
			c.Subscriptions = []SubscriptionConfig{{URL: "ftp://example.com/sub"}}
		}, true},

		// routing.dns.domestic
		{"valid dns domestic IP", func(c *ClientConfig) { c.Routing.DNS.Domestic = "8.8.8.8" }, false},
		{"valid dns domestic host:port", func(c *ClientConfig) { c.Routing.DNS.Domestic = "dns.example.com:53" }, false},
		{"valid dns domestic DoH URL", func(c *ClientConfig) { c.Routing.DNS.Domestic = "https://dns.alidns.com/dns-query" }, false},
		{"invalid dns domestic", func(c *ClientConfig) { c.Routing.DNS.Domestic = "not-an-ip" }, true},
		{"invalid dns domestic URL scheme", func(c *ClientConfig) { c.Routing.DNS.Domestic = "ftp://bad.com" }, true},

		// routing.dns.remote.server
		{"valid dns remote server", func(c *ClientConfig) { c.Routing.DNS.Remote.Server = "https://1.1.1.1/dns-query" }, false},
		{"invalid dns remote server http", func(c *ClientConfig) { c.Routing.DNS.Remote.Server = "http://1.1.1.1/dns-query" }, true},
		{"invalid dns remote server no scheme", func(c *ClientConfig) { c.Routing.DNS.Remote.Server = "1.1.1.1" }, true},

		// routing.dns.remote.via
		{"valid dns remote via proxy", func(c *ClientConfig) { c.Routing.DNS.Remote.Via = "proxy" }, false},
		{"valid dns remote via direct", func(c *ClientConfig) { c.Routing.DNS.Remote.Via = "direct" }, false},
		{"invalid dns remote via", func(c *ClientConfig) { c.Routing.DNS.Remote.Via = "invalid" }, true},

		// transport.cdn
		{"cdn enabled missing domain", func(c *ClientConfig) {
			c.Transport.CDN.Enabled = true
			c.Transport.CDN.Domain = ""
		}, true},
		{"cdn enabled invalid mode", func(c *ClientConfig) {
			c.Transport.CDN.Enabled = true
			c.Transport.CDN.Domain = "cdn.example.com"
			c.Transport.CDN.Mode = "invalid"
		}, true},
		{"cdn enabled valid h2 mode", func(c *ClientConfig) {
			c.Transport.CDN.Enabled = true
			c.Transport.CDN.Domain = "cdn.example.com"
			c.Transport.CDN.Mode = "h2"
		}, false},
		{"cdn enabled valid grpc mode", func(c *ClientConfig) {
			c.Transport.CDN.Enabled = true
			c.Transport.CDN.Domain = "cdn.example.com"
			c.Transport.CDN.Mode = "grpc"
		}, false},

		// transport.reality
		{"reality enabled missing public key", func(c *ClientConfig) {
			c.Transport.Reality.Enabled = true
			c.Transport.Reality.PublicKey = ""
		}, true},

		// transport.webrtc
		{"webrtc enabled missing signal url", func(c *ClientConfig) {
			c.Transport.WebRTC.Enabled = true
			c.Transport.WebRTC.SignalURL = ""
		}, true},
		{"webrtc enabled invalid signal url", func(c *ClientConfig) {
			c.Transport.WebRTC.Enabled = true
			c.Transport.WebRTC.SignalURL = "not-a-url"
		}, true},
		{"webrtc enabled valid signal url", func(c *ClientConfig) {
			c.Transport.WebRTC.Enabled = true
			c.Transport.WebRTC.SignalURL = "https://signal.example.com"
		}, false},

		// transport duration fields
		{"valid pool_idle_ttl", func(c *ClientConfig) { c.Transport.PoolIdleTTL = "60s" }, false},
		{"invalid pool_idle_ttl", func(c *ClientConfig) { c.Transport.PoolIdleTTL = "not-a-duration" }, true},
		{"empty pool_idle_ttl", func(c *ClientConfig) { c.Transport.PoolIdleTTL = "" }, false},
		{"valid keepalive_interval", func(c *ClientConfig) { c.Transport.KeepaliveInterval = "15s" }, false},
		{"invalid keepalive_interval", func(c *ClientConfig) { c.Transport.KeepaliveInterval = "bad" }, true},
		{"valid keepalive_timeout", func(c *ClientConfig) { c.Transport.KeepaliveTimeout = "5s" }, false},
		{"invalid keepalive_timeout", func(c *ClientConfig) { c.Transport.KeepaliveTimeout = "bad" }, true},

		// routing.geodata.update_interval
		{"valid geodata update_interval", func(c *ClientConfig) { c.Routing.GeoData.UpdateInterval = "24h" }, false},
		{"invalid geodata update_interval", func(c *ClientConfig) { c.Routing.GeoData.UpdateInterval = "not-a-duration" }, true},
		{"empty geodata update_interval", func(c *ClientConfig) { c.Routing.GeoData.UpdateInterval = "" }, false},

		// obfs.min_delay
		{"valid obfs min_delay", func(c *ClientConfig) { c.Obfs.MinDelay = "10ms" }, false},
		{"invalid obfs min_delay", func(c *ClientConfig) { c.Obfs.MinDelay = "not-a-duration" }, true},
		{"empty obfs min_delay", func(c *ClientConfig) { c.Obfs.MinDelay = "" }, false},

		// log.level
		{"valid log level debug", func(c *ClientConfig) { c.Log.Level = "debug" }, false},
		{"invalid log level", func(c *ClientConfig) { c.Log.Level = "trace" }, true},

		// log.format
		{"valid log format json", func(c *ClientConfig) { c.Log.Format = "json" }, false},
		{"invalid log format", func(c *ClientConfig) { c.Log.Format = "xml" }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validClientConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServerConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*ServerConfig)
		wantErr bool
	}{
		{"valid default", func(c *ServerConfig) {}, false},
		{"invalid listen", func(c *ServerConfig) { c.Listen = "not-valid" }, true},
		{"invalid cover mode", func(c *ServerConfig) { c.Cover.Mode = "invalid" }, true},
		{"cluster missing node name", func(c *ServerConfig) {
			c.Cluster.Enabled = true
			c.Cluster.Secret = "s"
		}, true},
		{"cluster missing secret", func(c *ServerConfig) {
			c.Cluster.Enabled = true
			c.Cluster.NodeName = "n"
		}, true},
		{"cluster invalid peer addr", func(c *ServerConfig) {
			c.Cluster.Enabled = true
			c.Cluster.NodeName = "n"
			c.Cluster.Secret = "s"
			c.Cluster.Peers = []ClusterPeer{{Name: "p", Addr: "invalid"}}
		}, true},
		{"cluster invalid interval", func(c *ServerConfig) {
			c.Cluster.Enabled = true
			c.Cluster.NodeName = "n"
			c.Cluster.Secret = "s"
			c.Cluster.Interval = "invalid"
		}, true},

		// cover.mode == "reverse" requires reverse_url
		{"reverse cover missing url", func(c *ServerConfig) {
			c.Cover.Mode = "reverse"
		}, true},
		{"reverse cover invalid url", func(c *ServerConfig) {
			c.Cover.Mode = "reverse"
			c.Cover.ReverseURL = "not-a-url"
		}, true},
		{"reverse cover valid url", func(c *ServerConfig) {
			c.Cover.Mode = "reverse"
			c.Cover.ReverseURL = "https://example.com"
		}, false},

		// cover.mode == "static" requires static_dir
		{"static cover missing dir", func(c *ServerConfig) {
			c.Cover.Mode = "static"
		}, true},
		{"static cover valid dir", func(c *ServerConfig) {
			c.Cover.Mode = "static"
			c.Cover.StaticDir = "/var/www"
		}, false},

		// mesh.cidr when enabled
		{"mesh enabled invalid cidr", func(c *ServerConfig) {
			c.Mesh.Enabled = true
			c.Mesh.CIDR = "not-a-cidr"
		}, true},
		{"mesh enabled valid cidr", func(c *ServerConfig) {
			c.Mesh.Enabled = true
			c.Mesh.CIDR = "10.7.0.0/24"
		}, false},

		// admin.listen when enabled
		{"admin enabled invalid listen", func(c *ServerConfig) {
			c.Admin.Enabled = true
			c.Admin.Listen = "bad"
		}, true},
		{"admin enabled valid listen", func(c *ServerConfig) {
			c.Admin.Enabled = true
			c.Admin.Listen = "127.0.0.1:9090"
		}, false},

		// debug.pprof_listen
		{"pprof invalid listen", func(c *ServerConfig) {
			c.Debug.PprofListen = "bad"
		}, true},
		{"pprof valid listen", func(c *ServerConfig) {
			c.Debug.PprofListen = "127.0.0.1:6060"
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultServerConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSaveServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()
	dir := t.TempDir()
	path := filepath.Join(dir, "out.yaml")

	err := SaveServerConfig(path, cfg)
	if err != nil {
		t.Fatalf("SaveServerConfig: %v", err)
	}

	loaded, err := LoadServerConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.Listen != cfg.Listen {
		t.Fatalf("Listen mismatch after round-trip")
	}
}

func TestValidate_DurationBounds(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(c *ClientConfig)
		wantErr bool
	}{
		// short group
		{"keepalive-timeout-valid", func(c *ClientConfig) { c.Transport.KeepaliveTimeout = "5s" }, false},
		{"keepalive-timeout-too-small", func(c *ClientConfig) { c.Transport.KeepaliveTimeout = "1ns" }, true},
		{"keepalive-timeout-too-large", func(c *ClientConfig) { c.Transport.KeepaliveTimeout = "99h" }, true},
		{"probe-timeout-valid", func(c *ClientConfig) { c.Transport.ProbeTimeout = "5s" }, false},
		{"probe-timeout-too-small", func(c *ClientConfig) { c.Transport.ProbeTimeout = "1ms" }, true},
		// medium group
		{"keepalive-interval-valid", func(c *ClientConfig) { c.Transport.KeepaliveInterval = "15s" }, false},
		{"keepalive-interval-too-small", func(c *ClientConfig) { c.Transport.KeepaliveInterval = "1ns" }, true},
		{"keepalive-interval-too-large", func(c *ClientConfig) { c.Transport.KeepaliveInterval = "99h" }, true},
		{"retry-initial-backoff-valid", func(c *ClientConfig) { c.Retry.InitialBackoff = "1s" }, false},
		{"retry-initial-backoff-too-small", func(c *ClientConfig) { c.Retry.InitialBackoff = "1ms" }, true},
		{"retry-max-backoff-too-large", func(c *ClientConfig) { c.Retry.MaxBackoff = "1h" }, true},
		{"hole-punch-timeout-valid", func(c *ClientConfig) { c.Mesh.P2P.HolePunchTimeout = "10s" }, false},
		{"hole-punch-timeout-too-large", func(c *ClientConfig) { c.Mesh.P2P.HolePunchTimeout = "5m" }, true},
		// long group
		{"pool-idle-ttl-valid", func(c *ClientConfig) { c.Transport.PoolIdleTTL = "60s" }, false},
		{"pool-idle-ttl-too-small", func(c *ClientConfig) { c.Transport.PoolIdleTTL = "1s" }, true},
		{"pool-idle-ttl-too-large", func(c *ClientConfig) { c.Transport.PoolIdleTTL = "200h" }, true},
		{"geodata-update-valid", func(c *ClientConfig) { c.Routing.GeoData.UpdateInterval = "24h" }, false},
		{"geodata-update-too-small", func(c *ClientConfig) { c.Routing.GeoData.UpdateInterval = "100ms" }, true},
		{"proxy-provider-interval-valid", func(c *ClientConfig) {
			c.ProxyProviders = []ProxyProviderConfig{{Name: "x", URL: "http://a", Interval: "1h"}}
		}, false},
		{"proxy-provider-interval-too-small", func(c *ClientConfig) {
			c.ProxyProviders = []ProxyProviderConfig{{Name: "x", URL: "http://a", Interval: "1s"}}
		}, true},
		{"rule-provider-interval-too-large", func(c *ClientConfig) {
			c.RuleProviders = []RuleProviderConfig{{Name: "x", URL: "http://a", Interval: "9999h"}}
		}, true},
		// obfs group (0 allowed)
		{"obfs-min-delay-zero-valid", func(c *ClientConfig) { c.Obfs.MinDelay = "0s" }, false},
		{"obfs-min-delay-valid", func(c *ClientConfig) { c.Obfs.MinDelay = "100ms" }, false},
		{"obfs-min-delay-too-large", func(c *ClientConfig) { c.Obfs.MinDelay = "10s" }, true},
		{"obfs-max-delay-too-large", func(c *ClientConfig) { c.Obfs.MaxDelay = "1m" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := DefaultClientConfig()
			c.Transport.Reality.Enabled = false
			c.Transport.WebRTC.Enabled = false
			tc.setup(c)
			err := c.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate: err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestValidate_ServerDurationBounds(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(c *ServerConfig)
		wantErr bool
	}{
		{"drain-timeout-valid", func(c *ServerConfig) { c.DrainTimeout = "30s" }, false},
		{"drain-timeout-too-small", func(c *ServerConfig) { c.DrainTimeout = "1ns" }, true},
		{"drain-timeout-too-large", func(c *ServerConfig) { c.DrainTimeout = "5m" }, true},
		{"cluster-interval-valid", func(c *ServerConfig) {
			c.Cluster.Enabled = true
			c.Cluster.NodeName = "n"
			c.Cluster.Secret = "s"
			c.Cluster.Interval = "30s"
		}, false},
		{"cluster-interval-too-small", func(c *ServerConfig) {
			c.Cluster.Enabled = true
			c.Cluster.NodeName = "n"
			c.Cluster.Secret = "s"
			c.Cluster.Interval = "1ms"
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := DefaultServerConfig()
			tc.setup(c)
			err := c.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate: err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestConfigVersionConstants(t *testing.T) {
	if CurrentClientConfigVersion != 1 {
		t.Fatalf("CurrentClientConfigVersion = %d", CurrentClientConfigVersion)
	}
	if CurrentServerConfigVersion != 1 {
		t.Fatalf("CurrentServerConfigVersion = %d", CurrentServerConfigVersion)
	}
}
