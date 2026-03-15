//go:build sandbox

package test

import (
	"context"
	"testing"

	"github.com/shuttle-proxy/shuttle/transport/h3"
)

func TestH3ClientCreate(t *testing.T) {
	client := h3.NewClient(&h3.ClientConfig{
		ServerAddr: "127.0.0.1:443",
		ServerName: "test.example.com",
		Password:   "test-password",
	})
	if client.Type() != "h3" {
		t.Errorf("expected type 'h3', got '%s'", client.Type())
	}
	client.Close()
}

func TestH3Fingerprint(t *testing.T) {
	fp := h3.DefaultFingerprint()
	if fp.Browser != "chrome" {
		t.Errorf("expected browser 'chrome', got '%s'", fp.Browser)
	}
	if fp.Platform != "windows" {
		t.Errorf("expected platform 'windows', got '%s'", fp.Platform)
	}

	params := h3.DefaultChromeTransportParams()
	if params.MaxIdleTimeout != 30_000 {
		t.Errorf("unexpected MaxIdleTimeout: %d", params.MaxIdleTimeout)
	}
	if params.InitialMaxData != 15_728_640 {
		t.Errorf("unexpected InitialMaxData: %d", params.InitialMaxData)
	}
}

func TestH3ServerCreate(t *testing.T) {
	srv := h3.NewServer(&h3.ServerConfig{
		ListenAddr: "127.0.0.1:0",
		PathPrefix: "/cdn/stream/",
	}, nil)
	if srv.Type() != "h3" {
		t.Errorf("expected type 'h3', got '%s'", srv.Type())
	}
}

func TestH3ClientDialClosed(t *testing.T) {
	client := h3.NewClient(&h3.ClientConfig{
		ServerAddr: "127.0.0.1:443",
		ServerName: "test.example.com",
	})
	client.Close()

	_, err := client.Dial(context.Background(), "127.0.0.1:443")
	if err == nil {
		t.Error("expected error dialing closed client")
	}
}
