# 从 Clash 迁移

本指南帮助你将已有的 Clash 配置转换为 Shuttle 格式。两者概念高度对应，主要区别在于键名重命名和结构调整。

## 概念对照表

| Clash | Shuttle | 说明 |
|-------|---------|------|
| `proxies:` | `outbounds:` | 概念相同，键名不同 |
| `proxy-groups:` | `outbounds:` 中 `type: "group"` | 代理组与代理节点共用同一个列表 |
| `rules:` | `routing.rule_chain:` | Shuttle 使用结构化 YAML，而非内联字符串 |
| `proxy-providers:` | `proxy_providers:` | 概念相同，改为下划线命名 |
| `rule-providers:` | `rule_providers:` | 支持 domain / ipcidr / classical 三种行为 |
| `dns.fake-ip-range` | `routing.dns.fake_ip_range` | DNS 配置位于 `routing.dns` 下 |
| `tun.enable` | `proxy.tun.enabled` | TUN 配置位于 `proxy.tun` 下 |
| `url-test` 组 | `type: "group"` + `strategy: "url-test"` | 支持 `tolerance_ms` |
| `fallback` 组 | `type: "group"` + `strategy: "failover"` | 行为相同 |
| `select` 组 | `type: "group"` + `strategy: "select"` | 可通过 API 和 GUI 切换 |
| `load-balance` 组 | `type: "group"` + `strategy: "loadbalance"` | 轮询调度 |

## 配置转换示例

### 单个代理节点

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

### url-test 组（含健康检测）

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

### 路由规则

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

    - action: "Auto"   # 兜底规则（无匹配条件）
```

:::

核心区别：Clash 使用逗号分隔的内联字符串（`规则类型,值,动作`），Shuttle 使用带有明确 `match` 和 `action` 字段的结构化 YAML。

## 导入订阅链接

Clash 订阅链接可以直接用于 Shuttle，无需修改：

```yaml
proxy_providers:
  - name: "my-sub"
    url: "https://你的-clash-订阅链接"
    interval: "3600s"
    health_check:
      url: "http://www.gstatic.com/generate_204"
      interval: "300s"
```

Shuttle 自动解析 Clash YAML 格式的订阅。你可以在策略组中引用订阅中的节点：

```yaml
outbounds:
  - tag: "Auto"
    type: "group"
    strategy: "url-test"
    use:
      - "my-sub"   # 使用该 Provider 中的所有节点
```

## DNS 配置

DNS 设置从顶层 `dns:` 块移入 `routing.dns`：

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

## TUN 模式

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

## Shuttle 独有功能（Clash 没有）

- **quality 策略**（`strategy: "quality"`）：基于拥塞感知的智能路由 — 同时考虑延迟和带宽，而不仅仅是 ping 值。
- **自适应拥塞控制**：根据实时丢包率和 RTT，在 BBR、Brutal 和保守模式之间自动切换。
- **Mesh VPN**：基于 STUN 的 P2P NAT 穿透，支持打洞和 TURN 回退，无需外部 VPN 服务器。
- **多路径聚合**：同时通过多个传输通道聚合带宽。
- **后量子加密**：H3 和 Reality 传输协议支持可选的 ML-KEM 密钥交换。
- **策略组嵌套**：组可以引用其他组，不限于叶子节点。

## Clash 有但 Shuttle 暂未支持的功能

- **Script / Starlark 规则**：Clash 支持类 Python 脚本实现自定义路由逻辑。Shuttle 目前仅支持结构化 YAML 规则。
- **TProxy 模式**（`tproxy`）：用于路由器透明代理部署。Shuttle 目前支持 SOCKS5、HTTP 和 TUN 模式。

## 常见问题

**规则格式不是内联字符串。**

Shuttle 不解析 `DOMAIN-SUFFIX,google.com,PROXY` 格式的字符串。每条规则必须是包含 `match` 和 `action` 字段的 YAML 对象。请参考上方的[路由规则示例](#路由规则)。

**DNS 配置位置已变更。**

Clash 中 DNS 是顶层 `dns:` 块，Shuttle 中为 `routing.dns`。如果配置放错位置，会静默使用默认值。

**代理类型名称略有不同。**

| Clash 类型 | Shuttle 类型 |
|------------|-------------|
| `ss` | `shadowsocks` |
| `vmess` | `vmess` |
| `trojan` | `trojan` |
| `vless` | `vless` |
| `hysteria2` | `hysteria2` |
| `wireguard` | `wireguard` |

**`outbounds:` 同时包含代理节点和策略组。**

Clash 将 `proxies:` 和 `proxy-groups:` 分开。Shuttle 中两者都放在 `outbounds:` 里 — 普通代理节点使用协议类型（`shadowsocks`、`vmess` 等），策略组使用 `type: "group"`。

**`interval` 是时间字符串，不是纯数字。**

Clash 使用整数表示时间间隔（如 `interval: 300` 表示 300 秒）。Shuttle 使用 Go 时间字符串：`"300s"`、`"5m"`、`"1h"`。
