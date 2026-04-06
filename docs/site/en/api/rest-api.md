# REST API Reference

The Shuttle GUI exposes a REST API on a random local port (printed at startup). All endpoints use JSON unless noted otherwise.

**Base URL:** `http://127.0.0.1:{port}`

---

## Status

### GET /api/status

Returns the current engine status.

**Response:**
```json
{
  "state": "running",
  "circuit_state": "closed",
  "streams": 4,
  "transport": "h3",
  "uptime_seconds": 3600
}
```

### GET /api/version

Returns the current Shuttle version.

**Response:**
```json
{
  "version": "1.2.3"
}
```

### GET /api/debug/state

Returns detailed engine debug state including goroutine count and uptime.

**Response:**
```json
{
  "engine_state": "running",
  "circuit_breaker": "closed",
  "streams": 4,
  "transport": "h3",
  "uptime_seconds": 3600,
  "goroutines": 42
}
```

### GET /api/system/resources

Returns Go runtime memory and CPU resource usage.

**Response:**
```json
{
  "goroutines": 42,
  "mem_alloc_mb": 24.5,
  "mem_sys_mb": 64.0,
  "mem_gc_cycles": 12,
  "num_cpu": 8,
  "uptime_seconds": 3600
}
```

---

## Config

### GET /api/config

Returns the current client configuration (sensitive fields redacted).

**Response:** Full `ClientConfig` JSON object.

### PUT /api/config

Replaces the entire client configuration and hot-reloads the engine.

**Request body:** Full `ClientConfig` JSON object.

**Response:**
```json
{ "status": "reloaded" }
```

### GET /api/config/servers

Returns the active server and the saved server list.

**Response:**
```json
{
  "active": { "addr": "example.com:443", "name": "Main", "sni": "" },
  "servers": [
    { "addr": "example.com:443", "name": "Main", "password": "", "sni": "" }
  ]
}
```

### PUT /api/config/servers

Switches the active server and reloads.

**Request body:**
```json
{ "addr": "example.com:443", "name": "Main", "password": "secret", "sni": "" }
```

**Response:**
```json
{ "status": "updated" }
```

### POST /api/config/servers

Adds a new server to the saved server list.

**Request body:**
```json
{ "addr": "example.com:443", "name": "US Node", "password": "secret", "sni": "" }
```

**Response:**
```json
{ "status": "ok" }
```

**Errors:** `409 Conflict` if a server with the same address already exists.

### DELETE /api/config/servers

Removes a server from the saved list by address.

**Request body:**
```json
{ "addr": "example.com:443" }
```

**Response:**
```json
{ "status": "ok" }
```

**Errors:** `404 Not Found` if the address does not exist.

### POST /api/config/validate

Validates a configuration object without applying it.

**Request body:** Full `ClientConfig` JSON object.

**Response:**
```json
{
  "valid": true,
  "errors": []
}
```

### GET /api/config/export

Exports the current configuration as a downloadable file.

**Query parameters:**

| Parameter | Values | Default | Description |
|---|---|---|---|
| `format` | `json`, `yaml`, `uri` | `json` | Export format |
| `include_secrets` | `true`, `false` | `false` | Include passwords and keys |

Returns a file attachment (`application/json`, `text/yaml`, or `text/plain`).

### POST /api/config/import

Imports servers from a JSON/YAML/URI string. Duplicates are skipped.

**Request body:**
```json
{ "data": "shuttle://..." }
```

**Response:**
```json
{
  "status": "imported",
  "added": 2,
  "total": 3,
  "servers": [...],
  "errors": [],
  "mesh_enabled": false
}
```

---

## Proxy

### POST /api/connect

Starts the engine and optionally sets the system proxy.

**Response:**
```json
{ "status": "connected" }
```

**Errors:** `409 Conflict` if already running.

### POST /api/disconnect

Stops the engine and clears the system proxy.

**Response:**
```json
{ "status": "disconnected" }
```

### GET /api/autostart

Returns whether launch-at-login is enabled.

**Response:**
```json
{ "enabled": true }
```

### PUT /api/autostart

Enables or disables launch-at-login.

**Request body:**
```json
{ "enabled": true }
```

**Response:**
```json
{ "enabled": true }
```

### GET /api/network/lan

Returns LAN sharing status and local listener addresses.

**Response:**
```json
{
  "allow_lan": false,
  "addresses": ["192.168.1.100"],
  "socks5": "127.0.0.1:1080",
  "http": "127.0.0.1:8080"
}
```

---

## Routing

### GET /api/routing/rules

Returns the current routing configuration.

**Response:** Full `RoutingConfig` JSON object.

### PUT /api/routing/rules

Replaces the routing configuration and reloads.

**Request body:** Full `RoutingConfig` JSON object.

**Response:**
```json
{ "status": "updated" }
```

### GET /api/routing/export

Exports routing rules as a downloadable JSON file.

**Response:** `RoutingConfig` JSON attachment.

### POST /api/routing/import

Imports routing rules, merging with or replacing existing rules.

**Request body:**
```json
{
  "rules": [
    { "geosite": "cn", "action": "direct" }
  ],
  "default": "proxy",
  "mode": "merge"
}
```

`mode` is `merge` (append, default) or `replace`.

**Response:**
```json
{
  "status": "imported",
  "added": 1,
  "total": 5,
  "existing": 4
}
```

### GET /api/routing/templates

Returns built-in routing templates.

**Response:**
```json
[
  {
    "id": "bypass-cn",
    "name": "Bypass China",
    "description": "Direct connection for China sites, proxy for others",
    "rules": [...],
    "default": "proxy"
  },
  { "id": "proxy-all", ... },
  { "id": "direct-all", ... },
  { "id": "block-ads", ... }
]
```

Available template IDs: `bypass-cn`, `proxy-all`, `direct-all`, `block-ads`.

### POST /api/routing/templates/:id

Applies a built-in template as the active routing configuration (DNS settings are preserved).

**Response:**
```json
{ "status": "applied", "template": "bypass-cn" }
```

**Errors:** `404 Not Found` for unknown template IDs.

### POST /api/routing/test

Tests which routing action would be applied to a domain or URL.

**Request body:**
```json
{ "url": "https://www.google.com" }
```

**Response:** Router dry-run result object (action, matched rule, etc.).

### GET /api/routing/conflicts

Detects conflicting or shadowed rules in the current routing config.

**Response:**
```json
{
  "conflicts": [...],
  "count": 0
}
```

### GET /api/pac

Generates and returns a PAC (Proxy Auto-Config) script based on current routing rules.

**Query parameters:**

| Parameter | Value | Description |
|---|---|---|
| `download` | `true` | Adds `Content-Disposition: attachment` header |

**Response:** `application/x-ns-proxy-autoconfig` content.

---

## Transport

### POST /api/transport/strategy

Switches the active transport selection strategy at runtime.

**Request body:**
```json
{ "strategy": "auto" }
```

Valid strategies: `auto`, `priority`, `latency`, `multipath`.

**Response:**
```json
{ "ok": true, "strategy": "auto" }
```

**Errors:** `400 Bad Request` for invalid strategy; `409 Conflict` if the switch fails.

---

## Subscriptions

### GET /api/subscriptions

Returns all configured subscriptions.

**Response:**
```json
[
  {
    "id": "abc123",
    "name": "My Provider",
    "url": "https://example.com/sub"
  }
]
```

### POST /api/subscriptions

Adds a new subscription, fetches it immediately, and saves to config.

**Request body:**
```json
{ "name": "My Provider", "url": "https://example.com/sub" }
```

**Response:** The created subscription object.

### PUT /api/subscriptions/:id/refresh

Manually refreshes a subscription by ID.

**Response:** Updated subscription object.

### DELETE /api/subscriptions/:id

Removes a subscription and saves config.

**Response:**
```json
{ "status": "deleted" }
```

---

## Stats

### GET /api/stats/history

Returns per-day traffic statistics.

**Query parameters:**

| Parameter | Range | Default | Description |
|---|---|---|---|
| `days` | 1–90 | `7` | Number of days of history |

**Response:**
```json
{
  "history": [
    { "date": "2026-04-05", "bytes_sent": 1048576, "bytes_received": 5242880 }
  ],
  "total": { "bytes_sent": 1048576, "bytes_received": 5242880 }
}
```

### GET /api/stats/weekly

Returns per-week traffic summaries.

**Query parameters:**

| Parameter | Range | Default | Description |
|---|---|---|---|
| `weeks` | 1–52 | `4` | Number of weeks |

**Response:** Array of `PeriodStats` objects.

### GET /api/stats/monthly

Returns per-month traffic summaries.

**Query parameters:**

| Parameter | Range | Default | Description |
|---|---|---|---|
| `months` | 1–24 | `6` | Number of months |

**Response:** Array of `PeriodStats` objects.

### GET /api/connections/history

Returns the 100 most recent connection log entries.

**Response:** Array of connection log entries.

### GET /api/connections/:id/streams

Returns all streams belonging to a specific connection.

**Response:**
```json
[
  {
    "stream_id": 1,
    "conn_id": "abc123",
    "target": "www.google.com:443",
    "transport": "h3",
    "bytes_sent": 1024,
    "bytes_received": 4096,
    "errors": 0,
    "closed": false,
    "duration_ms": 250
  }
]
```

### GET /api/transports/stats

Returns per-transport traffic breakdown from the current engine status.

**Response:** Array of transport breakdown objects.

### GET /api/multipath/stats

Returns multipath scheduler statistics.

**Response:** Array of per-path statistics objects.

### GET /api/logs *(WebSocket)*

Streams log events in real time. Upgrade with `Connection: Upgrade, Upgrade: websocket`.

### GET /api/speed *(WebSocket)*

Streams speed tick events (bytes/sec up and down) in real time.

### GET /api/connections *(WebSocket)*

Streams active connection events in real time.

---

## Mesh

### GET /api/mesh/status

Returns high-level mesh VPN status.

**Response:**
```json
{
  "enabled": true,
  "virtual_ip": "10.7.0.3",
  "cidr": "10.7.0.0/24",
  "peer_count": 2
}
```

### GET /api/mesh/peers

Returns all mesh peers with connection quality information.

**Response:**
```json
[
  {
    "vip": "10.7.0.2",
    "name": "client-b",
    "connected": true,
    "latency_ms": 12
  }
]
```

### POST /api/mesh/peers/:vip/connect

Triggers a P2P connection attempt to a peer by virtual IP.

**Response:**
```json
{ "ok": true, "vip": "10.7.0.2" }
```

**Errors:** `409 Conflict` if the connection attempt fails.

---

## Groups

### GET /api/groups

Returns all strategy groups with current status.

**Response:**
```json
[
  {
    "tag": "auto",
    "type": "url-test",
    "selected": "US Node",
    "members": ["US Node", "EU Node"]
  }
]
```

### GET /api/groups/:tag

Returns details for a specific group including member latencies.

**Response:** Group detail object.

### PUT /api/groups/:tag/selected

Selects a specific outbound node in a `select`-strategy group.

**Request body:**
```json
{ "selected": "US Node" }
```

**Response:** `204 No Content`

### POST /api/groups/:tag/test

Triggers a health check for all members of a group and returns latency results.

**Response:** Map of member name to latency (ms).

---

## Providers

### GET /api/providers/proxy

Returns all proxy providers with status and proxy counts.

**Response:**
```json
[
  {
    "name": "my-provider",
    "url": "https://example.com/proxies.yaml",
    "count": 10,
    "last_updated": "2026-04-05T12:00:00Z"
  }
]
```

### POST /api/providers/proxy/:name/refresh

Manually refreshes a proxy provider by name.

**Response:** `204 No Content`

**Errors:** `404 Not Found` if the provider does not exist.

### GET /api/providers/rule

Returns all rule providers with status.

**Response:**
```json
[
  {
    "name": "direct-list",
    "url": "https://example.com/direct.txt",
    "behavior": "domain",
    "count": 500
  }
]
```

### POST /api/providers/rule/:name/refresh

Manually refreshes a rule provider by name.

**Response:** `204 No Content`

**Errors:** `404 Not Found` if the provider does not exist.
