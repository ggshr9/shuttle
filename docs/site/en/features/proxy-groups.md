# Proxy Groups

Proxy groups let you bundle outbounds and apply a selection strategy. The chosen member handles the actual traffic.

## Strategies

| Strategy | Description |
|----------|-------------|
| `url-test` | Periodically tests all members; picks the fastest |
| `failover` | Uses the first healthy member in order |
| `select` | Manually chosen via API or GUI |
| `loadbalance` | Round-robin across all members |
| `quality` | Congestion-aware routing — unique to Shuttle |

### quality

The `quality` strategy samples each member's BBR bandwidth estimate and routes new connections to the highest-throughput member. It re-evaluates every `interval` seconds and avoids members whose loss rate exceeds `max_loss`.

---

## Configuration

```yaml
proxy_groups:
  - tag: auto
    type: url-test
    outbounds:
      - hk-01
      - hk-02
      - sg-01
    url: https://www.gstatic.com/generate_204
    interval: 300          # health check interval, seconds
    tolerance_ms: 50       # treat members within 50 ms of the best as equal

  - tag: fallback
    type: failover
    outbounds:
      - hk-01
      - sg-01
      - us-01
    url: https://www.gstatic.com/generate_204
    interval: 60

  - tag: manual
    type: select
    outbounds:
      - auto
      - fallback
      - DIRECT

  - tag: balance
    type: loadbalance
    outbounds:
      - hk-01
      - hk-02
      - hk-03

  - tag: best-quality
    type: quality
    outbounds:
      - hk-01
      - sg-01
    interval: 30
    max_loss: 0.05         # exclude members with >5% packet loss
```

### Field Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `tag` | string | required | Unique group name |
| `type` | string | required | Strategy: `url-test` / `failover` / `select` / `loadbalance` / `quality` |
| `outbounds` | list | required | Member outbound tags (may include other groups) |
| `url` | string | `https://www.gstatic.com/generate_204` | Health check URL |
| `interval` | int | `300` | Check interval in seconds |
| `tolerance_ms` | int | `0` | url-test: accept members within N ms of best |
| `max_loss` | float | `0.10` | quality: exclude members above this loss rate |

---

## Group Nesting

Groups can reference other groups as members, allowing tiered logic:

```yaml
proxy_groups:
  - tag: hk-auto
    type: url-test
    outbounds: [hk-01, hk-02]

  - tag: sg-auto
    type: url-test
    outbounds: [sg-01, sg-02]

  - tag: global
    type: failover
    outbounds:
      - hk-auto      # tries fastest HK first
      - sg-auto      # falls back to fastest SG
      - DIRECT
```

---

## Selecting via API

For `select`-type groups, use the REST API to change the active member at runtime:

```http
PUT /api/groups/{tag}/selected
Content-Type: application/json

{"selected": "sg-01"}
```

Response `200 OK` on success. The new selection persists until the next config reload.

---

## GUI Usage

In the GUI, open **Proxies** from the sidebar. Each group is shown as a card with its current selection and latency badges. Click any member to select it (for `select` groups) or force-test (for `url-test` groups).
