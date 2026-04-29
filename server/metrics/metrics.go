// Package metrics provides lightweight Prometheus-compatible metrics
// exposition for the Shuttle proxy server. It implements the Prometheus
// text exposition format directly without external dependencies.
package metrics

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Default histogram buckets for connection duration (in seconds).
var defaultDurationBuckets = []float64{0.1, 0.5, 1, 5, 10, 30, 60, 300, 600, 3600}

// HandshakeDurationBuckets are the default histogram buckets for handshake latency, in seconds.
var HandshakeDurationBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5}

// DNSQueryDurationBuckets are the default histogram buckets for DNS query latency, in seconds.
var DNSQueryDurationBuckets = []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1}

// Collector gathers server-side metrics for Prometheus exposition.
type Collector struct {
	// Gauges (current values)
	ActiveConns  atomic.Int64
	ActiveStreams atomic.Int64

	// Counters (monotonically increasing)
	TotalConns  atomic.Int64
	TotalStreams atomic.Int64
	BytesIn     atomic.Int64
	BytesOut    atomic.Int64
	AuthFailures atomic.Int64

	// Per-transport counters
	mu             sync.RWMutex
	transportConns map[string]*transportCounter

	// Histogram (connection duration)
	durationBuckets []float64
	durationCounts  []atomic.Int64
	durationSum     atomic.Int64 // stored as nanoseconds
	durationTotal   atomic.Int64

	// Handshake metrics (per-transport histogram + failure counter)
	handshakeDuration *labeledHistogram
	handshakeFailures *labeledCounter

	startTime time.Time

	// Cached MemStats to avoid STW pause on every /metrics scrape.
	memMu        sync.Mutex
	memCache     runtime.MemStats
	memCacheTime atomic.Int64 // unix nano
}

// transportCounter tracks active and total connection counts for a transport.
type transportCounter struct {
	active atomic.Int64
	total  atomic.Int64
}

// NewCollector creates a new metrics collector with default histogram buckets.
func NewCollector() *Collector {
	buckets := make([]float64, len(defaultDurationBuckets))
	copy(buckets, defaultDurationBuckets)

	c := &Collector{
		transportConns:  make(map[string]*transportCounter),
		durationBuckets: buckets,
		durationCounts:  make([]atomic.Int64, len(buckets)),
		startTime:       time.Now(),
	}
	c.handshakeDuration = newLabeledHistogram(
		"shuttle_handshake_duration_seconds",
		HandshakeDurationBuckets,
		[]string{"transport"},
	)
	c.handshakeFailures = newLabeledCounter(
		"shuttle_handshake_failures_total",
		[]string{"transport", "reason"},
	)
	return c
}

// ConnOpened records a new connection for the given transport.
func (c *Collector) ConnOpened(transport string) {
	c.ActiveConns.Add(1)
	c.TotalConns.Add(1)

	tc := c.getOrCreateTransport(transport)
	tc.active.Add(1)
	tc.total.Add(1)
}

// ConnClosed records a connection closure for the given transport,
// and adds the duration to the histogram.
func (c *Collector) ConnClosed(transport string, duration time.Duration) {
	c.ActiveConns.Add(-1)

	tc := c.getOrCreateTransport(transport)
	tc.active.Add(-1)

	// Record duration in histogram
	secs := duration.Seconds()
	for i, bound := range c.durationBuckets {
		if secs <= bound {
			c.durationCounts[i].Add(1)
		}
	}
	c.durationSum.Add(int64(duration))
	c.durationTotal.Add(1)
}

// StreamOpened records a new stream.
func (c *Collector) StreamOpened() {
	c.ActiveStreams.Add(1)
	c.TotalStreams.Add(1)
}

// StreamClosed records a stream closure.
func (c *Collector) StreamClosed() {
	c.ActiveStreams.Add(-1)
}

// RecordBytes adds to the byte counters.
func (c *Collector) RecordBytes(in, out int64) {
	c.BytesIn.Add(in)
	c.BytesOut.Add(out)
}

// RecordAuthFailure increments the auth failure counter.
func (c *Collector) RecordAuthFailure() {
	c.AuthFailures.Add(1)
}

// RecordHandshake records a successful handshake's duration for the given transport.
func (c *Collector) RecordHandshake(transport string, duration time.Duration) {
	c.handshakeDuration.Observe(duration.Seconds(), transport)
}

// RecordHandshakeFailure records a handshake failure with a categorised reason.
// Reason should be one of: timeout, auth, protocol.
func (c *Collector) RecordHandshakeFailure(transport, reason string) {
	c.handshakeFailures.Inc(transport, reason)
}

// Handler returns an http.Handler that serves /metrics in Prometheus text format.
func (c *Collector) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		c.writeMetrics(w)
	})
}

// getMemStats returns cached MemStats, refreshing at most every 10 seconds
// to avoid the STW pause of runtime.ReadMemStats on every /metrics scrape.
func (c *Collector) getMemStats() runtime.MemStats {
	const maxAge = 10 * time.Second
	if time.Since(time.Unix(0, c.memCacheTime.Load())) < maxAge {
		c.memMu.Lock()
		mem := c.memCache
		c.memMu.Unlock()
		return mem
	}
	c.memMu.Lock()
	// Double-check after acquiring lock.
	if time.Since(time.Unix(0, c.memCacheTime.Load())) < maxAge {
		mem := c.memCache
		c.memMu.Unlock()
		return mem
	}
	runtime.ReadMemStats(&c.memCache)
	c.memCacheTime.Store(time.Now().UnixNano())
	mem := c.memCache
	c.memMu.Unlock()
	return mem
}

// getOrCreateTransport returns the counter for a transport, creating it if needed.
func (c *Collector) getOrCreateTransport(name string) *transportCounter {
	c.mu.RLock()
	tc, ok := c.transportConns[name]
	c.mu.RUnlock()
	if ok {
		return tc
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// Double-check after acquiring write lock.
	if tc, ok = c.transportConns[name]; ok {
		return tc
	}
	tc = &transportCounter{}
	c.transportConns[name] = tc
	return tc
}

// writeMetrics writes all metrics in Prometheus text exposition format.
func (c *Collector) writeMetrics(w io.Writer) {
	// --- Gauges ---
	writeMetric(w, "shuttle_active_connections", "gauge",
		"Current number of active connections",
		c.ActiveConns.Load())

	writeMetric(w, "shuttle_active_streams", "gauge",
		"Current number of active streams",
		c.ActiveStreams.Load())

	// --- Counters ---
	writeMetric(w, "shuttle_connections_total", "counter",
		"Total connections since start",
		c.TotalConns.Load())

	writeMetric(w, "shuttle_streams_total", "counter",
		"Total streams since start",
		c.TotalStreams.Load())

	writeMetric(w, "shuttle_bytes_received_total", "counter",
		"Total bytes received from clients",
		c.BytesIn.Load())

	writeMetric(w, "shuttle_bytes_sent_total", "counter",
		"Total bytes sent to clients",
		c.BytesOut.Load())

	writeMetric(w, "shuttle_auth_failures_total", "counter",
		"Total authentication failures",
		c.AuthFailures.Load())

	// --- Per-transport metrics ---
	c.mu.RLock()
	transports := make(map[string]*transportCounter, len(c.transportConns))
	for k, v := range c.transportConns {
		transports[k] = v
	}
	c.mu.RUnlock()

	if len(transports) > 0 {
		fmt.Fprintf(w, "# HELP shuttle_transport_active_connections Current active connections by transport\n")
		fmt.Fprintf(w, "# TYPE shuttle_transport_active_connections gauge\n")
		for name, tc := range transports {
			fmt.Fprintf(w, "shuttle_transport_active_connections{transport=%q} %d\n", name, tc.active.Load())
		}

		fmt.Fprintf(w, "# HELP shuttle_transport_connections_total Total connections by transport\n")
		fmt.Fprintf(w, "# TYPE shuttle_transport_connections_total counter\n")
		for name, tc := range transports {
			fmt.Fprintf(w, "shuttle_transport_connections_total{transport=%q} %d\n", name, tc.total.Load())
		}
	}

	// --- Handshake metrics ---
	c.handshakeDuration.write(w, "Server-side handshake duration in seconds, by transport")
	c.handshakeFailures.write(w, "Total handshake failures, by transport and reason")

	// --- Duration histogram ---
	fmt.Fprintf(w, "# HELP shuttle_connection_duration_seconds Connection duration histogram\n")
	fmt.Fprintf(w, "# TYPE shuttle_connection_duration_seconds histogram\n")
	for i, bound := range c.durationBuckets {
		fmt.Fprintf(w, "shuttle_connection_duration_seconds_bucket{le=\"%s\"} %d\n",
			formatFloat(bound), c.durationCounts[i].Load())
	}
	fmt.Fprintf(w, "shuttle_connection_duration_seconds_bucket{le=\"+Inf\"} %d\n", c.durationTotal.Load())
	durationSumSecs := float64(c.durationSum.Load()) / float64(time.Second)
	fmt.Fprintf(w, "shuttle_connection_duration_seconds_sum %s\n", formatFloat(durationSumSecs))
	fmt.Fprintf(w, "shuttle_connection_duration_seconds_count %d\n", c.durationTotal.Load())

	// --- Server uptime ---
	uptime := time.Since(c.startTime).Seconds()
	fmt.Fprintf(w, "# HELP shuttle_uptime_seconds Time since server start\n")
	fmt.Fprintf(w, "# TYPE shuttle_uptime_seconds gauge\n")
	fmt.Fprintf(w, "shuttle_uptime_seconds %s\n", formatFloat(uptime))

	// --- Go runtime metrics ---
	mem := c.getMemStats()

	writeMetric(w, "shuttle_go_goroutines", "gauge",
		"Number of goroutines",
		int64(runtime.NumGoroutine()))

	writeMetric(w, "shuttle_go_memory_alloc_bytes", "gauge",
		"Allocated heap memory in bytes",
		int64(mem.Alloc)) //nolint:gosec // G115: heap alloc bytes won't exceed int64 max

	writeMetric(w, "shuttle_go_memory_sys_bytes", "gauge",
		"Total system memory in bytes",
		int64(mem.Sys)) //nolint:gosec // G115: system memory bytes won't exceed int64 max

	writeMetric(w, "shuttle_go_gc_completed_total", "counter",
		"Total completed GC cycles",
		int64(mem.NumGC))
}

// writeMetric writes a single metric with HELP, TYPE, and value lines.
func writeMetric(w io.Writer, name, metricType, help string, value int64) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n%s %d\n",
		name, help, name, metricType, name, value)
}

// formatFloat formats a float64 for Prometheus output, avoiding scientific notation.
func formatFloat(f float64) string {
	if f == math.Inf(1) {
		return "+Inf"
	}
	if f == math.Inf(-1) {
		return "-Inf"
	}
	// Use enough precision but trim trailing zeros
	s := fmt.Sprintf("%.6f", f)
	// Trim trailing zeros after decimal point
	for len(s) > 1 && s[len(s)-1] == '0' && s[len(s)-2] != '.' {
		s = s[:len(s)-1]
	}
	return s
}
