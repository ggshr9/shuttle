package test

import (
	"testing"

	"github.com/shuttleX/shuttle/transport/reality"
)

func TestRealityClientCreate(t *testing.T) {
	client := reality.NewClient(&reality.ClientConfig{
		ServerAddr: "127.0.0.1:443",
		ServerName: "www.apple.com",
		Password:   "test",
	})
	if client.Type() != "reality" {
		t.Errorf("expected type 'reality', got '%s'", client.Type())
	}
}

func TestRealityServerCreate(t *testing.T) {
	// Valid 32-byte hex key (all zeros for testing)
	srv, err := reality.NewServer(&reality.ServerConfig{
		ListenAddr: "127.0.0.1:0",
		PrivateKey: "0000000000000000000000000000000000000000000000000000000000000000",
		TargetSNI:  "www.apple.com",
		TargetAddr: "www.apple.com:443",
	}, nil)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	if srv.Type() != "reality" {
		t.Errorf("expected type 'reality', got '%s'", srv.Type())
	}
}

func TestRealityServerCreateMissingKey(t *testing.T) {
	_, err := reality.NewServer(&reality.ServerConfig{
		ListenAddr: "127.0.0.1:0",
		TargetSNI:  "www.apple.com",
		TargetAddr: "www.apple.com:443",
	}, nil)
	if err == nil {
		t.Fatal("expected error for missing private key, got nil")
	}
}

func TestRealityServerCreateInvalidKey(t *testing.T) {
	_, err := reality.NewServer(&reality.ServerConfig{
		ListenAddr: "127.0.0.1:0",
		PrivateKey: "not-valid-hex",
		TargetSNI:  "www.apple.com",
		TargetAddr: "www.apple.com:443",
	}, nil)
	if err == nil {
		t.Fatal("expected error for invalid private key hex, got nil")
	}
}
