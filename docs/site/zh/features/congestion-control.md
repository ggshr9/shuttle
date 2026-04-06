# 拥塞控制

Shuttle 为基于 QUIC 的传输层（H3 和 Reality）提供三种拥塞控制算法，请根据网络状况选择合适的模式。

---

## 模式说明

### BBR

Google 的带宽估算拥塞控制算法。BBR 主动探测可用带宽并维护瓶颈链路模型。在高丢包或高延迟链路上，BBR 表现优于基于丢包的算法（如 Cubic），因为它不会因误判而过度退让。

**适用场景：** 通用场景、网络行为正常的云服务器。

### Brutal

以固定的配置速率发送数据，忽略网络反馈，不在丢包时退让。适用于已知数据包被主动丢弃（主动干扰）的场景，即便有干扰也能以稳定速率推送流量。

**适用场景：** 存在刻意限速或数据包注入导致虚假丢包信号的网络。

> **警告：** Brutal 具有侵略性，会与其他流量竞争不公平。仅在必要时使用。

### Adaptive

实时监控丢包率和 RTT，在 BBR 和 Brutal 之间自动切换：

- 初始进入 **BBR** 模式。
- 若丢包率超过 `loss_threshold` 或 RTT 超过 `rtt_threshold`，切换到 **Brutal**。
- `switch_cooldown` 秒内无阈值超限，切回 **BBR**。

**适用场景：** 网络条件频繁变化的场景，例如移动网络或间歇性受干扰的链路。

---

## 配置示例

```yaml
congestion:
  mode: adaptive          # bbr | brutal | adaptive

  # Brutal 参数（mode 为 brutal 或 adaptive 切换至 brutal 时使用）
  bandwidth: 50           # 目标发送速率（Mbps）

  # Adaptive 阈值
  loss_threshold: 0.02    # 丢包率超过 2% 时切换到 Brutal
  rtt_threshold: 400      # RTT 超过 400ms 时切换到 Brutal
  switch_cooldown: 30     # 切回 BBR 前等待的秒数
```

### 字段说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `mode` | string | `bbr` | 算法：`bbr` / `brutal` / `adaptive` |
| `bandwidth` | int（Mbps） | `100` | Brutal 模式的目标速率 |
| `loss_threshold` | float | `0.02` | Adaptive：触发切换到 Brutal 的丢包率 |
| `rtt_threshold` | int（ms） | `400` | Adaptive：触发切换到 Brutal 的 RTT 值 |
| `switch_cooldown` | int（s） | `10` | Adaptive：切回 BBR 前的等待秒数 |

---

## 切换逻辑（Adaptive）

```
         丢包率 > 阈值
         或 RTT > 阈值
BBR ───────────────────────► Brutal
  ◄───────────────────────
         switch_cooldown 秒内
         无阈值超限
```

切换以连接为粒度，每条新连接始终从 BBR 模式启动。

---

## 如何选择？

| 场景 | 推荐模式 |
|------|---------|
| 普通云服务器 VPS | `bbr` |
| DPI 主动干扰 / 限速 | `brutal` |
| 移动网络或网络条件不稳定 | `adaptive` |
| 未知情况 / 首次部署 | `adaptive` |
