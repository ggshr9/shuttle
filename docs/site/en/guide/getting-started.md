# Getting Started

## Install

### Download Binary

Pre-built binaries are available on the [GitHub Releases](https://github.com/your-org/shuttle/releases) page. Download the archive for your platform and extract it to a directory in your `$PATH`.

| Platform | File |
|----------|------|
| Linux x86_64 | `shuttle-linux-amd64.tar.gz` |
| Linux arm64 | `shuttle-linux-arm64.tar.gz` |
| macOS arm64 | `shuttle-darwin-arm64.tar.gz` |
| Windows x86_64 | `shuttle-windows-amd64.zip` |

Each archive contains two binaries: `shuttle` (client) and `shuttled` (server).

### Build from Source

Go 1.24 or later is required.

```bash
git clone https://github.com/your-org/shuttle.git
cd shuttle

# Client
CGO_ENABLED=0 go build -o shuttle ./cmd/shuttle

# Server
CGO_ENABLED=0 go build -o shuttled ./cmd/shuttled
```

No CGo is required for the CLI binaries. Cross-compilation works out of the box with `GOOS`/`GOARCH`.

---

## Quick Start — Client

Create a `config.yaml` file:

```yaml
# config.yaml — minimal client config

# Outbound server
outbounds:
  - name: my-server
    type: auto          # auto-negotiate H3 / Reality / CDN
    server: your.server.example.com
    port: 443
    password: your-password-here

# Local proxy listeners
inbounds:
  - type: socks5
    listen: 127.0.0.1
    port: 1080
  - type: http
    listen: 127.0.0.1
    port: 8080

# Routing: send everything through the proxy
routing:
  default: my-server
```

Start the client:

```bash
./shuttle -c config.yaml
```

Configure your application to use `127.0.0.1:1080` as its SOCKS5 proxy (or `127.0.0.1:8080` for HTTP proxy).

---

## Quick Start — Server

Create a `server.yaml` file on your server:

```yaml
# server.yaml — minimal server config

listen: 0.0.0.0
port: 443
password: your-password-here

tls:
  cert: /etc/shuttle/cert.pem
  key:  /etc/shuttle/key.pem

transport:
  preferred: auto   # enables H3, Reality, and CDN listeners
```

Start the server:

```bash
./shuttled -c server.yaml
```

For a self-signed certificate during testing:

```bash
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
  -keyout key.pem -out cert.pem -days 365 -nodes \
  -subj "/CN=your.server.example.com"
```

---

## Using with Existing Servers

Shuttle can connect to servers running Shadowsocks, VLESS, or Trojan. Add the relevant outbound to your `config.yaml`:

### Shadowsocks

```yaml
outbounds:
  - name: ss-server
    type: shadowsocks
    server: ss.example.com
    port: 8388
    cipher: aes-256-gcm
    password: your-ss-password
```

### VLESS

```yaml
outbounds:
  - name: vless-server
    type: vless
    server: vless.example.com
    port: 443
    uuid: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
    tls:
      enabled: true
      server_name: vless.example.com
    transport:
      type: ws
      path: /path
```

### Trojan

```yaml
outbounds:
  - name: trojan-server
    type: trojan
    server: trojan.example.com
    port: 443
    password: your-trojan-password
    tls:
      enabled: true
      server_name: trojan.example.com
```

### Hysteria2

```yaml
outbounds:
  - name: hy2-server
    type: hysteria2
    server: hy2.example.com
    port: 443
    password: your-hy2-password
    tls:
      server_name: hy2.example.com
```

Point your routing default at the outbound name you want to use.

---

## GUI

**shuttle-gui** is a desktop application with a system tray icon, Simple Mode for quick setup, and an Advanced Mode with full configuration access including Mesh VPN, congestion control settings, and live traffic charts.

Download `shuttle-gui` from the [GitHub Releases](https://github.com/your-org/shuttle/releases) page. On first launch it will guide you through adding a server.

The GUI manages its own config file and starts/stops the engine internally — no separate `shuttle` process is needed.

---

## Next Steps

- [Configuration Reference](/en/guide/configuration) — full list of all options
- [Proxy Groups](/en/features/proxy-groups) — url-test, fallback, load-balance, quality groups
- [Mesh VPN](/en/features/mesh-vpn) — P2P VPN between clients
- [Congestion Control](/en/features/congestion-control) — BBR, Brutal, Adaptive
