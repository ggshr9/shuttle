package router

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestResolver creates a DNSResolver with the given config and an injected HTTP client.
func newTestResolver(cfg *DNSConfig, client *http.Client) *DNSResolver {
	if cfg.CacheSize == 0 {
		cfg.CacheSize = 100
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 10 * time.Minute
	}
	r := NewDNSResolver(cfg, nil, nil)
	r.httpClient = client
	return r
}

// newDoHServer starts an httptest server that returns a fixed DoH JSON response.
// It also counts requests via the returned *atomic.Int32.
func newDoHServer(t *testing.T, responseJSON string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(responseJSON))
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

func TestDNSResolver_CacheHit(t *testing.T) {
	const dohJSON = `{"Status":0,"Answer":[{"type":1,"data":"1.2.3.4"}]}`

	srv, reqCount := newDoHServer(t, dohJSON)

	r := newTestResolver(&DNSConfig{
		RemoteServer:   srv.URL,
		DomesticDoH:    srv.URL, // use DoH for domestic too so no real DNS
		LeakPrevention: true,
	}, srv.Client())

	ctx := context.Background()

	// First resolve — should hit the mock server (domestic + remote = 2 requests)
	ips, err := r.Resolve(ctx, "example.com")
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if len(ips) == 0 || !ips[0].Equal(net.ParseIP("1.2.3.4")) {
		t.Fatalf("unexpected IPs: %v", ips)
	}
	firstCount := reqCount.Load()
	if firstCount == 0 {
		t.Fatal("expected at least one request to DoH server")
	}

	// Second resolve — should come from cache, no new requests
	ips2, err := r.Resolve(ctx, "example.com")
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if len(ips2) == 0 || !ips2[0].Equal(net.ParseIP("1.2.3.4")) {
		t.Fatalf("unexpected cached IPs: %v", ips2)
	}
	if reqCount.Load() != firstCount {
		t.Fatalf("expected no new requests after cache hit, got %d (was %d)", reqCount.Load(), firstCount)
	}
}

func TestDNSResolver_StripECSParam(t *testing.T) {
	var capturedURL atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL.Store(r.URL.String())
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(`{"Status":0,"Answer":[{"type":1,"data":"1.2.3.4"}]}`))
	}))
	defer srv.Close()

	t.Run("StripECS_GoogleDoH", func(t *testing.T) {
		// buildDoHURL checks for "dns.google" in the server string.
		// We can't use a real Google URL in tests, so we test buildDoHURL directly.
		r := newTestResolver(&DNSConfig{
			StripECS: true,
		}, nil)

		url := r.buildDoHURL("https://dns.google/resolve", "example.com")
		if !strings.Contains(url, "edns_client_subnet=0.0.0.0/0") {
			t.Fatalf("expected ECS stripping param in URL, got: %s", url)
		}
	})

	t.Run("NoStripECS", func(t *testing.T) {
		r := newTestResolver(&DNSConfig{
			StripECS: false,
		}, nil)

		url := r.buildDoHURL("https://dns.google/resolve", "example.com")
		if strings.Contains(url, "edns_client_subnet") {
			t.Fatalf("expected no ECS param when StripECS is false, got: %s", url)
		}
	})

	t.Run("StripECS_NonGoogle", func(t *testing.T) {
		r := newTestResolver(&DNSConfig{
			StripECS: true,
		}, nil)

		url := r.buildDoHURL("https://1.1.1.1/dns-query", "example.com")
		if strings.Contains(url, "edns_client_subnet") {
			t.Fatalf("expected no ECS param for non-Google DoH, got: %s", url)
		}
	})
}

func TestDNSResolver_LeakPrevention(t *testing.T) {
	t.Run("BlocksPlaintextDomestic", func(t *testing.T) {
		// With leak prevention ON and no DomesticDoH, domestic queries should fail
		// rather than using plaintext UDP.
		r := newTestResolver(&DNSConfig{
			DomesticServer: "223.5.5.5:53",
			RemoteServer:   "https://1.1.1.1/dns-query", // will fail (no real network)
			LeakPrevention: true,
			// DomesticDoH intentionally empty
		}, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := r.queryDomestic(ctx, "example.com")
		if err == nil {
			t.Fatal("expected error when leak prevention blocks plaintext DNS")
		}
		if !strings.Contains(err.Error(), "leak prevention") {
			t.Fatalf("expected leak prevention error, got: %v", err)
		}
	})

	t.Run("AllowsDoHWithLeakPrevention", func(t *testing.T) {
		const dohJSON = `{"Status":0,"Answer":[{"type":1,"data":"5.6.7.8"}]}`
		srv, _ := newDoHServer(t, dohJSON)

		r := newTestResolver(&DNSConfig{
			DomesticDoH:    srv.URL,
			LeakPrevention: true,
		}, srv.Client())

		ctx := context.Background()
		ips, err := r.queryDomestic(ctx, "example.com")
		if err != nil {
			t.Fatalf("expected DoH to work with leak prevention: %v", err)
		}
		if len(ips) == 0 || !ips[0].Equal(net.ParseIP("5.6.7.8")) {
			t.Fatalf("unexpected IPs: %v", ips)
		}
	})
}

func TestDNSResolver_DomesticDoH(t *testing.T) {
	t.Run("UsesConfiguredDoHURL", func(t *testing.T) {
		var capturedPath atomic.Value

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath.Store(r.URL.String())
			w.Header().Set("Content-Type", "application/dns-json")
			_, _ = w.Write([]byte(`{"Status":0,"Answer":[{"type":1,"data":"10.20.30.40"}]}`))
		}))
		defer srv.Close()

		r := newTestResolver(&DNSConfig{
			DomesticDoH: srv.URL,
		}, srv.Client())

		ctx := context.Background()
		ips, err := r.resolveDomesticDoH(ctx, "baidu.com")
		if err != nil {
			t.Fatalf("domestic DoH failed: %v", err)
		}
		if len(ips) == 0 || !ips[0].Equal(net.ParseIP("10.20.30.40")) {
			t.Fatalf("unexpected IPs: %v", ips)
		}

		path, _ := capturedPath.Load().(string)
		if !strings.Contains(path, "name=baidu.com") {
			t.Fatalf("expected domain in DoH query URL, got: %s", path)
		}
		if !strings.Contains(path, "type=A") {
			t.Fatalf("expected type=A in DoH query URL, got: %s", path)
		}
	})

	t.Run("DomesticDoHWithStripECS", func(t *testing.T) {
		r := newTestResolver(&DNSConfig{
			DomesticDoH: "https://dns.google/resolve",
			StripECS:    true,
		}, nil)

		url := r.buildDoHURL(r.config.DomesticDoH, "example.cn")
		if !strings.Contains(url, "edns_client_subnet=0.0.0.0/0") {
			t.Fatalf("expected ECS strip param for Google domestic DoH, got: %s", url)
		}
	})
}
