package engine

import (
	"encoding/json"
	"testing"

	"github.com/ggshr9/shuttle/config"
)

func TestAdaptLegacyConfig_SOCKS5(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.Proxy.SOCKS5.Enabled = true
	cfg.Proxy.SOCKS5.Listen = "127.0.0.1:1080"

	adaptLegacyConfig(cfg)

	if len(cfg.Inbounds) != 1 {
		t.Fatalf("expected 1 inbound, got %d", len(cfg.Inbounds))
	}
	ib := cfg.Inbounds[0]
	if ib.Tag != "socks5" {
		t.Errorf("tag = %q, want %q", ib.Tag, "socks5")
	}
	if ib.Type != "socks5" {
		t.Errorf("type = %q, want %q", ib.Type, "socks5")
	}
	var opts map[string]interface{}
	if err := json.Unmarshal(ib.Options, &opts); err != nil {
		t.Fatalf("unmarshal options: %v", err)
	}
	if opts["listen"] != "127.0.0.1:1080" {
		t.Errorf("listen = %v, want %q", opts["listen"], "127.0.0.1:1080")
	}
}

func TestAdaptLegacyConfig_HTTP(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.Proxy.HTTP.Enabled = true
	cfg.Proxy.HTTP.Listen = "127.0.0.1:8080"

	adaptLegacyConfig(cfg)

	if len(cfg.Inbounds) != 1 {
		t.Fatalf("expected 1 inbound, got %d", len(cfg.Inbounds))
	}
	ib := cfg.Inbounds[0]
	if ib.Tag != "http" {
		t.Errorf("tag = %q, want %q", ib.Tag, "http")
	}
	if ib.Type != "http" {
		t.Errorf("type = %q, want %q", ib.Type, "http")
	}
	var opts map[string]interface{}
	if err := json.Unmarshal(ib.Options, &opts); err != nil {
		t.Fatalf("unmarshal options: %v", err)
	}
	if opts["listen"] != "127.0.0.1:8080" {
		t.Errorf("listen = %v, want %q", opts["listen"], "127.0.0.1:8080")
	}
}

func TestAdaptLegacyConfig_TUN(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.Proxy.TUN.Enabled = true
	cfg.Proxy.TUN.DeviceName = "utun7"
	cfg.Proxy.TUN.CIDR = "198.18.0.0/16"
	cfg.Proxy.TUN.MTU = 1400
	cfg.Proxy.TUN.AutoRoute = true
	cfg.Proxy.TUN.TunFD = 42

	adaptLegacyConfig(cfg)

	if len(cfg.Inbounds) != 1 {
		t.Fatalf("expected 1 inbound, got %d", len(cfg.Inbounds))
	}
	ib := cfg.Inbounds[0]
	if ib.Tag != "tun" {
		t.Errorf("tag = %q, want %q", ib.Tag, "tun")
	}
	if ib.Type != "tun" {
		t.Errorf("type = %q, want %q", ib.Type, "tun")
	}
	var opts map[string]interface{}
	if err := json.Unmarshal(ib.Options, &opts); err != nil {
		t.Fatalf("unmarshal options: %v", err)
	}
	if opts["device_name"] != "utun7" {
		t.Errorf("device_name = %v, want %q", opts["device_name"], "utun7")
	}
	if opts["cidr"] != "198.18.0.0/16" {
		t.Errorf("cidr = %v, want %q", opts["cidr"], "198.18.0.0/16")
	}
	// JSON numbers are float64
	if opts["mtu"] != float64(1400) {
		t.Errorf("mtu = %v, want %v", opts["mtu"], 1400)
	}
	if opts["auto_route"] != true {
		t.Errorf("auto_route = %v, want true", opts["auto_route"])
	}
	if opts["tun_fd"] != float64(42) {
		t.Errorf("tun_fd = %v, want %v", opts["tun_fd"], 42)
	}
}

func TestAdaptLegacyConfig_SkipsIfInboundsExist(t *testing.T) {
	cfg := &config.ClientConfig{
		Inbounds: []config.InboundConfig{
			{Tag: "custom", Type: "socks5"},
		},
	}
	cfg.Proxy.SOCKS5.Enabled = true
	cfg.Proxy.SOCKS5.Listen = "127.0.0.1:1080"

	adaptLegacyConfig(cfg)

	if len(cfg.Inbounds) != 1 {
		t.Fatalf("expected 1 inbound (unchanged), got %d", len(cfg.Inbounds))
	}
	if cfg.Inbounds[0].Tag != "custom" {
		t.Errorf("existing inbound was overwritten: tag = %q", cfg.Inbounds[0].Tag)
	}
}

func TestAdaptLegacyConfig_AllThree(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.Proxy.SOCKS5.Enabled = true
	cfg.Proxy.SOCKS5.Listen = "127.0.0.1:1080"
	cfg.Proxy.HTTP.Enabled = true
	cfg.Proxy.HTTP.Listen = "127.0.0.1:8080"
	cfg.Proxy.TUN.Enabled = true
	cfg.Proxy.TUN.DeviceName = "utun7"
	cfg.Proxy.TUN.CIDR = "198.18.0.0/16"
	cfg.Proxy.TUN.MTU = 1500
	cfg.Proxy.TUN.AutoRoute = true

	adaptLegacyConfig(cfg)

	if len(cfg.Inbounds) != 3 {
		t.Fatalf("expected 3 inbounds, got %d", len(cfg.Inbounds))
	}

	tags := map[string]bool{}
	for _, ib := range cfg.Inbounds {
		tags[ib.Tag] = true
	}
	for _, expected := range []string{"socks5", "http", "tun"} {
		if !tags[expected] {
			t.Errorf("missing inbound tag %q", expected)
		}
	}
}
