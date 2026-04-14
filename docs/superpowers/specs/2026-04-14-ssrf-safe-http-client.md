# Spec: SSRF Safe HTTP Client

**Date:** 2026-04-14
**Status:** Draft
**Owner:** TBD
**Type:** Security hardening

## Problem

Shuttle fetches attacker-influenced URLs from three call sites:

1. **Subscription Manager** (`subscription/subscription.go:170`) — `fetch()` GETs user-supplied subscription URLs. The existing `Manager.Add()` blocks literal private IPs in the URL host but does **not** resolve hostnames, so `evil.com → 127.0.0.1` bypasses the check. The `http.Client` uses default `DialContext` which performs DNS resolution at connect time — after validation has already passed. Redirects only check count, not target.
2. **GUI Probe** (`gui/api/routes_misc.go:293`, `:390`) — `/api/test/probe` and `/api/test/probe/batch` handlers take user-supplied URLs and send HTTP requests. `validateProbeURL` (`gui/api/api.go:213`) blocks `localhost` string and literal-IP private ranges via `server.IsBlockedIP`, but does **not** resolve hostnames. DNS rebinding / indirect hostname → private IP bypasses the check.
3. **Config Validation** (`config/config_validate.go:329-337`) — `cover.reverse_url` check uses inline `ip.IsLoopback() || ip.IsPrivate() || ...` methods instead of the canonical `server.IsBlockedIP`. The inline check misses `0.0.0.0/8` unspecified and cloud-metadata ranges (shuttle's `IsBlockedIP` covers both). It also only checks literal IPs (correctly — config validation must not do network I/O), but is inconsistent with the rest of the codebase.

### Concrete Attack Scenarios

- **DNS bypass**: User adds subscription `http://subscribe.attacker.tld/cfg`. That hostname has an A record for `127.0.0.1`. `Manager.Add()` sees "subscribe.attacker.tld" → not a literal IP → passes. `fetch()` does `client.Do(req)` → Go's DNS resolver returns `127.0.0.1` → HTTP client connects to local services. Any local HTTP endpoint on the shuttle host is now reachable.
- **Redirect bypass**: User adds subscription at `https://real-provider.example/sub`. The provider responds `302 Location: http://169.254.169.254/latest/meta-data/iam/security-credentials/`. `CheckRedirect` only limits depth to 5, not target. Client follows → cloud metadata exfiltrated into subscription parsing error message or returned body.
- **GUI probe**: Same DNS / redirect bypasses apply to `/api/test/probe`. Even with `via=socks5` (tunneling through a user's outbound), the `via=direct` path is directly reachable. A compromised frontend or trusted local app could POST to the probe endpoint.

### What's Already In Place (Do Not Re-Implement)

- `server/ssrf.go:36` — `IsBlockedTarget(target string)` **already performs DNS resolution** via `net.LookupHost(host)` and checks every resolved IP against `blockedCIDRs`. Returns `true` if any resolution is blocked or resolution fails. This is the correct primitive; it's just not wired into the three call sites above.
- `server/ssrf.go:70` — `IsBlockedIP(ip net.IP)` literal-IP helper. Already used by `gui/api/api.go:233`, `subscription/subscription.go:72`.
- `Subscription.AllowPrivateNetworks` escape hatch (set from sandbox config) must be preserved.

## Goals

1. Every outbound HTTP call initiated on behalf of a user-supplied URL validates the **resolved destination IP** against `IsBlockedIP`, not just the literal hostname.
2. Redirects are subject to the same check — the `Location` target is re-validated.
3. Validation happens at **dial time**, not just at request-submission time — this closes the TOCTOU window where DNS answers change between validation and connect (DNS rebinding).
4. Call sites share a single helper so future HTTP fetch paths inherit the protection by default.
5. `AllowPrivateNetworks` bypass for sandbox environments continues to work identically.

## Non-Goals

- Proxied probes (`via=socks5`/`via=proxy` in GUI probe): SSRF semantics differ when the request tunnels through a user outbound — the proxy resolves the destination, not our local dialer. We perform a best-effort pre-flight DNS check (using `IsBlockedTarget`), but we do **not** attempt to intercept resolution inside the proxy transport. Out of scope.
- Full egress firewall for all outbound HTTP (metrics push, update check, etc.). Only user-attacker-influenced URLs.
- IPv6 literal brackets parsing fixes (already handled by `net.SplitHostPort`).
- Config validation doing DNS lookups. Config validation remains pure / offline.

## Design

### New Component: `server.SafeHTTPClient`

Add `server/httpsafe.go` with:

```go
// SafeHTTPClientOptions configures a SafeHTTPClient.
type SafeHTTPClientOptions struct {
    Timeout              time.Duration
    AllowPrivateNetworks bool // bypass SSRF checks (sandbox/testing only)
    MaxRedirects         int  // default 5
}

// NewSafeHTTPClient returns an *http.Client whose DialContext rejects
// connections to any IP in the SSRF blocked range, and whose CheckRedirect
// re-validates each redirect target hostname via IsBlockedTarget.
//
// When AllowPrivateNetworks is true, no SSRF checks are performed — this
// is intended only for sandbox/test environments.
func NewSafeHTTPClient(opts SafeHTTPClientOptions) *http.Client
```

Implementation:

- Custom `http.Transport` with `DialContext` that:
  1. Calls `net.DefaultResolver.LookupIPAddr(ctx, host)` to get all candidate IPs.
  2. For each IP, if `IsBlockedIP(ip)` and `!AllowPrivateNetworks`, **skip** it.
  3. If no allowed IPs remain, return `ErrBlockedTarget`.
  4. Otherwise, dial the first allowed IP via `net.Dialer{...}.DialContext(ctx, "tcp", net.JoinHostPort(allowed, port))`.
  5. If dial fails and other allowed IPs remain, try the next one (standard "Happy Eyeballs"-style fallback is out of scope; sequential try is sufficient).
- `CheckRedirect` func that:
  1. Limits redirect count to `MaxRedirects`.
  2. Extracts `req.URL.Hostname()`.
  3. If literal IP: calls `IsBlockedIP(ip)`; reject if blocked.
  4. If hostname: calls `IsBlockedTarget(host)`; reject if blocked.
- Export `ErrBlockedTarget = errors.New("safehttp: target address is blocked")` so callers can distinguish SSRF rejection from network errors.

### Why Dial-Time Validation

Pre-flight `IsBlockedTarget(host)` validation alone is insufficient because DNS answers can change between the pre-flight check and the actual `client.Do` resolution (TOCTOU / DNS rebinding). Dial-time validation in `DialContext` closes this window: the dial only uses IPs that the Go resolver returned **at dial time**, and we block before TCP connect.

Pre-flight checks are still valuable for fast rejection (before creating the request, we can return a clean 400 to the GUI), so both layers coexist.

### Wire-In Plan

| Call site | Change |
|---|---|
| `subscription/subscription.go:49` `NewManager` | Replace `&http.Client{Timeout: 30s, CheckRedirect: ...}` with `server.NewSafeHTTPClient(SafeHTTPClientOptions{Timeout: 30s, AllowPrivateNetworks: m.AllowPrivateNetworks})`. Note: `AllowPrivateNetworks` is mutated after construction (`m.AllowPrivateNetworks = true` in sandbox), so the constructor must be callable lazily. Change: make `client` a method `httpClient()` that returns a fresh `*http.Client` each Refresh, OR require callers to set `AllowPrivateNetworks` before `NewManager`. We'll add `SetAllowPrivateNetworks(bool)` that rebuilds the client. |
| `subscription/subscription.go:62` `Add` | Keep existing literal-IP check (fast fail for obvious cases, preserves error messages tests depend on). `fetch()` now gets defense-in-depth from dial-time validation. |
| `gui/api/api.go:213` `validateProbeURL` | Replace literal-IP-only check (line 232-235) with `server.IsBlockedTarget(host)` which does DNS resolution. Keep the `localhost` string short-circuit and scheme validation. |
| `gui/api/routes_misc.go:293`, `:390` | The probe HTTP client currently uses a custom `Transport` built by `buildProbeTransport(via, cfg)`. For `via=direct`, this transport is a stdlib default. Modify `buildProbeTransport` (or its caller) so that when `via=direct`, it returns a transport using `server.SafeDialContext`. For `via=socks5`/`via=proxy`, leave unchanged (dial goes via user outbound — not locally dialable). |
| `config/config_validate.go:329-337` | Replace inline `ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()` with `server.IsBlockedIP(ip)`. Still literal-IP only (no DNS). This is consistency + broader coverage (adds `0.0.0.0/8`, `fc00::/7`, etc.). |

### `SafeDialContext` Helper Export

Export a lower-level helper:

```go
// SafeDialContext returns a DialContext function suitable for http.Transport
// that rejects connections to IPs in the SSRF blocked range.
func SafeDialContext(allowPrivate bool) func(ctx context.Context, network, addr string) (net.Conn, error)
```

This lets `buildProbeTransport` in `gui/api/routes_misc.go` install the safe dialer into the existing `http.Transport` without adopting the full `SafeHTTPClient`.

### Error Semantics

All three call sites return HTTP 400 / error strings when SSRF rejection occurs, matching existing behavior for the literal-IP cases. `ErrBlockedTarget` is wrapped into messages like `"invalid subscription URL: target address is not allowed"`.

## Testing Strategy

### Unit Tests (host-safe, no real network)

- `server/httpsafe_test.go`
  - `TestSafeHTTPClient_BlocksLiteralPrivateIP` — dial `http://127.0.0.1:<port>/` → `ErrBlockedTarget` (no TCP connect occurs; we use an injected resolver or just rely on literal parsing bypassing `LookupIPAddr`).
  - `TestSafeHTTPClient_BlocksHostnameResolvingToPrivate` — uses a `Resolver` with custom `Dial` returning a stub that resolves `test.invalid` to `127.0.0.1`. Closes the TOCTOU gap.
  - `TestSafeHTTPClient_AllowsPublicIP` — using a `httptest.Server` bound on `127.0.0.1` is **blocked** (expected). To test the allow-path without real internet, inject a custom resolver that maps `safe.test` to `httptest.Server`'s literal IP **but without running SafeHTTPClient** — instead unit-test the dial helper directly using a public-range IP literal. (We don't have a "public IP" test fixture, so the allow-path test uses a mocked `IsBlockedIP` via a test seam — see below.)
  - `TestSafeHTTPClient_RedirectRejected` — `httptest.Server` returning `302 Location: http://127.0.0.1/`. Client follows once, `CheckRedirect` rejects. Note: this requires the initial request to reach the test server, so bind the test server on... hmm, we need a test server on a public IP but there isn't one. **Solution**: use `httptest.NewServer` + `http.Client` with `AllowPrivateNetworks: true` for the first hop, then `CheckRedirect` logic still runs and rejects the redirect target. The first hop is exempted but redirects must still validate against the **strict** policy. Change API: `AllowPrivateNetworks` allows dial but `CheckRedirect` uses a separate flag (or shares it). Simplest: tests can exercise `CheckRedirect` as a standalone function (`server.SafeCheckRedirect`) without needing a live client — direct unit test.
  - `TestSafeCheckRedirect_RejectsPrivateTarget` — direct call.
  - `TestSafeCheckRedirect_RejectsHostnameResolvingToPrivate` — direct call with injected resolver.
  - `TestSafeCheckRedirect_AllowsPublicTarget` — direct call with injected resolver returning a non-blocked IP.
  - `TestSafeCheckRedirect_MaxRedirects` — 6 `*http.Request` via array, expect error on the 6th.

- `gui/api/api_test.go` — existing `TestValidateProbeURL` cases for literal IPs continue to pass. Add:
  - `TestValidateProbeURL_BlocksHostnameResolvingToPrivate` — gated by `//go:build sandbox` (requires real DNS or injected resolver). In host tests, skip.
  - Or: refactor `validateProbeURL` to take an injectable resolver and test with a stub.

- `config/config_test.go` — add test that `cover.reverse_url: "http://0.0.0.0/"` is rejected (previously uncaught).

### Integration Tests (sandbox, `//go:build sandbox`)

- `subscription/sandbox_test.go`
  - Spin up an HTTP listener on `127.0.0.1:N`, attempt `Manager.Refresh` → expect `ErrBlockedTarget`.
  - Same with `AllowPrivateNetworks: true` → expect success.
  - Redirect test: listener returns `302 → 127.0.0.1/private` → expect rejection with `MaxRedirects` not yet reached.

### Resolver Injection Seam

To unit-test DNS-dependent behavior without live DNS, add an unexported seam:

```go
// server/httpsafe.go
var lookupIPAddr = net.DefaultResolver.LookupIPAddr // overridden in tests
```

Tests set `lookupIPAddr` via a helper in a `_test.go` file.

## Rollout

Single PR, no feature flag. No config changes. Behavior change is a strict security improvement; regressions would be "legitimate subscription URL was rejected" which surfaces as a clear error message with the blocked IP in it (operator can whitelist via `AllowPrivateNetworks` in sandbox).

## Risks

- **Breaking legitimate private-network subscriptions**: Users running shuttle inside a lab may host their subscription server on `10.x.x.x`. Mitigation: `AllowPrivateNetworks` config flag already exists on `subscription.Manager`, documented path.
- **Resolver injection test seam adds a package-level variable** — minor global state. Acceptable for a test-only escape.
- **Sequential IP try on dial failure** may differ from stdlib happy-eyeballs parallelism → slightly slower failover on multi-A-record hosts. Acceptable.

## Open Questions

1. Should `config_validate.go` eventually do DNS lookups for `cover.reverse_url`? — **No**, config validation must remain offline for CI determinism and fast-fail. Dial-time validation in the Cover transport already catches DNS bypasses at runtime. Out of scope.
2. Should we also wire `SafeHTTPClient` into `update/` (auto-update download)? — Update URLs are hardcoded to GitHub, not user-supplied. Out of scope for this spec.
3. Should `MaxRedirects` be per-call-site configurable? — Default 5 is enough. Out of scope.
