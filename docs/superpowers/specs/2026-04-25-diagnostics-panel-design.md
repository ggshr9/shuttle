# Settings → Diagnostics 面板 —— 设计文档

> 状态：设计审核中
> 作者：Claude（与 user 通过 brainstorming 推导）
> 日期：2026-04-25
> 关联：`docs/superpowers/specs/2026-04-24-ios-vpn-mode-spa-design.md` §11.3
> 关联：`docs/superpowers/plans/2026-04-25-post-merge-followups.md` "Settings → Diagnostics 面板"

## 1. 背景

`docs/superpowers/specs/2026-04-24-ios-vpn-mode-spa-design.md` §11.3 规定 iOS VPN 模式下要在 Settings 页加一个"诊断"小段，显示 bridge 失败率、RTT 分布、回退触发次数，作为 Phase β TestFlight 的前置项。原 spec 的实现路线是把计数器放在 Swift 端 `UserDefaults`，因为 fallback 触发时 SPA 会被替换成 HTML，JS 内存计数器随之丢失。

post-merge followups 文档（2026-04-25）措辞略有不同：暗示在 `lib/data/connection-state.ts` 这一层导出累计计数。两份措辞催生了一个选择：

- A. 纯 Swift native（按 spec 字面）
- B. 纯 JS localStorage
- C. 混合（native 存 fallback 计数 + JS 存 RTT/error）

均经评估后舍弃。最终方案 D 以下文呈上。

## 2. 目标 / 非目标

### 目标

- iOS VPN 模式下在 Settings 加一个 Diagnostics sub-page，显示 spec §11.3 规定的三项：bridge 失败率、RTT 分布、fallback 触发次数
- Diagnostics 实现**全平台统一**：iOS bridge / iOS proxy / Android / Wails 桌面 / Web 浏览器使用同一份代码与同一份 UI
- 仅本地存储，**不打远程上报**
- 用户可在 UI 主动 Reset 计数器
- 重构现有 `DataAdapter` 抽象层添加诊断职责，无平台分支代码

### 非目标

- 不改任何 iOS Swift native 代码
- 不增加任何新 Go API endpoint
- 不引入远程上报 / 远程聚合
- 不做 RTT histogram / 长尾分析 —— 滚动窗口 + p50/p95 已足够覆盖 spec §11.3 要求
- 不持久化 RTT 样本（每次 SPA 启动从 0 开始累积，符合"per-session 健康度"语义）

## 3. 架构

诊断职责注入到现有的 `DataAdapter` 抽象层，零平台分支：

```
DataAdapter (interface, lib/data/types.ts)
├─ request(opts)              ← 已有
├─ subscribe(topic)           ← 已有
├─ connectionState            ← 已有 (per-topic ok/error)
└─ diagnostics: Diagnostics   ← 新增（READ-ONLY snapshot 接口）

Diagnostics (lib/data/diagnostics.ts, ~100 行)
├─ recordRequest(durationMs, ok, errorReason?)   ← adapter 内部调
├─ recordFallback(reason)                        ← boot.ts 调，同步落 localStorage
├─ snapshot(): DiagnosticsSnapshot               ← UI 读（响应式：内部字段是 $state，UI 通过 $derived 自动跟踪）
└─ reset()                                        ← UI 主动清空
```

`BridgeAdapter` / `HttpAdapter` 各自构造时 `new Diagnostics()` 挂到 `this.diagnostics`，并在 `request()` 完成路径（无论成功或失败）调 `recordRequest()`。

UI 这一侧：
- 新增 `features/settings/sub/Diagnostics.svelte`
- `nav.ts` 的 `diagnostics` section 加一项 `{ slug: 'diagnostics', icon: 'activity', section: 'diagnostics' }`，置于 `logging` 之前
- `SettingsPage.svelte` 的 `pageMap` 加 `diagnostics: Diagnostics`

### 3.1 关键设计选择

1. **fallback 事件持久化用 localStorage 而非 UserDefaults**：localStorage 的 `setItem` 在 WKWebView 里是同步落盘的，写完再 `postMessage` 不会丢。spec §11.3 选 UserDefaults 是当时没考虑到这个写入顺序，本 spec 视为 §11.3 的实现细节修订。
2. **RTT 由 adapter 自测，不依赖 Swift 计时**：request 进出的时间戳都在 JS 这一层，测的就是"用户感受到的端到端"，更准确。
3. **UI 完全平台无关**：`Diagnostics.svelte` 只读 `adapter.diagnostics.snapshot()`，不知道当前是 BridgeAdapter 还是 HttpAdapter。所有运行时同一份组件。
4. **桌面/浏览器同样受益**：`HttpAdapter` 也实例化 Diagnostics，桌面用户也能在 Settings 看自己的 API 健康度。

## 4. 组件细节

### 4.1 `lib/data/diagnostics.ts`

完整签名：

```typescript
const STORAGE_KEY = 'shuttle.diag.fallbacks'
const MAX_FALLBACKS = 10
const RTT_WINDOW = 100
const MIN_RTT_SAMPLES = 10

export interface DiagnosticsSnapshot {
  // since session start (in-memory)
  requestsTotal: number
  requestsErr: number
  errorRate: number              // 0 if requestsTotal===0
  rttP50: number | null          // ms; null if <MIN_RTT_SAMPLES
  rttP95: number | null
  lastError: { reason: string; at: number } | null   // wall clock ms

  // cross-session (localStorage)
  fallbacks: { reason: string; at: number }[]        // up to MAX_FALLBACKS, oldest first
  fallbacksTotal: number                             // monotonic count, never decreases
}

export class Diagnostics {
  // in-memory $state
  #requestsTotal = $state(0)
  #requestsErr = $state(0)
  #rttSamples: number[] = []
  #lastError = $state<{reason: string; at: number} | null>(null)
  // cross-session $state
  #fallbacks = $state<{reason: string; at: number}[]>([])
  #fallbacksTotal = $state(0)

  constructor(private storage: Storage = globalThis.localStorage) {
    this.hydrate()
  }

  recordRequest(durationMs: number, ok: boolean, reason?: string): void
  recordFallback(reason: string): void
  snapshot(): DiagnosticsSnapshot
  reset(): void

  static persistDirect(reason: string, storage?: Storage): void  // 极早期 fallback 路径

  private hydrate(): void
  private persist(): void
}
```

**关键决定**：

- 用 Svelte 5 `$state` 让 UI 直接订阅 —— 一行 `const snap = $derived(adapter.diagnostics.snapshot())`，无需手写 listener
- RTT 不存 histogram，存原始 samples ring buffer —— 100 个 number ≈ 800 字节，简单、足够算 p50/p95
- `recordFallback` 内部直接 `persist()` 同步写，`boot.ts` 一行调用就够
- `lastError.reason` 来自 `TransportError.message` / `ApiError.code+message`，由 adapter 传入

### 4.2 `DataAdapter` interface 改动

`lib/data/types.ts`：

```typescript
export interface DataAdapter {
  request<T>(opts: RequestOptions): Promise<T>
  subscribe<K extends TopicKey>(topic: K, opts?: SubscribeOptions<K>): Subscription<TopicValue<K>>
  readonly connectionState: ReadableValue<ConnectionState>
  readonly diagnostics: Diagnostics    // ← 新增
}
```

`BridgeAdapter` / `HttpAdapter` 构造时各自 `this.diagnostics = new Diagnostics()`。

### 4.3 `request()` 内的埋点

`HttpAdapter.request()` 用 try/finally 包：

```typescript
async request<T>(opts: RequestOptions): Promise<T> {
  const t0 = performance.now()
  let ok = false
  let reason: string | undefined
  try {
    const result = await this.#requestImpl<T>(opts)  // 现有 body 抽到 private 方法
    ok = true
    return result
  } catch (err) {
    reason = err instanceof Error ? err.message : String(err)
    throw err
  } finally {
    this.diagnostics.recordRequest(performance.now() - t0, ok, reason)
  }
}
```

`BridgeAdapter` 同样改造。这是把现有 try/finally 再外包一层 timing，**不是**重写。

### 4.4 `boot.ts` 改动

```typescript
function requestFallback(reason: string, adapter?: DataAdapter | null): void {
  if (typeof window === 'undefined') return
  // CRITICAL: persist BEFORE postMessage, so the localStorage write
  // commits before the SPA is torn down.
  try {
    if (adapter) {
      adapter.diagnostics.recordFallback(reason)
    } else {
      Diagnostics.persistDirect(reason)
    }
  } catch { /* don't block fallback on telemetry */ }
  window.webkit?.messageHandlers?.fallback?.postMessage({ reason, timestamp: Date.now() })
}
```

调用点：
- `bootstrapAdapter` 里 bridge handshake 失败 / 5xx / 超时 → 传入已构造的 `bridge` 实例
- 全局 `unhandledrejection` 兜底（`[bridge-fatal]`）→ 传入 `getAdapter()` 当时的实例（可能为 null）
- `window.ShuttleBridge` 完全没注入的极早期路径 → 不传 adapter，走 `persistDirect`

### 4.5 UI — `features/settings/sub/Diagnostics.svelte`

布局（参考现有 sub-page 风格）：

```
┌─ PageHeader: "Diagnostics" ───────────┐
│                                        │
│  Bridge Health                         │
│  ┌─ Stats Grid ───────────────────┐    │
│  │  Requests       1,247          │    │
│  │  Errors         3 (0.24%)      │    │
│  │  RTT p50        18 ms          │    │
│  │  RTT p95        42 ms          │    │
│  └────────────────────────────────┘    │
│                                        │
│  Last Error                            │
│  ┌─ Card ─────────────────────────┐    │
│  │  TransportError: timeout       │    │
│  │  3 minutes ago                 │    │
│  └────────────────────────────────┘    │
│                                        │
│  Fallback History                      │
│  ┌─ List (most recent first) ─────┐    │
│  │  • timeout    2026-04-25 14:22 │    │
│  │  • 5xx ×3     2026-04-24 09:10 │    │
│  └────────────────────────────────┘    │
│  Total triggers: 2                     │
│                                        │
│  [ Reset counters ]                    │
└────────────────────────────────────────┘
```

样式继承 `--shuttle-*` token，PageHeader 与现有 sub-page 一致。

i18n keys 全部在 `settings.diagnostics.*` 命名空间：

```
settings.diagnostics.title
settings.diagnostics.section.bridgeHealth
settings.diagnostics.section.lastError
settings.diagnostics.section.fallbackHistory
settings.diagnostics.stat.requests
settings.diagnostics.stat.errors
settings.diagnostics.stat.errorRate
settings.diagnostics.stat.rttP50
settings.diagnostics.stat.rttP95
settings.diagnostics.empty.noSamples
settings.diagnostics.empty.noErrors
settings.diagnostics.empty.noFallbacks
settings.diagnostics.action.reset
settings.diagnostics.action.confirmReset
settings.diagnostics.relative.justNow
settings.diagnostics.relative.minutesAgo
settings.diagnostics.relative.hoursAgo
settings.diagnostics.relative.daysAgo
```

最少 zh-CN 和 en 两份（项目已有的两个语言）。`lastError.reason` 不翻译（`TransportError`/`ApiError` 的英文 message，给开发者排错用）。

**相对时间渲染**：项目当前没有 helper，本 spec 不引入新依赖，UI 内联用浏览器原生 `Intl.RelativeTimeFormat`。阈值规则：

```
diff < 60s              → t('settings.diagnostics.relative.justNow')        // "刚才" / "just now"
60s ≤ diff < 60min      → rtf.format(-Math.floor(diff/60_000), 'minute')
60min ≤ diff < 24h      → rtf.format(-Math.floor(diff/3_600_000), 'hour')
diff ≥ 24h              → rtf.format(-Math.floor(diff/86_400_000), 'day')
```

只在 UI 渲染时计算，不影响数据层。

## 5. 数据流

```
┌────────────── 运行时 (in-memory) ──────────────┐

  adapter.request(opts)
    │ t0 = performance.now()
    │ try { ... }
    │ finally {
    │   diagnostics.recordRequest(dt, ok, reason)
    │     ├─ requestsTotal++
    │     ├─ if !ok: requestsErr++, lastError = {reason, t}
    │     └─ rttSamples.push(dt) [ring 100]
    │ }
    │
    ↓ Svelte $state 触发响应式
  Diagnostics.svelte 渲染最新值

┌────────────── 跨会话 (localStorage) ───────────┐

  boot.ts: requestFallback(reason)
    │ ① diagnostics.recordFallback(reason)  ← 同步
    │     ├─ fallbacks.push({reason, at}) [last 10]
    │     ├─ fallbacksTotal++
    │     └─ localStorage.setItem(KEY, JSON)  ← 落盘
    │ ② postMessage('fallback')              ← Swift 替换 WebView
    │
    ↓ SPA 被卸载，HTML fallback 接管
    ↓ 用户重新打开 / 引擎重启 → SPA 重新加载
    │
  boot.ts → new BridgeAdapter() → new Diagnostics()
    │ Diagnostics.hydrate()
    │   ├─ JSON.parse(localStorage.getItem(KEY))
    │   ├─ fallbacks = parsed.entries (validated)
    │   └─ fallbacksTotal = parsed.total
    │
    ↓ Diagnostics.svelte 显示历史
```

两条路径完全独立、互不阻塞：
- in-memory 那条只在请求 finally 里调一次方法，零 IO
- localStorage 那条只在 fallback 触发（理想 0 次）和启动 hydrate（每次启动 1 次）时碰，amortized 成本可忽略

### 5.1 极早期 fallback 路径

`window.ShuttleBridge` 完全没注入时，boot.ts 还没构造 adapter。走 `Diagnostics.persistDirect(reason)` 静态方法直接写同一份 localStorage KEY。下次启动时，新构造的 `Diagnostics` 实例 `hydrate()` 自然读到。

### 5.2 Reset 流程

```
adapter.diagnostics.reset()
  ├─ in-memory 全清零（requestsTotal/Err、rttSamples、lastError、fallbacks、fallbacksTotal）
  └─ localStorage.removeItem(KEY)
  ↓
$state 触发 → UI 即时刷新到空状态
```

UI 上点 Reset 后弹一次确认，避免误触。

## 6. 错误处理与边界

### 6.1 localStorage 失败模式

| 场景 | Diagnostics 行为 |
|---|---|
| Storage 被禁用（隐私模式） | `setItem` throw → try/catch 静默吞掉，退化成 in-memory only |
| Quota 超限（5MB 满） | 同上吞掉。本设计 payload <2KB，正常碰不到 |
| 已存内容 JSON 损坏 / schema 漂移 | `hydrate()` 内 try/catch + 字段类型校验，损坏 entries 丢弃，counter 视为 0 |
| 多 WebView 写入冲突（理论） | iOS WebView 单实例，不监听 storage 事件 |

**Diagnostics 永不抛错给调用方** —— 它是观测工具，不能因为它自己出问题导致 fallback 失败。所有 setItem/getItem/JSON.parse 全包 try/catch。

### 6.2 数据有效性

| 字段 | 边界条件 | 处理 |
|---|---|---|
| `errorRate` | `requestsTotal === 0` | snapshot 返回 `0`，UI 显示 `—`（区分"没数据"和"完美"）|
| `rttP50/P95` | `samples.length < 10` | 返回 `null`，UI 显示 `—` 加 tooltip "需要更多样本" |
| `lastError.at` | timestamp 在未来 / NaN | 渲染时 `Math.min(at, Date.now())` 兜底 |
| `fallbacks` 中过期条目 | 不过期，只受 last 10 限制 | timestamp diff 由 UI 渲染时判断 |
| 时区 | 全部用 UTC ms | UI 渲染时用 `Intl.RelativeTimeFormat`（相对时间）+ `toLocaleString()`（绝对时间）|

### 6.3 性能边界

| 关注点 | 数字 | 评估 |
|---|---|---|
| `recordRequest` 开销 | `++` × 2 + `Date.now()` + array push/shift | <1µs；request 本身几 ms 起，可忽略 |
| `recordFallback` 开销 | JSON.stringify + setItem | ~100µs；触发频率 N/天，无影响 |
| 内存常驻 | 100 × 8B (RTT) + 10 × ~50B (fallbacks) ≈ 1.5KB | 远低于 extension 50MB 上限 |
| Snapshot 开销 | 100 个数排序求 p50/p95 | <50µs；UI 渲染节流由 Svelte 负责 |

### 6.4 隐私

- localStorage 数据仅本地，spec §11.3 明确禁止远程上报
- `lastError.reason` 可能含 `127.0.0.1:<port>` 和 `/api/*` 路径，无用户身份信息
- `fallbacks[].reason` 同上
- Reset 按钮仅清本地存储

## 7. 测试

### 7.1 `lib/data/__tests__/diagnostics.spec.ts`（新增）

| 用例 | 验证什么 |
|---|---|
| `recordRequest()` 累计计数 / 错误 / lastError 正确 | 基本 happy path |
| `recordRequest()` RTT 滚动窗口超过 100 时 shift 旧值 | ring buffer 边界 |
| `snapshot()` p50/p95 在 <10 samples 时返回 null | MIN_RTT_SAMPLES 边界 |
| `snapshot()` p50/p95 在偶数/奇数样本数下计算正确 | 中位数算法边界 |
| `snapshot()` errorRate 在 requestsTotal=0 时为 0 | 零除防御 |
| `recordFallback()` 写入 localStorage（mock storage） | persist 路径 |
| `recordFallback()` 超过 MAX_FALLBACKS=10 时丢弃最早一条 | cap 边界 |
| 累计的 `fallbacksTotal` 单调递增不受 cap 影响 | 计数语义 |
| `hydrate()` 从合法 localStorage 读出条目 | 启动恢复 |
| `hydrate()` 在损坏 JSON 时静默退化为空 | 容错 |
| `hydrate()` 丢弃 schema 不合法的条目 | schema 防御 |
| `persistDirect()` 静态方法在无 adapter 实例时也能写 | 极早期 fallback |
| `recordFallback` 后 `persistDirect` + 之后 `hydrate` 能合并不重复 | 同一 KEY 路径一致 |
| `setItem` throw QuotaExceeded → 不抛给 caller | 防御性 |
| `reset()` 清 in-memory + localStorage | UI reset 路径 |

注入 `Storage` mock（`Map<string, string>` 包成 Storage 接口），无浏览器依赖。

### 7.2 适配器埋点测试

| 用例 | 文件 |
|---|---|
| `HttpAdapter.request()` 成功后 `diagnostics.requestsTotal++` | `http-adapter.spec.ts` 加 case |
| `HttpAdapter.request()` 抛 ApiError 后 `requestsErr++` 且 lastError.reason 含 status | 同上 |
| `HttpAdapter.request()` 抛 TransportError 后 lastError 反映底层 message | 同上 |
| `BridgeAdapter.request()` 同上三项 | `bridge-adapter.spec.ts` 加 case |
| 现有 conformance suite 验证两 adapter 行为一致 | `conformance.spec.ts` 加新 section |

### 7.3 boot.ts 测试

| 用例 | 验证 |
|---|---|
| `requestFallback()` 在 adapter 已 setAdapter 时调 `adapter.diagnostics.recordFallback` | 主路径 |
| `requestFallback()` 在 adapter 未构造时 fallback 到 `Diagnostics.persistDirect` | 极早期路径 |
| `recordFallback` 抛错时 `postMessage` 仍然被调用 | 不阻塞 fallback |
| localStorage 写入在 postMessage 之前完成（mock 验证调用顺序） | 同步落盘契约 |

### 7.4 UI 测试 — `Diagnostics.svelte`

| 用例 | 渲染什么 |
|---|---|
| 空状态（无请求、无 fallback）显示三处 empty 文案 | empty.* i18n |
| 有 RTT 但 <10 samples → p50/p95 显示 `—` 加 tooltip | 边界态 |
| `lastError` 存在 → 显示 reason + 相对时间 | 主要功能 |
| Fallback list 按 most-recent-first 渲染 | 排序契约 |
| Reset 按钮点击调 `adapter.diagnostics.reset()` | 用户操作 |
| 响应式更新：mock adapter 改变 snapshot → DOM 自动更新 | $state 集成 |

vitest + jsdom，注入 mock adapter，验证 textContent。

### 7.5 集成（host-safe）

加一个 vitest 集成 case：构造真实 `HttpAdapter`，mock `fetch` 返回固定 latency 的 200 / 500 / network error；跑 50 个请求，断言 `snapshot().rttP50/P95/errorRate` 数值合理。不进 sandbox（不涉及网络/系统状态）。

### 7.6 不测的（明确排除）

- iOS Swift 端 —— 本设计零 native 改动
- 真实 fallback 触发的 SPA 销毁 —— WebView 集成行为，已在 §11.1 测试范围内
- 性能基准 —— 算法复杂度足够低

总用例数估计：~25 个 spec.ts 用例 + 5 个 conformance + 6 个 svelte + 2 个集成 ≈ 38 个新测试。

## 8. spec §11.3 修订说明

本 spec 与 `2026-04-24-ios-vpn-mode-spa-design.md` §11.3 有以下偏差，需在 §11.3 加修订脚注或在主 spec 顶部加 errata：

| §11.3 原文 | 本 spec 修订 | 理由 |
|---|---|---|
| "在主进程一个本地计数器（UserDefaults）" | localStorage 替代 UserDefaults | 写入语义同步落盘 + 跨平台 API 一致 |
| 仅 iOS bridge 模式 | 全平台统一 | DataAdapter 是已有的平台抽象，注入诊断职责无平台分支 |
| 仅 fallback 触发计数 + RTT 分布 + 失败率 | 同上三项 + lastError 详情 + reset 操作 | 增量价值：用户 troubleshoot 时直接看到原因 |

§11.3 的核心要求（仅本地、不打远程上报）完全保留。

## 9. 范围外（明确排除）

- 远程上报（spec §11.3 明确禁止）
- RTT histogram / 长尾分析
- 多设备聚合
- 跨 SPA 重新加载持久化 RTT 样本（per-session 已足够）
- iOS Swift native 代码改动
- 新 Go API endpoint
- Phase γ 后的 fallback 移除（属另一 PR）
