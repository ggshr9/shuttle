package cdn

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func TestH2ClientFrontDomain(t *testing.T) {
	cfg := &H2Config{
		ServerAddr:  "server.example.com:443",
		CDNDomain:   "cdn.example.com",
		Path:        "/ws",
		Password:    "test",
		FrontDomain: "allowed.example.com",
	}
	c := NewH2Client(cfg)

	tr, ok := c.client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if got := tr.TLSClientConfig.ServerName; got != "allowed.example.com" {
		t.Errorf("TLS ServerName = %q, want %q", got, "allowed.example.com")
	}
}

func TestH2ClientNoFrontDomain(t *testing.T) {
	cfg := &H2Config{
		ServerAddr: "server.example.com:443",
		CDNDomain:  "cdn.example.com",
		Path:       "/ws",
		Password:   "test",
	}
	c := NewH2Client(cfg)

	tr, ok := c.client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	// Without FrontDomain, ServerName should be empty (Go TLS defaults to host from URL)
	if got := tr.TLSClientConfig.ServerName; got != "" {
		t.Errorf("TLS ServerName = %q, want empty (default behavior)", got)
	}
}

func TestGRPCClientFrontDomain(t *testing.T) {
	cfg := &GRPCConfig{
		ServerAddr:  "server.example.com:443",
		CDNDomain:   "cdn.example.com",
		Password:    "test",
		FrontDomain: "fronted.example.com",
	}
	c := NewGRPCClient(cfg)

	tr, ok := c.client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if got := tr.TLSClientConfig.ServerName; got != "fronted.example.com" {
		t.Errorf("TLS ServerName = %q, want %q", got, "fronted.example.com")
	}
}

func TestFrontDomainTLSVersion(t *testing.T) {
	// Verify that domain fronting doesn't weaken TLS settings.
	cfg := &H2Config{
		CDNDomain:   "cdn.example.com",
		FrontDomain: "front.example.com",
		Password:    "test",
	}
	c := NewH2Client(cfg)

	tr := c.client.Transport.(*http.Transport)
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want TLS 1.2 (%d)", tr.TLSClientConfig.MinVersion, tls.VersionTLS12)
	}
}
