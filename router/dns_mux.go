package router

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"
)

// DNSMultiplexer provides persistent HTTP/2 connection pooling and query
// deduplication for DNS-over-HTTPS resolution.
type DNSMultiplexer struct {
	clients  map[string]*http.Client // keyed by DoH server URL
	inflight sync.Map                // domain → *inflightQuery (dedup)
	mu       sync.RWMutex
}

// inflightQuery tracks a DNS query that is currently in progress so that
// concurrent lookups for the same domain can share the result.
type inflightQuery struct {
	done   chan struct{}
	result []net.IP
	err    error
}

// NewDNSMultiplexer creates a new multiplexer with an empty client pool.
func NewDNSMultiplexer() *DNSMultiplexer {
	return &DNSMultiplexer{
		clients: make(map[string]*http.Client),
	}
}

// Query performs a DoH lookup for domain via the given server URL. If an
// identical query is already in-flight, the caller blocks until the first
// query completes and receives the same result (deduplication).
//
// The provided queryFn performs the actual HTTP request and DNS response
// parsing; the multiplexer only handles connection reuse and dedup.
func (m *DNSMultiplexer) Query(ctx context.Context, url, domain string, queryFn func(ctx context.Context, client *http.Client) ([]net.IP, error)) ([]net.IP, error) {
	key := url + "\x00" + domain

	// Fast path: an identical query is already running — wait for it.
	if v, loaded := m.inflight.Load(key); loaded {
		q := v.(*inflightQuery)
		select {
		case <-q.done:
			return q.result, q.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Slow path: register ourselves as the in-flight query.
	q := &inflightQuery{done: make(chan struct{})}
	if actual, loaded := m.inflight.LoadOrStore(key, q); loaded {
		// Another goroutine beat us to it.
		q = actual.(*inflightQuery)
		select {
		case <-q.done:
			return q.result, q.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// We own this query — execute it.
	client := m.getOrCreateClient(url)
	q.result, q.err = queryFn(ctx, client)
	close(q.done)
	m.inflight.Delete(key)

	return q.result, q.err
}

// getOrCreateClient returns a persistent HTTP/2 client for the given DoH
// server URL, creating one if it does not yet exist.
func (m *DNSMultiplexer) getOrCreateClient(serverURL string) *http.Client {
	m.mu.RLock()
	c, ok := m.clients[serverURL]
	m.mu.RUnlock()
	if ok {
		return c
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Double-check after acquiring write lock.
	if c, ok = m.clients[serverURL]; ok {
		return c
	}
	c = &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        2,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     120 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
	m.clients[serverURL] = c
	return c
}

// Close closes idle connections on all pooled HTTP clients.
func (m *DNSMultiplexer) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.clients {
		c.CloseIdleConnections()
	}
	m.clients = make(map[string]*http.Client)
}
