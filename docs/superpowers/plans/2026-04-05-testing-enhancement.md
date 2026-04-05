# Testing Enhancement Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add E2E sandbox tests for all new features, Playwright GUI tests, and 100-concurrent load test.

**Decisions:** A (E2E first), A (Playwright), A (100 concurrent)

---

## Phase A: E2E Sandbox Tests for New Features (4 tasks)

All tests use `//go:build sandbox` tag. Run in Docker via `./sandbox/run.sh gotest`.

### Task A1: Multi-outbound + Quality Routing E2E

**File:** `test/e2e/outbound_test.go`

Tests:
- `TestSandboxMultiOutbound` — configure 2 proxy outbounds (client-a → server via different paths), verify routing rules direct traffic to correct outbound
- `TestSandboxOutboundGroupFailover` — configure group with primary (working) + secondary, kill primary, verify failover
- `TestSandboxOutboundGroupQuality` — configure quality group, verify lowest latency server is preferred (use netem to add latency to one path)

### Task A2: Selector Hot-Switch + Migration E2E

**File:** `test/e2e/migration_test.go`

Tests:
- `TestSandboxSetStrategy` — switch strategy via API while proxying traffic, verify no connection drop
- `TestSandboxProactiveMigration` — enable proactive_migration, use netem to simulate network change, verify migration event emitted
- `TestSandboxConfigHotReloadStrategy` — reload config with different strategy, verify switch without restart

### Task A3: Subscription E2E

**File:** `test/e2e/subscription_test.go`

Tests:
- `TestSandboxSubscriptionClash` — serve Clash YAML on httpbin, add subscription, verify servers parsed
- `TestSandboxSubscriptionSingbox` — serve sing-box JSON on httpbin, add subscription, verify servers parsed
- `TestSandboxSubscriptionAutoRefresh` — add subscription, wait for auto-refresh, verify updated

### Task A4: Server Plugin Chain + Events E2E

**File:** `test/e2e/server_test.go`

Tests:
- `TestSandboxServerMetrics` — proxy traffic through server, check `/api/metrics` shows plugin chain stats
- `TestSandboxServerSSEEvents` — connect SSE at `/api/events`, proxy traffic, verify connected/disconnected events

---

## Phase B: Playwright GUI Tests (3 tasks)

### Task B1: SimpleMode Tests

**File:** `gui/web/tests/simple-mode.spec.ts`

Tests:
- Navigate to app, verify SimpleMode renders by default
- Click connect button, verify status changes
- Click "Advanced Mode", verify tab layout appears
- Switch back to Simple Mode from Settings

### Task B2: Mesh Page Tests

**File:** `gui/web/tests/mesh.spec.ts`

Tests:
- Navigate to Mesh tab, verify page renders
- Verify peer table is visible (may be empty)
- Verify status card shows mesh state

### Task B3: Subscription Management Tests

**File:** `gui/web/tests/subscriptions.spec.ts`

Tests:
- Navigate to Subscriptions tab
- Add subscription URL, verify it appears in list
- Refresh subscription, verify no error
- Delete subscription, verify removal

---

## Phase C: Load Test (1 task)

### Task C1: 100-Concurrent Load Test

**File:** `test/e2e/load_test.go` (`//go:build sandbox`)

Tests:
- `TestSandboxLoad100Concurrent` — open 100 concurrent SOCKS5 connections through proxy, each making HTTP requests to httpbin, verify all complete within 30s, measure p50/p95/p99 latency
- `TestSandboxLoadSustained` — 50 connections sustained for 60s, verify zero connection drops, memory stable
