# Phase 1: Protocol Adapter Layer + Strategy Groups + Providers

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the infrastructure layer that all new protocols, strategy groups, and providers will use — without adding any new protocols yet.

**Architecture:** Extend the adapter layer with a `Dialer` interface for per-request protocols alongside existing `ClientTransport`. Add `HealthChecker` for application-layer probing. Extend `OutboundGroup` with url-test and select strategies. Add Proxy Provider and Rule Provider frameworks with hot-reload. All validated using existing transports.

**Tech Stack:** Go 1.24+, existing adapter/engine/router packages, net/http for health checks

**Spec:** `docs/superpowers/specs/2026-04-05-ecosystem-compatibility-design.md` — Sections 3, 4

---

## File Structure

### New Files
| File | Responsibility |
|------|---------------|
| `adapter/dialer.go` | `Dialer` and `InboundHandler` interfaces |
| `adapter/bridge.go` | Bidirectional adapters: `DialerAsTransport`, `TransportAsDialer` |
| `outbound/healthcheck/checker.go` | HTTP-based health checking for all outbound types |
| `outbound/healthcheck/checker_test.go` | Tests for HealthChecker |
| `engine/outbound_group_urltest.go` | url-test strategy implementation |
| `engine/outbound_group_select.go` | select strategy implementation |
| `engine/outbound_group_dag.go` | Group DAG validation (cycle detection) |
| `engine/outbound_group_dag_test.go` | Tests for DAG validation |
| `provider/proxy_provider.go` | Proxy Provider: fetch, parse, cache node lists |
| `provider/proxy_provider_test.go` | Tests for Proxy Provider |
| `provider/rule_provider.go` | Rule Provider: fetch, parse, cache rule sets |
| `provider/rule_provider_test.go` | Tests for Rule Provider |
| `provider/parser.go` | Auto-format detection for proxy provider data |
| `provider/parser_test.go` | Tests for format detection |
| `gui/api/routes_groups.go` | API endpoints for strategy groups |
| `gui/api/routes_providers.go` | API endpoints for providers |

### Modified Files
| File | Change |
|------|--------|
| `adapter/registry.go` | Extend `TransportFactory` with `NewDialer` and `NewInboundHandler` methods |
| `adapter/outbound.go` | No change needed — existing `Outbound.DialContext` already matches `Dialer` |
| `config/config.go` | Add `ProxyProviders` and `RuleProviders` fields to `ClientConfig` |
| `config/config_routing.go` | Add `RuleProvider []string` to `RuleMatch` |
| `engine/outbound_group.go` | Add `GroupURLTest` and `GroupSelect` constants, refactor `DialContext` to dispatch |
| `engine/engine_inbound.go` | Wire providers into outbound/router build pipeline |
| `router/rule_chain.go` | Add `ruleProviderMatcher` type |
| `router/router.go` | Add provider rule segment with atomic swap |

---

### Task 1: Dialer and InboundHandler Interfaces

**Files:**
- Create: `adapter/dialer.go`
- Modify: `adapter/registry.go`

- [ ] **Step 1: Create Dialer and InboundHandler interfaces**

```go
// adapter/dialer.go
package adapter

import (
	"context"
	"net"
)

// Dialer is the interface for per-request protocols (SS, VLESS, Trojan, etc.)
// that create a new connection for each request instead of multiplexing.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
	Type() string
	Close() error
}

// InboundHandler handles incoming connections for per-request protocol servers.
type InboundHandler interface {
	Type() string
	Serve(ctx context.Context, listener net.Listener, handler ConnHandler) error
	Close() error
}

// ConnHandler is invoked by InboundHandler when a new proxied connection arrives.
type ConnHandler func(ctx context.Context, conn net.Conn, metadata ConnMetadata)
```

Note: `ConnMetadata` already exists in `adapter/inbound.go`. Check it has `Network`, `Destination`, `Source` fields. If it needs extending, add fields there.

- [ ] **Step 2: Extend TransportFactory with Dialer/InboundHandler creation**

In `adapter/registry.go`, the existing `TransportFactory` interface has `NewClient` and `NewServer`. Add two new optional methods. To avoid breaking existing factories, use a separate optional interface:

```go
// adapter/registry.go — add after existing TransportFactory

// DialerFactory is optionally implemented by TransportFactory for per-request protocols.
type DialerFactory interface {
	NewDialer(cfg map[string]any, opts FactoryOptions) (Dialer, error)
	NewInboundHandler(cfg map[string]any, opts FactoryOptions) (InboundHandler, error)
}
```

Using `map[string]any` for config allows each protocol to define its own config shape without changing the central config package in this task.

- [ ] **Step 3: Add factory lookup helper**

```go
// adapter/registry.go — add helper

// GetDialerFactory returns the DialerFactory for the given type, or nil if not supported.
func GetDialerFactory(typeName string) DialerFactory {
	f := Get(typeName)
	if f == nil {
		return nil
	}
	if df, ok := f.(DialerFactory); ok {
		return df
	}
	return nil
}
```

- [ ] **Step 4: Run existing tests to verify no breakage**

Run: `./scripts/test.sh --pkg ./adapter/`
Expected: All existing tests pass, no regressions.

- [ ] **Step 5: Commit**

```bash
git add adapter/dialer.go adapter/registry.go
git commit -m "feat(adapter): add Dialer and InboundHandler interfaces for per-request protocols"
```

---

### Task 2: Bridge Adapters

**Files:**
- Create: `adapter/bridge.go`
- Create: `adapter/bridge_test.go`

- [ ] **Step 1: Write test for DialerAsTransport**

```go
// adapter/bridge_test.go
package adapter_test

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shuttle/adapter"
)

// mockDialer is a test Dialer that connects to a local listener.
type mockDialer struct {
	addr string
}

func (d *mockDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return net.Dial("tcp", d.addr)
}
func (d *mockDialer) Type() string { return "mock" }
func (d *mockDialer) Close() error { return nil }

func TestDialerAsTransport_DialAndStream(t *testing.T) {
	// Start a TCP echo server
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				io.Copy(conn, conn)
				conn.Close()
			}()
		}
	}()

	d := &mockDialer{addr: ln.Addr().String()}
	ct := adapter.DialerAsTransport(d)

	assert.Equal(t, "mock", ct.Type())

	conn, err := ct.Dial(context.Background(), ln.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	stream, err := conn.OpenStream(context.Background())
	require.NoError(t, err)

	_, err = stream.Write([]byte("hello"))
	require.NoError(t, err)

	buf := make([]byte, 5)
	_, err = io.ReadFull(stream, buf)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(buf))
}

func TestTransportAsDialer(t *testing.T) {
	// Start a TCP echo server
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				io.Copy(conn, conn)
				conn.Close()
			}()
		}
	}()

	d := &mockDialer{addr: ln.Addr().String()}
	ct := adapter.DialerAsTransport(d)
	dialer := adapter.TransportAsDialer(ct, ln.Addr().String())

	assert.Equal(t, "mock", dialer.Type())

	conn, err := dialer.DialContext(context.Background(), "tcp", "ignored")
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("world"))
	require.NoError(t, err)

	buf := make([]byte, 5)
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	assert.Equal(t, "world", string(buf))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `./scripts/test.sh --pkg ./adapter/ --run TestDialerAsTransport`
Expected: FAIL — `DialerAsTransport` not defined.

- [ ] **Step 3: Implement bridge adapters**

```go
// adapter/bridge.go
package adapter

import (
	"context"
	"net"
	"sync/atomic"
)

// DialerAsTransport wraps a Dialer as a ClientTransport.
// Each Dial() returns a single-stream Connection wrapping one net.Conn.
func DialerAsTransport(d Dialer) ClientTransport {
	return &dialerTransport{dialer: d}
}

type dialerTransport struct {
	dialer Dialer
}

func (t *dialerTransport) Dial(ctx context.Context, addr string) (Connection, error) {
	conn, err := t.dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	return &singleStreamConn{conn: conn}, nil
}

func (t *dialerTransport) Type() string { return t.dialer.Type() }
func (t *dialerTransport) Close() error { return t.dialer.Close() }

// singleStreamConn wraps a net.Conn as a Connection with exactly one stream.
type singleStreamConn struct {
	conn   net.Conn
	opened atomic.Bool
}

func (c *singleStreamConn) OpenStream(ctx context.Context) (Stream, error) {
	if !c.opened.CompareAndSwap(false, true) {
		return nil, net.ErrClosed
	}
	return &connStream{conn: c.conn}, nil
}

func (c *singleStreamConn) AcceptStream(ctx context.Context) (Stream, error) {
	// Single-stream connections don't accept inbound streams.
	<-ctx.Done()
	return nil, ctx.Err()
}

func (c *singleStreamConn) Close() error              { return c.conn.Close() }
func (c *singleStreamConn) LocalAddr() net.Addr        { return c.conn.LocalAddr() }
func (c *singleStreamConn) RemoteAddr() net.Addr       { return c.conn.RemoteAddr() }

// connStream wraps a net.Conn as a Stream.
type connStream struct {
	conn net.Conn
	id   uint64
}

func (s *connStream) Read(p []byte) (int, error)  { return s.conn.Read(p) }
func (s *connStream) Write(p []byte) (int, error) { return s.conn.Write(p) }
func (s *connStream) Close() error                { return s.conn.Close() }
func (s *connStream) StreamID() uint64             { return s.id }

// TransportAsDialer wraps a ClientTransport as a Dialer.
// Each DialContext opens a new Connection and returns the first stream as net.Conn.
func TransportAsDialer(t ClientTransport, serverAddr string) Dialer {
	return &transportDialer{transport: t, addr: serverAddr}
}

type transportDialer struct {
	transport ClientTransport
	addr      string
}

func (d *transportDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := d.transport.Dial(ctx, d.addr)
	if err != nil {
		return nil, err
	}
	stream, err := conn.OpenStream(ctx)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &streamConn{
		Stream:     stream,
		localAddr:  conn.LocalAddr(),
		remoteAddr: conn.RemoteAddr(),
		parent:     conn,
	}, nil
}

func (d *transportDialer) Type() string { return d.transport.Type() }
func (d *transportDialer) Close() error { return d.transport.Close() }

// streamConn wraps a Stream as a net.Conn.
type streamConn struct {
	Stream
	localAddr  net.Addr
	remoteAddr net.Addr
	parent     Connection
}

func (c *streamConn) LocalAddr() net.Addr  { return c.localAddr }
func (c *streamConn) RemoteAddr() net.Addr { return c.remoteAddr }

func (c *streamConn) SetDeadline(_ interface{ ... }) error      { return nil }
func (c *streamConn) SetReadDeadline(_ interface{ ... }) error  { return nil }
func (c *streamConn) SetWriteDeadline(_ interface{ ... }) error { return nil }
```

Note: The `SetDeadline` methods need proper `time.Time` signatures to satisfy `net.Conn`. Fix:

```go
import "time"

func (c *streamConn) SetDeadline(t time.Time) error      { return nil }
func (c *streamConn) SetReadDeadline(t time.Time) error   { return nil }
func (c *streamConn) SetWriteDeadline(t time.Time) error  { return nil }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `./scripts/test.sh --pkg ./adapter/ --run "TestDialerAsTransport|TestTransportAsDialer"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add adapter/bridge.go adapter/bridge_test.go
git commit -m "feat(adapter): add DialerAsTransport and TransportAsDialer bridge adapters"
```

---

### Task 3: HealthChecker

**Files:**
- Create: `outbound/healthcheck/checker.go`
- Create: `outbound/healthcheck/checker_test.go`

- [ ] **Step 1: Write test for HealthChecker**

```go
// outbound/healthcheck/checker_test.go
package healthcheck_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shuttle/outbound/healthcheck"
)

func TestChecker_SingleCheck(t *testing.T) {
	// HTTP 204 server simulating gstatic generate_204
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	checker := healthcheck.New(&healthcheck.Config{
		URL:       srv.URL,
		Interval:  1 * time.Hour, // won't auto-fire in test
		Timeout:   5 * time.Second,
		Tolerance: 1,
	})

	// dialFunc simulates outbound dialing by just doing a direct TCP dial
	result := checker.Check(context.Background(), "test-node", healthcheck.DirectDialer())
	require.True(t, result.Available)
	assert.True(t, result.Latency > 0)
	assert.True(t, result.Latency < 5*time.Second)
}

func TestChecker_FailedCheck(t *testing.T) {
	checker := healthcheck.New(&healthcheck.Config{
		URL:       "http://127.0.0.1:1", // nothing listening
		Interval:  1 * time.Hour,
		Timeout:   500 * time.Millisecond,
		Tolerance: 1,
	})

	result := checker.Check(context.Background(), "bad-node", healthcheck.DirectDialer())
	assert.False(t, result.Available)
}

func TestChecker_ToleranceThreshold(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			// First 2 calls timeout (close connection)
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				conn.Close()
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	checker := healthcheck.New(&healthcheck.Config{
		URL:       srv.URL,
		Interval:  1 * time.Hour,
		Timeout:   500 * time.Millisecond,
		Tolerance: 3, // need 3 consecutive failures to mark down
	})

	dialer := healthcheck.DirectDialer()

	// First check fails but tolerance not reached
	r1 := checker.Check(context.Background(), "node-a", dialer)
	// Tolerance logic: single check reports raw result, tolerance is tracked in Results()
	assert.False(t, r1.Available) // raw result is false

	r2 := checker.Check(context.Background(), "node-a", dialer)
	assert.False(t, r2.Available)

	// Third success
	r3 := checker.Check(context.Background(), "node-a", dialer)
	assert.True(t, r3.Available)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `./scripts/test.sh --pkg ./outbound/healthcheck/`
Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Implement HealthChecker**

```go
// outbound/healthcheck/checker.go
package healthcheck

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

// DialFunc creates a connection through a specific outbound.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// DirectDialer returns a DialFunc that dials directly (for testing).
func DirectDialer() DialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
}

// Config configures the health checker.
type Config struct {
	URL       string        // Health check URL (default: http://www.gstatic.com/generate_204)
	Interval  time.Duration // Check interval (default: 300s)
	Timeout   time.Duration // Single check timeout (default: 5s)
	Tolerance int           // Consecutive failures before marking down (default: 3)
	Lazy      bool          // Only check when used
}

func (c *Config) defaults() {
	if c.URL == "" {
		c.URL = "http://www.gstatic.com/generate_204"
	}
	if c.Interval == 0 {
		c.Interval = 300 * time.Second
	}
	if c.Timeout == 0 {
		c.Timeout = 5 * time.Second
	}
	if c.Tolerance == 0 {
		c.Tolerance = 3
	}
}

// Result is the outcome of a single health check.
type Result struct {
	Latency   time.Duration
	Available bool
	UpdatedAt time.Time
}

// Checker performs HTTP health checks against outbounds.
type Checker struct {
	cfg     Config
	mu      sync.RWMutex
	results map[string]*nodeState
}

type nodeState struct {
	latest          Result
	consecutiveFail int
}

// New creates a new Checker.
func New(cfg *Config) *Checker {
	cfg.defaults()
	return &Checker{
		cfg:     *cfg,
		results: make(map[string]*nodeState),
	}
}

// Check performs a single health check for the named node using the given dialer.
func (c *Checker) Check(ctx context.Context, tag string, dial DialFunc) Result {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	transport := &http.Transport{
		DialContext: dial,
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.URL, nil)
	if err != nil {
		return c.recordResult(tag, Result{Available: false, UpdatedAt: time.Now()})
	}

	resp, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return c.recordResult(tag, Result{Available: false, UpdatedAt: time.Now()})
	}
	resp.Body.Close()

	available := resp.StatusCode >= 200 && resp.StatusCode < 400
	return c.recordResult(tag, Result{
		Latency:   latency,
		Available: available,
		UpdatedAt: time.Now(),
	})
}

func (c *Checker) recordResult(tag string, r Result) Result {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, ok := c.results[tag]
	if !ok {
		state = &nodeState{}
		c.results[tag] = state
	}

	if !r.Available {
		state.consecutiveFail++
	} else {
		state.consecutiveFail = 0
	}
	state.latest = r
	return r
}

// Results returns the latest results for all checked nodes.
// A node is considered available only if consecutiveFail < tolerance.
func (c *Checker) Results() map[string]Result {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make(map[string]Result, len(c.results))
	for tag, state := range c.results {
		r := state.latest
		if state.consecutiveFail >= c.cfg.Tolerance {
			r.Available = false
		}
		out[tag] = r
	}
	return out
}

// Result returns the latest result for a specific node.
func (c *Checker) Result(tag string) (Result, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	state, ok := c.results[tag]
	if !ok {
		return Result{}, false
	}
	r := state.latest
	if state.consecutiveFail >= c.cfg.Tolerance {
		r.Available = false
	}
	return r, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `./scripts/test.sh --pkg ./outbound/healthcheck/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add outbound/healthcheck/
git commit -m "feat(healthcheck): add HTTP health checker for outbound nodes"
```

---

### Task 4: url-test Strategy Group

**Files:**
- Create: `engine/outbound_group_urltest.go`
- Modify: `engine/outbound_group.go`

- [ ] **Step 1: Read existing outbound_group.go**

Read `engine/outbound_group.go` to understand the current `OutboundGroup` struct and `DialContext` dispatch logic.

- [ ] **Step 2: Add GroupURLTest and GroupSelect constants**

In `engine/outbound_group.go`, add to the existing constants:

```go
const (
	GroupFailover    GroupStrategy = "failover"
	GroupLoadBalance GroupStrategy = "loadbalance"
	GroupQuality     GroupStrategy = "quality"
	GroupURLTest     GroupStrategy = "url-test"   // new
	GroupSelect      GroupStrategy = "select"     // new
)
```

And in the `DialContext` method's switch statement, add:

```go
case GroupURLTest:
    return g.dialURLTest(ctx, network, address)
case GroupSelect:
    return g.dialSelect(ctx, network, address)
```

- [ ] **Step 3: Implement url-test strategy**

```go
// engine/outbound_group_urltest.go
package engine

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"shuttle/adapter"
	"shuttle/outbound/healthcheck"
)

// URLTestConfig configures the url-test strategy.
type URLTestConfig struct {
	ToleranceMS int `json:"tolerance_ms"` // Switch only if new node is this much faster (default: 50)
}

// urlTestState holds the url-test selection state for an OutboundGroup.
type urlTestState struct {
	checker    *healthcheck.Checker
	selected   atomic.Pointer[adapter.Outbound]
	tolerance  time.Duration
	mu         sync.Mutex
	logger     *slog.Logger
	cancelLoop context.CancelFunc
}

func newURLTestState(checker *healthcheck.Checker, toleranceMS int, logger *slog.Logger) *urlTestState {
	if toleranceMS <= 0 {
		toleranceMS = 50
	}
	return &urlTestState{
		checker:   checker,
		tolerance: time.Duration(toleranceMS) * time.Millisecond,
		logger:    logger,
	}
}

// Start begins the periodic health check loop.
func (s *urlTestState) Start(ctx context.Context, outbounds []adapter.Outbound) {
	ctx, s.cancelLoop = context.WithCancel(ctx)
	go s.loop(ctx, outbounds)
}

func (s *urlTestState) Stop() {
	if s.cancelLoop != nil {
		s.cancelLoop()
	}
}

func (s *urlTestState) loop(ctx context.Context, outbounds []adapter.Outbound) {
	// Initial check
	s.checkAll(ctx, outbounds)

	ticker := time.NewTicker(s.checker.Cfg().Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAll(ctx, outbounds)
		}
	}
}

func (s *urlTestState) checkAll(ctx context.Context, outbounds []adapter.Outbound) {
	var wg sync.WaitGroup
	for _, ob := range outbounds {
		wg.Add(1)
		go func(ob adapter.Outbound) {
			defer wg.Done()
			dialFn := func(dctx context.Context, network, addr string) (net.Conn, error) {
				return ob.DialContext(dctx, network, addr)
			}
			s.checker.Check(ctx, ob.Tag(), dialFn)
		}(ob)
	}
	wg.Wait()

	// Select best node
	s.selectBest(outbounds)
}

func (s *urlTestState) selectBest(outbounds []adapter.Outbound) {
	s.mu.Lock()
	defer s.mu.Unlock()

	results := s.checker.Results()
	current := s.selected.Load()

	var bestOB adapter.Outbound
	var bestLatency time.Duration

	for _, ob := range outbounds {
		r, ok := results[ob.Tag()]
		if !ok || !r.Available {
			continue
		}
		if bestOB == nil || r.Latency < bestLatency {
			bestOB = ob
			bestLatency = r.Latency
		}
	}

	if bestOB == nil {
		// All down — keep current or pick first
		if current == nil && len(outbounds) > 0 {
			s.selected.Store(&outbounds[0])
		}
		return
	}

	// Only switch if improvement exceeds tolerance
	if current != nil {
		currentResult, ok := results[(*current).Tag()]
		if ok && currentResult.Available {
			if currentResult.Latency-bestLatency < s.tolerance {
				return // current is good enough
			}
		}
	}

	s.selected.Store(&bestOB)
	if s.logger != nil {
		s.logger.Info("url-test selected new node", "tag", bestOB.Tag(), "latency", bestLatency)
	}
}

func (g *OutboundGroup) dialURLTest(ctx context.Context, network, address string) (net.Conn, error) {
	if g.urlTest == nil {
		return nil, net.ErrClosed
	}

	sel := g.urlTest.selected.Load()
	if sel != nil {
		conn, err := (*sel).DialContext(ctx, network, address)
		if err == nil {
			return conn, nil
		}
	}

	// Fallback: try all members in order
	return g.dialFailover(ctx, network, address)
}
```

- [ ] **Step 4: Add `urlTest` field to OutboundGroup**

In `engine/outbound_group.go`, add to the `OutboundGroup` struct:

```go
type OutboundGroup struct {
	tag         string
	strategy    GroupStrategy
	outbounds   []adapter.Outbound
	counter     atomic.Uint64
	qualityCfg  QualityConfig
	probeGetter func() map[string]ProbeSnapshot
	logger      *slog.Logger
	urlTest     *urlTestState  // new: for url-test strategy
	selectState *selectState   // new: for select strategy (Task 5)
}
```

- [ ] **Step 5: Run tests**

Run: `./scripts/test.sh --pkg ./engine/ --run TestOutboundGroup`
Expected: All existing tests pass plus new url-test behavior works.

- [ ] **Step 6: Commit**

```bash
git add engine/outbound_group_urltest.go engine/outbound_group.go
git commit -m "feat(engine): add url-test strategy group with health-check-based auto-selection"
```

---

### Task 5: select Strategy Group

**Files:**
- Create: `engine/outbound_group_select.go`
- Modify: `engine/outbound_group.go` (add `dialSelect`)

- [ ] **Step 1: Implement select strategy**

```go
// engine/outbound_group_select.go
package engine

import (
	"context"
	"fmt"
	"net"
	"sync"

	"shuttle/adapter"
)

// selectState holds the manual selection state for an OutboundGroup.
type selectState struct {
	mu       sync.RWMutex
	selected adapter.Outbound
	members  map[string]adapter.Outbound // tag → outbound for fast lookup
}

func newSelectState(outbounds []adapter.Outbound) *selectState {
	members := make(map[string]adapter.Outbound, len(outbounds))
	for _, ob := range outbounds {
		members[ob.Tag()] = ob
	}
	var initial adapter.Outbound
	if len(outbounds) > 0 {
		initial = outbounds[0]
	}
	return &selectState{
		selected: initial,
		members:  members,
	}
}

// Select sets the active outbound by tag. Returns error if tag not found.
func (s *selectState) Select(tag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ob, ok := s.members[tag]
	if !ok {
		return fmt.Errorf("outbound %q not found in group", tag)
	}
	s.selected = ob
	return nil
}

// Selected returns the currently selected outbound tag.
func (s *selectState) Selected() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.selected == nil {
		return ""
	}
	return s.selected.Tag()
}

func (g *OutboundGroup) dialSelect(ctx context.Context, network, address string) (net.Conn, error) {
	if g.selectState == nil {
		return nil, net.ErrClosed
	}

	g.selectState.mu.RLock()
	sel := g.selectState.selected
	g.selectState.mu.RUnlock()

	if sel == nil {
		return nil, fmt.Errorf("no outbound selected in group %q", g.tag)
	}
	return sel.DialContext(ctx, network, address)
}

// SelectOutbound sets the active outbound for a select-strategy group.
func (g *OutboundGroup) SelectOutbound(tag string) error {
	if g.selectState == nil {
		return fmt.Errorf("group %q is not a select group", g.tag)
	}
	return g.selectState.Select(tag)
}

// SelectedOutbound returns the currently selected outbound tag.
func (g *OutboundGroup) SelectedOutbound() string {
	if g.selectState == nil {
		return ""
	}
	return g.selectState.Selected()
}
```

- [ ] **Step 2: Run tests**

Run: `./scripts/test.sh --pkg ./engine/`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add engine/outbound_group_select.go engine/outbound_group.go
git commit -m "feat(engine): add select strategy group with manual node switching"
```

---

### Task 6: Group DAG Validation

**Files:**
- Create: `engine/outbound_group_dag.go`
- Create: `engine/outbound_group_dag_test.go`

- [ ] **Step 1: Write tests**

```go
// engine/outbound_group_dag_test.go
package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateGroupDAG_NoCycle(t *testing.T) {
	groups := map[string][]string{
		"A": {"proxy-1", "proxy-2"},
		"B": {"A", "proxy-3"},       // B references A
		"C": {"B", "direct"},        // C references B
	}
	err := validateGroupDAG(groups)
	assert.NoError(t, err)
}

func TestValidateGroupDAG_DirectCycle(t *testing.T) {
	groups := map[string][]string{
		"A": {"B"},
		"B": {"A"}, // A → B → A
	}
	err := validateGroupDAG(groups)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestValidateGroupDAG_IndirectCycle(t *testing.T) {
	groups := map[string][]string{
		"A": {"B"},
		"B": {"C"},
		"C": {"A"}, // A → B → C → A
	}
	err := validateGroupDAG(groups)
	assert.Error(t, err)
}

func TestValidateGroupDAG_SelfCycle(t *testing.T) {
	groups := map[string][]string{
		"A": {"A"}, // self-reference
	}
	err := validateGroupDAG(groups)
	assert.Error(t, err)
}

func TestValidateGroupDAG_Empty(t *testing.T) {
	err := validateGroupDAG(map[string][]string{})
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `./scripts/test.sh --pkg ./engine/ --run TestValidateGroupDAG`
Expected: FAIL — `validateGroupDAG` not defined.

- [ ] **Step 3: Implement DAG validation**

```go
// engine/outbound_group_dag.go
package engine

import "fmt"

// validateGroupDAG checks that group references form a DAG (no cycles).
// groups maps group tag → list of member tags (which may include other group tags).
func validateGroupDAG(groups map[string][]string) error {
	// Build adjacency: group → referenced groups
	adj := make(map[string][]string)
	for tag, members := range groups {
		for _, m := range members {
			if _, isGroup := groups[m]; isGroup {
				adj[tag] = append(adj[tag], m)
			}
		}
	}

	// DFS cycle detection
	const (
		white = 0 // unvisited
		gray  = 1 // in current path
		black = 2 // fully explored
	)
	color := make(map[string]int)

	var visit func(node string) error
	visit = func(node string) error {
		color[node] = gray
		for _, next := range adj[node] {
			switch color[next] {
			case gray:
				return fmt.Errorf("cycle detected: %s → %s", node, next)
			case white:
				if err := visit(next); err != nil {
					return err
				}
			}
		}
		color[node] = black
		return nil
	}

	for tag := range groups {
		if color[tag] == white {
			if err := visit(tag); err != nil {
				return err
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `./scripts/test.sh --pkg ./engine/ --run TestValidateGroupDAG`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add engine/outbound_group_dag.go engine/outbound_group_dag_test.go
git commit -m "feat(engine): add DAG validation for nested strategy groups"
```

---

### Task 7: Config Types for Providers

**Files:**
- Modify: `config/config.go`
- Modify: `config/config_routing.go`

- [ ] **Step 1: Read current config files**

Read `config/config.go` and `config/config_routing.go` to understand existing structures.

- [ ] **Step 2: Add provider config types**

In `config/config.go`, add fields to `ClientConfig`:

```go
// Add to ClientConfig struct
ProxyProviders []ProxyProviderConfig `yaml:"proxy_providers,omitempty" json:"proxy_providers,omitempty"`
RuleProviders  []RuleProviderConfig  `yaml:"rule_providers,omitempty" json:"rule_providers,omitempty"`
```

Add new types (in `config/config.go` or a new `config/config_provider.go`):

```go
// ProxyProviderConfig defines a remote node list source.
type ProxyProviderConfig struct {
	Name        string             `yaml:"name" json:"name"`
	URL         string             `yaml:"url" json:"url"`
	Path        string             `yaml:"path,omitempty" json:"path,omitempty"` // local cache
	Interval    string             `yaml:"interval,omitempty" json:"interval,omitempty"` // e.g. "3600s"
	Filter      string             `yaml:"filter,omitempty" json:"filter,omitempty"` // regex
	HealthCheck *HealthCheckConfig `yaml:"health_check,omitempty" json:"health_check,omitempty"`
}

// RuleProviderConfig defines a remote rule set source.
type RuleProviderConfig struct {
	Name     string `yaml:"name" json:"name"`
	URL      string `yaml:"url" json:"url"`
	Path     string `yaml:"path,omitempty" json:"path,omitempty"`
	Behavior string `yaml:"behavior" json:"behavior"` // "domain", "ipcidr", "classical"
	Interval string `yaml:"interval,omitempty" json:"interval,omitempty"`
}

// HealthCheckConfig is shared between strategy groups and proxy providers.
type HealthCheckConfig struct {
	URL         string `yaml:"url,omitempty" json:"url,omitempty"`
	Interval    string `yaml:"interval,omitempty" json:"interval,omitempty"`
	Timeout     string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Tolerance   int    `yaml:"tolerance,omitempty" json:"tolerance,omitempty"`
	ToleranceMS int    `yaml:"tolerance_ms,omitempty" json:"tolerance_ms,omitempty"`
	Lazy        bool   `yaml:"lazy,omitempty" json:"lazy,omitempty"`
}
```

- [ ] **Step 3: Add `Use` and `HealthCheck` to OutboundConfig**

Read `config/config.go` for `OutboundConfig` struct. Add:

```go
// Add to OutboundConfig
Use         []string           `yaml:"use,omitempty" json:"use,omitempty"` // proxy provider names
HealthCheck *HealthCheckConfig `yaml:"health_check,omitempty" json:"health_check,omitempty"`
```

- [ ] **Step 4: Add `RuleProvider` to RuleMatch**

In `config/config_routing.go`, add to `RuleMatch`:

```go
RuleProvider []string `yaml:"rule_provider,omitempty" json:"rule_provider,omitempty"`
```

- [ ] **Step 5: Run tests**

Run: `./scripts/test.sh --pkg ./config/`
Expected: PASS — config changes are additive, no breakage.

- [ ] **Step 6: Commit**

```bash
git add config/
git commit -m "feat(config): add ProxyProvider, RuleProvider, and HealthCheck config types"
```

---

### Task 8: Proxy Provider

**Files:**
- Create: `provider/proxy_provider.go`
- Create: `provider/parser.go`
- Create: `provider/parser_test.go`
- Create: `provider/proxy_provider_test.go`

- [ ] **Step 1: Write test for format auto-detection**

```go
// provider/parser_test.go
package provider_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shuttle/provider"
)

func TestDetectFormat_ClashYAML(t *testing.T) {
	data := []byte(`proxies:
  - name: "hk-01"
    type: ss
    server: hk.example.com
    port: 8388
    cipher: aes-256-gcm
    password: "test"
`)
	format := provider.DetectFormat(data)
	assert.Equal(t, provider.FormatClash, format)
}

func TestDetectFormat_SingboxJSON(t *testing.T) {
	data := []byte(`{"outbounds":[{"type":"shadowsocks","tag":"ss-hk","server":"hk.example.com","server_port":8388}]}`)
	format := provider.DetectFormat(data)
	assert.Equal(t, provider.FormatSingbox, format)
}

func TestDetectFormat_Base64URI(t *testing.T) {
	// base64 of "ss://YWVzLTI1Ni1nY206dGVzdA@hk.example.com:8388#hk-01"
	data := []byte("c3M6Ly9ZV1Z6TFRJMU5pMW5ZMjA2ZEdWemRBQGhrLmV4YW1wbGUuY29tOjgzODgjaGstMDE=")
	format := provider.DetectFormat(data)
	assert.Equal(t, provider.FormatBase64URI, format)
}

func TestDetectFormat_PlainURI(t *testing.T) {
	data := []byte("ss://YWVzLTI1Ni1nY206dGVzdA@hk.example.com:8388#hk-01\nvless://uuid@jp.example.com:443#jp-01\n")
	format := provider.DetectFormat(data)
	assert.Equal(t, provider.FormatPlainURI, format)
}

func TestParseProxyList_ClashFormat(t *testing.T) {
	data := []byte(`proxies:
  - name: "hk-01"
    type: ss
    server: hk.example.com
    port: 8388
    cipher: aes-256-gcm
    password: "test"
  - name: "jp-01"
    type: ss
    server: jp.example.com
    port: 8388
    cipher: aes-256-gcm
    password: "test2"
`)
	nodes, err := provider.ParseProxyList(data)
	require.NoError(t, err)
	require.Len(t, nodes, 2)
	assert.Equal(t, "hk-01", nodes[0].Name)
	assert.Equal(t, "jp-01", nodes[1].Name)
}
```

- [ ] **Step 2: Implement format detection and parser**

```go
// provider/parser.go
package provider

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"
)

// Format represents a subscription/provider data format.
type Format string

const (
	FormatClash    Format = "clash"
	FormatSingbox  Format = "singbox"
	FormatBase64URI Format = "base64-uri"
	FormatPlainURI Format = "plain-uri"
	FormatUnknown  Format = "unknown"
)

// ProxyNode is a parsed node from a provider.
type ProxyNode struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"` // "shadowsocks", "vless", "trojan", etc.
	Server   string            `json:"server"`
	Port     int               `json:"port"`
	Options  map[string]any    `json:"options"` // protocol-specific fields
}

// DetectFormat auto-detects the format of provider data.
func DetectFormat(data []byte) Format {
	trimmed := strings.TrimSpace(string(data))

	// Try JSON (sing-box)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var obj map[string]any
		if json.Unmarshal([]byte(trimmed), &obj) == nil {
			if _, ok := obj["outbounds"]; ok {
				return FormatSingbox
			}
		}
	}

	// Try YAML (Clash)
	if strings.Contains(trimmed, "proxies:") {
		var obj map[string]any
		if yaml.Unmarshal([]byte(trimmed), &obj) == nil {
			if _, ok := obj["proxies"]; ok {
				return FormatClash
			}
		}
	}

	// Try plain URI (lines starting with protocol://)
	lines := strings.Split(trimmed, "\n")
	if len(lines) > 0 {
		first := strings.TrimSpace(lines[0])
		for _, scheme := range []string{"ss://", "vless://", "vmess://", "trojan://", "hysteria2://"} {
			if strings.HasPrefix(first, scheme) {
				return FormatPlainURI
			}
		}
	}

	// Try base64 decode → then re-detect
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(trimmed))
	if err != nil {
		// Try URL-safe or raw base64
		decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(trimmed))
	}
	if err == nil && len(decoded) > 0 {
		inner := strings.TrimSpace(string(decoded))
		for _, scheme := range []string{"ss://", "vless://", "vmess://", "trojan://", "hysteria2://"} {
			if strings.HasPrefix(inner, scheme) {
				return FormatBase64URI
			}
		}
	}

	return FormatUnknown
}

// ParseProxyList parses provider data in any supported format.
func ParseProxyList(data []byte) ([]ProxyNode, error) {
	format := DetectFormat(data)
	switch format {
	case FormatClash:
		return parseClashProxies(data)
	case FormatSingbox:
		return parseSingboxOutbounds(data)
	case FormatBase64URI:
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(string(data)))
			if err != nil {
				return nil, err
			}
		}
		return parseURIList(decoded)
	case FormatPlainURI:
		return parseURIList(data)
	default:
		return nil, fmt.Errorf("unknown provider format")
	}
}

func parseClashProxies(data []byte) ([]ProxyNode, error) {
	var raw struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	nodes := make([]ProxyNode, 0, len(raw.Proxies))
	for _, p := range raw.Proxies {
		node := ProxyNode{
			Name:    stringVal(p, "name"),
			Type:    stringVal(p, "type"),
			Server:  stringVal(p, "server"),
			Port:    intVal(p, "port"),
			Options: p,
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func parseSingboxOutbounds(data []byte) ([]ProxyNode, error) {
	var raw struct {
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	nodes := make([]ProxyNode, 0, len(raw.Outbounds))
	for _, o := range raw.Outbounds {
		node := ProxyNode{
			Name:    stringVal(o, "tag"),
			Type:    stringVal(o, "type"),
			Server:  stringVal(o, "server"),
			Port:    intVal(o, "server_port"),
			Options: o,
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func parseURIList(data []byte) ([]ProxyNode, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var nodes []ProxyNode
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// URI parsing will be implemented per-protocol in Phase 2.
		// For now, store raw URI for later parsing.
		nodes = append(nodes, ProxyNode{
			Name:    line, // placeholder — proper URI parsing in Phase 2
			Type:    "raw-uri",
			Options: map[string]any{"uri": line},
		})
	}
	return nodes, nil
}

func stringVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func intVal(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}
```

Add missing `fmt` import to `parser.go`.

- [ ] **Step 3: Implement ProxyProvider**

```go
// provider/proxy_provider.go
package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// ProxyProvider fetches and caches a list of proxy nodes from a remote URL.
type ProxyProvider struct {
	name     string
	url      string
	path     string        // local cache file
	interval time.Duration
	filter   *regexp.Regexp
	client   *http.Client

	mu       sync.RWMutex
	nodes    []ProxyNode
	updatedAt time.Time
	lastErr  error
	cancel   context.CancelFunc
}

// ProxyProviderConfig initializes a ProxyProvider.
type ProxyProviderConfig struct {
	Name     string
	URL      string
	Path     string
	Interval time.Duration
	Filter   string // regexp pattern for node name filtering
}

// NewProxyProvider creates a new ProxyProvider.
func NewProxyProvider(cfg ProxyProviderConfig) (*ProxyProvider, error) {
	var filter *regexp.Regexp
	if cfg.Filter != "" {
		var err error
		filter, err = regexp.Compile(cfg.Filter)
		if err != nil {
			return nil, fmt.Errorf("invalid filter regex: %w", err)
		}
	}
	if cfg.Interval == 0 {
		cfg.Interval = 1 * time.Hour
	}

	p := &ProxyProvider{
		name:     cfg.Name,
		url:      cfg.URL,
		path:     cfg.Path,
		interval: cfg.Interval,
		filter:   filter,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Load from cache if available
	if cfg.Path != "" {
		if data, err := os.ReadFile(cfg.Path); err == nil {
			if nodes, err := ParseProxyList(data); err == nil {
				p.nodes = p.applyFilter(nodes)
			}
		}
	}

	return p, nil
}

// Start begins the periodic refresh loop.
func (p *ProxyProvider) Start(ctx context.Context) {
	ctx, p.cancel = context.WithCancel(ctx)

	// Initial fetch if no cached data
	if len(p.nodes) == 0 {
		p.Refresh(ctx)
	}

	go func() {
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.Refresh(ctx)
			}
		}
	}()
}

// Stop stops the refresh loop.
func (p *ProxyProvider) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
}

// Refresh fetches the latest node list from the URL.
func (p *ProxyProvider) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		p.mu.Lock()
		p.lastErr = err
		p.mu.Unlock()
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		p.mu.Lock()
		p.lastErr = err
		p.mu.Unlock()
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB limit
	if err != nil {
		p.mu.Lock()
		p.lastErr = err
		p.mu.Unlock()
		return err
	}

	nodes, err := ParseProxyList(data)
	if err != nil {
		p.mu.Lock()
		p.lastErr = err
		p.mu.Unlock()
		return err
	}

	filtered := p.applyFilter(nodes)

	p.mu.Lock()
	p.nodes = filtered
	p.updatedAt = time.Now()
	p.lastErr = nil
	p.mu.Unlock()

	// Save to cache
	if p.path != "" {
		os.MkdirAll(filepath.Dir(p.path), 0o755)
		os.WriteFile(p.path, data, 0o644)
	}

	return nil
}

func (p *ProxyProvider) applyFilter(nodes []ProxyNode) []ProxyNode {
	if p.filter == nil {
		return nodes
	}
	var out []ProxyNode
	for _, n := range nodes {
		if p.filter.MatchString(n.Name) {
			out = append(out, n)
		}
	}
	return out
}

// Nodes returns the current node list.
func (p *ProxyProvider) Nodes() []ProxyNode {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.nodes
}

// Name returns the provider name.
func (p *ProxyProvider) Name() string { return p.name }

// Error returns the last error, if any.
func (p *ProxyProvider) Error() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastErr
}

// UpdatedAt returns when the node list was last refreshed.
func (p *ProxyProvider) UpdatedAt() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.updatedAt
}
```

- [ ] **Step 4: Write ProxyProvider test**

```go
// provider/proxy_provider_test.go
package provider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shuttle/provider"
)

func TestProxyProvider_FetchAndFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`proxies:
  - name: "HK-01"
    type: ss
    server: hk1.example.com
    port: 8388
    cipher: aes-256-gcm
    password: "test"
  - name: "JP-01"
    type: ss
    server: jp1.example.com
    port: 8388
    cipher: aes-256-gcm
    password: "test2"
  - name: "US-01"
    type: ss
    server: us1.example.com
    port: 8388
    cipher: aes-256-gcm
    password: "test3"
`))
	}))
	defer srv.Close()

	p, err := provider.NewProxyProvider(provider.ProxyProviderConfig{
		Name:   "test-provider",
		URL:    srv.URL,
		Filter: "(?i)hk|hong kong",
	})
	require.NoError(t, err)

	err = p.Refresh(context.Background())
	require.NoError(t, err)

	nodes := p.Nodes()
	require.Len(t, nodes, 1)
	assert.Equal(t, "HK-01", nodes[0].Name)
}

func TestProxyProvider_ErrorHandling(t *testing.T) {
	p, err := provider.NewProxyProvider(provider.ProxyProviderConfig{
		Name: "bad-provider",
		URL:  "http://127.0.0.1:1/nonexistent",
	})
	require.NoError(t, err)

	err = p.Refresh(context.Background())
	assert.Error(t, err)
	assert.Error(t, p.Error())
}
```

- [ ] **Step 5: Run tests**

Run: `./scripts/test.sh --pkg ./provider/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add provider/
git commit -m "feat(provider): add ProxyProvider with auto-format detection and filtering"
```

---

### Task 9: Rule Provider

**Files:**
- Create: `provider/rule_provider.go`
- Create: `provider/rule_provider_test.go`

- [ ] **Step 1: Write test**

```go
// provider/rule_provider_test.go
package provider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shuttle/provider"
)

func TestRuleProvider_DomainBehavior(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("google.com\nfacebook.com\ntwitter.com\n"))
	}))
	defer srv.Close()

	rp, err := provider.NewRuleProvider(provider.RuleProviderConfig{
		Name:     "test-domains",
		URL:      srv.URL,
		Behavior: "domain",
	})
	require.NoError(t, err)

	err = rp.Refresh(context.Background())
	require.NoError(t, err)

	assert.True(t, rp.MatchDomain("google.com"))
	assert.True(t, rp.MatchDomain("www.google.com")) // suffix match
	assert.False(t, rp.MatchDomain("example.com"))
}

func TestRuleProvider_IPCIDRBehavior(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("10.0.0.0/8\n172.16.0.0/12\n192.168.0.0/16\n"))
	}))
	defer srv.Close()

	rp, err := provider.NewRuleProvider(provider.RuleProviderConfig{
		Name:     "test-cidrs",
		URL:      srv.URL,
		Behavior: "ipcidr",
	})
	require.NoError(t, err)

	err = rp.Refresh(context.Background())
	require.NoError(t, err)

	assert.True(t, rp.MatchIP("10.1.2.3"))
	assert.True(t, rp.MatchIP("192.168.1.1"))
	assert.False(t, rp.MatchIP("8.8.8.8"))
}

func TestRuleProvider_ClassicalBehavior(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("DOMAIN-SUFFIX,google.com\nIP-CIDR,8.8.8.0/24\nDOMAIN-KEYWORD,facebook\n"))
	}))
	defer srv.Close()

	rp, err := provider.NewRuleProvider(provider.RuleProviderConfig{
		Name:     "test-classical",
		URL:      srv.URL,
		Behavior: "classical",
	})
	require.NoError(t, err)

	err = rp.Refresh(context.Background())
	require.NoError(t, err)

	assert.True(t, rp.MatchDomain("www.google.com"))
	assert.True(t, rp.MatchIP("8.8.8.1"))
	assert.True(t, rp.MatchDomain("m.facebook.com"))
	assert.False(t, rp.MatchDomain("example.com"))
}
```

- [ ] **Step 2: Implement RuleProvider**

```go
// provider/rule_provider.go
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
)

// RuleProviderConfig configures a rule provider.
type RuleProviderConfig struct {
	Name     string
	URL      string
	Path     string
	Behavior string // "domain", "ipcidr", "classical"
	Interval time.Duration
}

// RuleProvider fetches and caches rule sets from a remote URL.
type RuleProvider struct {
	name     string
	url      string
	path     string
	behavior string
	interval time.Duration
	client   *http.Client

	rules     atomic.Pointer[ruleSet]
	mu        sync.Mutex
	updatedAt time.Time
	lastErr   error
	cancel    context.CancelFunc
}

type ruleSet struct {
	domains        map[string]bool // exact domain match
	domainSuffixes []string        // suffix match
	domainKeywords []string        // keyword match
	cidrs          []*net.IPNet
}

func newRuleSet() *ruleSet {
	return &ruleSet{
		domains: make(map[string]bool),
	}
}

// NewRuleProvider creates a new RuleProvider.
func NewRuleProvider(cfg RuleProviderConfig) (*RuleProvider, error) {
	if cfg.Behavior == "" {
		return nil, fmt.Errorf("behavior is required")
	}
	if cfg.Interval == 0 {
		cfg.Interval = 24 * time.Hour
	}

	rp := &RuleProvider{
		name:     cfg.Name,
		url:      cfg.URL,
		path:     cfg.Path,
		behavior: cfg.Behavior,
		interval: cfg.Interval,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
	rp.rules.Store(newRuleSet())

	// Load from cache
	if cfg.Path != "" {
		if data, err := os.ReadFile(cfg.Path); err == nil {
			if rs, err := rp.parse(data); err == nil {
				rp.rules.Store(rs)
			}
		}
	}

	return rp, nil
}

// Start begins periodic refresh.
func (rp *RuleProvider) Start(ctx context.Context) {
	ctx, rp.cancel = context.WithCancel(ctx)
	if rp.rules.Load().isEmpty() {
		rp.Refresh(ctx)
	}
	go func() {
		ticker := time.NewTicker(rp.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rp.Refresh(ctx)
			}
		}
	}()
}

func (rp *RuleProvider) Stop() {
	if rp.cancel != nil {
		rp.cancel()
	}
}

// Refresh fetches and parses the latest rules.
func (rp *RuleProvider) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rp.url, nil)
	if err != nil {
		rp.mu.Lock()
		rp.lastErr = err
		rp.mu.Unlock()
		return err
	}

	resp, err := rp.client.Do(req)
	if err != nil {
		rp.mu.Lock()
		rp.lastErr = err
		rp.mu.Unlock()
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		rp.mu.Lock()
		rp.lastErr = err
		rp.mu.Unlock()
		return err
	}

	rs, err := rp.parse(data)
	if err != nil {
		rp.mu.Lock()
		rp.lastErr = err
		rp.mu.Unlock()
		return err
	}

	rp.rules.Store(rs)
	rp.mu.Lock()
	rp.updatedAt = time.Now()
	rp.lastErr = nil
	rp.mu.Unlock()

	if rp.path != "" {
		os.MkdirAll(filepath.Dir(rp.path), 0o755)
		os.WriteFile(rp.path, data, 0o644)
	}

	return nil
}

func (rp *RuleProvider) parse(data []byte) (*ruleSet, error) {
	rs := newRuleSet()
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	switch rp.behavior {
	case "domain":
		for _, line := range lines {
			domain := strings.TrimSpace(strings.ToLower(line))
			if domain == "" || strings.HasPrefix(domain, "#") {
				continue
			}
			rs.domains[domain] = true
			rs.domainSuffixes = append(rs.domainSuffixes, "."+domain)
		}

	case "ipcidr":
		for _, line := range lines {
			cidr := strings.TrimSpace(line)
			if cidr == "" || strings.HasPrefix(cidr, "#") {
				continue
			}
			_, ipNet, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			rs.cidrs = append(rs.cidrs, ipNet)
		}

	case "classical":
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, ",", 3)
			if len(parts) < 2 {
				continue
			}
			ruleType := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch strings.ToUpper(ruleType) {
			case "DOMAIN":
				rs.domains[strings.ToLower(value)] = true
			case "DOMAIN-SUFFIX":
				rs.domains[strings.ToLower(value)] = true
				rs.domainSuffixes = append(rs.domainSuffixes, "."+strings.ToLower(value))
			case "DOMAIN-KEYWORD":
				rs.domainKeywords = append(rs.domainKeywords, strings.ToLower(value))
			case "IP-CIDR", "IP-CIDR6":
				_, ipNet, err := net.ParseCIDR(value)
				if err == nil {
					rs.cidrs = append(rs.cidrs, ipNet)
				}
			}
		}

	default:
		return nil, fmt.Errorf("unknown behavior: %s", rp.behavior)
	}

	return rs, nil
}

func (rs *ruleSet) isEmpty() bool {
	return len(rs.domains) == 0 && len(rs.domainSuffixes) == 0 &&
		len(rs.domainKeywords) == 0 && len(rs.cidrs) == 0
}

// MatchDomain checks if a domain matches any rule.
func (rp *RuleProvider) MatchDomain(domain string) bool {
	rs := rp.rules.Load()
	domain = strings.ToLower(domain)

	// Exact match
	if rs.domains[domain] {
		return true
	}

	// Suffix match
	for _, suffix := range rs.domainSuffixes {
		if strings.HasSuffix(domain, suffix) {
			return true
		}
	}

	// Keyword match
	for _, kw := range rs.domainKeywords {
		if strings.Contains(domain, kw) {
			return true
		}
	}

	return false
}

// MatchIP checks if an IP matches any CIDR rule.
func (rp *RuleProvider) MatchIP(ipStr string) bool {
	rs := rp.rules.Load()
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
func (rp *RuleProvider) Name() string { return rp.name }

// Behavior returns the provider behavior type.
func (rp *RuleProvider) Behavior() string { return rp.behavior }
```

- [ ] **Step 3: Run tests**

Run: `./scripts/test.sh --pkg ./provider/`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add provider/rule_provider.go provider/rule_provider_test.go
git commit -m "feat(provider): add RuleProvider with domain/ipcidr/classical behaviors"
```

---

### Task 10: Router Rule Provider Integration

**Files:**
- Modify: `router/rule_chain.go`
- Modify: `router/router.go`

- [ ] **Step 1: Read existing rule_chain.go and router.go**

Read both files to understand the existing `Matcher` interface and `compiledRule` structure.

- [ ] **Step 2: Add ruleProviderMatcher**

In `router/rule_chain.go`, add a new matcher type:

```go
// ruleProviderMatcher matches against a RuleProvider's rules.
type ruleProviderMatcher struct {
	providers []*provider.RuleProvider
}

func (m *ruleProviderMatcher) Match(ctx *MatchContext) bool {
	for _, rp := range m.providers {
		if ctx.Domain != "" && rp.MatchDomain(ctx.Domain) {
			return true
		}
		if ctx.IP != nil && rp.MatchIP(ctx.IP.String()) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: Integrate into CompileRuleChain**

In the rule chain compilation function, handle `RuleProvider` entries in `RuleMatch`:

```go
// In CompileRuleChain, add case for RuleProvider
if len(entry.Match.RuleProvider) > 0 {
    var rps []*provider.RuleProvider
    for _, name := range entry.Match.RuleProvider {
        rp, ok := ruleProviders[name]
        if !ok {
            return nil, fmt.Errorf("rule provider %q not found", name)
        }
        rps = append(rps, rp)
    }
    matchers = append(matchers, &ruleProviderMatcher{providers: rps})
}
```

The `CompileRuleChain` function signature needs to accept `ruleProviders map[string]*provider.RuleProvider` parameter.

- [ ] **Step 4: Run tests**

Run: `./scripts/test.sh --pkg ./router/`
Expected: PASS — existing tests unaffected (they don't use rule providers).

- [ ] **Step 5: Commit**

```bash
git add router/rule_chain.go router/router.go
git commit -m "feat(router): integrate RuleProvider matcher into rule chain compilation"
```

---

### Task 11: API Endpoints for Groups and Providers

**Files:**
- Create: `gui/api/routes_groups.go`
- Create: `gui/api/routes_providers.go`
- Modify: `gui/api/api.go` (register routes)

- [ ] **Step 1: Read gui/api/api.go for routing pattern**

Read `gui/api/api.go` to understand how routes are registered (likely `mux.HandleFunc` or similar).

- [ ] **Step 2: Implement group API routes**

```go
// gui/api/routes_groups.go
package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) registerGroupRoutes() {
	s.mux.HandleFunc("GET /api/groups", s.handleListGroups)
	s.mux.HandleFunc("GET /api/groups/{tag}", s.handleGetGroup)
	s.mux.HandleFunc("PUT /api/groups/{tag}/selected", s.handleSelectGroup)
	s.mux.HandleFunc("POST /api/groups/{tag}/test", s.handleTestGroup)
}

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups := s.engine.ListGroups()
	writeJSON(w, groups)
}

func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	group, err := s.engine.GetGroup(tag)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, group)
}

func (s *Server) handleSelectGroup(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	var body struct {
		Selected string `json:"selected"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.engine.SelectGroupOutbound(tag, body.Selected); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestGroup(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	results, err := s.engine.TestGroup(tag)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, results)
}
```

- [ ] **Step 3: Implement provider API routes**

```go
// gui/api/routes_providers.go
package api

import (
	"net/http"
)

func (s *Server) registerProviderRoutes() {
	s.mux.HandleFunc("GET /api/providers/proxy", s.handleListProxyProviders)
	s.mux.HandleFunc("POST /api/providers/proxy/{name}/refresh", s.handleRefreshProxyProvider)
	s.mux.HandleFunc("GET /api/providers/rule", s.handleListRuleProviders)
	s.mux.HandleFunc("POST /api/providers/rule/{name}/refresh", s.handleRefreshRuleProvider)
}

func (s *Server) handleListProxyProviders(w http.ResponseWriter, r *http.Request) {
	providers := s.engine.ListProxyProviders()
	writeJSON(w, providers)
}

func (s *Server) handleRefreshProxyProvider(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.engine.RefreshProxyProvider(r.Context(), name); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListRuleProviders(w http.ResponseWriter, r *http.Request) {
	providers := s.engine.ListRuleProviders()
	writeJSON(w, providers)
}

func (s *Server) handleRefreshRuleProvider(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.engine.RefreshRuleProvider(r.Context(), name); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Register routes in api.go**

Add calls to `s.registerGroupRoutes()` and `s.registerProviderRoutes()` in the existing route setup function.

- [ ] **Step 5: Commit**

```bash
git add gui/api/routes_groups.go gui/api/routes_providers.go gui/api/api.go
git commit -m "feat(api): add REST endpoints for strategy groups and providers"
```

---

### Task 12: Wire Providers into Engine Pipeline

**Files:**
- Modify: `engine/engine_inbound.go`

- [ ] **Step 1: Read engine_inbound.go**

Understand how outbounds and the router are built during engine startup.

- [ ] **Step 2: Add provider initialization to engine startup**

In the engine's startup sequence (where outbounds are built), add:

1. **Initialize Proxy Providers** from `cfg.ProxyProviders`:
   - Create each `ProxyProvider`, call `Start(ctx)`
   - For each strategy group with `use:` referencing a provider, resolve provider nodes into outbounds and add them to the group's member list

2. **Initialize Rule Providers** from `cfg.RuleProviders`:
   - Create each `RuleProvider`, call `Start(ctx)`
   - Pass `map[string]*RuleProvider` to `CompileRuleChain`

3. **Add cleanup** to engine stop: call `Stop()` on all providers.

The exact code depends on the current `engine_inbound.go` structure — read it, then add the provider wiring at the appropriate points.

- [ ] **Step 3: Add engine methods for API**

Add methods to `Engine` that the API routes call:

```go
func (e *Engine) ListGroups() []GroupInfo { ... }
func (e *Engine) GetGroup(tag string) (*GroupInfo, error) { ... }
func (e *Engine) SelectGroupOutbound(groupTag, outboundTag string) error { ... }
func (e *Engine) TestGroup(tag string) (map[string]healthcheck.Result, error) { ... }
func (e *Engine) ListProxyProviders() []ProviderInfo { ... }
func (e *Engine) RefreshProxyProvider(ctx context.Context, name string) error { ... }
func (e *Engine) ListRuleProviders() []ProviderInfo { ... }
func (e *Engine) RefreshRuleProvider(ctx context.Context, name string) error { ... }
```

- [ ] **Step 4: Run full test suite**

Run: `./scripts/test.sh`
Expected: All existing tests pass.

- [ ] **Step 5: Commit**

```bash
git add engine/
git commit -m "feat(engine): wire ProxyProvider and RuleProvider into startup pipeline"
```
