# Shuttle

A multi-transport proxy system with adaptive congestion control, designed for high-censorship environments.

## Features

- **Multi-Transport**: HTTP/3 (QUIC), Reality (TLS + Noise IK), CDN (HTTP/2 + gRPC)
- **Adaptive Congestion Control**: BBR, Brutal, and Adaptive modes that auto-switch based on network conditions
- **Smart Routing**: Domain trie, GeoIP/GeoSite matching, process-based rules, DNS anti-pollution (DoH)
- **Mesh VPN**: Layer 3 virtual network with advanced P2P NAT traversal (STUN, UPnP, NAT-PMP, PCP, mDNS, TURN relay)
- **Traffic Obfuscation**: Packet padding, timing jitter, idle noise, cuckoo filter replay protection
- **Cross-Platform GUI**: Native desktop (Wails v2), Android, iOS -- all sharing one Svelte SPA
- **System Tray**: Connect/disconnect, show/hide window from tray icon
- **One-Click Server Deploy**: Interactive install script, Docker, systemd service

## Architecture

```
Client                              Server
+--------------------+              +--------------------+
| SOCKS5 / HTTP / TUN|              | TLS Listener       |
|         |          |              |    |               |
|     Router         |              | Auth + Cover Site  |
|    (GeoIP/DNS)     |              |    |               |
|         |          |   Internet   |  Relay to Target   |
|   Transport Layer  | -----------> |   Transport Layer  |
|  H3 / Reality / CDN|              |  H3 / Reality      |
|         |          |              |    |               |
| Congestion Control |              | Congestion Control |
|  BBR / Brutal / CC |              |  Adaptive CC       |
+--------------------+              +--------------------+
```

## Supported Platforms

| Platform | Architecture | CLI | Server | GUI |
|----------|-------------|-----|--------|-----|
| Linux | amd64, arm64, arm | ✅ | ✅ | ✅ |
| macOS | amd64, arm64 | ✅ | ✅ | ✅ |
| Windows | amd64 | ✅ | ✅ | ✅ |
| FreeBSD | amd64 | ✅ | ✅ | - |
| OpenWrt | mips, mipsle, arm | ✅ | ✅ | - |
| Android | arm64, arm | ✅ | - | ✅ |
| iOS | arm64 | ✅ | - | ✅ |

## Download

Pre-built binaries available on [GitHub Releases](https://github.com/shuttle-proxy/shuttle/releases):

- `shuttle-linux-amd64` / `shuttle-linux-arm64` - Linux CLI
- `shuttle-linux-mipsle` - OpenWrt (MIPS soft-float)
- `shuttle-darwin-arm64` - macOS CLI
- `shuttle-windows-amd64.exe` - Windows CLI
- `shuttle-gui-*` - Desktop GUI apps
- `shuttle-amd64.deb` / `shuttle-amd64.rpm` - Linux packages
- `shuttle-android.aar` / `shuttle-ios.xcframework.zip` - Mobile libraries

## Quick Start

### 1. Deploy Server (one command)

```bash
# One-click install on any Linux VPS (generates config, certs, and systemd service)
curl -fsSL https://raw.githubusercontent.com/shuttle-proxy/shuttle/main/deploy/install.sh | bash
```

The installer prints a `shuttle://` URI at the end -- copy it for the next step.

Or deploy manually:

```bash
# Docker
docker compose -f deploy/docker-compose.yml up -d

# Or build from source
go build -o shuttled ./cmd/shuttled
./shuttled run -c server.yaml
```

### 2. Connect Client

**CLI (2 steps):**

```bash
# Edit config/client.example.yaml — fill in your server addr and password, then:
shuttle run -c config.yaml
```

**GUI (easiest):**

```bash
shuttle-gui
# Paste the shuttle:// URI from the server output — done.
```

Pre-built binaries for all platforms are on the [Releases](https://github.com/shuttle-proxy/shuttle/releases) page.

## Import URI

Shuttle supports a `shuttle://` URI scheme for easy server sharing. The server
installer prints one automatically. Format: `shuttle://` + base64-encoded JSON.

**CLI import:**

```bash
# Paste the URI from server install output
shuttle import "shuttle://eyJhZGRy..."

# Writes config.yaml with sensible defaults (SOCKS5 :1080, HTTP :8080, CN direct)
shuttle run -c config.yaml
```

**Server export:**

```bash
shuttled share -c /etc/shuttle/server.yaml
# Prints: shuttle://eyJhZGRy...
```

- In the **GUI**, paste the URI into Servers → Import.
- SIP008 subscription URLs are also supported.

## Client Config Example

```yaml
server:
  addr: "example.com:443"
  password: "your-password"
  sni: "example.com"

transport:
  preferred: "auto"   # auto, h3, reality, cdn
  h3:
    enabled: true
  reality:
    enabled: true
    server_name: "www.microsoft.com"
    public_key: "<server-public-key>"
    short_id: "0123456789abcdef"

proxy:
  socks5:
    enabled: true
    listen: "127.0.0.1:1080"
  http:
    enabled: true
    listen: "127.0.0.1:8080"

congestion:
  mode: "adaptive"  # adaptive, bbr, brutal

routing:
  default: "proxy"
  rules:
    - domains: "geosite:cn"
      action: "direct"
    - geoip: "cn"
      action: "direct"
  dns:
    domestic: "223.5.5.5"
    remote:
      server: "https://1.1.1.1/dns-query"
      via: "proxy"
```

## Server Config Example

```yaml
listen: ":443"

tls:
  cert_file: "/etc/letsencrypt/live/example.com/fullchain.pem"
  key_file: "/etc/letsencrypt/live/example.com/privkey.pem"

auth:
  password: "your-password"

transport:
  h3:
    enabled: true
    path_prefix: "/h3"
  reality:
    enabled: true
    target_sni: "www.microsoft.com"
    target_addr: "www.microsoft.com:443"
    short_ids:
      - "0123456789abcdef"

cover:
  mode: "default"
```

## Building

### Prerequisites

- Go 1.24+
- Node.js 22+ (for frontend)
- CGo toolchain (for GUI: Wails + systray)

### Build All Platforms

```bash
# Build all platforms at once
./build/scripts/build-all.sh v1.0.0

# Or build individually:

# Frontend
cd gui/web && npm install && npm run build && cd ../..

# CLI (no CGo needed)
CGO_ENABLED=0 go build -ldflags="-s -w" -o shuttle ./cmd/shuttle
CGO_ENABLED=0 go build -ldflags="-s -w" -o shuttled ./cmd/shuttled

# GUI (needs CGo + Wails build tags)
# Linux: apt install gcc libayatana-appindicator3-dev libgtk-3-dev libwebkit2gtk-4.0-dev
CGO_ENABLED=1 go build -tags desktop,production -ldflags="-s -w" -o shuttle-gui ./cmd/shuttle-gui
```

### Cross-Compile for OpenWrt

```bash
# MIPS soft-float (MT7621, etc.)
CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat \
  go build -ldflags="-s -w" -o shuttle ./cmd/shuttle

# ARM (Raspberry Pi, etc.)
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 \
  go build -ldflags="-s -w" -o shuttle ./cmd/shuttle

# Optional: compress with UPX (~3MB)
upx --best shuttle
```

### Linux Packages (deb/rpm)

```bash
# Requires nFPM: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
VERSION=v1.0.0 GOARCH=amd64 nfpm pkg --packager deb -f build/package/nfpm.yaml
VERSION=v1.0.0 GOARCH=amd64 nfpm pkg --packager rpm -f build/package/nfpm.yaml
```

### Run Tests

```bash
# Host-safe unit tests (fast, no network impact)
./scripts/test.sh

# Full suite including Docker sandbox integration tests
./scripts/test.sh --all
```

## OpenWrt Installation

### Quick Deploy (Pre-built Binary)

```bash
# Download and install
scp shuttle-linux-mipsle root@192.168.1.1:/usr/bin/shuttle
ssh root@192.168.1.1 "chmod +x /usr/bin/shuttle"

# Create config
ssh root@192.168.1.1 "mkdir -p /etc/shuttle"
scp config/client.example.yaml root@192.168.1.1:/etc/shuttle/client.yaml

# Create init script
ssh root@192.168.1.1 'cat > /etc/init.d/shuttle << "EOF"
#!/bin/sh /etc/rc.common
START=99
USE_PROCD=1
start_service() {
    procd_open_instance
    procd_set_param command /usr/bin/shuttle run -c /etc/shuttle/client.yaml
    procd_set_param respawn
    procd_close_instance
}
EOF
chmod +x /etc/init.d/shuttle'

# Enable and start
ssh root@192.168.1.1 "/etc/init.d/shuttle enable && /etc/init.d/shuttle start"
```

### OpenWrt SDK (ipk Package)

See `build/package/openwrt/` for package definition and `build/README.md` for detailed instructions.

## GUI

The desktop app uses Wails v2 to render a Svelte SPA in a native WebView window.

**Pages:**

| Page | Description |
|------|-------------|
| Dashboard | Connect/disconnect toggle, live speed chart, transport status, traffic stats |
| Servers | Server list management, speed test, import/export (base64/JSON/URI) |
| Subscriptions | Subscription management (SIP008), auto-refresh |
| Routing | Visual rule editor, rule templates, DNS settings |
| Logs | Real-time log viewer with expandable details (target, rule, protocol, traffic) |
| Settings | Proxy modes, transport selection, congestion control, i18n (中/EN), backup/restore |

**Additional Features:**
- **Auto-Update**: Check GitHub Releases with changelog display
- **Keyboard Shortcuts**: Cmd/Ctrl+K for quick connect toggle
- **Browser Notifications**: Connection status alerts
- **Traffic Charts**: Real-time Canvas-based bandwidth curves (no external deps)
- **Configuration Backup/Restore**: Full backup including servers, subscriptions, rules

**REST API** (also usable standalone at `http://127.0.0.1:{port}`):

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/status` | GET | Engine state, speed, connections |
| `/api/connect` | POST | Start engine |
| `/api/disconnect` | POST | Stop engine |
| `/api/config` | GET/PUT | Read/write full config |
| `/api/config/servers` | GET/PUT/POST/DELETE | Server management |
| `/api/routing/rules` | GET/PUT | Routing rules |
| `/api/logs` | WebSocket | Real-time log stream |
| `/api/speed` | WebSocket | Real-time speed stream |

## Mesh VPN

Shuttle includes an optional Mesh VPN that creates a virtual Layer 3 network between clients. All clients get a virtual IP (e.g., `10.7.0.x`) and can communicate directly with each other.

### Features

- **Hub-and-Spoke + P2P**: Server relays traffic by default, with automatic P2P upgrade when possible
- **Zero-Config NAT Traversal**: Automatically tries UPnP, NAT-PMP, PCP, and STUN for best connectivity
- **mDNS Local Discovery**: Automatically discover other Shuttle clients on the same LAN
- **TURN Relay Fallback**: RFC 5766/8656 compliant relay when direct P2P fails
- **Port Spoofing**: Use privileged ports (53, 443, 500) to bypass restrictive firewalls
- **Connection Quality Monitoring**: Auto-switches between P2P and relay based on packet loss/RTT
- **Path Caching**: Remembers successful connection methods for faster reconnection
- **Parallel Protocol Discovery**: Tries UPnP/NAT-PMP/PCP simultaneously, uses first success

### Mesh Config Example

```yaml
mesh:
  enabled: true
  p2p_enabled: true    # Enable P2P NAT traversal
  p2p:
    # All settings below are optional (auto-configured by default)
    stun_servers:
      - "stun.l.google.com:19302"
    hole_punch_timeout: "10s"
    fallback_threshold: 0.3    # Switch to relay if >30% packet loss

    # Port spoofing (optional, for restrictive networks)
    spoof_mode: "dns"          # "dns" (53), "https" (443), "ike" (500)

    # UPnP/NAT-PMP (auto-enabled, set to disable)
    disable_upnp: false
    preferred_port: 0          # 0 = same as local port
```

### NAT Traversal Strategy

```
┌─────────────────────────────────────────────────────────────────┐
│                     Automatic NAT Traversal                      │
├─────────────────────────────────────────────────────────────────┤
│  1. mDNS Discovery     →  Find peers on local network            │
│  2. UPnP / NAT-PMP / PCP →  Creates port mapping (parallel)      │
│  3. STUN               →  Discovers external IP:port             │
│  4. ICE Candidates     →  Gathers all possible endpoints         │
│  5. Hole Punching      →  Attempts direct UDP connection         │
│  6. TURN Relay         →  RFC 5766 relay if direct fails         │
│  7. Server Relay       →  Final fallback via Shuttle server      │
└─────────────────────────────────────────────────────────────────┘
```

### Use Cases

- **Remote Desktop**: VNC/RDP between mesh clients
- **File Sharing**: SMB/NFS/SSH between clients
- **Gaming**: LAN games over the internet
- **Development**: Access services on other clients (databases, APIs)

## Project Structure

```
cmd/shuttle/         Client CLI
cmd/shuttle-gui/     Desktop GUI (Wails v2 + systray)
cmd/shuttled/        Server CLI
config/              YAML/JSON config parsing
congestion/          BBR, Brutal, Adaptive CC + QUIC adapter
crypto/              Noise IK, ChaCha20/AES-GCM, cuckoo replay filter
engine/              Core lifecycle (Start/Stop/Reload/Status/Subscribe)
gui/api/             REST + WebSocket API
gui/tray/            System tray (desktop only)
gui/web/             Svelte 5 SPA frontend (i18n, speed charts, backup)
mesh/                Mesh VPN virtual network layer
mesh/p2p/            NAT traversal (STUN, UPnP, NAT-PMP, PCP, mDNS, TURN)
mesh/signal/         P2P signaling protocol
transport/h3/        HTTP/3 over QUIC (Chrome fingerprint, HMAC auth)
transport/reality/   TLS + Noise IK + yamux (SNI impersonation)
transport/cdn/       HTTP/2 + gRPC through CDN
transport/selector/  Auto transport selection + migration
router/              Domain trie, GeoIP/GeoSite, DoH
proxy/               SOCKS5, HTTP CONNECT, TUN
server/              Multi-protocol listener, cover site
obfs/                Packet padding, timing jitter, idle noise
plugin/              Logger, metrics, domain filter
stats/               Persistent traffic statistics storage
subscription/        SIP008 subscription management
speedtest/           TCP+TLS speed testing utilities
sysproxy/            System proxy auto-config (macOS/Linux/Windows)
update/              Auto-update checker (GitHub Releases)
internal/            Buffer pools, rate limiter, sysopt, procnet
quicfork/            Local fork of quic-go with CC hook
mobile/              gomobile bindings (Android/iOS)
build/               Build scripts & packaging (deb/rpm/openwrt)
deploy/              install.sh, Dockerfile, systemd
test/                E2E, transport, mesh, router, benchmark tests
```

## License

MIT
