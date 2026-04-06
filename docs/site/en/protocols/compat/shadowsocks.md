# Shadowsocks

## Overview

Shadowsocks is a lightweight AEAD-encrypted proxy protocol — use it when you need broad ecosystem compatibility and simple setup.

## Client Configuration

```yaml
outbounds:
  - tag: "ss-out"
    type: "shadowsocks"
    server: "server.example.com:8388"
    method: "aes-256-gcm"
    password: "your-password"
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this outbound |
| `type` | string | yes | — | Must be `shadowsocks` |
| `server` | string | yes | — | `host:port` of the Shadowsocks server |
| `method` | string | yes | — | Cipher method (see below) |
| `password` | string | yes | — | Shared secret |

**Supported methods:**

| Method | Key size | Notes |
|--------|----------|-------|
| `aes-128-gcm` | 128-bit | Fast on hardware with AES-NI |
| `aes-256-gcm` | 256-bit | Stronger; same performance with AES-NI |
| `chacha20-ietf-poly1305` | 256-bit | Best for devices without AES-NI |

## Server Configuration

```yaml
inbounds:
  - tag: "ss-in"
    type: "shadowsocks"
    listen: ":8388"
    method: "aes-256-gcm"
    password: "your-password"
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this inbound |
| `type` | string | yes | — | Must be `shadowsocks` |
| `listen` | string | yes | — | `[addr]:port` to bind |
| `method` | string | yes | — | Must match client |
| `password` | string | yes | — | Must match client |

## URI Format

```
ss://base64(method:password)@host:port#name
```

**Example:**

```
ss://YWVzLTI1Ni1nY206eW91ci1wYXNzd29yZA==@server.example.com:8388#my-server
```

The base64 payload encodes `method:password` (no padding required by most clients).

## Compatibility

| Tool | Equivalent config |
|------|------------------|
| **Clash** | `type: ss`, `cipher: aes-256-gcm` |
| **sing-box** | `type: shadowsocks`, `method: aes-256-gcm` |
| **Xray** | Shadowsocks outbound with `method` + `password` |
