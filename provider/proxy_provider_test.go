package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var clashYAMLFixture = `proxies:
  - name: "HK-1"
    type: trojan
    server: hk.example.com
    port: 443
    password: pass1
    sni: hk.example.com
  - name: "HK-2"
    type: ss
    server: hk2.example.com
    port: 8388
    password: pass2
    cipher: chacha20-ietf-poly1305
  - name: "SG-1"
    type: ss
    server: sg.example.com
    port: 8388
    password: pass3
    cipher: aes-128-gcm
  - name: "US-1"
    type: trojan
    server: us.example.com
    port: 443
    password: pass4
    sni: us.example.com
`

// TestProxyProvider_FetchAndFilter verifies that the provider fetches the remote
// Clash YAML, parses it, and applies the filter regexp to keep only matching nodes.
func TestProxyProvider_FetchAndFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(clashYAMLFixture))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "provider.yaml")

	p, err := NewProxyProvider(ProxyProviderConfig{
		Name:     "test-provider",
		URL:      srv.URL,
		Path:     cachePath,
		Interval: time.Hour,
		Filter:   "^HK",
	})
	if err != nil {
		t.Fatalf("NewProxyProvider: %v", err)
	}
	allowLoopback(p)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	nodes := p.Nodes()
	if len(nodes) != 2 {
		t.Fatalf("expected 2 HK nodes, got %d: %v", len(nodes), nodeNames(nodes))
	}
	for _, n := range nodes {
		if n.Name != "HK-1" && n.Name != "HK-2" {
			t.Errorf("unexpected node %q; filter should only keep HK nodes", n.Name)
		}
	}

	if p.Error() != nil {
		t.Errorf("Error() should be nil after successful refresh, got %v", p.Error())
	}
	if p.UpdatedAt().IsZero() {
		t.Error("UpdatedAt() should not be zero after successful refresh")
	}

	// Cache file should have been written
	if _, err := os.Stat(cachePath); err != nil {
		t.Errorf("cache file not created: %v", err)
	}
}

// TestProxyProvider_NoFilter verifies that without a filter all nodes are returned.
func TestProxyProvider_NoFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(clashYAMLFixture))
	}))
	defer srv.Close()

	p, err := NewProxyProvider(ProxyProviderConfig{
		Name:     "no-filter",
		URL:      srv.URL,
		Interval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewProxyProvider: %v", err)
	}
	allowLoopback(p)

	if err := p.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got := len(p.Nodes()); got != 4 {
		t.Fatalf("expected 4 nodes without filter, got %d", got)
	}
}

// TestProxyProvider_ErrorHandling verifies that a bad URL records the error.
func TestProxyProvider_ErrorHandling(t *testing.T) {
	p, err := NewProxyProvider(ProxyProviderConfig{
		Name:     "bad-provider",
		URL:      "http://127.0.0.1:1", // nothing listening here
		Interval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewProxyProvider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	refreshErr := p.Refresh(ctx)
	if refreshErr == nil {
		t.Fatal("expected error from unreachable URL, got nil")
	}
	if p.Error() == nil {
		t.Fatal("Error() should be non-nil after failed refresh")
	}
	if len(p.Nodes()) != 0 {
		t.Errorf("expected 0 nodes after failed refresh, got %d", len(p.Nodes()))
	}
}

// TestProxyProvider_HTTP404 verifies that a non-2xx status records an error.
func TestProxyProvider_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p, err := NewProxyProvider(ProxyProviderConfig{
		Name: "404-provider",
		URL:  srv.URL,
	})
	if err != nil {
		t.Fatalf("NewProxyProvider: %v", err)
	}

	if err := p.Refresh(context.Background()); err == nil {
		t.Fatal("expected error for HTTP 404, got nil")
	}
	if p.Error() == nil {
		t.Error("Error() should be non-nil after HTTP 404")
	}
}

// TestProxyProvider_InvalidFilterRegexp verifies that a bad filter regexp
// is rejected at construction time.
func TestProxyProvider_InvalidFilterRegexp(t *testing.T) {
	_, err := NewProxyProvider(ProxyProviderConfig{
		Name:   "bad-filter",
		URL:    "http://example.com/sub",
		Filter: "[invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid filter regexp")
	}
}

// TestProxyProvider_InvalidURLScheme verifies that file:// etc. are rejected.
func TestProxyProvider_InvalidURLScheme(t *testing.T) {
	_, err := NewProxyProvider(ProxyProviderConfig{
		Name: "file-scheme",
		URL:  "file:///etc/passwd",
	})
	if err == nil {
		t.Fatal("expected error for file:// URL scheme")
	}
}

// TestProxyProvider_CacheLoad verifies that existing cache is loaded at construction.
func TestProxyProvider_CacheLoad(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "cache.yaml")

	// Write a valid Clash YAML into the cache path
	if err := os.WriteFile(cachePath, []byte(clashYAMLFixture), 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	p, err := NewProxyProvider(ProxyProviderConfig{
		Name: "cached",
		URL:  "http://example.com/sub", // won't be fetched
		Path: cachePath,
	})
	if err != nil {
		t.Fatalf("NewProxyProvider: %v", err)
	}

	if got := len(p.Nodes()); got != 4 {
		t.Fatalf("expected 4 cached nodes, got %d", got)
	}
}

// TestProxyProvider_StartStop verifies Start/Stop don't panic.
func TestProxyProvider_StartStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(clashYAMLFixture))
	}))
	defer srv.Close()

	p, err := NewProxyProvider(ProxyProviderConfig{
		Name:     "start-stop",
		URL:      srv.URL,
		Interval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewProxyProvider: %v", err)
	}
	allowLoopback(p)

	ctx := context.Background()
	p.Start(ctx)
	// Give the initial fetch and one tick time to run
	time.Sleep(120 * time.Millisecond)
	p.Stop()

	if len(p.Nodes()) == 0 {
		t.Error("expected nodes after Start, got none")
	}
}

// TestProxyProvider_Name verifies Name() returns the configured name.
func TestProxyProvider_Name(t *testing.T) {
	p, err := NewProxyProvider(ProxyProviderConfig{
		Name: "my-provider",
		URL:  "http://example.com/sub",
	})
	if err != nil {
		t.Fatalf("NewProxyProvider: %v", err)
	}
	if p.Name() != "my-provider" {
		t.Errorf("Name() = %q, want my-provider", p.Name())
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func nodeNames(nodes []ProxyNode) []string {
	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Name
	}
	return names
}
