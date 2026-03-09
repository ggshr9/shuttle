package speedtest

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

// TestResult represents the result of testing a single server.
type TestResult struct {
	ServerAddr string        `json:"server_addr"`
	ServerName string        `json:"server_name,omitempty"`
	Latency    time.Duration `json:"latency_ms"`
	LatencyMs  int64         `json:"latency"` // for JSON
	Available  bool          `json:"available"`
	Error      string        `json:"error,omitempty"`
}

// TestConfig configures the speedtest behavior.
type TestConfig struct {
	Timeout     time.Duration
	Concurrency int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *TestConfig {
	return &TestConfig{
		Timeout:     5 * time.Second,
		Concurrency: 10,
	}
}

// Server represents a server to test.
type Server struct {
	Addr     string
	Name     string
	Password string
	SNI      string
}

// Tester performs latency tests on servers.
type Tester struct {
	cfg *TestConfig
}

// NewTester creates a new speedtest tester.
func NewTester(cfg *TestConfig) *Tester {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Tester{cfg: cfg}
}

// TestAll tests all provided servers concurrently.
func (t *Tester) TestAll(ctx context.Context, servers []Server) []TestResult {
	results := make([]TestResult, len(servers))
	var wg sync.WaitGroup

	// Use semaphore for concurrency control
	sem := make(chan struct{}, t.cfg.Concurrency)

	for i, srv := range servers {
		wg.Add(1)
		go func(idx int, server Server) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := t.testOne(ctx, server)
			results[idx] = result
		}(i, srv)
	}

	wg.Wait()
	return results
}

// TestAllStream tests servers and sends results as they complete.
func (t *Tester) TestAllStream(ctx context.Context, servers []Server, resultCh chan<- TestResult) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, t.cfg.Concurrency)

	for _, srv := range servers {
		wg.Add(1)
		go func(server Server) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := t.testOne(ctx, server)
			select {
			case resultCh <- result:
			case <-ctx.Done():
			}
		}(srv)
	}

	wg.Wait()
	close(resultCh)
}

// testOne tests a single server.
func (t *Tester) testOne(ctx context.Context, srv Server) TestResult {
	result := TestResult{
		ServerAddr: srv.Addr,
		ServerName: srv.Name,
	}

	ctx, cancel := context.WithTimeout(ctx, t.cfg.Timeout)
	defer cancel()

	start := time.Now()
	latency, err := t.measureLatency(ctx, srv)
	if err != nil {
		result.Available = false
		result.Error = err.Error()
		return result
	}

	result.Latency = latency
	result.LatencyMs = latency.Milliseconds()
	result.Available = true

	// If basic TCP succeeded but took too long, still mark as available but note it
	if time.Since(start) > t.cfg.Timeout {
		result.Error = "timeout exceeded"
	}

	return result
}

// measureLatency attempts to connect to the server and measure round-trip time.
// It tries TCP first; if that fails it falls back to a UDP probe (for QUIC servers).
func (t *Tester) measureLatency(ctx context.Context, srv Server) (time.Duration, error) {
	host, port, err := net.SplitHostPort(srv.Addr)
	if err != nil {
		// Assume port 443 if not specified
		host = srv.Addr
		port = "443"
	}

	// Resolve DNS first (not counted in latency)
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return 0, fmt.Errorf("dns: %w", err)
	}
	if len(ips) == 0 {
		return 0, fmt.Errorf("dns: no addresses found")
	}

	addr := net.JoinHostPort(ips[0].String(), port)

	// Measure TCP connect time
	start := time.Now()
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		// TCP failed — try QUIC handshake (server may be QUIC-only)
		quicLatency, quicErr := t.measureQUICLatency(ctx, addr, srv)
		if quicErr != nil {
			return 0, fmt.Errorf("tcp: %w; quic: %v", err, quicErr)
		}
		return quicLatency, nil
	}
	tcpLatency := time.Since(start)
	defer conn.Close()

	// If port is 443, also measure TLS handshake
	if port == "443" {
		serverName := srv.SNI
		if serverName == "" {
			serverName = host
		}

		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: true, // We just want to measure connectivity
		})

		start = time.Now()
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			// TLS failed but TCP succeeded - return TCP latency
			return tcpLatency, nil
		}
		tlsLatency := time.Since(start)

		// Return combined latency (more representative of real usage)
		return tcpLatency + tlsLatency, nil
	}

	return tcpLatency, nil
}

// measureQUICLatency performs a real QUIC handshake to measure server latency.
func (t *Tester) measureQUICLatency(ctx context.Context, addr string, srv Server) (time.Duration, error) {
	host, _, _ := net.SplitHostPort(addr)
	serverName := srv.SNI
	if serverName == "" {
		serverName = host
	}

	tlsConf := &tls.Config{
		ServerName:         serverName,
		NextProtos:         []string{"h3"},
		InsecureSkipVerify: true, //nolint:gosec // just measuring latency
	}

	start := time.Now()
	qconn, err := quic.DialAddr(ctx, addr, tlsConf, &quic.Config{
		MaxIdleTimeout:  t.cfg.Timeout,
		KeepAlivePeriod: 0,
	})
	if err != nil {
		return 0, fmt.Errorf("quic: %w", err)
	}
	latency := time.Since(start)
	qconn.CloseWithError(0, "speedtest")
	return latency, nil
}

// SortByLatency sorts results by latency (available servers first, then by latency).
func SortByLatency(results []TestResult) {
	sort.Slice(results, func(i, j int) bool {
		// Available servers come first
		if results[i].Available != results[j].Available {
			return results[i].Available
		}
		// Among available servers, sort by latency
		if results[i].Available {
			return results[i].Latency < results[j].Latency
		}
		// Among unavailable servers, maintain order
		return false
	})
}
