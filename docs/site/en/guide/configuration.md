# Configuration Reference

Shuttle uses YAML configuration files. Client config is loaded from `config.yaml` (or the path passed via `--config`); server config defaults to `server.yaml`.

Both files support hot-reload: send `SIGHUP` or call `PUT /api/config` to apply changes without restarting.

---

## Client Configuration

Top-level structure: `ClientConfig`

| Field | Type | Default | Description |
|---|---|---|---|
| `version` | int | `1` | Config schema version |
| `server` | ServerEndpoint | — | Active server endpoint |
| `servers` | []ServerEndpoint | `[]` | Saved server list |
| `subscriptions` | []SubscriptionConfig | `[]` | Remote server subscription sources |
| `transport` | TransportConfig | — | Transport protocol settings |
| `proxy` | ProxyConfig | — | Local proxy listeners |
| `routing` | RoutingConfig | — | Traffic routing rules |
| `qos` | QoSConfig | — | Quality of Service marking |
| `congestion` | CongestionConfig | — | Congestion control algorithm |
| `retry` | RetryConfig | — | Connection retry with backoff |
| `mesh` | MeshConfig | — | Mesh VPN settings |
| `obfs` | ObfsConfig | — | Traffic obfuscation |
| `yamux` | YamuxConfig | — | Yamux multiplexer tuning |
| `log` | LogConfig | — | Logging settings |
| `inbounds` | []InboundConfig | `[]` | Pluggable inbound listeners |
| `outbounds` | []OutboundConfig | `[]` | Pluggable outbound dialers |
| `proxy_providers` | []ProxyProviderConfig | `[]` | Remote proxy provider sources |
| `rule_providers` | []RuleProviderConfig | `[]` | Remote rule provider sources |

### ServerEndpoint

Defines a remote Shuttle server.

| Field | Type | Default | Description |
|---|---|---|---|
| `addr` | string | — | Server address, e.g. `example.com:443` |
| `name` | string | `""` | Human-readable label |
| `password` | string | `""` | Shared secret for authentication |
| `sni` | string | `""` | TLS SNI override |

**Example:**
```yaml
server:
  addr: example.com:443
  name: Main Server
  password: my-secret
  sni: ""
```

### SubscriptionConfig

| Field | Type | Description |
|---|---|---|
| `id` | string | Unique subscription ID (auto-generated) |
| `name` | string | Human-readable label |
| `url` | string | HTTP(S) URL to the subscription feed |

---

## Transport Configuration

`TransportConfig` — controls which transports are available and how they are selected.

| Field | Type | Default | Description |
|---|---|---|---|
| `preferred` | string | `"auto"` | Preferred transport: `h3`, `reality`, `cdn`, `webrtc`, `auto`, `multipath` |
| `multipath_schedule` | string | `"weighted"` | Multipath scheduler: `weighted`, `min-latency`, `load-balance` |
| `warm_up_conns` | int | `0` | Pre-dial N connections on startup to eliminate cold-start latency |
| `pool_max_idle` | int | `4` | Max idle connections per transport |
| `pool_idle_ttl` | string | `"60s"` | Idle connection TTL |
| `keepalive_interval` | string | `"15s"` | Transport keepalive interval |
| `keepalive_timeout` | string | `"5s"` | Keepalive response timeout |
| `proactive_migration` | bool | `false` | Zero-downtime network switching |
| `migration_probe_timeout` | string | `"3s"` | Probe timeout for proactive migration |
| `h3` | H3Config | — | HTTP/3 settings |
| `reality` | RealityConfig | — | Reality transport settings |
| `cdn` | CDNConfig | — | CDN transport settings |
| `webrtc` | WebRTCConfig | — | WebRTC DataChannel settings |

### H3Config (HTTP/3 over QUIC)

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Enable H3 transport |
| `path_prefix` | string | `"/shuttle"` | URL path prefix for H3 streams |
| `insecure_skip_verify` | bool | `false` | Skip TLS certificate verification |
| `multipath` | MultipathConfig | — | QUIC multipath settings |

### MultipathConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable QUIC multipath |
| `interfaces` | []string | `[]` | Bind to specific interfaces (empty = auto-detect) |
| `mode` | string | `"aggregate"` | `redundant`, `aggregate`, `failover` |
| `probe_interval` | string | `"5s"` | Path probe interval |

### RealityConfig (TLS + Noise IK)

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Enable Reality transport |
| `server_name` | string | `""` | SNI for Reality handshake impersonation |
| `short_id` | string | `""` | Short ID for client identification |
| `public_key` | string | `""` | Server's Noise IK public key (base64) |
| `post_quantum` | bool | `false` | Enable hybrid X25519 + ML-KEM-768 key exchange |

### CDNConfig (HTTP/2 + gRPC)

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable CDN transport |
| `domain` | string | `""` | CDN domain (actual destination) |
| `path` | string | `"/cdn/stream"` | gRPC/H2 path |
| `mode` | string | `"grpc"` | `h2` or `grpc` |
| `front_domain` | string | `""` | SNI domain for domain fronting |
| `insecure_skip_verify` | bool | `false` | Skip TLS verification |

### WebRTCConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable WebRTC DataChannel transport |
| `signal_url` | string | `""` | Signaling server WebSocket URL |
| `stun_servers` | []string | `[]` | STUN server URLs |
| `turn_servers` | []string | `[]` | TURN server URLs |
| `turn_user` | string | `""` | TURN username |
| `turn_pass` | string | `""` | TURN password |
| `ice_policy` | string | `"all"` | ICE candidate policy: `all`, `relay`, `public` |

---

## Proxy Configuration

`ProxyConfig` — local listener settings.

| Field | Type | Default | Description |
|---|---|---|---|
| `allow_lan` | bool | `false` | Allow other LAN devices to use this proxy |
| `socks5` | SOCKS5Config | — | SOCKS5 listener |
| `http` | HTTPConfig | — | HTTP CONNECT listener |
| `tun` | TUNConfig | — | TUN device (transparent proxy) |
| `system_proxy` | SystemProxyConfig | — | Automatic system proxy |

### SOCKS5Config

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Enable SOCKS5 listener |
| `listen` | string | `"127.0.0.1:1080"` | Listen address |

### HTTPConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Enable HTTP CONNECT listener |
| `listen` | string | `"127.0.0.1:8080"` | Listen address |

### TUNConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable TUN device |
| `device_name` | string | `"shuttle0"` | TUN device name |
| `cidr` | string | `"198.18.0.0/15"` | TUN device CIDR |
| `mtu` | int | `1500` | MTU size |
| `auto_route` | bool | `false` | Automatically add routes for TUN |
| `per_app_mode` | string | `""` | Per-app routing: `allow`, `deny`, or `""` (disabled) |
| `app_list` | []string | `[]` | App package names / bundle IDs for per-app routing |

### SystemProxyConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Auto-set OS system proxy on connect |

---

## Routing Configuration

`RoutingConfig` — traffic routing rules and DNS settings.

| Field | Type | Default | Description |
|---|---|---|---|
| `rules` | []RouteRule | `[]` | Ordered list of routing rules |
| `rule_chain` | []RuleChainEntry | `[]` | Advanced multi-condition rule chain (evaluated first) |
| `default` | string | `"proxy"` | Default action when no rule matches: `proxy`, `direct`, `reject` |
| `dns` | DNSConfig | — | DNS resolver settings |
| `geodata` | GeoDataConfig | — | GeoIP/GeoSite data management |

### RouteRule

Each rule matches on one condition and specifies an action. The first matching rule wins.

| Field | Type | Description |
|---|---|---|
| `domains` | string | Domain pattern (suffix match) |
| `geosite` | string | GeoSite category, e.g. `cn`, `google`, `category-ads-all` |
| `geoip` | string | GeoIP country code, e.g. `CN`, `US` |
| `ip_cidr` | []string | List of IP CIDR ranges |
| `process` | []string | Process names (app-level routing) |
| `protocol` | string | Network protocol: `tcp`, `udp` |
| `network_type` | string | Network type: `wifi`, `cellular`, `ethernet` |
| `action` | string | **Required.** `proxy`, `direct`, `reject` |

**Example:**
```yaml
routing:
  default: proxy
  rules:
    - geosite: cn
      action: direct
    - geoip: CN
      action: direct
    - geosite: private
      action: direct
    - geosite: category-ads-all
      action: reject
```

### RuleChainEntry

Advanced rule with multiple match conditions evaluated together.

| Field | Type | Default | Description |
|---|---|---|---|
| `match` | RuleMatch | — | Match conditions |
| `logic` | string | `"and"` | Combine conditions: `and`, `or` |
| `action` | string | — | `proxy`, `direct`, `reject` |

### RuleMatch fields

| Field | Type | Description |
|---|---|---|
| `domain` | []string | Exact domain matches |
| `domain_suffix` | []string | Suffix domain matches |
| `domain_keyword` | []string | Keyword domain matches |
| `geosite` | []string | GeoSite categories |
| `ip_cidr` | []string | IP CIDR ranges |
| `geoip` | []string | GeoIP country codes |
| `process` | []string | Process names |
| `protocol` | []string | Protocols |
| `network_type` | []string | Network types |
| `rule_provider` | []string | Rule provider names |

### DNSConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `domestic` | string | `"223.5.5.5"` | Domestic DNS server (plain UDP/TCP) |
| `remote.server` | string | `"8.8.8.8"` | Remote DNS server |
| `remote.via` | string | `"proxy"` | Route remote DNS through `proxy` or `direct` |
| `cache` | bool | `true` | Enable DNS response cache |
| `prefetch` | bool | `false` | Prefetch popular DNS records |
| `leak_prevention` | bool | `false` | Force all DNS through proxy |
| `domestic_doh` | string | `""` | DoH URL for domestic queries, e.g. `https://dns.alidns.com/dns-query` |
| `strip_ecs` | bool | `false` | Strip EDNS Client Subnet from responses |
| `persistent_conn` | bool | `true` | Persistent HTTP/2 connections for DoH |
| `mode` | string | `"normal"` | DNS mode: `normal` or `fake-ip` |
| `fake_ip_range` | string | `"198.18.0.0/15"` | CIDR pool for fake-ip mappings |
| `fake_ip_filter` | []string | `[]` | Domains to bypass fake-ip and use real DNS |
| `persist` | bool | `false` | Persist fake-ip mappings across restarts |

### GeoDataConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Enable GeoIP/GeoSite data |
| `data_dir` | string | OS cache dir | Directory for geo database files |
| `auto_update` | bool | `true` | Auto-update geo databases |
| `update_interval` | string | `"24h"` | Update check interval |
| `direct_list_url` | string | Loyalsoldier upstream | Override URL for direct list |
| `proxy_list_url` | string | Loyalsoldier upstream | Override URL for proxy list |
| `reject_list_url` | string | Loyalsoldier upstream | Override URL for reject list |
| `gfw_list_url` | string | Loyalsoldier upstream | Override URL for GFW list |
| `cn_cidr_url` | string | Loyalsoldier upstream | Override URL for CN CIDR list |
| `private_cidr_url` | string | Loyalsoldier upstream | Override URL for private CIDR list |

---

## Congestion Control

`CongestionConfig`

| Field | Type | Default | Description |
|---|---|---|---|
| `mode` | string | `"adaptive"` | Algorithm: `adaptive`, `bbr`, `brutal` |
| `brutal_rate` | uint64 | `104857600` | Target send rate in bytes/sec for Brutal mode (default 100 MB/s) |

- **adaptive** — automatically switches between BBR and Brutal based on packet loss and RTT
- **bbr** — bandwidth-based congestion control (good for clean networks)
- **brutal** — constant-rate sending (effective under active interference)

---

## Retry Configuration

`RetryConfig`

| Field | Type | Default | Description |
|---|---|---|---|
| `max_attempts` | int | `3` | Maximum connection retry attempts |
| `initial_backoff` | string | `"100ms"` | Initial retry delay |
| `max_backoff` | string | `"5s"` | Maximum retry delay |

---

## Mesh VPN

`MeshConfig`

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable mesh VPN |
| `p2p_enabled` | bool | `false` | Enable P2P NAT traversal |
| `p2p` | P2PConfig | — | P2P traversal settings |
| `split_routes` | []SplitRoute | `[]` | Per-subnet routing overrides |

### P2PConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `stun_servers` | []string | `[]` | STUN server URLs for NAT detection |
| `hole_punch_timeout` | string | `"10s"` | Timeout for hole-punch attempts |
| `direct_retry_interval` | string | `"60s"` | Interval before retrying direct P2P after failure |
| `keep_alive_interval` | string | `"30s"` | P2P connection keepalive interval |
| `fallback_threshold` | float64 | `0.3` | Packet-loss rate at which to fall back to relay |
| `spoof_mode` | string | `"none"` | Port spoofing: `none`, `dns` (53), `https` (443), `ike` (500) |
| `spoof_port` | int | `0` | Custom spoof port when `spoof_mode` is a number |
| `disable_upnp` | bool | `false` | Disable UPnP/NAT-PMP auto-detection |
| `preferred_port` | int | `0` | Preferred external port (0 = same as local) |

### SplitRoute

| Field | Type | Description |
|---|---|---|
| `cidr` | string | Subnet, e.g. `10.7.0.128/25` |
| `action` | string | `mesh`, `direct`, or `proxy` |

---

## Traffic Obfuscation

`ObfsConfig`

| Field | Type | Default | Description |
|---|---|---|---|
| `padding_enabled` | bool | `false` | Add random padding to packets |
| `shaping_enabled` | bool | `false` | Add timing jitter to traffic |
| `min_delay` | string | `"0s"` | Minimum inter-packet delay |
| `max_delay` | string | `"50ms"` | Maximum inter-packet delay |
| `chunk_size` | int | `64` | Minimum chunk size for packet splitting |

---

## Quality of Service

`QoSConfig`

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable QoS DSCP marking |
| `rules` | []QoSRule | `[]` | Traffic classification rules |

### QoSRule

| Field | Type | Description |
|---|---|---|
| `ports` | []uint16 | Destination port numbers to match |
| `protocol` | string | `tcp` or `udp` |
| `domains` | []string | Domain patterns to match |
| `process` | []string | Process names to match |
| `priority` | string | Priority level: `critical`, `high`, `normal`, `bulk`, `low` |

Priority → DSCP mapping: `critical`=EF(46), `high`=AF41(34), `normal`=AF21(18), `bulk`=AF11(10), `low`=BE(0).

---

## Yamux Multiplexer

`YamuxConfig`

| Field | Type | Default | Description |
|---|---|---|---|
| `max_stream_window_size` | uint32 | `262144` | Max stream window size in bytes (256 KB) |
| `keep_alive_interval` | int | `30` | Keepalive interval in seconds |
| `connection_write_timeout` | int | `10` | Write timeout in seconds |

---

## Logging

`LogConfig`

| Field | Type | Default | Description |
|---|---|---|---|
| `level` | string | `"info"` | Log level: `debug`, `info`, `warn`, `error` |
| `format` | string | `"text"` | Log format: `text` or `json` |
| `output` | string | `"stdout"` | Output destination: `stdout`, `stderr`, or a file path |

---

## Inbounds and Outbounds

### InboundConfig

Pluggable inbound listeners (Shadowsocks, VLESS, Trojan, etc.).

| Field | Type | Description |
|---|---|---|
| `tag` | string | Unique identifier for this inbound |
| `type` | string | Protocol type: `shadowsocks`, `vless`, `trojan`, etc. |
| `listen` | string | Listen address, e.g. `0.0.0.0:1234` |
| `options` | object | Protocol-specific options (JSON/YAML object) |

### OutboundConfig

Pluggable outbound dialers and strategy groups.

| Field | Type | Description |
|---|---|---|
| `tag` | string | Unique identifier |
| `type` | string | Type: `direct`, `reject`, `shadowsocks`, `vless`, `trojan`, `urltest`, `fallback`, `select`, `loadbalance`, `quality` |
| `options` | object | Protocol/strategy-specific options |
| `use` | []string | Proxy provider names to pull members from (for group outbounds) |
| `health_check` | HealthCheckConfig | Health check settings |

---

## Providers

### ProxyProviderConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `name` | string | — | Unique provider name |
| `url` | string | — | Remote URL for the provider feed |
| `path` | string | `""` | Local cache file path |
| `interval` | string | `"1h"` | Auto-refresh interval |
| `filter` | string | `""` | Regex filter for proxy names |
| `health_check` | HealthCheckConfig | — | Health check configuration |

### RuleProviderConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `name` | string | — | Unique provider name |
| `url` | string | — | Remote URL for the rule list |
| `path` | string | `""` | Local cache file path |
| `behavior` | string | — | Rule behavior: `domain`, `ipcidr`, `classical` |
| `interval` | string | `"24h"` | Auto-refresh interval |

### HealthCheckConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `url` | string | `"http://www.gstatic.com/generate_204"` | Health check URL |
| `interval` | string | `"5m"` | Check interval |
| `timeout` | string | `"5s"` | Request timeout |
| `tolerance` | int | `0` | Allowed failure count before marking unhealthy |
| `tolerance_ms` | int | `0` | Latency tolerance in milliseconds for url-test groups |

---

## Server Configuration

Top-level structure: `ServerConfig`

| Field | Type | Default | Description |
|---|---|---|---|
| `version` | int | `1` | Config schema version |
| `listen` | string | `"0.0.0.0:443"` | Main listen address |
| `drain_timeout` | string | `"30s"` | Graceful shutdown drain timeout |
| `tls` | TLSConfig | — | TLS certificate settings |
| `auth` | AuthConfig | — | Authentication settings |
| `cover` | CoverSiteConfig | — | Cover website (anti-probing) |
| `transport` | ServerTransportConfig | — | Server-side transport settings |
| `mesh` | ServerMeshConfig | — | Server-side mesh VPN |
| `admin` | AdminConfig | — | Admin API settings |
| `audit` | AuditConfig | — | Audit logging |
| `reputation` | ReputationConfig | — | IP reputation / anti-probing |
| `cluster` | ClusterConfig | — | Multi-instance clustering |
| `max_streams` | int | `1024` | Max concurrent streams per connection |
| `debug` | DebugConfig | — | pprof debug endpoints |
| `allow_private_networks` | bool | `false` | Disable SSRF protection (testing only) |
| `yamux` | YamuxConfig | — | Yamux tuning |
| `log` | LogConfig | — | Logging settings |
| `inbounds` | []InboundConfig | `[]` | Per-protocol inbound listeners |

### TLSConfig

| Field | Type | Description |
|---|---|---|
| `cert_file` | string | Path to TLS certificate PEM file |
| `key_file` | string | Path to TLS private key PEM file |

### AuthConfig

| Field | Type | Description |
|---|---|---|
| `password` | string | Shared password for client authentication |
| `private_key` | string | Server Noise IK private key (base64, Reality) |
| `public_key` | string | Server Noise IK public key (base64, Reality) |

### CoverSiteConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `mode` | string | `"default"` | Cover mode: `static`, `reverse`, `default` |
| `static_dir` | string | `""` | Directory to serve static files from |
| `reverse_url` | string | `""` | Upstream URL to reverse-proxy to |

### AdminConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable admin API |
| `listen` | string | `"127.0.0.1:9090"` | Admin API listen address |
| `token` | string | `""` | Admin bearer token |
| `users` | []User | `[]` | Per-user accounts with traffic quotas |

### User

| Field | Type | Description |
|---|---|---|
| `name` | string | Username |
| `token` | string | Per-user auth token |
| `max_bytes` | int64 | Traffic quota in bytes (0 = unlimited) |
| `enabled` | bool | Whether this user is active |

### AuditConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable connection audit logging |
| `log_dir` | string | `""` | Directory to write audit logs |
| `max_entries` | int | `10000` | Max entries to retain |

### ReputationConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable IP reputation / ban on probe |
| `max_failures` | int | `5` | Auth failures before IP ban |

### ClusterConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable multi-instance clustering |
| `node_name` | string | `""` | This node's name |
| `secret` | string | `""` | Shared cluster secret |
| `peers` | []ClusterPeer | `[]` | Known peer nodes |
| `interval` | string | `"15s"` | Peer sync interval |
| `max_conns` | int64 | `0` | Max total connections across cluster (0 = unlimited) |

### ClusterPeer

| Field | Type | Description |
|---|---|---|
| `name` | string | Peer node name |
| `addr` | string | Peer admin API address |

### DebugConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `pprof_enabled` | bool | `false` | Enable Go pprof HTTP endpoint |
| `pprof_listen` | string | `"127.0.0.1:6060"` | pprof listen address |

### ServerMeshConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable server-side mesh VPN |
| `cidr` | string | `"10.7.0.0/24"` | Virtual IP range for mesh clients |
| `p2p_enabled` | bool | `false` | Enable P2P signaling service |

### ServerTransportConfig

Server-side transport mirrors the client-side structure. Each transport can be individually enabled.

| Field | Type | Description |
|---|---|---|
| `h3` | ServerH3Config | H3 transport |
| `reality` | ServerRealityConfig | Reality transport |
| `cdn` | ServerCDNConfig | CDN transport |
| `webrtc` | ServerWebRTCConfig | WebRTC transport |

#### ServerRealityConfig

| Field | Type | Description |
|---|---|---|
| `enabled` | bool | Enable Reality transport |
| `target_sni` | string | SNI to impersonate |
| `target_addr` | string | Upstream address for cover traffic |
| `short_ids` | []string | Accepted short IDs from clients |
| `post_quantum` | bool | Enable hybrid X25519 + ML-KEM-768 |

#### ServerCDNConfig

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable CDN transport |
| `path` | string | `"/cdn/stream"` | URL path |
| `listen` | string | Same as main | Separate listen address |

---

## Full Client Config Example

```yaml
version: 1

server:
  addr: example.com:443
  name: Main
  password: my-secret

transport:
  preferred: auto
  h3:
    enabled: true
    path_prefix: /shuttle
  reality:
    enabled: true
    server_name: www.apple.com
    public_key: BASE64_KEY_HERE
    short_id: abc123
  cdn:
    enabled: false

proxy:
  allow_lan: false
  socks5:
    enabled: true
    listen: 127.0.0.1:1080
  http:
    enabled: true
    listen: 127.0.0.1:8080
  system_proxy:
    enabled: true

routing:
  default: proxy
  rules:
    - geosite: cn
      action: direct
    - geoip: CN
      action: direct
    - geosite: private
      action: direct
    - geosite: category-ads-all
      action: reject
  dns:
    domestic: 223.5.5.5
    remote:
      server: 8.8.8.8
      via: proxy
    cache: true
    mode: normal

congestion:
  mode: adaptive

log:
  level: info
  format: text
  output: stdout
```

## Full Server Config Example

```yaml
version: 1
listen: 0.0.0.0:443

tls:
  cert_file: /etc/shuttle/cert.pem
  key_file: /etc/shuttle/key.pem

auth:
  password: my-secret
  private_key: BASE64_PRIVATE_KEY
  public_key: BASE64_PUBLIC_KEY

cover:
  mode: reverse
  reverse_url: https://www.apple.com

transport:
  reality:
    enabled: true
    target_sni: www.apple.com
    target_addr: www.apple.com:443
    short_ids:
      - abc123
  h3:
    enabled: true
  cdn:
    enabled: true
    path: /cdn/stream

mesh:
  enabled: false
  cidr: 10.7.0.0/24

admin:
  enabled: true
  listen: 127.0.0.1:9090
  token: admin-secret

reputation:
  enabled: true
  max_failures: 5

log:
  level: info
  format: json
  output: /var/log/shuttled.log
```
