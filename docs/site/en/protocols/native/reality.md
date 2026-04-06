# Reality (Shuttle Native)

## Overview

Reality is Shuttle's TLS + Noise IK transport with SNI impersonation and optional post-quantum encryption — designed to be indistinguishable from TLS traffic to a real website.

## Client Configuration

```yaml
outbounds:
  - tag: "reality-out"
    type: "reality"
    server: "server.example.com:443"
    auth_key: "your-auth-key"
    tls:
      server_name: "www.cloudflare.com"   # impersonated domain
    reality:
      enabled: true
      public_key: "SERVER_X25519_PUBLIC_KEY"
      short_id: "abcdef01"
      post_quantum: false
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this outbound |
| `type` | string | yes | — | Must be `reality` |
| `server` | string | yes | — | `host:port` of the Reality server |
| `auth_key` | string | yes | — | Noise IK authentication key |
| `tls.server_name` | string | yes | — | SNI of the impersonated real website |
| `reality.enabled` | bool | yes | — | Must be `true` |
| `reality.public_key` | string | yes | — | Server's X25519 public key (base64) |
| `reality.short_id` | string | yes | — | Short ID (hex, 2–16 chars) matching server |
| `reality.post_quantum` | bool | no | `false` | Enable ML-KEM post-quantum key exchange |

## Server Configuration

```yaml
inbounds:
  - tag: "reality-in"
    type: "reality"
    listen: ":443"
    auth_key: "your-auth-key"
    reality:
      enabled: true
      private_key: "SERVER_X25519_PRIVATE_KEY"
      short_ids:
        - "abcdef01"
        - "12345678"
      dest: "www.cloudflare.com:443"     # actual TLS destination for non-Reality clients
      server_names:
        - "www.cloudflare.com"
      post_quantum: false
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this inbound |
| `type` | string | yes | — | Must be `reality` |
| `listen` | string | yes | — | `[addr]:port` to bind |
| `auth_key` | string | yes | — | Must match client |
| `reality.private_key` | string | yes | — | Server's X25519 private key (base64) |
| `reality.short_ids` | list | yes | — | Accepted short IDs from clients |
| `reality.dest` | string | yes | — | Pass-through destination for non-Reality TLS |
| `reality.server_names` | list | yes | — | Acceptable SNI values |
| `reality.post_quantum` | bool | no | `false` | Enable post-quantum key exchange |

## Generating Keys

```bash
# Generate an X25519 key pair for Reality
shuttle keygen reality
# Output:
#   private_key: <base64>
#   public_key:  <base64>
```

## How It Works

1. The client opens a TLS connection with the SNI of a real, popular website.
2. If the `short_id` is not recognized by the server, the server transparently proxies the connection to the real destination — the server behaves identically to a CDN edge node from an outside observer's perspective.
3. Recognized clients proceed with Noise IK handshake inside the TLS session, establishing an encrypted + authenticated tunnel.
4. `post_quantum: true` adds an ML-KEM encapsulation step for forward secrecy against quantum adversaries.

## URI Format

```
shuttle://reality?server=server.example.com:443&pk=PUBLIC_KEY&sid=abcdef01&sni=www.cloudflare.com#name
```

## Compatibility

Reality is Shuttle-native. For VLESS+Reality (Xray-compatible), configure a separate VLESS inbound. Shuttle's Reality is not wire-compatible with Xray's REALITY extension.
