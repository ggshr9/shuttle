# Path Unification & Code Simplification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the legacy/inbound fork in `startProxies`, unify to a single inbound path, extract MeshManager as an independent Engine component, and add config validation for inbound/outbound types.

**Architecture:** Legacy `proxy.*` config is automatically converted to `Inbounds[]` entries via `adaptLegacyConfig()`. TUNInbound factory completes the inbound registry. MeshManager owns mesh lifecycle independently from TUN. `dialProxyStream` (with embedded retry+CB) is deleted; only `dialProxyStreamSimple` remains.

**Tech Stack:** Go 1.24+, `log/slog`, existing adapter registry pattern

---

## File Structure

### Phase A: Unify to Single Path

- Create: `proxy/tun_inbound.go` — TUNInbound + TUNInboundFactory
- Create: `engine/adapt_legacy.go` — adaptLegacyConfig() converter
- Create: `engine/adapt_legacy_test.go` — tests for config adaptation
- Modify: `engine/engine_lifecycle.go` — call adaptLegacyConfig, remove startProxies call
- Modify: `engine/engine_setup.go` — delete startProxies, startSOCKS5, startHTTPProxy, startTUN, dialProxyStream
- Modify: `config/config_validate.go` — add inbound/outbound validation
- Test: `proxy/tun_inbound_test.go`, `config/config_validate_test.go`

### Phase B: Mesh Independence

- Create: `proxy/mesh_handler.go` — MeshPacketHandler interface
- Create: `engine/mesh_manager.go` — MeshManager component
- Create: `engine/mesh_manager_test.go` — MeshManager tests
- Modify: `proxy/tun.go` — replace `MeshClient *mesh.MeshClient` with `MeshHandler MeshPacketHandler`
- Modify: `mesh/client.go` — implement MeshPacketHandler interface
- Modify: `engine/engine_lifecycle.go` — remove connectMesh, wire MeshManager
- Modify: `engine/engine.go` — add meshManager field

---

## Task 1: Create TUNInbound Factory

Create `proxy/tun_inbound.go` following the exact same pattern as SOCKS5Inbound and HTTPInbound.

**Files:**
- Create: `proxy/tun_inbound.go`
- Create: `proxy/tun_inbound_test.go`

- [ ] **Step 1: Write the failing test**

```go
// proxy/tun_inbound_test.go
package proxy

import (
	"testing"

	"github.com/shuttleX/shuttle/adapter"
)

func TestTUNInboundFactory_Registered(t *testing.T) {
	f := adapter.GetInbound("tun")
	if f == nil {
		t.Fatal("tun inbound factory not registered")
	}
	if f.Type() != "tun" {
		t.Errorf("Type() = %q, want %q", f.Type(), "tun")
	}
}

func TestTUNInboundFactory_Create(t *testing.T) {
	f := adapter.GetInbound("tun")
	if f == nil {
		t.Fatal("tun inbound factory not registered")
	}

	ib, err := f.Create("tun-test", []byte(`{"device_name":"test0","cidr":"198.18.0.0/16"}`), adapter.InboundDeps{})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if ib.Tag() != "tun-test" {
		t.Errorf("Tag() = %q, want %q", ib.Tag(), "tun-test")
	}
	if ib.Type() != "tun" {
		t.Errorf("Type() = %q, want %q", ib.Type(), "tun")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `./scripts/test.sh --run TestTUNInbound --pkg ./proxy/`
Expected: FAIL — tun inbound factory not registered

- [ ] **Step 3: Implement TUNInbound**

```go
// proxy/tun_inbound.go
package proxy

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"

	"github.com/shuttleX/shuttle/adapter"
)

// TUNInboundConfig configures a TUN inbound device.
type TUNInboundConfig struct {
	DeviceName string `json:"device_name,omitempty"`
	CIDR       string `json:"cidr,omitempty"`
	MTU        int    `json:"mtu,omitempty"`
	AutoRoute  bool   `json:"auto_route,omitempty"`
	TunFD      int    `json:"tun_fd,omitempty"`
}

// TUNInbound wraps TUNServer as an adapter.Inbound.
type TUNInbound struct {
	tag    string
	config TUNInboundConfig
	server *TUNServer
	logger *slog.Logger
}

func (t *TUNInbound) Tag() string  { return t.tag }
func (t *TUNInbound) Type() string { return "tun" }

// Server returns the underlying TUNServer for mesh integration.
func (t *TUNInbound) Server() *TUNServer { return t.server }

func (t *TUNInbound) Start(ctx context.Context, router adapter.InboundRouter) error {
	dialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return router.RouteConnection(ctx, &adapter.ConnMetadata{
			Destination: addr,
			Network:     network,
			Process:     ProcessFromContext(ctx),
			Protocol:    "tun",
			InboundTag:  t.tag,
		})
	}
	t.server = NewTUNServer(&TUNConfig{
		DeviceName: t.config.DeviceName,
		CIDR:       t.config.CIDR,
		MTU:        t.config.MTU,
		AutoRoute:  t.config.AutoRoute,
		TunFD:      t.config.TunFD,
	}, dialer, t.logger)
	return t.server.Start(ctx)
}

func (t *TUNInbound) Close() error {
	if t.server != nil {
		return t.server.Close()
	}
	return nil
}

// TUNInboundFactory creates TUNInbound instances.
type TUNInboundFactory struct{}

func (f *TUNInboundFactory) Type() string { return "tun" }

func (f *TUNInboundFactory) Create(tag string, options json.RawMessage, deps adapter.InboundDeps) (adapter.Inbound, error) {
	var cfg TUNInboundConfig
	if options != nil {
		if err := json.Unmarshal(options, &cfg); err != nil {
			return nil, err
		}
	}
	return &TUNInbound{tag: tag, config: cfg, logger: deps.Logger}, nil
}

var _ adapter.Inbound = (*TUNInbound)(nil)
var _ adapter.InboundFactory = (*TUNInboundFactory)(nil)

func init() {
	adapter.RegisterInbound(&TUNInboundFactory{})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `./scripts/test.sh --run TestTUNInbound --pkg ./proxy/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add proxy/tun_inbound.go proxy/tun_inbound_test.go
git commit -m "feat(proxy): add TUNInbound factory for pluggable inbound registry"
```

---

## Task 2: Create adaptLegacyConfig()

Convert legacy `proxy.*` config into equivalent `Inbounds[]` entries so the legacy YAML format continues to work through the unified inbound path.

**Files:**
- Create: `engine/adapt_legacy.go`
- Create: `engine/adapt_legacy_test.go`

- [ ] **Step 1: Write the failing test**

```go
// engine/adapt_legacy_test.go
package engine

import (
	"encoding/json"
	"testing"

	"github.com/shuttleX/shuttle/config"
)

func TestAdaptLegacyConfig_SOCKS5(t *testing.T) {
	cfg := config.DefaultClientConfig()
	cfg.Proxy.SOCKS5.Enabled = true
	cfg.Proxy.SOCKS5.Listen = "127.0.0.1:1080"

	adaptLegacyConfig(cfg)

	if len(cfg.Inbounds) == 0 {
		t.Fatal("expected inbounds to be populated")
	}
	found := false
	for _, ib := range cfg.Inbounds {
		if ib.Type == "socks5" {
			found = true
			if ib.Tag != "socks5" {
				t.Errorf("tag = %q, want %q", ib.Tag, "socks5")
			}
			var opts map[string]string
			json.Unmarshal(ib.Options, &opts)
			if opts["listen"] != "127.0.0.1:1080" {
				t.Errorf("listen = %q, want %q", opts["listen"], "127.0.0.1:1080")
			}
		}
	}
	if !found {
		t.Error("socks5 inbound not found")
	}
}

func TestAdaptLegacyConfig_HTTP(t *testing.T) {
	cfg := config.DefaultClientConfig()
	cfg.Proxy.HTTP.Enabled = true
	cfg.Proxy.HTTP.Listen = "127.0.0.1:8080"

	adaptLegacyConfig(cfg)

	found := false
	for _, ib := range cfg.Inbounds {
		if ib.Type == "http" {
			found = true
		}
	}
	if !found {
		t.Error("http inbound not found")
	}
}

func TestAdaptLegacyConfig_TUN(t *testing.T) {
	cfg := config.DefaultClientConfig()
	cfg.Proxy.TUN.Enabled = true
	cfg.Proxy.TUN.CIDR = "198.18.0.0/16"

	adaptLegacyConfig(cfg)

	found := false
	for _, ib := range cfg.Inbounds {
		if ib.Type == "tun" {
			found = true
			var opts map[string]interface{}
			json.Unmarshal(ib.Options, &opts)
			if opts["cidr"] != "198.18.0.0/16" {
				t.Errorf("cidr = %v, want %q", opts["cidr"], "198.18.0.0/16")
			}
		}
	}
	if !found {
		t.Error("tun inbound not found")
	}
}

func TestAdaptLegacyConfig_SkipsIfInboundsExist(t *testing.T) {
	cfg := config.DefaultClientConfig()
	cfg.Proxy.SOCKS5.Enabled = true
	cfg.Inbounds = []config.InboundConfig{{Tag: "custom", Type: "socks5"}}

	adaptLegacyConfig(cfg)

	if len(cfg.Inbounds) != 1 {
		t.Errorf("expected 1 inbound (existing), got %d", len(cfg.Inbounds))
	}
	if cfg.Inbounds[0].Tag != "custom" {
		t.Error("existing inbound was overwritten")
	}
}

func TestAdaptLegacyConfig_AllThree(t *testing.T) {
	cfg := config.DefaultClientConfig()
	cfg.Proxy.SOCKS5.Enabled = true
	cfg.Proxy.SOCKS5.Listen = "127.0.0.1:1080"
	cfg.Proxy.HTTP.Enabled = true
	cfg.Proxy.HTTP.Listen = "127.0.0.1:8080"
	cfg.Proxy.TUN.Enabled = true

	adaptLegacyConfig(cfg)

	if len(cfg.Inbounds) != 3 {
		t.Errorf("expected 3 inbounds, got %d", len(cfg.Inbounds))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `./scripts/test.sh --run TestAdaptLegacyConfig --pkg ./engine/`
Expected: FAIL — `adaptLegacyConfig` not defined

- [ ] **Step 3: Implement adaptLegacyConfig**

```go
// engine/adapt_legacy.go
package engine

import (
	"encoding/json"

	"github.com/shuttleX/shuttle/config"
)

// adaptLegacyConfig converts legacy proxy.* config into equivalent Inbound entries.
// If cfg.Inbounds is already populated, it does nothing (explicit config takes precedence).
// This allows old YAML configs to work through the unified inbound path.
func adaptLegacyConfig(cfg *config.ClientConfig) {
	if len(cfg.Inbounds) > 0 {
		return
	}

	if cfg.Proxy.SOCKS5.Enabled {
		opts, _ := json.Marshal(map[string]string{
			"listen": cfg.Proxy.SOCKS5.Listen,
		})
		cfg.Inbounds = append(cfg.Inbounds, config.InboundConfig{
			Tag:     "socks5",
			Type:    "socks5",
			Options: opts,
		})
	}

	if cfg.Proxy.HTTP.Enabled {
		opts, _ := json.Marshal(map[string]string{
			"listen": cfg.Proxy.HTTP.Listen,
		})
		cfg.Inbounds = append(cfg.Inbounds, config.InboundConfig{
			Tag:     "http",
			Type:    "http",
			Options: opts,
		})
	}

	if cfg.Proxy.TUN.Enabled {
		opts, _ := json.Marshal(map[string]interface{}{
			"device_name": cfg.Proxy.TUN.DeviceName,
			"cidr":        cfg.Proxy.TUN.CIDR,
			"mtu":         cfg.Proxy.TUN.MTU,
			"auto_route":  cfg.Proxy.TUN.AutoRoute,
			"tun_fd":      cfg.Proxy.TUN.TunFD,
		})
		cfg.Inbounds = append(cfg.Inbounds, config.InboundConfig{
			Tag:     "tun",
			Type:    "tun",
			Options: opts,
		})
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `./scripts/test.sh --run TestAdaptLegacyConfig --pkg ./engine/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add engine/adapt_legacy.go engine/adapt_legacy_test.go
git commit -m "feat(engine): add adaptLegacyConfig to convert proxy.* config to inbound entries"
```

---

## Task 3: Remove Legacy Path — Unify to startInbounds

Delete the legacy proxy starters, the startProxies fork, and dialProxyStream. Wire startInternal to always use startInbounds via adaptLegacyConfig.

**Files:**
- Modify: `engine/engine_lifecycle.go` — call adaptLegacyConfig before startInbounds, remove connectMesh call (moved to Task 6)
- Modify: `engine/engine_setup.go` — delete startProxies, startSOCKS5, startHTTPProxy, startTUN, dialProxyStream, buildRetryConfig (if only used by legacy)
- Modify: `engine/engine_inbound.go` — now the only proxy start path
- Test: all existing engine tests must pass

- [ ] **Step 1: Read current startInternal to understand the full flow**

The current flow in `startInternal` is:
1. Build CC → build transports → create selector
2. Build router + DNS
3. Create dialer via `traffic.CreateDialer(...)` with `dialProxyStream` callback
4. Build plugin chain via obs.BuildChain
5. Wrap dialer with chain
6. Call `startProxies(ctx, cfg, dialer, sel, cancel)` which forks
7. Start speed loop, network monitor

After this task:
1. Build CC → build transports → create selector
2. Build router + DNS (store on engine for startInbounds to access)
3. `adaptLegacyConfig(cfgSnap)` — convert legacy proxy config
4. Build plugin chain
5. Call `startInbounds(ctx, cfgSnap)` — unified path, wraps dialer internally
6. Start speed loop, network monitor

Key: The legacy path built a centralized dialer and passed it to each proxy. The inbound path doesn't need that — each inbound routes via InboundRouter → Outbound.DialContext. So the `traffic.CreateDialer` + `wrapDialerWithChain` calls in startInternal become unnecessary when going through the inbound path.

However, the inbound path's ProxyOutbound still needs the plugin chain wrapping. Check: does `startInbounds` already handle this? Looking at engine_inbound.go — the ProxyOutbound calls `dialProxyStreamSimple` which does NOT go through the plugin chain. The plugin chain wrapping was done on the legacy dialer.

**Important**: We need to ensure the plugin chain (metrics, conntrack, logger) still wraps connections in the unified path. Currently the chain wrapping happens in `wrapDialer` which wraps the legacy dialer. For the inbound path, we need to wrap the outbound's DialContext instead.

Solution: Apply the chain wrapping inside `startInbounds` by wrapping the ProxyOutbound's connections. This can be done by inserting a chain-wrapping middleware (similar to how ResilientOutbound wraps).

- [ ] **Step 2: Create ChainOutbound middleware** (if needed)

Add to `engine/outbound_middleware.go`:

```go
// ChainOutbound wraps an adapter.Outbound so connections flow through a plugin chain.
type ChainOutbound struct {
	inner adapter.Outbound
	chain *plugin.Chain
}

func NewChainOutbound(inner adapter.Outbound, chain *plugin.Chain) *ChainOutbound {
	return &ChainOutbound{inner: inner, chain: chain}
}

func (c *ChainOutbound) Tag() string  { return c.inner.Tag() }
func (c *ChainOutbound) Type() string { return c.inner.Type() }
func (c *ChainOutbound) Close() error { return c.inner.Close() }

func (c *ChainOutbound) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := c.inner.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	wrapped, err := c.chain.OnConnect(conn, address)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &chainConn{Conn: wrapped, chain: c.chain}, nil
}

var _ adapter.Outbound = (*ChainOutbound)(nil)
```

- [ ] **Step 3: Update startInbounds to apply chain middleware**

In `engine/engine_inbound.go`, after building the proxy outbound and wrapping with ResilientOutbound, also wrap with ChainOutbound:

```go
outbounds := e.traffic.BuildBuiltinOutbounds(cfg, e)

// Apply plugin chain to proxy outbound for metrics/conntrack/logging.
chain := e.obs.Chain()
if proxyOb, ok := outbounds["proxy"]; ok && chain != nil {
	resilient := NewResilientOutbound(proxyOb, ResilientOutboundConfig{
		CircuitBreaker: e.circuitBreaker,
		RetryConfig:    e.buildRetryConfig(cfg.Retry),
	})
	outbounds["proxy"] = NewChainOutbound(resilient, chain)
}
```

- [ ] **Step 4: Simplify startInternal**

Remove from `startInternal`:
- The `retryCfg`, `shaperCfg`, `classifier` variables
- The `traffic.CreateDialer(...)` call
- The `traffic.WrapDialerWithChain(...)` call
- The `startProxies(...)` call

Replace with:
```go
adaptLegacyConfig(cfgSnap)

closers, err := e.startInbounds(ctx, cfgSnap)
if err != nil {
	sel.Close()
	e.obs.CloseChain()
	return fail(err)
}
```

- [ ] **Step 5: Delete legacy methods from engine_setup.go**

Delete these methods entirely:
- `startProxies` (~50 lines)
- `startSOCKS5` (~12 lines)
- `startHTTPProxy` (~12 lines)
- `startTUN` (~22 lines)
- `dialProxyStream` (~78 lines)

Rename `dialProxyStreamSimple` to `dialProxyStream` (it's now the only version).

- [ ] **Step 6: Delete connectMesh from engine_lifecycle.go**

Remove the `connectMesh` method (~45 lines). Mesh will be handled by MeshManager in Task 6.

- [ ] **Step 7: Run all tests**

Run: `./scripts/test.sh`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add engine/engine_lifecycle.go engine/engine_setup.go engine/engine_inbound.go engine/outbound_middleware.go
git commit -m "refactor(engine): unify to single inbound path, delete legacy proxy starters"
```

---

## Task 4: Add Inbound/Outbound Config Validation

Add validation for `cfg.Inbounds` and `cfg.Outbounds` to `ClientConfig.Validate()`.

**Files:**
- Modify: `config/config_validate.go`
- Test: `config/config_validate_test.go` (add new tests)

- [ ] **Step 1: Write the failing tests**

```go
// Add to config/config_validate_test.go

func TestValidate_InboundTypeUnknown(t *testing.T) {
	cfg := validClientConfig()
	cfg.Inbounds = []InboundConfig{{Tag: "foo", Type: "nonexistent"}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for unknown inbound type")
	}
}

func TestValidate_InboundDuplicateTag(t *testing.T) {
	cfg := validClientConfig()
	cfg.Inbounds = []InboundConfig{
		{Tag: "s1", Type: "socks5"},
		{Tag: "s1", Type: "http"},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for duplicate inbound tag")
	}
}

func TestValidate_OutboundReservedTag(t *testing.T) {
	cfg := validClientConfig()
	cfg.Outbounds = []OutboundConfig{{Tag: "direct", Type: "custom"}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for reserved outbound tag")
	}
}

func TestValidate_InboundValid(t *testing.T) {
	cfg := validClientConfig()
	cfg.Inbounds = []InboundConfig{{Tag: "s1", Type: "socks5"}}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
```

Note: The validation should call `adapter.GetInbound(type)` to check if a type is registered. However, this creates a dependency from `config` package to `adapter` package. If this is undesirable, validate only structural constraints (non-empty tag/type, uniqueness) and leave type validation to runtime.

**Recommended approach**: Only validate structural constraints in `Validate()` to avoid circular dependency:
- Tag must be non-empty
- Type must be non-empty
- Tags must be unique within inbounds and within outbounds
- Outbound tags must not collide with builtins: "direct", "reject", "proxy"

- [ ] **Step 2: Run tests to verify they fail**

Run: `./scripts/test.sh --run TestValidate_Inbound --pkg ./config/`

- [ ] **Step 3: Implement validation**

Add to `config/config_validate.go` inside `ClientConfig.Validate()`:

```go
// Validate inbound configs.
inboundTags := make(map[string]bool)
for i, ib := range c.Inbounds {
	if ib.Tag == "" {
		errs = append(errs, fmt.Errorf("inbounds[%d]: tag is required", i))
	}
	if ib.Type == "" {
		errs = append(errs, fmt.Errorf("inbounds[%d]: type is required", i))
	}
	if ib.Tag != "" {
		if inboundTags[ib.Tag] {
			errs = append(errs, fmt.Errorf("inbounds[%d]: duplicate tag %q", i, ib.Tag))
		}
		inboundTags[ib.Tag] = true
	}
}

// Validate outbound configs.
reservedTags := map[string]bool{"direct": true, "reject": true, "proxy": true}
outboundTags := make(map[string]bool)
for i, ob := range c.Outbounds {
	if ob.Tag == "" {
		errs = append(errs, fmt.Errorf("outbounds[%d]: tag is required", i))
	}
	if ob.Type == "" {
		errs = append(errs, fmt.Errorf("outbounds[%d]: type is required", i))
	}
	if ob.Tag != "" {
		if reservedTags[ob.Tag] {
			errs = append(errs, fmt.Errorf("outbounds[%d]: tag %q is reserved", i, ob.Tag))
		}
		if outboundTags[ob.Tag] {
			errs = append(errs, fmt.Errorf("outbounds[%d]: duplicate tag %q", i, ob.Tag))
		}
		outboundTags[ob.Tag] = true
	}
}
```

- [ ] **Step 4: Run tests**

Run: `./scripts/test.sh --run TestValidate --pkg ./config/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add config/config_validate.go config/config_validate_test.go
git commit -m "feat(config): add inbound/outbound structural validation"
```

---

## Task 5: Create MeshPacketHandler Interface

Decouple TUN from direct mesh.MeshClient dependency by introducing an interface.

**Files:**
- Create: `proxy/mesh_handler.go`
- Modify: `proxy/tun.go` — replace `MeshClient *mesh.MeshClient` with `MeshHandler MeshPacketHandler`
- Modify: `mesh/client.go` — implement MeshPacketHandler (add SendPacket/ReceivePacket methods if needed)
- Test: existing TUN and mesh tests must pass

- [ ] **Step 1: Define the interface**

```go
// proxy/mesh_handler.go
package proxy

import "net"

// MeshPacketHandler abstracts mesh packet send/receive for TUN integration.
// This decouples TUN from the concrete mesh.MeshClient type, allowing
// MeshManager to inject a handler without TUN knowing about mesh internals.
type MeshPacketHandler interface {
	IsMeshDestination(ip net.IP) bool
	SendPacket(pkt []byte) error
	ReceivePacket() ([]byte, error)
	MeshCIDR() string
	Close() error
}
```

- [ ] **Step 2: Update TUNServer**

In `proxy/tun.go`, replace:
```go
MeshClient *mesh.MeshClient
```
with:
```go
MeshHandler MeshPacketHandler
```

Update all references: `t.MeshClient` → `t.MeshHandler`, `mc.Send(pkt)` → `mh.SendPacket(pkt)`, `mc.Receive()` → `mh.ReceivePacket()`, `mc.IsMeshDestination(ip)` → `mh.IsMeshDestination(ip)`.

- [ ] **Step 3: Implement interface on MeshClient**

In `mesh/client.go`, add adapter methods if `Send`/`Receive` signatures differ:
```go
func (mc *MeshClient) SendPacket(pkt []byte) error   { return mc.Send(pkt) }
func (mc *MeshClient) ReceivePacket() ([]byte, error) { return mc.Receive() }
```

Verify MeshClient already has `IsMeshDestination`, `MeshCIDR`, `Close`.

- [ ] **Step 4: Run tests**

Run: `./scripts/test.sh`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add proxy/mesh_handler.go proxy/tun.go mesh/client.go
git commit -m "refactor(proxy): introduce MeshPacketHandler interface to decouple TUN from mesh"
```

---

## Task 6: Create MeshManager

Extract mesh connection lifecycle from Engine into an independent MeshManager component.

**Files:**
- Create: `engine/mesh_manager.go`
- Create: `engine/mesh_manager_test.go`
- Modify: `engine/engine.go` — add meshManager field
- Modify: `engine/engine_lifecycle.go` — start/stop MeshManager
- Modify: `engine/engine_inbound.go` — after TUNInbound starts, inject mesh handler

- [ ] **Step 1: Write the failing test**

```go
// engine/mesh_manager_test.go
package engine

import (
	"testing"
)

func TestMeshManager_New(t *testing.T) {
	mm := NewMeshManager(nil)
	if mm == nil {
		t.Fatal("NewMeshManager returned nil")
	}
}
```

- [ ] **Step 2: Implement MeshManager**

```go
// engine/mesh_manager.go
package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/mesh"
	"github.com/shuttleX/shuttle/proxy"
	"github.com/shuttleX/shuttle/transport/selector"
)

const meshMaxRetries = 3

// MeshManager owns the mesh VPN connection lifecycle independently from TUN.
// It connects to the server via the transport selector, performs the mesh
// handshake, and injects the MeshClient into TUN via the MeshPacketHandler interface.
type MeshManager struct {
	mu     sync.Mutex
	client *mesh.MeshClient
	logger *slog.Logger
	bgWg   sync.WaitGroup
}

// NewMeshManager creates a MeshManager.
func NewMeshManager(logger *slog.Logger) *MeshManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &MeshManager{logger: logger}
}

// Start connects to the mesh server and injects the handler into TUN.
// It retries up to meshMaxRetries times.
func (mm *MeshManager) Start(ctx context.Context, cfg *config.ClientConfig, sel *selector.Selector, tunInbound *proxy.TUNInbound) error {
	if !cfg.Mesh.Enabled {
		return nil
	}
	if tunInbound == nil || tunInbound.Server() == nil {
		mm.logger.Warn("mesh requires TUN to be enabled, skipping mesh")
		return nil
	}

	serverAddr := cfg.Server.Addr
	var lastErr error
	for attempt := 1; attempt <= meshMaxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		conn, err := sel.Dial(ctx, serverAddr)
		if err != nil {
			lastErr = err
			mm.logger.Warn("mesh: dial failed, retrying", "attempt", attempt, "err", err)
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		mc, err := mesh.NewMeshClient(ctx, func(ctx context.Context) (io.ReadWriteCloser, error) {
			return conn.OpenStream(ctx)
		})
		if err != nil {
			conn.Close()
			lastErr = err
			mm.logger.Warn("mesh: handshake failed, retrying", "attempt", attempt, "err", err)
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		mm.mu.Lock()
		mm.client = mc
		mm.mu.Unlock()

		tunServer := tunInbound.Server()
		tunServer.MeshHandler = mc
		if err := tunServer.AddMeshRoute(mc.MeshCIDR()); err != nil {
			mm.logger.Warn("mesh: add route failed", "err", err)
		}

		// Start receive loop in background.
		mm.bgWg.Add(1)
		go func() {
			defer mm.bgWg.Done()
			tunServer.MeshReceiveLoop(ctx)
		}()

		mm.logger.Info("mesh connected", "virtual_ip", mc.VirtualIP(), "cidr", mc.MeshCIDR())
		return nil
	}
	return fmt.Errorf("mesh: all %d attempts failed: %w", meshMaxRetries, lastErr)
}

// Client returns the current MeshClient, or nil if not connected.
func (mm *MeshManager) Client() *mesh.MeshClient {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	return mm.client
}

// Close shuts down the mesh connection and waits for background goroutines.
func (mm *MeshManager) Close() error {
	mm.mu.Lock()
	mc := mm.client
	mm.client = nil
	mm.mu.Unlock()

	if mc != nil {
		mc.Close()
	}
	mm.bgWg.Wait()
	return nil
}
```

- [ ] **Step 3: Wire MeshManager into Engine**

In `engine/engine.go`, add field:
```go
meshManager *MeshManager
```

In `New()`, initialize:
```go
meshManager: NewMeshManager(logger),
```

In `engine/engine_inbound.go` or `engine/engine_lifecycle.go`, after `startInbounds` succeeds, find the TUN inbound and start mesh:

```go
// After startInbounds succeeds, start mesh if configured.
if cfgSnap.Mesh.Enabled {
	var tunIB *proxy.TUNInbound
	for _, ib := range e.inbounds {
		if tun, ok := ib.(*proxy.TUNInbound); ok {
			tunIB = tun
			break
		}
	}
	if err := e.meshManager.Start(ctx, cfgSnap, sel, tunIB); err != nil {
		e.logger.Warn("mesh start failed", "err", err)
	}
}
```

In `stopInternal`, add mesh cleanup:
```go
if e.meshManager != nil {
	e.meshManager.Close()
}
```

- [ ] **Step 4: Remove old meshClient field and connectMesh references**

Remove from Engine struct: `meshClient *mesh.MeshClient`
Update `Status()` to use `e.meshManager.Client()` instead of `e.meshClient`.

- [ ] **Step 5: Run all tests**

Run: `./scripts/test.sh`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add engine/mesh_manager.go engine/mesh_manager_test.go engine/engine.go engine/engine_lifecycle.go engine/engine_inbound.go
git commit -m "refactor(engine): extract MeshManager as independent component"
```

---

## Task 7: Call full Validate() in Engine.Start

Wire `cfg.Validate()` into `startInternal` before any state changes.

**Files:**
- Modify: `engine/engine_lifecycle.go`

- [ ] **Step 1: Add Validate call at the start of startInternal**

After the state check and before building anything, add:

```go
if err := cfgSnap.Validate(); err != nil {
	return fail(fmt.Errorf("config validation: %w", err))
}
```

This should go right after `cfgSnap := e.cfg.DeepCopy()`.

- [ ] **Step 2: Run tests**

Run: `./scripts/test.sh`
Expected: All PASS (existing test configs must be valid)

- [ ] **Step 3: Commit**

```bash
git add engine/engine_lifecycle.go
git commit -m "feat(engine): call full config.Validate() in Start before building subsystems"
```

---

## Self-Review

1. **Spec coverage**: All items from the design doc are covered — TUNInbound (T1), adaptLegacyConfig (T2), legacy path removal (T3), config validation (T4), MeshPacketHandler (T5), MeshManager (T6), Validate in Start (T7).

2. **Placeholder scan**: No TBD/TODO found.

3. **Type consistency**: `TUNInbound`, `MeshPacketHandler`, `MeshManager`, `ChainOutbound`, `adaptLegacyConfig` — names consistent across all tasks.

4. **Dependency order**:
   - T1 (TUNInbound) — independent
   - T2 (adaptLegacyConfig) — independent
   - T3 (remove legacy) — depends on T1, T2
   - T4 (validation) — independent
   - T5 (MeshPacketHandler) — independent
   - T6 (MeshManager) — depends on T3, T5
   - T7 (Validate in Start) — depends on T4

   Parallelizable: T1+T2+T4+T5 can run in parallel. T3 follows T1+T2. T6 follows T3+T5. T7 follows T4.
