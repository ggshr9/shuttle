# Production Readiness — Yellow-Light Resolution Design

**Date:** 2026-04-28
**Status:** Draft (pending review)
**Scope:** Close the three "yellow-light" gaps identified in the v0.4 readiness audit, so Shuttle can ship v1.0 with confidence.

## Background

The 2026-04-28 production-readiness audit rated Shuttle at ~80% maturity, with three categories blocking a clean v1.0 release:

1. **Observability** — health endpoints exist but are shallow; Prometheus coverage is uneven between server (~14 metrics) and client (~8 metrics); no router/CB/subscription visibility.
2. **Install experience** — Linux has a polished interactive installer (`deploy/install.sh`); macOS and Windows do not have CLI install paths (only GUI bundles).
3. **Security documentation** — `SECURITY.md` is 38 lines covering only download verification, issue reporting, and supported versions. No threat model, no hardening checklist, no rotation guidance.

OpenTelemetry tracing was deliberately deferred — it adds dependency weight without clear payoff for an end-user proxy tool. Users running Shuttle at scale already integrate via Prometheus.

## Goals

- Production-grade health checks distinguishing **liveness** (is the process alive?) from **readiness** (can it serve traffic?).
- Prometheus metrics that drive a useful Grafana dashboard for both server and client deployments.
- A `SECURITY.md` that an operator can use as a pre-deploy checklist.
- Single-command installation on Linux / macOS / Windows for headless CLI use.
- All five workstreams must land without breaking existing API consumers (`engine.Status()`, `/api/health`, existing metric names).

## Non-Goals

- OpenTelemetry tracing or distributed tracing of any kind.
- Bug bounty or formal vulnerability disclosure programme.
- Chocolatey / Scoop / winget / Snap / Flatpak packages (community can submit later).
- macOS `.pkg` installer (Homebrew covers the headless-server use case; the existing `.app` covers desktop users).
- Adopting `prometheus/client_golang` — current zero-dep text-format approach is preserved.

## Architecture Overview

Five orthogonal workstreams. Workstreams 1 & 4 are small; 2 & 3 share a metrics-emission pattern; 5 is independent of the rest.

| # | Workstream | Files touched | Side |
|---|---|---|---|
| 1 | Deep health & readiness | `server/admin/health.go` (new), `gui/api/health_deep.go` (new) | both |
| 2 | Server metrics expansion | `server/metrics/metrics.go`, hooks in `router/`, `engine/circuit.go`, `subscription/` | server |
| 3 | Client metrics expansion | `gui/api/routes_prometheus.go`, new `engine.Metrics()` accessor | client |
| 4 | `SECURITY.md` expansion | `SECURITY.md`, `examples/server.example.yaml` (one comment line) | docs |
| 5 | Cross-platform CLI installers | `Formula/*.rb` in new `homebrew-shuttle` tap repo, `scripts/install-windows.ps1`, README install section | release/docs |

### Cross-cutting principles

- **Backward compatibility:** `/api/health` keeps its `{"status":"ok"}` response. `engine.Status()` keeps every existing field; metrics are added via a new `engine.Metrics()` accessor.
- **No new runtime dependencies:** Server-side metrics continue to use the in-house Prometheus text-format writer in `server/metrics/metrics.go`. Client-side `gui/api/routes_prometheus.go` does the same.
- **Hook injection over scattered call sites:** New metrics receive their data through registered hooks (`router.SetDecisionHook`, CB `OnStateChange`, subscription event bus) rather than `metrics.Inc(...)` calls peppered across the codebase.

## Workstream 1 — Deep Health & Readiness

### Endpoints

Three endpoints per side (server admin + client API), Kubernetes-style:

| Path | Semantics | 200 conditions | Failure code |
|---|---|---|---|
| `GET /api/health` | Compatibility shim, shallow liveness | Process alive | — (always 200 unless dead) |
| `GET /api/health/live` | Liveness | Process alive AND main goroutine responsive within timeout | 503 |
| `GET /api/health/ready` | Readiness | Config validated AND all enabled inbound listeners bound AND outbound has at least one healthy upstream | 503 with JSON details |

### Readiness check matrix

**Liveness check (both sides):** A select with a 1-second timeout against a heartbeat channel updated by a low-priority goroutine. If the heartbeat is stale (no update in ≥10s) the runtime is considered wedged → 503.

**Server (`shuttled`) readiness:**
1. Config loaded — `cfg != nil` and `cfg.Validate()` returns nil.
2. Listeners bound — for each enabled transport in `cfg.Transport.{H3,Reality,CDN}`, the corresponding listener is registered in `info` (server start sets these refs after a successful `Listen`).
3. Metrics collector — `mc != nil` if `cfg.Metrics.Enabled`.
4. Auth backing store — if `cfg.Users.Backend != "memory"`, the backend connection is reachable.

**Client (`shuttle` / `shuttle-gui`) readiness:**
1. Engine state — `eng.Status().State` is one of `Running` / `Starting` (not `Stopping` / `Error`).
2. Outbound health — at least one outbound passed its last `outbound/healthcheck/` probe.
3. Config — last reload returned no validation error.

### Response format (unified)

```json
{
  "status": "ok|degraded|unhealthy",
  "checks": {
    "config":           {"status": "ok"},
    "listener_h3":      {"status": "ok",   "addr": ":443"},
    "listener_reality": {"status": "fail", "error": "bind: address in use"}
  },
  "ts": "2026-04-28T10:30:00Z"
}
```

`ts` is RFC3339 with second precision. `status: degraded` is reserved for future use (e.g., partial outbound failure) — not emitted in v1; the v1 binary returns `ok` or `unhealthy` only.

### Implementation layout

- New file `server/admin/health.go` with the route handlers; `admin.go` only mounts the routes.
- New file `gui/api/health_deep.go` with the client-side handlers; `healthz.go` keeps its iOS BridgeAdapter probe semantics unchanged.
- Shared check helpers extracted to `internal/healthcheck/` if (and only if) ≥3 helpers overlap between sides — otherwise keep them local.

### Tests

- Listener-failure simulation: bring up engine with H3 enabled but pre-bound port → `/api/health/ready` returns 503 with `listener_h3.status="fail"`.
- Stopping state: engine in `Stopping` → readiness 503, liveness 200.
- Backward compat: `/api/health` still returns `{"status":"ok"}` regardless of inner state.
- Concurrent scrape (`-race`): readiness handler is safe under 100 parallel hits.

## Workstream 2 — Server Metrics Expansion

The server is a relay: it accepts inbound transport connections, authenticates, and forwards to destinations. It does **not** make routing decisions, hold outbound circuit breakers, or manage subscriptions — those are client-side concepts (covered in Workstream 3).

### New metrics (added to `server/metrics/metrics.go` `Collector`)

| Metric | Type | Labels | Hook |
|---|---|---|---|
| `shuttle_handshake_duration_seconds` | histogram | `transport` ∈ {h3, reality, cdn} | transport `Accept()` completion (server side) |
| `shuttle_handshake_failures_total` | counter | `transport`, `reason` ∈ {timeout, auth, protocol} | transport error returns |
| `shuttle_dns_query_duration_seconds` | histogram | `protocol` ∈ {udp, system}, `cached` ∈ {true, false} | server destination resolution |
| `shuttle_destination_resolve_failures_total` | counter | `reason` ∈ {nxdomain, timeout, refused} | server DNS error path |
| `shuttle_user_active_connections` | gauge | `user` | when per-user accounting is enabled (`cfg.Users` non-empty) |

### Histogram buckets

- Handshake: `[0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5]` seconds (typical proxy handshake is sub-second).
- DNS query: `[0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1]` seconds.

Both bucket sets are exposed as exported package vars so operators can override at compile-time if needed.

### Collector internals

The current `Collector` only handles atomic counters and one transport-keyed map. Two new generic helpers are added:

- `labeledCounter(name string, labelKeys []string)` — `map[labelTuple]*atomic.Int64` with `sync.RWMutex`, mirrors the `getOrCreateTransport` pattern (RLock fast path, double-check on write).
- `labeledHistogram(name string, buckets []float64, labelKeys []string)` — same locking pattern, each bucket is an `atomic.Int64`.

Lock contention is bounded: every metric label tuple takes the read lock for the fast path, write lock only on first occurrence.

### Hook injection (pattern established here, reused in Workstream 3)

To avoid sprinkling `metrics.Inc(...)` across the codebase, every new metric receives data through a single hook registered at startup:

- Transport `Accept` / failure paths: `transport.SetHandshakeMetrics(func(transport string, dur time.Duration, err error))` — set once when the server boots.
- DNS — server's destination resolver gains a single `metricHook` field, populated at construction time.
- Per-user accounting (when enabled): `users.SetActivityHook(func(user string, delta int))`.

This pattern is the contract that Workstream 3 (client metrics) reuses — the same `SetXHook` style is applied on the client side for routing decisions, CB transitions, and subscription refreshes.

### Tests

- One unit test per new metric: trigger the hook → scrape `/metrics` → assert the expected line is in the text body (matches existing `metrics_test.go` style).
- Concurrent metric updates with `-race`: 1000 goroutines hitting different label values.
- Bucket-boundary assertions for histograms (value at boundary lands in the right bucket).

## Workstream 3 — Client Metrics Expansion

### New `engine.Metrics()` accessor

To avoid bloating `engine.Status()` with metric-specific fields, a new method:

```go
type MetricsSnapshot struct {
    RoutingDecisions      map[string]int64  // key: "decision/rule"
    SubscriptionRefreshes map[string]SubscriptionStats
    HandshakeDurations    HistogramSnapshot // per-transport
    // ... extend without touching Status()
}

func (e *Engine) Metrics() MetricsSnapshot
```

`Status()` remains source of truth for connection counts, bytes, CB state, draining — `routes_prometheus.go` reads from both.

### `routes_prometheus.go` additions

Extends the existing 8 metrics with:

- `shuttle_routing_decisions_total{decision, rule}` — `decision` ∈ {proxy, direct, reject}, `rule` ∈ {geoip, domain, port, default}; hooked from `router/router.go`.
- `shuttle_circuit_breaker_state{outbound}` — extends the existing single-CB gauge to a per-outbound gauge; `engine/circuit.go` gains an `OnStateChange(func(outbound, state string))` callback. The legacy unlabelled `shuttle_circuit_breaker_state` is kept emitting the global state (max severity across outbounds) for one minor version, then removed.
- `shuttle_subscription_refresh_total{subscription, result}` — hooked via the existing subscription event bus.
- `shuttle_subscription_last_refresh_timestamp{subscription}` — same hook.
- `shuttle_handshake_duration_seconds{transport}` — client-side dial completion.
- `shuttle_handshake_failures_total{transport, reason}` — client-side dial failures.
- `shuttle_dns_query_duration_seconds{protocol, cached}` — `protocol` ∈ {doh, udp, system}; client-side router DNS.

The hook-injection pattern from Workstream 2 is reused: each subsystem exposes one `SetXHook` and the GUI API server wires them up at startup.

### Auth & exposure

`/api/prometheus` is mounted by default in the GUI API server but token-protected via the existing admin token. No config changes required. Documented in `examples/client.example.yaml` near the existing `ui:` block.

### Tests

- Same test style as server: trigger condition → scrape → assert text content.
- `Metrics()` method: assert zero-value snapshot when engine is freshly constructed.

## Workstream 4 — `SECURITY.md` Expansion

Target: ~150 lines, structured as below. Existing content is preserved verbatim except where marked.

```
1. Verifying Download Integrity        (preserved)
2. Reporting Security Issues           (rewritten — see below)
3. Supported Versions                  (preserved)
4. Threat Model                        (NEW)
5. Hardening Checklist                 (NEW)
6. Configuration Best Practices        (NEW)
7. Key & Token Rotation                (NEW)
```

A `> Last reviewed: 2026-04-28` line is added under the H1 title, with a note that this document is reviewed at every release.

### Section 2 rewrite

Switches the primary reporting channel to **GitHub Security Advisory** (private). Public issue link is retained as a fallback for non-sensitive concerns. PGP key field is included as a placeholder marked "currently no PGP — please use GitHub Security Advisory for confidential reports".

### Section 4 — Threat Model (~20 lines)

In scope: passive traffic analysis, active SNI probing, passive DPI, unauthorized management-plane access.

Out of scope: local-host compromise, upstream CDN/provider active collaboration, post-quantum break of Noise IK long-term confidentiality.

Trust boundary description: client ⟷ transport ⟷ server ⟷ destination, with the management plane (`/api/*`) treated as a separate trust domain requiring its own token.

### Section 5 — Hardening Checklist (~30 lines)

Operator-facing checklist (markdown checkboxes). Each item indicates whether it is a default (✓) or requires explicit configuration:

- Service runs as dedicated low-privilege user (✓ via `install.sh`).
- Admin port (`admin.listen`) bound to `127.0.0.1` or restricted by firewall.
- `auth.password` ≥ 16 chars random / prefer `auth.private_key` (Reality).
- `admin.token` ≥ 32 chars random.
- `router.allow_private_networks: false` in production (sandbox-only override).
- TLS private key file mode `0600`, directory `0700`.
- `cdn` outbound disabled or quota-limited (SSRF risk surface).
- `systemd` hardening flags as shipped in `install.sh` (✓).
- IP reputation rate-limiting enabled (✓ default).
- Logs do not echo `Authorization` header / passwords / private keys (✓; resampled at each release).

### Section 6 — Configuration Best Practices (~30 lines)

- Strong password generation: `openssl rand -base64 32`.
- TLS certs: prefer Let's Encrypt + cert-manager / acme.sh; avoid wildcard certs (revocation blast radius).
- Reality `target_sni`: choose a target that is **actually reachable** and behaviourally similar to the host; a dead domain is worse than no SNI camouflage.
- Subscription sources: HTTPS-only (already enforced); pass tokens via header, not query string (logs).
- Mesh: avoid CIDR overlap with corporate networks (default `10.7.0.0/24`).
- `metrics.listen` bound to `127.0.0.1` + token; never expose publicly.

### Section 7 — Key & Token Rotation (~25 lines)

| Credential | Rotation trigger | Procedure |
|---|---|---|
| Reality `auth.private_key` | Suspected leak | `shuttled keygen` → push new pubkey to clients → rolling restart → 24h grace period for old short-id |
| `auth.password` (H3) | Scheduled (90d) | Hot reload via `/api/reload`; multiple passwords coexist transiently |
| `admin.token` | Scheduled (30d) or leak | Edit config + `/api/reload`; old token invalid immediately |
| Subscription token | Per provider's scheme | Provider-driven |

### Reference from examples

`examples/server.example.yaml` gets one line at the top: `# Read SECURITY.md before deploying: https://github.com/ggshr9/shuttle/blob/main/SECURITY.md`.

## Workstream 5 — Cross-Platform CLI Installers

### 5a. macOS — Homebrew Tap

A separate public repo `shuttleX/homebrew-shuttle` (created during implementation, with explicit confirmation at `gh repo create` time).

Two formulae:

- `Formula/shuttle.rb` — installs `shuttle` CLI (client).
- `Formula/shuttled.rb` — installs `shuttled` (server) plus a `service` block declaring the `launchd` plist.

Each formula:

- Reads `url` from the matching release artifact `shuttle{,d}-darwin-{amd64,arm64}.tar.gz`.
- Pins `sha256` from `checksums.txt`.
- `def install` copies the binary to `bin/`, default config templates to `etc/`, shell completions to `bash_completion`/`zsh_completion`.
- `service` block (for `shuttled`): launchd label, `run` command, `keep_alive`, `log_path`.
- `test do` runs `shuttle --version` (and `shuttled --version`) as a smoke test.

User experience:
```bash
brew tap ggshr9/shuttle
brew install shuttled
shuttled init
brew services start shuttled
```

Automation: an extra step appended to `release.yml` uses `mislav/bump-homebrew-formula-action` to PR the tap repo with the new version + sha256 on each tag.

### 5b. Windows — `scripts/install-windows.ps1`

A single PowerShell script (~250 lines) mirroring the structure of `deploy/install.sh`:

- Architecture detection via `$env:PROCESSOR_ARCHITECTURE` → amd64/arm64.
- Download to `C:\Program Files\Shuttle\` (requires admin elevation).
- Three-step wizard matching install.sh: domain/IP → password → transport.
- Public IP autodetection (`Invoke-RestMethod https://api.ipify.org` with fallbacks).
- Calls `shuttled.exe init` to write config to `%ProgramData%\Shuttle\`.
- Registers Windows service via `shuttled service install` (uses existing `service_windows.go`).
- Subcommands: `install` / `uninstall` / `upgrade` / `status` (parity with install.sh).
- Firewall: `New-NetFirewallRule` for H3/Reality ports — **interactive confirmation before adding**, never silent.
- Detects `Set-ExecutionPolicy` restrictions and prints remediation guidance.

Entry point:
```powershell
iwr -useb https://raw.githubusercontent.com/ggshr9/shuttle/main/scripts/install-windows.ps1 | iex
```

### 5c. Shared changes

- Existing `deploy/install.sh` is **kept in place** (no break of existing one-line installer URLs in docs/community).
- A new `scripts/install-linux.sh` is created as a thin wrapper that `exec`s `deploy/install.sh` — provides URL parity (`scripts/install-{linux,macos,windows}.{sh,ps1}` pattern).
- `README.md` install section reorganized with three tabs: Linux / macOS (Homebrew) / Windows (PowerShell), each with a copy-paste command.
- `docs/site/` gets per-platform deployment pages.

### 5d. README install-decision navigator

To prevent "wrong installer" confusion (mentioned during design), the `README.md` install section starts with a 3-line decision table:

| If you want to... | Use |
|---|---|
| Run the server (`shuttled`) on a VPS | CLI installer (Linux/macOS/Windows) |
| Run a desktop client with a UI | GUI installer (`.dmg` / `.exe` / AppImage) |
| Automate / script / CI | CLI installer |

This decision navigator is also added to the top of each platform's deployment page in `docs/site/`.

### What is explicitly NOT done

- Chocolatey / Scoop / winget (community can submit later).
- macOS `.pkg` (Homebrew covers headless install; existing `.app` covers desktop).
- Snap / Flatpak (Linux already has deb/rpm + install.sh).
- Bundling GUI + CLI into a single Windows installer (orthogonal binaries; separation is intentional).

## Cross-cutting Test Strategy

- All host-safe tests run via `./scripts/test.sh` (per CLAUDE.md — never raw `go test`).
- Sandbox tests (`//go:build sandbox`) only for changes that touch real network state.
- Health/metrics/SECURITY changes are pure unit tests, no sandbox needed.
- Installer scripts: shell tests (`shellcheck` on `install-windows.ps1` via `Invoke-ScriptAnalyzer`; `shellcheck` on `install-linux.sh` wrapper), plus a documented manual smoke checklist in `docs/install-smoke.md`.
- CI: `.github/workflows/ci.yml` extended with a "metrics scrape" step that boots `shuttled` in a temp dir, hits `/metrics` and `/api/health/ready`, asserts presence of new metric names. No real network listeners outside of localhost.

## Risks and Mitigations

| Risk | Likelihood | Mitigation |
|---|---|---|
| New metrics increase scrape body size beyond Prometheus default | Low | Bucket counts kept tight; total new metric lines per scrape ~ +30 |
| `OnStateChange` hook on CB causes deadlock under high churn | Medium | Hook invocation runs in a goroutine; tests under `-race` with 100 concurrent transitions |
| Homebrew tap repo creation needs explicit `gh repo create` | Certain | Treated as a side-effecting action; explicit confirmation at execution time |
| Windows installer breaks on PowerShell Constrained Language Mode | Medium | Detect early, print remediation; fall back to manual download instructions |
| Readiness 503 confuses existing LB configs that scrape `/api/health` | Low | Old endpoint preserved with original behaviour; new endpoints opt-in |

## Build Sequence

Workstreams 1, 4, and 5 are independent of each other and of the metrics work. Workstream 2 establishes the hook-injection pattern that Workstream 3 reuses, so 3 follows 2.

```
Workstream 1 (deep health)        ─┐
Workstream 4 (SECURITY.md)        ─┤  (parallel, ~1-2 days each, no shared code)
Workstream 5 (installers)         ─┤
                                   │
Workstream 2 (server metrics)     ──┐  establishes hook-injection contract
                                    ↓
Workstream 3 (client metrics)     ──→  inherits hook style, adds router/CB/subscription
```

## Approval

This spec captures the agreed scope from the 2026-04-28 brainstorming. Any change to the scope (e.g., adding OpenTelemetry, splitting GUI installer) requires a new spec or amendment.
