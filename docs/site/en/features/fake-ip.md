# fake-ip DNS

fake-ip is a DNS mode designed for TUN-based transparent proxying. Instead of resolving a domain to its real IP, Shuttle returns a synthetic IP from a reserved range and maps that IP back to the original domain internally.

## Why fake-ip?

In TUN mode every packet goes through the Shuttle virtual interface. If real DNS resolution happens first, the kernel routes packets to the real IP — bypassing the proxy for direct connections, or making it impossible to intercept traffic that should be proxied.

With fake-ip:

1. An app queries DNS for `example.com`.
2. Shuttle returns `198.18.0.1` (a fake IP from the reserved pool).
3. The app connects to `198.18.0.1`.
4. Shuttle intercepts the packet, looks up `198.18.0.1` → `example.com`, and routes it through the appropriate outbound.

This eliminates a round-trip DNS query on the critical path, reducing perceived latency for the first connection.

---

## Configuration

```yaml
dns:
  mode: fake-ip             # enable fake-ip mode (alternative: normal)
  fake_ip_range: 198.18.0.0/15   # pool of synthetic IPs
  fake_ip_filter:           # domains that bypass fake-ip and get real IPs
    - "*.lan"
    - "*.local"
    - "*.stun.*"
    - "stun.*.*"
    - "+.stun.*.*.*"
    - "localhost"
    - "time.*.com"
    - "ntp.*.*"
    - "*.ntp.org"
  persist: false            # persist the fake-ip mapping across restarts
```

### Field Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `mode` | string | `normal` | `fake-ip` or `normal` |
| `fake_ip_range` | CIDR | `198.18.0.0/15` | Pool for synthetic IPs |
| `fake_ip_filter` | list | see below | Domains that get real IPs |
| `persist` | bool | `false` | Save mapping to disk on shutdown |

---

## Default Filter Patterns

The following patterns are filtered by default (they receive real IP responses):

| Pattern | Reason |
|---------|--------|
| `*.local`, `*.lan` | LAN service discovery |
| `localhost` | Loopback |
| `*.stun.*`, `stun.*.*` | WebRTC STUN — needs real IPs |
| `ntp.*.*`, `*.ntp.org` | NTP — clock sync requires real IPs |
| `time.*.com` | Time services |

Add entries to `fake_ip_filter` to extend this list.

---

## Known Compatibility Issues

**NTP / time sync** — Always filter your NTP server. Fake IPs break `ntpd` / `chronyd`.

**STUN / WebRTC** — STUN probes send the source IP; fake IPs cause incorrect reflexive address detection. The default filter covers common STUN hostnames.

**mDNS / Bonjour** — Multicast DNS operates outside the normal DNS stack; fake-ip has no effect and does not interfere.

**iOS / Android captive portal detection** — Some platforms probe specific Apple/Google URLs. If these return fake IPs, the device may show a "no internet" warning. Add the relevant hostnames to `fake_ip_filter`.

**Split-DNS environments** — If your internal DNS uses private domains not delegated publicly, ensure those domains are in `fake_ip_filter` so they resolve against your internal DNS rather than receiving fake IPs.
