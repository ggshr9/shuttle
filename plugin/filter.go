package plugin

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"sync"
)

// Filter blocks connections to certain domains or IPs.
type Filter struct {
	mu      sync.RWMutex
	blocked map[string]struct{}
	logger  *slog.Logger
}

func NewFilter(blockedDomains []string, logger *slog.Logger) *Filter {
	if logger == nil {
		logger = slog.Default()
	}
	blocked := make(map[string]struct{}, len(blockedDomains))
	for _, d := range blockedDomains {
		blocked[strings.ToLower(d)] = struct{}{}
	}
	return &Filter{blocked: blocked, logger: logger}
}

func (f *Filter) Name() string                  { return "filter" }
func (f *Filter) Init(ctx context.Context) error { return nil }
func (f *Filter) Close() error                   { return nil }

func (f *Filter) OnConnect(conn net.Conn, target string) (net.Conn, error) {
	host, _, _ := net.SplitHostPort(target)
	f.mu.RLock()
	_, blocked := f.blocked[strings.ToLower(host)]
	f.mu.RUnlock()

	if blocked {
		f.logger.Debug("connection blocked", "target", target)
		return nil, &net.DNSError{Err: "blocked by filter", Name: host}
	}
	return conn, nil
}

func (f *Filter) OnDisconnect(conn net.Conn) {}

// AddBlock adds a domain to the blocklist.
func (f *Filter) AddBlock(domain string) {
	f.mu.Lock()
	f.blocked[strings.ToLower(domain)] = struct{}{}
	f.mu.Unlock()
}

// RemoveBlock removes a domain from the blocklist.
func (f *Filter) RemoveBlock(domain string) {
	f.mu.Lock()
	delete(f.blocked, strings.ToLower(domain))
	f.mu.Unlock()
}

var _ ConnPlugin = (*Filter)(nil)
