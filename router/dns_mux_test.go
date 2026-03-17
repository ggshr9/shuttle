package router

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDNSMultiplexer_Dedup(t *testing.T) {
	// Mock DoH server that counts requests per domain.
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		// Small delay so concurrent callers overlap.
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(`{"Status":0,"Answer":[{"type":1,"data":"9.8.7.6"}]}`))
	}))
	defer srv.Close()

	resolver := NewDNSResolver(&DNSConfig{
		RemoteServer:   srv.URL,
		DomesticDoH:    srv.URL,
		LeakPrevention: true,
		PersistentConn: true,
		CacheSize:      100,
		CacheTTL:       1 * time.Millisecond, // tiny TTL so cache doesn't mask dedup
	}, nil, nil)
	defer resolver.Close()

	// Fire 10 concurrent queries for the same domain.
	const n = 10
	var wg sync.WaitGroup
	results := make([][]net.IP, n)
	errs := make([]error, n)

	// Bypass the Resolve() cache by calling resolveDoH directly.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = resolver.resolveDoH(context.Background(), srv.URL, "dedup.example.com")
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("query %d failed: %v", i, errs[i])
		}
		if len(results[i]) == 0 || !results[i][0].Equal(net.ParseIP("9.8.7.6")) {
			t.Fatalf("query %d unexpected result: %v", i, results[i])
		}
	}

	// Only 1 HTTP request should have been sent (the rest should have been deduped).
	got := reqCount.Load()
	if got != 1 {
		t.Fatalf("expected 1 HTTP request (dedup), got %d", got)
	}
}

func TestDNSMultiplexer_DifferentDomains(t *testing.T) {
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		time.Sleep(20 * time.Millisecond)
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(`{"Status":0,"Answer":[{"type":1,"data":"1.1.1.1"}]}`))
	}))
	defer srv.Close()

	resolver := NewDNSResolver(&DNSConfig{
		RemoteServer:   srv.URL,
		PersistentConn: true,
		CacheSize:      100,
		CacheTTL:       1 * time.Millisecond,
	}, nil, nil)
	defer resolver.Close()

	var wg sync.WaitGroup
	domains := []string{"a.example.com", "b.example.com", "c.example.com"}
	errs := make([]error, len(domains))

	for i, d := range domains {
		wg.Add(1)
		go func(idx int, domain string) {
			defer wg.Done()
			_, errs[idx] = resolver.resolveDoH(context.Background(), srv.URL, domain)
		}(i, d)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("query %d (%s) failed: %v", i, domains[i], err)
		}
	}

	got := reqCount.Load()
	if got != int32(len(domains)) { //nolint:gosec // G115: test domain count is small, fits int32
		t.Fatalf("expected %d HTTP requests (one per domain), got %d", len(domains), got)
	}
}

func TestDNSMultiplexer_ContextCancel(t *testing.T) {
	// Server that blocks longer than context allows.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(`{"Status":0,"Answer":[{"type":1,"data":"1.2.3.4"}]}`))
	}))
	defer srv.Close()

	resolver := NewDNSResolver(&DNSConfig{
		RemoteServer:   srv.URL,
		PersistentConn: true,
		CacheSize:      100,
		CacheTTL:       1 * time.Millisecond,
	}, nil, nil)
	defer resolver.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := resolver.resolveDoH(ctx, srv.URL, "slow.example.com")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestDNSMultiplexer_Close(t *testing.T) {
	mux := NewDNSMultiplexer()

	// Force client creation.
	_ = mux.getOrCreateClient("https://example.com/dns-query")

	mux.mu.RLock()
	if len(mux.clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(mux.clients))
	}
	mux.mu.RUnlock()

	mux.Close()

	mux.mu.RLock()
	defer mux.mu.RUnlock()
	if len(mux.clients) != 0 {
		t.Fatalf("expected 0 clients after Close, got %d", len(mux.clients))
	}
}

func TestDNSMultiplexer_Disabled(t *testing.T) {
	// When PersistentConn is false, the multiplexer should not be created.
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(`{"Status":0,"Answer":[{"type":1,"data":"2.3.4.5"}]}`))
	}))
	defer srv.Close()

	resolver := NewDNSResolver(&DNSConfig{
		RemoteServer:   srv.URL,
		PersistentConn: false,
		CacheSize:      100,
		CacheTTL:       1 * time.Millisecond,
	}, nil, nil)

	if resolver.mux != nil {
		t.Fatal("expected nil multiplexer when PersistentConn is false")
	}

	// Should still work via the fallback path.
	resolver.httpClient = srv.Client()
	ips, err := resolver.resolveDoH(context.Background(), srv.URL, "fallback.example.com")
	if err != nil {
		t.Fatalf("fallback resolveDoH failed: %v", err)
	}
	if len(ips) == 0 || !ips[0].Equal(net.ParseIP("2.3.4.5")) {
		t.Fatalf("unexpected IPs: %v", ips)
	}
}
