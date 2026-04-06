# Trojan

## Overview

Trojan disguises proxy traffic as HTTPS by using a password-based SHA224 authentication over TLS — ideal when you want traffic that blends in with normal web traffic.

## Client Configuration

```yaml
outbounds:
  - tag: "trojan-out"
    type: "trojan"
    server: "server.example.com:443"
    password: "your-password"
    tls:
      server_name: "server.example.com"
      insecure: false
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this outbound |
| `type` | string | yes | — | Must be `trojan` |
| `server` | string | yes | — | `host:port` of the Trojan server |
| `password` | string | yes | — | Shared secret (sent as SHA224 hash) |
| `tls.server_name` | string | no | server host | SNI for TLS handshake |
| `tls.insecure` | bool | no | `false` | Skip certificate verification |
| `tls.ca_file` | string | no | — | Custom CA certificate path |

## Server Configuration

```yaml
inbounds:
  - tag: "trojan-in"
    type: "trojan"
    listen: ":443"
    passwords:
      - "your-password"
    tls:
      cert_file: "/path/to/cert.pem"
      key_file: "/path/to/key.pem"
    fallback:
      host: "127.0.0.1"
      port: 80
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this inbound |
| `type` | string | yes | — | Must be `trojan` |
| `listen` | string | yes | — | `[addr]:port` to bind |
| `passwords` | list | yes | — | List of allowed passwords |
| `tls.cert_file` | string | yes | — | Path to TLS certificate |
| `tls.key_file` | string | yes | — | Path to TLS private key |
| `fallback.host` | string | no | — | Host to forward non-Trojan connections to |
| `fallback.port` | int | no | — | Port to forward non-Trojan connections to |

The `fallback` option lets the server forward unrecognized connections to a real web server, making the service indistinguishable from HTTPS from outside.

## URI Format

```
trojan://password@host:port?sni=server.example.com#name
```

**Query parameters:**

| Parameter | Description |
|-----------|-------------|
| `sni` | Server name indication |
| `allowInsecure` | `1` to skip certificate verification |
| `alpn` | ALPN protocols (e.g., `h2,http/1.1`) |

## Compatibility

| Tool | Equivalent config |
|------|------------------|
| **Clash** | `type: trojan` with `sni` |
| **sing-box** | `type: trojan` outbound |
| **Xray** | Trojan outbound with `streamSettings` |
