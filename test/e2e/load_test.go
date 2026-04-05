//go:build sandbox

package e2e

import (
	"io"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSandboxLoad100Concurrent opens 100 concurrent SOCKS5 connections through
// the proxy, each making an HTTP request to httpbin.
func TestSandboxLoad100Concurrent(t *testing.T) {
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	socks5Addr := clientA + ":1080"
	targetURL := "http://" + httpbinAddr + "/ip"

	waitForService(t, socks5Addr, 30*time.Second)

	const concurrency = 100
	var (
		wg        sync.WaitGroup
		successes atomic.Int64
		failures  atomic.Int64
		latencies []time.Duration
		latMu     sync.Mutex
	)

	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			reqStart := time.Now()

			proxyURL, _ := url.Parse("socks5://" + socks5Addr)
			client := &http.Client{
				Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
				Timeout:   30 * time.Second,
			}
			defer client.CloseIdleConnections()

			resp, err := client.Get(targetURL)
			if err != nil {
				failures.Add(1)
				return
			}
			io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode == 200 {
				successes.Add(1)
				latMu.Lock()
				latencies = append(latencies, time.Since(reqStart))
				latMu.Unlock()
			} else {
				failures.Add(1)
			}
		}(i)
	}

	wg.Wait()
	totalDuration := time.Since(start)

	s := successes.Load()
	f := failures.Load()

	t.Logf("Load test: %d/%d succeeded, %d failed, total %v", s, concurrency, f, totalDuration)

	if s < int64(concurrency*90/100) {
		t.Fatalf("expected >= 90%% success rate, got %d/%d", s, concurrency)
	}

	// Calculate percentiles
	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		p50 := latencies[len(latencies)*50/100]
		p95 := latencies[len(latencies)*95/100]
		p99 := latencies[len(latencies)*99/100]
		t.Logf("Latency: p50=%v p95=%v p99=%v", p50, p95, p99)
	}
}

// TestSandboxLoadSustained maintains 50 connections for 30 seconds.
func TestSandboxLoadSustained(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sustained load test in short mode")
	}

	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	socks5Addr := clientA + ":1080"
	targetURL := "http://" + httpbinAddr + "/ip"

	waitForService(t, socks5Addr, 30*time.Second)

	const (
		workers  = 50
		duration = 30 * time.Second
	)

	var (
		total    atomic.Int64
		failures atomic.Int64
	)

	deadline := time.Now().Add(duration)
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			proxyURL, _ := url.Parse("socks5://" + socks5Addr)
			client := &http.Client{
				Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
				Timeout:   10 * time.Second,
			}
			defer client.CloseIdleConnections()

			for time.Now().Before(deadline) {
				resp, err := client.Get(targetURL)
				if err != nil {
					failures.Add(1)
					total.Add(1)
					continue
				}
				io.ReadAll(resp.Body)
				resp.Body.Close()
				total.Add(1)
			}
		}()
	}

	wg.Wait()

	tot := total.Load()
	fail := failures.Load()
	successRate := float64(tot-fail) / float64(tot) * 100

	t.Logf("Sustained load: %d total requests, %d failures, %.1f%% success rate over %v", tot, fail, successRate, duration)

	if successRate < 95 {
		t.Fatalf("expected >= 95%% success rate, got %.1f%%", successRate)
	}
}
