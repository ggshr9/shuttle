# CDN (Shuttle Native)

## Overview

CDN is Shuttle's HTTP/2 or gRPC transport designed to pass through CDN networks (Cloudflare, Fastly, etc.) — use it when direct connections are blocked but CDN-fronted HTTPS is allowed.

## Client Configuration

```yaml
outbounds:
  - tag: "cdn-out"
    type: "cdn"
    server: "server.example.com:443"
    auth_key: "your-auth-key"
    cdn:
      mode: "h2"                          # h2 or grpc
      domain: "server.example.com"
      path: "/stream"
      front_domain: "allowed.cdn.com"     # optional domain fronting host
    tls:
      server_name: "allowed.cdn.com"
      insecure_skip_verify: false
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this outbound |
| `type` | string | yes | — | Must be `cdn` |
| `server` | string | yes | — | IP or CDN edge address with port |
| `auth_key` | string | yes | — | HMAC authentication key |
| `cdn.mode` | string | no | `h2` | Transport sub-mode: `h2` or `grpc` |
| `cdn.domain` | string | yes | — | The `Host` header / gRPC authority sent to CDN |
| `cdn.path` | string | no | `/` | HTTP path (h2) or gRPC service path |
| `cdn.front_domain` | string | no | — | Domain-fronting override for SNI vs Host |
| `tls.server_name` | string | no | `cdn.domain` | SNI for TLS (set to `front_domain` when fronting) |
| `tls.insecure_skip_verify` | bool | no | `false` | Skip certificate verification |

**Mode differences:**

| Mode | Description |
|------|-------------|
| `h2` | HTTP/2 streaming over TLS; compatible with standard CDN behavior |
| `grpc` | gRPC streaming; some CDNs allow gRPC passthrough for WebRTC/API traffic |

## Server Configuration

```yaml
inbounds:
  - tag: "cdn-in"
    type: "cdn"
    listen: ":443"
    auth_key: "your-auth-key"
    cdn:
      mode: "h2"
      path: "/stream"
    tls:
      cert_file: "/path/to/cert.pem"
      key_file: "/path/to/key.pem"
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this inbound |
| `type` | string | yes | — | Must be `cdn` |
| `listen` | string | yes | — | `[addr]:port` to bind |
| `auth_key` | string | yes | — | Must match client |
| `cdn.mode` | string | no | `h2` | Must match client |
| `cdn.path` | string | no | `/` | Must match client |
| `tls.cert_file` | string | yes | — | Path to TLS certificate |
| `tls.key_file` | string | yes | — | Path to TLS private key |

## Domain Fronting Setup

Domain fronting allows traffic to appear destined for a popular CDN domain while actually reaching your server:

1. Put your server behind a CDN (e.g., Cloudflare) with a proxied A record.
2. Set `cdn.front_domain` to a popular domain on the same CDN (e.g., `www.cloudflare.com`).
3. Set `tls.server_name` to `front_domain` — the SNI goes to the CDN.
4. Set `cdn.domain` to your real domain — the `Host` header routes inside the CDN.

```yaml
cdn:
  mode: "h2"
  domain: "myserver.example.com"
  front_domain: "www.cloudflare.com"
tls:
  server_name: "www.cloudflare.com"
```

> **Note:** Domain fronting policies vary by CDN. Check your CDN provider's terms of service.

## URI Format

```
shuttle://cdn?server=IP:443&key=KEY&domain=myserver.example.com&mode=h2#name
```

## Compatibility

CDN is Shuttle-native. Similar concepts exist in other tools as WebSocket + TLS transports, but the wire format is not compatible.
