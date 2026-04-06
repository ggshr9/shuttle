# Migrate from Clash

This guide helps you translate an existing Clash config into Shuttle's format. The concepts map closely — mostly it's key renames and structure differences.

## Concept Mapping

| Clash | Shuttle | Notes |
|-------|---------|-------|
| `proxies:` | `outbounds:` | Same concept, different key |
| `proxy-groups:` | `outbounds:` with `type: "group"` | Groups and proxies share the same list |
| `rules:` | `routing.rule_chain:` | Shuttle uses structured YAML, not inline strings |
| `proxy-providers:` | `proxy_providers:` | Same concept, snake_case key |
| `rule-providers:` | `rule_providers:` | domain / ipcidr / classical behaviors supported |
| `dns.fake-ip-range` | `routing.dns.fake_ip_range` | DNS config lives under `routing.dns` |
| `tun.enable` | `proxy.tun.enabled` | TUN config lives under `proxy.tun` |
| `url-test` group | `type: "group"` + `strategy: "url-test"` | Supports `tolerance_ms` |
| `fallback` group | `type: "group"` + `strategy: "failover"` | Same behavior |
| `select` group | `type: "group"` + `strategy: "select"` | Switchable via API and GUI |
| `load-balance` group | `type: "group"` + `strategy: "loadbalance"` | Round-robin scheduling |

## Config Conversion Example

### Single Proxy

::: code-group

```yaml [Clash]
proxies:
  - name: "hk-01"
    type: ss
    server: hk.example.com
    port: 8388
    cipher: aes-256-gcm
    password: "your-password"
```

```yaml [Shuttle]
outbounds:
  - tag: "hk-01"
    type: "shadowsocks"
    server: "hk.example.com:8388"
    method: "aes-256-gcm"
    password: "your-password"
```

:::

### url-test Group with Health Check

::: code-group

```yaml [Clash]
proxy-groups:
  - name: "Auto"
    type: url-test
    proxies:
      - hk-01
      - sg-01
    url: "http://www.gstatic.com/generate_204"
    interval: 300
    tolerance: 50
```

```yaml [Shuttle]
outbounds:
  - tag: "Auto"
    type: "group"
    strategy: "url-test"
    use:
      - "hk-01"
      - "sg-01"
    health_check:
      url: "http://www.gstatic.com/generate_204"
      interval: "300s"
      tolerance_ms: 50
```

:::

### Rules

::: code-group

```yaml [Clash]
rules:
  - DOMAIN-SUFFIX,google.com,Auto
  - GEOIP,CN,DIRECT
  - MATCH,Auto
```

```yaml [Shuttle]
routing:
  rule_chain:
    - match:
        domain_suffix:
          - "google.com"
      action: "Auto"

    - match:
        geoip:
          - "CN"
      action: "direct"

    - action: "Auto"   # catch-all (no match conditions)
```

:::

The key difference: Clash uses comma-separated inline strings (`RULE_TYPE,value,action`). Shuttle uses structured YAML with explicit `match` and `action` fields.

## Importing Subscriptions

Your existing Clash subscription URLs work directly in Shuttle:

```yaml
proxy_providers:
  - name: "my-sub"
    url: "https://your-clash-subscription-url"
    interval: "3600s"
    health_check:
      url: "http://www.gstatic.com/generate_204"
      interval: "300s"
```

Shuttle parses Clash YAML subscriptions automatically. You can reference the provider's proxies in groups:

```yaml
outbounds:
  - tag: "Auto"
    type: "group"
    strategy: "url-test"
    use:
      - "my-sub"   # all proxies from this provider
```

## DNS Configuration

DNS settings moved from the top-level `dns:` block into `routing.dns`:

::: code-group

```yaml [Clash]
dns:
  enable: true
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.0/15
  nameserver:
    - 114.114.114.114
  fallback:
    - tls://8.8.8.8:853
```

```yaml [Shuttle]
routing:
  dns:
    mode: "fake-ip"
    fake_ip_range: "198.18.0.0/15"
    domestic: "114.114.114.114"
    remote:
      server: "tls://8.8.8.8:853"
      via: "proxy"
    cache: true
    leak_prevention: true
```

:::

## TUN Mode

::: code-group

```yaml [Clash]
tun:
  enable: true
  stack: gvisor
  auto-route: true
  auto-detect-interface: true
```

```yaml [Shuttle]
proxy:
  tun:
    enabled: true
    auto_route: true
    device_name: "utun8"
    cidr: "198.18.0.1/16"
    mtu: 9000
```

:::

## What Shuttle Has That Clash Doesn't

- **Quality strategy** (`strategy: "quality"`): congestion-aware group routing — picks the proxy with lowest latency *and* best bandwidth, not just lowest ping.
- **Adaptive congestion control**: automatically switches between BBR, Brutal, and conservative modes based on real-time packet loss and RTT.
- **Mesh VPN**: P2P NAT traversal with STUN, hole punching, and TURN fallback — no external VPN server needed.
- **Multipath**: aggregate bandwidth across multiple transports simultaneously.
- **Post-quantum crypto**: optional ML-KEM key exchange on H3 and Reality transports.
- **Group nesting**: groups can reference other groups, not just leaf proxies.

## What Clash Has That Shuttle Doesn't Yet

- **Script / Starlark rules**: Clash allows Python-like scripting for custom routing logic. Shuttle uses structured YAML rules only.
- **TProxy mode** (`tproxy`): transparent proxy for router deployments. Shuttle currently supports SOCKS5, HTTP, and TUN modes.

## Common Gotchas

**Rule syntax is not inline strings.**

Shuttle does not parse `DOMAIN-SUFFIX,google.com,PROXY` strings. Each rule is a YAML object with `match` and `action`. See the [Rules example](#rules) above.

**DNS config location changed.**

In Clash, DNS is a top-level `dns:` block. In Shuttle, it's `routing.dns`. Moving the block elsewhere will silently use defaults.

**Proxy type names differ slightly.**

| Clash type | Shuttle type |
|------------|--------------|
| `ss` | `shadowsocks` |
| `vmess` | `vmess` |
| `trojan` | `trojan` |
| `vless` | `vless` |
| `hysteria2` | `hysteria2` |
| `wireguard` | `wireguard` |

**Groups and proxies share `outbounds:`.**

Clash separates `proxies:` from `proxy-groups:`. In Shuttle both live in `outbounds:` — regular proxies have protocol types (`shadowsocks`, `vmess`, etc.) while groups use `type: "group"`.

**`interval` is a duration string, not seconds.**

Clash uses plain integers for intervals (e.g., `interval: 300` meaning 300 seconds). Shuttle uses Go duration strings: `"300s"`, `"5m"`, `"1h"`.
