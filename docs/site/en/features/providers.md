# Providers

Providers let Shuttle fetch outbound lists and rule sets from remote URLs (or local files), keeping your config thin while pulling in external subscriptions.

---

## Proxy Providers

A proxy provider fetches a list of outbound servers. Shuttle auto-detects the format: Clash YAML, sing-box JSON, base64-encoded SIP002/URI lists, or plain `ss://` / `vmess://` URIs.

### Configuration

```yaml
proxy_providers:
  - name: my-sub
    url: https://sub.example.com/clash.yaml
    interval: 3600          # refresh every hour
    filter: "HK|SG"         # keep only entries matching this regex
    health_check:
      url: https://www.gstatic.com/generate_204
      interval: 300

  - name: local-list
    path: ./servers.yaml    # local file, no interval needed
    filter: ""
```

### Field Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | required | Provider name, referenced in groups |
| `url` | string | — | Remote subscription URL |
| `path` | string | — | Local file path (use `url` or `path`, not both) |
| `interval` | int | `3600` | Refresh interval in seconds |
| `filter` | string | `""` | Regex applied to outbound names; empty = keep all |
| `health_check.url` | string | generate_204 | URL for latency probing |
| `health_check.interval` | int | `300` | Probe interval in seconds |

### Auto-Format Detection

Shuttle tries formats in this order:

1. Clash YAML (`proxies:` key present)
2. sing-box JSON (`outbounds` array present)
3. Base64 — decodes and re-parses
4. Plain URI list — one `ss://` / `vmess://` / `vless://` / `trojan://` per line

### Local Cache

Downloaded content is cached to disk so the proxy still starts if the remote is unreachable. The cache file lives next to your config as `.<name>.cache`.

### Using in Groups

Reference the provider in a proxy group with `use`:

```yaml
proxy_groups:
  - tag: auto
    type: url-test
    use:
      - my-sub        # pulls all (filtered) entries from the provider
    interval: 300
```

---

## Rule Providers

A rule provider fetches a list of matching rules — domains, IPs, or classical rules — and injects them into your routing pipeline.

### Configuration

```yaml
rule_providers:
  - name: gfw-domains
    url: https://rules.example.com/gfw.yaml
    behavior: domain          # domain | ipcidr | classical
    interval: 86400
    format: yaml              # yaml | text

  - name: cn-cidr
    url: https://rules.example.com/cn-ipcidr.yaml
    behavior: ipcidr
    interval: 86400
```

### Behaviors

| Behavior | Matches | Example entry |
|----------|---------|---------------|
| `domain` | Exact domain or suffix | `google.com` |
| `ipcidr` | IPv4/IPv6 CIDR | `8.8.8.0/24` |
| `classical` | Full Clash-style rules | `DOMAIN-SUFFIX,google.com` |

### Hot-Reload

Rule providers are refreshed in the background. Shuttle swaps the rule set atomically — in-flight connections are not affected.

### Referencing in Routing

Use `rule_provider` inside a `match` block:

```yaml
routing:
  rules:
    - match:
        rule_provider: ["gfw-domains", "extra-domains"]
      outbound: my-proxy

    - match:
        rule_provider: ["cn-cidr"]
      outbound: DIRECT

    - outbound: my-proxy    # default
```

Multiple provider names in the list are OR-combined.
