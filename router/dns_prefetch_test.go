package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrefetcherRecord(t *testing.T) {
	p := NewPrefetcher(nil, 10, time.Second, nil)

	p.Record("example.com", 5*time.Minute)
	p.Record("example.com", 5*time.Minute)
	p.Record("other.com", 3*time.Minute)

	p.mu.Lock()
	defer p.mu.Unlock()

	if ds, ok := p.domains["example.com"]; !ok {
		t.Fatal("expected example.com to be tracked")
	} else if ds.count != 2 {
		t.Fatalf("expected count 2, got %d", ds.count)
	} else if ds.ttl != 5*time.Minute {
		t.Fatalf("expected ttl 5m, got %v", ds.ttl)
	}

	if ds, ok := p.domains["other.com"]; !ok {
		t.Fatal("expected other.com to be tracked")
	} else if ds.count != 1 {
		t.Fatalf("expected count 1, got %d", ds.count)
	}
}

func TestPrefetcherTopDomains(t *testing.T) {
	p := NewPrefetcher(nil, 3, time.Second, nil)

	// Record domains with different frequencies.
	for i := 0; i < 10; i++ {
		p.Record("top.com", time.Minute)
	}
	for i := 0; i < 5; i++ {
		p.Record("mid.com", time.Minute)
	}
	for i := 0; i < 3; i++ {
		p.Record("low.com", time.Minute)
	}
	p.Record("rare.com", time.Minute)

	top := p.topDomains()
	if len(top) != 3 {
		t.Fatalf("expected 3 top domains, got %d", len(top))
	}
	if top[0] != "top.com" {
		t.Fatalf("expected top.com first, got %s", top[0])
	}
	if top[1] != "mid.com" {
		t.Fatalf("expected mid.com second, got %s", top[1])
	}
	if top[2] != "low.com" {
		t.Fatalf("expected low.com third, got %s", top[2])
	}
}

func TestPrefetcherCleanup(t *testing.T) {
	p := NewPrefetcher(nil, 10, time.Second, nil)

	// Record a domain and manually set lastSeen to 2 hours ago.
	p.Record("old.com", time.Minute)
	p.mu.Lock()
	p.domains["old.com"].lastSeen = time.Now().Add(-2 * time.Hour)
	p.mu.Unlock()

	// Record a recent domain.
	p.Record("new.com", time.Minute)

	p.cleanup()

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.domains["old.com"]; ok {
		t.Fatal("expected old.com to be cleaned up")
	}
	if _, ok := p.domains["new.com"]; !ok {
		t.Fatal("expected new.com to remain")
	}
}

func TestPrefetcherStart(t *testing.T) {
	// Track how many times the mock resolver is called.
	var resolveCount int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&resolveCount, 1)
		w.Header().Set("Content-Type", "application/dns-json")
		resp := dohResponse{
			Answer: []dohAnswer{
				{Type: 1, Data: "1.2.3.4"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	resolver := NewDNSResolver(&DNSConfig{
		RemoteServer:   srv.URL,
		DomesticDoH:    srv.URL,
		LeakPrevention: true,
		CacheSize:      100,
		CacheTTL:       50 * time.Millisecond, // very short TTL so cache expires quickly
	}, nil, nil)
	resolver.httpClient = srv.Client()

	p := NewPrefetcher(resolver, 10, 10*time.Millisecond, nil)

	// Record a domain with a very short TTL and set lastSeen in the past
	// so it qualifies for prefetching immediately.
	p.Record("prefetch-me.com", 20*time.Millisecond)
	p.mu.Lock()
	p.domains["prefetch-me.com"].lastSeen = time.Now().Add(-1 * time.Minute)
	p.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start blocks until ctx is done.
	p.Start(ctx)

	count := atomic.LoadInt64(&resolveCount)
	if count == 0 {
		t.Fatal("expected at least one prefetch resolve call")
	}

	stats := p.Stats()
	if stats.PrefetchCount == 0 {
		t.Fatal("expected PrefetchCount > 0")
	}
}

func TestPrefetcherStats(t *testing.T) {
	p := NewPrefetcher(nil, 5, time.Second, nil)

	for i := 0; i < 3; i++ {
		p.Record(fmt.Sprintf("d%d.com", i), time.Minute)
	}

	stats := p.Stats()
	if stats.TrackedDomains != 3 {
		t.Fatalf("expected 3 tracked domains, got %d", stats.TrackedDomains)
	}
	if len(stats.TopDomains) != 3 {
		t.Fatalf("expected 3 top domains, got %d", len(stats.TopDomains))
	}
	if stats.PrefetchCount != 0 {
		t.Fatalf("expected 0 prefetch count, got %d", stats.PrefetchCount)
	}
}
