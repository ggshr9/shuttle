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
	"net/url"
	"strings"
	"sync"
	"time"
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
}

// DNSResolver implements split DNS with anti-pollution.
type DNSResolver struct {
	config     *DNSConfig
	cache      *dnsCache
	geoIP      *GeoIPDB
	logger     *slog.Logger
	httpClient *http.Client       // injectable HTTP client for DoH (nil = default)
	mux        *DNSMultiplexer    // persistent connection pool + dedup (nil when disabled)
}

type dnsCache struct {
	mu      sync.RWMutex
	entries map[string]*dnsCacheEntry
	maxSize int
}

type dnsCacheEntry struct {
	ips       []net.IP
	expiresAt time.Time
	domestic  bool // resolved via domestic DNS
	lastUsed  time.Time
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
	return &DNSResolver{
		config: cfg,
		cache: &dnsCache{
			entries: make(map[string]*dnsCacheEntry, cfg.CacheSize),
			maxSize: cfg.CacheSize,
		},
		geoIP:  geoIP,
		logger: logger,
		mux:    mux,
	}
}

// Close releases resources held by the resolver.
func (r *DNSResolver) Close() {
	if r.mux != nil {
		r.mux.Close()
	}
}

// Resolve resolves a domain using the anti-pollution strategy:
// 1. Query both domestic and remote DNS
// 2. If results agree → use domestic (faster)
// 3. If they disagree → check if domestic result is foreign IP (pollution)
// 4. If polluted → use remote result
func (r *DNSResolver) Resolve(ctx context.Context, domain string) ([]net.IP, error) {
	// Check cache first
	if ips, ok := r.cache.get(domain); ok {
		return ips, nil
	}

	// Query both DNS servers in parallel
	type result struct {
		ips []net.IP
		err error
	}
	domCh := make(chan result, 1)
	remCh := make(chan result, 1)

	go func() {
		ips, err := r.queryDomestic(ctx, domain)
		domCh <- result{ips, err}
	}()
	go func() {
		ips, err := r.queryRemote(ctx, domain)
		remCh <- result{ips, err}
	}()

	var domResult, remResult result

	select {
	case domResult = <-domCh:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case remResult = <-remCh:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return r.selectResult(domain, domResult.ips, domResult.err, remResult.ips, remResult.err)
}

func (r *DNSResolver) selectResult(domain string, domIPs []net.IP, domErr error, remIPs []net.IP, remErr error) ([]net.IP, error) {
	// If only one succeeded, use that
	if domErr != nil && remErr != nil {
		return nil, fmt.Errorf("both DNS queries failed: domestic=%v, remote=%v", domErr, remErr)
	}
	if domErr != nil {
		r.cache.put(domain, remIPs, r.config.CacheTTL, false)
		return remIPs, nil
	}
	if remErr != nil {
		r.cache.put(domain, domIPs, r.config.CacheTTL, true)
		return domIPs, nil
	}

	// Both succeeded — check for pollution
	if r.ipsMatch(domIPs, remIPs) {
		// Results agree — use domestic (faster)
		r.cache.put(domain, domIPs, r.config.CacheTTL, true)
		return domIPs, nil
	}

	// Results disagree — check if domestic result looks polluted
	if r.isPolluted(domIPs) {
		r.logger.Debug("DNS pollution detected",
			"domain", domain,
			"domestic", domIPs,
			"remote", remIPs)
		r.cache.put(domain, remIPs, r.config.CacheTTL, false)
		return remIPs, nil
	}

	// Use domestic result (probably a CDN with region-specific IPs)
	r.cache.put(domain, domIPs, r.config.CacheTTL, true)
	return domIPs, nil
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
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[domain]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.entries, domain)
		return nil, false
	}
	entry.lastUsed = time.Now()
	return entry.ips, true
}

func (c *dnsCache) put(domain string, ips []net.IP, ttl time.Duration, domestic bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxSize {
		// Evict expired entries first, then oldest up to 10% of max
		now := time.Now()
		evicted := 0
		target := c.maxSize / 10
		if target < 1 {
			target = 1
		}
		for k, v := range c.entries {
			if now.After(v.expiresAt) {
				delete(c.entries, k)
				evicted++
			}
		}
		// Evict least recently used entries if not enough expired entries were removed
		for evicted < target {
			var oldestKey string
			var oldestTime time.Time
			first := true
			for k, v := range c.entries {
				if first || v.lastUsed.Before(oldestTime) {
					oldestKey = k
					oldestTime = v.lastUsed
					first = false
				}
			}
			if oldestKey == "" {
				break
			}
			delete(c.entries, oldestKey)
			evicted++
		}
	}
	now := time.Now()
	c.entries[domain] = &dnsCacheEntry{
		ips:       ips,
		expiresAt: now.Add(ttl),
		domestic:  domestic,
		lastUsed:  now,
	}
}
