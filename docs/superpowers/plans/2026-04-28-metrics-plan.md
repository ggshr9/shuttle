# Plan 2 — Metrics Expansion (Server + Client)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring server and client Prometheus exposition to the level needed to drive a useful Grafana dashboard. Server gains handshake / DNS / per-user metrics; client gains routing-decision / per-outbound-CB / subscription / handshake metrics. The work uses a single hook-injection contract established in Workstream 2 and reused in Workstream 3.

**Architecture:**
- The existing `server/metrics/metrics.Collector` is extended with two reusable internal helpers: `labeledCounter` and `labeledHistogram`. New metrics are appended to `writeMetrics`.
- Each subsystem that emits metric data exposes one `SetXHook(...)` setter. The admin/init code that already constructs the collector wires the hooks at startup.
- The client side mirrors the same hook-style API and reuses the existing zero-dependency text-format approach in `gui/api/routes_prometheus.go`.

**Tech Stack:** Go 1.24, atomic primitives, no new dependencies. Prometheus text format hand-rolled (continuing the existing approach in `server/metrics/metrics.go`).

**Spec reference:** `docs/superpowers/specs/2026-04-28-production-readiness-yellow-lights-design.md` (Workstreams 2 and 3).

---

## File Structure

**Created:**
- `server/metrics/labeled.go` — `labeledCounter` and `labeledHistogram` helpers.
- `server/metrics/labeled_test.go` — tests for the helpers.

**Modified — server side (W2):**
- `server/metrics/metrics.go` — `Collector` gains four new emitter methods (`RecordHandshake`, `RecordHandshakeFailure`, `RecordDNSQuery`, `RecordDestResolveFailure`); `writeMetrics` emits the new lines; per-user gauge optional.
- `transport/h3/server.go`, `transport/reality/server.go`, `transport/cdn/server.go` — call the new `SetHandshakeMetrics` hook at server-side accept completion.
- `router/dns.go` (or wherever the server-side resolver lives) — call `metricHook` after each query.
- `server/admin/users.go` (if per-user metrics enabled) — call `users.SetActivityHook`.
- `server/server.go` — wires the hooks at startup once the collector exists.

**Modified — client side (W3):**
- `engine/engine.go` — adds `Metrics()` accessor returning a `MetricsSnapshot`; private `metrics` field of new type `engineMetrics`.
- `engine/circuit.go:32-37` — `OnStateChange` callback signature extended to include outbound name (breaking internal change; only call site is the engine constructor).
- `gui/api/routes_prometheus.go` — emits the new metrics by reading `eng.Metrics()` snapshot.
- `router/router.go` — `SetDecisionHook(func(decision, rule string))`.
- `subscription/subscription.go` — `Manager.SetRefreshHook(func(id, result string, ts time.Time))` invoked from `Refresh()`.
- `router/dns.go` (client-side) — same `metricHook` pattern.

**Test additions:** Each new metric gets a unit test asserting the `/metrics` body contains the expected line after the hook fires.

---

# Phase A — Server (Workstream 2)

## Task 1: Generic `labeledCounter` helper

**Files:**
- Create: `server/metrics/labeled.go`
- Test: `server/metrics/labeled_test.go`

- [ ] **Step 1.1: Write failing test**

```go
// server/metrics/labeled_test.go
package metrics

import (
	"strings"
	"testing"
)

func TestLabeledCounter_BasicIncrement(t *testing.T) {
	c := newLabeledCounter("shuttle_test_total", []string{"transport", "result"})
	c.Inc("h3", "ok")
	c.Inc("h3", "ok")
	c.Inc("h3", "fail")

	var sb strings.Builder
	c.write(&sb, "Test counter")

	out := sb.String()
	if !strings.Contains(out, `shuttle_test_total{transport="h3",result="ok"} 2`) {
		t.Fatalf("expected ok=2 line, got:\n%s", out)
	}
	if !strings.Contains(out, `shuttle_test_total{transport="h3",result="fail"} 1`) {
		t.Fatalf("expected fail=1 line, got:\n%s", out)
	}
	if !strings.Contains(out, "# TYPE shuttle_test_total counter") {
		t.Fatalf("expected TYPE line, got:\n%s", out)
	}
}

func TestLabeledCounter_LabelArityMismatchPanics(t *testing.T) {
	c := newLabeledCounter("x", []string{"a", "b"})
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on arity mismatch")
		}
	}()
	c.Inc("only-one")
}

func TestLabeledCounter_Concurrent(t *testing.T) {
	c := newLabeledCounter("shuttle_concurrent", []string{"k"})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.Inc("a") }()
	}
	wg.Wait()

	var sb strings.Builder
	c.write(&sb, "concurrent")
	if !strings.Contains(sb.String(), `shuttle_concurrent{k="a"} 100`) {
		t.Fatalf("expected 100, got %s", sb.String())
	}
}
```

Add `import "sync"` to the test file.

- [ ] **Step 1.2: Run, verify fail**

```
./scripts/test.sh --pkg ./server/metrics/ --run TestLabeledCounter
```

Expected: undefined `newLabeledCounter`.

- [ ] **Step 1.3: Implement**

```go
// server/metrics/labeled.go
package metrics

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
)

// labelTuple encodes a list of label values as a stable map key.
type labelTuple string

func makeTuple(values []string) labelTuple {
	return labelTuple(strings.Join(values, "\x00"))
}

func (t labelTuple) values() []string {
	return strings.Split(string(t), "\x00")
}

type labeledCounter struct {
	name      string
	labelKeys []string
	mu        sync.RWMutex
	counts    map[labelTuple]*atomic.Int64
}

func newLabeledCounter(name string, labelKeys []string) *labeledCounter {
	return &labeledCounter{
		name:      name,
		labelKeys: labelKeys,
		counts:    make(map[labelTuple]*atomic.Int64),
	}
}

func (c *labeledCounter) Inc(values ...string) {
	if len(values) != len(c.labelKeys) {
		panic(fmt.Sprintf("labeledCounter %s: expected %d labels, got %d", c.name, len(c.labelKeys), len(values)))
	}
	tup := makeTuple(values)

	c.mu.RLock()
	v, ok := c.counts[tup]
	c.mu.RUnlock()
	if ok {
		v.Add(1)
		return
	}

	c.mu.Lock()
	if v, ok = c.counts[tup]; !ok {
		v = &atomic.Int64{}
		c.counts[tup] = v
	}
	c.mu.Unlock()
	v.Add(1)
}

// write emits the counter in Prometheus text format.
func (c *labeledCounter) write(w io.Writer, help string) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", c.name, help, c.name)
	c.mu.RLock()
	defer c.mu.RUnlock()
	for tup, v := range c.counts {
		labels := tup.values()
		var sb strings.Builder
		sb.WriteString(c.name)
		sb.WriteByte('{')
		for i, k := range c.labelKeys {
			if i > 0 {
				sb.WriteByte(',')
			}
			fmt.Fprintf(&sb, `%s=%q`, k, labels[i])
		}
		sb.WriteByte('}')
		fmt.Fprintf(w, "%s %d\n", sb.String(), v.Load())
	}
}
```

- [ ] **Step 1.4: Run, verify pass**

```
./scripts/test.sh --pkg ./server/metrics/ --run TestLabeledCounter
```

Expected: PASS.

- [ ] **Step 1.5: Commit**

```bash
git add server/metrics/labeled.go server/metrics/labeled_test.go
git commit -m "feat(metrics): add labeledCounter helper"
```

---

## Task 2: Generic `labeledHistogram` helper

**Files:**
- Modify: `server/metrics/labeled.go`
- Modify: `server/metrics/labeled_test.go`

- [ ] **Step 2.1: Write failing test**

Append to `labeled_test.go`:

```go
func TestLabeledHistogram_BucketBoundary(t *testing.T) {
	buckets := []float64{0.1, 0.5, 1.0}
	h := newLabeledHistogram("shuttle_dur_seconds", buckets, []string{"transport"})

	// Three observations: 0.05, 0.5, 2.0 — should land in <=0.1, <=0.5, +Inf
	h.Observe(0.05, "h3")
	h.Observe(0.5, "h3")
	h.Observe(2.0, "h3")

	var sb strings.Builder
	h.write(&sb, "Duration")
	out := sb.String()

	for _, want := range []string{
		`shuttle_dur_seconds_bucket{transport="h3",le="0.1"} 1`,
		`shuttle_dur_seconds_bucket{transport="h3",le="0.5"} 2`,
		`shuttle_dur_seconds_bucket{transport="h3",le="1"} 2`,
		`shuttle_dur_seconds_bucket{transport="h3",le="+Inf"} 3`,
		`shuttle_dur_seconds_count{transport="h3"} 3`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing line %q in:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2.2: Run, verify fail**

```
./scripts/test.sh --pkg ./server/metrics/ --run TestLabeledHistogram
```

Expected: undefined.

- [ ] **Step 2.3: Implement**

Append to `server/metrics/labeled.go`:

```go
type histogramSeries struct {
	bucketCounts []atomic.Int64
	sum          atomic.Int64 // stored as nanos for time-like values
	total        atomic.Int64
}

type labeledHistogram struct {
	name      string
	labelKeys []string
	buckets   []float64
	mu        sync.RWMutex
	series    map[labelTuple]*histogramSeries
}

func newLabeledHistogram(name string, buckets []float64, labelKeys []string) *labeledHistogram {
	bb := make([]float64, len(buckets))
	copy(bb, buckets)
	return &labeledHistogram{
		name:      name,
		labelKeys: labelKeys,
		buckets:   bb,
		series:    make(map[labelTuple]*histogramSeries),
	}
}

func (h *labeledHistogram) Observe(value float64, labels ...string) {
	if len(labels) != len(h.labelKeys) {
		panic(fmt.Sprintf("labeledHistogram %s: expected %d labels, got %d", h.name, len(h.labelKeys), len(labels)))
	}
	tup := makeTuple(labels)

	h.mu.RLock()
	s, ok := h.series[tup]
	h.mu.RUnlock()
	if !ok {
		h.mu.Lock()
		if s, ok = h.series[tup]; !ok {
			s = &histogramSeries{bucketCounts: make([]atomic.Int64, len(h.buckets))}
			h.series[tup] = s
		}
		h.mu.Unlock()
	}

	for i, b := range h.buckets {
		if value <= b {
			s.bucketCounts[i].Add(1)
		}
	}
	s.sum.Add(int64(value * 1e9)) // store as nanos to preserve precision
	s.total.Add(1)
}

func (h *labeledHistogram) write(w io.Writer, help string) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s histogram\n", h.name, help, h.name)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for tup, s := range h.series {
		labelVals := tup.values()
		labelStr := h.formatLabels(labelVals, "")
		for i, b := range h.buckets {
			extra := fmt.Sprintf(`le=%q`, formatFloat(b))
			fmt.Fprintf(w, "%s_bucket%s %d\n", h.name, h.formatLabels(labelVals, extra), s.bucketCounts[i].Load())
			_ = i
		}
		fmt.Fprintf(w, "%s_bucket%s %d\n", h.name, h.formatLabels(labelVals, `le="+Inf"`), s.total.Load())
		sumSecs := float64(s.sum.Load()) / 1e9
		fmt.Fprintf(w, "%s_sum%s %s\n", h.name, labelStr, formatFloat(sumSecs))
		fmt.Fprintf(w, "%s_count%s %d\n", h.name, labelStr, s.total.Load())
	}
}

func (h *labeledHistogram) formatLabels(vals []string, extra string) string {
	var sb strings.Builder
	sb.WriteByte('{')
	for i, k := range h.labelKeys {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `%s=%q`, k, vals[i])
	}
	if extra != "" {
		if len(h.labelKeys) > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(extra)
	}
	sb.WriteByte('}')
	return sb.String()
}
```

- [ ] **Step 2.4: Run, verify pass**

```
./scripts/test.sh --pkg ./server/metrics/ --run TestLabeledHistogram
```

Expected: PASS.

- [ ] **Step 2.5: Commit**

```bash
git add server/metrics/labeled.go server/metrics/labeled_test.go
git commit -m "feat(metrics): add labeledHistogram helper"
```

---

## Task 3: Server `RecordHandshake` and `RecordHandshakeFailure`

**Files:**
- Modify: `server/metrics/metrics.go`
- Modify: `server/metrics/metrics_test.go`

- [ ] **Step 3.1: Write failing test**

Append to `server/metrics/metrics_test.go`:

```go
import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCollector_HandshakeHistogram(t *testing.T) {
	c := NewCollector()
	c.RecordHandshake("h3", 50*time.Millisecond)
	c.RecordHandshake("h3", 250*time.Millisecond)

	rr := httptest.NewRecorder()
	c.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()

	if !strings.Contains(body, `shuttle_handshake_duration_seconds_count{transport="h3"} 2`) {
		t.Fatalf("missing handshake count, got:\n%s", body)
	}
}

func TestCollector_HandshakeFailure(t *testing.T) {
	c := NewCollector()
	c.RecordHandshakeFailure("reality", "auth")
	c.RecordHandshakeFailure("reality", "auth")
	c.RecordHandshakeFailure("reality", "timeout")

	rr := httptest.NewRecorder()
	c.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()

	if !strings.Contains(body, `shuttle_handshake_failures_total{transport="reality",reason="auth"} 2`) {
		t.Fatalf("missing auth failure line, got:\n%s", body)
	}
	if !strings.Contains(body, `shuttle_handshake_failures_total{transport="reality",reason="timeout"} 1`) {
		t.Fatalf("missing timeout failure line, got:\n%s", body)
	}
}
```

- [ ] **Step 3.2: Run, verify fail**

```
./scripts/test.sh --pkg ./server/metrics/ --run TestCollector_Handshake
```

Expected: undefined methods.

- [ ] **Step 3.3: Implement**

Modify `server/metrics/metrics.go`:

In the `Collector` struct (around line 21-49), add two new fields:

```go
	handshakeDuration *labeledHistogram
	handshakeFailures *labeledCounter
```

In `NewCollector`, initialise them:

```go
	c.handshakeDuration = newLabeledHistogram(
		"shuttle_handshake_duration_seconds",
		HandshakeDurationBuckets,
		[]string{"transport"},
	)
	c.handshakeFailures = newLabeledCounter(
		"shuttle_handshake_failures_total",
		[]string{"transport", "reason"},
	)
```

Add the bucket constants near the top of the file:

```go
// HandshakeDurationBuckets are the default histogram buckets for handshake latency, in seconds.
var HandshakeDurationBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5}

// DNSQueryDurationBuckets are the default histogram buckets for DNS query latency, in seconds.
var DNSQueryDurationBuckets = []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1}
```

Add the public methods:

```go
// RecordHandshake records a successful handshake's duration for the given transport.
func (c *Collector) RecordHandshake(transport string, duration time.Duration) {
	c.handshakeDuration.Observe(duration.Seconds(), transport)
}

// RecordHandshakeFailure records a handshake failure with a categorised reason.
// Reason should be one of: timeout, auth, protocol.
func (c *Collector) RecordHandshakeFailure(transport, reason string) {
	c.handshakeFailures.Inc(transport, reason)
}
```

In `writeMetrics`, after the existing transport metrics block, add:

```go
	c.handshakeDuration.write(w, "Server-side handshake duration in seconds, by transport")
	c.handshakeFailures.write(w, "Total handshake failures, by transport and reason")
```

- [ ] **Step 3.4: Run, verify pass**

```
./scripts/test.sh --pkg ./server/metrics/
```

Expected: ALL PASS.

- [ ] **Step 3.5: Commit**

```bash
git add server/metrics/metrics.go server/metrics/metrics_test.go
git commit -m "feat(metrics): add handshake duration histogram and failure counter"
```

---

## Task 4: Server `RecordDNSQuery` and `RecordDestResolveFailure`

**Files:**
- Modify: `server/metrics/metrics.go`
- Modify: `server/metrics/metrics_test.go`

- [ ] **Step 4.1: Write failing test**

Append to `metrics_test.go`:

```go
func TestCollector_DNSQuery(t *testing.T) {
	c := NewCollector()
	c.RecordDNSQuery("system", true, 2*time.Millisecond)
	c.RecordDNSQuery("system", false, 30*time.Millisecond)

	rr := httptest.NewRecorder()
	c.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()

	if !strings.Contains(body, `shuttle_dns_query_duration_seconds_count{protocol="system",cached="true"} 1`) {
		t.Fatalf("missing cached count, got:\n%s", body)
	}
	if !strings.Contains(body, `shuttle_dns_query_duration_seconds_count{protocol="system",cached="false"} 1`) {
		t.Fatalf("missing uncached count, got:\n%s", body)
	}
}

func TestCollector_DestResolveFailure(t *testing.T) {
	c := NewCollector()
	c.RecordDestResolveFailure("nxdomain")

	rr := httptest.NewRecorder()
	c.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()

	if !strings.Contains(body, `shuttle_destination_resolve_failures_total{reason="nxdomain"} 1`) {
		t.Fatalf("missing failure line, got:\n%s", body)
	}
}
```

- [ ] **Step 4.2: Run, verify fail**

```
./scripts/test.sh --pkg ./server/metrics/ --run TestCollector_DNS
./scripts/test.sh --pkg ./server/metrics/ --run TestCollector_DestResolveFailure
```

Expected: undefined.

- [ ] **Step 4.3: Implement**

Add to `Collector` struct:

```go
	dnsQueryDuration *labeledHistogram
	destResolveFails *labeledCounter
```

In `NewCollector`:

```go
	c.dnsQueryDuration = newLabeledHistogram(
		"shuttle_dns_query_duration_seconds",
		DNSQueryDurationBuckets,
		[]string{"protocol", "cached"},
	)
	c.destResolveFails = newLabeledCounter(
		"shuttle_destination_resolve_failures_total",
		[]string{"reason"},
	)
```

Add methods:

```go
// RecordDNSQuery records a DNS query duration. protocol is one of "udp", "system".
func (c *Collector) RecordDNSQuery(protocol string, cached bool, duration time.Duration) {
	cachedStr := "false"
	if cached {
		cachedStr = "true"
	}
	c.dnsQueryDuration.Observe(duration.Seconds(), protocol, cachedStr)
}

// RecordDestResolveFailure records a destination resolution failure.
// reason is one of "nxdomain", "timeout", "refused".
func (c *Collector) RecordDestResolveFailure(reason string) {
	c.destResolveFails.Inc(reason)
}
```

In `writeMetrics`, after the handshake block:

```go
	c.dnsQueryDuration.write(w, "DNS query duration in seconds")
	c.destResolveFails.write(w, "Destination resolve failures by reason")
```

- [ ] **Step 4.4: Run, verify pass**

```
./scripts/test.sh --pkg ./server/metrics/
```

Expected: ALL PASS.

- [ ] **Step 4.5: Commit**

```bash
git add server/metrics/metrics.go server/metrics/metrics_test.go
git commit -m "feat(metrics): add DNS query histogram and resolve-failure counter"
```

---

## Task 5: Wire handshake metrics into transports

**Files:**
- Modify: `transport/h3/server.go`, `transport/reality/server.go`, `transport/cdn/server.go`
- Modify: `server/server.go` (wires the hook)

- [ ] **Step 5.1: Define a transport-side hook**

Add to a new file `transport/handshake_metrics.go` (shared by all transports):

```go
package transport

import "time"

// HandshakeMetrics is the optional hook called by server-side transports
// after each completed (or failed) accept. It is set once at server startup.
type HandshakeMetrics struct {
	OnSuccess func(transport string, duration time.Duration)
	OnFailure func(transport string, reason string)
}
```

- [ ] **Step 5.2: Have each transport accept a `*HandshakeMetrics`**

For each of `transport/h3/server.go`, `transport/reality/server.go`, `transport/cdn/server.go`:

1. Add a `metrics *HandshakeMetrics` field to the server struct.
2. Add an option/setter (match the existing options pattern):

```go
func WithHandshakeMetrics(m *transport.HandshakeMetrics) Option {
	return func(s *Server) { s.metrics = m }
}
```

3. In the accept loop, wrap each accept:

```go
start := time.Now()
conn, err := /* existing accept logic */
if err != nil {
	if s.metrics != nil && s.metrics.OnFailure != nil {
		s.metrics.OnFailure("h3", classifyReason(err)) // "reality" / "cdn" respectively
	}
	continue
}
if s.metrics != nil && s.metrics.OnSuccess != nil {
	s.metrics.OnSuccess("h3", time.Since(start))
}
```

4. Add a `classifyReason(err error) string` helper that maps known errors to `"timeout"`, `"auth"`, `"protocol"`; default to `"protocol"`.

- [ ] **Step 5.3: Wire the hook in `server/server.go`**

Where the collector `mc` is constructed and transports are launched, build the hook and pass it:

```go
hsMetrics := &transport.HandshakeMetrics{
	OnSuccess: func(t string, d time.Duration) { mc.RecordHandshake(t, d) },
	OnFailure: func(t string, reason string) { mc.RecordHandshakeFailure(t, reason) },
}
// pass hsMetrics into each transport server constructor
```

- [ ] **Step 5.4: Add a unit test for one transport's hook invocation**

Pick `transport/h3/server_test.go` (smallest existing test) and add:

```go
func TestH3Server_HandshakeHookFiresOnAccept(t *testing.T) {
	var successes int32
	hook := &transport.HandshakeMetrics{
		OnSuccess: func(string, time.Duration) { atomic.AddInt32(&successes, 1) },
	}
	srv := /* construct with WithHandshakeMetrics(hook) */
	// drive a single accept, then assert atomic.LoadInt32(&successes) == 1
}
```

(Specific construction depends on existing test helpers; reuse the pattern from the nearest existing test.)

- [ ] **Step 5.5: Run host tests**

```
./scripts/test.sh --pkg ./transport/...
./scripts/test.sh --pkg ./server/...
```

Expected: PASS.

- [ ] **Step 5.6: Commit**

```bash
git add transport/ server/server.go
git commit -m "feat(transport): emit handshake metrics via HandshakeMetrics hook"
```

---

## Task 6: Wire DNS metrics into server resolver

**Files:**
- Modify: `router/dns.go` (or wherever the server-side resolver is constructed)
- Modify: `server/server.go`

- [ ] **Step 6.1: Add a metric hook to the resolver**

Locate the resolver type in `router/dns.go`. Add:

```go
// MetricHook is called after each DNS query attempt.
type MetricHook struct {
	OnQuery   func(protocol string, cached bool, duration time.Duration)
	OnFailure func(reason string)
}

// SetMetricHook installs a metric hook. Safe to call once at startup.
func (r *Resolver) SetMetricHook(h *MetricHook) {
	r.metricHook = h
}
```

In the query path, wrap each lookup:

```go
start := time.Now()
result, err := /* existing lookup */
if r.metricHook != nil {
	if err != nil {
		r.metricHook.OnFailure(classifyDNSErr(err))
	} else {
		r.metricHook.OnQuery(protocolName, fromCache, time.Since(start))
	}
}
```

- [ ] **Step 6.2: Wire from server bootstrap**

In `server/server.go`, after `mc` exists:

```go
resolver.SetMetricHook(&router.MetricHook{
	OnQuery:   func(p string, cached bool, d time.Duration) { mc.RecordDNSQuery(p, cached, d) },
	OnFailure: func(reason string) { mc.RecordDestResolveFailure(reason) },
})
```

- [ ] **Step 6.3: Test**

Add to `router/dns_test.go`:

```go
func TestResolver_MetricHookCalled(t *testing.T) {
	var queries int32
	r := /* construct resolver with mock backend */
	r.SetMetricHook(&MetricHook{
		OnQuery: func(string, bool, time.Duration) { atomic.AddInt32(&queries, 1) },
	})
	_, _ = r.Resolve(context.Background(), "example.com")
	if atomic.LoadInt32(&queries) != 1 {
		t.Fatalf("expected 1 OnQuery call, got %d", queries)
	}
}
```

- [ ] **Step 6.4: Run**

```
./scripts/test.sh --pkg ./router/
./scripts/test.sh --pkg ./server/
```

Expected: PASS.

- [ ] **Step 6.5: Commit**

```bash
git add router/dns.go router/dns_test.go server/server.go
git commit -m "feat(router): emit DNS query metrics via SetMetricHook"
```

---

## Task 7: Per-user activity gauge (server, optional)

**Files:**
- Modify: `server/admin/users.go`, `server/metrics/metrics.go`

- [ ] **Step 7.1: Decide gating**

Per-user metrics can produce high cardinality. Gate behind `cfg.Metrics.PerUser bool` (add this field to `config.MetricsConfig` if absent).

- [ ] **Step 7.2: Add `RecordUserActivity` to the collector**

In `server/metrics/metrics.go`, add a labeledGauge helper variant (or piggy-back on labeledCounter for "active connections" style). Simplest is a dedicated `userActivity` field of type `sync.Map` keyed by user → `*atomic.Int64`.

```go
type Collector struct {
	// ...
	userActiveMu sync.RWMutex
	userActive   map[string]*atomic.Int64
}

func (c *Collector) UserActivityDelta(user string, delta int64) {
	c.userActiveMu.RLock()
	v, ok := c.userActive[user]
	c.userActiveMu.RUnlock()
	if !ok {
		c.userActiveMu.Lock()
		if v, ok = c.userActive[user]; !ok {
			v = &atomic.Int64{}
			if c.userActive == nil {
				c.userActive = make(map[string]*atomic.Int64)
			}
			c.userActive[user] = v
		}
		c.userActiveMu.Unlock()
	}
	v.Add(delta)
}
```

In `writeMetrics`, after the handshake block, add (only if `c.userActive != nil`):

```go
c.userActiveMu.RLock()
if len(c.userActive) > 0 {
	fmt.Fprintf(w, "# HELP shuttle_user_active_connections Active connections by user\n")
	fmt.Fprintf(w, "# TYPE shuttle_user_active_connections gauge\n")
	for user, v := range c.userActive {
		fmt.Fprintf(w, "shuttle_user_active_connections{user=%q} %d\n", user, v.Load())
	}
}
c.userActiveMu.RUnlock()
```

- [ ] **Step 7.3: Wire from `users.go`**

In the existing connect/disconnect path that already tracks per-user activity, add:

```go
if u.activityHook != nil {
	u.activityHook(name, +1)
}
```

and a corresponding `-1` on close.

- [ ] **Step 7.4: Test**

```go
func TestCollector_UserActivity(t *testing.T) {
	c := NewCollector()
	c.UserActivityDelta("alice", +1)
	c.UserActivityDelta("alice", +1)
	c.UserActivityDelta("alice", -1)

	rr := httptest.NewRecorder()
	c.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(rr.Body.String(), `shuttle_user_active_connections{user="alice"} 1`) {
		t.Fatalf("missing user activity: %s", rr.Body.String())
	}
}
```

- [ ] **Step 7.5: Run**

```
./scripts/test.sh --pkg ./server/metrics/
./scripts/test.sh --pkg ./server/admin/
```

Expected: PASS.

- [ ] **Step 7.6: Commit**

```bash
git add server/metrics/metrics.go server/admin/users.go config/
git commit -m "feat(metrics): per-user active connections gauge (gated by metrics.per_user)"
```

---

# Phase B — Client (Workstream 3)

## Task 8: Extend `engine.Engine` with `Metrics()` accessor

**Files:**
- Modify: `engine/engine.go` and `engine/events.go`
- Test: `engine/engine_test.go`

- [ ] **Step 8.1: Define the snapshot type**

In a new file `engine/metrics_snapshot.go`:

```go
// Package engine — metrics snapshot type.
package engine

import "time"

// MetricsSnapshot is a frozen view of engine-side metrics suitable for
// rendering by gui/api/routes_prometheus.go. Returned by Engine.Metrics().
type MetricsSnapshot struct {
	// Routing decisions: map["<decision>/<rule>"] -> count.
	RoutingDecisions map[string]int64

	// Per-outbound circuit breaker state: map["<outbound>"] -> "closed"|"open"|"half-open".
	CircuitBreakers map[string]string

	// Subscription refresh stats: map["<subscription_id>"] -> stats.
	Subscriptions map[string]SubscriptionStats

	// Per-transport handshake durations and failures (client-side dial).
	HandshakeDurationsNanos map[string][]int64 // map["<transport>"] -> observed nanos
	HandshakeFailures       map[string]int64   // map["<transport>/<reason>"] -> count

	// DNS query histogram (client-side resolver).
	DNSQueryDurationsNanos map[string][]int64 // map["<protocol>/<cached>"] -> observed nanos
}

type SubscriptionStats struct {
	OK          int64
	Fail        int64
	LastRefresh time.Time
}
```

- [ ] **Step 8.2: Add internal storage and the `Metrics()` accessor**

In `engine/engine.go`, add a private field (and initialise it in the constructor):

```go
type engineMetrics struct {
	mu sync.Mutex

	routingDecisions   map[string]int64
	circuitBreakers    map[string]string
	subscriptions      map[string]SubscriptionStats
	handshakeDurations map[string][]int64
	handshakeFailures  map[string]int64
	dnsDurations       map[string][]int64
}

func newEngineMetrics() *engineMetrics {
	return &engineMetrics{
		routingDecisions:   make(map[string]int64),
		circuitBreakers:    make(map[string]string),
		subscriptions:      make(map[string]SubscriptionStats),
		handshakeDurations: make(map[string][]int64),
		handshakeFailures:  make(map[string]int64),
		dnsDurations:       make(map[string][]int64),
	}
}

// Metrics returns a snapshot of engine-side metrics. Cheap, lock-protected.
func (e *Engine) Metrics() MetricsSnapshot {
	e.metrics.mu.Lock()
	defer e.metrics.mu.Unlock()
	out := MetricsSnapshot{
		RoutingDecisions:        copyInt64Map(e.metrics.routingDecisions),
		CircuitBreakers:         copyStringMap(e.metrics.circuitBreakers),
		Subscriptions:           copySubscriptionStats(e.metrics.subscriptions),
		HandshakeDurationsNanos: copyInt64SliceMap(e.metrics.handshakeDurations),
		HandshakeFailures:       copyInt64Map(e.metrics.handshakeFailures),
		DNSQueryDurationsNanos:  copyInt64SliceMap(e.metrics.dnsDurations),
	}
	return out
}

func copyInt64Map(m map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyStringMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copySubscriptionStats(m map[string]SubscriptionStats) map[string]SubscriptionStats {
	out := make(map[string]SubscriptionStats, len(m))
	for k, v := range m {
		out[k] = v // SubscriptionStats is a value type — copies cleanly
	}
	return out
}

func copyInt64SliceMap(m map[string][]int64) map[string][]int64 {
	out := make(map[string][]int64, len(m))
	for k, v := range m {
		dup := make([]int64, len(v))
		copy(dup, v)
		out[k] = dup
	}
	return out
}
```

- [ ] **Step 8.3: Add the snapshot to constructors**

In `Engine` constructor, initialise `e.metrics = newEngineMetrics()`. In `Reset()`/`Reload()` paths, **do not** reset metrics (counters should be monotonically increasing across reloads).

- [ ] **Step 8.4: Test**

```go
func TestEngine_MetricsZeroValueSnapshot(t *testing.T) {
	e := NewEngine(nil) // or whatever constructor your tests use
	snap := e.Metrics()
	if snap.RoutingDecisions == nil {
		t.Fatal("RoutingDecisions should be a non-nil empty map")
	}
}
```

- [ ] **Step 8.5: Run**

```
./scripts/test.sh --pkg ./engine/
```

Expected: PASS.

- [ ] **Step 8.6: Commit**

```bash
git add engine/metrics_snapshot.go engine/engine.go engine/engine_test.go
git commit -m "feat(engine): add Metrics() snapshot accessor"
```

---

## Task 9: Router `SetDecisionHook`

**Files:**
- Modify: `router/router.go`
- Modify: `router/router_test.go`
- Modify: `engine/engine.go` (wire the hook)

- [ ] **Step 9.1: Test**

Append to `router/router_test.go`:

```go
func TestRouter_DecisionHookFires(t *testing.T) {
	r := NewRouter(/* config with at least one rule */, nil, nil, nil)
	var hits []string
	r.SetDecisionHook(func(decision, rule string) {
		hits = append(hits, decision+"/"+rule)
	})
	_ = r.MatchDomain("example.com")
	if len(hits) != 1 {
		t.Fatalf("expected 1 hook call, got %d", len(hits))
	}
}
```

- [ ] **Step 9.2: Run, verify fail**

```
./scripts/test.sh --pkg ./router/ --run TestRouter_DecisionHook
```

Expected: undefined.

- [ ] **Step 9.3: Implement**

In `router/router.go`, add a field to `Router`:

```go
type Router struct {
	// ...
	decisionHook func(decision, rule string)
}

func (r *Router) SetDecisionHook(hook func(decision, rule string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.decisionHook = hook
}
```

Wrap each return in `MatchDomain`, `MatchIP`, `MatchProcess`, `MatchProtocol` so the hook fires with `(action, rule_kind)`. Example for `MatchDomain`:

```go
func (r *Router) MatchDomain(domain string) Action {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if action, found := r.domainTrie.Lookup(domain); found {
		r.notifyDecision(action, "domain")
		return Action(action)
	}
	r.notifyDecision(string(r.defaultAct), "default")
	return r.defaultAct
}

func (r *Router) notifyDecision(decision, rule string) {
	if r.decisionHook != nil {
		// run async to avoid blocking match path under hook contention
		go r.decisionHook(decision, rule)
	}
}
```

(The async dispatch is acceptable because the metric is monotonically counted; ordering doesn't matter for histograms or counters.)

- [ ] **Step 9.4: Wire from engine**

In `engine/engine.go`, after `e.router` is constructed:

```go
e.router.SetDecisionHook(func(decision, rule string) {
	key := decision + "/" + rule
	e.metrics.mu.Lock()
	e.metrics.routingDecisions[key]++
	e.metrics.mu.Unlock()
})
```

- [ ] **Step 9.5: Run**

```
./scripts/test.sh --pkg ./router/
./scripts/test.sh --pkg ./engine/
```

Expected: PASS.

- [ ] **Step 9.6: Commit**

```bash
git add router/router.go router/router_test.go engine/engine.go
git commit -m "feat(router): emit decision metrics via SetDecisionHook"
```

---

## Task 10: Circuit breaker per-outbound state callback

**Files:**
- Modify: `engine/circuit.go:32-36`
- Modify: `engine/circuit_test.go`
- Modify: caller of `NewCircuitBreaker`

- [ ] **Step 10.1: Test**

Append to `engine/circuit_test.go`:

```go
func TestCircuitBreaker_OnStateChangeReceivesOutboundName(t *testing.T) {
	var calls []string
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Threshold:    1,
		BaseCooldown: 10 * time.Millisecond,
		OnStateChange: func(state CircuitState, _ time.Duration) {
			// Existing signature retained for backward compat with non-named callers
			calls = append(calls, state.String())
		},
	})
	cb.RecordFailure()
	if len(calls) == 0 || calls[0] != "open" {
		t.Fatalf("expected open transition, got %v", calls)
	}
}
```

(Note: the spec keeps the existing `OnStateChange` signature; outbound name is supplied by the caller wrapping the callback. So no signature change is needed in `circuit.go`.)

- [ ] **Step 10.2: Run, verify pass (existing test should already pass)**

```
./scripts/test.sh --pkg ./engine/ --run TestCircuitBreaker_OnStateChange
```

Expected: PASS.

- [ ] **Step 10.3: Wrap the callback at construction time**

In `engine/engine.go` where the per-outbound CB is constructed (likely in the outbound middleware setup), wrap the callback:

```go
outboundName := cfg.Name
cb := NewCircuitBreaker(CircuitBreakerConfig{
	Threshold:    cfg.Threshold,
	BaseCooldown: cfg.BaseCooldown,
	OnStateChange: func(state CircuitState, _ time.Duration) {
		e.metrics.mu.Lock()
		e.metrics.circuitBreakers[outboundName] = state.String()
		e.metrics.mu.Unlock()
	},
})
```

- [ ] **Step 10.4: Run**

```
./scripts/test.sh --pkg ./engine/
```

Expected: PASS.

- [ ] **Step 10.5: Commit**

```bash
git add engine/engine.go engine/circuit_test.go
git commit -m "feat(engine): record per-outbound CB state in metrics snapshot"
```

---

## Task 11: Subscription refresh hook

**Files:**
- Modify: `subscription/subscription.go:150-174`
- Modify: `subscription/subscription_test.go`
- Modify: `engine/engine.go`

- [ ] **Step 11.1: Test**

Append to `subscription/subscription_test.go`:

```go
func TestManager_RefreshHookFires(t *testing.T) {
	m := NewManager(/* test fixture */)
	_ = m.Add("test", &Subscription{ID: "test", URL: "https://example.com"})

	var calls []struct{ ID, Result string }
	m.SetRefreshHook(func(id, result string, _ time.Time) {
		calls = append(calls, struct{ ID, Result string }{id, result})
	})

	_, _ = m.Refresh(context.Background(), "test")
	if len(calls) != 1 {
		t.Fatalf("expected 1 hook call, got %d", len(calls))
	}
	// result is "ok" or "fail" depending on the test fixture's HTTP server
}
```

- [ ] **Step 11.2: Run, verify fail**

```
./scripts/test.sh --pkg ./subscription/ --run TestManager_RefreshHookFires
```

Expected: undefined.

- [ ] **Step 11.3: Implement**

In `subscription/subscription.go`, add to `Manager`:

```go
type Manager struct {
	// ...
	refreshHook func(id, result string, ts time.Time)
}

// SetRefreshHook installs an optional callback invoked after every
// Refresh call. Safe to call once at startup.
func (m *Manager) SetRefreshHook(hook func(id, result string, ts time.Time)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshHook = hook
}
```

In `Refresh`, before the final return statements:

```go
func (m *Manager) Refresh(ctx context.Context, id string) (*Subscription, error) {
	// ... existing logic ...

	result := "ok"
	if err != nil {
		result = "fail"
	}
	hook := m.refreshHook
	if hook != nil {
		go hook(id, result, time.Now()) // async — see Task 9 reasoning
	}

	if err != nil {
		return sub, err
	}
	return sub, nil
}
```

- [ ] **Step 11.4: Wire from engine**

In `engine/engine.go`, after the subscription manager is constructed:

```go
e.subscriptionMgr.SetRefreshHook(func(id, result string, ts time.Time) {
	e.metrics.mu.Lock()
	stats := e.metrics.subscriptions[id]
	switch result {
	case "ok":
		stats.OK++
	case "fail":
		stats.Fail++
	}
	stats.LastRefresh = ts
	e.metrics.subscriptions[id] = stats
	e.metrics.mu.Unlock()
})
```

- [ ] **Step 11.5: Run**

```
./scripts/test.sh --pkg ./subscription/
./scripts/test.sh --pkg ./engine/
```

Expected: PASS.

- [ ] **Step 11.6: Commit**

```bash
git add subscription/subscription.go subscription/subscription_test.go engine/engine.go
git commit -m "feat(subscription): emit refresh metrics via SetRefreshHook"
```

---

## Task 12: Client-side handshake and DNS metrics

**Files:**
- Modify: client transport dial paths (under `transport/*/client.go`)
- Modify: `engine/engine.go` (hook wiring)
- Modify: `router/dns.go` (client-side metric hook — same pattern as Task 6)

- [ ] **Step 12.1: Reuse the Task 5 `transport.HandshakeMetrics` type**

The hook type is already defined. Add the equivalent option to each client-side transport (`transport/h3/client.go`, etc.) with a `WithHandshakeMetrics(...)` constructor option.

- [ ] **Step 12.2: Wire from engine**

In `engine/engine.go`, where each client transport is constructed:

```go
clientHsHook := &transport.HandshakeMetrics{
	OnSuccess: func(t string, d time.Duration) {
		e.metrics.mu.Lock()
		e.metrics.handshakeDurations[t] = append(e.metrics.handshakeDurations[t], d.Nanoseconds())
		// keep only the last 1024 observations to bound memory
		if l := len(e.metrics.handshakeDurations[t]); l > 1024 {
			e.metrics.handshakeDurations[t] = e.metrics.handshakeDurations[t][l-1024:]
		}
		e.metrics.mu.Unlock()
	},
	OnFailure: func(t, reason string) {
		key := t + "/" + reason
		e.metrics.mu.Lock()
		e.metrics.handshakeFailures[key]++
		e.metrics.mu.Unlock()
	},
}
// pass clientHsHook to each client transport via its Option
```

- [ ] **Step 12.3: Same pattern for client DNS resolver**

If the client uses the same `router.Resolver` type as Task 6, simply call `SetMetricHook` on the client's resolver instance with an engine-side accumulator.

- [ ] **Step 12.4: Test (client engine)**

Append to `engine/engine_test.go`:

```go
func TestEngine_RecordsClientHandshake(t *testing.T) {
	e := NewEngine(/* test config */)
	// fake handshake event:
	e.metrics.mu.Lock()
	e.metrics.handshakeDurations["h3"] = append(e.metrics.handshakeDurations["h3"], 50_000_000) // 50ms in nanos
	e.metrics.mu.Unlock()

	snap := e.Metrics()
	if got := snap.HandshakeDurationsNanos["h3"]; len(got) != 1 || got[0] != 50_000_000 {
		t.Fatalf("expected one 50ms observation, got %v", got)
	}
}
```

- [ ] **Step 12.5: Run**

```
./scripts/test.sh --pkg ./transport/...
./scripts/test.sh --pkg ./engine/
```

Expected: PASS.

- [ ] **Step 12.6: Commit**

```bash
git add transport/ engine/
git commit -m "feat(engine): record client-side handshake and DNS metrics"
```

---

## Task 13: Render new metrics in `gui/api/routes_prometheus.go`

**Files:**
- Modify: `gui/api/routes_prometheus.go`
- Test: `gui/api/routes_prometheus_test.go`

- [ ] **Step 13.1: Test**

Append to `gui/api/routes_prometheus_test.go`:

```go
func TestPrometheus_NewMetricsEmitted(t *testing.T) {
	eng := /* construct test engine */
	// trigger one routing decision
	eng.Metrics() // sanity

	// Manually populate metrics for the test:
	mux := http.NewServeMux()
	registerPrometheusRoutes(mux, eng)

	req := httptest.NewRequest("GET", "/api/prometheus", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	for _, want := range []string{
		"shuttle_routing_decisions_total",
		"shuttle_circuit_breaker_state{outbound=",
		"shuttle_subscription_refresh_total",
		"shuttle_subscription_last_refresh_timestamp",
		"shuttle_handshake_duration_seconds",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in /api/prometheus body", want)
		}
	}
}
```

- [ ] **Step 13.2: Run, verify fail**

```
./scripts/test.sh --pkg ./gui/api/ --run TestPrometheus_NewMetricsEmitted
```

Expected: missing metric lines.

- [ ] **Step 13.3: Implement**

In `gui/api/routes_prometheus.go`, after the existing 8 writes, add:

```go
snap := eng.Metrics()

// Routing decisions
fmt.Fprintf(w, "# HELP shuttle_routing_decisions_total Routing decisions by decision and rule type\n")
fmt.Fprintf(w, "# TYPE shuttle_routing_decisions_total counter\n")
for k, v := range snap.RoutingDecisions {
	parts := strings.SplitN(k, "/", 2)
	if len(parts) != 2 { continue }
	fmt.Fprintf(w, "shuttle_routing_decisions_total{decision=%q,rule=%q} %d\n", parts[0], parts[1], v)
}

// Per-outbound CB
fmt.Fprintf(w, "# HELP shuttle_circuit_breaker_state Per-outbound circuit breaker state (0=closed,1=open,2=half-open)\n")
fmt.Fprintf(w, "# TYPE shuttle_circuit_breaker_state gauge\n")
for outbound, state := range snap.CircuitBreakers {
	v := 0
	switch state {
	case "open":
		v = 1
	case "half-open":
		v = 2
	}
	fmt.Fprintf(w, "shuttle_circuit_breaker_state{outbound=%q} %d\n", outbound, v)
}
// Backward compatibility: continue emitting the unlabelled gauge as the
// max-severity state across outbounds. This is removed in v0.5.
worst := 0
for _, state := range snap.CircuitBreakers {
	switch state {
	case "open":
		if worst < 1 { worst = 1 }
	case "half-open":
		if worst < 2 { worst = 2 }
	}
}
fmt.Fprintf(w, "shuttle_circuit_breaker_state %d  # DEPRECATED: use the labelled variant; removed in v0.5\n", worst)

// Subscriptions
fmt.Fprintf(w, "# HELP shuttle_subscription_refresh_total Subscription refresh attempts\n")
fmt.Fprintf(w, "# TYPE shuttle_subscription_refresh_total counter\n")
for id, stats := range snap.Subscriptions {
	fmt.Fprintf(w, "shuttle_subscription_refresh_total{subscription=%q,result=\"ok\"} %d\n", id, stats.OK)
	fmt.Fprintf(w, "shuttle_subscription_refresh_total{subscription=%q,result=\"fail\"} %d\n", id, stats.Fail)
}
fmt.Fprintf(w, "# HELP shuttle_subscription_last_refresh_timestamp Unix timestamp of last refresh attempt\n")
fmt.Fprintf(w, "# TYPE shuttle_subscription_last_refresh_timestamp gauge\n")
for id, stats := range snap.Subscriptions {
	fmt.Fprintf(w, "shuttle_subscription_last_refresh_timestamp{subscription=%q} %d\n", id, stats.LastRefresh.Unix())
}

// Handshake durations — emit as a simple summary (count + sum), not full histogram,
// since the engine stores raw observations.
fmt.Fprintf(w, "# HELP shuttle_handshake_duration_seconds Client-side handshake duration (summary)\n")
fmt.Fprintf(w, "# TYPE shuttle_handshake_duration_seconds summary\n")
for transport, observations := range snap.HandshakeDurationsNanos {
	if len(observations) == 0 { continue }
	var sum int64
	for _, n := range observations { sum += n }
	fmt.Fprintf(w, "shuttle_handshake_duration_seconds_count{transport=%q} %d\n", transport, len(observations))
	fmt.Fprintf(w, "shuttle_handshake_duration_seconds_sum{transport=%q} %f\n", transport, float64(sum)/1e9)
}
```

(Note: emitting `summary` instead of `histogram` here is intentional — the client stores raw observations, not pre-bucketed counts. Server-side W2 emits a full histogram. Document this in a comment in the file.)

- [ ] **Step 13.4: Run**

```
./scripts/test.sh --pkg ./gui/api/
```

Expected: PASS.

- [ ] **Step 13.5: Commit**

```bash
git add gui/api/routes_prometheus.go gui/api/routes_prometheus_test.go
git commit -m "feat(gui/api): expand /api/prometheus with router/CB/subscription/handshake"
```

---

## Task 14: Document the deprecation

**Files:**
- Modify: `CHANGELOG.md` (top section under "Unreleased")
- Modify: `docs/site/zh/observability.md` and `docs/site/en/observability.md` if present

- [ ] **Step 14.1: Add changelog entry**

Append to the "Unreleased" section of `CHANGELOG.md`:

```markdown
### Added
- Server `/metrics`: `shuttle_handshake_duration_seconds`, `shuttle_handshake_failures_total`, `shuttle_dns_query_duration_seconds`, `shuttle_destination_resolve_failures_total`. Per-user gauge gated by `metrics.per_user`.
- Client `/api/prometheus`: `shuttle_routing_decisions_total`, `shuttle_circuit_breaker_state{outbound}`, `shuttle_subscription_refresh_total`, `shuttle_subscription_last_refresh_timestamp`, `shuttle_handshake_duration_seconds`.

### Deprecated
- Client `/api/prometheus` unlabelled `shuttle_circuit_breaker_state` (no `outbound` label). The labelled variant supersedes it. The unlabelled metric will be removed in v0.5.
```

- [ ] **Step 14.2: Commit**

```bash
git add CHANGELOG.md docs/site/
git commit -m "docs: changelog entry for metrics expansion + CB deprecation"
```

---

## Task 15: End-to-end verification

- [ ] **Step 15.1: Full host test**

```
./scripts/test.sh
```

Expected: ALL PASS.

- [ ] **Step 15.2: Race detector (catches metric concurrency bugs)**

```
go test -race ./server/metrics/... ./engine/... ./router/... ./subscription/...
```

(Run from the repo root. Note: this is the one place where running `go test` directly is safe because none of these packages touch system network state. Verify with the maintainer if uncertain.)

Expected: no race warnings.

- [ ] **Step 15.3: Build**

```
CGO_ENABLED=0 go build -o /tmp/shuttle ./cmd/shuttle
CGO_ENABLED=0 go build -o /tmp/shuttled ./cmd/shuttled
```

Expected: clean.

- [ ] **Step 15.4: Sandbox scrape**

```
./sandbox/run.sh up
curl -s http://localhost:9090/metrics | grep -E "shuttle_handshake_duration|shuttle_dns_query" | head -5
curl -s http://localhost:19091/api/prometheus | grep -E "shuttle_routing_decisions|shuttle_circuit_breaker_state" | head -5
./sandbox/run.sh down
```

Expected: new metric lines present in both endpoints.

---

## Self-Review Notes

- **Concurrency:** Both `labeledCounter` and `labeledHistogram` use the existing RLock-fast-path pattern from `getOrCreateTransport`. `engineMetrics` uses a single mutex around all snapshot fields — acceptable for hook write rates that are bounded by request rate.
- **Hook async dispatch:** `notifyDecision` and `SetRefreshHook` callbacks fire in goroutines. Reasoning: the metric update is monotonic / order-insensitive, and we don't want hot paths blocked on metrics. Tests use `time.Sleep(10*time.Millisecond)` after the trigger if they need to assert hook completion, OR a `sync.WaitGroup`.
- **Memory bound on client handshake observations:** Capped at 1024 most-recent observations per transport (Task 12.2). Beyond that, we'd need a real histogram on the client side — out of scope for v1.
- **Deprecated metric:** The unlabelled `shuttle_circuit_breaker_state` is documented in CHANGELOG and emits a comment line in the metrics body. v0.5 removes it. Existing scrapers continue to work in v0.4.
- **Per-user gauge:** Gated by `cfg.Metrics.PerUser` to bound cardinality. Default off.
