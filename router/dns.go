package router

import (
	"context"
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
}

// DNSResolver implements split DNS with anti-pollution.
type DNSResolver struct {
	config *DNSConfig
	cache  *dnsCache
	geoIP  *GeoIPDB
	logger *slog.Logger
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
	return &DNSResolver{
		config: cfg,
		cache: &dnsCache{
			entries: make(map[string]*dnsCacheEntry, cfg.CacheSize),
			maxSize: cfg.CacheSize,
		},
		geoIP:  geoIP,
		logger: logger,
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

func (r *DNSResolver) isPolluted(ips []net.IP) bool {
	if r.geoIP == nil {
		return false
	}
	for _, ip := range ips {
		country := r.geoIP.LookupCountry(ip)
		if country != "" && country != "CN" {
			// Domestic DNS returned a foreign IP — likely polluted
			return true
		}
	}
	return false
}

func (r *DNSResolver) queryDomestic(ctx context.Context, domain string) ([]net.IP, error) {
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

func (r *DNSResolver) queryRemote(ctx context.Context, domain string) ([]net.IP, error) {
	// Use DNS-over-HTTPS (DoH) with JSON API
	server := r.config.RemoteServer
	if server == "" {
		server = "https://1.1.1.1/dns-query"
	}
	// Ensure it's an HTTPS URL
	if !strings.HasPrefix(server, "https://") {
		// Legacy plain DNS config — upgrade to DoH
		server = "https://1.1.1.1/dns-query"
	}

	reqURL := fmt.Sprintf("%s?name=%s&type=A", server, url.QueryEscape(domain))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("doh request: %w", err)
	}
	req.Header.Set("Accept", "application/dns-json")

	client := &http.Client{Timeout: 5 * time.Second}
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
	defer c.mu.RUnlock()
	entry, ok := c.entries[domain]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.ips, true
}

func (c *dnsCache) put(domain string, ips []net.IP, ttl time.Duration, domestic bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxSize {
		// Evict oldest entries (simple approach)
		count := 0
		for k, v := range c.entries {
			if time.Now().After(v.expiresAt) || count > c.maxSize/10 {
				delete(c.entries, k)
			}
			count++
		}
	}
	c.entries[domain] = &dnsCacheEntry{
		ips:       ips,
		expiresAt: time.Now().Add(ttl),
		domestic:  domestic,
	}
}
