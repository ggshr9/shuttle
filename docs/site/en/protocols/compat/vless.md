# VLESS

## Overview

VLESS is a lightweight proxy protocol with no built-in encryption — pair it with TLS or Reality for security in production.

## Client Configuration

```yaml
outbounds:
  - tag: "vless-out"
    type: "vless"
    server: "server.example.com:443"
    uuid: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    tls:
      enabled: true
      server_name: "server.example.com"
      # For Reality:
      reality:
        enabled: false
        public_key: ""
        short_id: ""
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this outbound |
| `type` | string | yes | — | Must be `vless` |
| `server` | string | yes | — | `host:port` of the VLESS server |
| `uuid` | string | yes | — | User UUID for authentication |
| `tls.enabled` | bool | no | `false` | Enable TLS |
| `tls.server_name` | string | no | — | SNI for TLS handshake |
| `tls.insecure` | bool | no | `false` | Skip certificate verification |
| `tls.reality.enabled` | bool | no | `false` | Use Reality instead of standard TLS |
| `tls.reality.public_key` | string | if Reality | — | Server's X25519 public key |
| `tls.reality.short_id` | string | if Reality | — | Short ID matching server |

## Server Configuration

```yaml
inbounds:
  - tag: "vless-in"
    type: "vless"
    listen: ":443"
    users:
      - uuid: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    tls:
      enabled: true
      cert_file: "/path/to/cert.pem"
      key_file: "/path/to/key.pem"
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this inbound |
| `type` | string | yes | — | Must be `vless` |
| `listen` | string | yes | — | `[addr]:port` to bind |
| `users` | list | yes | — | List of `{uuid}` objects |
| `tls.enabled` | bool | no | `false` | Enable TLS |
| `tls.cert_file` | string | if TLS | — | Path to TLS certificate |
| `tls.key_file` | string | if TLS | — | Path to TLS private key |

## URI Format

```
vless://uuid@host:port?security=tls&sni=server.example.com&type=tcp#name
```

**Query parameters:**

| Parameter | Description |
|-----------|-------------|
| `security` | `tls`, `reality`, or `none` |
| `sni` | Server name indication |
| `fp` | TLS fingerprint (e.g., `chrome`) |
| `pbk` | Reality public key |
| `sid` | Reality short ID |
| `type` | Network type (`tcp`, `ws`, `grpc`) |

## Compatibility

| Tool | Equivalent config |
|------|------------------|
| **Clash** | `type: vless` with `tls: true` |
| **sing-box** | `type: vless` outbound |
| **Xray** | VLESS outbound with `streamSettings` |
