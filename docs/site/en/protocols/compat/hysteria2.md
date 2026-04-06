# Hysteria2

## Overview

Hysteria2 is a QUIC-based protocol using Brutal congestion control — best for high-loss or bandwidth-limited networks where TCP-based protocols struggle.

## Client Configuration

```yaml
outbounds:
  - tag: "hy2-out"
    type: "hysteria2"
    server: "server.example.com:443"
    password: "your-password"
    tls:
      server_name: "server.example.com"
      insecure: false
    bandwidth:
      up: "50 mbps"
      down: "200 mbps"
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this outbound |
| `type` | string | yes | — | Must be `hysteria2` |
| `server` | string | yes | — | `host:port` of the Hysteria2 server |
| `password` | string | yes | — | Authentication password |
| `tls.server_name` | string | no | server host | SNI for TLS handshake |
| `tls.insecure` | bool | no | `false` | Skip certificate verification |
| `tls.ca_file` | string | no | — | Custom CA certificate path |
| `bandwidth.up` | string | no | — | Upload bandwidth hint (e.g., `50 mbps`) |
| `bandwidth.down` | string | no | — | Download bandwidth hint (e.g., `200 mbps`) |

Providing accurate bandwidth hints helps Brutal CC achieve optimal throughput. Units: `bps`, `kbps`, `mbps`, `gbps`.

## Server Configuration

```yaml
inbounds:
  - tag: "hy2-in"
    type: "hysteria2"
    listen: ":443"
    passwords:
      - "your-password"
    tls:
      cert_file: "/path/to/cert.pem"
      key_file: "/path/to/key.pem"
    bandwidth:
      up: "1 gbps"
      down: "1 gbps"
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this inbound |
| `type` | string | yes | — | Must be `hysteria2` |
| `listen` | string | yes | — | `[addr]:port` to bind |
| `passwords` | list | yes | — | List of allowed passwords |
| `tls.cert_file` | string | yes | — | Path to TLS certificate |
| `tls.key_file` | string | yes | — | Path to TLS private key |
| `bandwidth.up` | string | no | — | Server upload capacity |
| `bandwidth.down` | string | no | — | Server download capacity |

## URI Format

```
hysteria2://password@host:port?sni=server.example.com#name
```

**Query parameters:**

| Parameter | Description |
|-----------|-------------|
| `sni` | Server name indication |
| `insecure` | `1` to skip certificate verification |
| `up` | Upload bandwidth hint |
| `down` | Download bandwidth hint |

## Compatibility

| Tool | Equivalent config |
|------|------------------|
| **Clash** | `type: hysteria2` (Meta/Premium) |
| **sing-box** | `type: hysteria2` outbound |
| **Xray** | Not supported natively |
