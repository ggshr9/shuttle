package router

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ggshr9/shuttle/internal/dnsclass"
	"github.com/ggshr9/shuttle/router/dns/fakeip"
)

// DNSConfig configures the DNS resolver.
type DNSConfig struct {
	DomesticServer string // e.g., "223.5.5.5:53"
	RemoteServer   string // e.g., "https://1.1.1.1/dns-query"
	RemoteViaProxy bool   // Whether remote DNS goes through proxy
	CacheSize      int
	CacheTTL       time.Duration
	Prefetch       bool
	LeakPrevention bool   // Force all DNS through DoH, never fall back to system resolver
	DomesticDoH    string // DoH URL for domestic queries (e.g., "https://dns.alidns.com/dns-query")
	StripECS       bool   // Strip EDNS Client Subnet from DoH queries
	PersistentConn bool   // Use persistent HTTP/2 connections with query deduplication
	Mode           string   // "normal" or "fake-ip"; empty defaults to normal
	FakeIPRange    string   // CIDR for fake-ip pool (default "198.18.0.0/15")
	FakeIPFilter   []string // domains to bypass fake-ip
	Hosts          map[string]string   // static hostname → IP mappings (supports *.example.com wildcards)
	DomainPolicy   []DomainPolicyEntry // per-domain nameserver policy
}

// DomainPolicyEntry maps a domain pattern to a specific DNS server.
type DomainPolicyEntry struct {
	Domain string
	Server string // DoH URL (https://...) or plain UDP (host:port)
}

// DNSResolver implements split DNS with anti-pollution.
type DNSResolver struct {
	config     *DNSConfig
	cache      *dnsCache
	geoIP      *GeoIPDB
	logger     *slog.Logger
	httpClient *http.Client       // injectable HTTP client for DoH (nil = default)
	mux        *DNSMultiplexer    // persistent connection pool + dedup (nil when disabled)
	prefetcher *Prefetcher        // optional DNS prefetcher (nil when disabled)
	fakeIPPool *fakeip.Pool       // fake-ip pool (nil when mode != "fake-ip")
	metricHook *MetricHook        // optional metric hook (nil when not installed)
}

// MetricHook is called after each resolver lookup attempt so callers can
// emit observability signals without coupling the resolver to a specific
// metrics package.
//
// OnQuery fires for each successful resolution. protocol is one of
// "cache", "hosts", "fakeip", "policy", "split-dns" describing which path
// produced the answer. cached is true when the result came from the
// in-memory DNS cache. duration is wall-clock time spent in the lookup.
//
// OnFailure fires for each failed resolution with a categorised reason
// returned by ClassifyDNSErr ("nxdomain", "timeout", "refused").
type MetricHook struct {
	OnQuery   func(protocol string, cached bool, duration time.Duration)
	OnFailure func(reason string)
}

// SetMetricHook installs a metric hook on the resolver. It is intended to
// be called once at startup; concurrent calls during active resolution
// are not synchronised.
func (r *DNSResolver) SetMetricHook(h *MetricHook) {
	r.metricHook = h
}

// SetPrefetcher sets the DNS prefetcher that will be notified of successful resolutions.
func (r *DNSResolver) SetPrefetcher(p *Prefetcher) {
	r.prefetcher = p
}

// ClassifyDNSErr is an alias for dnsclass.Classify, retained on the
// router package for ergonomics. It maps a resolver error to one of
// "nxdomain", "timeout", "refused".
func ClassifyDNSErr(err error) string { return dnsclass.Classify(err) }

type dnsCache struct {
	mu      sync.RWMutex
	entries map[string]*dnsCacheEntry
	maxSize int
}

type dnsCacheEntry struct {
	ips       []net.IP
	expiresAt time.Time
	domestic  bool // resolved via domestic DNS
	lastUsed  atomic.Int64 // Unix nanoseconds
}

// NewDNSResolver creates a new DNS resolver with anti-pollution.
func NewDNSResolver(cfg *DNSConfig, geoIP *GeoIPDB, logger *slog.Logger) *DNSResolver {
	if cfg.CacheSize == 0 {
		cfg.CacheSize = 10000
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 10 * time.Minute
	}
	if cfg.DomesticServer == "" {
		cfg.DomesticServer = "223.5.5.5:53"
	}
	if cfg.RemoteServer == "" {
		cfg.RemoteServer = "1.1.1.1:53"
	}
	if logger == nil {
		logger = slog.Default()
	}
	var mux *DNSMultiplexer
	if cfg.PersistentConn {
		mux = NewDNSMultiplexer()
	}

	var pool *fakeip.Pool
	if cfg.Mode == "fake-ip" {
		cidrStr := cfg.FakeIPRange
		if cidrStr == "" {
			cidrStr = "198.18.0.0/15"
		}
		prefix, err := netip.ParsePrefix(cidrStr)
		if err != nil {
			logger.Error("fakeip: invalid CIDR, falling back to normal DNS", "cidr", cidrStr, "err", err)
		} else {
			p, err := fakeip.NewPool(fakeip.PoolConfig{
				CIDR:   prefix,
				Filter: cfg.FakeIPFilter,
			})
			if err != nil {
				logger.Error("fakeip: pool creation failed, falling back to normal DNS", "err", err)
			} else {
				pool = p
			}
		}
	}

	return &DNSResolver{
		config: cfg,
		cache: &dnsCache{
			entries: make(map[string]*dnsCacheEntry, cfg.CacheSize),
			maxSize: cfg.CacheSize,
		},
		geoIP:      geoIP,
		logger:     logger,
		mux:        mux,
		fakeIPPool: pool,
	}
}

// Close releases resources held by the resolver.
func (r *DNSResolver) Close() {
	if r.mux != nil {
		r.mux.Close()
	}
}

// IsFakeIP reports whether ip falls within the fake-ip pool's CIDR range.
// Returns false when fake-ip mode is not active.
func (r *DNSResolver) IsFakeIP(ip net.IP) bool {
	if r.fakeIPPool == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	return r.fakeIPPool.IsFakeIP(addr.Unmap())
}

// ReverseFakeIP looks up the domain that was assigned the given fake IP.
// Returns ("", false) when fake-ip mode is not active or the IP is not a known fake IP.
func (r *DNSResolver) ReverseFakeIP(ip net.IP) (string, bool) {
	if r.fakeIPPool == nil {
		return "", false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return "", false
	}
	return r.fakeIPPool.Reverse(addr.Unmap())
}

// Resolve resolves a domain using the anti-pollution strategy:
// 1. Query both domestic and remote DNS
// 2. If results agree → use domestic (faster)
// 3. If they disagree → check if domestic result is foreign IP (pollution)
// 4. If polluted → use remote result
func (r *DNSResolver) Resolve(ctx context.Context, domain string) ([]net.IP, error) {
	start := time.Now()
	ips, protocol, cached, err := r.resolve(ctx, domain)
	if r.metricHook != nil {
		if err != nil {
			if r.metricHook.OnFailure != nil {
				r.metricHook.OnFailure(ClassifyDNSErr(err))
			}
		} else if r.metricHook.OnQuery != nil {
			r.metricHook.OnQuery(protocol, cached, time.Since(start))
		}
	}
	return ips, err
}

// resolve is the inner implementation of Resolve. It returns the resolved
// IPs along with a protocol label and a cache-hit flag for the metric
// hook in Resolve. Callers outside of Resolve should not use this method.
func (r *DNSResolver) resolve(ctx context.Context, domain string) ([]net.IP, string, bool, error) {
	// Fake-ip mode: return a virtual IP instead of querying upstream DNS.
	if r.fakeIPPool != nil && r.fakeIPPool.ShouldFakeIP(domain) {
		fakeAddr := r.fakeIPPool.Lookup(domain)
		return []net.IP{fakeAddr.AsSlice()}, "fakeip", false, nil
	}

	// Static hosts table — checked before cache
	if ip := r.resolveHosts(domain); ip != nil {
		return []net.IP{ip}, "hosts", false, nil
	}

	// Check cache first
	if ips, ok := r.cache.get(domain); ok {
		return ips, "cache", true, nil
	}

	// Per-domain DNS policy — route to a specific nameserver
	if server := r.matchDomainPolicy(domain); server != "" {
		ips, err := r.querySpecificServer(ctx, domain, server)
		if err != nil {
			return nil, "policy", false, err
		}
		r.cache.put(domain, ips, r.config.CacheTTL, r.isDomesticServer(server))
		r.recordPrefetch(domain)
		return ips, "policy", false, nil
	}

	// Query both DNS servers in parallel
	type result struct {
		ips []net.IP
		err error
	}
	domCh := make(chan result, 1)
	remCh := make(chan result, 1)

	go func() {
		select {
		case <-ctx.Done():
			domCh <- result{nil, ctx.Err()}
		default:
			ips, err := r.queryDomestic(ctx, domain)
			domCh <- result{ips, err}
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			remCh <- result{nil, ctx.Err()}
		default:
			ips, err := r.queryRemote(ctx, domain)
			remCh <- result{ips, err}
		}
	}()

	var domResult, remResult result

	select {
	case domResult = <-domCh:
	case <-ctx.Done():
		return nil, "split-dns", false, ctx.Err()
	}
	select {
	case remResult = <-remCh:
	case <-ctx.Done():
		return nil, "split-dns", false, ctx.Err()
	}

	ips, err := r.selectResult(domain, domResult.ips, domResult.err, remResult.ips, remResult.err)
	return ips, "split-dns", false, err
}

// resolveHosts checks the static hosts table for a matching entry.
// Supports exact match and wildcard (*.example.com) patterns.
func (r *DNSResolver) resolveHosts(domain string) net.IP {
	if len(r.config.Hosts) == 0 {
		return nil
	}
	// Exact match
	if addr, ok := r.config.Hosts[domain]; ok {
		return net.ParseIP(addr)
	}
	// Wildcard: *.example.com matches sub.example.com
	parts := strings.SplitN(domain, ".", 2)
	if len(parts) == 2 {
		if addr, ok := r.config.Hosts["*."+parts[1]]; ok {
			return net.ParseIP(addr)
		}
	}
	return nil
}

// matchDomainPolicy returns the DNS server for a domain if it matches a policy entry.
// Supports "+.example.com" (matches example.com and all subdomains) and exact match.
func (r *DNSResolver) matchDomainPolicy(domain string) string {
	for _, entry := range r.config.DomainPolicy {
		pattern := entry.Domain
		if strings.HasPrefix(pattern, "+.") {
			// "+.example.com" matches "example.com" and "*.example.com"
			base := pattern[2:]
			if domain == base || strings.HasSuffix(domain, "."+base) {
				return entry.Server
			}
		} else if domain == pattern {
			// Exact match
			return entry.Server
		}
	}
	return ""
}

// querySpecificServer queries a specific DNS server. Supports DoH (https://...) and plain UDP (host:port).
func (r *DNSResolver) querySpecificServer(ctx context.Context, domain, server string) ([]net.IP, error) {
	if strings.HasPrefix(server, "https://") || strings.HasPrefix(server, "http://") {
		return r.resolveDoH(ctx, server, domain)
	}
	// Plain UDP DNS
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "udp", server)
		},
	}
	addrs, err := resolver.LookupIPAddr(ctx, domain)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, len(addrs))
	for i, addr := range addrs {
		ips[i] = addr.IP
	}
	return ips, nil
}

// isDomesticServer returns true if the server appears to be a domestic (Chinese) DNS server.
func (r *DNSResolver) isDomesticServer(server string) bool {
	return server == r.config.DomesticServer || server == r.config.DomesticDoH
}

func (r *DNSResolver) selectResult(domain string, domIPs []net.IP, domErr error, remIPs []net.IP, remErr error) ([]net.IP, error) {
	// If only one succeeded, use that
	if domErr != nil && remErr != nil {
		return nil, fmt.Errorf("both DNS queries failed: domestic=%v, remote=%v", domErr, remErr)
	}
	if domErr != nil {
		r.cache.put(domain, remIPs, r.config.CacheTTL, false)
		r.recordPrefetch(domain)
		return remIPs, nil //nolint:nilerr // intentional: domestic failed, fall back to remote result
	}
	if remErr != nil {
		r.cache.put(domain, domIPs, r.config.CacheTTL, true)
		r.recordPrefetch(domain)
		return domIPs, nil //nolint:nilerr // intentional: remote failed, fall back to domestic result
	}

	// Both succeeded — check for pollution
	if r.ipsMatch(domIPs, remIPs) {
		// Results agree — use domestic (faster)
		r.cache.put(domain, domIPs, r.config.CacheTTL, true)
		r.recordPrefetch(domain)
		return domIPs, nil
	}

	// Results disagree — check if domestic result looks polluted
	if r.isPolluted(domIPs) {
		r.logger.Debug("DNS pollution detected",
			"domain", domain,
			"domestic", domIPs,
			"remote", remIPs)
		r.cache.put(domain, remIPs, r.config.CacheTTL, false)
		r.recordPrefetch(domain)
		return remIPs, nil
	}

	// Use domestic result (probably a CDN with region-specific IPs)
	r.cache.put(domain, domIPs, r.config.CacheTTL, true)
	r.recordPrefetch(domain)
	return domIPs, nil
}

// recordPrefetch notifies the prefetcher of a successful resolution.
func (r *DNSResolver) recordPrefetch(domain string) {
	if r.prefetcher != nil {
		r.prefetcher.Record(domain, r.config.CacheTTL)
	}
}

func (r *DNSResolver) ipsMatch(a, b []net.IP) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	// Check if at least one IP matches
	for _, ipA := range a {
		for _, ipB := range b {
			if ipA.Equal(ipB) {
				return true
			}
		}
	}
	return false
}

// knownPoisonIPs contains IPs commonly returned by DNS pollution in China.
// These are typically non-routable or belong to well-known hijacked ranges.
var knownPoisonIPs = map[string]bool{
	"1.1.1.1": true, "8.8.8.8": true, // Sometimes returned as poison
	"127.0.0.1": true, "0.0.0.0": true,
	"255.255.255.255": true,
	// Facebook IPs commonly used in DNS pollution
	"74.125.127.102": true, "74.125.155.102": true,
	"74.125.39.102": true, "74.125.39.113": true,
	"209.85.229.138": true,
	// Other known pollution IPs
	"4.36.66.178": true, "8.7.198.45": true,
	"37.61.54.158": true, "46.82.174.68": true,
	"59.24.3.173": true, "64.33.88.161": true,
	"64.33.99.47": true, "64.66.163.251": true,
	"65.104.202.252": true, "65.160.219.113": true,
	"66.45.252.237": true, "72.14.205.104": true,
	"72.14.205.99": true, "78.16.49.15": true,
	"93.46.8.89": true, "203.98.7.65": true,
	"207.12.88.98": true, "208.56.31.43": true,
	"209.145.54.50": true, "209.220.30.174": true,
	"211.94.66.147": true, "213.169.251.35": true,
	"216.221.188.182": true,
}

func (r *DNSResolver) isPolluted(ips []net.IP) bool {
	for _, ip := range ips {
		if knownPoisonIPs[ip.String()] {
			return true
		}
	}
	return false
}

func (r *DNSResolver) queryDomestic(ctx context.Context, domain string) ([]net.IP, error) {
	// Prefer DoH for domestic queries when configured
	if r.config.DomesticDoH != "" {
		return r.resolveDomesticDoH(ctx, domain)
	}

	// Leak prevention mode: refuse plaintext DNS
	if r.config.LeakPrevention {
		return nil, fmt.Errorf("domestic DNS skipped: leak prevention enabled and no domestic DoH configured")
	}

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "udp", r.config.DomesticServer)
		},
	}
	addrs, err := resolver.LookupIPAddr(ctx, domain)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, len(addrs))
	for i, addr := range addrs {
		ips[i] = addr.IP
	}
	return ips, nil
}

// resolveDomesticDoH resolves a domain using a domestic DoH endpoint.
func (r *DNSResolver) resolveDomesticDoH(ctx context.Context, domain string) ([]net.IP, error) {
	return r.resolveDoH(ctx, r.config.DomesticDoH, domain)
}

func (r *DNSResolver) queryRemote(ctx context.Context, domain string) ([]net.IP, error) {
	// Use DNS-over-HTTPS (DoH) with JSON API
	server := r.config.RemoteServer
	if server == "" {
		server = "https://1.1.1.1/dns-query"
	}
	// Ensure it's an HTTPS URL (allow http:// for testing)
	if !strings.HasPrefix(server, "https://") && !strings.HasPrefix(server, "http://") {
		// Legacy plain DNS config — upgrade to DoH
		server = "https://1.1.1.1/dns-query"
	}

	return r.resolveDoH(ctx, server, domain)
}

// buildDoHURL constructs the DoH query URL with appropriate parameters.
// When StripECS is enabled and the server is Google's DoH, it adds
// edns_client_subnet=0.0.0.0/0 to suppress ECS forwarding. For other
// providers, omitting the parameter is sufficient (ECS is opt-in).
func (r *DNSResolver) buildDoHURL(server, domain string) string {
	reqURL := fmt.Sprintf("%s?name=%s&type=A", server, url.QueryEscape(domain))
	if r.config.StripECS && strings.Contains(server, "dns.google") {
		reqURL += "&edns_client_subnet=0.0.0.0/0"
	}
	return reqURL
}

// resolveDoH performs a DNS-over-HTTPS query using the JSON API against the given server URL.
// This is the shared implementation used by both remote and domestic DoH resolution.
func (r *DNSResolver) resolveDoH(ctx context.Context, server, domain string) ([]net.IP, error) {
	// When the multiplexer is active and no injected httpClient is set, use it
	// for persistent connections and query deduplication.
	if r.mux != nil && r.httpClient == nil {
		return r.mux.Query(ctx, server, domain, func(ctx context.Context, client *http.Client) ([]net.IP, error) {
			return r.doDoHRequest(ctx, server, domain, client)
		})
	}
	return r.doDoHRequest(ctx, server, domain, r.httpClient)
}

// doDoHRequest executes a single DoH HTTP request and parses the JSON response.
func (r *DNSResolver) doDoHRequest(ctx context.Context, server, domain string, client *http.Client) ([]net.IP, error) {
	reqURL := r.buildDoHURL(server, domain)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("doh request: %w", err)
	}
	req.Header.Set("Accept", "application/dns-json")

	if client == nil {
		client = &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doh query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("doh status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 65536))
	if err != nil {
		return nil, fmt.Errorf("doh read: %w", err)
	}

	// Parse JSON response
	var dohResp dohResponse
	if err := json.Unmarshal(body, &dohResp); err != nil {
		return nil, fmt.Errorf("doh parse: %w", err)
	}

	var ips []net.IP
	for _, ans := range dohResp.Answer {
		if ans.Type == 1 || ans.Type == 28 { // A or AAAA
			ip := net.ParseIP(ans.Data)
			if ip != nil {
				ips = append(ips, ip)
			}
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("doh: no addresses for %s", domain)
	}
	return ips, nil
}

type dohResponse struct {
	Answer []dohAnswer `json:"Answer"`
}

type dohAnswer struct {
	Type int    `json:"type"`
	Data string `json:"data"`
}

// Cache methods
func (c *dnsCache) get(domain string) ([]net.IP, bool) {
	c.mu.RLock()
	entry, ok := c.entries[domain]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}
	if time.Since(entry.expiresAt) > 0 {
		return nil, false
	}
	entry.lastUsed.Store(time.Now().UnixNano())
	return entry.ips, true
}

func (c *dnsCache) put(domain string, ips []net.IP, ttl time.Duration, domestic bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxSize {
		// Evict expired entries first, then sort the survivors by
		// lastUsed ascending and drop the oldest 10%. The previous
		// version found the LRU entry by linear scan in an inner loop
		// repeated `target` times — O(N²) when nothing was expired
		// (≈10⁷ map iterations at maxSize=10k under sustained pressure).
		now := time.Now()
		target := c.maxSize / 10
		if target < 1 {
			target = 1
		}
		evicted := 0
		for k, v := range c.entries {
			if now.After(v.expiresAt) {
				delete(c.entries, k)
				evicted++
			}
		}
		if evicted < target {
			type kv struct {
				key  string
				nano int64
			}
			survivors := make([]kv, 0, len(c.entries))
			for k, v := range c.entries {
				survivors = append(survivors, kv{k, v.lastUsed.Load()})
			}
			sort.Slice(survivors, func(i, j int) bool {
				return survivors[i].nano < survivors[j].nano
			})
			toEvict := target - evicted
			if toEvict > len(survivors) {
				toEvict = len(survivors)
			}
			for i := 0; i < toEvict; i++ {
				delete(c.entries, survivors[i].key)
			}
		}
	}
	now := time.Now()
	entry := &dnsCacheEntry{
		ips:       ips,
		expiresAt: now.Add(ttl),
		domestic:  domestic,
	}
	entry.lastUsed.Store(now.UnixNano())
	c.entries[domain] = entry
}
