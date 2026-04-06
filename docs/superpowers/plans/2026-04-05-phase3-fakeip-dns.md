# Phase 3: fake-ip DNS Mode

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add fake-ip DNS mode so TUN can route by domain name (not just IP), reducing latency and preventing DNS leaks.

**Architecture:** A `fakeip.Pool` manages a virtual IP range (198.18.0.0/15), mapping domains to fake IPs. The DNS resolver returns fake IPs in fake-ip mode. The router and outbound layer reverse-map fake IPs back to domains. A domain filter exempts certain domains (STUN, LAN, NTP).

**Tech Stack:** Go 1.24+, `net/netip` for IP pool management

**Spec:** `docs/superpowers/specs/2026-04-05-ecosystem-compatibility-design.md` — Section 6

**Depends on:** None (independent of Phase 1/2, only needs existing DNS resolver)

---

## File Structure

### New Files
| File | Responsibility |
|------|---------------|
| `router/dns/fakeip/pool.go` | fake-ip pool: allocate, lookup, reverse |
| `router/dns/fakeip/pool_test.go` | Pool tests |
| `router/dns/fakeip/filter.go` | Domain filter (whitelist for real DNS) |
| `router/dns/fakeip/filter_test.go` | Filter tests |
| `router/dns/fakeip/store.go` | Optional persistent store (BoltDB or file) |

### Modified Files
| File | Change |
|------|--------|
| `router/dns.go` | Add fake-ip mode: return fake IP from pool instead of real resolution |
| `config/config_routing.go` | Add `Mode`, `FakeIPRange`, `FakeIPFilter`, `Persist` to `DNSConfig` |
| `engine/engine_inbound.go` | In routing path, reverse fake IP → domain before matching rules |

---

### Task 1: fake-ip Domain Filter

**Files:**
- Create: `router/dns/fakeip/filter.go`
- Create: `router/dns/fakeip/filter_test.go`

- [ ] **Step 1: Write filter tests**

```go
// router/dns/fakeip/filter_test.go
package fakeip_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"shuttle/router/dns/fakeip"
)

func TestFilter_ExactMatch(t *testing.T) {
	f := fakeip.NewFilter([]string{"stun.l.google.com", "ntp.ubuntu.com"})
	assert.True(t, f.ShouldSkip("stun.l.google.com"))
	assert.True(t, f.ShouldSkip("ntp.ubuntu.com"))
	assert.False(t, f.ShouldSkip("www.google.com"))
}

func TestFilter_WildcardSuffix(t *testing.T) {
	f := fakeip.NewFilter([]string{"+.lan", "+.local", "stun.*"})
	assert.True(t, f.ShouldSkip("router.lan"))
	assert.True(t, f.ShouldSkip("printer.local"))
	assert.True(t, f.ShouldSkip("stun.l.google.com"))
	assert.False(t, f.ShouldSkip("www.example.com"))
}

func TestFilter_GlobPattern(t *testing.T) {
	f := fakeip.NewFilter([]string{"time.*.com", "*.ntp.org"})
	assert.True(t, f.ShouldSkip("time.google.com"))
	assert.True(t, f.ShouldSkip("time.apple.com"))
	assert.True(t, f.ShouldSkip("0.pool.ntp.org"))
	assert.False(t, f.ShouldSkip("www.example.com"))
}

func TestFilter_Empty(t *testing.T) {
	f := fakeip.NewFilter(nil)
	assert.False(t, f.ShouldSkip("anything.com"))
}
```

- [ ] **Step 2: Implement filter**

```go
// router/dns/fakeip/filter.go
package fakeip

import (
	"path/filepath"
	"strings"
)

// Filter determines which domains should bypass fake-ip (get real DNS).
type Filter struct {
	exact    map[string]bool
	suffixes []string // "+.lan" → ".lan"
	patterns []string // "stun.*", "time.*.com"
}

// NewFilter creates a filter from a list of patterns.
// Supported formats:
//   - "example.com" — exact match
//   - "+.lan" — suffix match (anything ending in .lan)
//   - "stun.*" — glob pattern using filepath.Match
func NewFilter(patterns []string) *Filter {
	f := &Filter{
		exact: make(map[string]bool),
	}
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "+.") {
			f.suffixes = append(f.suffixes, p[1:]) // "+.lan" → ".lan"
		} else if strings.ContainsAny(p, "*?") {
			f.patterns = append(f.patterns, p)
		} else {
			f.exact[p] = true
		}
	}
	return f
}

// ShouldSkip returns true if the domain should bypass fake-ip.
func (f *Filter) ShouldSkip(domain string) bool {
	domain = strings.ToLower(domain)

	if f.exact[domain] {
		return true
	}

	for _, suffix := range f.suffixes {
		if strings.HasSuffix(domain, suffix) {
			return true
		}
	}

	for _, pattern := range f.patterns {
		if matched, _ := filepath.Match(pattern, domain); matched {
			return true
		}
	}

	return false
}
```

- [ ] **Step 3: Run tests**

Run: `./scripts/test.sh --pkg ./router/dns/fakeip/`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add router/dns/fakeip/
git commit -m "feat(fakeip): add domain filter for fake-ip bypass list"
```

---

### Task 2: fake-ip Pool

**Files:**
- Create: `router/dns/fakeip/pool.go`
- Create: `router/dns/fakeip/pool_test.go`

- [ ] **Step 1: Write pool tests**

```go
// router/dns/fakeip/pool_test.go
package fakeip_test

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shuttle/router/dns/fakeip"
)

func TestPool_AllocateAndReverse(t *testing.T) {
	pool, err := fakeip.NewPool(fakeip.PoolConfig{
		CIDR: netip.MustParsePrefix("198.18.0.0/30"), // 4 IPs: .0, .1, .2, .3
	})
	require.NoError(t, err)

	// Allocate
	ip1 := pool.Lookup("google.com")
	ip2 := pool.Lookup("facebook.com")
	assert.NotEqual(t, ip1, ip2)

	// Same domain returns same IP
	ip1b := pool.Lookup("google.com")
	assert.Equal(t, ip1, ip1b)

	// Reverse
	domain, ok := pool.Reverse(ip1)
	assert.True(t, ok)
	assert.Equal(t, "google.com", domain)

	domain2, ok := pool.Reverse(ip2)
	assert.True(t, ok)
	assert.Equal(t, "facebook.com", domain2)
}

func TestPool_IsFakeIP(t *testing.T) {
	pool, err := fakeip.NewPool(fakeip.PoolConfig{
		CIDR: netip.MustParsePrefix("198.18.0.0/15"),
	})
	require.NoError(t, err)

	assert.True(t, pool.IsFakeIP(netip.MustParseAddr("198.18.0.1")))
	assert.True(t, pool.IsFakeIP(netip.MustParseAddr("198.19.255.254")))
	assert.False(t, pool.IsFakeIP(netip.MustParseAddr("8.8.8.8")))
	assert.False(t, pool.IsFakeIP(netip.MustParseAddr("198.20.0.1")))
}

func TestPool_Wraparound(t *testing.T) {
	// Tiny pool: /30 = 4 addresses, but .0 is network, .3 is broadcast
	// Usable: .1 and .2
	pool, err := fakeip.NewPool(fakeip.PoolConfig{
		CIDR: netip.MustParsePrefix("198.18.0.0/30"),
	})
	require.NoError(t, err)

	ip1 := pool.Lookup("a.com")
	ip2 := pool.Lookup("b.com")
	// Third allocation should evict the oldest
	ip3 := pool.Lookup("c.com")

	// ip3 should reuse ip1's address (LRU eviction)
	assert.Equal(t, ip1, ip3)
	// "a.com" is now evicted
	_, ok := pool.Reverse(ip1)
	assert.True(t, ok) // ip1 now maps to c.com
	domain, _ := pool.Reverse(ip1)
	assert.Equal(t, "c.com", domain)
}

func TestPool_FilteredDomain(t *testing.T) {
	pool, err := fakeip.NewPool(fakeip.PoolConfig{
		CIDR:   netip.MustParsePrefix("198.18.0.0/24"),
		Filter: []string{"+.lan", "stun.*"},
	})
	require.NoError(t, err)

	// Filtered domain should not get a fake IP
	assert.False(t, pool.ShouldFakeIP("router.lan"))
	assert.False(t, pool.ShouldFakeIP("stun.l.google.com"))
	assert.True(t, pool.ShouldFakeIP("www.google.com"))
}
```

- [ ] **Step 2: Implement pool**

```go
// router/dns/fakeip/pool.go
package fakeip

import (
	"fmt"
	"net/netip"
	"sync"
)

// PoolConfig configures the fake-ip pool.
type PoolConfig struct {
	CIDR   netip.Prefix
	Filter []string // domain patterns to exclude
}

// Pool manages fake IP allocation for domains.
type Pool struct {
	prefix  netip.Prefix
	start   netip.Addr // first usable address
	size    uint32     // number of usable addresses
	next    uint32     // next allocation index (wraps around)
	filter  *Filter

	mu      sync.RWMutex
	forward map[string]netip.Addr // domain → fake IP
	reverse map[netip.Addr]string // fake IP → domain
}

// NewPool creates a new fake-ip pool.
func NewPool(cfg PoolConfig) (*Pool, error) {
	if !cfg.CIDR.IsValid() {
		return nil, fmt.Errorf("invalid CIDR: %s", cfg.CIDR)
	}

	// Calculate usable range (skip network and broadcast for IPv4)
	start := cfg.CIDR.Addr().Next() // skip network address
	bits := cfg.CIDR.Bits()
	var size uint32
	if cfg.CIDR.Addr().Is4() {
		total := uint32(1) << (32 - bits)
		if total <= 2 {
			return nil, fmt.Errorf("CIDR too small: %s", cfg.CIDR)
		}
		size = total - 2 // exclude network and broadcast
	} else {
		// IPv6: just use a large chunk
		size = 1 << 20 // 1M addresses
	}

	return &Pool{
		prefix:  cfg.CIDR,
		start:   start,
		size:    size,
		filter:  NewFilter(cfg.Filter),
		forward: make(map[string]netip.Addr),
		reverse: make(map[netip.Addr]string),
	}, nil
}

// ShouldFakeIP returns true if the domain should get a fake IP.
func (p *Pool) ShouldFakeIP(domain string) bool {
	return !p.filter.ShouldSkip(domain)
}

// Lookup returns a fake IP for the domain. If already allocated, returns the same IP.
func (p *Pool) Lookup(domain string) netip.Addr {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Return existing allocation
	if ip, ok := p.forward[domain]; ok {
		return ip
	}

	// Allocate new
	ip := p.addrAt(p.next)
	p.next = (p.next + 1) % p.size

	// Evict old mapping if this IP was already assigned
	if oldDomain, ok := p.reverse[ip]; ok {
		delete(p.forward, oldDomain)
	}

	p.forward[domain] = ip
	p.reverse[ip] = domain
	return ip
}

// Reverse returns the domain for a fake IP.
func (p *Pool) Reverse(ip netip.Addr) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	domain, ok := p.reverse[ip]
	return domain, ok
}

// IsFakeIP returns true if the IP falls within the fake-ip range.
func (p *Pool) IsFakeIP(ip netip.Addr) bool {
	return p.prefix.Contains(ip)
}

// addrAt returns the IP at offset idx from the start address.
func (p *Pool) addrAt(idx uint32) netip.Addr {
	addr := p.start
	for i := uint32(0); i < idx; i++ {
		addr = addr.Next()
	}
	return addr
}
```

- [ ] **Step 3: Run tests**

Run: `./scripts/test.sh --pkg ./router/dns/fakeip/`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add router/dns/fakeip/pool.go router/dns/fakeip/pool_test.go
git commit -m "feat(fakeip): implement fake-ip pool with LRU eviction and domain filter"
```

---

### Task 3: DNS Resolver Integration

**Files:**
- Modify: `router/dns.go`
- Modify: `config/config_routing.go`

- [ ] **Step 1: Read current DNS resolver**

Read `router/dns.go` to understand the `Resolve(domain)` method signature and flow.

- [ ] **Step 2: Add fake-ip config fields**

In `config/config_routing.go`, add to `DNSConfig`:

```go
type DNSConfig struct {
	// ... existing fields ...
	Mode         string   `yaml:"mode,omitempty" json:"mode,omitempty"`              // "normal" or "fake-ip"
	FakeIPRange  string   `yaml:"fake_ip_range,omitempty" json:"fake_ip_range,omitempty"` // default "198.18.0.0/15"
	FakeIPFilter []string `yaml:"fake_ip_filter,omitempty" json:"fake_ip_filter,omitempty"`
	Persist      bool     `yaml:"persist,omitempty" json:"persist,omitempty"`
}
```

- [ ] **Step 3: Integrate fake-ip into DNS resolver**

In `router/dns.go`, modify the `Resolve` method:

```go
func (r *DNSResolver) Resolve(ctx context.Context, domain string) (net.IP, error) {
	// fake-ip mode check
	if r.fakeIPPool != nil && r.fakeIPPool.ShouldFakeIP(domain) {
		fakeAddr := r.fakeIPPool.Lookup(domain)
		return fakeAddr.AsSlice(), nil
	}

	// ... existing resolution logic (domestic/remote split DNS) ...
}
```

Add a `fakeIPPool *fakeip.Pool` field to `DNSResolver`. Initialize it during DNS resolver construction if `cfg.Mode == "fake-ip"`.

- [ ] **Step 4: Add reverse lookup helper**

```go
// ReverseFakeIP returns the domain for a fake IP, or empty string if not a fake IP.
func (r *DNSResolver) ReverseFakeIP(ip net.IP) (string, bool) {
	if r.fakeIPPool == nil {
		return "", false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return "", false
	}
	return r.fakeIPPool.Reverse(addr)
}

// IsFakeIP returns true if the IP is in the fake-ip range.
func (r *DNSResolver) IsFakeIP(ip net.IP) bool {
	if r.fakeIPPool == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	return r.fakeIPPool.IsFakeIP(addr)
}
```

- [ ] **Step 5: Integrate into routing path**

In `engine/engine_inbound.go` (or wherever the inbound router resolves destinations), add fake-ip reverse lookup:

```go
// Before routing decision, if destination IP is a fake IP, recover the domain
if r.dns.IsFakeIP(destIP) {
    if domain, ok := r.dns.ReverseFakeIP(destIP); ok {
        metadata.Domain = domain
        // For proxy outbound: use domain (server will resolve)
        // For direct outbound: resolve domain to real IP
    }
}
```

- [ ] **Step 6: Run tests**

Run: `./scripts/test.sh --pkg ./router/...`
Expected: PASS — `mode: ""` defaults to normal, no behavior change.

- [ ] **Step 7: Commit**

```bash
git add router/dns.go router/dns/fakeip/ config/config_routing.go engine/engine_inbound.go
git commit -m "feat(dns): integrate fake-ip mode into DNS resolver and routing pipeline"
```
