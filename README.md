# Shuttle

A multi-transport proxy system with adaptive congestion control, designed for high-censorship environments.

## Features

- **Multi-Transport**: HTTP/3 (QUIC), Reality (TLS + Noise IK), CDN (HTTP/2 + gRPC)
- **Adaptive Congestion Control**: BBR, Brutal, and Adaptive modes that auto-switch based on network conditions
- **Smart Routing**: Domain trie, GeoIP/GeoSite matching, process-based rules, DNS anti-pollution (DoH)
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

## Quick Start

### Client (CLI)

```bash
# Download from releases or build
go build -o shuttle ./cmd/shuttle

# Run with config
./shuttle run -c config.yaml
```

### Client (GUI)

```bash
# Requires CGo (Wails + systray)
go build -o shuttle-gui ./cmd/shuttle-gui

# Opens native window with web UI
./shuttle-gui -c config.yaml
```

### Server

```bash
# One-click install on Linux VPS
curl -fsSL https://raw.githubusercontent.com/ggshr9/shuttle/main/deploy/install.sh | bash

# Or with Docker
docker compose -f deploy/docker-compose.yml up -d

# Or build manually
go build -o shuttled ./cmd/shuttled
./shuttled run -c server.yaml
```

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

### Build All

```bash
# Frontend
cd gui/web && npm install && npm run build && cd ../..

# CLI (no CGo needed)
CGO_ENABLED=0 go build -o shuttle ./cmd/shuttle
CGO_ENABLED=0 go build -o shuttled ./cmd/shuttled

# GUI (needs CGo)
# Linux: apt install gcc libayatana-appindicator3-dev libgtk-3-dev libwebkit2gtk-4.0-dev
CGO_ENABLED=1 go build -o shuttle-gui ./cmd/shuttle-gui
```

### Run Tests

```bash
go test -count=1 -v ./...
```

## GUI

The desktop app uses Wails v2 to render a Svelte SPA in a native WebView window.

**Pages:**

| Page | Description |
|------|-------------|
| Dashboard | Connect/disconnect toggle, live speed, transport status |
| Servers | Active server config, saved server list (add/remove/switch) |
| Routing | Visual rule editor, default action, DNS settings |
| Logs | Real-time log viewer with auto-scroll |
| Settings | Proxy modes, transport selection, congestion control, log level |

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
gui/web/             Svelte 5 SPA frontend
transport/h3/        HTTP/3 over QUIC (Chrome fingerprint, HMAC auth)
transport/reality/   TLS + Noise IK + yamux (SNI impersonation)
transport/cdn/       HTTP/2 + gRPC through CDN
transport/selector/  Auto transport selection + migration
router/              Domain trie, GeoIP/GeoSite, DoH
proxy/               SOCKS5, HTTP CONNECT, TUN
server/              Multi-protocol listener, cover site
obfs/                Packet padding, timing jitter, idle noise
plugin/              Logger, metrics, domain filter
internal/            Buffer pools, rate limiter, sysopt
quicfork/            Local fork of quic-go with CC hook
mobile/              gomobile bindings (Android/iOS)
deploy/              install.sh, Dockerfile, systemd
```

## License

MIT
