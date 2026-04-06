# 策略组

策略组将多个出站节点聚合在一起，并通过指定的选择策略决定由哪个成员处理流量。

## 策略说明

| 策略 | 说明 |
|------|------|
| `url-test` | 定期测速，自动选择延迟最低的节点 |
| `failover` | 按顺序尝试，使用第一个健康节点 |
| `select` | 通过 API 或 GUI 手动选择 |
| `loadbalance` | 在所有成员间轮询分发流量 |
| `quality` | 感知拥塞的智能路由 — Shuttle 独有功能 |

### quality 策略

`quality` 策略实时采样每个成员的 BBR 带宽估算值，将新连接路由到吞吐量最高的成员。每隔 `interval` 秒重新评估一次，并跳过丢包率超过 `max_loss` 的成员。

---

## 配置示例

```yaml
proxy_groups:
  - tag: auto
    type: url-test
    outbounds:
      - hk-01
      - hk-02
      - sg-01
    url: https://www.gstatic.com/generate_204
    interval: 300          # 健康检查间隔（秒）
    tolerance_ms: 50       # 延迟在最优值 50ms 以内的节点视为等效

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
    max_loss: 0.05         # 排除丢包率超过 5% 的节点
```

### 字段说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `tag` | string | 必填 | 策略组唯一名称 |
| `type` | string | 必填 | 策略：`url-test` / `failover` / `select` / `loadbalance` / `quality` |
| `outbounds` | list | 必填 | 成员出站标签（可包含其他策略组） |
| `url` | string | generate_204 | 健康检查 URL |
| `interval` | int | `300` | 检查间隔（秒） |
| `tolerance_ms` | int | `0` | url-test：接受与最优值相差 N ms 以内的节点 |
| `max_loss` | float | `0.10` | quality：排除超过此丢包率的节点 |

---

## 策略组嵌套

策略组可以引用其他策略组作为成员，实现分层逻辑：

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
      - hk-auto      # 优先使用最快的香港节点
      - sg-auto      # 回退到最快的新加坡节点
      - DIRECT
```

---

## 通过 API 切换节点

对于 `select` 类型的策略组，可通过 REST API 在运行时切换当前节点：

```http
PUT /api/groups/{tag}/selected
Content-Type: application/json

{"selected": "sg-01"}
```

成功返回 `200 OK`。新选择在下次配置重载前持续生效。

---

## GUI 使用

在 GUI 中，从侧边栏打开**代理**页面。每个策略组显示为一张卡片，包含当前选择和延迟徽标。点击成员可切换选择（`select` 类型）或强制测速（`url-test` 类型）。
