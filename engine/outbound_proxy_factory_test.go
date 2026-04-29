package engine

import (
	"encoding/json"
	"testing"

	"github.com/ggshr9/shuttle/config"
)

func TestCreateCustomProxyOutbound(t *testing.T) {
	cfg := config.DefaultClientConfig()
	cfg.Server.Addr = "default.example.com:443"

	eng := &Engine{}

	opts, _ := json.Marshal(ProxyOutboundConfig{Server: "us.example.com:443"})
	ob, err := eng.createCustomProxyOutbound("us-server", opts, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ob.Tag() != "us-server" {
		t.Errorf("Tag() = %q, want %q", ob.Tag(), "us-server")
	}
	if ob.Type() != "proxy" {
		t.Errorf("Type() = %q, want %q", ob.Type(), "proxy")
	}
	if ob.serverAddr != "us.example.com:443" {
		t.Errorf("serverAddr = %q, want %q", ob.serverAddr, "us.example.com:443")
	}

	// Original config must not be mutated.
	if cfg.Server.Addr != "default.example.com:443" {
		t.Errorf("base config mutated: Server.Addr = %q", cfg.Server.Addr)
	}
}

func TestCreateCustomProxyOutbound_MissingServer(t *testing.T) {
	eng := &Engine{}
	cfg := config.DefaultClientConfig()

	opts, _ := json.Marshal(ProxyOutboundConfig{Server: ""})
	_, err := eng.createCustomProxyOutbound("bad", opts, cfg)
	if err == nil {
		t.Fatal("expected error for empty server address")
	}
}

func TestCreateCustomProxyOutbound_InvalidJSON(t *testing.T) {
	eng := &Engine{}
	cfg := config.DefaultClientConfig()

	_, err := eng.createCustomProxyOutbound("bad", json.RawMessage(`{invalid`), cfg)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestNewProxyOutboundWithTag(t *testing.T) {
	cfg := config.DefaultClientConfig()
	cfg.Server.Addr = "jp.example.com:443"

	eng := &Engine{}
	ob := newProxyOutboundWithTag("jp-server", eng, cfg)

	if ob.Tag() != "jp-server" {
		t.Errorf("Tag() = %q, want %q", ob.Tag(), "jp-server")
	}
	if ob.serverAddr != "jp.example.com:443" {
		t.Errorf("serverAddr = %q, want %q", ob.serverAddr, "jp.example.com:443")
	}
}

func TestNewProxyOutbound_DefaultTag(t *testing.T) {
	cfg := config.DefaultClientConfig()
	cfg.Server.Addr = "server.example.com:443"

	eng := &Engine{}
	ob := newProxyOutbound(eng, cfg)

	if ob.Tag() != "proxy" {
		t.Errorf("Tag() = %q, want %q", ob.Tag(), "proxy")
	}
}

func TestMultipleCustomProxyOutbounds(t *testing.T) {
	cfg := config.DefaultClientConfig()
	cfg.Server.Addr = "default.example.com:443"
	eng := &Engine{}

	servers := map[string]string{
		"us-server": "us.example.com:443",
		"jp-server": "jp.example.com:443",
		"eu-server": "eu.example.com:443",
	}

	outbounds := make(map[string]*ProxyOutbound)
	for tag, addr := range servers {
		opts, _ := json.Marshal(ProxyOutboundConfig{Server: addr})
		ob, err := eng.createCustomProxyOutbound(tag, opts, cfg)
		if err != nil {
			t.Fatalf("failed to create %q: %v", tag, err)
		}
		outbounds[tag] = ob
	}

	// Verify each outbound has independent tag and server address.
	for tag, addr := range servers {
		ob := outbounds[tag]
		if ob.Tag() != tag {
			t.Errorf("outbound %q: Tag() = %q", tag, ob.Tag())
		}
		if ob.serverAddr != addr {
			t.Errorf("outbound %q: serverAddr = %q, want %q", tag, ob.serverAddr, addr)
		}
	}
}
