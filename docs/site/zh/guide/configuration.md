# 配置参考

Shuttle 使用 YAML 配置文件。客户端配置从 `config.yaml` 加载（或通过 `--config` 指定路径）；服务端配置默认为 `server.yaml`。

两个文件均支持热重载：发送 `SIGHUP` 信号或调用 `PUT /api/config` 即可在不重启的情况下应用更改。

---

## 客户端配置

顶层结构：`ClientConfig`

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `version` | int | `1` | 配置模式版本 |
| `server` | ServerEndpoint | — | 当前活跃服务器 |
| `servers` | []ServerEndpoint | `[]` | 已保存的服务器列表 |
| `subscriptions` | []SubscriptionConfig | `[]` | 远程节点订阅源 |
| `transport` | TransportConfig | — | 传输协议设置 |
| `proxy` | ProxyConfig | — | 本地代理监听器 |
| `routing` | RoutingConfig | — | 流量路由规则 |
| `qos` | QoSConfig | — | 服务质量标记 |
| `congestion` | CongestionConfig | — | 拥塞控制算法 |
| `retry` | RetryConfig | — | 连接重试及退避 |
| `mesh` | MeshConfig | — | Mesh VPN 设置 |
| `obfs` | ObfsConfig | — | 流量混淆 |
| `yamux` | YamuxConfig | — | Yamux 多路复用调参 |
| `log` | LogConfig | — | 日志设置 |
| `inbounds` | []InboundConfig | `[]` | 可插拔入站监听器 |
| `outbounds` | []OutboundConfig | `[]` | 可插拔出站拨号器 |
| `proxy_providers` | []ProxyProviderConfig | `[]` | 远程代理 Provider 源 |
| `rule_providers` | []RuleProviderConfig | `[]` | 远程规则 Provider 源 |

### ServerEndpoint

定义远程 Shuttle 服务器。

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `addr` | string | — | 服务器地址，如 `example.com:443` |
| `name` | string | `""` | 可读标签 |
| `password` | string | `""` | 认证共享密钥 |
| `sni` | string | `""` | TLS SNI 覆盖 |

**示例：**
```yaml
server:
  addr: example.com:443
  name: 主节点
  password: my-secret
  sni: ""
```

### SubscriptionConfig

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 唯一订阅 ID（自动生成） |
| `name` | string | 可读标签 |
| `url` | string | 订阅链接（HTTP/HTTPS） |

---

## 传输配置

`TransportConfig` — 控制可用传输协议及其选择方式。

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `preferred` | string | `"auto"` | 首选传输：`h3`、`reality`、`cdn`、`webrtc`、`auto`、`multipath` |
| `multipath_schedule` | string | `"weighted"` | 多路径调度器：`weighted`、`min-latency`、`load-balance` |
| `warm_up_conns` | int | `0` | 启动时预建连接数，消除冷启动延迟 |
| `pool_max_idle` | int | `4` | 每个传输的最大空闲连接数 |
| `pool_idle_ttl` | string | `"60s"` | 空闲连接存活时间 |
| `keepalive_interval` | string | `"15s"` | 传输层 keepalive 间隔 |
| `keepalive_timeout` | string | `"5s"` | keepalive 响应超时 |
| `proactive_migration` | bool | `false` | 零停机网络切换 |
| `migration_probe_timeout` | string | `"3s"` | 主动迁移探测超时 |
| `h3` | H3Config | — | HTTP/3 设置 |
| `reality` | RealityConfig | — | Reality 传输设置 |
| `cdn` | CDNConfig | — | CDN 传输设置 |
| `webrtc` | WebRTCConfig | — | WebRTC DataChannel 设置 |

### H3Config（HTTP/3 over QUIC）

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `true` | 启用 H3 传输 |
| `path_prefix` | string | `"/shuttle"` | H3 流的 URL 路径前缀 |
| `insecure_skip_verify` | bool | `false` | 跳过 TLS 证书验证 |
| `multipath` | MultipathConfig | — | QUIC 多路径设置 |

### MultipathConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 启用 QUIC 多路径 |
| `interfaces` | []string | `[]` | 绑定特定网络接口（空 = 自动检测） |
| `mode` | string | `"aggregate"` | `redundant`、`aggregate`、`failover` |
| `probe_interval` | string | `"5s"` | 路径探测间隔 |

### RealityConfig（TLS + Noise IK）

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `true` | 启用 Reality 传输 |
| `server_name` | string | `""` | Reality 握手伪装 SNI |
| `short_id` | string | `""` | 客户端标识 Short ID |
| `public_key` | string | `""` | 服务端 Noise IK 公钥（base64） |
| `post_quantum` | bool | `false` | 启用 X25519 + ML-KEM-768 混合密钥交换 |

### CDNConfig（HTTP/2 + gRPC）

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 启用 CDN 传输 |
| `domain` | string | `""` | CDN 域名（实际目标） |
| `path` | string | `"/cdn/stream"` | gRPC/H2 路径 |
| `mode` | string | `"grpc"` | `h2` 或 `grpc` |
| `front_domain` | string | `""` | 域前置 SNI 域名 |
| `insecure_skip_verify` | bool | `false` | 跳过 TLS 验证 |

### WebRTCConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 启用 WebRTC DataChannel 传输 |
| `signal_url` | string | `""` | 信令服务器 WebSocket URL |
| `stun_servers` | []string | `[]` | STUN 服务器 URL 列表 |
| `turn_servers` | []string | `[]` | TURN 服务器 URL 列表 |
| `turn_user` | string | `""` | TURN 用户名 |
| `turn_pass` | string | `""` | TURN 密码 |
| `ice_policy` | string | `"all"` | ICE 候选策略：`all`、`relay`、`public` |

---

## 代理配置

`ProxyConfig` — 本地监听器设置。

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `allow_lan` | bool | `false` | 允许局域网内其他设备使用此代理 |
| `socks5` | SOCKS5Config | — | SOCKS5 监听器 |
| `http` | HTTPConfig | — | HTTP CONNECT 监听器 |
| `tun` | TUNConfig | — | TUN 设备（透明代理） |
| `system_proxy` | SystemProxyConfig | — | 自动系统代理 |

### SOCKS5Config

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `true` | 启用 SOCKS5 监听器 |
| `listen` | string | `"127.0.0.1:1080"` | 监听地址 |

### HTTPConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `true` | 启用 HTTP CONNECT 监听器 |
| `listen` | string | `"127.0.0.1:8080"` | 监听地址 |

### TUNConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 启用 TUN 设备 |
| `device_name` | string | `"shuttle0"` | TUN 设备名称 |
| `cidr` | string | `"198.18.0.0/15"` | TUN 设备 CIDR |
| `mtu` | int | `1500` | MTU 大小 |
| `auto_route` | bool | `false` | 自动为 TUN 添加路由 |
| `per_app_mode` | string | `""` | 按应用路由：`allow`、`deny` 或 `""`（禁用） |
| `app_list` | []string | `[]` | 应用包名 / Bundle ID 列表 |

### SystemProxyConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 连接时自动设置系统代理 |

---

## 路由配置

`RoutingConfig` — 流量路由规则和 DNS 设置。

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `rules` | []RouteRule | `[]` | 有序路由规则列表 |
| `rule_chain` | []RuleChainEntry | `[]` | 高级多条件规则链（优先匹配） |
| `default` | string | `"proxy"` | 无规则匹配时的默认动作：`proxy`、`direct`、`reject` |
| `dns` | DNSConfig | — | DNS 解析器设置 |
| `geodata` | GeoDataConfig | — | GeoIP/GeoSite 数据管理 |

### RouteRule

每条规则匹配一个条件并指定动作，第一条匹配的规则生效。

| 字段 | 类型 | 说明 |
|---|---|---|
| `domains` | string | 域名匹配（后缀） |
| `geosite` | string | GeoSite 分类，如 `cn`、`google`、`category-ads-all` |
| `geoip` | string | GeoIP 国家代码，如 `CN`、`US` |
| `ip_cidr` | []string | IP CIDR 范围列表 |
| `process` | []string | 进程名（应用级路由） |
| `protocol` | string | 网络协议：`tcp`、`udp` |
| `network_type` | string | 网络类型：`wifi`、`cellular`、`ethernet` |
| `action` | string | **必填。** `proxy`、`direct`、`reject` |

**示例：**
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

支持多个匹配条件组合的高级规则。

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `match` | RuleMatch | — | 匹配条件 |
| `logic` | string | `"and"` | 条件组合方式：`and`、`or` |
| `action` | string | — | `proxy`、`direct`、`reject` |

### RuleMatch 字段

| 字段 | 类型 | 说明 |
|---|---|---|
| `domain` | []string | 精确域名匹配 |
| `domain_suffix` | []string | 域名后缀匹配 |
| `domain_keyword` | []string | 域名关键字匹配 |
| `geosite` | []string | GeoSite 分类 |
| `ip_cidr` | []string | IP CIDR 范围 |
| `geoip` | []string | GeoIP 国家代码 |
| `process` | []string | 进程名 |
| `protocol` | []string | 协议 |
| `network_type` | []string | 网络类型 |
| `rule_provider` | []string | 规则 Provider 名称 |

### DNSConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `domestic` | string | `"223.5.5.5"` | 国内 DNS 服务器（UDP/TCP） |
| `remote.server` | string | `"8.8.8.8"` | 远程 DNS 服务器 |
| `remote.via` | string | `"proxy"` | 远程 DNS 路由：`proxy` 或 `direct` |
| `cache` | bool | `true` | 启用 DNS 缓存 |
| `prefetch` | bool | `false` | 预取热门 DNS 记录 |
| `leak_prevention` | bool | `false` | 强制所有 DNS 走代理 |
| `domestic_doh` | string | `""` | 国内 DoH URL，如 `https://dns.alidns.com/dns-query` |
| `strip_ecs` | bool | `false` | 去除响应中的 EDNS Client Subnet |
| `persistent_conn` | bool | `true` | DoH 使用持久 HTTP/2 连接 |
| `mode` | string | `"normal"` | DNS 模式：`normal` 或 `fake-ip` |
| `fake_ip_range` | string | `"198.18.0.0/15"` | fake-ip 映射 CIDR 池 |
| `fake_ip_filter` | []string | `[]` | 跳过 fake-ip、使用真实 DNS 的域名 |
| `persist` | bool | `false` | 跨重启持久化 fake-ip 映射 |

### GeoDataConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `true` | 启用 GeoIP/GeoSite 数据 |
| `data_dir` | string | 系统缓存目录 | 地理数据库文件目录 |
| `auto_update` | bool | `true` | 自动更新地理数据库 |
| `update_interval` | string | `"24h"` | 更新检查间隔 |
| `direct_list_url` | string | Loyalsoldier 上游 | 直连列表 URL 覆盖 |
| `proxy_list_url` | string | Loyalsoldier 上游 | 代理列表 URL 覆盖 |
| `reject_list_url` | string | Loyalsoldier 上游 | 拦截列表 URL 覆盖 |
| `gfw_list_url` | string | Loyalsoldier 上游 | GFW 列表 URL 覆盖 |
| `cn_cidr_url` | string | Loyalsoldier 上游 | CN CIDR 列表 URL 覆盖 |
| `private_cidr_url` | string | Loyalsoldier 上游 | 私有 CIDR 列表 URL 覆盖 |

---

## 拥塞控制

`CongestionConfig`

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `mode` | string | `"adaptive"` | 算法：`adaptive`、`bbr`、`brutal` |
| `brutal_rate` | uint64 | `104857600` | Brutal 模式目标发送速率（字节/秒，默认 100 MB/s） |

- **adaptive** — 根据丢包率和 RTT 自动在 BBR 和 Brutal 之间切换
- **bbr** — 基于带宽的拥塞控制（适合干净网络）
- **brutal** — 固定速率发送（在主动干扰下有效）

---

## 重试配置

`RetryConfig`

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `max_attempts` | int | `3` | 最大重试次数 |
| `initial_backoff` | string | `"100ms"` | 初始重试延迟 |
| `max_backoff` | string | `"5s"` | 最大重试延迟 |

---

## Mesh VPN

`MeshConfig`

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 启用 Mesh VPN |
| `p2p_enabled` | bool | `false` | 启用 P2P NAT 穿透 |
| `p2p` | P2PConfig | — | P2P 穿透设置 |
| `split_routes` | []SplitRoute | `[]` | 按子网的路由策略覆盖 |

### P2PConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `stun_servers` | []string | `[]` | STUN 服务器 URL（用于 NAT 检测） |
| `hole_punch_timeout` | string | `"10s"` | 打洞超时时间 |
| `direct_retry_interval` | string | `"60s"` | 直连失败后重试间隔 |
| `keep_alive_interval` | string | `"30s"` | P2P 连接保活间隔 |
| `fallback_threshold` | float64 | `0.3` | 触发回退中继的丢包率阈值 |
| `spoof_mode` | string | `"none"` | 端口伪装：`none`、`dns`(53)、`https`(443)、`ike`(500) |
| `spoof_port` | int | `0` | 自定义伪装端口 |
| `disable_upnp` | bool | `false` | 禁用 UPnP/NAT-PMP 自动检测 |
| `preferred_port` | int | `0` | 首选外部端口（0 = 与本地端口相同） |

### SplitRoute

| 字段 | 类型 | 说明 |
|---|---|---|
| `cidr` | string | 子网，如 `10.7.0.128/25` |
| `action` | string | `mesh`、`direct` 或 `proxy` |

---

## 流量混淆

`ObfsConfig`

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `padding_enabled` | bool | `false` | 为数据包添加随机填充 |
| `shaping_enabled` | bool | `false` | 为流量添加时间抖动 |
| `min_delay` | string | `"0s"` | 最小包间延迟 |
| `max_delay` | string | `"50ms"` | 最大包间延迟 |
| `chunk_size` | int | `64` | 数据包分割的最小块大小 |

---

## 服务质量（QoS）

`QoSConfig`

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 启用 QoS DSCP 标记 |
| `rules` | []QoSRule | `[]` | 流量分类规则 |

### QoSRule

| 字段 | 类型 | 说明 |
|---|---|---|
| `ports` | []uint16 | 目标端口号 |
| `protocol` | string | `tcp` 或 `udp` |
| `domains` | []string | 域名匹配模式 |
| `process` | []string | 进程名 |
| `priority` | string | 优先级：`critical`、`high`、`normal`、`bulk`、`low` |

优先级与 DSCP 映射：`critical`=EF(46)、`high`=AF41(34)、`normal`=AF21(18)、`bulk`=AF11(10)、`low`=BE(0)。

---

## Yamux 多路复用器

`YamuxConfig`

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `max_stream_window_size` | uint32 | `262144` | 最大流窗口大小（字节，256 KB） |
| `keep_alive_interval` | int | `30` | 保活间隔（秒） |
| `connection_write_timeout` | int | `10` | 写超时（秒） |

---

## 日志配置

`LogConfig`

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `level` | string | `"info"` | 日志级别：`debug`、`info`、`warn`、`error` |
| `format` | string | `"text"` | 日志格式：`text` 或 `json` |
| `output` | string | `"stdout"` | 输出目标：`stdout`、`stderr` 或文件路径 |

---

## 入站与出站

### InboundConfig

可插拔入站监听器（Shadowsocks、VLESS、Trojan 等）。

| 字段 | 类型 | 说明 |
|---|---|---|
| `tag` | string | 此入站的唯一标识符 |
| `type` | string | 协议类型：`shadowsocks`、`vless`、`trojan` 等 |
| `listen` | string | 监听地址，如 `0.0.0.0:1234` |
| `options` | object | 协议特定选项（JSON/YAML 对象） |

### OutboundConfig

可插拔出站拨号器和策略组。

| 字段 | 类型 | 说明 |
|---|---|---|
| `tag` | string | 唯一标识符 |
| `type` | string | 类型：`direct`、`reject`、`shadowsocks`、`vless`、`trojan`、`urltest`、`fallback`、`select`、`loadbalance`、`quality` |
| `options` | object | 协议/策略特定选项 |
| `use` | []string | 从中拉取成员的代理 Provider 名称列表（用于策略组） |
| `health_check` | HealthCheckConfig | 健康检测设置 |

---

## Provider

### ProxyProviderConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `name` | string | — | 唯一 Provider 名称 |
| `url` | string | — | Provider 订阅链接 |
| `path` | string | `""` | 本地缓存文件路径 |
| `interval` | string | `"1h"` | 自动刷新间隔 |
| `filter` | string | `""` | 节点名称过滤正则表达式 |
| `health_check` | HealthCheckConfig | — | 健康检测配置 |

### RuleProviderConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `name` | string | — | 唯一 Provider 名称 |
| `url` | string | — | 规则列表链接 |
| `path` | string | `""` | 本地缓存文件路径 |
| `behavior` | string | — | 规则行为：`domain`、`ipcidr`、`classical` |
| `interval` | string | `"24h"` | 自动刷新间隔 |

### HealthCheckConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `url` | string | `"http://www.gstatic.com/generate_204"` | 健康检测 URL |
| `interval` | string | `"5m"` | 检测间隔 |
| `timeout` | string | `"5s"` | 请求超时 |
| `tolerance` | int | `0` | 允许的失败次数 |
| `tolerance_ms` | int | `0` | url-test 策略组的延迟容差（毫秒） |

---

## 服务端配置

顶层结构：`ServerConfig`

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `version` | int | `1` | 配置模式版本 |
| `listen` | string | `"0.0.0.0:443"` | 主监听地址 |
| `drain_timeout` | string | `"30s"` | 优雅停机排空超时 |
| `tls` | TLSConfig | — | TLS 证书设置 |
| `auth` | AuthConfig | — | 认证设置 |
| `cover` | CoverSiteConfig | — | 伪装站点（防探测） |
| `transport` | ServerTransportConfig | — | 服务端传输设置 |
| `mesh` | ServerMeshConfig | — | 服务端 Mesh VPN |
| `admin` | AdminConfig | — | 管理 API |
| `audit` | AuditConfig | — | 审计日志 |
| `reputation` | ReputationConfig | — | IP 信誉 / 防探测 |
| `cluster` | ClusterConfig | — | 多实例集群 |
| `max_streams` | int | `1024` | 每连接最大并发流数 |
| `debug` | DebugConfig | — | pprof 调试端点 |
| `allow_private_networks` | bool | `false` | 禁用 SSRF 防护（仅测试用） |
| `yamux` | YamuxConfig | — | Yamux 调参 |
| `log` | LogConfig | — | 日志设置 |
| `inbounds` | []InboundConfig | `[]` | 协议入站监听器 |

### TLSConfig

| 字段 | 类型 | 说明 |
|---|---|---|
| `cert_file` | string | TLS 证书 PEM 文件路径 |
| `key_file` | string | TLS 私钥 PEM 文件路径 |

### AuthConfig

| 字段 | 类型 | 说明 |
|---|---|---|
| `password` | string | 客户端认证共享密码 |
| `private_key` | string | 服务端 Noise IK 私钥（base64，Reality） |
| `public_key` | string | 服务端 Noise IK 公钥（base64，Reality） |

### CoverSiteConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `mode` | string | `"default"` | 伪装模式：`static`、`reverse`、`default` |
| `static_dir` | string | `""` | 静态文件目录 |
| `reverse_url` | string | `""` | 反向代理目标 URL |

### AdminConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 启用管理 API |
| `listen` | string | `"127.0.0.1:9090"` | 管理 API 监听地址 |
| `token` | string | `""` | 管理员 Bearer Token |
| `users` | []User | `[]` | 含流量配额的用户账户列表 |

### User

| 字段 | 类型 | 说明 |
|---|---|---|
| `name` | string | 用户名 |
| `token` | string | 用户级认证 Token |
| `max_bytes` | int64 | 流量配额（字节，0 = 无限制） |
| `enabled` | bool | 此用户是否启用 |

### AuditConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 启用连接审计日志 |
| `log_dir` | string | `""` | 审计日志目录 |
| `max_entries` | int | `10000` | 最多保留条目数 |

### ReputationConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 启用 IP 信誉 / 探测封禁 |
| `max_failures` | int | `5` | IP 封禁前允许的认证失败次数 |

### ClusterConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 启用多实例集群 |
| `node_name` | string | `""` | 当前节点名称 |
| `secret` | string | `""` | 集群共享密钥 |
| `peers` | []ClusterPeer | `[]` | 已知对等节点列表 |
| `interval` | string | `"15s"` | 节点同步间隔 |
| `max_conns` | int64 | `0` | 集群最大总连接数（0 = 无限制） |

### ClusterPeer

| 字段 | 类型 | 说明 |
|---|---|---|
| `name` | string | 对等节点名称 |
| `addr` | string | 对等节点管理 API 地址 |

### DebugConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `pprof_enabled` | bool | `false` | 启用 Go pprof HTTP 端点 |
| `pprof_listen` | string | `"127.0.0.1:6060"` | pprof 监听地址 |

### ServerMeshConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 启用服务端 Mesh VPN |
| `cidr` | string | `"10.7.0.0/24"` | Mesh 客户端虚拟 IP 范围 |
| `p2p_enabled` | bool | `false` | 启用 P2P 信令服务 |

### ServerTransportConfig

服务端传输配置与客户端结构对应，每个传输可单独启用。

#### ServerRealityConfig

| 字段 | 类型 | 说明 |
|---|---|---|
| `enabled` | bool | 启用 Reality 传输 |
| `target_sni` | string | 伪装 SNI |
| `target_addr` | string | 伪装流量上游地址 |
| `short_ids` | []string | 接受的客户端 Short ID 列表 |
| `post_quantum` | bool | 启用 X25519 + ML-KEM-768 混合密钥交换 |

#### ServerCDNConfig

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 启用 CDN 传输 |
| `path` | string | `"/cdn/stream"` | URL 路径 |
| `listen` | string | 同主监听地址 | 独立监听地址 |

---

## 完整客户端配置示例

```yaml
version: 1

server:
  addr: example.com:443
  name: 主节点
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

## 完整服务端配置示例

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
