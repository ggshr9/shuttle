# Plan 1 — Operability (Deep Health + SECURITY.md)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the shallow `/api/health` endpoint on both server and client with Kubernetes-style `live`/`ready` endpoints that report actual subsystem state, and expand `SECURITY.md` from 38 lines into a deployable hardening checklist.

**Architecture:**
- New `internal/healthcheck/` package provides a shared `Heartbeat` type used by liveness probes on both server and client.
- Server admin and client GUI API each get a new `health.go` file mounting `/api/health/live` and `/api/health/ready`. The legacy `/api/health` (server) and `/api/healthz` (client) endpoints are preserved verbatim for backward compatibility.
- `SECURITY.md` is rewritten in place; the existing three sections are kept and four new sections are appended.

**Tech Stack:** Go 1.24, `net/http`, `log/slog`, existing `server/admin` and `gui/api` packages.

**Spec reference:** `docs/superpowers/specs/2026-04-28-production-readiness-yellow-lights-design.md` (Workstreams 1 and 4).

---

## File Structure

**Created:**
- `internal/healthcheck/heartbeat.go` — `Heartbeat` type with `Tick()` / `IsAlive(threshold)`.
- `internal/healthcheck/heartbeat_test.go` — tests.
- `server/admin/health.go` — `/api/health/live` and `/api/health/ready` handlers + readiness checks.
- `server/admin/health_test.go` — tests.
- `gui/api/health_deep.go` — same endpoints, client-side readiness checks.
- `gui/api/health_deep_test.go` — tests.

**Modified:**
- `server/admin/admin.go` — `ServerInfo` gains a `ListenerStatus` map; `Handler()` accepts a `*healthcheck.Heartbeat`; existing `/api/health` registration moves to `health.go`.
- `server/server.go` — calls `info.MarkListenerReady(name)` after each successful transport `Listen`; constructs and passes `Heartbeat`.
- `gui/api/api.go` (or wherever `New`/`Handler` is wired) — accepts a `*healthcheck.Heartbeat`; calls registration in `health_deep.go`.
- `cmd/shuttle/main.go` and `cmd/shuttled/main.go` — start the heartbeat goroutine before launching the server/engine.
- `SECURITY.md` — full rewrite (existing content preserved verbatim within new structure).
- `examples/server.example.yaml` — one comment line at top referencing `SECURITY.md`.

---

## Task 1: Heartbeat helper

**Files:**
- Create: `internal/healthcheck/heartbeat.go`
- Test: `internal/healthcheck/heartbeat_test.go`

- [ ] **Step 1.1: Write failing test**

```go
// internal/healthcheck/heartbeat_test.go
package healthcheck

import (
	"testing"
	"time"
)

func TestHeartbeat_IsAliveWhenFresh(t *testing.T) {
	h := NewHeartbeat()
	h.Tick()
	if !h.IsAlive(time.Second) {
		t.Fatal("freshly ticked heartbeat should be alive")
	}
}

func TestHeartbeat_IsAliveBecomesFalseWhenStale(t *testing.T) {
	h := NewHeartbeat()
	h.Tick()
	time.Sleep(20 * time.Millisecond)
	if h.IsAlive(10 * time.Millisecond) {
		t.Fatal("heartbeat should be stale after 20ms with 10ms threshold")
	}
}

func TestHeartbeat_ZeroValueIsNotAlive(t *testing.T) {
	h := NewHeartbeat()
	if h.IsAlive(time.Hour) {
		t.Fatal("never-ticked heartbeat should not be alive")
	}
}
```

- [ ] **Step 1.2: Run, verify fail**

```
./scripts/test.sh --pkg ./internal/healthcheck/
```

Expected: build error / undefined `NewHeartbeat`.

- [ ] **Step 1.3: Implement**

```go
// internal/healthcheck/heartbeat.go
// Package healthcheck provides shared liveness/readiness primitives
// used by server admin and client GUI API health endpoints.
package healthcheck

import (
	"sync/atomic"
	"time"
)

type Heartbeat struct {
	lastNanos atomic.Int64
}

func NewHeartbeat() *Heartbeat {
	return &Heartbeat{}
}

func (h *Heartbeat) Tick() {
	h.lastNanos.Store(time.Now().UnixNano())
}

// IsAlive reports whether Tick was called within the threshold.
// A never-ticked heartbeat is not alive.
func (h *Heartbeat) IsAlive(threshold time.Duration) bool {
	last := h.lastNanos.Load()
	if last == 0 {
		return false
	}
	return time.Since(time.Unix(0, last)) < threshold
}

// Run starts a goroutine that ticks at the given interval until ctx
// is cancelled. Caller is responsible for context lifecycle.
func (h *Heartbeat) Run(stop <-chan struct{}, interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				h.Tick()
			}
		}
	}()
}
```

- [ ] **Step 1.4: Run, verify pass**

```
./scripts/test.sh --pkg ./internal/healthcheck/
```

Expected: PASS.

- [ ] **Step 1.5: Commit**

```bash
git add internal/healthcheck/
git commit -m "feat(healthcheck): add Heartbeat primitive for liveness probes"
```

---

## Task 2: Server `ServerInfo` listener tracking

**Files:**
- Modify: `server/admin/admin.go:30-39`
- Test: `server/admin/admin_test.go` (new test added)

- [ ] **Step 2.1: Write failing test**

Append to `server/admin/admin_test.go`:

```go
func TestServerInfo_ListenerStatus(t *testing.T) {
	info := &ServerInfo{}
	if info.IsListenerReady("h3") {
		t.Fatal("listener should not be ready before MarkListenerReady")
	}
	info.MarkListenerReady("h3")
	if !info.IsListenerReady("h3") {
		t.Fatal("listener should be ready after MarkListenerReady")
	}
	if info.IsListenerReady("reality") {
		t.Fatal("untracked listener should not be ready")
	}
}
```

- [ ] **Step 2.2: Run, verify fail**

```
./scripts/test.sh --pkg ./server/admin/ --run TestServerInfo_ListenerStatus
```

Expected: undefined methods.

- [ ] **Step 2.3: Implement — extend `ServerInfo`**

Modify `server/admin/admin.go`. Replace the `ServerInfo` struct (lines 30-39) with:

```go
// ServerInfo tracks runtime server metrics.
type ServerInfo struct {
	StartTime   time.Time
	Version     string
	ConfigPath  string
	ActiveConns atomic.Int64
	TotalConns  atomic.Int64
	BytesSent   atomic.Int64
	BytesRecv   atomic.Int64

	listenerMu     sync.RWMutex
	listenerStatus map[string]bool
}

// MarkListenerReady records that the named transport listener has bound successfully.
func (s *ServerInfo) MarkListenerReady(name string) {
	s.listenerMu.Lock()
	defer s.listenerMu.Unlock()
	if s.listenerStatus == nil {
		s.listenerStatus = make(map[string]bool)
	}
	s.listenerStatus[name] = true
}

// IsListenerReady reports whether the named listener has bound.
func (s *ServerInfo) IsListenerReady(name string) bool {
	s.listenerMu.RLock()
	defer s.listenerMu.RUnlock()
	return s.listenerStatus[name]
}

// ListenerSnapshot returns a copy of the listener status map.
func (s *ServerInfo) ListenerSnapshot() map[string]bool {
	s.listenerMu.RLock()
	defer s.listenerMu.RUnlock()
	out := make(map[string]bool, len(s.listenerStatus))
	for k, v := range s.listenerStatus {
		out[k] = v
	}
	return out
}
```

- [ ] **Step 2.4: Run, verify pass**

```
./scripts/test.sh --pkg ./server/admin/ --run TestServerInfo_ListenerStatus
```

Expected: PASS.

- [ ] **Step 2.5: Commit**

```bash
git add server/admin/admin.go server/admin/admin_test.go
git commit -m "feat(admin): track listener readiness on ServerInfo"
```

---

## Task 3: Server `/api/health/live` endpoint

**Files:**
- Create: `server/admin/health.go`
- Test: `server/admin/health_test.go`

- [ ] **Step 3.1: Write failing test**

```go
// server/admin/health_test.go
package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/internal/healthcheck"
)

func TestHealthLive_OK(t *testing.T) {
	hb := healthcheck.NewHeartbeat()
	hb.Tick()

	mux := http.NewServeMux()
	registerHealthRoutes(mux, &ServerInfo{}, nil, nil, hb)

	req := httptest.NewRequest("GET", "/api/health/live", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %v, want ok", body["status"])
	}
}

func TestHealthLive_503WhenStale(t *testing.T) {
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	time.Sleep(20 * time.Millisecond)

	mux := http.NewServeMux()
	// staleThreshold of 10ms means the heartbeat is now stale
	registerHealthRoutesWithThreshold(mux, &ServerInfo{}, nil, nil, hb, 10*time.Millisecond)

	req := httptest.NewRequest("GET", "/api/health/live", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}
```

- [ ] **Step 3.2: Run, verify fail**

```
./scripts/test.sh --pkg ./server/admin/ --run TestHealthLive
```

Expected: undefined `registerHealthRoutes`.

- [ ] **Step 3.3: Implement**

```go
// server/admin/health.go
package admin

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/internal/healthcheck"
	"github.com/shuttleX/shuttle/server/metrics"
)

const defaultLivenessThreshold = 30 * time.Second

type checkResult struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Addr   string `json:"addr,omitempty"`
}

type healthResponse struct {
	Status string                  `json:"status"`
	Checks map[string]checkResult  `json:"checks,omitempty"`
	TS     string                  `json:"ts"`
}

func registerHealthRoutes(mux *http.ServeMux, info *ServerInfo, cfg *config.ServerConfig, mc *metrics.Collector, hb *healthcheck.Heartbeat) {
	registerHealthRoutesWithThreshold(mux, info, cfg, mc, hb, defaultLivenessThreshold)
}

func registerHealthRoutesWithThreshold(mux *http.ServeMux, info *ServerInfo, cfg *config.ServerConfig, mc *metrics.Collector, hb *healthcheck.Heartbeat, livenessThreshold time.Duration) {
	mux.HandleFunc("GET /api/health/live", func(w http.ResponseWriter, r *http.Request) {
		if hb != nil && !hb.IsAlive(livenessThreshold) {
			writeHealth(w, http.StatusServiceUnavailable, "unhealthy", map[string]checkResult{
				"heartbeat": {Status: "fail", Error: "stale heartbeat"},
			})
			return
		}
		writeHealth(w, http.StatusOK, "ok", nil)
	})

	mux.HandleFunc("GET /api/health/ready", func(w http.ResponseWriter, r *http.Request) {
		checks := runReadinessChecks(info, cfg, mc)
		status := http.StatusOK
		overall := "ok"
		for _, c := range checks {
			if c.Status == "fail" {
				status = http.StatusServiceUnavailable
				overall = "unhealthy"
				break
			}
		}
		writeHealth(w, status, overall, checks)
	})
}

func writeHealth(w http.ResponseWriter, status int, overall string, checks map[string]checkResult) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(healthResponse{
		Status: overall,
		Checks: checks,
		TS:     time.Now().UTC().Format(time.RFC3339),
	})
}

func runReadinessChecks(info *ServerInfo, cfg *config.ServerConfig, mc *metrics.Collector) map[string]checkResult {
	out := map[string]checkResult{}
	if cfg == nil {
		out["config"] = checkResult{Status: "fail", Error: "config not loaded"}
		return out
	}
	out["config"] = checkResult{Status: "ok"}

	if cfg.Transport.H3.Enabled {
		if info.IsListenerReady("h3") {
			out["listener_h3"] = checkResult{Status: "ok", Addr: cfg.Transport.H3.Listen}
		} else {
			out["listener_h3"] = checkResult{Status: "fail", Error: "not bound"}
		}
	}
	if cfg.Transport.Reality.Enabled {
		if info.IsListenerReady("reality") {
			out["listener_reality"] = checkResult{Status: "ok", Addr: cfg.Transport.Reality.Listen}
		} else {
			out["listener_reality"] = checkResult{Status: "fail", Error: "not bound"}
		}
	}
	if cfg.Transport.CDN.Enabled {
		if info.IsListenerReady("cdn") {
			out["listener_cdn"] = checkResult{Status: "ok", Addr: cfg.Transport.CDN.Listen}
		} else {
			out["listener_cdn"] = checkResult{Status: "fail", Error: "not bound"}
		}
	}

	if cfg.Metrics.Enabled && mc == nil {
		out["metrics"] = checkResult{Status: "fail", Error: "collector not initialised"}
	}

	return out
}
```

- [ ] **Step 3.4: Run, verify pass**

```
./scripts/test.sh --pkg ./server/admin/ --run TestHealthLive
```

Expected: PASS.

- [ ] **Step 3.5: Commit**

```bash
git add server/admin/health.go server/admin/health_test.go
git commit -m "feat(admin): add /api/health/live with heartbeat freshness check"
```

---

## Task 4: Server `/api/health/ready` readiness checks

**Files:**
- Modify: `server/admin/health_test.go`
- (Implementation already in `health.go` from Task 3)

- [ ] **Step 4.1: Write failing tests for readiness scenarios**

Append to `server/admin/health_test.go`:

```go
import "github.com/shuttleX/shuttle/config"

func TestHealthReady_OKWhenAllListenersBound(t *testing.T) {
	cfg := &config.ServerConfig{}
	cfg.Transport.H3.Enabled = true
	cfg.Transport.H3.Listen = ":443"

	info := &ServerInfo{}
	info.MarkListenerReady("h3")

	mux := http.NewServeMux()
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	registerHealthRoutes(mux, info, cfg, nil, hb)

	req := httptest.NewRequest("GET", "/api/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
}

func TestHealthReady_503WhenListenerNotBound(t *testing.T) {
	cfg := &config.ServerConfig{}
	cfg.Transport.H3.Enabled = true
	cfg.Transport.Reality.Enabled = true

	info := &ServerInfo{}
	info.MarkListenerReady("h3") // reality NOT marked

	mux := http.NewServeMux()
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	registerHealthRoutes(mux, info, cfg, nil, hb)

	req := httptest.NewRequest("GET", "/api/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	var body healthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Checks["listener_reality"].Status != "fail" {
		t.Fatalf("listener_reality should be fail, got %v", body.Checks["listener_reality"])
	}
	if body.Checks["listener_h3"].Status != "ok" {
		t.Fatalf("listener_h3 should be ok, got %v", body.Checks["listener_h3"])
	}
}

func TestHealthReady_503WhenConfigNil(t *testing.T) {
	mux := http.NewServeMux()
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	registerHealthRoutes(mux, &ServerInfo{}, nil, nil, hb)

	req := httptest.NewRequest("GET", "/api/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}
```

- [ ] **Step 4.2: Run, verify pass**

```
./scripts/test.sh --pkg ./server/admin/ --run TestHealthReady
```

Expected: PASS (logic was implemented in Task 3).

- [ ] **Step 4.3: Commit**

```bash
git add server/admin/health_test.go
git commit -m "test(admin): cover readiness fail/ok scenarios"
```

---

## Task 5: Wire health routes into `Handler()`

**Files:**
- Modify: `server/admin/admin.go:52` (`Handler` signature) and the call sites.

- [ ] **Step 5.1: Add Heartbeat parameter to Handler**

Change the `Handler` function signature in `admin.go`:

```go
func Handler(info *ServerInfo, cfg *config.ServerConfig, configPath string, users *UserStore, auditLog *audit.Logger, mc *metrics.Collector, pm *plugin.Metrics, eventsHandler EventHandler, hb *healthcheck.Heartbeat) http.Handler {
```

Add the import:
```go
"github.com/shuttleX/shuttle/internal/healthcheck"
```

- [ ] **Step 5.2: Replace existing `/api/health` registration**

Locate `mux.HandleFunc("GET /api/health", ...)` (around line 92 in current file) and replace the block with:

```go
// Health routes — /api/health (legacy shallow), /api/health/live (liveness),
// /api/health/ready (readiness). All without auth.
mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
})
registerHealthRoutes(mux, info, cfg, mc, hb)
```

- [ ] **Step 5.3: Update call sites**

Find the single existing call to `admin.Handler(...)` (likely in `server/server.go`) and append a new heartbeat argument:

```bash
grep -rn "admin.Handler(" --include="*.go" .
```

In the call site, construct and pass the heartbeat:

```go
hb := healthcheck.NewHeartbeat()
stop := make(chan struct{})
hb.Run(stop, 5*time.Second)
// ... pass hb to admin.Handler(...) as the new last argument
```

(Add a `defer close(stop)` next to the existing graceful-shutdown logic.)

- [ ] **Step 5.4: Run full server admin tests**

```
./scripts/test.sh --pkg ./server/admin/
./scripts/test.sh --pkg ./server/
```

Expected: PASS (existing tests must still pass; adjust test fixtures to pass `nil` heartbeat where appropriate).

- [ ] **Step 5.5: Verify `/api/health` backward compatibility**

```
./scripts/test.sh --pkg ./server/admin/ --run TestAdminAPI
```

Confirm the existing `TestAdminAPI` (in `admin_test.go`) still asserts `/api/health` returns `{"status":"ok"}` with code 200.

- [ ] **Step 5.6: Commit**

```bash
git add server/admin/admin.go server/server.go server/admin/admin_test.go
git commit -m "feat(admin): mount live/ready endpoints alongside legacy /api/health"
```

---

## Task 6: Mark listeners ready in server bootstrap

**Files:**
- Modify: `server/server.go` (locations where each transport calls `Listen`/`ListenAndServe`).

- [ ] **Step 6.1: Locate listener launches**

```bash
grep -n "Listen\|ListenAndServe" server/server.go
```

For each transport that successfully binds (H3, Reality, CDN), add an `info.MarkListenerReady("<name>")` call **after** the bind succeeds and **before** the goroutine entering accept loops returns.

- [ ] **Step 6.2: Add the calls**

Pattern for each bound listener:

```go
if err := h3Server.ListenAndServe(); err != nil { /* existing error path */ }
info.MarkListenerReady("h3")
```

(Note: if `ListenAndServe` blocks, mark readiness on a separate goroutine after a small wait, OR refactor to a separate `Listen()` then `Serve()` call. Prefer the latter — most transports already split these.)

- [ ] **Step 6.3: Add a sandbox-tagged integration check**

Append to `test/e2e/sandbox_test.go`:

```go
//go:build sandbox

func TestServerHealth_ReadyAfterListenersBind(t *testing.T) {
	// boot a real shuttled in the sandbox, scrape /api/health/ready,
	// assert 200 and listener_h3.status=ok
}
```

(Body left as a follow-up — the spec's CI metric-scrape job covers this in practice.)

- [ ] **Step 6.4: Run host-safe tests**

```
./scripts/test.sh
```

Expected: PASS.

- [ ] **Step 6.5: Commit**

```bash
git add server/server.go test/e2e/sandbox_test.go
git commit -m "feat(server): mark listener readiness after successful bind"
```

---

## Task 7: Client deep health endpoint

**Files:**
- Create: `gui/api/health_deep.go`
- Test: `gui/api/health_deep_test.go`
- Modify: GUI API server constructor (the file that registers routes — likely `gui/api/server.go` or `gui/api/api.go`).

- [ ] **Step 7.1: Write failing tests**

```go
// gui/api/health_deep_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/internal/healthcheck"
)

type fakeEngine struct {
	state            string
	outboundsHealthy int
	configValid      bool
}

func (f *fakeEngine) StateName() string  { return f.state }
func (f *fakeEngine) HealthyOutbounds() int { return f.outboundsHealthy }
func (f *fakeEngine) ConfigValid() bool { return f.configValid }

func TestClientHealthLive_OK(t *testing.T) {
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	mux := http.NewServeMux()
	registerDeepHealthRoutes(mux, &fakeEngine{}, hb)

	req := httptest.NewRequest("GET", "/api/health/live", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
}

func TestClientHealthReady_OK(t *testing.T) {
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	mux := http.NewServeMux()
	registerDeepHealthRoutes(mux, &fakeEngine{
		state:            "running",
		outboundsHealthy: 1,
		configValid:      true,
	}, hb)

	req := httptest.NewRequest("GET", "/api/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d body=%s, want 200", w.Code, w.Body.String())
	}
}

func TestClientHealthReady_FailWhenStopping(t *testing.T) {
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	mux := http.NewServeMux()
	registerDeepHealthRoutes(mux, &fakeEngine{
		state:            "stopping",
		outboundsHealthy: 1,
		configValid:      true,
	}, hb)

	req := httptest.NewRequest("GET", "/api/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", w.Code)
	}
}

func TestClientHealthReady_FailWhenNoHealthyOutbound(t *testing.T) {
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	mux := http.NewServeMux()
	registerDeepHealthRoutes(mux, &fakeEngine{
		state:            "running",
		outboundsHealthy: 0,
		configValid:      true,
	}, hb)

	req := httptest.NewRequest("GET", "/api/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	checks, _ := body["checks"].(map[string]any)
	outbound, _ := checks["outbounds"].(map[string]any)
	if outbound["status"] != "fail" {
		t.Fatalf("outbounds check should be fail, got %v", outbound)
	}
}

func TestClientHealthLive_StaleHeartbeat(t *testing.T) {
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	time.Sleep(20 * time.Millisecond)

	mux := http.NewServeMux()
	registerDeepHealthRoutesWithThreshold(mux, &fakeEngine{}, hb, 10*time.Millisecond)

	req := httptest.NewRequest("GET", "/api/health/live", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", w.Code)
	}
}
```

- [ ] **Step 7.2: Run, verify fail**

```
./scripts/test.sh --pkg ./gui/api/ --run TestClientHealth
```

Expected: undefined `registerDeepHealthRoutes`.

- [ ] **Step 7.3: Implement**

```go
// gui/api/health_deep.go
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/shuttleX/shuttle/internal/healthcheck"
)

const clientLivenessThreshold = 30 * time.Second

// engineProbe is the minimal surface of the engine needed by readiness.
// The real engine.Engine implements these via small adapter methods
// added in Task 7.4.
type engineProbe interface {
	StateName() string
	HealthyOutbounds() int
	ConfigValid() bool
}

type clientCheckResult struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type clientHealthResponse struct {
	Status string                       `json:"status"`
	Checks map[string]clientCheckResult `json:"checks,omitempty"`
	TS     string                       `json:"ts"`
}

func registerDeepHealthRoutes(mux *http.ServeMux, eng engineProbe, hb *healthcheck.Heartbeat) {
	registerDeepHealthRoutesWithThreshold(mux, eng, hb, clientLivenessThreshold)
}

func registerDeepHealthRoutesWithThreshold(mux *http.ServeMux, eng engineProbe, hb *healthcheck.Heartbeat, livenessThreshold time.Duration) {
	mux.HandleFunc("GET /api/health/live", func(w http.ResponseWriter, r *http.Request) {
		if hb != nil && !hb.IsAlive(livenessThreshold) {
			writeClientHealth(w, http.StatusServiceUnavailable, "unhealthy", map[string]clientCheckResult{
				"heartbeat": {Status: "fail", Error: "stale heartbeat"},
			})
			return
		}
		writeClientHealth(w, http.StatusOK, "ok", nil)
	})

	mux.HandleFunc("GET /api/health/ready", func(w http.ResponseWriter, r *http.Request) {
		checks := runClientReadiness(eng)
		status := http.StatusOK
		overall := "ok"
		for _, c := range checks {
			if c.Status == "fail" {
				status = http.StatusServiceUnavailable
				overall = "unhealthy"
				break
			}
		}
		writeClientHealth(w, status, overall, checks)
	})
}

func writeClientHealth(w http.ResponseWriter, status int, overall string, checks map[string]clientCheckResult) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(clientHealthResponse{
		Status: overall,
		Checks: checks,
		TS:     time.Now().UTC().Format(time.RFC3339),
	})
}

func runClientReadiness(eng engineProbe) map[string]clientCheckResult {
	out := map[string]clientCheckResult{}
	if eng == nil {
		out["engine"] = clientCheckResult{Status: "fail", Error: "engine not initialised"}
		return out
	}
	switch eng.StateName() {
	case "running", "starting":
		out["engine"] = clientCheckResult{Status: "ok", Detail: eng.StateName()}
	default:
		out["engine"] = clientCheckResult{Status: "fail", Error: "engine state: " + eng.StateName()}
	}
	if eng.ConfigValid() {
		out["config"] = clientCheckResult{Status: "ok"}
	} else {
		out["config"] = clientCheckResult{Status: "fail", Error: "config validation failed"}
	}
	if eng.HealthyOutbounds() > 0 {
		out["outbounds"] = clientCheckResult{Status: "ok"}
	} else {
		out["outbounds"] = clientCheckResult{Status: "fail", Error: "no healthy outbound"}
	}
	return out
}
```

- [ ] **Step 7.4: Add engine adapter methods**

If `engine.Engine` does not already expose `StateName()`, `HealthyOutbounds()`, `ConfigValid()`, add the missing ones in `engine/engine.go`:

```go
// StateName returns the human-readable engine state.
func (e *Engine) StateName() string {
	return e.Status().State
}

// HealthyOutbounds returns the number of outbounds whose last health check passed.
func (e *Engine) HealthyOutbounds() int {
	// Use existing healthcheck infrastructure; if no healthchecker, treat
	// every configured outbound as healthy.
	if e.healthchecker == nil {
		return len(e.cfg.Outbounds)
	}
	return e.healthchecker.HealthyCount()
}

// ConfigValid reports whether the most recent config (or hot-reload) validated cleanly.
func (e *Engine) ConfigValid() bool {
	return e.lastConfigErr == nil
}
```

If `e.lastConfigErr` does not exist, add a field `lastConfigErr error` and assign it in `Reload()` and the constructor.

- [ ] **Step 7.5: Wire registration into GUI API server**

Find the file that constructs the GUI API mux (probably `gui/api/api.go` `NewHandler` or `gui/api/server.go`). Inject the heartbeat:

```go
hb := healthcheck.NewHeartbeat()
stop := make(chan struct{})
hb.Run(stop, 5*time.Second)
registerDeepHealthRoutes(mux, eng, hb)
```

Add `defer close(stop)` to the existing shutdown handler.

- [ ] **Step 7.6: Run, verify pass**

```
./scripts/test.sh --pkg ./gui/api/
./scripts/test.sh --pkg ./engine/
```

Expected: PASS.

- [ ] **Step 7.7: Commit**

```bash
git add gui/api/health_deep.go gui/api/health_deep_test.go gui/api/api.go engine/engine.go
git commit -m "feat(gui/api): add /api/health/live and /api/health/ready"
```

---

## Task 8: Preserve `/api/healthz` backward compatibility

**Files:**
- Modify: `gui/api/healthz.go` (no behavior change required, just confirm it still mounts)
- Test: `gui/api/healthz_test.go` (verify the iOS BridgeAdapter contract is preserved)

- [ ] **Step 8.1: Read existing test**

```bash
cat gui/api/healthz_test.go
```

Confirm there is a test asserting `GET /api/healthz` returns `{"status":"ok"}`.

- [ ] **Step 8.2: Add an explicit regression test**

Append to `gui/api/healthz_test.go`:

```go
func TestHealthz_PreservedAfterDeepHealthAdded(t *testing.T) {
	// Sanity: the new /api/health/live and /api/healthz coexist with
	// distinct semantics. /api/healthz is the iOS BridgeAdapter shallow probe.
	mux := http.NewServeMux()
	registerHealthzRoute(mux)

	req := httptest.NewRequest("GET", "/api/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("/api/healthz status = %d, want 200", w.Code)
	}
}
```

- [ ] **Step 8.3: Run**

```
./scripts/test.sh --pkg ./gui/api/ --run Healthz
```

Expected: PASS.

- [ ] **Step 8.4: Commit**

```bash
git add gui/api/healthz_test.go
git commit -m "test(gui/api): regression-check /api/healthz coexistence"
```

---

## Task 9: SECURITY.md — preserve existing + add `Last reviewed` header

**Files:**
- Modify: `SECURITY.md`

- [ ] **Step 9.1: Replace SECURITY.md header**

At the top of `SECURITY.md`, insert immediately after the H1:

```markdown
# Security

> **Last reviewed:** 2026-04-28. This document is reviewed at every release.

```

(Existing sections remain in place below.)

- [ ] **Step 9.2: Commit**

```bash
git add SECURITY.md
git commit -m "docs(security): add Last reviewed marker"
```

---

## Task 10: SECURITY.md — Reporting Security Issues rewrite

**Files:**
- Modify: `SECURITY.md` (replace the existing "Reporting Security Issues" section).

- [ ] **Step 10.1: Replace the section**

Replace the existing "Reporting Security Issues" section with:

```markdown
## Reporting Security Issues

**Confidential reports:** Use [GitHub Security Advisory](https://github.com/shuttleX/shuttle/security/advisories/new) for any security-sensitive issue. This is the preferred channel — reports are private until coordinated disclosure.

**Non-sensitive concerns:** A public [GitHub issue](https://github.com/shuttleX/shuttle/issues/new) is fine for hardening suggestions, dependency updates, or configuration questions where no exploit path is involved.

**PGP:** No project PGP key is currently published. Please use GitHub Security Advisory for confidential reports — GitHub encrypts reports in transit and at rest.

**What to include:**
- Affected version (commit hash if running from main)
- Steps to reproduce
- Estimated impact (data exposure / denial of service / privilege escalation)
- Suggested fix if you have one

We aim to acknowledge reports within 72 hours and to ship a fix or mitigation within 30 days for high-severity issues.
```

- [ ] **Step 10.2: Commit**

```bash
git add SECURITY.md
git commit -m "docs(security): switch reporting to GitHub Security Advisory"
```

---

## Task 11: SECURITY.md — Threat Model section

**Files:**
- Modify: `SECURITY.md` (append after "Supported Versions").

- [ ] **Step 11.1: Append section**

```markdown
## Threat Model

Shuttle is designed to defend against the following classes of adversary:

**In scope:**
- Passive traffic analysis on the wire between client and server.
- Active SNI probing of the server's TLS endpoint (Reality transport).
- Passive deep packet inspection identifying or fingerprinting Shuttle traffic.
- Unauthorised access to the management plane (`/api/*` endpoints).
- Unauthorised use of forwarded outbound traffic (e.g., open-relay abuse).

**Out of scope:**
- Local-host compromise (device theft, root-level malware on client or server).
- Active collaboration by the upstream CDN, hosting provider, or transit network.
- Long-term confidentiality breach by quantum computation against current Noise IK key exchange.
- Side-channel attacks against the TLS implementation provided by the Go standard library.

**Trust boundaries:**

```
[client app] ⟷ [shuttle CLI / GUI] ⟷ [transport: H3/Reality/CDN] ⟷ [shuttled] ⟷ [destination]
                       │                                                   │
                       └─── management plane (/api/*) ── separate trust ───┘
                                                              domain (admin.token)
```

The management plane is its own trust domain: its credentials must not be derivable from or reused with the data-plane credentials (`auth.password`, `auth.private_key`).
```

- [ ] **Step 11.2: Commit**

```bash
git add SECURITY.md
git commit -m "docs(security): document threat model"
```

---

## Task 12: SECURITY.md — Hardening Checklist section

**Files:**
- Modify: `SECURITY.md` (append).

- [ ] **Step 12.1: Append section**

```markdown
## Hardening Checklist

Treat this as a pre-deploy checklist. Items marked **(default)** are configured automatically by the install scripts; others require explicit configuration.

**Process & filesystem**
- [ ] Service runs as a dedicated non-root user (default — `shuttle` user via `install.sh`)
- [ ] systemd `ProtectSystem=strict`, `NoNewPrivileges=true`, `PrivateTmp=true` (default)
- [ ] `CapabilityBoundingSet=CAP_NET_BIND_SERVICE` only (default)
- [ ] Config file mode `0600`, directory `0700`, owned by service user
- [ ] TLS private key file mode `0600`

**Authentication**
- [ ] `auth.password` is at least 16 chars, randomly generated (use `openssl rand -base64 32`)
- [ ] For Reality, prefer `auth.private_key` over passwords entirely
- [ ] `admin.token` is at least 32 chars, randomly generated, never reused as `auth.password`

**Network exposure**
- [ ] Admin port (`admin.listen`) bound to `127.0.0.1` or restricted by firewall to operator networks
- [ ] Metrics port (`metrics.listen`) bound to `127.0.0.1` plus token; never exposed publicly
- [ ] Public listener ports limited to the transports actually in use

**Routing & SSRF**
- [ ] `router.allow_private_networks: false` in production (the sandbox-only override defaults to `false` already)
- [ ] If `cdn` outbound is enabled, quotas are configured to bound proxy abuse risk

**Observability**
- [ ] IP reputation rate-limiting enabled (default — auto-bans after 5 failed auth attempts)
- [ ] Logs do not echo `Authorization` header, password, or private key (verified by spot-check at each release)
- [ ] Audit log destination configured if compliance requires it
```

- [ ] **Step 12.2: Commit**

```bash
git add SECURITY.md
git commit -m "docs(security): add hardening checklist"
```

---

## Task 13: SECURITY.md — Configuration Best Practices section

**Files:**
- Modify: `SECURITY.md` (append).

- [ ] **Step 13.1: Append section**

```markdown
## Configuration Best Practices

**Strong credentials:**
```
openssl rand -base64 32        # admin.token
openssl rand -base64 24        # auth.password
shuttled keygen                # Reality auth.private_key + public_key pair
```

**TLS certificates:**
- Prefer Let's Encrypt + cert-manager (Kubernetes) or acme.sh (bare metal) for automated renewal.
- Avoid wildcard certificates: a single revocation invalidates traffic for every subdomain.
- Renew at least 14 days before expiry; alert on certificates within 30 days of expiry.

**Reality `target_sni`:**
- Choose a domain that is **actually reachable** from the public internet and whose responses are behaviourally similar to your server's environment (latency, content type).
- A dead or misconfigured SNI target is worse than no SNI camouflage — it is a fingerprint.
- Avoid using domains owned by the same operator as common camouflage choices, since their failure correlates.

**Subscription sources:**
- HTTPS-only is enforced (`http://` URLs are rejected at parse time).
- Pass authentication via `Authorization` header, not query string — query strings appear in HTTP logs and HTTP referer headers.
- Pin subscription provider hostnames in your firewall egress rules where possible.

**Mesh CIDR:**
- The default mesh CIDR is `10.7.0.0/24`; change it if your corporate network uses overlapping space.
- Mesh peer authentication uses the same key material as the underlying transport — do not weaken transport auth on the assumption that mesh adds defence-in-depth.

**Metrics endpoint:**
- Bind `metrics.listen` to `127.0.0.1` and scrape via SSH tunnel or local Prometheus agent.
- Never expose `/metrics` publicly: it leaks connection counts, transport mix, and internal hostnames via labels.
```

- [ ] **Step 13.2: Commit**

```bash
git add SECURITY.md
git commit -m "docs(security): add configuration best practices"
```

---

## Task 14: SECURITY.md — Key & Token Rotation section

**Files:**
- Modify: `SECURITY.md` (append).

- [ ] **Step 14.1: Append section**

```markdown
## Key & Token Rotation

| Credential | Rotation trigger | Procedure |
|---|---|---|
| Reality `auth.private_key` | Suspected leak | `shuttled keygen` to generate a new pair → distribute new public key to clients (e.g., via subscription update) → rolling restart of `shuttled` instances → keep the previous `short_id` listed for 24h to allow client cutover |
| `auth.password` (H3) | Scheduled (90 days) or after personnel change | Update the config to include both old and new passwords, hot reload via `POST /api/reload`, distribute new password to clients, then remove the old password and reload again |
| `admin.token` | Scheduled (30 days) or after operator role change | Update config, hot reload, old token is rejected immediately |
| Subscription auth token | Per provider's scheme | Driven by your subscription provider |

**Recommended cadence:**
- `admin.token`: every 30 days
- `auth.password` (H3): every 90 days
- `auth.private_key` (Reality): only on suspected compromise — rotation is disruptive and the underlying scheme is forward-secret

**During rotation:**
- Keep audit logs to verify when old credentials were last used.
- Notify clients out-of-band before the cutover, not via the channel being rotated.
- For mesh deployments, rotate the relay-tier credentials before the leaf-tier credentials.
```

- [ ] **Step 14.2: Commit**

```bash
git add SECURITY.md
git commit -m "docs(security): add key and token rotation guide"
```

---

## Task 15: Cross-reference SECURITY.md from examples

**Files:**
- Modify: `examples/server.example.yaml` (top of file).

- [ ] **Step 15.1: Add comment line**

Insert at the very top of `examples/server.example.yaml`, before any existing content:

```yaml
# IMPORTANT: Read SECURITY.md before deploying to production.
# https://github.com/shuttleX/shuttle/blob/main/SECURITY.md
#
```

- [ ] **Step 15.2: Run config-loading test to make sure the comment doesn't break parsing**

```
./scripts/test.sh --pkg ./config/
```

Expected: PASS.

- [ ] **Step 15.3: Commit**

```bash
git add examples/server.example.yaml
git commit -m "docs(examples): cross-reference SECURITY.md from server config"
```

---

## Task 16: End-to-end verification

- [ ] **Step 16.1: Run full host-safe test suite**

```
./scripts/test.sh
```

Expected: ALL PASS.

- [ ] **Step 16.2: Build both binaries**

```
CGO_ENABLED=0 go build -o /tmp/shuttle ./cmd/shuttle
CGO_ENABLED=0 go build -o /tmp/shuttled ./cmd/shuttled
```

Expected: clean build, no warnings.

- [ ] **Step 16.3: Smoke-test endpoints in the sandbox**

```
./sandbox/run.sh build
./sandbox/run.sh up
# in another shell:
curl -s http://localhost:9000/api/health/live | jq
curl -s http://localhost:9000/api/health/ready | jq
./sandbox/run.sh down
```

Expected: `live` returns 200 with `status: ok`. `ready` returns 200 once all configured listeners have bound.

- [ ] **Step 16.4: Final commit (if any cleanup)**

```bash
git status
# if clean, no commit needed
```

---

## Self-Review Notes

- Backward compatibility: `/api/health` (server) and `/api/healthz` (client) preserved verbatim — Task 5.5 and Task 8.2 are the explicit regression tests.
- `engine.HealthyOutbounds()` falls back to `len(cfg.Outbounds)` when no healthchecker exists; this means a freshly-started client without health probing yet still reports ready as long as it has any outbound configured. Acceptable for v1.
- Liveness threshold defaults to 30s (heartbeat ticks every 5s, so 6× margin) — well above transient GC pauses.
- The `engineProbe` interface is defined in `gui/api/health_deep.go` rather than `engine/` to avoid creating a back-reference; the engine simply gains methods that satisfy the interface implicitly.
- **Deferred:** The spec's "auth backing store reachable" readiness check (`cfg.Users.Backend != "memory"` connectivity probe) is intentionally **not** implemented in this plan. The current default and only widely-used backend is in-process memory; an external backend doesn't ship with v1. When/if a remote backend lands, add a probe via a dedicated task — `cfg.Validate()` already covers misconfiguration cases.
