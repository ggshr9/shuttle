# Shuttle 后续演进路线图

> **排序原则**: 低成本高价值优先，每个 Phase 产出可独立交付的功能，后续 Phase 在前者基础上增值。

---

## Phase 1: 可观测性补全（1-2 天）

**目标**: 让 retry/CB/迁移行为对用户和监控系统可见。

### Task 1.1: ResilientOutbound 事件发射

**问题**: ResilientOutbound 的 retry 尝试和 circuit breaker 状态变更是静默的，用户看不到重试过程。

**方案**:
- ResilientOutbound.DialContext 在每次 retry 时通过回调发射事件
- CircuitBreaker 状态变更（Open/HalfOpen/Closed）发射到 EventBus
- 事件类型: `EventRetry{Attempt, Delay, Error}`, `EventCircuitState{State, Cooldown}`

**文件**:
- 修改: `engine/outbound_middleware.go` — 添加 `OnRetry`/`OnCircuitChange` 回调
- 修改: `engine/engine_inbound.go` — 构造时注入 `e.obs.Emit` 回调
- 修改: `engine/events.go` — 添加 EventRetry/EventCircuitState 类型
- 测试: `engine/outbound_middleware_test.go` — 验证事件发射

**工作量**: 0.5 天

### Task 1.2: 客户端 Prometheus Metrics 端点

**问题**: 服务端已有 `/metrics` Prometheus 端点，客户端没有。

**方案**:
- 复用 `server/admin/prometheus.go` 的格式化逻辑
- 在 GUI API 添加 `GET /api/prometheus` 端点
- 暴露: active_conns, total_conns, bytes_sent/received, upload/download_speed, circuit_breaker_state, retry_count

**文件**:
- 创建: `gui/api/routes_prometheus.go` — Prometheus 格式化端点
- 修改: `gui/api/api.go` — 注册路由
- 测试: `gui/api/routes_prometheus_test.go`

**工作量**: 0.5 天

### Task 1.3: 连接迁移完善

**问题**: Migrator 已实现 95%，缺少排水超时和可观测性。

**现状**: `transport/selector/migrate.go` 有完整的 Track/Migrate/Drain 逻辑，5 秒循环检查，但无最大超时。

**方案**:
- 添加 `DrainTimeout` 配置（默认 30s），超时后强制关闭
- 发射迁移事件: `EventMigration{OldTransport, NewTransport, DrainingConns, MigratedConns}`
- 暴露到 `/api/status` 和 `/api/prometheus`

**文件**:
- 修改: `transport/selector/migrate.go` — 添加 DrainTimeout
- 修改: `transport/selector/selector.go` — 迁移时发射事件
- 测试: `transport/selector/migrate_test.go` — 添加超时测试

**工作量**: 0.5 天

---

## Phase 2: 多出口路由（2-3 天）

**目标**: 支持多个代理服务器，按规则选择不同出口。

**价值**: 负载均衡、故障转移、按地域/用途分流。

### Task 2.1: ProxyOutbound 工厂注册

**问题**: ProxyOutbound 硬编码单一 server addr，无法从 config 创建多个实例。

**方案**:
- 创建 `ProxyOutboundFactory` 注册到 `adapter.RegisterOutbound()`
- Options JSON: `{"server": "addr:port", "transport": "h3", ...}`
- 每个自定义 ProxyOutbound 有独立的 selector（或共享 engine 的 selector）

**文件**:
- 创建: `engine/outbound_proxy_factory.go` — ProxyOutboundFactory
- 修改: `adapter/outbound_registry.go` — 确保 outbound 注册可用
- 测试: `engine/outbound_proxy_factory_test.go`

**工作量**: 1 天

### Task 2.2: 路由规则支持自定义出口标签

**问题**: 路由规则 action 目前只有 proxy/direct/reject，虽然代码已支持自定义标签，但配置和文档未覆盖。

**现状**: `inbound_router.go:54-60` 已有 `outbounds[string(action)]` 查找逻辑。

**方案**:
- 配置校验支持自定义 action 标签（匹配 outbound tag）
- RuleChain 的 action 可以引用自定义 outbound
- 文档化用法示例

**文件**:
- 修改: `config/config_validate.go` — action 允许引用已注册的 outbound tag
- 修改: `router/router.go` — 确保自定义 action 正确传播
- 创建: `docs/multi-outbound-routing.md` — 用法文档
- 测试: `engine/inbound_router_test.go` — 自定义出口路由测试

**工作量**: 1 天

### Task 2.3: 出口健康检查与故障转移

**方案**:
- 每个 ProxyOutbound 有独立 CircuitBreaker
- 当主出口 CB 打开时，路由器自动 fallback 到备用出口
- 配置: `outbound_group` 概念（类似 Clash 的 proxy-group）

**文件**:
- 创建: `engine/outbound_group.go` — OutboundGroup（failover/loadbalance）
- 测试: `engine/outbound_group_test.go`

**工作量**: 1 天

---

## Phase 3: Selector 热切换（1-2 天）

**目标**: 切换传输策略（Auto/Priority/Latency/Multipath）无需 Reload。

**现状**: strategy 字段在 `sync.RWMutex` 保护下，但切换到 Multipath 需要初始化连接池。

### Task 3.1: SetStrategy 方法

**方案**:
- 添加 `Selector.SetStrategy(s Strategy) error`
- 单路径 → 单路径: 原子交换 strategy，触发 `maybeSwitch()`
- 单路径 → Multipath: 初始化 MultipathPool，开始使用
- Multipath → 单路径: 标记 pool 为 draining，graceful 关闭

**文件**:
- 修改: `transport/selector/selector.go` — 添加 SetStrategy
- 修改: `transport/selector/multipath.go` — 支持 graceful drain
- 测试: `transport/selector/selector_test.go` — 策略热切换测试

**工作量**: 1 天

### Task 3.2: 暴露到 API 和配置热重载

**方案**:
- GUI API: `POST /api/transport/strategy` 支持运行时切换
- Engine.Reload 不再需要 stop+start 来切换策略

**文件**:
- 修改: `gui/api/routes_transport.go` — 策略切换端点
- 修改: `engine/engine_lifecycle.go` — Reload 检测策略变更，调用 SetStrategy

**工作量**: 0.5 天

---

## Phase 4: 服务端对齐（1-2 天）

**目标**: 服务端已模块化，补全 plugin chain 和事件系统使其与客户端一致。

**现状**: 服务端用 `metrics.Collector` 而非 plugin chain，无 EventBus。

### Task 4.1: 服务端 Plugin Chain

**方案**:
- 复用 `plugin.Chain` 到服务端 handler
- 每个代理连接经过 metrics → conntrack → logger
- 使服务端和客户端的连接生命周期追踪一致

**文件**:
- 修改: `server/server.go` — handler 中集成 plugin.Chain
- 修改: `server/handler.go` — OnConnect/OnDisconnect 包装

**工作量**: 1 天

### Task 4.2: 服务端 EventBus

**方案**:
- 复用 `ObservabilityManager` 到服务端
- 服务端 admin WebSocket 支持实时事件推送
- 与客户端事件类型统一

**文件**:
- 修改: `server/server.go` — 嵌入 ObservabilityManager
- 修改: `server/admin/` — WebSocket 事件推送

**工作量**: 1 天

---

## 总览

```
Phase 1 (可观测性)     Phase 2 (多出口)      Phase 3 (热切换)     Phase 4 (服务端)
  1.5 天                 3 天                 1.5 天               2 天
┌──────────────┐    ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│ 1.1 Retry    │    │ 2.1 Proxy    │    │ 3.1 Set      │    │ 4.1 Server   │
│     事件发射  │    │     工厂注册  │    │     Strategy  │    │     Plugin   │
│ 1.2 Prom     │    │ 2.2 自定义   │    │ 3.2 API +    │    │     Chain    │
│     端点     │    │     出口标签  │    │     热重载    │    │ 4.2 Server   │
│ 1.3 迁移     │    │ 2.3 健康检查  │    └──────────────┘    │     Events   │
│     完善     │    │     故障转移  │                         └──────────────┘
└──────────────┘    └──────────────┘
       │                   │                    │                    │
       ▼                   ▼                    ▼                    ▼
   基础可见性         功能差异化            运维友好度           架构一致性
```

**依赖关系**:
- Phase 1 无依赖（随时开始）
- Phase 2.3 依赖 2.1
- Phase 3.2 依赖 3.1
- Phase 4 依赖 Phase 1（复用 ObservabilityManager）
- Phase 2 和 3 可并行

**总工作量**: ~8 天，产出完整的可观测性、多出口路由、策略热切换、服务端对齐。
