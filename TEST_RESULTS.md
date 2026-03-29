# Test Environment Validation Report

**Date:** 2026-03-29
**Environment:** Cloud (Claude Code Web)
**Runner:** `./scripts/test.sh` (host-safe unit tests, Tier 1)
**Go version:** 1.24+
**Duration:** ~37s

## Summary

| Metric | Count |
|--------|-------|
| Total PASS | 1382 |
| Total FAIL | 0 |
| Total SKIP | 49 |
| Packages tested | 52 |

**Result: ALL TESTS PASSED**

## Skipped Tests (expected)

### Network-dependent (no real gateway/STUN in cloud)
- `TestPortMapperIntegration` — needs real UPnP/NAT-PMP gateway
- `TestGatewayDiscovery` — needs real network gateway
- `TestLookupPeersTimeout` — needs mDNS
- `TestGetDefaultGateway` — needs real gateway
- `TestNATPMPDiscoverNoGateway` — needs NAT-PMP
- `TestPCPClientDiscoverNoGateway` — needs PCP
- `TestGetOutboundIP` — needs outbound network
- `TestUPnPDiscoverTimeout` — needs UPnP
- `TestPortMapperMapPortNoGateway` — needs gateway

### Platform-specific
- `TestTrySpliceNonLinux` — splice test for non-Linux (we're on Linux, so skip is correct)

### Transport conformance (require real server/client pair)
- 40x `TestConformance/*` — skipped across H3, Reality, CDN, WebRTC transports
- These need full client-server setup, covered by Tier 2 sandbox tests

## What These Skips Mean

All skips are **expected and by design**. These tests are gated behind:
1. **Network availability checks** — skip when no real gateway/STUN
2. **`//go:build sandbox` tag** — only run inside Docker sandbox (Tier 2)
3. **Transport conformance** — require live server, tested in sandbox

## How to Run Full Suite (including skipped tests)

```bash
# Full suite: host tests + Docker sandbox integration
./scripts/test.sh --all

# Docker sandbox only (STUN, NAT, mDNS, hole punch, transport conformance)
./scripts/test.sh --sandbox

# Manual sandbox control
./sandbox/run.sh up       # Start Docker environment
./sandbox/run.sh test     # Shell integration tests (11 checks)
./sandbox/run.sh gotest   # Go integration tests (18 sandbox tests)
./sandbox/run.sh down     # Cleanup
```

## Cloud Environment Safety Notes

- `go test ./...` is **safe in cloud** — dangerous tests are behind `//go:build sandbox`
- `sysproxy/` and `autostart/` system-modifying tests require `-tags sandbox`
- The CLAUDE.md warning about `go test` is primarily for **local development machines**
- Cloud environment has no real system proxy or autostart to corrupt

## Test Architecture Reference

| Tier | Scope | Command | Docker? |
|------|-------|---------|---------|
| 1 | Unit tests (host-safe) | `./scripts/test.sh` | No |
| 2 | Integration (sandbox) | `./scripts/test.sh --sandbox` | Yes |
| Full | Both tiers | `./scripts/test.sh --all` | Yes |
