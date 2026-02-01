package test

import (
	"testing"

	"github.com/shuttle-proxy/shuttle/transport/reality"
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
	srv := reality.NewServer(&reality.ServerConfig{
		ListenAddr: "127.0.0.1:0",
		TargetSNI:  "www.apple.com",
		TargetAddr: "www.apple.com:443",
	}, nil)
	if srv.Type() != "reality" {
		t.Errorf("expected type 'reality', got '%s'", srv.Type())
	}
}
