# Shuttle 产品演进路线图

**Date:** 2026-04-05
**Status:** Approved

## 决策记录

| # | 决策点 | 选择 |
|---|--------|------|
| 1 | 订阅格式 | 全部支持：Clash YAML + sing-box JSON + Shuttle 原生 + 自动检测 |
| 2 | 更新策略 | 定时拉取 + CB 触发智能更新，两者都做 |
| 3 | 迁移触发 | 主动探测（新网络并行建连），用户可选开启，默认关闭保持稳定 |
| 4 | 质量评估 | 延迟 + 丢包率（B 方案） |
| 5 | Mesh 定位 | 代理+组网深度融合（C 方案） |
| 6 | GUI 目标 | 简单模式 + 高级模式双模切换（C 方案） |

---

## 子项目拆分（5 个独立交付）

### Sub-1: 订阅系统（追赶 + 创新）
- 多格式解析器（Clash/sing-box/Shuttle/自动检测）
- 订阅拉取 + 定时更新
- CB 触发智能更新（创新）
- 服务器自动测速 + 分组

**依赖**: 多出口路由（已完成）
**工作量**: 4-5 天

### Sub-2: 智能连接迁移（差异化）
- 网络变化检测 → 新网络主动探测 dial
- 探测成功 → Migrator 平滑切换
- 用户配置开关（默认关闭）
- QUIC connection migration 利用

**依赖**: Migrator drain timeout（已完成）、netmon.Monitor（已有）
**工作量**: 3-4 天

### Sub-3: 拥塞感知路由（创新）
- GroupQuality 策略：基于延迟 + 丢包自动切换出口
- 30 秒周期质量评估
- 与 OutboundGroup 集成

**依赖**: OutboundGroup（已完成）、Adaptive CC metrics（已有）
**工作量**: 2-3 天

### Sub-4: Mesh 代理深度融合
- Mesh 流量走路由规则（split_routes 增强）
- 设备自动发现 UI 数据
- mesh outbound：直接路由到 mesh 设备

**依赖**: MeshManager（已完成）、路由规则系统（已完成）
**工作量**: 4-5 天

### Sub-5: GUI 双模重构
- 简单模式：一键连接 + 自动选节点 + 流量统计
- 高级模式：路由规则编辑 + 订阅管理 + 实时诊断
- 对接 Sub-1~4 所有新 API

**依赖**: Sub-1~4 的 API 就绪
**工作量**: 5-7 天

---

## 推荐执行顺序

```
Week 1:  Sub-3 (拥塞感知路由, 2-3d) + Sub-2 (连接迁移, 3-4d) 并行
Week 2:  Sub-1 (订阅系统, 4-5d)
Week 3:  Sub-4 (Mesh 融合, 4-5d)
Week 4:  Sub-5 (GUI 双模, 5-7d)
```

Sub-3 最小（2-3 天）且最具创新性，先出成果。Sub-2 与 Sub-3 无依赖可并行。
Sub-1 是用户获取的基础（没有订阅 = 没有用户），Week 2 必须完成。
Sub-4 和 Sub-5 可以按节奏推进。

---

## Sub-1: 订阅系统 设计

### 架构

```
用户配置 subscription URL
         │
    SubscriptionManager
         │
    ┌────┼────┐
    ▼    ▼    ▼
  Clash  sing  Shuttle    ← 格式解析器 (Parser 接口)
  YAML   box   YAML
  Parser JSON  Parser
         Parser
    └────┬────┘
         ▼
    []ServerNode           ← 统一中间表示
         │
    ┌────┼────────┐
    ▼    ▼        ▼
  测速  分组     生成 Outbound 配置
         │
    写入 cfg.Outbounds + cfg.Routing.RuleChain
         │
    Engine.Reload()
```

### 统一 ServerNode 结构

```go
type ServerNode struct {
    Name       string            // 显示名
    Server     string            // host:port
    Transport  string            // h3, reality, cdn, webrtc
    Settings   map[string]any    // 传输特定参数
    Group      string            // 分组名 (地区、用途)
}
```

### Parser 接口

```go
type SubscriptionParser interface {
    // CanParse returns true if the data looks like this format
    CanParse(data []byte) bool
    // Parse extracts server nodes from subscription data
    Parse(data []byte) ([]ServerNode, error)
}
```

自动检测：依次尝试各 Parser 的 CanParse，第一个返回 true 的执行解析。

### CB 触发智能更新（创新）

```go
// 在 CircuitBreaker.OnStateChange 回调中：
if state == CircuitOpen {
    // 所有出口都打开了 CB → 可能服务器集体下线
    // 触发订阅更新
    go subscriptionManager.ForceUpdate()
}
```

### Clash YAML 格式兼容

需解析的关键字段：
- `proxies[]`: 服务器列表 (ss, vmess, trojan, hysteria2 等)
- `proxy-groups[]`: 分组策略 (select, url-test, fallback, load-balance)
- `rules[]`: 路由规则 (DOMAIN, DOMAIN-SUFFIX, GEOIP, IP-CIDR, MATCH)

映射关系：
- Clash `proxy` → Shuttle `ServerNode` → `OutboundConfig{type:"proxy"}`
- Clash `proxy-group` → Shuttle `OutboundGroup`
- Clash `rule` → Shuttle `RuleChainEntry`

### sing-box JSON 格式兼容

需解析的关键字段：
- `outbounds[]`: 出口列表 (type: shadowsocks, vmess, trojan, hysteria2 等)
- `route.rules[]`: 路由规则

---

## Sub-2: 智能连接迁移 设计

### 架构

```
netmon.Monitor 检测网络变化
         │
         ▼
Engine.onNetworkChange()
         │
    ┌────┴────┐
    │ 开关关闭  │ 开关开启
    │         ▼
    │   ProactiveMigrator
    │         │
    │    在新网络发起探测 dial
    │         │
    │    ┌────┴────┐
    │    ▼         ▼
    │  失败       成功
    │  (保持旧)   │
    │             ▼
    │    Migrator.Migrate()
    │    旧连接 drain
    │    新连接接管
    │         │
    └────┬────┘
         ▼
    emit EventMigration
```

### 配置

```yaml
transport:
  proactive_migration: false  # 默认关闭，用户显式开启
  migration_probe_timeout: 3s # 探测超时
```

### 核心逻辑

```go
type ProactiveMigrator struct {
    sel        *selector.Selector
    serverAddr string
    timeout    time.Duration
    logger     *slog.Logger
}

func (pm *ProactiveMigrator) OnNetworkChange(ctx context.Context) {
    // 在新网络上并行发起探测 dial
    probeCtx, cancel := context.WithTimeout(ctx, pm.timeout)
    defer cancel()
    
    conn, err := pm.sel.ProbeDial(probeCtx, pm.serverAddr)
    if err != nil {
        pm.logger.Info("proactive migration probe failed, keeping current connection", "err", err)
        return
    }
    conn.Close() // 探测成功，关闭探测连接
    
    // 触发正式迁移
    pm.sel.Migrate()
    pm.logger.Info("proactive migration: switched to new network")
}
```

---

## Sub-3: 拥塞感知路由 设计

### GroupQuality 策略

```go
const GroupQuality GroupStrategy = "quality"

type QualityConfig struct {
    MaxLatency    time.Duration `json:"max_latency"`    // e.g., 200ms
    MaxLossRate   float64       `json:"max_loss_rate"`  // e.g., 0.02 (2%)
    ProbeInterval time.Duration `json:"probe_interval"` // e.g., 30s
}
```

### 评估逻辑

```go
func (g *OutboundGroup) qualitySelect(ctx context.Context, network, address string) (net.Conn, error) {
    // 1. 按质量评分排序所有成员
    ranked := g.rankByQuality()
    
    // 2. 尝试最优成员
    for _, member := range ranked {
        conn, err := member.ob.DialContext(ctx, network, address)
        if err == nil {
            return conn, nil
        }
    }
    return nil, fmt.Errorf("all outbounds in group %q failed quality check", g.tag)
}

func (g *OutboundGroup) rankByQuality() []qualityEntry {
    // 从 Selector 的 ProbeResult 获取延迟和丢包数据
    // 按 score = latency_ms + loss_rate * 1000 排序
    // 过滤掉超过 MaxLatency 或 MaxLossRate 的
}
```

### 质量探测

复用 Selector 已有的 `probeLoop`（30 秒周期）。每个 ProxyOutbound 对应一个 transport，其 ProbeResult 包含 Latency 和 Loss。

---

## Sub-4: Mesh 代理深度融合 设计

### Mesh Outbound

创建 `MeshOutbound` 实现 `adapter.Outbound`：

```go
type MeshOutbound struct {
    tag         string
    meshManager *MeshManager
    targetVIP   net.IP  // 目标设备的 mesh 虚拟 IP
}

func (m *MeshOutbound) DialContext(ctx, network, addr) (net.Conn, error) {
    // 通过 mesh tunnel 连接到目标设备
    mc := m.meshManager.Client()
    if mc == nil {
        return nil, fmt.Errorf("mesh not connected")
    }
    return mc.DialPeer(ctx, m.targetVIP, addr)
}
```

### 增强 split_routes

```yaml
mesh:
  enabled: true
  split_routes:
    - cidr: "10.7.0.0/24"
      action: "mesh"          # mesh 内部流量走 mesh 直连
    - cidr: "192.168.1.0/24"
      action: "mesh-relay"    # 通过 mesh 中继访问对端局域网
    - cidr: "0.0.0.0/0"
      action: "proxy"         # 其他流量走代理
```

### 设备发现 API

```
GET /api/mesh/peers → [{vip, name, latency, last_seen, online}]
```

---

## Sub-5: GUI 双模 设计

### 简单模式

```
┌──────────────────────────────┐
│  🟢 已连接 · 东京节点 · 32ms  │
│                              │
│    ↑ 12.3 MB/s  ↓ 45.6 MB/s │
│                              │
│  [一键连接/断开]              │
│                              │
│  ── 今日流量 ──               │
│  上传: 1.2 GB  下载: 3.4 GB  │
│                              │
│  [切换节点 ▾]  [⚙ 高级模式]   │
└──────────────────────────────┘
```

### 高级模式

```
┌─ 侧栏 ──────┐─────────────────────────┐
│ 📊 仪表盘    │                         │
│ 🔗 连接      │   [当前选中面板内容]      │
│ 🌐 路由规则  │                         │
│ 📡 订阅管理  │                         │
│ 🔀 出口管理  │                         │
│ 🕸 Mesh 设备 │                         │
│ 📈 诊断      │                         │
│ ⚙ 设置      │                         │
└──────────────┘─────────────────────────┘
```
