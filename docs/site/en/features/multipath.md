# Multipath

Multipath lets Shuttle use several transports simultaneously for a single logical connection, distributing traffic across them according to a scheduling policy.

---

## What It Does

Without multipath, each connection uses exactly one transport (H3, Reality, or CDN). With multipath enabled, Shuttle opens a connection on each configured transport and splits traffic across them:

- **Aggregate bandwidth** — combine two 50 Mbps paths into ~100 Mbps effective throughput.
- **Redundancy** — if one path degrades or fails, traffic shifts to the surviving paths with no reconnection.
- **Latency hedging** — with `min-latency` scheduling, each new stream is sent on whichever path has the lowest current RTT.

---

## Scheduling Modes

| Mode | Description |
|------|-------------|
| `weighted` | Distribute traffic proportionally by configured weights |
| `min-latency` | Send each new stream on the path with the lowest measured RTT |
| `load-balance` | Round-robin across all healthy paths |

---

## Configuration

```yaml
outbounds:
  - tag: my-server
    type: auto
    server: your.server.example.com
    port: 443
    password: your-password

    transport:
      preferred: multipath
      multipath_schedule: min-latency   # weighted | min-latency | load-balance
      multipath_paths:
        - type: h3
          weight: 2          # used only with weighted scheduling
        - type: reality
          weight: 1
        - type: cdn
          weight: 1
```

### Field Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `transport.preferred` | string | `auto` | Set to `multipath` to enable |
| `transport.multipath_schedule` | string | `min-latency` | Scheduling policy |
| `transport.multipath_paths` | list | all available | Transports to include |
| `multipath_paths[].type` | string | required | `h3` / `reality` / `cdn` |
| `multipath_paths[].weight` | int | `1` | Weight for `weighted` scheduling |

---

## Use Cases

### Bandwidth Aggregation

If your client has two ISP links (or a wired + cellular connection), configure one path per transport type pointing to the same server. Shuttle will spread streams across both physical paths.

### Resilient Tunnels

Use multipath with `load-balance` to keep traffic flowing even if one ISP has a brief outage. Because all paths are already established, failover is instantaneous — there is no reconnect delay.

### Low-Latency Streaming

Use `min-latency` when you have one fast low-latency path and one slower higher-latency path. Interactive traffic (gaming, video calls) naturally lands on the fast path while bulk downloads use both.

---

## Notes

- The server must be reachable on all transport types included in `multipath_paths`.
- Each path maintains its own congestion controller. The `congestion` settings in the outbound apply to each path independently.
- Multipath is not compatible with CDN-only deployments where the CDN enforces a single connection per client.
