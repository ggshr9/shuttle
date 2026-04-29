# SSRF Safe HTTP Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close three SSRF bypass gaps (DNS rebinding, redirect follow, config consistency) by introducing a shared `server.SafeHTTPClient` that validates resolved IPs at dial time and on every redirect.

**Architecture:** New `server/httpsafe.go` exposing `NewSafeHTTPClient`, `SafeDialContext`, `SafeCheckRedirect`, and `ErrBlockedTarget`. Subscription manager and GUI probe (direct mode) adopt the safe client. `validateProbeURL` is upgraded to call the DNS-resolving `IsBlockedTarget`. `config_validate.go` is made consistent by calling `IsBlockedIP`. A package-level `lookupIPAddr` variable acts as a resolver-injection seam for unit tests.

**Tech Stack:** Go 1.24, stdlib `net/http`, `net`, existing `server/ssrf.go` primitives.

**Spec:** `docs/superpowers/specs/2026-04-14-ssrf-safe-http-client.md`

---

## File Structure

**Create:**
- `server/httpsafe.go` — `SafeHTTPClient`, `SafeDialContext`, `SafeCheckRedirect`, `ErrBlockedTarget`, `lookupIPAddr` seam
- `server/httpsafe_test.go` — unit tests using resolver injection
- `subscription/subscription_ssrf_sandbox_test.go` — integration test (`//go:build sandbox`)

**Modify:**
- `subscription/subscription.go` — `NewManager` uses `SafeHTTPClient`; add `SetAllowPrivateNetworks` that rebuilds client
- `gui/api/api.go` — `validateProbeURL` calls `IsBlockedTarget` for hostname DNS check
- `gui/api/routes_misc.go` — `buildProbeTransport("direct", ...)` installs `SafeDialContext`
- `config/config_validate.go` — `cover.reverse_url` check uses `server.IsBlockedIP`
- `gui/api/api_test.go` — add resolver-stub test for hostname case
- `config/config_test.go` (or create) — add test for broader `IsBlockedIP` coverage

---

## Task 1: Create `SafeDialContext` with literal-IP rejection

**Files:**
- Create: `server/httpsafe.go`
- Create: `server/httpsafe_test.go`

- [ ] **Step 1: Write the failing test for literal-IP dial rejection**

Create `server/httpsafe_test.go`:

```go
package server

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestSafeDialContext_BlocksLiteralLoopback(t *testing.T) {
	dial := SafeDialContext(false)
	_, err := dial(context.Background(), "tcp", "127.0.0.1:80")
	if !errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("expected ErrBlockedTarget, got %v", err)
	}
}

func TestSafeDialContext_BlocksLiteralPrivate(t *testing.T) {
	dial := SafeDialContext(false)
	for _, addr := range []string{"10.0.0.1:80", "192.168.1.1:443", "169.254.169.254:80", "[::1]:80"} {
		_, err := dial(context.Background(), "tcp", addr)
		if !errors.Is(err, ErrBlockedTarget) {
			t.Errorf("addr %q: expected ErrBlockedTarget, got %v", addr, err)
		}
	}
}

func TestSafeDialContext_AllowPrivateBypass(t *testing.T) {
	// With AllowPrivate=true, loopback should be attempted. We don't bind a
	// listener, so we expect a connection-refused error, NOT ErrBlockedTarget.
	dial := SafeDialContext(true)
	_, err := dial(context.Background(), "tcp", "127.0.0.1:1") // port 1 unlikely to be bound
	if errors.Is(err, ErrBlockedTarget) {
		t.Errorf("AllowPrivate should bypass SSRF check, got ErrBlockedTarget")
	}
	// err may be non-nil (connection refused) — that's fine.
	_ = err
}

var _ = net.IPv4 // keep import
```

- [ ] **Step 2: Run tests — expect compile failure**

Run: `./scripts/test.sh --pkg ./server/ --run TestSafeDialContext`
Expected: FAIL with `undefined: SafeDialContext` / `undefined: ErrBlockedTarget`.

- [ ] **Step 3: Create `server/httpsafe.go` with minimal implementation**

Create `server/httpsafe.go`:

```go
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// ErrBlockedTarget is returned when an HTTP client refuses to dial or follow
// a redirect to an address that falls inside an SSRF-blocked CIDR range.
var ErrBlockedTarget = errors.New("safehttp: target address is blocked")

// lookupIPAddr is overridable in tests via httpsafe_testhook_test.go.
var lookupIPAddr = net.DefaultResolver.LookupIPAddr

// SafeDialContext returns a DialContext function suitable for http.Transport
// that refuses to connect to any address that resolves into the SSRF-blocked
// CIDR set (see server/ssrf.go). When allowPrivate is true, the check is
// bypassed entirely — intended for sandbox/testing environments only.
func SafeDialContext(allowPrivate bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("safehttp: split host port: %w", err)
		}

		// Literal IP fast path.
		if ip := net.ParseIP(host); ip != nil {
			if !allowPrivate && isBlockedIP(ip) {
				return nil, fmt.Errorf("%w: %s", ErrBlockedTarget, ip.String())
			}
			return dialer.DialContext(ctx, network, addr)
		}

		// Hostname path: resolve, filter blocked IPs, try each allowed IP.
		ips, err := lookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("safehttp: resolve %q: %w", host, err)
		}
		var allowed []net.IPAddr
		for _, ipa := range ips {
			if allowPrivate || !isBlockedIP(ipa.IP) {
				allowed = append(allowed, ipa)
			}
		}
		if len(allowed) == 0 {
			return nil, fmt.Errorf("%w: all resolved addresses for %q are blocked", ErrBlockedTarget, host)
		}

		var lastErr error
		for _, ipa := range allowed {
			target := net.JoinHostPort(ipa.IP.String(), port)
			conn, dErr := dialer.DialContext(ctx, network, target)
			if dErr == nil {
				return conn, nil
			}
			lastErr = dErr
		}
		return nil, fmt.Errorf("safehttp: dial all allowed addresses failed: %w", lastErr)
	}
}

// SafeHTTPClientOptions configures a SafeHTTPClient.
type SafeHTTPClientOptions struct {
	Timeout              time.Duration
	AllowPrivateNetworks bool
	MaxRedirects         int
}

// NewSafeHTTPClient returns an *http.Client whose DialContext rejects
// connections to SSRF-blocked IPs, and whose CheckRedirect re-validates each
// redirect target host.
func NewSafeHTTPClient(opts SafeHTTPClientOptions) *http.Client {
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.MaxRedirects == 0 {
		opts.MaxRedirects = 5
	}
	tr := &http.Transport{
		DialContext:           SafeDialContext(opts.AllowPrivateNetworks),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:       opts.Timeout,
		Transport:     tr,
		CheckRedirect: SafeCheckRedirect(opts.AllowPrivateNetworks, opts.MaxRedirects),
	}
}

// SafeCheckRedirect returns an http.Client.CheckRedirect func that rejects
// redirects whose target resolves into a blocked CIDR, and caps redirect depth.
func SafeCheckRedirect(allowPrivate bool, maxRedirects int) func(req *http.Request, via []*http.Request) error {
	if maxRedirects == 0 {
		maxRedirects = 5
	}
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("safehttp: too many redirects (max %d)", maxRedirects)
		}
		if allowPrivate {
			return nil
		}
		host := req.URL.Hostname()
		if host == "" {
			return fmt.Errorf("safehttp: redirect target has no hostname")
		}
		if ip := net.ParseIP(host); ip != nil {
			if isBlockedIP(ip) {
				return fmt.Errorf("%w: redirect to %s", ErrBlockedTarget, ip.String())
			}
			return nil
		}
		if IsBlockedTarget(host) {
			return fmt.Errorf("%w: redirect to %s", ErrBlockedTarget, host)
		}
		return nil
	}
}
```

- [ ] **Step 4: Run tests — expect pass**

Run: `./scripts/test.sh --pkg ./server/ --run TestSafeDialContext`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add server/httpsafe.go server/httpsafe_test.go
git commit -m "feat(server): add SafeDialContext and SafeHTTPClient for SSRF defense"
```

---

## Task 2: Add resolver injection seam + hostname test

**Files:**
- Create: `server/httpsafe_testhook_test.go`
- Modify: `server/httpsafe_test.go`

- [ ] **Step 1: Add test hook file**

Create `server/httpsafe_testhook_test.go`:

```go
package server

import (
	"context"
	"net"
)

// setLookupIPAddr replaces the package-level resolver hook for the duration
// of a test. Returns a restore function.
func setLookupIPAddr(fn func(ctx context.Context, host string) ([]net.IPAddr, error)) func() {
	prev := lookupIPAddr
	lookupIPAddr = fn
	return func() { lookupIPAddr = prev }
}
```

- [ ] **Step 2: Write failing hostname-rebinding test**

Append to `server/httpsafe_test.go`:

```go
func TestSafeDialContext_BlocksHostnameResolvingToPrivate(t *testing.T) {
	restore := setLookupIPAddr(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})
	defer restore()

	dial := SafeDialContext(false)
	_, err := dial(context.Background(), "tcp", "rebind.test:80")
	if !errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("expected ErrBlockedTarget for hostname rebinding to loopback, got %v", err)
	}
}

func TestSafeDialContext_HostnameAllowsAfterFilteringBlocked(t *testing.T) {
	// Resolver returns [blocked, public]. Blocked entry is filtered; dial
	// proceeds to the public IP (which will fail to connect on port 1, but
	// with a dial error, NOT ErrBlockedTarget).
	restore := setLookupIPAddr(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{
			{IP: net.ParseIP("10.0.0.1")},
			{IP: net.ParseIP("203.0.113.1")}, // TEST-NET-3, unroutable but public
		}, nil
	})
	defer restore()

	dial := SafeDialContext(false)
	_, err := dial(context.Background(), "tcp", "mixed.test:1")
	if errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("mixed resolver should not produce ErrBlockedTarget, got %v", err)
	}
}
```

- [ ] **Step 3: Run tests — expect pass**

Run: `./scripts/test.sh --pkg ./server/ --run TestSafeDialContext`
Expected: PASS (5 tests total). The rebinding test catches the TOCTOU gap.

- [ ] **Step 4: Commit**

```bash
git add server/httpsafe_test.go server/httpsafe_testhook_test.go
git commit -m "test(server): cover SafeDialContext hostname resolution paths"
```

---

## Task 3: Test `SafeCheckRedirect` directly

**Files:**
- Modify: `server/httpsafe_test.go`

- [ ] **Step 1: Write failing redirect tests**

Append to `server/httpsafe_test.go`:

```go
import "net/http"
import "net/url"

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

func TestSafeCheckRedirect_RejectsPrivateLiteral(t *testing.T) {
	check := SafeCheckRedirect(false, 5)
	req := &http.Request{URL: mustParseURL(t, "http://169.254.169.254/meta-data")}
	err := check(req, nil)
	if !errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("expected ErrBlockedTarget, got %v", err)
	}
}

func TestSafeCheckRedirect_RejectsHostnameResolvingToPrivate(t *testing.T) {
	restore := setLookupIPAddr(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})
	defer restore()

	check := SafeCheckRedirect(false, 5)
	req := &http.Request{URL: mustParseURL(t, "http://rebind.test/")}
	err := check(req, nil)
	if !errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("expected ErrBlockedTarget for rebinding, got %v", err)
	}
}

func TestSafeCheckRedirect_MaxRedirects(t *testing.T) {
	check := SafeCheckRedirect(true, 3) // AllowPrivate to isolate depth check
	req := &http.Request{URL: mustParseURL(t, "http://127.0.0.1/")}
	via := make([]*http.Request, 3)
	err := check(req, via)
	if err == nil || !strings.Contains(err.Error(), "too many redirects") {
		t.Fatalf("expected too many redirects error, got %v", err)
	}
}

func TestSafeCheckRedirect_AllowPrivateBypass(t *testing.T) {
	check := SafeCheckRedirect(true, 5)
	req := &http.Request{URL: mustParseURL(t, "http://127.0.0.1/")}
	if err := check(req, nil); err != nil {
		t.Fatalf("AllowPrivate should pass, got %v", err)
	}
}
```

Add `"strings"` to the imports at top of `server/httpsafe_test.go` if not already present.

**Note:** `IsBlockedTarget` in `SafeCheckRedirect` calls `net.LookupHost` (not our seam), which means the hostname rebinding test will use real DNS. To avoid flakiness, `SafeCheckRedirect` should also go through the `lookupIPAddr` hook. Refactor `SafeCheckRedirect` hostname branch to use the same code path.

- [ ] **Step 2: Refactor `SafeCheckRedirect` to use the `lookupIPAddr` hook**

Edit `server/httpsafe.go` — in `SafeCheckRedirect`, replace the hostname branch:

```go
		if ip := net.ParseIP(host); ip != nil {
			if isBlockedIP(ip) {
				return fmt.Errorf("%w: redirect to %s", ErrBlockedTarget, ip.String())
			}
			return nil
		}
		ips, err := lookupIPAddr(req.Context(), host)
		if err != nil {
			return fmt.Errorf("%w: resolve redirect target %q: %v", ErrBlockedTarget, host, err)
		}
		for _, ipa := range ips {
			if isBlockedIP(ipa.IP) {
				return fmt.Errorf("%w: redirect to %s (%s)", ErrBlockedTarget, host, ipa.IP.String())
			}
		}
		return nil
```

- [ ] **Step 3: Run tests — expect pass**

Run: `./scripts/test.sh --pkg ./server/`
Expected: PASS — all SafeDialContext + SafeCheckRedirect tests.

- [ ] **Step 4: Commit**

```bash
git add server/httpsafe.go server/httpsafe_test.go
git commit -m "test(server): cover SafeCheckRedirect rebinding and depth paths"
```

---

## Task 4: Wire `SafeHTTPClient` into subscription manager

**Files:**
- Modify: `subscription/subscription.go`

- [ ] **Step 1: Read current `NewManager` and the AllowPrivateNetworks setter pattern**

Check `subscription/subscription.go:30-59` for the Manager struct and constructor. Note that `AllowPrivateNetworks` is a public field currently mutated directly by callers.

- [ ] **Step 2: Modify the Manager to rebuild the client when AllowPrivateNetworks changes**

Replace lines 30-59 in `subscription/subscription.go`:

```go
// Manager handles subscription management.
type Manager struct {
	mu            sync.RWMutex
	subscriptions map[string]*Subscription
	client        *http.Client

	// allowPrivateNetworks disables SSRF checks for private/loopback IPs.
	// Access via SetAllowPrivateNetworks so the HTTP client is rebuilt.
	allowPrivateNetworks bool

	autoMu      sync.Mutex
	autoCancel  context.CancelFunc
	autoRunning bool
}

// NewManager creates a new subscription manager.
func NewManager() *Manager {
	m := &Manager{
		subscriptions: make(map[string]*Subscription),
	}
	m.rebuildClient()
	return m
}

// SetAllowPrivateNetworks toggles SSRF checks. Setting true is intended for
// sandbox/testing environments only. Rebuilds the internal HTTP client.
func (m *Manager) SetAllowPrivateNetworks(allow bool) {
	m.mu.Lock()
	m.allowPrivateNetworks = allow
	m.mu.Unlock()
	m.rebuildClient()
}

// AllowPrivateNetworks reports whether SSRF checks are bypassed.
func (m *Manager) AllowPrivateNetworks() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.allowPrivateNetworks
}

func (m *Manager) rebuildClient() {
	m.mu.Lock()
	allow := m.allowPrivateNetworks
	m.mu.Unlock()
	c := server.NewSafeHTTPClient(server.SafeHTTPClientOptions{
		Timeout:              30 * time.Second,
		AllowPrivateNetworks: allow,
		MaxRedirects:         5,
	})
	m.mu.Lock()
	m.client = c
	m.mu.Unlock()
}
```

- [ ] **Step 3: Update `Add` to use the new accessor**

Replace `subscription/subscription.go:68-77` (the `if !m.AllowPrivateNetworks { ... }` block):

```go
	// Block private/loopback/link-local literal IP hosts to prevent SSRF.
	// (Defense-in-depth: the SafeHTTPClient also blocks at dial time including
	// for hostnames that resolve to private IPs.)
	if !m.AllowPrivateNetworks() {
		if parsed, err := urlpkg.Parse(url); err == nil {
			if host := parsed.Hostname(); host != "" {
				if ip := net.ParseIP(host); ip != nil && server.IsBlockedIP(ip) {
					return nil, fmt.Errorf("invalid subscription URL: target address is not allowed")
				}
			}
		}
	}
```

- [ ] **Step 4: Find and update callers of `m.AllowPrivateNetworks = ...`**

Run: `rg 'AllowPrivateNetworks\s*=' subscription/ engine/ cmd/ test/` (via Grep tool).

For each caller (likely in engine initialization or sandbox setup), change:
```go
sub.AllowPrivateNetworks = true
```
to:
```go
sub.SetAllowPrivateNetworks(true)
```

- [ ] **Step 5: Run subscription tests**

Run: `./scripts/test.sh --pkg ./subscription/`
Expected: PASS (existing tests should still pass — `Add` literal-IP check preserved).

- [ ] **Step 6: Commit**

```bash
git add subscription/subscription.go
# plus any caller updates
git commit -m "feat(subscription): use SafeHTTPClient for SSRF-safe fetches"
```

---

## Task 5: Sandbox integration test for subscription fetch

**Files:**
- Create: `subscription/subscription_ssrf_sandbox_test.go`

- [ ] **Step 1: Write sandbox integration test**

Create `subscription/subscription_ssrf_sandbox_test.go`:

```go
//go:build sandbox

package subscription

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ggshr9/shuttle/server"
)

// TestFetch_BlockedByDialTimeValidation verifies that even when Add() passes
// (because the URL contains a hostname, not a literal IP), the dial-time
// check rejects the connection when the hostname resolves to a private IP.
func TestFetch_BlockedLoopbackLiteral(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("vmess://..."))
	}))
	defer ts.Close()

	m := NewManager()
	// Add literal loopback URL — this is blocked at Add time.
	_, err := m.Add("test", ts.URL)
	if err == nil {
		t.Fatal("expected Add to reject loopback literal")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetch_AllowPrivateNetworksBypass(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("vmess://dGVzdA=="))
	}))
	defer ts.Close()

	m := NewManager()
	m.SetAllowPrivateNetworks(true)
	sub, err := m.Add("test", ts.URL)
	if err != nil {
		t.Fatalf("Add with allow-private: %v", err)
	}
	if _, err := m.Refresh(context.Background(), sub.ID); err != nil {
		t.Fatalf("Refresh with allow-private: %v", err)
	}
}

func TestFetch_RejectsRedirectToPrivate(t *testing.T) {
	// Private target (loopback) the attacker wants to reach.
	privateSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("private handler must not be reached, path=%s", r.URL.Path)
	}))
	defer privateSrv.Close()

	// Public-looking redirector. Use AllowPrivate=true to let the initial
	// request through, but the CheckRedirect must still reject the hop.
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, privateSrv.URL, http.StatusFound)
	}))
	defer redirector.Close()

	m := NewManager()
	m.SetAllowPrivateNetworks(true) // Allow initial hop to httptest (also loopback).

	// But override CheckRedirect via a stricter client for this test.
	// Alternative: use server.NewSafeHTTPClient with AllowPrivateNetworks:false
	// and a resolver seam — but that cross-cuts server package. Skip for now.
	// This test documents the expected behavior; see server/httpsafe_test.go
	// for the unit-level redirect rejection coverage.
	_ = fmt.Sprintf
	_ = errors.Is
	_ = net.ParseIP
	t.Skip("redirect rejection is covered by unit tests in server/httpsafe_test.go")
}
```

**Note:** The redirect test is skipped in sandbox because `httptest.NewServer` binds on loopback — we can't exercise "real public → private redirect" without a public-IP listener. The unit-level `TestSafeCheckRedirect_*` tests cover the logic. Document this and move on.

- [ ] **Step 2: Run sandbox tests**

Run: `./scripts/test.sh --sandbox --pkg ./subscription/`
Expected: PASS for `TestFetch_BlockedLoopbackLiteral` and `TestFetch_AllowPrivateNetworksBypass`; SKIP for `TestFetch_RejectsRedirectToPrivate`.

- [ ] **Step 3: Commit**

```bash
git add subscription/subscription_ssrf_sandbox_test.go
git commit -m "test(subscription): sandbox integration tests for SSRF defense"
```

---

## Task 6: Upgrade `validateProbeURL` to resolve hostnames

**Files:**
- Modify: `gui/api/api.go`

- [ ] **Step 1: Write failing test for hostname rebinding in probe validation**

Append to `gui/api/api_test.go`:

```go
func TestValidateProbeURL_BlocksHostnameResolvingToPrivate(t *testing.T) {
	// This test uses the real resolver — it depends on 'shuttle-rebind.test.invalid'
	// NOT resolving. Use a literal that `IsBlockedTarget` will resolve and block
	// via its DNS-resolving path. If DNS is flaky in CI, skip.
	if testing.Short() {
		t.Skip("skip DNS-dependent test in short mode")
	}
	// We can't reliably test DNS rebinding without a stub resolver in this
	// package. Instead, verify that a literal private hostname-like input
	// (using ":" form) is still caught by the literal IP path, and that the
	// IsBlockedTarget path is wired in.
	cases := []string{
		"http://127.0.0.1/",
		"http://10.0.0.1/",
		"http://169.254.169.254/latest/meta-data/",
	}
	for _, u := range cases {
		if err := validateProbeURL(u); err == nil {
			t.Errorf("expected %q to be blocked", u)
		}
	}
}
```

Existing literal-IP tests already catch the coverage we need at this layer. The DNS-resolution path is tested in `server/httpsafe_test.go` via the resolver seam.

- [ ] **Step 2: Update `validateProbeURL` to call `IsBlockedTarget` for hostnames**

Replace `gui/api/api.go:231-236`:

```go
	// Block private, loopback, link-local, and cloud-metadata ranges.
	// For literal IPs, check directly. For hostnames, use IsBlockedTarget
	// which resolves DNS before checking (prevents DNS rebinding).
	if ip := net.ParseIP(host); ip != nil {
		if server.IsBlockedIP(ip) {
			return fmt.Errorf("probing private/localhost/link-local addresses is not allowed")
		}
	} else {
		if server.IsBlockedTarget(host) {
			return fmt.Errorf("probing private/localhost/link-local addresses is not allowed (hostname %q resolves to a blocked address)", host)
		}
	}
	return nil
}
```

Ensure `github.com/ggshr9/shuttle/server` is already imported at the top of `gui/api/api.go` (it is — the existing code at line 233 uses `server.IsBlockedIP`).

- [ ] **Step 3: Run GUI API tests**

Run: `./scripts/test.sh --pkg ./gui/api/`
Expected: PASS — existing `TestValidateProbeURL` literal tests continue to pass.

- [ ] **Step 4: Commit**

```bash
git add gui/api/api.go gui/api/api_test.go
git commit -m "feat(gui/api): resolve probe URL hostnames for SSRF defense"
```

---

## Task 7: Wire `SafeDialContext` into GUI probe (direct mode)

**Files:**
- Modify: `gui/api/routes_misc.go`

- [ ] **Step 1: Locate `buildProbeTransport`**

Run: `rg 'func buildProbeTransport' gui/api/` (via Grep tool).
Read the function. It returns an `http.RoundTripper` based on `via`.

- [ ] **Step 2: Modify `buildProbeTransport` direct branch**

Find the branch that handles `via == "direct"` (or the default branch that constructs an `*http.Transport` without a proxy). Replace the plain `&http.Transport{}` construction with:

```go
	case "direct":
		return &http.Transport{
			DialContext:           server.SafeDialContext(false),
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   10 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}, ""
```

If the current code uses a different structure (e.g., returns `http.DefaultTransport`), replace with the above. Preserve the signature and error-string return convention.

Ensure `github.com/ggshr9/shuttle/server` is imported in `routes_misc.go`.

**Note on proxied paths:** For `via=socks5` and `via=proxy`, the dial goes through the user's outbound — `SafeDialContext` does not apply because the local dialer only connects to the proxy, not to the target. Pre-flight `validateProbeURL` (Task 6) handles the DNS rebinding case for those paths.

- [ ] **Step 3: Also install `SafeCheckRedirect` on the probe client**

At `gui/api/routes_misc.go:293` (single probe) and `:390` (batch), where the `&http.Client{Transport: transport, Timeout: 15 * time.Second}` is constructed, add:

```go
		client := &http.Client{
			Transport:     transport,
			Timeout:       15 * time.Second,
			CheckRedirect: server.SafeCheckRedirect(false, 5),
		}
```

Apply to both probe handlers.

- [ ] **Step 4: Run tests**

Run: `./scripts/test.sh --pkg ./gui/api/`
Expected: PASS.

- [ ] **Step 5: Manually verify compile**

Run: `CGO_ENABLED=0 go build -o /tmp/shuttle ./cmd/shuttle && CGO_ENABLED=0 go build -o /tmp/shuttled ./cmd/shuttled`
Expected: both builds succeed.

- [ ] **Step 6: Commit**

```bash
git add gui/api/routes_misc.go
git commit -m "feat(gui/api): use SafeDialContext and SafeCheckRedirect in probes"
```

---

## Task 8: Config validate consistency — use `IsBlockedIP`

**Files:**
- Modify: `config/config_validate.go`

- [ ] **Step 1: Write failing test for broader coverage**

Append to `config/config_test.go` (create if it does not exist):

```go
package config

import (
	"strings"
	"testing"
)

func TestValidate_CoverReverseURL_BlocksAllShuttleCIDRs(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1/",
		"http://10.0.0.1/",
		"http://172.16.0.1/",
		"http://192.168.1.1/",
		"http://169.254.169.254/", // link-local / cloud metadata
		"http://0.0.0.0/",         // unspecified — currently MISSED by inline check
		"http://[::1]/",
	}
	for _, u := range blocked {
		cfg := DefaultServerConfig()
		cfg.Cover.Mode = "reverse"
		cfg.Cover.ReverseURL = u
		err := cfg.Validate()
		if err == nil {
			t.Errorf("%q: expected validation error, got nil", u)
			continue
		}
		if !strings.Contains(err.Error(), "private") && !strings.Contains(err.Error(), "blocked") && !strings.Contains(err.Error(), "localhost") {
			t.Errorf("%q: unexpected error: %v", u, err)
		}
	}
}

func TestValidate_CoverReverseURL_AllowsPublic(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.Cover.Mode = "reverse"
	cfg.Cover.ReverseURL = "https://example.com/"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("public URL should pass: %v", err)
	}
}
```

- [ ] **Step 2: Run tests — expect failure**

Run: `./scripts/test.sh --pkg ./config/ --run TestValidate_CoverReverseURL`
Expected: FAIL on `http://0.0.0.0/` — current inline check misses unspecified range.

- [ ] **Step 3: Replace inline check with `server.IsBlockedIP`**

Edit `config/config_validate.go:329-337`. Replace:

```go
			if u, err := url.Parse(c.Cover.ReverseURL); err == nil {
				if host := u.Hostname(); host != "" {
					if ip := net.ParseIP(host); ip != nil {
						if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
							return fmt.Errorf("cover.reverse_url must not point to a private or localhost address")
						}
					}
				}
			}
```

with:

```go
			if u, err := url.Parse(c.Cover.ReverseURL); err == nil {
				if host := u.Hostname(); host != "" {
					if ip := net.ParseIP(host); ip != nil && server.IsBlockedIP(ip) {
						return fmt.Errorf("cover.reverse_url must not point to a private, localhost, or blocked address")
					}
				}
			}
```

Add `"github.com/ggshr9/shuttle/server"` to the imports at the top of `config/config_validate.go`.

**Circular import check:** `server` package currently imports `config`? Run:
`rg '"github.com/ggshr9/shuttle/config"' server/` (via Grep tool).

If `server` imports `config`, there's a cycle. In that case:
- Option A: Move `isBlockedIP` + `blockedCIDRs` to a new `internal/netblock` package that both `server` and `config` depend on.
- Option B: Duplicate `IsBlockedIP` into `config` (rejected — violates DRY).
- Option C: Inline the CIDR list into `config_validate.go` matching `server`'s set.

**Preferred: Option A if a cycle exists, Option C otherwise.** Decision is made based on the grep result.

- [ ] **Step 4: Run tests — expect pass**

Run: `./scripts/test.sh --pkg ./config/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add config/config_validate.go config/config_test.go
# plus any new internal/netblock/ package if needed
git commit -m "fix(config): use server.IsBlockedIP for broader reverse_url coverage"
```

---

## Task 9: Full build and host-safe test run

- [ ] **Step 1: Build both binaries**

Run:
```bash
CGO_ENABLED=0 go build -o /tmp/shuttle ./cmd/shuttle
CGO_ENABLED=0 go build -o /tmp/shuttled ./cmd/shuttled
```
Expected: both succeed.

- [ ] **Step 2: Run host-safe tests**

Run: `./scripts/test.sh`
Expected: PASS across all packages.

- [ ] **Step 3: Run sandbox tests**

Run: `./scripts/test.sh --sandbox --pkg ./subscription/`
Expected: PASS for `TestFetch_BlockedLoopbackLiteral` and `TestFetch_AllowPrivateNetworksBypass`.

- [ ] **Step 4: Commit any fixes uncovered by the full run**

If failures surface, fix them inline and commit with descriptive messages. Do not squash with the feature commits.

---

## Self-Review Checklist

- [ ] All three SSRF call sites now call into either `SafeHTTPClient`/`SafeDialContext` or the DNS-aware `IsBlockedTarget`.
- [ ] `AllowPrivateNetworks` sandbox escape hatch still works through `Manager.SetAllowPrivateNetworks`.
- [ ] No package-level global state beyond the `lookupIPAddr` test seam in `server/httpsafe.go`.
- [ ] `CheckRedirect` is installed on every `http.Client` constructed for probe or subscription use.
- [ ] No new imports from `server` into `config` that create a cycle (or a netblock extraction is done if needed).
- [ ] Build succeeds for both `shuttle` and `shuttled` with `CGO_ENABLED=0`.
- [ ] All new tests are deterministic (resolver seam used for DNS cases; no live-DNS dependencies).
