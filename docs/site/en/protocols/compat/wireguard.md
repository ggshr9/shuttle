# WireGuard

## Overview

WireGuard is a modern VPN protocol with minimal attack surface and kernel-speed performance — use it as a client-side outbound to tunnel traffic through a WireGuard VPN endpoint.

> **Client only.** Shuttle uses WireGuard as an outbound transport. There is no WireGuard inbound; use the official `wg-quick` or a WireGuard server implementation for the server side.

## Client Configuration

```yaml
outbounds:
  - tag: "wg-out"
    type: "wireguard"
    private_key: "CLIENT_PRIVATE_KEY_BASE64"
    addresses:
      - "10.0.0.2/32"
      - "fd00::2/128"
    dns:
      - "1.1.1.1"
      - "8.8.8.8"
    mtu: 1420
    peers:
      - public_key: "SERVER_PUBLIC_KEY_BASE64"
        endpoint: "server.example.com:51820"
        allowed_ips:
          - "0.0.0.0/0"
          - "::/0"
        keepalive: 25
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tag` | string | yes | — | Unique name for this outbound |
| `type` | string | yes | — | Must be `wireguard` |
| `private_key` | string | yes | — | Client private key (base64) |
| `addresses` | list | yes | — | Client tunnel IP addresses with prefix length |
| `dns` | list | no | system | DNS servers to use inside the tunnel |
| `mtu` | int | no | `1420` | Tunnel MTU |
| `peers` | list | yes | — | List of peer configs (see below) |

**Peer fields:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `public_key` | string | yes | — | Peer's public key (base64) |
| `pre_shared_key` | string | no | — | Optional pre-shared key for extra security |
| `endpoint` | string | yes | — | Peer's `host:port` |
| `allowed_ips` | list | yes | — | CIDRs to route through this peer |
| `keepalive` | int | no | `0` | Persistent keepalive interval in seconds |

## Generating Keys

```bash
# Generate a key pair
wg genkey | tee privatekey | wg pubkey > publickey

cat privatekey   # paste as private_key
cat publickey    # share with the server operator
```

## URI Format

WireGuard does not have a standard subscription URI format. Import configurations using the standard `wg-quick` `.conf` file format or enter fields manually.

## Compatibility

| Tool | Equivalent config |
|------|------------------|
| **Clash** | `type: wireguard` (Meta only) |
| **sing-box** | `type: wireguard` outbound |
| **Xray** | Not supported |
