# 多路径（Multipath）

多路径功能允许 Shuttle 同时使用多个传输层承载同一逻辑连接，根据调度策略将流量分发到各路径上。

---

## 功能介绍

未启用多路径时，每条连接只使用一种传输层（H3、Reality 或 CDN）。启用多路径后，Shuttle 在每种配置的传输层上各建立一条连接，并按策略分发流量：

- **带宽聚合** — 将两条 50Mbps 路径合并为约 100Mbps 的有效吞吐量。
- **冗余容灾** — 某条路径降级或断开时，流量自动切换到其余路径，无需重连。
- **延迟对冲** — 使用 `min-latency` 调度时，每条新流自动选择当前 RTT 最低的路径。

---

## 调度模式

| 模式 | 说明 |
|------|------|
| `weighted` | 按配置权重比例分发流量 |
| `min-latency` | 每条新流选择当前 RTT 最低的路径 |
| `load-balance` | 在所有健康路径间轮询 |

---

## 配置示例

```yaml
outbounds:
  - tag: my-server
    type: auto
    server: your.server.example.com
    port: 443
    password: your-password

    transport:
      preferred: multipath
      multipath_schedule: min-latency   # weighted | min-latency | load-balance
      multipath_paths:
        - type: h3
          weight: 2          # 仅在 weighted 调度时使用
        - type: reality
          weight: 1
        - type: cdn
          weight: 1
```

### 字段说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `transport.preferred` | string | `auto` | 设为 `multipath` 以启用 |
| `transport.multipath_schedule` | string | `min-latency` | 调度策略 |
| `transport.multipath_paths` | list | 所有可用 | 要包含的传输层列表 |
| `multipath_paths[].type` | string | 必填 | `h3` / `reality` / `cdn` |
| `multipath_paths[].weight` | int | `1` | `weighted` 调度的权重值 |

---

## 使用场景

### 带宽聚合

如果客户端有两条 ISP 线路（或有线 + 蜂窝），为每种传输类型各配置一条路径，指向同一服务器。Shuttle 会将流跨两条物理路径分散发送。

### 弹性隧道

使用 `load-balance` 多路径，即使某 ISP 短暂中断也能保持流量不断。由于所有路径已预先建立，故障切换是即时的，没有重连延迟。

### 低延迟流媒体

使用 `min-latency` 模式，当你有一条快速低延迟路径和一条较慢高延迟路径时，交互流量（游戏、视频通话）自然分配到快速路径，批量下载则同时利用两条路径。

---

## 注意事项

- 服务端必须在 `multipath_paths` 中包含的所有传输类型上均可访问。
- 每条路径维护独立的拥塞控制器，出站配置中的 `congestion` 设置对每条路径独立生效。
- 多路径与仅 CDN 部署不兼容（CDN 强制每个客户端只有单条连接的场景）。
