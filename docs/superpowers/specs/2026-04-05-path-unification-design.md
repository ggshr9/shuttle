# Path Unification & Code Simplification Design

**Date:** 2026-04-05
**Status:** Approved

## Problem

The Engine has two code paths for starting proxy listeners (`startProxies` forks between legacy and inbound), causing:

1. **Duplicated resilience logic** — `dialProxyStream` (retry+CB) for legacy, `ResilientOutbound` middleware for inbound
2. **Cognitive burden** — every bug fix or feature must consider both paths
3. **Missing TUN inbound** — SOCKS5/HTTP have inbound factories, TUN does not, blocking full migration
4. **No config validation** for inbound/outbound types
5. **Mesh tightly coupled to TUN** — `connectMesh` embedded in `startTUN`, mesh lifecycle managed by Engine ad-hoc

## Solution

### Phase A: Unify to Single Inbound Path

1. **Create TUNInbound factory** — register in `proxy/tun_inbound.go`, matching SOCKS5/HTTP pattern
2. **`adaptLegacyConfig()`** — converts `cfg.Proxy.*` into equivalent `cfg.Inbounds[]` entries at `startInternal` time, zero breaking change for existing YAML configs
3. **Remove legacy path** — delete `startProxies` fork, `startSOCKS5`, `startHTTPProxy`, `startTUN`, `dialProxyStream`. Keep only `startInbounds` and `dialProxyStreamSimple`.
4. **Validate inbound/outbound config** — check type exists in registry, validate options schema

### Phase B: Mesh Independence

5. **Create MeshManager** — owns mesh connection lifecycle (connect with retry, reconnect, close, stats)
6. **Decouple from TUN** — introduce `MeshPacketHandler` interface that TUN implements; MeshManager injects itself into TUN via this interface instead of direct field assignment
7. **Wire into Engine** — MeshManager is an Engine-level component (like ObservabilityManager), started after TUN inbound is ready

### Data Flow After Unification

```
User YAML Config
    │
    ├── proxy.socks5/http/tun (legacy)
    │       │
    │   adaptLegacyConfig()
    │       │
    └── inbounds: [...] ─────────→ cfg.Inbounds[]
                                       │
                                  startInbounds() (single path)
                                       │
                              ┌────────┼────────┐
                              ▼        ▼        ▼
                          SOCKS5    HTTP     TUN
                          Inbound   Inbound  Inbound
                              │        │        │
                              └───┬────┘────────┘
                                  ▼
                            InboundRouter
                                  │
                       ┌──────────┼──────────┐
                       ▼          ▼          ▼
                   DirectOB   RejectOB   ProxyOB + ResilientOutbound
                                             │
                                    dialProxyStreamSimple

MeshManager (independent)
    │
    ├── connects via selector
    ├── injects MeshClient into TUN via MeshPacketHandler interface
    └── owns reconnect + lifecycle
```

### MeshPacketHandler Interface

```go
// MeshPacketHandler allows a mesh subsystem to inject/extract packets
// through a TUN device without TUN knowing about mesh internals.
type MeshPacketHandler interface {
    IsMeshDestination(ip net.IP) bool
    SendPacket(pkt []byte) error
    ReceivePacket() ([]byte, error)
    MeshCIDR() string
    Close() error
}
```

TUNServer changes from `MeshClient *mesh.MeshClient` to `MeshHandler MeshPacketHandler`. MeshClient implements MeshPacketHandler.

### adaptLegacyConfig()

Converts at runtime, no config file modification:

```go
func adaptLegacyConfig(cfg *config.ClientConfig) {
    if len(cfg.Inbounds) > 0 {
        return // explicit inbounds take precedence
    }
    if cfg.Proxy.SOCKS5.Enabled {
        cfg.Inbounds = append(cfg.Inbounds, config.InboundConfig{
            Tag:  "socks5",
            Type: "socks5",
            Options: marshal(map[string]any{
                "listen": cfg.Proxy.SOCKS5.Listen,
            }),
        })
    }
    // same for HTTP, TUN
}
```

### Deletion List

| File/Method | Status |
|-------------|--------|
| `engine/engine_setup.go:startProxies` | Delete (replaced by direct startInbounds call) |
| `engine/engine_setup.go:startSOCKS5` | Delete |
| `engine/engine_setup.go:startHTTPProxy` | Delete |
| `engine/engine_setup.go:startTUN` | Delete |
| `engine/engine_setup.go:dialProxyStream` | Delete (keep only dialProxyStreamSimple) |
| `engine/engine_lifecycle.go:connectMesh` | Move to MeshManager |
| `proxy/tun.go:MeshClient` field | Replace with MeshPacketHandler |

### Config Validation Additions

Add to `ClientConfig.Validate()`:
- Each `cfg.Inbounds[i].Type` must exist in `adapter.GetInbound(type)`
- Each `cfg.Inbounds[i].Tag` must be unique
- Each `cfg.Outbounds[i].Type` must exist in `adapter.GetOutbound(type)`
- Each `cfg.Outbounds[i].Tag` must be unique and not collide with builtins (direct/reject/proxy)
