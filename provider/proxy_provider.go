package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/shuttleX/shuttle/server"
)

// ProxyProviderConfig holds configuration for a ProxyProvider.
type ProxyProviderConfig struct {
	// Name is a human-readable identifier for this provider.
	Name string
	// URL is the remote URL to fetch the proxy list from.
	URL string
	// Path is the local file path used to cache the fetched data.
	Path string
	// Interval is how often to re-fetch. Defaults to 1 hour if zero.
	Interval time.Duration
	// Filter is an optional regexp string; only nodes whose Name matches are kept.
	Filter string
}

// ProxyProvider fetches, caches, and exposes a list of ProxyNode values
// from a remote URL. It supports Clash YAML, sing-box JSON, plain URI, and
// base64-encoded URI formats via auto-detection.
type ProxyProvider struct {
	name     string
	url      string
	path     string
	interval time.Duration
	filter   *regexp.Regexp
	client   *http.Client

	mu        sync.RWMutex
	nodes     []ProxyNode
	updatedAt time.Time
	lastErr   error
	cancel    context.CancelFunc
}

// NewProxyProvider creates a ProxyProvider from cfg, compiles the optional
// filter regexp, and loads cached data from cfg.Path if it exists.
func NewProxyProvider(cfg ProxyProviderConfig) (*ProxyProvider, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("provider URL must not be empty")
	}
	if !strings.HasPrefix(cfg.URL, "http://") && !strings.HasPrefix(cfg.URL, "https://") {
		return nil, fmt.Errorf("provider URL must use http or https scheme")
	}
	if cfg.Interval <= 0 {
		cfg.Interval = time.Hour
	}

	var filter *regexp.Regexp
	if cfg.Filter != "" {
		var err error
		filter, err = regexp.Compile(cfg.Filter)
		if err != nil {
			return nil, fmt.Errorf("invalid filter regexp %q: %w", cfg.Filter, err)
		}
	}

	p := &ProxyProvider{
		name:     cfg.Name,
		url:      cfg.URL,
		path:     cfg.Path,
		interval: cfg.Interval,
		filter:   filter,
		client: server.NewSafeHTTPClient(server.SafeHTTPClientOptions{
			Timeout:      30 * time.Second,
			MaxRedirects: 5,
		}),
	}

	// Load from cache if available
	if cfg.Path != "" {
		if data, err := os.ReadFile(cfg.Path); err == nil {
			if nodes, err := ParseProxyList(data); err == nil {
				p.nodes = p.applyFilter(nodes)
				p.updatedAt = fileModTime(cfg.Path)
			}
		}
	}

	return p, nil
}

// Start performs an initial fetch if no cached nodes exist, then schedules
// periodic refreshes. The context controls the lifetime of the background loop.
func (p *ProxyProvider) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	p.mu.Lock()
	p.cancel = cancel
	hasNodes := len(p.nodes) > 0
	p.mu.Unlock()

	if !hasNodes {
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
func (p *ProxyProvider) Stop() {
	p.mu.Lock()
	cancel := p.cancel
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Refresh fetches the remote URL, parses the result, applies the filter, and
// updates the cached nodes. It also writes the raw response body to p.path
// when a cache path is configured.
func (p *ProxyProvider) Refresh(ctx context.Context) error {
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

	nodes, err := ParseProxyList(body)
	if err != nil {
		return p.setErr(fmt.Errorf("parse proxy list: %w", err))
	}

	filtered := p.applyFilter(nodes)

	// Persist cache
	if p.path != "" {
		if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err == nil {
			_ = os.WriteFile(p.path, body, 0o600)
		}
	}

	p.mu.Lock()
	p.nodes = filtered
	p.updatedAt = time.Now()
	p.lastErr = nil
	p.mu.Unlock()
	return nil
}

// Nodes returns a copy of the current node slice.
func (p *ProxyProvider) Nodes() []ProxyNode {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]ProxyNode, len(p.nodes))
	copy(out, p.nodes)
	return out
}

// Name returns the provider name.
func (p *ProxyProvider) Name() string { return p.name }

// Error returns the last error encountered during Refresh, or nil.
func (p *ProxyProvider) Error() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastErr
}

// UpdatedAt returns the time of the last successful refresh.
func (p *ProxyProvider) UpdatedAt() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.updatedAt
}

// ── internal helpers ──────────────────────────────────────────────────────────

// applyFilter returns the subset of nodes whose Name matches the filter
// regexp, or all nodes if no filter is set.
func (p *ProxyProvider) applyFilter(nodes []ProxyNode) []ProxyNode {
	if p.filter == nil {
		return nodes
	}
	out := make([]ProxyNode, 0, len(nodes))
	for _, n := range nodes {
		if p.filter.MatchString(n.Name) {
			out = append(out, n)
		}
	}
	return out
}

// setErr records err as the last error and returns it.
func (p *ProxyProvider) setErr(err error) error {
	p.mu.Lock()
	p.lastErr = err
	p.mu.Unlock()
	return err
}

// fileModTime returns the modification time of path, or zero on error.
func fileModTime(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}
