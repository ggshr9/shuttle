package router

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// domainStats tracks resolution frequency for a domain.
type domainStats struct {
	count    int64
	lastSeen time.Time
	ttl      time.Duration // last known TTL from DNS response
}

// Prefetcher proactively re-resolves popular domains before their TTL expires.
type Prefetcher struct {
	mu            sync.RWMutex
	domains       map[string]*domainStats
	resolver      *DNSResolver
	topN          int           // number of top domains to prefetch (default 100)
	maxDomains    int           // hard cap on tracked entries; bottom-by-count are evicted past this (default 10*topN)
	interval      time.Duration // check interval (default 30s)
	threshold     float64       // prefetch at this fraction of TTL (default 0.75)
	prefetchCount int64         // total prefetch resolutions performed (atomic)
	logger        *slog.Logger
}

// NewPrefetcher creates a new DNS prefetcher.
func NewPrefetcher(resolver *DNSResolver, topN int, interval time.Duration, logger *slog.Logger) *Prefetcher {
	if topN <= 0 {
		topN = 100
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Prefetcher{
		domains:    make(map[string]*domainStats),
		resolver:   resolver,
		topN:       topN,
		maxDomains: topN * 10,
		interval:   interval,
		threshold:  0.75,
		logger:     logger,
	}
}

// Record records a successful DNS resolution. Called after each resolve.
// ttl is the TTL from the DNS response.
func (p *Prefetcher) Record(domain string, ttl time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ds, ok := p.domains[domain]
	if !ok {
		// New entry: enforce the cap before insert. High-cardinality
		// FQDN traffic (e.g. CDN-style hostnames) used to balloon the
		// map between hourly cleanups; cap = 10*topN is large enough
		// to find the prefetch winners while bounding memory.
		if p.maxDomains > 0 && len(p.domains) >= p.maxDomains {
			p.evictLowestCountLocked(len(p.domains) - p.maxDomains + 1)
		}
		ds = &domainStats{}
		p.domains[domain] = ds
	}
	ds.count++
	ds.lastSeen = time.Now()
	if ttl > 0 {
		ds.ttl = ttl
	}
}

// evictLowestCountLocked removes n entries with the lowest count.
// Caller must hold p.mu.Lock.
func (p *Prefetcher) evictLowestCountLocked(n int) {
	if n <= 0 || len(p.domains) == 0 {
		return
	}
	type kv struct {
		key   string
		count int64
	}
	all := make([]kv, 0, len(p.domains))
	for k, ds := range p.domains {
		all = append(all, kv{k, ds.count})
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].count < all[j].count
	})
	if n > len(all) {
		n = len(all)
	}
	for i := 0; i < n; i++ {
		delete(p.domains, all[i].key)
	}
}

// Start begins the prefetch loop. Blocks until ctx is cancelled.
func (p *Prefetcher) Start(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.cleanup()
			p.prefetchTop(ctx)
		}
	}
}

// prefetchTop re-resolves the top domains whose cache entries are nearing expiry.
func (p *Prefetcher) prefetchTop(ctx context.Context) {
	top := p.topDomains()
	now := time.Now()

	p.mu.RLock()
	// Build a snapshot of domains that need prefetching.
	type prefetchItem struct {
		domain string
		ttl    time.Duration
	}
	var items []prefetchItem
	for _, domain := range top {
		ds := p.domains[domain]
		if ds == nil {
			continue
		}
		ttl := ds.ttl
		if ttl <= 0 {
			ttl = 10 * time.Minute // fallback
		}
		// Check if the domain's cache entry is within the threshold of expiring.
		// A domain last seen at time T with TTL D should be prefetched when
		// now >= T + threshold*D (i.e., 75% of TTL has elapsed since last seen).
		elapsed := now.Sub(ds.lastSeen)
		if elapsed >= time.Duration(float64(ttl)*p.threshold) {
			items = append(items, prefetchItem{domain: domain, ttl: ttl})
		}
	}
	p.mu.RUnlock()

	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		resolveCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := p.resolver.Resolve(resolveCtx, item.domain)
		cancel()
		if err != nil {
			p.logger.Debug("prefetch resolve failed", "domain", item.domain, "err", err)
			continue
		}
		atomic.AddInt64(&p.prefetchCount, 1)
		p.logger.Debug("prefetched domain", "domain", item.domain)
	}
}

// topDomains returns the top N most frequently resolved domains.
func (p *Prefetcher) topDomains() []string {
	type domainCount struct {
		domain string
		count  int64
	}

	p.mu.RLock()
	all := make([]domainCount, 0, len(p.domains))
	for d, ds := range p.domains {
		all = append(all, domainCount{domain: d, count: ds.count})
	}
	topN := p.topN
	p.mu.RUnlock()

	// Sort outside the lock
	sort.Slice(all, func(i, j int) bool {
		return all[i].count > all[j].count
	})

	n := topN
	if n > len(all) {
		n = len(all)
	}
	result := make([]string, n)
	for i := 0; i < n; i++ {
		result[i] = all[i].domain
	}
	return result
}

// cleanup removes domains not seen in the last hour.
func (p *Prefetcher) cleanup() {
	cutoff := time.Now().Add(-1 * time.Hour)

	// Collect stale keys under read lock.
	p.mu.RLock()
	var stale []string
	for domain, ds := range p.domains {
		if ds.lastSeen.Before(cutoff) {
			stale = append(stale, domain)
		}
	}
	p.mu.RUnlock()

	if len(stale) == 0 {
		return
	}

	// Delete under write lock.
	p.mu.Lock()
	for _, domain := range stale {
		// Re-check in case it was updated between RUnlock and Lock.
		if ds, ok := p.domains[domain]; ok && ds.lastSeen.Before(cutoff) {
			delete(p.domains, domain)
		}
	}
	p.mu.Unlock()
}

// PrefetchStats holds prefetcher statistics.
type PrefetchStats struct {
	TrackedDomains int      `json:"tracked_domains"`
	TopDomains     []string `json:"top_domains"`
	PrefetchCount  int64    `json:"prefetch_count"` // total prefetch resolutions performed
}

// Stats returns prefetcher statistics.
func (p *Prefetcher) Stats() PrefetchStats {
	top := p.topDomains()

	p.mu.RLock()
	tracked := len(p.domains)
	p.mu.RUnlock()

	return PrefetchStats{
		TrackedDomains: tracked,
		TopDomains:     top,
		PrefetchCount:  atomic.LoadInt64(&p.prefetchCount),
	}
}
