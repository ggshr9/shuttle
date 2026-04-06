# Shuttle 生态兼容性设计 — Ecosystem Compatibility Design

**Date**: 2026-04-05
**Status**: Approved
**Scope**: 协议兼容 + 策略组 + Provider + fake-ip + 文档站

## 1. 目标

让竞品（Clash/sing-box/Xray/Surge）的资深用户能低成本迁移到 Shuttle，同时保留 Shuttle 的技术优势（拥塞控制、Mesh VPN、Multipath）作为差异化壁垒。

**核心原则**：先打开大门（兼容主流协议 + 策略组），再用技术优势留人。

## 2. 实施策略

采用**水平分层**策略（策略 B）：先搭基础设施，再批量接协议。

```
Phase 1: 协议适配层 + 策略组框架 + Provider 框架
Phase 2: Shadowsocks + VLESS + Trojan 批量接入
Phase 3: fake-ip DNS
Phase 4: Hysteria2 + VMess + WireGuard
Phase 5: 文档站 + 迁移指南
```

## 3. 协议适配层

### 3.1 问题

现有 transport 接口面向多路复用设计：

```go
type ClientTransport interface {
    Dial(ctx context.Context, addr string) (Connection, error)  // 返回多路复用连接
    Type() string
    Close() error
}
```

SS/VLESS/Trojan/VMess/WireGuard 是 per-request 协议（每次请求独立连接），强行用 yamux 包装会引入不必要的开销和兼容性问题。

### 3.2 Dialer 接口

引入平行的 Dialer 接口，与现有 ClientTransport 共存：

```go
// adapter/dialer.go — 面向 per-request 协议
type Dialer interface {
    DialContext(ctx context.Context, network, address string) (net.Conn, error)
    Type() string
    Close() error
}

// adapter/server_handler.go — 服务端入站
type InboundHandler interface {
    Type() string
    Serve(ctx context.Context, listener net.Listener, handler ConnHandler) error
    Close() error
}

type ConnHandler func(ctx context.Context, conn net.Conn, metadata ConnMetadata)

type ConnMetadata struct {
    Network     string // "tcp" or "udp"
    Destination string // "host:port"
    Source      string
}
```

### 3.3 设计决策

1. **不改现有 ClientTransport 接口** — H3/Reality/CDN/WebRTC 继续用多路复用模型
2. **Dialer 直接返回 net.Conn** — 与 ProxyOutbound.DialContext 签名完美对齐，无需改 outbound 管线
3. **扩展 TransportFactory**：

```go
type TransportFactory interface {
    Type() string
    NewClient(cfg *config.ClientConfig, opts FactoryOptions) (ClientTransport, error)
    NewServer(cfg *config.ServerConfig, opts FactoryOptions) (ServerTransport, error)
    NewDialer(cfg OutboundOptions, opts FactoryOptions) (Dialer, error)           // 新增
    NewInboundHandler(cfg InboundOptions, opts FactoryOptions) (InboundHandler, error)  // 新增
}
```

4. **桥接层**：让 Dialer 也能参与 transport selector 的探测：

```go
// adapter/bridge.go
func DialerAsTransport(d Dialer) ClientTransport
func TransportAsDialer(t ClientTransport, addr string) Dialer
```

### 3.4 数据流

```
原有路径（H3/Reality/CDN）：
  Outbound → engine.dialProxyStream() → Selector → ClientTransport.Dial() → Connection → Stream

新增路径（SS/VLESS/Trojan/...）：
  Outbound → Dialer.DialContext(network, addr) → net.Conn
```

## 4. 策略组框架

### 4.1 统一健康检查

```go
// outbound/healthcheck/checker.go
type HealthChecker struct {
    interval    time.Duration       // 默认 300s
    url         string              // 默认 http://www.gstatic.com/generate_204
    timeout     time.Duration       // 默认 5s
    tolerance   int                 // 连续失败 N 次标记 down，默认 3
    results     map[string]*Result  // tag → 最新结果
    lazy        bool                // 仅使用时才检查
}

type Result struct {
    Latency   time.Duration
    Loss      float64
    Available bool
    UpdatedAt time.Time
}
```

HealthChecker 是应用层 HTTP 检查，覆盖所有 outbound 类型。quality 策略组优先用 HealthChecker 数据，fallback 到 transport probe。

### 4.2 策略类型

```go
type GroupStrategy string
const (
    GroupFailover    GroupStrategy = "failover"      // 已有 — 按顺序试
    GroupLoadBalance GroupStrategy = "loadbalance"    // 已有 — 轮询
    GroupQuality     GroupStrategy = "quality"        // 已有 — 拥塞感知
    GroupURLTest     GroupStrategy = "url-test"       // 新增 — 自动选最快
    GroupSelect      GroupStrategy = "select"         // 新增 — 手动选择
)
```

**url-test**：定期对所有成员跑 HealthChecker，自动选 latency 最低的可用节点。当前节点仍可用时不切换，除非新节点快 tolerance_ms（默认 50ms）以上。

**select**：用户通过 API `PUT /api/groups/{tag}/selected` 切换。持久化到配置文件。GUI 展示为下拉菜单。

### 4.3 策略组嵌套

OutboundGroup 成员是 `[]adapter.Outbound`，group 本身也是 Outbound，嵌套天然支持。初始化时做拓扑排序 + 循环检测：

```go
func validateGroupDAG(groups map[string]*OutboundGroupConfig) error
```

### 4.4 Proxy Provider

```go
// provider/proxy_provider.go
type ProxyProvider struct {
    name       string
    url        string
    path       string              // 本地缓存
    interval   time.Duration
    filter     *regexp.Regexp
    parser     func([]byte) ([]OutboundOptions, error)
    outbounds  []adapter.Outbound
}
```

**自动格式识别**顺序：
1. JSON → sing-box 格式
2. YAML → Clash 格式
3. base64 解码 → URI 列表（ss://、vless://、trojan://）
4. 逐行 URI 解析 → 混合协议列表

策略组通过 `use: ["provider-name"]` 引用 Provider。Provider 刷新后自动更新策略组成员列表。

### 4.5 Rule Provider

```go
// provider/rule_provider.go
type RuleProvider struct {
    name      string
    url       string
    path      string
    interval  time.Duration
    behavior  string              // "domain" | "ipcidr" | "classical"
    rules     []router.CompiledRule
}
```

**behavior 类型**：
- `domain`：纯域名列表
- `ipcidr`：纯 IP-CIDR 列表
- `classical`：完整规则（`DOMAIN-SUFFIX,example.com,PROXY`）

热更新：刷新后原子替换 router 中的 provider 规则段。

### 4.6 配置格式

```yaml
outbounds:
  - tag: "auto-select"
    type: "group"
    strategy: "url-test"
    outbounds: ["hk-01", "hk-02", "jp-01"]
    use: ["my-provider"]
    health_check:
      url: "http://www.gstatic.com/generate_204"
      interval: "300s"
      tolerance_ms: 50

  - tag: "manual"
    type: "group"
    strategy: "select"
    outbounds: ["auto-select", "hk-01", "direct"]

proxy_providers:
  - name: "my-provider"
    url: "https://sub.example.com/clash"
    interval: "3600s"
    filter: "(?i)hong kong|hk"
    health_check:
      url: "http://www.gstatic.com/generate_204"
      interval: "300s"

rule_providers:
  - name: "gfw-list"
    url: "https://cdn.example.com/gfw.txt"
    behavior: "domain"
    interval: "86400s"

routing:
  rule_chain:
    - match: { rule_provider: ["gfw-list"] }
      action: "auto-select"
```

### 4.7 API 扩展

| 端点 | 方法 | 用途 |
|------|------|------|
| `/api/groups` | GET | 列出所有策略组及其状态 |
| `/api/groups/{tag}` | GET | 获取策略组详情（成员、当前选中、延迟） |
| `/api/groups/{tag}/selected` | PUT | select 组手动切换节点 |
| `/api/groups/{tag}/test` | POST | 触发健康检查 |
| `/api/providers/proxy` | GET | 列出所有 Proxy Provider |
| `/api/providers/proxy/{name}/refresh` | POST | 手动刷新 Provider |
| `/api/providers/rule` | GET | 列出所有 Rule Provider |
| `/api/providers/rule/{name}/refresh` | POST | 手动刷新 Rule Provider |

## 5. 协议实现

### 5.1 Shadowsocks（客户端 + 服务端）

**加密方法**：
- SS 2022：`2022-blake3-aes-128-gcm`、`2022-blake3-aes-256-gcm`（优先）
- 经典 AEAD：`aes-128-gcm`、`aes-256-gcm`、`chacha20-ietf-poly1305`
- 不支持 stream cipher（已废弃，安全风险）

**客户端 Dialer**：TCP 连接 → AEAD 加密握手 → SOCKS-like 地址头 → 返回加密 net.Conn

**服务端 InboundHandler**：Accept → AEAD 解密 → 解析目标地址 → 回调 ConnHandler

**UDP 中继**：独立的 UDPRelay 组件处理 SS 的 UDP 通道。

**依赖**：
- 经典 AEAD：`github.com/shadowsocks/go-shadowsocks2`
- SS 2022：`github.com/sagernet/sing-shadowsocks2`

### 5.2 VLESS（客户端 + 服务端）

**特征**：无加密层（依赖外层 TLS/Reality），UUID 认证，极简头部。

**客户端 Dialer**：TLS 连接 → VLESS request header `[version(1)][uuid(16)][addon_len(1)][addon][cmd(1)][addr]` → 读 response → 返回 net.Conn

**服务端 InboundHandler**：TLS listener 在外层 → 读 header → 验证 UUID → 解析目标 → 回调 ConnHandler

**XTLS Vision**：不纳入第一版。Shuttle 的 Reality 传输已解决双重加密问题，后续按需添加。

**TLS 复用**：VLESS 和 Trojan 的外层 TLS 配置共享一套 TLS/Reality 基础设施，避免重复实现。

### 5.3 Trojan（客户端 + 服务端）

**特征**：SHA224(password) 认证，极简协议。

**客户端 Dialer**：TLS 连接 → 发送 `SHA224(password) + CRLF + cmd + addr + CRLF` → 返回 net.Conn

**服务端 InboundHandler**：认证失败时回落到 cover site（复用 Shuttle 已有 cover site 功能）。

### 5.4 Hysteria2（第二批，客户端 + 服务端）

基于 QUIC，自带多路复用 + Brutal CC。

**关键复用**：`quicfork/` 已有 Brutal CC hook，只需实现 Hysteria2 的 HTTP/3 认证握手和流协议。

### 5.5 VMess（第二批，客户端 + 服务端）

V2Ray 原始协议。**只支持 AEAD 模式**（alterId=0），不支持 legacy（已知安全漏洞）。

依赖：`github.com/v2fly/v2ray-core` VMess 实现。

### 5.6 WireGuard（第二批，仅客户端出站）

用户态实现（`wireguard-go` + gVisor netstack），不依赖内核模块。跨平台，不需要 root。

**不做 WireGuard 入站**：Shuttle 的 Mesh VPN 已覆盖组网场景，WG 仅作为出站串联。

### 5.7 配置格式

**客户端出站**：

```yaml
outbounds:
  - tag: "ss-hk"
    type: "shadowsocks"
    server: "hk.example.com:8388"
    method: "2022-blake3-aes-256-gcm"
    password: "base64-key-here"

  - tag: "vless-jp"
    type: "vless"
    server: "jp.example.com:443"
    uuid: "xxx-xxx-xxx"
    tls:
      enabled: true
      server_name: "jp.example.com"
      reality:
        public_key: "xxx"
        short_id: "xxx"

  - tag: "trojan-us"
    type: "trojan"
    server: "us.example.com:443"
    password: "my-password"
    tls:
      server_name: "us.example.com"
```

**服务端入站**：

```yaml
inbounds:
  - tag: "ss-in"
    type: "shadowsocks"
    listen: ":8388"
    method: "2022-blake3-aes-256-gcm"
    password: "base64-key-here"

  - tag: "vless-in"
    type: "vless"
    listen: ":443"
    users:
      - uuid: "xxx-xxx-xxx"
        tag: "user-1"
    tls:
      cert_file: "/path/to/cert.pem"
      key_file: "/path/to/key.pem"
    fallback: { server: "127.0.0.1:8080" }

  - tag: "trojan-in"
    type: "trojan"
    listen: ":443"
    users:
      - password: "my-password"
        tag: "user-1"
    tls:
      cert_file: "/path/to/cert.pem"
      key_file: "/path/to/key.pem"
    fallback: { server: "127.0.0.1:8080" }
```

### 5.8 URI 解析

每个协议实现 URI 解析器，注册到统一入口：

```
ss://method:password@host:port#name
vless://uuid@host:port?type=tcp&security=tls&sni=xxx#name
trojan://password@host:port?sni=xxx#name
hysteria2://password@host:port?sni=xxx#name
vmess://base64-json
```

统一入口：`subscription.ParseURI(uri string) (*OutboundOptions, error)`

## 6. fake-ip DNS

### 6.1 核心组件

```go
// router/dns/fakeip/pool.go
type Pool struct {
    cidr    netip.Prefix           // 默认 198.18.0.0/15（131k 地址）
    domain  map[netip.Addr]string  // fakeIP → domain
    reverse map[string]netip.Addr  // domain → fakeIP
    next    netip.Addr
    persist *Store                 // 可选持久化
    filter  *DomainFilter          // fake-ip-filter 白名单
    mu      sync.RWMutex
}

func (p *Pool) Lookup(domain string) netip.Addr     // 域名 → fake IP
func (p *Pool) Reverse(ip netip.Addr) (string, bool) // fake IP → 域名
func (p *Pool) IsFakeIP(ip netip.Addr) bool
```

### 6.2 集成点

1. **DNS Resolver**：fake-ip 模式开启时，Resolve(domain) 返回 fake IP
2. **Router**：检查目标 IP 是否在 fake-ip 段，是则反查域名做路由
3. **Outbound**：代理出站前用真实 DNS 替换 fake IP，直连出站直接解析域名

### 6.3 配置

```yaml
dns:
  mode: "fake-ip"                 # "normal" | "fake-ip"
  fake_ip_range: "198.18.0.0/15"
  fake_ip_filter:
    - "stun.*"
    - "+.lan"
    - "+.local"
    - "time.*.com"
    - "ntp.*"
  persist: true
```

`mode: "normal"` 时行为完全不变，不影响现有用户。

## 7. 文档站

### 7.1 技术选型

VitePress 静态站 → GitHub Pages。与项目 Vite 生态一致。

### 7.2 结构

```
docs/site/
├── .vitepress/config.ts
├── en/
│   ├── guide/
│   │   ├── getting-started.md
│   │   ├── configuration.md
│   │   └── migrate-from-clash.md
│   ├── protocols/
│   │   ├── shadowsocks.md
│   │   ├── vless.md
│   │   ├── trojan.md
│   │   ├── hysteria2.md
│   │   ├── vmess.md
│   │   ├── wireguard.md
│   │   ├── h3.md
│   │   ├── reality.md
│   │   └── cdn.md
│   ├── features/
│   │   ├── proxy-groups.md
│   │   ├── providers.md
│   │   ├── fake-ip.md
│   │   ├── mesh-vpn.md
│   │   ├── congestion-control.md
│   │   └── multipath.md
│   └── api/
│       └── rest-api.md
└── zh/                           # 中文，同结构
```

### 7.3 迁移指南内容

1. **概念映射表** — Clash 术语 → Shuttle 术语
2. **配置转换示例** — 左右对照 Clash vs Shuttle YAML
3. **订阅导入** — Clash 订阅 URL 直接可用（已有 parser）
4. **功能差异说明** — 有对应 / 没有 / Shuttle 独有
5. **常见问题** — DNS、TUN、规则语法差异

### 7.4 协议文档模板

每页包含：概述、客户端配置（完整 YAML + 字段说明）、服务端配置、URI 格式、与其他工具的兼容性对照。

## 8. Phase 总览

| Phase | 内容 | 可交付物 |
|-------|------|---------|
| **1** | 协议适配层（Dialer/InboundHandler）+ 策略组框架（url-test/select/HealthChecker）+ Provider 框架 | 基础设施就绪，可用现有协议验证 |
| **2** | Shadowsocks + VLESS + Trojan（客户端+服务端+URI 解析+订阅兼容） | 覆盖 80% 用户可迁移 |
| **3** | fake-ip DNS 模式 | TUN 重度用户可迁移 |
| **4** | Hysteria2 + VMess + WireGuard | 覆盖长尾用户 |
| **5** | VitePress 文档站 + Clash 迁移指南 + 协议文档 + API 参考 | 新用户有入口 |

## 9. 不做的事（明确排除）

- **Clash 配置直接导入**：不做格式兼容层，避免被 Clash 设计绑架
- **Stream cipher**：SS 的过时加密方式，安全风险，不支持
- **Legacy VMess**（alterId > 0）：已知漏洞，只支持 AEAD
- **XTLS Vision**：第一版不做，Shuttle Reality 已解决同类问题
- **WireGuard 入站**：Mesh VPN 已覆盖组网场景
- **Lua/JS 脚本插件**：不在本次范围
- **完整文档站**（教程、调优、FAQ、社区模板）：初期做基础版，随社区增长补充
