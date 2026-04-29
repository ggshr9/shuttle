package provider

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ggshr9/shuttle/server"
)

// RuleProviderConfig holds configuration for a RuleProvider.
type RuleProviderConfig struct {
	// Name is a human-readable identifier for this provider.
	Name string
	// URL is the remote URL to fetch the rule list from.
	URL string
	// Path is the local file path used to cache the fetched data.
	Path string
	// Behavior controls how lines are parsed: "domain", "ipcidr", or "classical".
	Behavior string
	// Interval is how often to re-fetch. Defaults to 1 hour if zero.
	Interval time.Duration
}

// ruleSet holds the parsed rules for atomic hot-reload.
type ruleSet struct {
	domains        map[string]bool // exact domain match (lower-cased)
	domainSuffixes []string        // suffix match entries like ".example.com"
	domainKeywords []string        // keyword match entries (lower-cased)
	cidrs          []*net.IPNet
}

// RuleProvider fetches, caches, and exposes a set of routing rules
// from a remote URL. It supports domain, ipcidr, and classical behavior modes.
type RuleProvider struct {
	name     string
	url      string
	path     string
	behavior string
	interval time.Duration
	client   *http.Client

	rules     atomic.Pointer[ruleSet] // atomic swap for hot-reload
	mu        sync.Mutex              // protects metadata (updatedAt, lastErr, cancel)
	updatedAt time.Time
	lastErr   error
	cancel    context.CancelFunc
}

// NewRuleProvider creates a RuleProvider from cfg, validates behavior,
// sets defaults, and loads any existing cached data from cfg.Path.
func NewRuleProvider(cfg RuleProviderConfig) (*RuleProvider, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("rule provider URL must not be empty")
	}
	if !strings.HasPrefix(cfg.URL, "http://") && !strings.HasPrefix(cfg.URL, "https://") {
		return nil, fmt.Errorf("rule provider URL must use http or https scheme")
	}

	switch cfg.Behavior {
	case "domain", "ipcidr", "classical":
		// valid
	case "":
		return nil, fmt.Errorf("rule provider behavior must not be empty")
	default:
		return nil, fmt.Errorf("unknown rule provider behavior %q: must be domain, ipcidr, or classical", cfg.Behavior)
	}

	if cfg.Interval <= 0 {
		cfg.Interval = time.Hour
	}

	p := &RuleProvider{
		name:     cfg.Name,
		url:      cfg.URL,
		path:     cfg.Path,
		behavior: cfg.Behavior,
		interval: cfg.Interval,
		client: server.NewSafeHTTPClient(server.SafeHTTPClientOptions{
			Timeout:      30 * time.Second,
			MaxRedirects: 5,
		}),
	}

	// Load from cache if available.
	if cfg.Path != "" {
		if data, err := os.ReadFile(cfg.Path); err == nil {
			if rs, err := parseRuleSet(cfg.Behavior, data); err == nil {
				p.rules.Store(rs)
				p.updatedAt = fileModTime(cfg.Path)
			}
		}
	}

	return p, nil
}

// Start performs an initial fetch if no rules are cached, then schedules
// periodic refreshes. The context controls the background loop lifetime.
func (p *RuleProvider) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	p.mu.Lock()
	p.cancel = cancel
	empty := p.rules.Load() == nil
	p.mu.Unlock()

	if empty {
		_ = p.Refresh(ctx)
	}

	go func() {
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = p.Refresh(ctx)
			}
		}
	}()
}

// Stop cancels the background refresh loop.
func (p *RuleProvider) Stop() {
	p.mu.Lock()
	cancel := p.cancel
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Refresh fetches the remote URL, parses it according to the configured
// behavior, atomically stores the new rule set, and caches the raw body.
func (p *RuleProvider) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return p.setErr(fmt.Errorf("create request: %w", err))
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return p.setErr(fmt.Errorf("fetch %s: %w", p.url, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return p.setErr(fmt.Errorf("fetch %s: HTTP %d", p.url, resp.StatusCode))
	}

	const maxBytes = 10 << 20 // 10 MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return p.setErr(fmt.Errorf("read body: %w", err))
	}

	rs, err := parseRuleSet(p.behavior, body)
	if err != nil {
		return p.setErr(fmt.Errorf("parse rule set: %w", err))
	}

	// Persist cache.
	if p.path != "" {
		if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err == nil {
			_ = os.WriteFile(p.path, body, 0o600)
		}
	}

	p.rules.Store(rs)
	p.mu.Lock()
	p.updatedAt = time.Now()
	p.lastErr = nil
	p.mu.Unlock()
	return nil
}

// MatchDomain reports whether domain matches any rule in the current rule set.
// Matching is case-insensitive and checks exact match, suffix match, and keyword match.
func (p *RuleProvider) MatchDomain(domain string) bool {
	rs := p.rules.Load()
	if rs == nil {
		return false
	}
	lower := strings.ToLower(domain)

	// Exact match.
	if rs.domains[lower] {
		return true
	}

	// Suffix match: ".example.com" matches "sub.example.com" and "example.com".
	for _, suffix := range rs.domainSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
		// Also match the bare domain itself (without leading dot).
		if lower == suffix[1:] {
			return true
		}
	}

	// Keyword match.
	for _, kw := range rs.domainKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}

	return false
}

// MatchIP reports whether ipStr falls within any CIDR in the current rule set.
func (p *RuleProvider) MatchIP(ipStr string) bool {
	rs := p.rules.Load()
	if rs == nil {
		return false
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, cidr := range rs.cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// Name returns the provider name.
func (p *RuleProvider) Name() string { return p.name }

// Behavior returns the provider behavior ("domain", "ipcidr", or "classical").
func (p *RuleProvider) Behavior() string { return p.behavior }

// UpdatedAt returns the time of the last successful refresh.
func (p *RuleProvider) UpdatedAt() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.updatedAt
}

// Error returns the last error encountered during Refresh, or nil.
func (p *RuleProvider) Error() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastErr
}

// ── internal helpers ──────────────────────────────────────────────────────────

// setErr records err as the last error and returns it.
func (p *RuleProvider) setErr(err error) error {
	p.mu.Lock()
	p.lastErr = err
	p.mu.Unlock()
	return err
}

// parseRuleSet parses raw bytes into a ruleSet according to the given behavior.
func parseRuleSet(behavior string, data []byte) (*ruleSet, error) {
	rs := &ruleSet{
		domains: make(map[string]bool),
	}

	lines := strings.Split(string(data), "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		switch behavior {
		case "domain":
			parseDomainLine(rs, line)
		case "ipcidr":
			if err := parseCIDRLine(rs, line); err != nil {
				// Skip malformed CIDRs rather than aborting the whole parse.
				continue
			}
		case "classical":
			parseClassicalLine(rs, line)
		}
	}

	return rs, nil
}

// parseDomainLine adds line as both an exact domain match and a suffix match.
func parseDomainLine(rs *ruleSet, line string) {
	domain := strings.ToLower(line)
	rs.domains[domain] = true
	suffix := "." + domain
	rs.domainSuffixes = append(rs.domainSuffixes, suffix)
}

// parseCIDRLine parses a CIDR string and appends to rs.cidrs.
func parseCIDRLine(rs *ruleSet, line string) error {
	_, ipnet, err := net.ParseCIDR(line)
	if err != nil {
		return err
	}
	rs.cidrs = append(rs.cidrs, ipnet)
	return nil
}

// parseClassicalLine handles "RULE_TYPE,value" lines for classical behavior.
func parseClassicalLine(rs *ruleSet, line string) {
	// Strip trailing comma-separated options (e.g. ",no-resolve").
	parts := strings.SplitN(line, ",", 3)
	if len(parts) < 2 {
		return
	}
	ruleType := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if value == "" {
		return
	}

	switch strings.ToUpper(ruleType) {
	case "DOMAIN":
		lower := strings.ToLower(value)
		rs.domains[lower] = true
	case "DOMAIN-SUFFIX":
		lower := strings.ToLower(value)
		suffix := "." + lower
		rs.domainSuffixes = append(rs.domainSuffixes, suffix)
	case "DOMAIN-KEYWORD":
		rs.domainKeywords = append(rs.domainKeywords, strings.ToLower(value))
	case "IP-CIDR", "IP-CIDR6":
		if _, ipnet, err := net.ParseCIDR(value); err == nil {
			rs.cidrs = append(rs.cidrs, ipnet)
		}
	}
}
