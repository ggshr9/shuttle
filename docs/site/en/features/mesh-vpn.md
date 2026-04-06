# Mesh VPN

Shuttle's mesh VPN assigns each client a virtual IP (VIP) and allows direct peer-to-peer traffic between clients — with the server acting as a relay fallback when a direct path cannot be established.

---

## Architecture

```
          ┌──────────┐
          │  Server  │  (relay + signalling)
          └────┬─────┘
         ┌─────┴─────┐
    ┌────┴────┐ ┌────┴────┐
    │client-a │ │client-b │
    │10.7.0.3 │ │10.7.0.2 │
    └─────────┘ └─────────┘
         ↑___direct P2P___↑
```

**Hub-and-spoke relay:** All clients connect to the server. The server can forward packets between any two clients.

**P2P upgrade:** After registration, clients attempt a direct path through NAT traversal. If successful, traffic flows peer-to-peer without passing through the server.

---

## NAT Traversal Sequence

Shuttle tries each mechanism in order, stopping at the first success:

1. **mDNS** — Discover peers on the same LAN via multicast DNS.
2. **STUN** — Discover the public endpoint for each peer.
3. **UPnP / NAT-PMP** — Request a port mapping on the gateway.
4. **Hole punching** — Simultaneous UDP packets from both sides to open NAT mappings.
5. **TURN relay** — Fall back to server-relayed traffic when all direct paths fail.

---

## Configuration

### Server

```yaml
mesh:
  enabled: true
  cidr: 10.7.0.0/24       # VIP pool; server takes the first address
  mtu: 1420
```

### Client

```yaml
mesh:
  enabled: true
  vip: 10.7.0.3            # requested virtual IP (server assigns if omitted)
  stun_servers:
    - stun:stun.l.google.com:19302
    - stun:stun1.l.google.com:19302
```

### Field Reference

**Server fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable mesh VPN |
| `cidr` | CIDR | required | VIP address pool |
| `mtu` | int | `1420` | Virtual interface MTU |

**Client fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable mesh VPN |
| `vip` | string | auto | Requested virtual IP |
| `stun_servers` | list | built-in | STUN server URIs |

---

## Virtual IP Addresses

Once connected, each client gets a VIP from the server's `cidr` pool. Peers are reachable by VIP — no DNS required. The server itself holds the first address in the pool (e.g. `10.7.0.1`).

Example with three nodes:

| Node | VIP |
|------|-----|
| server | 10.7.0.1 |
| client-a | 10.7.0.3 |
| client-b | 10.7.0.2 |

`ping 10.7.0.2` from client-a reaches client-b via the best available path (direct P2P or relay).
