# Multi-Outbound Routing

Shuttle supports routing traffic to multiple proxy servers using custom outbound tags.

## Configuration

### Define outbounds

```yaml
outbounds:
  - tag: "us-server"
    type: "proxy"
    options:
      server: "us.example.com:443"
  - tag: "jp-server"
    type: "proxy"
    options:
      server: "jp.example.com:443"
```

### Route by rules

```yaml
routing:
  default: proxy  # default outbound (built-in proxy)
  rule_chain:
    - match:
        geoip: "JP"
      action: "jp-server"    # route to jp-server outbound
    - match:
        geoip: "US"
      action: "us-server"    # route to us-server outbound
    - match:
        domain_suffix: ".internal.corp"
      action: "direct"       # bypass proxy
    - match:
        domain: "ads.example.com"
      action: "reject"       # block connection
```

### Built-in outbounds

These are always available without explicit configuration:

| Tag | Behavior |
|-----|----------|
| `proxy` | Route through the default proxy server (`server.addr`) |
| `direct` | Connect directly, bypass proxy |
| `reject` | Reject the connection |

### How it works

1. Each inbound connection is matched against `rule_chain` in order
2. The first matching rule's `action` determines which outbound handles the connection
3. If no rule matches, the `routing.default` action is used
4. Custom outbound tags (e.g., "us-server") must match a configured outbound's `tag`

### Use cases

- **Geographic routing**: Route traffic to the nearest server by GeoIP
- **Load distribution**: Split traffic across multiple servers
- **Specialized servers**: Use different servers for different protocols or domains
