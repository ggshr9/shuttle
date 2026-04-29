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

// --- Task 4: DNS Hosts Table ---

func TestResolveHosts_ExactMatch(t *testing.T) {
	r := newTestResolver(&DNSConfig{
		Hosts: map[string]string{
			"myhost.local": "10.0.0.1",
			"other.local":  "10.0.0.2",
		},
	}, nil)

	ip := r.resolveHosts("myhost.local")
	if ip == nil || !ip.Equal(net.ParseIP("10.0.0.1")) {
		t.Fatalf("expected 10.0.0.1, got %v", ip)
	}

	ip2 := r.resolveHosts("other.local")
	if ip2 == nil || !ip2.Equal(net.ParseIP("10.0.0.2")) {
		t.Fatalf("expected 10.0.0.2, got %v", ip2)
	}
}

func TestResolveHosts_Wildcard(t *testing.T) {
	r := newTestResolver(&DNSConfig{
		Hosts: map[string]string{
			"*.example.com": "10.0.0.99",
		},
	}, nil)

	// sub.example.com should match *.example.com
	ip := r.resolveHosts("sub.example.com")
	if ip == nil || !ip.Equal(net.ParseIP("10.0.0.99")) {
		t.Fatalf("expected 10.0.0.99, got %v", ip)
	}

	// foo.example.com should also match
	ip2 := r.resolveHosts("foo.example.com")
	if ip2 == nil || !ip2.Equal(net.ParseIP("10.0.0.99")) {
		t.Fatalf("expected 10.0.0.99, got %v", ip2)
	}

	// example.com itself should NOT match *.example.com
	ip3 := r.resolveHosts("example.com")
	if ip3 != nil {
		t.Fatalf("expected nil for bare domain, got %v", ip3)
	}
}

func TestResolveHosts_NoMatch(t *testing.T) {
	r := newTestResolver(&DNSConfig{
		Hosts: map[string]string{
			"myhost.local": "10.0.0.1",
		},
	}, nil)

	ip := r.resolveHosts("unknown.local")
	if ip != nil {
		t.Fatalf("expected nil for unknown domain, got %v", ip)
	}
}

func TestResolveHosts_EmptyHosts(t *testing.T) {
	r := newTestResolver(&DNSConfig{}, nil)

	ip := r.resolveHosts("anything.com")
	if ip != nil {
		t.Fatalf("expected nil for empty hosts, got %v", ip)
	}
}

func TestResolveHosts_InResolve(t *testing.T) {
	// Verify that Resolve() returns hosts entries without hitting upstream DNS
	r := newTestResolver(&DNSConfig{
		Hosts: map[string]string{
			"static.local": "192.168.1.100",
		},
		// No upstream servers configured — would fail if hosts didn't short-circuit
		RemoteServer:   "https://127.0.0.1:1/invalid",
		DomesticServer: "127.0.0.1:1",
		LeakPrevention: true,
	}, nil)

	ctx := context.Background()
	ips, err := r.Resolve(ctx, "static.local")
	if err != nil {
		t.Fatalf("Resolve with hosts entry should not fail: %v", err)
	}
	if len(ips) != 1 || !ips[0].Equal(net.ParseIP("192.168.1.100")) {
		t.Fatalf("expected [192.168.1.100], got %v", ips)
	}
}

// --- Task 5: Per-Domain DNS Policy ---

func TestDomainPolicy_ExactMatch(t *testing.T) {
	r := newTestResolver(&DNSConfig{
		DomainPolicy: []DomainPolicyEntry{
			{Domain: "corp.internal", Server: "10.0.0.1:53"},
		},
	}, nil)

	server := r.matchDomainPolicy("corp.internal")
	if server != "10.0.0.1:53" {
		t.Fatalf("expected 10.0.0.1:53, got %q", server)
	}
}

func TestDomainPolicy_SubdomainMatch(t *testing.T) {
	r := newTestResolver(&DNSConfig{
		DomainPolicy: []DomainPolicyEntry{
			{Domain: "+.google.com", Server: "https://8.8.8.8/dns-query"},
		},
	}, nil)

	// Exact base domain matches +.google.com
	server := r.matchDomainPolicy("google.com")
	if server != "https://8.8.8.8/dns-query" {
		t.Fatalf("expected base domain match, got %q", server)
	}

	// Subdomain matches +.google.com
	server2 := r.matchDomainPolicy("mail.google.com")
	if server2 != "https://8.8.8.8/dns-query" {
		t.Fatalf("expected subdomain match, got %q", server2)
	}

	// Deep subdomain matches
	server3 := r.matchDomainPolicy("a.b.google.com")
	if server3 != "https://8.8.8.8/dns-query" {
		t.Fatalf("expected deep subdomain match, got %q", server3)
	}
}

func TestDomainPolicy_NoMatch(t *testing.T) {
	r := newTestResolver(&DNSConfig{
		DomainPolicy: []DomainPolicyEntry{
			{Domain: "+.google.com", Server: "https://8.8.8.8/dns-query"},
			{Domain: "corp.internal", Server: "10.0.0.1:53"},
		},
	}, nil)

	server := r.matchDomainPolicy("example.com")
	if server != "" {
		t.Fatalf("expected no match, got %q", server)
	}
}

func TestDomainPolicy_DoHServerInResolve(t *testing.T) {
	// Verify that domain policy routes to the specific DoH server
	const dohJSON = `{"Status":0,"Answer":[{"type":1,"data":"172.16.0.1"}]}`
	srv, reqCount := newDoHServer(t, dohJSON)

	r := newTestResolver(&DNSConfig{
		DomainPolicy: []DomainPolicyEntry{
			{Domain: "+.corp.example", Server: srv.URL},
		},
		// Main servers are invalid — would fail if policy didn't short-circuit
		RemoteServer:   "https://127.0.0.1:1/invalid",
		DomesticServer: "127.0.0.1:1",
		LeakPrevention: true,
	}, srv.Client())

	ctx := context.Background()
	ips, err := r.Resolve(ctx, "app.corp.example")
	if err != nil {
		t.Fatalf("Resolve with domain policy should not fail: %v", err)
	}
	if len(ips) != 1 || !ips[0].Equal(net.ParseIP("172.16.0.1")) {
		t.Fatalf("expected [172.16.0.1], got %v", ips)
	}
	if reqCount.Load() != 1 {
		t.Fatalf("expected exactly 1 request to policy server, got %d", reqCount.Load())
	}
}

func TestResolver_MetricHookCalled(t *testing.T) {
	const dohJSON = `{"Status":0,"Answer":[{"type":1,"data":"1.2.3.4"}]}`
	srv, _ := newDoHServer(t, dohJSON)

	r := newTestResolver(&DNSConfig{
		RemoteServer:   srv.URL,
		DomesticDoH:    srv.URL,
		LeakPrevention: true,
	}, srv.Client())

	var queries, failures int32
	var lastProtocol string
	var lastCached atomic.Bool
	var lastDuration atomic.Int64
	r.SetMetricHook(&MetricHook{
		OnQuery: func(protocol string, cached bool, d time.Duration) {
			atomic.AddInt32(&queries, 1)
			lastProtocol = protocol
			lastCached.Store(cached)
			lastDuration.Store(int64(d))
		},
		OnFailure: func(string) { atomic.AddInt32(&failures, 1) },
	})

	ctx := context.Background()
	if _, err := r.Resolve(ctx, "example.com"); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if got := atomic.LoadInt32(&queries); got != 1 {
		t.Fatalf("expected 1 OnQuery call after first resolve, got %d", got)
	}
	if got := atomic.LoadInt32(&failures); got != 0 {
		t.Fatalf("expected 0 OnFailure calls, got %d", got)
	}
	if lastProtocol != "split-dns" {
		t.Fatalf("expected protocol=split-dns on first resolve, got %q", lastProtocol)
	}
	if lastCached.Load() {
		t.Fatal("expected cached=false on first resolve")
	}
	if lastDuration.Load() <= 0 {
		t.Fatal("expected duration > 0 on first resolve")
	}

	// Second resolve hits the cache — protocol should be "cache" with cached=true.
	if _, err := r.Resolve(ctx, "example.com"); err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if got := atomic.LoadInt32(&queries); got != 2 {
		t.Fatalf("expected 2 OnQuery calls after cache hit, got %d", got)
	}
	if lastProtocol != "cache" {
		t.Fatalf("expected protocol=cache on second resolve, got %q", lastProtocol)
	}
	if !lastCached.Load() {
		t.Fatal("expected cached=true on second resolve")
	}
}

func TestResolver_MetricHookFailure(t *testing.T) {
	// Point both servers at an unreachable port and disable leak prevention so
	// queryDomestic falls through to the actual UDP path. Use a context with
	// an immediate deadline so Resolve fails deterministically without making
	// a real network call.
	r := newTestResolver(&DNSConfig{
		RemoteServer:   "https://127.0.0.1:1/invalid",
		DomesticServer: "127.0.0.1:1",
		LeakPrevention: true, // refuses domestic plaintext, forces remote-only path which will fail
	}, nil)

	var failures int32
	var lastReason string
	r.SetMetricHook(&MetricHook{
		OnFailure: func(reason string) {
			atomic.AddInt32(&failures, 1)
			lastReason = reason
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately so both lookups fail with context error.

	if _, err := r.Resolve(ctx, "example.com"); err == nil {
		t.Fatal("expected resolve to fail with cancelled context")
	}
	if got := atomic.LoadInt32(&failures); got != 1 {
		t.Fatalf("expected 1 OnFailure call, got %d", got)
	}
	if lastReason != "timeout" {
		t.Fatalf("expected reason=timeout for cancelled context, got %q", lastReason)
	}
}

func TestClassifyDNSErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"deadline exceeded", context.DeadlineExceeded, "timeout"},
		{"context cancelled", context.Canceled, "timeout"},
		{"dnserror not found", &net.DNSError{IsNotFound: true}, "nxdomain"},
		{"dnserror timeout", &net.DNSError{IsTimeout: true}, "timeout"},
		{"no such host string", &timeoutError{msg: "lookup foo: no such host"}, "nxdomain"},
		{"i/o timeout string", &timeoutError{msg: "read udp: i/o timeout"}, "timeout"},
		{"unknown defaults to refused", &timeoutError{msg: "connection refused"}, "refused"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyDNSErr(tc.err); got != tc.want {
				t.Fatalf("ClassifyDNSErr(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// timeoutError is a minimal error type for ClassifyDNSErr string-matching tests.
type timeoutError struct{ msg string }

func (e *timeoutError) Error() string { return e.msg }
