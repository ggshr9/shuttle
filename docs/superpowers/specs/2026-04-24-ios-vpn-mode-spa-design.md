# iOS VPN 模式下的 SPA 替换 Fallback HTML —— 设计文档

> 状态：设计审核中
> 作者：Claude（与 user 通过 brainstorming 推导）
> 日期：2026-04-24
> 关联：`docs/superpowers/specs/2026-04-21-mobile-unification-design.md`

## 1. 背景与问题

iOS 上当 Shuttle 工作在 VPN（TUN）模式时，Apple Network Extension 框架强制 Go 引擎运行在 `NEPacketTunnelProvider` 扩展进程中，与主 App 进程隔离。扩展进程的 `127.0.0.1:<apiAddr>` 绑定对主 App 的 `WKWebView` 不可见，因此现状是 `mobile/ios/Shuttle/ShuttleApp.swift:152-194` 走一段硬编码的 fallback HTML，仅展示一个简单的 "Connect" 按钮，**整套 SPA 在 iOS VPN 模式下不可用**。

代理模式不受影响（引擎在主 App 进程内）。Android VPN 模式不受影响（VpnService 在主 App 进程内）。Wails 桌面不受影响。问题面只在 iOS VPN 模式这一个运行时格子。

`docs/superpowers/specs/2026-04-21-mobile-unification-design.md` 完成的统一目标是 "3 套导航 chrome + 6 个页面 + 1 套 SPA + N 个运行时"。本 spec 在保持这条统一前提下，把 iOS VPN 模式从"被降级"还原为"用了不同传输的同一 SPA"。

## 2. 目标 / 非目标

### 目标

- iOS VPN 模式下加载完整 SPA（6 大页面：Now / Servers / Traffic / Mesh / Activity / Settings），所有交互与其它运行时一致
- 状态、Speed、Mesh peers 实时更新延迟 ≤2s
- 日志 tail 延迟 ≤1s
- 维护性：加新 endpoint 不需要触碰 iOS native 代码；加新 subscription 仅一行配置
- 失败时可平滑回退到既有 fallback HTML，不破坏现有发布

### 非目标

- 不修改 iOS 代理模式路径（已工作）
- 不修改 Android / Wails / Web 浏览器路径
- 不改变 Apple Network Extension 进程模型（不可能）
- 不引入新的 SPA UI 概念或页面变化（这是底层重构）

## 3. 架构总览

四个组件协作：

```
┌──────────────── Shuttle.app（主进程）────────────────┐
│  WKWebView                                            │
│  ├─ loadFileURL("Shuttle/www/index.html")             │
│  ├─ userScript: 注入 window.ShuttleBridge             │
│  └─ messageHandler "shuttleBridge" → APIBridge.swift  │
│                                                        │
│  SPA                                                   │
│  ├─ lib/platform/index.ts: detect → BridgeAdapter     │
│  └─ lib/data/bridge-adapter.ts: 信封 IPC + 轮询       │
│                                                        │
│  APIBridge.swift (WKScriptMessageHandler)              │
│  ├─ 编码信封                                          │
│  └─ session.sendProviderMessage(envelope) ───────┐    │
└───────────────────────────────────────────────────┼───┘
                                                    │
┌── ShuttleExtension（NEPacketTunnelProvider）─────┼───┐
│  PacketTunnelProvider.handleAppMessage           ▼   │
│  ├─ 解码 envelope                                    │
│  ├─ URLSession → http://127.0.0.1:<apiAddr>/<path>  │
│  └─ 回写 APIResponse                                 │
│                                                       │
│  Go 引擎 (gomobile xcframework)                      │
│  └─ MobileStart → 127.0.0.1:apiAddr REST + WS        │
└──────────────────────────────────────────────────────┘
```

**关键架构决策：**

1. **DataAdapter 抽象层** —— SPA 的所有 HTTP/WS 调用收敛到 `lib/data/types.ts` 的 `DataAdapter` 接口。运行时注入两种实现：`HttpAdapter`（默认，所有运行时通用）和 `BridgeAdapter`（仅 iOS VPN 模式）。
2. **SPA 资源本地化** —— 不再依赖 HTTP server 提供 SPA assets。`build-ios.sh` 已经把 `gui/web/dist/*` 拷贝进 `Shuttle/www/`。`WKWebView.loadFileURL` 直接加载。
3. **Native 层 endpoint-agnostic** —— Swift 和 Extension 代码不感知任何 API 路径。它们只搬运信封，编解码契约由共享 `SharedBridge` Swift Package 定义。这意味着加新 endpoint 不需要改 Swift。
4. **subscribe 抽象代替直连 WebSocket** —— SPA 不再 `new WebSocket()`。取而代之是 `adapter.subscribe(topic)`。HTTP adapter 内部用 WS 实现，Bridge adapter 用轮询实现。SPA 业务代码看不到差别。

### 运行时矩阵

| 运行时 | 数据层 | 引擎进程 |
|---|---|---|
| Web 浏览器 | HttpAdapter | 远端 |
| Wails 桌面 | HttpAdapter | 主进程 |
| Android 代理 | HttpAdapter | 主进程 |
| Android VPN | HttpAdapter | 主进程（VpnService 同进程） |
| iOS 代理 | HttpAdapter | 主进程 |
| **iOS VPN** | **BridgeAdapter** | **Extension 进程** |

矩阵中只有一个格子使用 BridgeAdapter，这是 iOS 进程隔离的物理事实，不是设计缺陷。其余五种运行时全部走同一个 HttpAdapter 路径。

## 4. DataAdapter 接口契约

### 4.1 类型定义

```typescript
// gui/web/src/lib/data/types.ts

export interface DataAdapter {
  request<T = unknown>(opts: RequestOptions): Promise<T>
  subscribe<K extends TopicKey>(
    topic: K,
    opts?: SubscribeOptions<K>,
  ): Subscription<TopicValue<K>>
  readonly connectionState: ReadableValue<ConnectionState>
}

export type RequestOptions = {
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'
  path: string
  body?: unknown                       // 自动 JSON.stringify
  headers?: Record<string, string>     // adapter 自动注入 Authorization
  signal?: AbortSignal
}

export type Subscription<T> = {
  subscribe(callback: (value: T) => void): () => void
  readonly current: T | undefined      // snapshot 主题保留最新值；stream 主题始终 undefined
}

export type SubscribeOptions<K extends TopicKey> = {
  cursor?: TopicMap[K] extends { kind: 'stream' } ? string | number : never
  pollInterval?: number                // 覆盖 topicConfig 默认值，用于测试
}

export type ConnectionState = 'idle' | 'connecting' | 'connected' | 'error'

// 极薄响应式包装 —— Svelte 5 用 $state，框架无关用 mini observable。
// 任何订阅者获得当前值 + 后续变化通知。
export interface ReadableValue<T> {
  readonly value: T
  subscribe(callback: (value: T) => void): () => void
}

export class ApiError extends Error {
  constructor(public readonly status: number,
              public readonly code: string | undefined,
              message: string) { super(message) }
}

export class TransportError extends Error {
  constructor(public readonly cause: unknown, message: string) { super(message) }
}
```

### 4.2 request 行为契约（两 adapter 必须一致）

| 输入 | 输出 |
|---|---|
| 2xx + JSON body | `Promise.resolve(parsed)` |
| 2xx + empty body | `Promise.resolve(undefined)` |
| 4xx/5xx | `Promise.reject(new ApiError({status, code, message}))` |
| 网络错误 / IPC 超时 | `Promise.reject(new TransportError(...))` |
| AbortSignal 触发 | `Promise.reject(new DOMException('Aborted', 'AbortError'))` |
| 任何 request | adapter 自动注入认证 header（Bearer token） |

### 4.3 subscribe 语义 —— 两种 kind

主题分两类，由 topic registry 声明：

**`kind: 'snapshot'`** —— 有"当前值"概念（status / speed / mesh peers）
- 订阅时立即触发一次 callback 输出当前值
- 之后只在值变化时再触发（diff）
- `subscription.current` 总是返回最近一次值
- HTTP adapter：WS 连接成功后服务端立即 push 当前值
- Bridge adapter：立即发一次 REST，得到值；之后轮询，diff 不同才 emit

**`kind: 'stream'`** —— 增量事件流（logs / events）
- 订阅时不重放历史，只 emit 新增
- `subscription.current` 永远 undefined
- 支持 cursor 续接：`opts.cursor` 指定恢复点，断线重连不丢
- HTTP adapter：WS 上服务端按 cursor 续推
- Bridge adapter：轮询 `?since=<cursor>`，只 emit 新事件

### 4.4 Topic Registry —— 双重声明

```typescript
// gui/web/src/lib/data/topics.ts

export type TopicMap = {
  status: { value: Status, kind: 'snapshot' }
  speed:  { value: SpeedSample, kind: 'snapshot' }
  mesh:   { value: MeshPeer[], kind: 'snapshot' }
  logs:   { value: LogLine, kind: 'stream' }
  events: { value: EngineEvent, kind: 'stream' }
}

export type TopicKey = keyof TopicMap
export type TopicValue<K extends TopicKey> = TopicMap[K]['value']

export const topicConfig = {
  status: { wsPath: '/ws/status', restPath: '/api/status', pollMs: 2000, kind: 'snapshot' },
  speed:  { wsPath: '/ws/speed',  restPath: '/api/speed',  pollMs: 1000, kind: 'snapshot' },
  mesh:   { wsPath: '/ws/mesh',   restPath: '/api/mesh/peers', pollMs: 3000, kind: 'snapshot' },
  logs:   { wsPath: '/ws/logs',   restPath: '/api/logs',   pollMs: 1000, kind: 'stream', cursorParam: 'since' },
  events: { wsPath: '/ws/events', restPath: '/api/events', pollMs: 1000, kind: 'stream', cursorParam: 'since' },
} as const satisfies Record<TopicKey, TopicEntry>
```

加新主题 = 改这一个文件。两个 adapter 都从此读取。TS 编译期保证 `subscribe('status')` 的 callback 参数类型为 `Status`。

### 4.5 connectionState 行为

| 触发条件 | 状态变化 |
|---|---|
| 第一次 subscribe | `idle → connecting → connected` |
| 所有订阅取消 | `connected → idle`（关闭 WS / 停止轮询） |
| 传输失败（连续 3 次） | `connected → error`，指数 backoff 重试 |
| 重连成功 | `error → connecting → connected` |

UI 顶栏一个 `<ConnectionDot>` 订阅此值，所有运行时一致。

### 4.6 Svelte 5 hook

```typescript
// gui/web/src/lib/data/hooks.svelte.ts

export function useSubscription<K extends TopicKey>(
  topic: K,
  opts?: SubscribeOptions<K>,
) {
  let value = $state<TopicValue<K> | undefined>(undefined)
  $effect(() => {
    const sub = adapter.subscribe(topic, opts)
    return sub.subscribe(v => { value = v })
  })
  return { get value() { return value } }
}

export async function useRequest<T>(opts: RequestOptions): Promise<T> {
  return adapter.request<T>(opts)
}
```

业务代码：

```svelte
<script>
  import { useSubscription } from '$lib/data/hooks.svelte'
  const status = useSubscription('status')
</script>

{#if status.value?.connected}
  <span>已连接到 {status.value.serverName}</span>
{/if}
```

跨平台、跨传输完全一致。

## 5. BridgeAdapter 轮询引擎

### 5.1 TopicPoller 生命周期

每个主题独立一个 `TopicPoller` 实例，多订阅者共享一个 poller。

| 触发 | 动作 |
|---|---|
| `subscribers: 0 → 1` | 启动 poller，立即 tick 一次（不等 interval） |
| `subscribers: n → n+1` (n≥1) | 已运行不重启。snapshot 主题 `queueMicrotask` 把当前缓存值 emit 给新订阅者 |
| `subscribers: n → n-1` (n≥2) | 仅从订阅集移除 |
| `subscribers: 1 → 0` | 停止 poller，清 timer。**保留** `currentValue` 和 `cursor` 用于秒重订阅 |
| `document.visibilityState = 'hidden'` | 所有 poller 停 timer，订阅集保留 |
| `visibilitychange` to `visible` | 所有还有订阅者的 poller 立即 tick 并恢复 interval |

### 5.2 单飞防抖

```typescript
private async tick() {
  if (this.inFlight) return
  this.inFlight = true
  try { /* fetch */ }
  finally { this.inFlight = false }
}
```

避免轮询间隔小于 IPC RTT 时请求堆积。

### 5.3 snapshot diff 算法（v1）

```typescript
private handleSnapshot(result: TopicValue<K>) {
  const hash = JSON.stringify(result)
  if (hash === this.lastHash) return
  this.lastHash = hash
  this.currentValue = result
  for (const cb of this.subscribers) cb(result)
}
```

Go 侧 `json.Marshal` 字段顺序稳定，前端字符串比较足够。

复杂度 O(n)，500 条 servers 列表 ≈ 1ms on iPhone 14。后续可选优化：服务端 etag + `If-None-Match`，profiling 发现必要时再做。

### 5.4 stream cursor 处理

```typescript
private async streamTick() {
  const path = `${config.restPath}?${config.cursorParam}=${this.cursor ?? 0}`
  const { lines, cursor: nextCursor } = await this.bridge.request({ method: 'GET', path })
  this.cursor = nextCursor   // 先更新 cursor 再 emit，防止 emit 期间取消订阅导致重复推送
  for (const line of lines) {
    for (const cb of this.subscribers) cb(line)
  }
}
```

cursor 不持久化跨刷新（iOS WebView 刷新极少见）。

### 5.5 backoff 与错误恢复

```typescript
private handleError(err: unknown) {
  this.errorCount++
  this.connectionState.report(this.topic, 'error', err)
  if (this.errorCount === 1) return   // 第 1 次错让下个 interval 自然重试
  clearInterval(this.timer)
  const delay = Math.min(30_000, 500 * 2 ** (this.errorCount - 1))
  this.timer = setTimeout(() => this.start(), delay)
}
```

退避序列：1s → 2s（自然） → 1s（backoff） → 2s → 4s → 8s → 16s → 30s → 30s → ...

成功后 `errorCount` 清零并上报 `'connected'`。

### 5.6 connectionState 聚合

```typescript
class ConnectionStateController {
  private topicStates = new Map<TopicKey, 'ok' | 'error'>()
  state = $state<ConnectionState>('idle')

  report(topic: TopicKey, kind: 'ok' | 'error') {
    this.topicStates.set(topic, kind)
    this.recompute()
  }

  private recompute() {
    if (this.topicStates.size === 0) { this.state = 'idle'; return }
    const anyOk = [...this.topicStates.values()].some(s => s === 'ok')
    this.state = anyOk ? 'connected' : 'error'
  }
}
```

### 5.7 HttpAdapter 共享实现

`HttpAdapter` 用对称设计的 `HttpTopicSubscription`，与 `BridgeAdapter` 的 `TopicPoller` 共享 `SubscriptionBase` 基类。基类提供：引用计数、缓存、connectionState 上报、隐藏暂停、backoff。子类只实现 `connect()` / `disconnect()` / `tick()`。

```typescript
// gui/web/src/lib/data/subscription-base.ts
abstract class SubscriptionBase<T> {
  protected subscribers = new Set<(v: T) => void>()
  protected currentValue: T | undefined
  protected lastHash: string | undefined        // snapshot diff
  protected cursor: string | number | undefined // stream
  protected errorCount = 0
  protected inFlight = false

  abstract connect(): void | Promise<void>      // 启动 WS 或 timer
  abstract disconnect(): void                    // 停止
  abstract tick(): Promise<void>                 // BridgeAdapter 用；HttpAdapter 一般 noop（推送驱动）

  add(cb: (v: T) => void): () => void { /* 引用计数 + microtask 重放 */ }
  protected emit(v: T) { /* 公共 emit + diff 逻辑 */ }
  protected handleError(err: unknown) { /* 公共 backoff 逻辑 */ }
  pauseForHidden() { this.disconnect() }
  resumeFromHidden() { if (this.subscribers.size > 0) this.connect() }
}
```

这把"维护性统一"从靠测试升级为靠继承。

### 5.8 代码组织

```
gui/web/src/lib/data/
├── types.ts                    # 接口、错误类型
├── topics.ts                   # TopicMap + topicConfig
├── subscription-base.ts        # 通用引用计数 / 缓存 / hidden / backoff
├── http-adapter.ts             # WS 实现
├── http-subscription.ts        # extends SubscriptionBase
├── bridge-adapter.ts           # 信封 IPC 实现
├── bridge-subscription.ts      # extends SubscriptionBase
├── bridge-transport.ts         # bridge.send(envelope) → Promise 单一入口
├── connection-state.ts         # 聚合控制器
├── hooks.svelte.ts             # useSubscription / useRequest
├── index.ts                    # detect() 选 adapter + bridge probe
└── __tests__/
    └── conformance.spec.ts     # describe.each 双 adapter 跑
```

## 6. iOS Native 三件套 + Go 事件端点

### 6.1 主进程 JS · `window.ShuttleBridge`

通过 `WKUserContentController.addUserScript` 在 SPA 加载前注入：

```javascript
window.ShuttleBridge = (() => {
  const pending = new Map();
  let nextId = 0;
  return {
    send(envelope) {
      const id = ++nextId;
      return new Promise((resolve, reject) => {
        pending.set(id, { resolve, reject });
        webkit.messageHandlers.shuttleBridge.postMessage({ id, envelope });
      });
    },
    _complete(id, response) {
      const p = pending.get(id);
      if (!p) return;
      pending.delete(id);
      p.resolve(response);
    },
    _fail(id, message) {
      const p = pending.get(id);
      if (!p) return;
      pending.delete(id);
      p.reject(new Error(message));
    },
  };
})();
```

`lib/platform/index.ts` 的 `detect()` 中 sniff `window.ShuttleBridge` —— 命中时使用 BridgeAdapter。

### 6.2 主进程 Swift · `APIBridge`

```swift
final class APIBridge: NSObject, WKScriptMessageHandler {
    private weak var webView: WKWebView?
    private let manager: VPNManager

    func userContentController(_ ucc: WKUserContentController,
                               didReceive msg: WKScriptMessage) {
        guard let body = msg.body as? [String: Any],
              let id   = body["id"] as? Int,
              let env  = body["envelope"] as? [String: Any],
              let data = try? JSONSerialization.data(withJSONObject: env) else { return }
        manager.sendToExtension(data, timeout: 30) { [weak self] response in
            self?.complete(id: id, response: response)
        }
    }

    private func complete(id: Int, response: Data?) {
        DispatchQueue.main.async {
            guard let webView = self.webView else { return }
            if let data = response, let json = String(data: data, encoding: .utf8) {
                webView.evaluateJavaScript("window.ShuttleBridge._complete(\(id), \(json))")
            } else {
                webView.evaluateJavaScript(
                    "window.ShuttleBridge._fail(\(id), 'IPC timeout or no response')")
            }
        }
    }
}
```

`VPNManager.sendToExtension(_:timeout:completion:)` 在现有 `VPNManager.swift:120` 的 `sendProviderMessage` 之上加一个 30s 超时定时器。

### 6.3 Extension · `handleAppMessage` REST 转发

`PacketTunnelProvider.swift:86` 的 string-command dispatcher 添加 envelope 分支：

```swift
override func handleAppMessage(_ data: Data,
                               completionHandler: ((Data?) -> Void)?) {
    if let req = try? JSONDecoder().decode(APIRequest.self, from: data) {
        forwardToLocalAPI(req) { resp in
            completionHandler?(try? JSONEncoder().encode(resp))
        }
        return
    }
    if let cmd = String(data: data, encoding: .utf8) {
        handleLegacyCommand(cmd, completionHandler: completionHandler)
        return
    }
    completionHandler?(nil)
}

private func forwardToLocalAPI(_ req: APIRequest,
                               completion: @escaping (APIResponse) -> Void) {
    guard let apiAddr = currentAPIAddr else {
        completion(.engineNotReady())
        return
    }
    var url = URLComponents()
    url.scheme = "http"; url.host = "127.0.0.1"
    url.port = apiAddr.port; url.path = req.path
    var urlReq = URLRequest(url: url.url!)
    urlReq.httpMethod = req.method
    for (k, v) in req.headers { urlReq.setValue(v, forHTTPHeaderField: k) }
    if let b64 = req.body, let body = Data(base64Encoded: b64) {
        urlReq.httpBody = body
    }
    URLSession.shared.dataTask(with: urlReq) { data, resp, err in
        if let err = err {
            completion(.transportError("\(err)")); return
        }
        guard let http = resp as? HTTPURLResponse else {
            completion(.transportError("non-http response")); return
        }
        let bodyB64 = (data ?? Data()).base64EncodedString()
        let headers = (http.allHeaderFields as? [String: String]) ?? [:]
        completion(APIResponse(status: http.statusCode,
                               headers: headers,
                               body: bodyB64,
                               error: nil))
    }.resume()
}
```

`APIRequest` / `APIResponse` 定义放共享 SPM `mobile/ios/SharedBridge/`，主 App 与 Extension 都 link，保证编解码契约一致。

旧 string commands（`"status"` / `"stop"` / `"logs"` / 配置 reload JSON）短期保留向后兼容，Phase γ 删除。

### 6.4 信封大小处理

iOS `sendProviderMessage` payload 文档说 "应小"，实测 ~256KB 以下稳定。

**v1 硬约束：**
- 请求 body ≤ 64 KB
- 响应 body ≤ 192 KB
- 超出 → extension 端返回 `APIResponse(status: -1, error: "response too large")` → BridgeAdapter throw `TransportError`

**保证不触发的设计动作：**
1. `/api/servers` 加 `?page=N&size=50` 分页（Go 侧改动 ~30 行）
2. `/api/config` 全量 export 在 VPN 模式下隐藏入口，改为单条操作

**v2 chunking** 留接口空间不实现：`APIResponse` 加 `seq?: number, total?: number` 字段，extension 切片，BridgeAdapter 重组。

### 6.5 Go 侧 · `/api/events` 事件队列

唯一新增的 Go 端点。WS 推送和 polling 都从同一队列读取。

```go
// gui/api/events.go

type Event struct {
    Cursor int64           `json:"cursor"`
    Type   string          `json:"type"`
    Data   json.RawMessage `json:"data"`
    Time   time.Time       `json:"time"`
}

type EventQueue struct {
    mu       sync.RWMutex
    ring     []Event       // 容量 1024
    head     int
    full     bool
    cursor   int64
    cond     *sync.Cond
}

func (q *EventQueue) Push(typ string, data any)
func (q *EventQueue) Tail(since int64, max int) (events []Event, latest int64, gap bool)
func (q *EventQueue) Wait(ctx context.Context, since int64) ([]Event, int64, bool, error)
```

**事件来源：** Engine 现有 `EventBus`（`engine/eventbus.go`）订阅一个 sink，把事件 push 进 `EventQueue`。SSE 旧路径与新 EventQueue 并存。

**保留窗口：** 1024 条或 60s，先到为准。

**端点：**
- `GET /api/events?since=N&max=100` —— Bridge 轮询
- `WS /ws/events?since=N` —— HTTP adapter 长连接
- `GET /api/healthz` —— **新增**，bridge 探活专用，返回 `{status: "ok", apiAddr: "..."}`，用于 §11.1 的 handshake 检查

**事件类型清单（初版）：**

```
engine.state         { state: "starting"|"running"|"stopping"|"stopped" }
engine.error         { message: string }
server.connected     { id: string }
server.disconnected  { id: string, reason: string }
server.latency       { id: string, ms: number }
subscription.synced  { id: string, count: number }
mesh.peer_joined     { vip: string }
mesh.peer_left       { vip: string }
log.line             { level, msg, ts, source }
```

**gap 处理：** UI 收到 `gap: true` 时显示一次性 toast"事件流断点已恢复"，并重新拉取关键数据全量刷新。

### 6.6 改动清单

| 文件 | 改动 | 行数估 |
|---|---|---|
| `gui/web/src/lib/platform/index.ts` | sniff `window.ShuttleBridge` | +5 |
| `gui/web/src/lib/data/*` | **新模块** | +650 |
| `gui/web/src/lib/api/*` | 改用 `adapter.request()` 替代 `fetch()` | ~200 改 |
| `gui/web/src/features/**/use*.ts` | WS 直连改 `useSubscription(topic)` | ~100 改 |
| `mobile/ios/Shuttle/ShuttleApp.swift` | 注册 APIBridge + 注入 bootstrap JS；删除 ~150 行 fallback HTML | -100 净 |
| `mobile/ios/Shuttle/VPNManager.swift` | 暴露 `sendToExtension(_:timeout:completion:)` | +20 |
| `mobile/ios/Shuttle/APIBridge.swift` | **新文件** | +130 |
| `mobile/ios/SharedBridge/Sources/*.swift` | **新 SPM** —— `APIRequest` / `APIResponse` | +60 |
| `mobile/ios/ShuttleExtension/PacketTunnelProvider.swift` | `handleAppMessage` 加 envelope 分支 + `forwardToLocalAPI` | +80 |
| `gui/api/events.go` | **新文件** —— `EventQueue` + handler | +180 |
| `gui/api/server.go` | 注册 `/api/events` + `/ws/events` + `/api/healthz` | +25 |
| `gui/api/healthz.go` | **新文件** —— `/api/healthz` handler | +30 |
| `engine/eventbus.go` | 加订阅者 push 事件进 `EventQueue` | +30 |
| `gui/api/servers.go` | 加 `?page=N&size=50` | +30 |
| `mobile/ios/Shuttle/FallbackHandler.swift` | **新文件** —— 接收 fallback 信号重载 inline HTML | +40 |

净增约 +900 行新代码 + 改动既有 ~300 行。

### 6.7 SPA 不会改的部分

明确划出避免范围蔓延：

- 6 大页面 `.svelte` 组件 —— 0 改动
- `lib/platform/` 已有 5 个能力（engine/permission/scan/share/openExternal）—— 0 改动
- `app/AppShell.svelte` / `BottomTabs` / `Rail` / `Sidebar` —— 0 改动
- i18n / 主题 / 路由迁移 —— 0 改动
- Wails 桌面 / Android / 浏览器 / iOS 代理模式 —— 0 行为变化

## 7. 维护性

两个机制把跨 adapter 漂移风险降到可控：

### 7.1 Topic Registry —— 单一声明源

`topicConfig` 是双 adapter 唯一的真相源。加新 subscription = 在 `topics.ts` 加一条 + 在 Go 侧实现对应的 REST + WS endpoint。两个 adapter 自动适应。

### 7.2 Conformance 测试套件

```typescript
describe.each([
  ['http', () => new HttpAdapter(mockFetch, mockWS)],
  ['bridge', () => new BridgeAdapter(mockBridge)],
])('%s adapter', (_, makeAdapter) => {
  describe('request', () => {
    it('parses 200 JSON body')
    it('returns undefined for 204')
    it('rejects with ApiError on 4xx including server message')
    it('rejects with TransportError on network failure')
    it('honors AbortSignal mid-flight')
    it('injects auth header when token present')
  })
  describe('subscribe (snapshot)', () => {
    it('emits current value within 50ms of subscription')
    it('emits again on value change')
    it('does not emit when value unchanged (deep equal)')
    it('current() returns last emitted value')
    it('unsubscribe stops emissions')
    it('multiple subscribers all receive updates')
  })
  describe('subscribe (stream)', () => {
    it('does not emit until new event arrives')
    it('current() is undefined')
    it('honors cursor — does not replay events before cursor')
    it('reconnects with last cursor after transport failure')
  })
  describe('connectionState', () => {
    it('transitions idle → connecting → connected on first subscribe')
    it('transitions back to idle when last subscriber leaves')
    it('transitions to error and back on transport failure + recovery')
  })
})
```

任一 adapter 行为分叉，CI 即时失败。

### 7.3 Swift 层 endpoint-agnostic

`APIBridge` 与 `forwardToLocalAPI` 只搬运信封，不感知 endpoint。新增 endpoint 不需要改 Swift 代码。

### 7.4 加新功能的成本对比

| 操作 | 现状（HTTP-only + fallback HTML） | 本方案 |
|---|---|---|
| 加新 REST endpoint | Go handler + TS client + 调用点 | 同（adapter 透明） |
| 加新 subscription | Go WS handler + 调用点 | Go WS + REST endpoint + Topic Registry 一行 + 调用点 |
| 加新 platform 能力 | 无变化 | 无变化 |
| iOS 引擎 API 变更 | 无关 | Swift 不感知 endpoint，零修改 |

## 8. 测试策略

### 8.1 测试金字塔

| 层 | 工具 | 范围 | 通过条件 |
|---|---|---|---|
| TS 单元 | vitest | DataAdapter conformance + topicConfig 引用完整性 | 双 adapter 全部 20+ 项断言通过 |
| TS 集成（jsdom） | vitest + 假 `window.ShuttleBridge` | BridgeAdapter 全链路 | 假时钟下时序确定 |
| iOS 单元（XCTest） | XCTest | APIBridge 编解码、`handleAppMessage` envelope 分支 | mock `NETunnelProviderSession` |
| iOS UI（XCUITest） | XCUIApplication + WKWebView 探针 | SPA 在 VPN 模式真实加载 | `testSPALoadsInVPNMode` |
| 手动 smoke（设备） | `docs/mobile-smoke.md` | 真机 iPhone | 11 项清单 |

### 8.2 testSPALoadsInVPNMode 雏形

```swift
func testSPALoadsInVPNMode() throws {
    let app = XCUIApplication()
    app.launchEnvironment["FORCE_VPN_MODE"] = "1"
    app.launch()
    let allow = springboard.buttons["Allow"]
    if allow.waitForExistence(timeout: 5) { allow.tap() }
    let bottomTabs = app.staticTexts["Now"].firstMatch
    XCTAssertTrue(bottomTabs.waitForExistence(timeout: 15),
                  "SPA Now 标签未在 VPN 模式下渲染")
    let meshTab = app.staticTexts["Mesh"].firstMatch
    XCTAssertTrue(meshTab.waitForExistence(timeout: 5),
                  "Mesh 入口缺失，可能仍走 fallback HTML")
}
```

### 8.3 真机 smoke 增量条目（追加进 `docs/mobile-smoke.md`）

```
## iOS VPN 模式（真机）

- [ ] 首次连接，系统 VPN 权限对话框出现，允许后 SPA 首屏 <3s
- [ ] Now 页 Power 按钮：Connect → Connected 反馈 ≤2s
- [ ] Servers 页：列表加载、QR 扫码导入、编辑名称、删除全部成功
- [ ] Servers 页：500+ 条订阅同步后列表分页流畅
- [ ] Activity → Logs：日志行 1s 内出现新行
- [ ] Activity → Logs：连续 5 分钟无丢行
- [ ] Settings 修改主题立即生效，重启应用后保留
- [ ] 后台 30s → 前台：SPA 自动恢复，无白屏，状态正确
- [ ] 后台 5min → 前台：可能触发"事件流断点"toast，全量刷新后正常
- [ ] 强制关闭进程后重新打开（VPN 仍开）：SPA 重新加载并连上 bridge
- [ ] 切到代理模式 → 切回 VPN 模式：SPA 状态平滑切换
```

## 9. 灰度发布

三阶段，每阶段都可秒级回退。

### Phase α —— feature flag off（1 天落地）

- bridge 全套代码合入主线
- 默认仍走旧 fallback HTML
- 测试入口：URL 加 `?bridge=1` 强制启用
- 退出条件：内部测试 + conformance + UI 测试全绿

### Phase β —— 默认 on + safety net（5–7 天 wall time，含 ~2 工作日修复）

- bridge 默认启用
- 保留 fallback HTML 作为 safety net：bridge bootstrap 失败时自动回落
- TestFlight 内部 + 早期外部
- 退出条件：72h 真机使用，bridge 失败率 <0.1%

### Phase γ —— 清理（1 天）

- 删除 fallback HTML
- 删除 extension 端的 string command（`"status"` / `"stop"` / `"logs"`）
- 主分支只剩一条路径

**Feature flag 实现：** WKWebView 启动时 query string `?bridge=1|0|auto`。
- `auto` —— 自动探测（默认）
- `0` —— 强制旧 fallback
- `1` —— 强制新 bridge（即使探测失败也不回落，方便复现 bug）

## 10. 性能预算

| 指标 | 目标 | 测量方式 |
|---|---|---|
| SPA 冷启动（前台首像素） | <2.5s | XCTest 自动 + 真机 Instruments |
| Bridge request RTT p50 | <50ms | BridgeAdapter 内 `performance.now()` 标记 |
| Bridge request RTT p95 | <300ms | 同上 |
| Bridge 长尾 p99 | <1.5s | 同上，超阈值打 warn |
| 静态 polling CPU 占用 | <3% on iPhone 14 | Xcode Instruments Time Profiler，应用静置 5min |
| Extension 内存 | <40MB 稳态 / <50MB 峰值 | extension 内 `mach_task_basic_info` 自检 |
| 订阅泄漏 | 0 over 30min idle | Instruments Allocations，订阅/取消 1000 次后内存回归基线 |
| 长时连接稳定性 | 24h 不重启 bridge | 真机后台连续 24h，bridge 失败率 <0.05% |

任何指标 P0 红线超支阻塞 Phase β 进入 Phase γ。

## 11. 回退触发器

### 11.1 硬触发 —— 自动回退到 fallback HTML（Phase β 期间）

| 条件 | 检测点 | 动作 |
|---|---|---|
| `window.ShuttleBridge` 注入失败 | SPA 启动 boot.ts 1s 超时 | `webkit.messageHandlers.fallback.postMessage` → Swift 重载 fallback HTML |
| Bridge handshake 超时 | `adapter.request('/api/healthz')` 5s 超时 | 同上 |
| Extension 持续 5xx | 连续 3 次 `/api/healthz` 返回 5xx | 同上 |
| Bridge bootstrap JS 抛错 | 全局 `unhandledrejection` 中带特定标签 | 同上 |

**boot.ts 探活流程：**

```typescript
// gui/web/src/app/boot.ts (新增 ~30 行)
async function bootstrapAdapter() {
  // 等 100ms 让 userScript 注入；保险起见再检查
  if (!window.ShuttleBridge) {
    await new Promise(r => setTimeout(r, 1000))
    if (!window.ShuttleBridge && location.search.includes('bridge=1')) {
      // 强制 bridge 模式但没注入 → 立即回退
      requestFallback('ShuttleBridge not injected')
      return null
    }
    if (!window.ShuttleBridge) return new HttpAdapter()  // 非 iOS VPN 模式
  }
  const bridge = new BridgeAdapter()
  try {
    await Promise.race([
      bridge.request({ method: 'GET', path: '/api/healthz' }),
      timeout(5000),
    ])
    return bridge
  } catch (err) {
    requestFallback(String(err))
    return null
  }
}

function requestFallback(reason: string) {
  window.webkit?.messageHandlers?.fallback?.postMessage({
    reason, timestamp: Date.now(),
  })
}

window.addEventListener('unhandledrejection', (ev) => {
  if (String(ev.reason).includes('[bridge-fatal]')) requestFallback(String(ev.reason))
})
```

**FallbackHandler.swift：**

```swift
final class FallbackHandler: NSObject, WKScriptMessageHandler {
    private weak var webView: WKWebView?
    private let inlineHTML: String   // 复用现有 fallback HTML 字符串

    func userContentController(_ ucc: WKUserContentController,
                               didReceive msg: WKScriptMessage) {
        guard let body = msg.body as? [String: Any],
              let reason = body["reason"] as? String else { return }
        Logger.app.warning("Bridge fallback triggered: \(reason)")
        DispatchQueue.main.async {
            self.webView?.loadHTMLString(self.inlineHTML, baseURL: nil)
        }
    }
}
```

### 11.2 软退化 —— 不回退，提示用户

| 条件 | 反应 |
|---|---|
| `connectionState === 'error'` 持续 >30s | 顶栏红点 + 文字"重连中…" |
| RTT p95 持续 >2s 超过 5min | 一次性 toast "网络较慢" |
| 收到 `gap: true` 事件 | 一次性 toast "事件流断点已恢复" + 自动重新拉取关键数据 |

### 11.3 度量上报

bridge 失败率、RTT 分布、回退触发次数 —— Phase β 期间在主进程一个本地计数器（`UserDefaults`），Settings 页加一个"诊断"小段显示给用户。**不打远程上报**（Shuttle 是隐私敏感产品，VPN 工具不应静默打点）。

## 12. 风险

| 风险 | 影响 | 对策 |
|---|---|---|
| Bridge adapter 大数据传输 | servers 列表上千条 base64+IPC 卡顿 | 分页（默认 50 条/页），SPA 业务侧已有分页模式自然适配 |
| WS 事件保真度 | 离散事件被轮询丢失 | Go 侧 `/api/events?since=cursor` 持久队列 60s/1024 条 |
| Extension 50MB 内存上限 | 高负载下被系统杀 | 压测脚本 100 req/s × 5 分钟；extension 自检超 40MB 时主动响应 5xx 给 healthz，触发 §11.1 硬回退 |
| iOS Apple SDK 变更 | 未来 sendProviderMessage 行为变化 | 共享 SPM 抽象编解码契约，Swift 端可独立升级 |
| 真机网络环境差异 | 蜂窝/弱网下 RTT 长尾飙升 | 软退化 toast 提示用户；Bridge 单请求 30s 超时（§5.2 APIBridge 已配） |

## 13. 范围外（明确排除）

- 信封 chunking —— v2 工作，触发条件：`/api/config` 全量超阈值且没有更优分页方案
- etag-based diff 优化 —— profiling 证明必要才做
- cursor 持久化跨刷新 —— iOS WebView 刷新极少
- 修改 Android 任何路径
- 修改 Wails 桌面任何路径
- 修改 iOS 代理模式任何路径
- 引入新的 SPA UI 概念 —— 这是底层重构

## 14. 验收清单

整个 spec 落地 ≡ 全部勾选：

- [ ] DataAdapter conformance 套件双 adapter 全绿（vitest）
- [ ] iOS VPN 模式 SPA 6 大页面手动 smoke 全部通过
- [ ] `docs/mobile-smoke.md` iOS VPN 模式 11 项清单全过
- [ ] Bridge RTT p95 <300ms（10 设备样本）
- [ ] Extension 稳态内存 <40MB（24h 真机）
- [ ] CI `build-mobile.yml` iOS 任务在新 bridge 路径下绿
- [ ] Phase β TestFlight 72h，失败率 <0.1%
- [ ] Fallback HTML 与所有 string command 在 Phase γ 全部删除

## 15. 工作量估算

| 阶段 | 内容 | 工作日 |
|---|---|---|
| A | DataAdapter 抽象骨架 + conformance 套件 | 1 |
| B | HttpAdapter 落地 + 现有 SPA 数据层迁移 | 2 |
| C | BridgeAdapter 落地 + APIBridge.swift + handleAppMessage 扩展 | 2 |
| D | Go `/api/events` 队列 + 事件 publisher | 1 |
| E | servers 分页 + config export 路径调整 | 0.5 |
| F | iOS XCTest + UI 测试 | 0.5 |
| G | Phase α 测试与修复 | 1 |
| H | Phase β TestFlight wall time | 5–7 wall（含 ~2 工作日修复） |
| I | Phase γ 清理 | 0.5 |

**净工作日：~10**，wall time 含 TestFlight 等待 ~14 天。
