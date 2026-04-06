# REST API 参考

Shuttle GUI 在一个随机本地端口上提供 REST API（启动时打印）。除特殊说明外，所有端点均使用 JSON 格式。

**基础 URL：** `http://127.0.0.1:{port}`

---

## 状态（Status）

### GET /api/status

返回当前引擎状态。

**响应：**
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

返回当前 Shuttle 版本。

**响应：**
```json
{
  "version": "1.2.3"
}
```

### GET /api/debug/state

返回详细的引擎调试状态，包括 goroutine 数量和运行时长。

**响应：**
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

返回 Go 运行时内存和 CPU 资源使用情况。

**响应：**
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

## 配置（Config）

### GET /api/config

返回当前客户端配置（敏感字段已脱敏）。

**响应：** 完整的 `ClientConfig` JSON 对象。

### PUT /api/config

替换整个客户端配置并热重载引擎。

**请求体：** 完整的 `ClientConfig` JSON 对象。

**响应：**
```json
{ "status": "reloaded" }
```

### GET /api/config/servers

返回当前活跃服务器和已保存的服务器列表。

**响应：**
```json
{
  "active": { "addr": "example.com:443", "name": "主节点", "sni": "" },
  "servers": [
    { "addr": "example.com:443", "name": "主节点", "password": "", "sni": "" }
  ]
}
```

### PUT /api/config/servers

切换活跃服务器并重新加载。

**请求体：**
```json
{ "addr": "example.com:443", "name": "主节点", "password": "secret", "sni": "" }
```

**响应：**
```json
{ "status": "updated" }
```

### POST /api/config/servers

向已保存列表中添加一个新服务器。

**请求体：**
```json
{ "addr": "example.com:443", "name": "美国节点", "password": "secret", "sni": "" }
```

**响应：**
```json
{ "status": "ok" }
```

**错误：** 地址已存在时返回 `409 Conflict`。

### DELETE /api/config/servers

按地址从已保存列表中删除服务器。

**请求体：**
```json
{ "addr": "example.com:443" }
```

**响应：**
```json
{ "status": "ok" }
```

**错误：** 地址不存在时返回 `404 Not Found`。

### POST /api/config/validate

验证配置对象而不应用它。

**请求体：** 完整的 `ClientConfig` JSON 对象。

**响应：**
```json
{
  "valid": true,
  "errors": []
}
```

### GET /api/config/export

将当前配置导出为可下载文件。

**查询参数：**

| 参数 | 可选值 | 默认值 | 说明 |
|---|---|---|---|
| `format` | `json`、`yaml`、`uri` | `json` | 导出格式 |
| `include_secrets` | `true`、`false` | `false` | 是否包含密码和密钥 |

返回文件附件（`application/json`、`text/yaml` 或 `text/plain`）。

### POST /api/config/import

从 JSON/YAML/URI 字符串导入服务器，重复项将被跳过。

**请求体：**
```json
{ "data": "shuttle://..." }
```

**响应：**
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

## 代理（Proxy）

### POST /api/connect

启动引擎，并在配置启用时设置系统代理。

**响应：**
```json
{ "status": "connected" }
```

**错误：** 已在运行时返回 `409 Conflict`。

### POST /api/disconnect

停止引擎并清除系统代理。

**响应：**
```json
{ "status": "disconnected" }
```

### GET /api/autostart

返回是否已启用开机自启动。

**响应：**
```json
{ "enabled": true }
```

### PUT /api/autostart

启用或禁用开机自启动。

**请求体：**
```json
{ "enabled": true }
```

**响应：**
```json
{ "enabled": true }
```

### GET /api/network/lan

返回局域网共享状态和本地监听地址。

**响应：**
```json
{
  "allow_lan": false,
  "addresses": ["192.168.1.100"],
  "socks5": "127.0.0.1:1080",
  "http": "127.0.0.1:8080"
}
```

---

## 路由（Routing）

### GET /api/routing/rules

返回当前路由配置。

**响应：** 完整的 `RoutingConfig` JSON 对象。

### PUT /api/routing/rules

替换路由配置并重新加载。

**请求体：** 完整的 `RoutingConfig` JSON 对象。

**响应：**
```json
{ "status": "updated" }
```

### GET /api/routing/export

将路由规则导出为可下载的 JSON 文件。

**响应：** `RoutingConfig` JSON 附件。

### POST /api/routing/import

导入路由规则，可选择合并或替换现有规则。

**请求体：**
```json
{
  "rules": [
    { "geosite": "cn", "action": "direct" }
  ],
  "default": "proxy",
  "mode": "merge"
}
```

`mode` 为 `merge`（追加，默认）或 `replace`（替换）。

**响应：**
```json
{
  "status": "imported",
  "added": 1,
  "total": 5,
  "existing": 4
}
```

### GET /api/routing/templates

返回内置路由模板列表。

**响应：**
```json
[
  {
    "id": "bypass-cn",
    "name": "绕过中国大陆",
    "description": "国内直连，其余走代理",
    "rules": [...],
    "default": "proxy"
  }
]
```

可用模板 ID：`bypass-cn`、`proxy-all`、`direct-all`、`block-ads`。

### POST /api/routing/templates/:id

应用内置模板作为当前路由配置（DNS 设置会被保留）。

**响应：**
```json
{ "status": "applied", "template": "bypass-cn" }
```

**错误：** 未知模板 ID 返回 `404 Not Found`。

### POST /api/routing/test

测试指定域名或 URL 将匹配哪条路由规则。

**请求体：**
```json
{ "url": "https://www.google.com" }
```

**响应：** 路由测试结果对象（动作、匹配规则等）。

### GET /api/routing/conflicts

检测当前路由配置中的冲突或被遮蔽的规则。

**响应：**
```json
{
  "conflicts": [...],
  "count": 0
}
```

### GET /api/pac

根据当前路由规则生成 PAC（代理自动配置）脚本。

**查询参数：**

| 参数 | 值 | 说明 |
|---|---|---|
| `download` | `true` | 添加 `Content-Disposition: attachment` 响应头 |

**响应：** `application/x-ns-proxy-autoconfig` 内容。

---

## 传输（Transport）

### POST /api/transport/strategy

在运行时切换活跃的传输选择策略。

**请求体：**
```json
{ "strategy": "auto" }
```

有效策略：`auto`、`priority`、`latency`、`multipath`。

**响应：**
```json
{ "ok": true, "strategy": "auto" }
```

**错误：** 无效策略返回 `400 Bad Request`；切换失败返回 `409 Conflict`。

---

## 订阅（Subscriptions）

### GET /api/subscriptions

返回所有已配置的订阅。

**响应：**
```json
[
  {
    "id": "abc123",
    "name": "我的节点",
    "url": "https://example.com/sub"
  }
]
```

### POST /api/subscriptions

添加新订阅，立即拉取并保存到配置。

**请求体：**
```json
{ "name": "我的节点", "url": "https://example.com/sub" }
```

**响应：** 创建的订阅对象。

### PUT /api/subscriptions/:id/refresh

按 ID 手动刷新订阅。

**响应：** 更新后的订阅对象。

### DELETE /api/subscriptions/:id

删除订阅并保存配置。

**响应：**
```json
{ "status": "deleted" }
```

---

## 统计（Stats）

### GET /api/stats/history

返回每日流量统计。

**查询参数：**

| 参数 | 范围 | 默认值 | 说明 |
|---|---|---|---|
| `days` | 1–90 | `7` | 历史天数 |

**响应：**
```json
{
  "history": [
    { "date": "2026-04-05", "bytes_sent": 1048576, "bytes_received": 5242880 }
  ],
  "total": { "bytes_sent": 1048576, "bytes_received": 5242880 }
}
```

### GET /api/stats/weekly

返回每周流量汇总。

**查询参数：**

| 参数 | 范围 | 默认值 | 说明 |
|---|---|---|---|
| `weeks` | 1–52 | `4` | 周数 |

**响应：** `PeriodStats` 数组。

### GET /api/stats/monthly

返回每月流量汇总。

**查询参数：**

| 参数 | 范围 | 默认值 | 说明 |
|---|---|---|---|
| `months` | 1–24 | `6` | 月数 |

**响应：** `PeriodStats` 数组。

### GET /api/connections/history

返回最近 100 条连接日志。

**响应：** 连接日志条目数组。

### GET /api/connections/:id/streams

返回指定连接 ID 下的所有流。

**响应：**
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

返回当前引擎状态中各传输协议的流量分解。

**响应：** 传输分解对象数组。

### GET /api/multipath/stats

返回多路径调度器统计信息。

**响应：** 各路径统计对象数组。

### GET /api/logs *（WebSocket）*

实时推送日志事件。使用 `Connection: Upgrade, Upgrade: websocket` 升级连接。

### GET /api/speed *（WebSocket）*

实时推送速度 tick 事件（上传/下载字节每秒）。

### GET /api/connections *（WebSocket）*

实时推送活跃连接事件。

---

## Mesh VPN

### GET /api/mesh/status

返回 Mesh VPN 高层状态。

**响应：**
```json
{
  "enabled": true,
  "virtual_ip": "10.7.0.3",
  "cidr": "10.7.0.0/24",
  "peer_count": 2
}
```

### GET /api/mesh/peers

返回所有 Mesh 对等节点及连接质量信息。

**响应：**
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

按虚拟 IP 触发与对等节点的 P2P 连接。

**响应：**
```json
{ "ok": true, "vip": "10.7.0.2" }
```

**错误：** 连接失败时返回 `409 Conflict`。

---

## 策略组（Groups）

### GET /api/groups

返回所有策略组及当前状态。

**响应：**
```json
[
  {
    "tag": "auto",
    "type": "url-test",
    "selected": "美国节点",
    "members": ["美国节点", "欧洲节点"]
  }
]
```

### GET /api/groups/:tag

返回特定策略组的详细信息，包括成员延迟。

**响应：** 策略组详情对象。

### PUT /api/groups/:tag/selected

在 `select` 策略组中手动选择出站节点。

**请求体：**
```json
{ "selected": "美国节点" }
```

**响应：** `204 No Content`

### POST /api/groups/:tag/test

触发策略组中所有成员的健康检测并返回延迟结果。

**响应：** 成员名称到延迟（ms）的映射。

---

## Provider

### GET /api/providers/proxy

返回所有代理 Provider 及状态和代理数量。

**响应：**
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

按名称手动刷新代理 Provider。

**响应：** `204 No Content`

**错误：** Provider 不存在时返回 `404 Not Found`。

### GET /api/providers/rule

返回所有规则 Provider 及状态。

**响应：**
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

按名称手动刷新规则 Provider。

**响应：** `204 No Content`

**错误：** Provider 不存在时返回 `404 Not Found`。
