package provider

import (
	"time"

	"github.com/ggshr9/shuttle/server"
)

// allowLoopback swaps the provider's HTTP client for one that permits
// connections to private/loopback addresses. Intended for tests that serve
// fixtures via httptest (which binds 127.0.0.1) — the default SafeHTTPClient
// blocks loopback to enforce SSRF protection.
func allowLoopback(p *ProxyProvider) {
	p.client = server.NewSafeHTTPClient(server.SafeHTTPClientOptions{
		Timeout:              30 * time.Second,
		MaxRedirects:         5,
		AllowPrivateNetworks: true,
	})
}

func allowLoopbackRule(p *RuleProvider) {
	p.client = server.NewSafeHTTPClient(server.SafeHTTPClientOptions{
		Timeout:              30 * time.Second,
		MaxRedirects:         5,
		AllowPrivateNetworks: true,
	})
}
