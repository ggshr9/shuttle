# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Shuttle is a multi-transport proxy system written in Go for high-censorship environments. It features three transport protocols (H3/QUIC, Reality/TLS+Noise, CDN/HTTP2), adaptive congestion control, intelligent routing, and an optional mesh VPN.

## Build Commands

```bash
# Build CLI binaries (no CGo required)
CGO_ENABLED=0 go build -o shuttle ./cmd/shuttle
CGO_ENABLED=0 go build -o shuttled ./cmd/shuttled

# Build GUI (requires CGo, Wails, and frontend assets)
cd gui/web && npm install && npm run build
CGO_ENABLED=1 go build -tags desktop,production -o shuttle-gui ./cmd/shuttle-gui
```

## Running Tests

**IMPORTANT: NEVER run `go test ./...` directly — always use `scripts/test.sh`.**
Direct `go test` is host-safe but misses network integration tests. The unified script
handles both tiers automatically and manages Docker lifecycle.

```bash
# Host-safe unit tests only (fast, no network impact)
./scripts/test.sh

# Host tests + Docker sandbox integration tests (full suite)
./scripts/test.sh --all

# Docker sandbox tests only (STUN, NAT, mDNS, hole punch)
./scripts/test.sh --sandbox

# Test a specific package
./scripts/test.sh --pkg ./mesh/p2p/

# Run specific test(s) by regex
./scripts/test.sh --run TestRouter
```

## Architecture

### Entry Points
- `cmd/shuttle/` - Client CLI
- `cmd/shuttled/` - Server CLI
- `cmd/shuttle-gui/` - Desktop GUI (Wails + Svelte)

### Core Components

**Engine** (`engine/engine.go`): Central stateful component managing proxy lifecycle with state machine (Stopped → Starting → Running → Stopping), event subscription, and hot-reload support.

**Transports** (`transport/`):
- `h3/` - HTTP/3 over QUIC with Chrome fingerprint and HMAC auth
- `reality/` - TLS + Noise IK encryption + yamux multiplexing with SNI impersonation
- `cdn/` - HTTP/2 + gRPC for CDN passthrough
- `webrtc/` - WebRTC DataChannel with yamux multiplexing
- `selector/` - Auto-negotiation between transports

**Congestion Control** (`congestion/`):
- BBR (bandwidth-based)
- Brutal (constant rate for active interference)
- Adaptive (auto-switches based on packet loss & RTT)

**Router** (`router/`): Domain trie matching, GeoIP lookups, DNS-over-HTTPS with caching and prefetch.

**Proxy Listeners** (`proxy/`): SOCKS5, HTTP CONNECT, TUN device with per-app routing.

**Mesh VPN** (`mesh/`): Hub-and-spoke relay, P2P NAT traversal via STUN/hole-punching.

### Data Flow
1. Client app connects via SOCKS5/HTTP/TUN
2. Router determines proxy vs direct based on GeoIP/GeoSite rules
3. Selected transport (H3/Reality/CDN) encrypts and sends to server
4. Server decrypts, relays to destination, returns response

### Key Interfaces
- `transport.Connection` / `transport.Stream` - Multiplexed connection abstraction
- `transport.ClientTransport` / `transport.ServerTransport` - Transport protocol interface
- `congestion.CongestionController` - Congestion control algorithm interface

### Local QUIC Fork
`quicfork/` contains a local fork of quic-go with hooks for custom congestion control. This enables BBR/Brutal/Adaptive CC integration.

### GUI Architecture
- Frontend: Svelte 5 SPA in `gui/web/`, built with Vite, embedded in binary
- Backend: REST API + WebSocket in `gui/api/`
- System tray: `gui/tray/` using fyne.io/systray
- Communication: Random port REST API between Wails WebView and Go backend

## Configuration

Client and server configs are YAML. Key structures in `config/config.go`:
- `DefaultClientConfig()` and `DefaultServerConfig()` provide sensible defaults
- Config hot-reload supported via `Engine.Reload()`

## Testing

### Two-Tier Test Architecture

**Tier 1 — Host-safe unit tests** (`go test ./...`):
- Run directly on the host machine, zero network impact
- All external network operations (STUN, UPnP, mDNS, gateway discovery) are `t.Skip()`-ed
- Tests in `test/`, `mesh/p2p/*_test.go`, `transport/`, etc.

**Tier 2 — Docker sandbox integration tests** (`//go:build sandbox`):
- Run inside Docker containers with isolated virtual networks
- Real STUN queries, NAT detection, hole punching, mDNS — all within Docker
- Test file: `mesh/p2p/sandbox_test.go`
- Docker topology: STUN server + NAT router + gotest container on 3 subnets

### Running Tests
```bash
# Recommended: unified test runner
./scripts/test.sh          # Host tests only (fast)
./scripts/test.sh --all    # Host + Docker sandbox (full)
./scripts/test.sh --sandbox # Docker sandbox only
```

### Sandbox Environment
Docker-based isolated test environment in `sandbox/`:
```bash
./sandbox/run.sh              # Shell integration tests (server/client/proxy)
./sandbox/run.sh gotest       # Go integration tests (STUN/NAT/P2P)
./sandbox/run.sh build        # Build binaries and images
./sandbox/run.sh up           # Start environment
./sandbox/run.sh down         # Stop environment
```

Network topology:
- `net-server` (10.100.0.0/24): server, STUN, httpbin, router
- `net-a` (10.100.1.0/24): client-a, router, gotest
- `net-b` (10.100.2.0/24): client-b, router, gotest
- `gotest` container connects to all 3 networks for cross-NAT testing

## Platform Build Requirements

- Go 1.24+
- Node.js 22+ (for GUI frontend)
- Linux GUI: `libayatana-appindicator3-dev`, `libgtk-3-dev`, `libwebkit2gtk-4.0-dev`
- CGo required only for GUI builds
