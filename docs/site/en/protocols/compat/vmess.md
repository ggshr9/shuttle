# VMess

## Overview

VMess is a V2Ray-originated protocol with built-in AEAD encryption — use it when connecting to existing VMess servers; for new deployments prefer VLESS.

## Client Configuration

```yaml
outbounds:
  - tag: "vmess-out"
    type: "vmess"
    server: "server.example.com:443"
    uuid: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    cipher: "aes-128-gcm"
    tls:
      enabled: true
      server_name: "server.example.com"
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this outbound |
| `type` | string | yes | — | Must be `vmess` |
| `server` | string | yes | — | `host:port` of the VMess server |
| `uuid` | string | yes | — | User UUID |
| `cipher` | string | no | `aes-128-gcm` | Encryption cipher (see below) |
| `alter_id` | int | no | `0` | Must be `0` (AEAD mode only) |
| `tls.enabled` | bool | no | `false` | Enable TLS |
| `tls.server_name` | string | no | server host | SNI for TLS handshake |
| `tls.insecure` | bool | no | `false` | Skip certificate verification |

**Supported ciphers:**

| Cipher | Notes |
|--------|-------|
| `aes-128-gcm` | Default; hardware-accelerated on most platforms |
| `chacha20-poly1305` | Best for devices without AES-NI |
| `none` | No encryption (use only over TLS) |

> **Note:** Shuttle only supports AEAD mode (`alter_id: 0`). Legacy MD5-based VMess is not supported.

## Server Configuration

```yaml
inbounds:
  - tag: "vmess-in"
    type: "vmess"
    listen: ":443"
    users:
      - uuid: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
        cipher: "aes-128-gcm"
    tls:
      enabled: true
      cert_file: "/path/to/cert.pem"
      key_file: "/path/to/key.pem"
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this inbound |
| `type` | string | yes | — | Must be `vmess` |
| `listen` | string | yes | — | `[addr]:port` to bind |
| `users` | list | yes | — | List of `{uuid, cipher}` objects |
| `tls.enabled` | bool | no | `false` | Enable TLS |
| `tls.cert_file` | string | if TLS | — | Path to TLS certificate |
| `tls.key_file` | string | if TLS | — | Path to TLS private key |

## URI Format

VMess uses a base64-encoded JSON link:

```
vmess://base64(json)
```

The JSON payload:

```json
{
  "v": "2",
  "ps": "name",
  "add": "server.example.com",
  "port": "443",
  "id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "aid": "0",
  "scy": "aes-128-gcm",
  "net": "tcp",
  "tls": "tls",
  "sni": "server.example.com"
}
```

## Compatibility

| Tool | Equivalent config |
|------|------------------|
| **Clash** | `type: vmess`, `cipher: aes-128-gcm`, `alterId: 0` |
| **sing-box** | `type: vmess` outbound with `security` field |
| **Xray** | VMess outbound with AEAD enabled |
