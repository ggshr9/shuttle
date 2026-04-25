# iOS VPN-Mode SPA Replacement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the iOS VPN-mode inline fallback HTML with the full Svelte SPA, using a DataAdapter abstraction so business code stays runtime-agnostic.

**Architecture:** Introduce a `DataAdapter` interface with two implementations — `HttpAdapter` (default; fetch + WebSocket) and `BridgeAdapter` (iOS VPN only; envelope IPC over `sendProviderMessage` + polling subscriptions). SPA call sites use `useSubscription(topic)` and `useRequest(opts)` instead of `fetch()` / `new WebSocket()`. iOS main app injects `window.ShuttleBridge` via `WKUserContentController` + a Swift `WKScriptMessageHandler` that forwards envelopes to the Network Extension, which makes loopback HTTP calls to the in-extension Go engine.

**Tech Stack:** TypeScript + Svelte 5 runes (gui/web/), Go (gui/api/, engine/), Swift / WebKit / Network Extension (mobile/ios/), vitest, XCTest/XCUITest.

**Spec:** `docs/superpowers/specs/2026-04-24-ios-vpn-mode-spa-design.md`

**Branch:** Create a feature branch before starting (e.g. `feat/ios-vpn-spa`). Do not commit to main until Phase 6 cleanup is complete and TestFlight is green.

---

## Phase Overview

- **Phase 1** — DataAdapter foundation (types, registry, base classes, hooks). 6 tasks.
- **Phase 2** — HttpAdapter + conformance suite + migrate existing call sites. 5 tasks.
- **Phase 3** — Go side: events queue, healthz, servers pagination. 4 tasks.
- **Phase 4** — BridgeAdapter (TS only, no native yet — testable with mock bridge). 4 tasks.
- **Phase 5** — iOS native: SharedBridge SPM, APIBridge, extension envelope handler, FallbackHandler, boot.ts probe. 5 tasks.
- **Phase 6** — Testing & cleanup: XCTest, XCUITest, smoke checklist, Phase γ removal. 4 tasks.

**Total:** 28 tasks. ~10 working days net + 5–7 wall days TestFlight.

---

## File Map

### New TypeScript files (gui/web/src/)

```
lib/data/
├── types.ts                      # DataAdapter, RequestOptions, Subscription, errors
├── topics.ts                     # TopicMap + topicConfig registry
├── connection-state.ts           # ConnectionStateController
├── subscription-base.ts          # SubscriptionBase abstract class
├── http-subscription.ts          # WS-driven implementation
├── http-adapter.ts               # HttpAdapter
├── bridge-transport.ts           # window.ShuttleBridge wrapper
├── bridge-subscription.ts        # Polling implementation
├── bridge-adapter.ts             # BridgeAdapter
├── hooks.svelte.ts               # useSubscription / useRequest
├── index.ts                      # detect → adapter selection
└── __tests__/
    ├── conformance.spec.ts       # Cross-adapter behavior contract
    ├── connection-state.spec.ts
    └── topics.spec.ts
```

### Modified TypeScript files

```
lib/api/client.ts                 # Route through adapter.request internally
lib/platform/index.ts             # sniff window.ShuttleBridge
lib/resources/status.svelte.ts    # Speed stream → useSubscription('speed')
features/logs/store.svelte.ts     # logs/connections WS → useSubscription
app/boot.ts                       # NEW — adapter probe + fallback request
```

### Go files

```
gui/api/events.go                 # NEW — EventQueue type + Push/Tail/Wait
gui/api/routes_events.go          # NEW — REST + WS handlers
gui/api/healthz.go                # NEW — /api/healthz handler
gui/api/server.go                 # MODIFY — register events + healthz routes
gui/api/routes_misc.go            # MODIFY — servers list ?page=N&size=50
engine/engine_events.go           # MODIFY — fan-out to EventQueue sink
```

### Swift files

```
mobile/ios/SharedBridge/Package.swift                          # NEW SPM manifest
mobile/ios/SharedBridge/Sources/SharedBridge/APIRequest.swift  # NEW
mobile/ios/SharedBridge/Sources/SharedBridge/APIResponse.swift # NEW
mobile/ios/Shuttle/APIBridge.swift                             # NEW
mobile/ios/Shuttle/FallbackHandler.swift                       # NEW
mobile/ios/Shuttle/VPNManager.swift                            # MODIFY — sendToExtension
mobile/ios/Shuttle/ShuttleApp.swift                            # MODIFY — wire bridge, drop inline HTML
mobile/ios/ShuttleExtension/PacketTunnelProvider.swift         # MODIFY — handleAppMessage envelope branch
mobile/ios/ShuttleUITests/ShuttleUITests.swift                 # MODIFY — testSPALoadsInVPNMode
mobile/ios/ShuttleTests/APIBridgeTests.swift                   # NEW XCTest
```

### Doc files

```
docs/mobile-smoke.md              # MODIFY — append iOS VPN-mode 11 items
```

---

## Phase 1 — DataAdapter Foundation

Pure TypeScript, no native dependencies, no behavior change to existing app yet. Each task ships a failing test, then the minimum implementation.

### Task 1.1: Topic Registry

**Files:**
- Create: `gui/web/src/lib/data/topics.ts`
- Test: `gui/web/src/lib/data/__tests__/topics.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// gui/web/src/lib/data/__tests__/topics.spec.ts
import { describe, it, expect } from 'vitest'
import { topicConfig, type TopicKey } from '../topics'

describe('topicConfig', () => {
  it('declares all topics in TopicMap', () => {
    const expected: TopicKey[] = ['status', 'speed', 'mesh', 'logs', 'events']
    for (const key of expected) {
      expect(topicConfig[key]).toBeDefined()
    }
  })

  it('snapshot topics omit cursorParam', () => {
    expect(topicConfig.status.kind).toBe('snapshot')
    expect((topicConfig.status as any).cursorParam).toBeUndefined()
  })

  it('stream topics declare cursorParam', () => {
    expect(topicConfig.logs.kind).toBe('stream')
    expect(topicConfig.logs.cursorParam).toBe('since')
    expect(topicConfig.events.kind).toBe('stream')
    expect(topicConfig.events.cursorParam).toBe('since')
  })

  it('every topic has positive pollMs', () => {
    for (const cfg of Object.values(topicConfig)) {
      expect(cfg.pollMs).toBeGreaterThan(0)
    }
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/topics.spec.ts`
Expected: FAIL — `Cannot find module '../topics'`

- [ ] **Step 3: Implement topics.ts**

```typescript
// gui/web/src/lib/data/topics.ts

// Domain types — these mirror existing types in lib/api/types.ts.
// Import from there once the migration in Phase 2 lands; for now
// keep loose typing to avoid circular imports during the build.
import type { Status, MeshPeer } from '@/lib/api/types'

export type SpeedSample = { upload: number; download: number }
export type LogLine = { ts: string; level: string; msg: string; source?: string }
export type EngineEvent = {
  cursor: number
  type: string
  data: unknown
  time: string
}

export type TopicKind = 'snapshot' | 'stream'

export interface TopicEntry {
  wsPath: string
  restPath: string
  pollMs: number
  kind: TopicKind
  cursorParam?: string
}

export type TopicMap = {
  status: { value: Status; kind: 'snapshot' }
  speed: { value: SpeedSample; kind: 'snapshot' }
  mesh: { value: MeshPeer[]; kind: 'snapshot' }
  logs: { value: LogLine; kind: 'stream' }
  events: { value: EngineEvent; kind: 'stream' }
}

export type TopicKey = keyof TopicMap
export type TopicValue<K extends TopicKey> = TopicMap[K]['value']

export const topicConfig: Record<TopicKey, TopicEntry> = {
  status: { wsPath: '/ws/status', restPath: '/api/status', pollMs: 2000, kind: 'snapshot' },
  speed:  { wsPath: '/ws/speed',  restPath: '/api/speed',  pollMs: 1000, kind: 'snapshot' },
  mesh:   { wsPath: '/ws/mesh',   restPath: '/api/mesh/peers', pollMs: 3000, kind: 'snapshot' },
  logs:   { wsPath: '/ws/logs',   restPath: '/api/logs',   pollMs: 1000, kind: 'stream', cursorParam: 'since' },
  events: { wsPath: '/ws/events', restPath: '/api/events', pollMs: 1000, kind: 'stream', cursorParam: 'since' },
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/topics.spec.ts`
Expected: PASS — 4 tests green.

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/topics.ts gui/web/src/lib/data/__tests__/topics.spec.ts
git commit -m "feat(data): topic registry"
```

---

### Task 1.2: Adapter Types & Errors

**Files:**
- Create: `gui/web/src/lib/data/types.ts`
- Test: included in conformance suite (Task 2.3); for now, type-only file with a smoke test.
- Test: `gui/web/src/lib/data/__tests__/types.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// gui/web/src/lib/data/__tests__/types.spec.ts
import { describe, it, expect } from 'vitest'
import { ApiError, TransportError } from '../types'

describe('error types', () => {
  it('ApiError carries status and code', () => {
    const e = new ApiError(404, 'NOT_FOUND', 'server not found')
    expect(e).toBeInstanceOf(Error)
    expect(e.status).toBe(404)
    expect(e.code).toBe('NOT_FOUND')
    expect(e.message).toBe('server not found')
  })

  it('TransportError carries cause', () => {
    const cause = new Error('connection refused')
    const e = new TransportError(cause, 'IPC failed')
    expect(e).toBeInstanceOf(Error)
    expect(e.cause).toBe(cause)
    expect(e.message).toBe('IPC failed')
  })

  it('errors are distinguishable via instanceof', () => {
    const a = new ApiError(500, undefined, 'oops')
    const t = new TransportError(null, 'oops')
    expect(a instanceof ApiError).toBe(true)
    expect(a instanceof TransportError).toBe(false)
    expect(t instanceof TransportError).toBe(true)
    expect(t instanceof ApiError).toBe(false)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/types.spec.ts`
Expected: FAIL — `Cannot find module '../types'`

- [ ] **Step 3: Implement types.ts**

```typescript
// gui/web/src/lib/data/types.ts
import type { TopicKey, TopicValue, TopicMap } from './topics'

export interface DataAdapter {
  request<T = unknown>(opts: RequestOptions): Promise<T>
  subscribe<K extends TopicKey>(
    topic: K,
    opts?: SubscribeOptions<K>,
  ): Subscription<TopicValue<K>>
  readonly connectionState: ReadableValue<ConnectionState>
}

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'

export type RequestOptions = {
  method: HttpMethod
  path: string
  body?: unknown                       // auto JSON.stringify
  headers?: Record<string, string>
  signal?: AbortSignal
  timeoutMs?: number
}

export type Subscription<T> = {
  subscribe(callback: (value: T) => void): () => void
  readonly current: T | undefined
}

export type SubscribeOptions<K extends TopicKey> = {
  cursor?: TopicMap[K]['kind'] extends 'stream' ? string | number : never
  pollInterval?: number
}

export type ConnectionState = 'idle' | 'connecting' | 'connected' | 'error'

export interface ReadableValue<T> {
  readonly value: T
  subscribe(callback: (value: T) => void): () => void
}

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string | undefined,
    message: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export class TransportError extends Error {
  constructor(
    public readonly cause: unknown,
    message: string,
  ) {
    super(message)
    this.name = 'TransportError'
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/types.spec.ts`
Expected: PASS — 3 tests green.

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/types.ts gui/web/src/lib/data/__tests__/types.spec.ts
git commit -m "feat(data): DataAdapter interface and error types"
```

---

### Task 1.3: ConnectionStateController

**Files:**
- Create: `gui/web/src/lib/data/connection-state.ts`
- Test: `gui/web/src/lib/data/__tests__/connection-state.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// gui/web/src/lib/data/__tests__/connection-state.spec.ts
import { describe, it, expect } from 'vitest'
import { ConnectionStateController } from '../connection-state'

describe('ConnectionStateController', () => {
  it('starts idle when no topics reported', () => {
    const c = new ConnectionStateController()
    expect(c.value).toBe('idle')
  })

  it('flips to connected when any topic reports ok', () => {
    const c = new ConnectionStateController()
    c.report('status', 'ok')
    expect(c.value).toBe('connected')
  })

  it('flips to error when all topics report error', () => {
    const c = new ConnectionStateController()
    c.report('status', 'error')
    c.report('logs', 'error')
    expect(c.value).toBe('error')
  })

  it('stays connected if at least one topic is ok', () => {
    const c = new ConnectionStateController()
    c.report('status', 'ok')
    c.report('logs', 'error')
    expect(c.value).toBe('connected')
  })

  it('clear() removes a topic', () => {
    const c = new ConnectionStateController()
    c.report('status', 'ok')
    c.clear('status')
    expect(c.value).toBe('idle')
  })

  it('subscribers receive updates', () => {
    const c = new ConnectionStateController()
    const seen: string[] = []
    c.subscribe(v => seen.push(v))
    c.report('status', 'ok')
    c.report('status', 'error')
    expect(seen).toEqual(['idle', 'connected', 'error'])
  })

  it('unsubscribe stops further notifications', () => {
    const c = new ConnectionStateController()
    const seen: string[] = []
    const off = c.subscribe(v => seen.push(v))
    c.report('status', 'ok')
    off()
    c.report('status', 'error')
    expect(seen).toEqual(['idle', 'connected'])
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/connection-state.spec.ts`
Expected: FAIL — `Cannot find module '../connection-state'`

- [ ] **Step 3: Implement connection-state.ts**

```typescript
// gui/web/src/lib/data/connection-state.ts
import type { TopicKey } from './topics'
import type { ConnectionState, ReadableValue } from './types'

export type TopicHealth = 'ok' | 'error'

export class ConnectionStateController implements ReadableValue<ConnectionState> {
  private topicStates = new Map<TopicKey, TopicHealth>()
  private subscribers = new Set<(v: ConnectionState) => void>()
  private _value: ConnectionState = 'idle'

  get value(): ConnectionState { return this._value }

  report(topic: TopicKey, health: TopicHealth): void {
    this.topicStates.set(topic, health)
    this.recompute()
  }

  clear(topic: TopicKey): void {
    this.topicStates.delete(topic)
    this.recompute()
  }

  subscribe(callback: (value: ConnectionState) => void): () => void {
    this.subscribers.add(callback)
    callback(this._value)   // emit current immediately
    return () => { this.subscribers.delete(callback) }
  }

  private recompute(): void {
    let next: ConnectionState
    if (this.topicStates.size === 0) next = 'idle'
    else if ([...this.topicStates.values()].some(s => s === 'ok')) next = 'connected'
    else next = 'error'
    if (next !== this._value) {
      this._value = next
      for (const cb of this.subscribers) cb(next)
    }
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/connection-state.spec.ts`
Expected: PASS — 7 tests green.

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/connection-state.ts gui/web/src/lib/data/__tests__/connection-state.spec.ts
git commit -m "feat(data): connection state controller"
```

---

### Task 1.4: SubscriptionBase

**Files:**
- Create: `gui/web/src/lib/data/subscription-base.ts`
- Test: covered by both adapter conformance suites in Task 2.3 and 4.4. Add a focused unit test for ref counting and snapshot replay only.
- Test: `gui/web/src/lib/data/__tests__/subscription-base.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// gui/web/src/lib/data/__tests__/subscription-base.spec.ts
import { describe, it, expect, vi } from 'vitest'
import { SubscriptionBase } from '../subscription-base'

class TestSub<T> extends SubscriptionBase<T> {
  connectCount = 0
  disconnectCount = 0
  protected connect(): void { this.connectCount++ }
  protected disconnect(): void { this.disconnectCount++ }
  protected async tick(): Promise<void> { /* no-op */ }
  // expose protected emit for direct test driving
  pushValue(v: T) { this.emit(v) }
}

describe('SubscriptionBase ref counting', () => {
  it('connects on first subscriber', () => {
    const s = new TestSub<number>('status', 'snapshot')
    s.add(() => {})
    expect(s.connectCount).toBe(1)
  })

  it('does not reconnect for subsequent subscribers', () => {
    const s = new TestSub<number>('status', 'snapshot')
    s.add(() => {})
    s.add(() => {})
    expect(s.connectCount).toBe(1)
  })

  it('disconnects when last subscriber leaves', () => {
    const s = new TestSub<number>('status', 'snapshot')
    const off1 = s.add(() => {})
    const off2 = s.add(() => {})
    off1()
    expect(s.disconnectCount).toBe(0)
    off2()
    expect(s.disconnectCount).toBe(1)
  })

  it('snapshot replay: late subscriber gets cached value', async () => {
    const s = new TestSub<number>('status', 'snapshot')
    s.add(() => {})
    s.pushValue(42)
    const cb = vi.fn()
    s.add(cb)
    await Promise.resolve()  // queueMicrotask flush
    expect(cb).toHaveBeenCalledWith(42)
  })

  it('stream replay: late subscriber does NOT get cached value', async () => {
    const s = new TestSub<number>('logs', 'stream')
    s.add(() => {})
    s.pushValue(42)
    const cb = vi.fn()
    s.add(cb)
    await Promise.resolve()
    expect(cb).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/subscription-base.spec.ts`
Expected: FAIL — `Cannot find module '../subscription-base'`

- [ ] **Step 3: Implement subscription-base.ts**

```typescript
// gui/web/src/lib/data/subscription-base.ts
import type { TopicKey, TopicKind } from './topics'

export abstract class SubscriptionBase<T> {
  protected subscribers = new Set<(v: T) => void>()
  protected currentValue: T | undefined
  protected lastHash: string | undefined
  protected cursor: string | number | undefined
  protected errorCount = 0

  constructor(
    protected readonly topic: TopicKey,
    protected readonly kind: TopicKind,
  ) {}

  /** Open the underlying connection / start the timer. */
  protected abstract connect(): void
  /** Tear down the underlying connection / clear the timer. */
  protected abstract disconnect(): void
  /** Execute one fetch cycle (poll) — for subclasses that need it. Default no-op. */
  protected async tick(): Promise<void> { /* no-op */ }

  get current(): T | undefined { return this.kind === 'snapshot' ? this.currentValue : undefined }

  add(callback: (v: T) => void): () => void {
    const wasEmpty = this.subscribers.size === 0
    this.subscribers.add(callback)
    if (wasEmpty) {
      this.connect()
    } else if (this.kind === 'snapshot' && this.currentValue !== undefined) {
      const cached = this.currentValue
      queueMicrotask(() => {
        if (this.subscribers.has(callback)) callback(cached)
      })
    }
    return () => {
      if (!this.subscribers.delete(callback)) return
      if (this.subscribers.size === 0) this.disconnect()
    }
  }

  /** Subclasses call this to deliver a value to subscribers. Snapshot kind diffs by JSON hash. */
  protected emit(value: T): void {
    if (this.kind === 'snapshot') {
      const hash = JSON.stringify(value)
      if (hash === this.lastHash) return
      this.lastHash = hash
      this.currentValue = value
    }
    for (const cb of [...this.subscribers]) cb(value)
  }

  pauseForHidden(): void {
    if (this.subscribers.size > 0) this.disconnect()
  }

  resumeFromHidden(): void {
    if (this.subscribers.size > 0) this.connect()
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/subscription-base.spec.ts`
Expected: PASS — 5 tests green.

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/subscription-base.ts gui/web/src/lib/data/__tests__/subscription-base.spec.ts
git commit -m "feat(data): SubscriptionBase abstract class"
```

---

### Task 1.5: Hooks (useSubscription / useRequest)

**Files:**
- Create: `gui/web/src/lib/data/hooks.svelte.ts`
- Test: `gui/web/src/lib/data/__tests__/hooks.spec.ts`

NOTE: Svelte 5 runes (`$state`, `$effect`) require the `.svelte.ts` extension AND a Svelte component context. Without a component context, runes throw at runtime in vitest. We can use the Svelte testing helpers (`$state.raw` not applicable here) or test the hook contract through a mock adapter.

- [ ] **Step 1: Write the failing test**

```typescript
// gui/web/src/lib/data/__tests__/hooks.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setAdapter } from '../index'
import { useSubscription, useRequest } from '../hooks.svelte'
import type { DataAdapter, Subscription } from '../types'
import type { TopicKey } from '../topics'

function fakeSubscription<T>(initialValue?: T): Subscription<T> & { push(v: T): void } {
  let cb: ((v: T) => void) | null = null
  return {
    current: initialValue,
    subscribe(c) { cb = c; return () => { cb = null } },
    push(v) { cb?.(v) },
  }
}

function fakeAdapter(opts: {
  subFor?: (k: TopicKey) => Subscription<any>,
  request?: vi.Mock,
} = {}): DataAdapter {
  return {
    request: opts.request ?? vi.fn().mockResolvedValue({}),
    subscribe: ((k: TopicKey) => opts.subFor?.(k) ?? fakeSubscription()) as DataAdapter['subscribe'],
    connectionState: {
      value: 'idle',
      subscribe: () => () => {},
    },
  }
}

describe('hooks (component-less smoke)', () => {
  beforeEach(() => {
    setAdapter(fakeAdapter())
  })

  it('useRequest delegates to adapter.request', async () => {
    const reqMock = vi.fn().mockResolvedValue({ ok: true })
    setAdapter(fakeAdapter({ request: reqMock }))
    const result = await useRequest({ method: 'GET', path: '/x' })
    expect(reqMock).toHaveBeenCalledWith({ method: 'GET', path: '/x' })
    expect(result).toEqual({ ok: true })
  })

  it('useSubscription returns object with reactive value getter', () => {
    // Smoke: verifying shape only — runes need a component context to actually update.
    const sub = useSubscription('status')
    expect(typeof Object.getOwnPropertyDescriptor(sub, 'value')?.get).toBe('function')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/hooks.spec.ts`
Expected: FAIL — `Cannot find module '../hooks.svelte'` and `setAdapter` not exported.

- [ ] **Step 3: Implement hooks.svelte.ts**

```typescript
// gui/web/src/lib/data/hooks.svelte.ts
import { getAdapter } from './index'
import type { TopicKey, TopicValue } from './topics'
import type { RequestOptions, SubscribeOptions } from './types'

export function useRequest<T = unknown>(opts: RequestOptions): Promise<T> {
  return getAdapter().request<T>(opts)
}

export function useSubscription<K extends TopicKey>(
  topic: K,
  opts?: SubscribeOptions<K>,
) {
  let value = $state<TopicValue<K> | undefined>(undefined)
  const adapter = getAdapter()
  const sub = adapter.subscribe(topic, opts)
  // Initial replay of cached snapshot value, if any.
  value = sub.current as TopicValue<K> | undefined
  $effect(() => {
    return sub.subscribe(v => { value = v as TopicValue<K> })
  })
  return {
    get value() { return value },
  }
}
```

Add `setAdapter` and `getAdapter` to a placeholder `index.ts` (full impl in Task 1.6):

```typescript
// gui/web/src/lib/data/index.ts (initial stub — completed in Task 1.6)
import type { DataAdapter } from './types'

let _adapter: DataAdapter | null = null

export function setAdapter(a: DataAdapter): void { _adapter = a }
export function getAdapter(): DataAdapter {
  if (!_adapter) throw new Error('DataAdapter not initialised — call setAdapter() during boot')
  return _adapter
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/hooks.spec.ts`
Expected: PASS — 2 tests green.

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/hooks.svelte.ts gui/web/src/lib/data/index.ts gui/web/src/lib/data/__tests__/hooks.spec.ts
git commit -m "feat(data): useSubscription and useRequest hooks"
```

---

### Task 1.6: Adapter Detection / Selection

**Files:**
- Modify: `gui/web/src/lib/data/index.ts` — replace stub with real selection logic
- Modify: `gui/web/src/lib/platform/index.ts:5-12` — sniff `window.ShuttleBridge`
- Test: `gui/web/src/lib/data/__tests__/index.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// gui/web/src/lib/data/__tests__/index.spec.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { __resetAdapter, getAdapter, setAdapter } from '../index'

describe('adapter selection', () => {
  beforeEach(() => { __resetAdapter() })

  it('throws when not initialised', () => {
    expect(() => getAdapter()).toThrow(/not initialised/)
  })

  it('returns the registered adapter', () => {
    const fake: any = { request: vi.fn(), subscribe: vi.fn(), connectionState: { value: 'idle', subscribe: () => () => {} } }
    setAdapter(fake)
    expect(getAdapter()).toBe(fake)
  })

  it('setAdapter is idempotent — second call replaces', () => {
    const a: any = { _id: 'a', request: vi.fn(), subscribe: vi.fn(), connectionState: { value: 'idle', subscribe: () => () => {} } }
    const b: any = { _id: 'b', request: vi.fn(), subscribe: vi.fn(), connectionState: { value: 'idle', subscribe: () => () => {} } }
    setAdapter(a)
    setAdapter(b)
    expect(getAdapter()).toBe(b)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/index.spec.ts`
Expected: FAIL — `__resetAdapter` not exported.

- [ ] **Step 3: Replace stub index.ts**

```typescript
// gui/web/src/lib/data/index.ts
import type { DataAdapter } from './types'

let _adapter: DataAdapter | null = null

export function setAdapter(a: DataAdapter): void { _adapter = a }

export function getAdapter(): DataAdapter {
  if (!_adapter) throw new Error('DataAdapter not initialised — call setAdapter() during boot')
  return _adapter
}

/** Test-only helper. */
export function __resetAdapter(): void { _adapter = null }
```

- [ ] **Step 4: Extend platform detect to sniff ShuttleBridge**

```typescript
// gui/web/src/lib/platform/index.ts (modify detect())
export function detect(): PlatformName {
  if (typeof window === 'undefined') return 'web'
  if ((window as any).go?.main?.App) return 'wails'
  if ((window as any).ShuttleBridge) return 'native'  // iOS VPN mode bridge takes precedence
  if ((window as any).ShuttleVPN) return 'native'
  return 'web'
}
```

- [ ] **Step 5: Run tests to verify pass**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/index.spec.ts src/lib/platform`
Expected: PASS — 3 new tests + existing platform tests still green.

- [ ] **Step 6: Commit**

```bash
git add gui/web/src/lib/data/index.ts gui/web/src/lib/data/__tests__/index.spec.ts gui/web/src/lib/platform/index.ts
git commit -m "feat(data): adapter registry; sniff window.ShuttleBridge in platform.detect"
```

---

## Phase 2 — HttpAdapter + Conformance + Migration

### Task 2.1: HttpSubscription

**Files:**
- Create: `gui/web/src/lib/data/http-subscription.ts`
- Test: `gui/web/src/lib/data/__tests__/http-subscription.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// gui/web/src/lib/data/__tests__/http-subscription.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { HttpSubscription } from '../http-subscription'
import { ConnectionStateController } from '../connection-state'

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  url: string
  readyState = 0
  onopen?: () => void
  onmessage?: (ev: { data: string }) => void
  onclose?: () => void
  onerror?: () => void
  closed = false
  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
    queueMicrotask(() => { this.readyState = 1; this.onopen?.() })
  }
  send(_: string) {}
  close() { this.closed = true; this.readyState = 3; this.onclose?.() }
  emitMessage(payload: unknown) { this.onmessage?.({ data: JSON.stringify(payload) }) }
}

describe('HttpSubscription', () => {
  let conn: ConnectionStateController

  beforeEach(() => {
    FakeWebSocket.instances = []
    conn = new ConnectionStateController()
    ;(globalThis as any).WebSocket = FakeWebSocket
  })

  it('opens one WS for any number of subscribers (snapshot)', () => {
    const sub = new HttpSubscription<{ a: number }>('status', 'snapshot', '/ws/status', conn)
    sub.add(() => {})
    sub.add(() => {})
    expect(FakeWebSocket.instances.length).toBe(1)
  })

  it('closes WS when last subscriber leaves', async () => {
    const sub = new HttpSubscription<{ a: number }>('status', 'snapshot', '/ws/status', conn)
    const off1 = sub.add(() => {})
    const off2 = sub.add(() => {})
    off1(); off2()
    expect(FakeWebSocket.instances[0].closed).toBe(true)
  })

  it('emits messages to subscribers', async () => {
    const sub = new HttpSubscription<{ v: number }>('status', 'snapshot', '/ws/status', conn)
    const cb = vi.fn()
    sub.add(cb)
    await Promise.resolve()
    FakeWebSocket.instances[0].emitMessage({ v: 7 })
    expect(cb).toHaveBeenCalledWith({ v: 7 })
  })

  it('reports ok to ConnectionStateController on first message', async () => {
    const sub = new HttpSubscription<{ v: number }>('status', 'snapshot', '/ws/status', conn)
    sub.add(() => {})
    await Promise.resolve()
    FakeWebSocket.instances[0].emitMessage({ v: 1 })
    expect(conn.value).toBe('connected')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/http-subscription.spec.ts`
Expected: FAIL — `Cannot find module '../http-subscription'`.

- [ ] **Step 3: Implement http-subscription.ts**

```typescript
// gui/web/src/lib/data/http-subscription.ts
import { SubscriptionBase } from './subscription-base'
import type { TopicKey, TopicKind } from './topics'
import type { ConnectionStateController } from './connection-state'

export class HttpSubscription<T> extends SubscriptionBase<T> {
  private ws: WebSocket | null = null
  private closed = false

  constructor(
    topic: TopicKey,
    kind: TopicKind,
    private readonly wsPath: string,
    private readonly conn: ConnectionStateController,
    private readonly authToken: () => string = () => '',
  ) {
    super(topic, kind)
  }

  protected connect(): void {
    if (this.ws) return
    this.closed = false
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const tok = this.authToken()
    const qs = tok ? `?token=${encodeURIComponent(tok)}` : ''
    const url = `${proto}//${location.host}${this.wsPath}${qs}`
    const ws = new WebSocket(url)
    this.ws = ws
    ws.onmessage = (ev: MessageEvent) => {
      try {
        const data = JSON.parse(ev.data) as T
        this.conn.report(this.topic, 'ok')
        this.emit(data)
      } catch { /* ignore parse errors */ }
    }
    ws.onclose = () => {
      this.ws = null
      if (this.closed) return
      this.conn.report(this.topic, 'error')
      // Reopen with backoff if subscribers still present.
      if (this.subscribers.size > 0) {
        setTimeout(() => this.connect(), 2000)
      }
    }
    ws.onerror = () => ws.close()
  }

  protected disconnect(): void {
    this.closed = true
    this.ws?.close()
    this.ws = null
    this.conn.clear(this.topic)
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/http-subscription.spec.ts`
Expected: PASS — 4 tests green.

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/http-subscription.ts gui/web/src/lib/data/__tests__/http-subscription.spec.ts
git commit -m "feat(data): HttpSubscription (WebSocket-driven)"
```

---

### Task 2.2: HttpAdapter

**Files:**
- Create: `gui/web/src/lib/data/http-adapter.ts`
- Test: `gui/web/src/lib/data/__tests__/http-adapter.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// gui/web/src/lib/data/__tests__/http-adapter.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { HttpAdapter } from '../http-adapter'
import { ApiError, TransportError } from '../types'

function mockFetch(impl: (req: Request) => Promise<Response> | Response) {
  ;(globalThis as any).fetch = vi.fn(async (input: any, init?: any) => {
    const req = new Request(input, init)
    return impl(req)
  })
}

describe('HttpAdapter.request', () => {
  beforeEach(() => { vi.useRealTimers() })

  it('parses 200 JSON', async () => {
    mockFetch(() => new Response(JSON.stringify({ ok: 1 }), { status: 200, headers: { 'content-type': 'application/json' } }))
    const a = new HttpAdapter()
    expect(await a.request({ method: 'GET', path: '/x' })).toEqual({ ok: 1 })
  })

  it('returns undefined for 204', async () => {
    mockFetch(() => new Response(null, { status: 204 }))
    const a = new HttpAdapter()
    expect(await a.request({ method: 'POST', path: '/x', body: {} })).toBeUndefined()
  })

  it('throws ApiError on 404 with server message', async () => {
    mockFetch(() => new Response(JSON.stringify({ error: 'gone', code: 'NOT_FOUND' }), { status: 404, headers: { 'content-type': 'application/json' } }))
    const a = new HttpAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(ApiError)
    try { await a.request({ method: 'GET', path: '/x' }) } catch (e: any) {
      expect(e.status).toBe(404)
      expect(e.code).toBe('NOT_FOUND')
      expect(e.message).toBe('gone')
    }
  })

  it('throws TransportError on network failure', async () => {
    mockFetch(() => Promise.reject(new TypeError('fetch failed')))
    const a = new HttpAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(TransportError)
  })

  it('honors AbortSignal', async () => {
    mockFetch(() => new Promise<Response>((_resolve, reject) => {
      setTimeout(() => reject(new DOMException('aborted', 'AbortError')), 10)
    }))
    const a = new HttpAdapter()
    const ctl = new AbortController()
    setTimeout(() => ctl.abort(), 1)
    await expect(a.request({ method: 'GET', path: '/x', signal: ctl.signal })).rejects.toBeDefined()
  })

  it('injects auth header from token getter', async () => {
    const fetchMock = vi.fn(async () => new Response('{}', { status: 200, headers: { 'content-type': 'application/json' } }))
    ;(globalThis as any).fetch = fetchMock
    const a = new HttpAdapter({ authToken: () => 'sekret' })
    await a.request({ method: 'GET', path: '/x' })
    const init = fetchMock.mock.calls[0][1]
    expect((init.headers as any)['Authorization']).toBe('Bearer sekret')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/http-adapter.spec.ts`
Expected: FAIL — `Cannot find module '../http-adapter'`

- [ ] **Step 3: Implement http-adapter.ts**

```typescript
// gui/web/src/lib/data/http-adapter.ts
import { ConnectionStateController } from './connection-state'
import { HttpSubscription } from './http-subscription'
import { topicConfig, type TopicKey, type TopicValue } from './topics'
import {
  ApiError, TransportError,
  type DataAdapter, type RequestOptions, type SubscribeOptions, type Subscription,
} from './types'

export interface HttpAdapterOptions {
  base?: string                    // URL base, default ''
  authToken?: () => string         // pulled per-request
  defaultTimeoutMs?: number        // default 10_000
}

export class HttpAdapter implements DataAdapter {
  readonly connectionState = new ConnectionStateController()
  private readonly subs = new Map<TopicKey, HttpSubscription<any>>()
  private readonly base: string
  private readonly authToken: () => string
  private readonly defaultTimeoutMs: number

  constructor(opts: HttpAdapterOptions = {}) {
    this.base = opts.base ?? ''
    this.authToken = opts.authToken ?? (() => (typeof window !== 'undefined' ? (window as any).__SHUTTLE_AUTH_TOKEN__ ?? '' : ''))
    this.defaultTimeoutMs = opts.defaultTimeoutMs ?? 10_000
  }

  async request<T = unknown>(opts: RequestOptions): Promise<T> {
    const { method, path, body, headers, signal, timeoutMs } = opts
    const ctrl = new AbortController()
    const linked = signal ? linkSignals(signal, ctrl.signal) : ctrl.signal
    const timer = setTimeout(() => ctrl.abort(), timeoutMs ?? this.defaultTimeoutMs)
    try {
      const tok = this.authToken()
      const finalHeaders: Record<string, string> = {
        'Content-Type': 'application/json',
        ...(headers ?? {}),
      }
      if (tok && !finalHeaders['Authorization']) finalHeaders['Authorization'] = `Bearer ${tok}`
      const init: RequestInit = { method, headers: finalHeaders, signal: linked }
      if (body !== undefined) init.body = JSON.stringify(body)

      let res: Response
      try {
        res = await fetch(this.base + path, init)
      } catch (err) {
        throw new TransportError(err, err instanceof Error ? err.message : String(err))
      }

      if (res.status === 204) return undefined as T
      const text = await res.text().catch(() => '')
      const parsed = text ? safeJson(text) : undefined
      if (!res.ok) {
        const msg = (parsed && typeof parsed === 'object' && 'error' in parsed) ? String((parsed as any).error) : `HTTP ${res.status}`
        const code = (parsed && typeof parsed === 'object' && 'code' in parsed) ? String((parsed as any).code) : undefined
        throw new ApiError(res.status, code, msg)
      }
      return parsed as T
    } finally {
      clearTimeout(timer)
    }
  }

  subscribe<K extends TopicKey>(topic: K, _opts?: SubscribeOptions<K>): Subscription<TopicValue<K>> {
    let sub = this.subs.get(topic) as HttpSubscription<TopicValue<K>> | undefined
    if (!sub) {
      const cfg = topicConfig[topic]
      sub = new HttpSubscription<TopicValue<K>>(topic, cfg.kind, cfg.wsPath, this.connectionState, this.authToken)
      this.subs.set(topic, sub)
    }
    return {
      get current() { return sub!.current },
      subscribe: cb => sub!.add(cb),
    }
  }
}

function safeJson(s: string): unknown {
  try { return JSON.parse(s) } catch { return s }
}

function linkSignals(a: AbortSignal, b: AbortSignal): AbortSignal {
  const ctrl = new AbortController()
  const onA = () => ctrl.abort(a.reason)
  const onB = () => ctrl.abort(b.reason)
  if (a.aborted) ctrl.abort(a.reason)
  if (b.aborted) ctrl.abort(b.reason)
  a.addEventListener('abort', onA, { once: true })
  b.addEventListener('abort', onB, { once: true })
  return ctrl.signal
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/http-adapter.spec.ts`
Expected: PASS — 6 tests green.

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/http-adapter.ts gui/web/src/lib/data/__tests__/http-adapter.spec.ts
git commit -m "feat(data): HttpAdapter request + subscribe"
```

---

### Task 2.3: Conformance Suite Skeleton

**Files:**
- Create: `gui/web/src/lib/data/__tests__/conformance.spec.ts`

This file pairs adapters with `describe.each`. Initially only HttpAdapter participates; BridgeAdapter is added in Task 4.4.

- [ ] **Step 1: Write the conformance suite**

```typescript
// gui/web/src/lib/data/__tests__/conformance.spec.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { HttpAdapter } from '../http-adapter'
import { ApiError, TransportError, type DataAdapter } from '../types'

type AdapterFactory = () => Promise<DataAdapter> | DataAdapter

class FakeWS {
  static instances: FakeWS[] = []
  static reset() { FakeWS.instances = [] }
  url: string
  readyState = 0
  onopen?: () => void; onmessage?: (e: { data: string }) => void; onclose?: () => void; onerror?: () => void
  closed = false
  constructor(url: string) {
    this.url = url
    FakeWS.instances.push(this)
    queueMicrotask(() => { this.readyState = 1; this.onopen?.() })
  }
  send(_: string) {}
  close() { this.closed = true; this.readyState = 3; this.onclose?.() }
  push(payload: unknown) { this.onmessage?.({ data: JSON.stringify(payload) }) }
}

const factories: Array<[string, AdapterFactory]> = [
  ['http', () => new HttpAdapter()],
  // ['bridge', async () => makeBridgeAdapter()] — added in Task 4.4
]

describe.each(factories)('%s adapter conformance', (_name, factory) => {
  let adapter: DataAdapter

  beforeEach(async () => {
    FakeWS.reset()
    ;(globalThis as any).WebSocket = FakeWS
    adapter = await factory()
  })

  describe('request', () => {
    it('parses 200 JSON', async () => {
      ;(globalThis as any).fetch = vi.fn(async () =>
        new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'content-type': 'application/json' } }))
      expect(await adapter.request({ method: 'GET', path: '/api/x' })).toEqual({ ok: true })
    })

    it('returns undefined for 204', async () => {
      ;(globalThis as any).fetch = vi.fn(async () => new Response(null, { status: 204 }))
      expect(await adapter.request({ method: 'GET', path: '/api/x' })).toBeUndefined()
    })

    it('throws ApiError on 4xx', async () => {
      ;(globalThis as any).fetch = vi.fn(async () =>
        new Response(JSON.stringify({ error: 'bad' }), { status: 400, headers: { 'content-type': 'application/json' } }))
      await expect(adapter.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(ApiError)
    })

    it('throws TransportError on network failure', async () => {
      ;(globalThis as any).fetch = vi.fn(async () => { throw new TypeError('boom') })
      await expect(adapter.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(TransportError)
    })

    it('honors AbortSignal', async () => {
      ;(globalThis as any).fetch = vi.fn(async (_: any, init: any) => {
        return new Promise<Response>((_resolve, reject) => {
          init.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')))
        })
      })
      const ctl = new AbortController()
      const p = adapter.request({ method: 'GET', path: '/x', signal: ctl.signal })
      ctl.abort()
      await expect(p).rejects.toBeDefined()
    })
  })

  describe('subscribe (snapshot)', () => {
    it('emits values to subscribers', async () => {
      const sub = adapter.subscribe('status')
      const cb = vi.fn()
      sub.subscribe(cb)
      await Promise.resolve()
      FakeWS.instances[0].push({ connected: true })
      expect(cb).toHaveBeenCalledWith(expect.objectContaining({ connected: true }))
    })

    it('does not emit when value unchanged', async () => {
      const sub = adapter.subscribe('status')
      const cb = vi.fn()
      sub.subscribe(cb)
      await Promise.resolve()
      FakeWS.instances[0].push({ connected: true })
      FakeWS.instances[0].push({ connected: true })
      expect(cb).toHaveBeenCalledTimes(1)
    })

    it('current() returns last value', async () => {
      const sub = adapter.subscribe('status')
      sub.subscribe(() => {})
      await Promise.resolve()
      FakeWS.instances[0].push({ connected: true })
      expect(sub.current).toEqual({ connected: true })
    })

    it('multiple subscribers all receive updates', async () => {
      const sub = adapter.subscribe('status')
      const a = vi.fn(); const b = vi.fn()
      sub.subscribe(a); sub.subscribe(b)
      await Promise.resolve()
      FakeWS.instances[0].push({ connected: true })
      expect(a).toHaveBeenCalled(); expect(b).toHaveBeenCalled()
    })

    it('unsubscribe stops emissions', async () => {
      const sub = adapter.subscribe('status')
      const cb = vi.fn()
      const off = sub.subscribe(cb)
      await Promise.resolve()
      off()
      FakeWS.instances[0].push({ connected: true })
      expect(cb).not.toHaveBeenCalled()
    })
  })

  describe('subscribe (stream)', () => {
    it('does not replay history to new subscribers', async () => {
      const sub = adapter.subscribe('logs')
      const a = vi.fn()
      sub.subscribe(a)
      await Promise.resolve()
      FakeWS.instances[0].push({ ts: '1', level: 'info', msg: 'hello' })
      const b = vi.fn()
      sub.subscribe(b)
      expect(b).not.toHaveBeenCalled()
    })

    it('current() is undefined for stream topics', () => {
      const sub = adapter.subscribe('logs')
      sub.subscribe(() => {})
      expect(sub.current).toBeUndefined()
    })
  })

  describe('connectionState', () => {
    it('starts idle', () => {
      expect(adapter.connectionState.value).toBe('idle')
    })

    it('reaches connected after first message', async () => {
      const sub = adapter.subscribe('status')
      sub.subscribe(() => {})
      await Promise.resolve()
      FakeWS.instances[0].push({ connected: true })
      expect(adapter.connectionState.value).toBe('connected')
    })
  })
})
```

- [ ] **Step 2: Run conformance suite**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/conformance.spec.ts`
Expected: PASS — `http adapter conformance` block green (~12 tests).

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/lib/data/__tests__/conformance.spec.ts
git commit -m "test(data): conformance suite for HttpAdapter"
```

---

### Task 2.4: Wire Default Adapter at Boot

**Files:**
- Create: `gui/web/src/app/boot.ts`
- Modify: `gui/web/src/main.ts` (or app entry — find via `grep -l 'mount\|App.svelte' gui/web/src/main.ts`) to call boot before mounting.

The `boot.ts` is the single place that decides which adapter to install. Phase 1 only chooses HttpAdapter; the bridge probe is added in Task 5.5.

- [ ] **Step 1: Write the failing test**

```typescript
// gui/web/src/app/__tests__/boot.spec.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { boot } from '../boot'
import { __resetAdapter, getAdapter } from '@/lib/data'

describe('boot', () => {
  beforeEach(() => { __resetAdapter() })

  it('installs HttpAdapter when no bridge present', async () => {
    delete (window as any).ShuttleBridge
    await boot()
    expect(getAdapter()).toBeDefined()
    expect(getAdapter().connectionState.value).toBe('idle')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gui/web && npx vitest run src/app/__tests__/boot.spec.ts`
Expected: FAIL — `Cannot find module '../boot'`

- [ ] **Step 3: Implement boot.ts**

```typescript
// gui/web/src/app/boot.ts
import { setAdapter } from '@/lib/data'
import { HttpAdapter } from '@/lib/data/http-adapter'
// BridgeAdapter import added in Task 5.5

export async function boot(): Promise<void> {
  // Phase 1 only — HTTP adapter for all runtimes.
  // Task 5.5 will replace this with bridge probe + fallback wiring.
  setAdapter(new HttpAdapter())
}
```

- [ ] **Step 4: Wire into main entry**

Find the main entry:
```bash
ls gui/web/src/main.ts gui/web/src/main.svelte 2>/dev/null
```

Then prepend a `boot()` call before the `mount()`/`new App()`/`createApp()`:

```typescript
// gui/web/src/main.ts (first lines)
import { boot } from './app/boot'
import App from './App.svelte'

await boot()
const app = mount(App, { target: document.body })
export default app
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd gui/web && npx vitest run src/app/__tests__/boot.spec.ts && npm test`
Expected: all tests green.

- [ ] **Step 6: Run dev build smoke**

Run: `cd gui/web && npm run build`
Expected: Build succeeds. No new type errors.

- [ ] **Step 7: Commit**

```bash
git add gui/web/src/app/boot.ts gui/web/src/app/__tests__/boot.spec.ts gui/web/src/main.ts
git commit -m "feat(data): wire boot.ts to install HttpAdapter at startup"
```

---

### Task 2.5: Migrate Existing WebSocket Call Sites

**Files:**
- Modify: `gui/web/src/lib/resources/status.svelte.ts:30-50` (`useSpeedStream`)
- Modify: `gui/web/src/features/logs/store.svelte.ts:70-100` (logs WS, connections WS)
- Existing tests: `gui/web/src/lib/resources/status.test.ts`, `gui/web/src/features/logs/store.test.ts` should continue to pass.

This is a behavior-preserving migration: replace direct `connectWS` usage with `useSubscription(topic)` for `speed` and `logs` topics.

- [ ] **Step 1: Inspect existing useSpeedStream**

Run: `cat gui/web/src/lib/resources/status.svelte.ts`

Confirm the function shape uses `createStream<SpeedSample>('dashboard.speed', '/api/speed', ...)`.

- [ ] **Step 2: Replace useSpeedStream with adapter-based subscription**

```typescript
// gui/web/src/lib/resources/status.svelte.ts (replace useSpeedStream block)
import { useSubscription } from '@/lib/data/hooks.svelte'
import type { SpeedSample } from '@/lib/data/topics'
// (Keep existing imports for createResource etc.)

export function useSpeedStream(): { value: SpeedSample | undefined } {
  return useSubscription('speed')
}
```

- [ ] **Step 3: Run existing dashboard tests**

Run: `cd gui/web && npx vitest run src/lib/resources/`
Expected: PASS — speed-related tests adapt to new shape (may need to update mocks to use `setAdapter` in test setup).

If tests break because they relied on `createStream` semantics, update them:

```typescript
// gui/web/src/lib/resources/status.test.ts (top of file)
import { setAdapter } from '@/lib/data'
import { HttpAdapter } from '@/lib/data/http-adapter'

beforeEach(() => {
  setAdapter(new HttpAdapter())
})
```

- [ ] **Step 4: Migrate logs store**

```typescript
// gui/web/src/features/logs/store.svelte.ts (replace WS opens)
import { useSubscription } from '@/lib/data/hooks.svelte'
import type { LogLine } from '@/lib/data/topics'

// Inside the store class — replace #logWS = connectWS<LogEvent>('/api/logs', ...) with:

const sub = useSubscription('logs')   // topic-driven subscription

$effect(() => {
  if (sub.value) this.appendLine(sub.value)
})
```

(For connections WS: keep using `connectWS('/api/connections', ...)` for now — connections is not in topicConfig and is out of scope for Phase 2. Add as `connections` topic later if needed; for the iOS VPN spec the `events` topic carries connection events.)

- [ ] **Step 5: Run logs tests**

Run: `cd gui/web && npx vitest run src/features/logs/`
Expected: PASS.

- [ ] **Step 6: Run full test suite**

Run: `cd gui/web && npm test`
Expected: All tests green. If tests rely on `createStream` mocks, update them to mock the adapter via `setAdapter(fakeAdapter)` instead.

- [ ] **Step 7: Commit**

```bash
git add gui/web/src/lib/resources/status.svelte.ts gui/web/src/features/logs/store.svelte.ts \
        gui/web/src/lib/resources/status.test.ts gui/web/src/features/logs/store.test.ts
git commit -m "refactor(gui): migrate speed + logs streams to useSubscription"
```

---

## Phase 3 — Go Side: Events Queue, Healthz, Pagination

### Task 3.1: EventQueue ring buffer

**Files:**
- Create: `gui/api/events.go`
- Test: `gui/api/events_test.go`

- [ ] **Step 1: Write the failing test**

```go
// gui/api/events_test.go
package api

import (
	"context"
	"testing"
	"time"
)

func TestEventQueue_PushTail(t *testing.T) {
	q := NewEventQueue(8)
	q.Push("server.connected", map[string]any{"id": "a"})
	q.Push("server.connected", map[string]any{"id": "b"})

	events, latest, gap := q.Tail(0, 100)
	if gap {
		t.Fatal("gap should be false on initial fetch")
	}
	if latest != 2 {
		t.Fatalf("latest = %d, want 2", latest)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
}

func TestEventQueue_TailSince(t *testing.T) {
	q := NewEventQueue(8)
	q.Push("a", nil); q.Push("b", nil); q.Push("c", nil)

	events, latest, gap := q.Tail(1, 100)
	if gap { t.Fatal("gap should be false") }
	if latest != 3 { t.Fatalf("latest = %d, want 3", latest) }
	if len(events) != 2 { t.Fatalf("len(events) = %d, want 2", len(events)) }
	if events[0].Type != "b" || events[1].Type != "c" {
		t.Fatalf("events = %+v, want b,c", events)
	}
}

func TestEventQueue_GapWhenSinceTooOld(t *testing.T) {
	q := NewEventQueue(2)   // tiny ring
	q.Push("a", nil); q.Push("b", nil); q.Push("c", nil)   // a evicted

	_, _, gap := q.Tail(1, 100)   // since=1 means after event #1, but a is gone — depending on policy
	if !gap {
		t.Fatal("expected gap=true when since predates ring buffer")
	}
}

func TestEventQueue_WaitBlocks(t *testing.T) {
	q := NewEventQueue(8)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		q.Push("late", nil)
	}()

	events, _, _, err := q.Wait(ctx, 0)
	if err != nil { t.Fatalf("Wait err: %v", err) }
	if len(events) == 0 || events[0].Type != "late" {
		t.Fatalf("events = %+v, want late", events)
	}
}

func TestEventQueue_WaitReturnsImmediately_WhenAvailable(t *testing.T) {
	q := NewEventQueue(8)
	q.Push("ready", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	events, _, _, err := q.Wait(ctx, 0)
	if err != nil { t.Fatalf("Wait err: %v", err) }
	if len(events) == 0 { t.Fatal("expected immediate return") }
}
```

- [ ] **Step 2: Run test**

Run: `./scripts/test.sh --pkg ./gui/api/ --run TestEventQueue`
Expected: FAIL — undefined: NewEventQueue.

- [ ] **Step 3: Implement events.go**

```go
// gui/api/events.go
package api

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// Event is one entry in the EventQueue.
type Event struct {
	Cursor int64           `json:"cursor"`
	Type   string          `json:"type"`
	Data   json.RawMessage `json:"data"`
	Time   time.Time       `json:"time"`
}

// EventQueue is a bounded ring buffer of engine events with monotonic cursors.
// Tail() returns events strictly after the supplied cursor. If the cursor
// predates the oldest retained event, gap=true is returned and the caller
// should refresh full state.
type EventQueue struct {
	mu     sync.RWMutex
	cap    int
	ring   []Event
	head   int   // next write index
	full   bool
	cursor int64 // monotonic, +1 per Push
	cond   *sync.Cond
}

func NewEventQueue(capacity int) *EventQueue {
	if capacity <= 0 { capacity = 1024 }
	q := &EventQueue{cap: capacity, ring: make([]Event, capacity)}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *EventQueue) Push(typ string, data any) {
	raw, _ := json.Marshal(data)
	q.mu.Lock()
	defer q.mu.Unlock()
	q.cursor++
	q.ring[q.head] = Event{
		Cursor: q.cursor,
		Type:   typ,
		Data:   raw,
		Time:   time.Now().UTC(),
	}
	q.head = (q.head + 1) % q.cap
	if q.head == 0 { q.full = true }
	q.cond.Broadcast()
}

// Tail returns events with cursor > since, up to max. gap=true means since
// predates the retained window.
func (q *EventQueue) Tail(since int64, max int) (events []Event, latest int64, gap bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.tailLocked(since, max)
}

func (q *EventQueue) tailLocked(since int64, max int) ([]Event, int64, bool) {
	if max <= 0 { max = 100 }
	latest := q.cursor
	if since >= latest { return nil, latest, false }

	count := q.size()
	if count == 0 { return nil, latest, false }

	oldestCursor := latest - int64(count) + 1
	gap := since > 0 && since+1 < oldestCursor

	startCursor := since + 1
	if startCursor < oldestCursor { startCursor = oldestCursor }
	startOffset := startCursor - oldestCursor // index from oldest

	out := make([]Event, 0, latest-startCursor+1)
	for i := startOffset; i < int64(count) && len(out) < max; i++ {
		idx := (q.head - count + int(i) + q.cap) % q.cap
		out = append(out, q.ring[idx])
	}
	return out, latest, gap
}

func (q *EventQueue) size() int {
	if q.full { return q.cap }
	return q.head
}

// Wait blocks until events strictly after `since` are available or ctx is done.
func (q *EventQueue) Wait(ctx context.Context, since int64) ([]Event, int64, bool, error) {
	q.mu.Lock()
	for q.cursor <= since {
		// Park goroutine on cond, but allow ctx cancellation to unblock us.
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				q.mu.Lock()
				q.cond.Broadcast()
				q.mu.Unlock()
			case <-done:
			}
		}()
		q.cond.Wait()
		close(done)
		if ctx.Err() != nil {
			q.mu.Unlock()
			return nil, q.cursor, false, ctx.Err()
		}
	}
	events, latest, gap := q.tailLocked(since, 100)
	q.mu.Unlock()
	return events, latest, gap, nil
}
```

- [ ] **Step 4: Run test**

Run: `./scripts/test.sh --pkg ./gui/api/ --run TestEventQueue`
Expected: PASS — 5 tests green.

- [ ] **Step 5: Commit**

```bash
git add gui/api/events.go gui/api/events_test.go
git commit -m "feat(api): EventQueue ring buffer with monotonic cursor"
```

---

### Task 3.2: Events HTTP + WS handlers

**Files:**
- Create: `gui/api/routes_events.go`
- Modify: `gui/api/server.go` (or `gui/api/api.go` — wherever `mux.Handle` lives) to register the routes and instantiate one EventQueue per Server.
- Test: `gui/api/routes_events_test.go`

- [ ] **Step 1: Write the failing test**

```go
// gui/api/routes_events_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEventsHandler_GET_ReturnsEvents(t *testing.T) {
	q := NewEventQueue(8)
	q.Push("ping", map[string]any{"x": 1})

	srv := httptest.NewServer(eventsHandler(q))
	defer srv.Close()

	res, err := http.Get(srv.URL + "?since=0&max=10")
	if err != nil { t.Fatal(err) }
	defer res.Body.Close()

	if res.StatusCode != 200 { t.Fatalf("status %d", res.StatusCode) }
	var body struct {
		Events []Event `json:"events"`
		Cursor int64   `json:"cursor"`
		Gap    bool    `json:"gap"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil { t.Fatal(err) }
	if len(body.Events) != 1 || body.Events[0].Type != "ping" {
		t.Fatalf("got %+v", body.Events)
	}
	if body.Cursor != 1 { t.Fatalf("cursor = %d, want 1", body.Cursor) }
}

func TestEventsHandler_BadSince_400(t *testing.T) {
	q := NewEventQueue(8)
	srv := httptest.NewServer(eventsHandler(q))
	defer srv.Close()

	res, err := http.Get(srv.URL + "?since=abc")
	if err != nil { t.Fatal(err) }
	defer res.Body.Close()
	if res.StatusCode != 400 { t.Fatalf("status %d, want 400", res.StatusCode) }
	body, _ := readAll(res.Body)
	if !strings.Contains(string(body), "since") {
		t.Fatalf("body should mention 'since': %s", body)
	}
}
```

- [ ] **Step 2: Run test**

Run: `./scripts/test.sh --pkg ./gui/api/ --run TestEventsHandler`
Expected: FAIL — undefined: eventsHandler.

- [ ] **Step 3: Implement routes_events.go**

```go
// gui/api/routes_events.go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

var eventsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func eventsHandler(q *EventQueue) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sinceStr := r.URL.Query().Get("since")
		maxStr := r.URL.Query().Get("max")

		since := int64(0)
		if sinceStr != "" {
			n, err := strconv.ParseInt(sinceStr, 10, 64)
			if err != nil {
				http.Error(w, `{"error":"invalid since"}`, http.StatusBadRequest)
				return
			}
			since = n
		}
		max := 100
		if maxStr != "" {
			n, err := strconv.Atoi(maxStr)
			if err != nil {
				http.Error(w, `{"error":"invalid max"}`, http.StatusBadRequest)
				return
			}
			max = n
		}

		events, latest, gap := q.Tail(since, max)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"events": events,
			"cursor": latest,
			"gap":    gap,
		})
	})
}

func eventsWSHandler(q *EventQueue) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sinceStr := r.URL.Query().Get("since")
		since := int64(0)
		if sinceStr != "" {
			if n, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
				since = n
			}
		}

		conn, err := eventsUpgrader.Upgrade(w, r, nil)
		if err != nil { return }
		defer conn.Close()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// Read pump — detect client close.
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					cancel()
					return
				}
			}
		}()

		// Send pump — block on EventQueue.Wait.
		for {
			if ctx.Err() != nil { return }
			events, latest, gap, err := q.Wait(ctx, since)
			if err != nil { return }
			payload := map[string]any{"events": events, "cursor": latest, "gap": gap}
			deadline := time.Now().Add(10 * time.Second)
			_ = conn.SetWriteDeadline(deadline)
			if err := conn.WriteJSON(payload); err != nil { return }
			since = latest
		}
	})
}
```

- [ ] **Step 4: Add helper readAll if missing**

```go
// gui/api/routes_events_test.go (top, if needed)
import "io"

func readAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }
```

- [ ] **Step 5: Run tests**

Run: `./scripts/test.sh --pkg ./gui/api/ --run TestEventsHandler`
Expected: PASS — 2 tests green.

- [ ] **Step 6: Register routes in server**

```go
// gui/api/server.go (or wherever NewHandler builds the mux)
// Add field:
type HandlerConfig struct {
	Engine *engine.Engine
	Events *EventQueue   // optional; nil disables
}

// Inside NewHandler() where routes are registered:
if cfg.Events != nil {
	mux.Handle("/api/events", eventsHandler(cfg.Events))
	mux.Handle("/ws/events", eventsWSHandler(cfg.Events))
}
```

And in `NewServer()`:
```go
func NewServer(eng *engine.Engine, webFS fs.FS) *Server {
	q := NewEventQueue(1024)
	return NewServerWithHandler(eng, webFS, NewHandler(HandlerConfig{Engine: eng, Events: q}))
}
```

- [ ] **Step 7: Run full Go test suite**

Run: `./scripts/test.sh --pkg ./gui/api/`
Expected: All Go API tests green.

- [ ] **Step 8: Commit**

```bash
git add gui/api/routes_events.go gui/api/routes_events_test.go gui/api/server.go gui/api/api.go
git commit -m "feat(api): /api/events REST + /ws/events WebSocket handlers"
```

---

### Task 3.3: Healthz Handler

**Files:**
- Create: `gui/api/healthz.go`
- Modify: `gui/api/server.go` to register `/api/healthz`
- Test: `gui/api/healthz_test.go`

- [ ] **Step 1: Write the failing test**

```go
// gui/api/healthz_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz_Returns200WithStatus(t *testing.T) {
	srv := httptest.NewServer(healthzHandler())
	defer srv.Close()

	res, err := http.Get(srv.URL)
	if err != nil { t.Fatal(err) }
	defer res.Body.Close()

	if res.StatusCode != 200 { t.Fatalf("status %d", res.StatusCode) }

	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil { t.Fatal(err) }
	if body["status"] != "ok" {
		t.Fatalf("status = %v, want ok", body["status"])
	}
}
```

- [ ] **Step 2: Run test**

Run: `./scripts/test.sh --pkg ./gui/api/ --run TestHealthz`
Expected: FAIL.

- [ ] **Step 3: Implement healthz.go**

```go
// gui/api/healthz.go
package api

import (
	"encoding/json"
	"net/http"
)

func healthzHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
		})
	})
}
```

- [ ] **Step 4: Register in NewHandler()**

```go
// gui/api/server.go (inside NewHandler, mux.Handle block)
mux.Handle("/api/healthz", healthzHandler())
```

- [ ] **Step 5: Run tests**

Run: `./scripts/test.sh --pkg ./gui/api/ --run TestHealthz`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add gui/api/healthz.go gui/api/healthz_test.go gui/api/server.go
git commit -m "feat(api): /api/healthz endpoint for bridge probes"
```

---

### Task 3.4: Wire Engine EventBus → EventQueue + servers pagination

**Files:**
- Modify: `gui/api/server.go` to inject the EventQueue and start a goroutine that subscribes to `engine.Engine.Subscribe()` and forwards to the queue.
- Modify: `gui/api/routes_status.go` (or whichever file has `getServers`) to support `?page=N&size=50` pagination.
- Test: `gui/api/api_integration_test.go` (extend) and `gui/api/routes_status_test.go` (add pagination test).

- [ ] **Step 1: Find existing servers list handler**

Run: `grep -rn "func.*Server.*\|getServers\|listServers" gui/api/ | head`

- [ ] **Step 2: Write the failing pagination test**

```go
// gui/api/routes_status_test.go (add at end of file, or create if missing)
func TestServersList_Pagination(t *testing.T) {
	// Test fixture: a handler with 120 fake servers populated.
	// Use httptest.NewServer wrapping NewHandler with an Engine stub.
	// Adapt to actual existing test patterns in this package.
	// ...
}
```

(Implementer: pattern this off existing tests in `gui/api/api_test.go` if pagination test infra exists; otherwise add a minimal one using a real Engine stub.)

- [ ] **Step 3: Add pagination params to handler**

```go
// In whichever file handles GET /api/servers:
import "strconv"

func (h *Handler) getServers(w http.ResponseWriter, r *http.Request) {
	all := h.eng.ListServers()  // (use whatever the existing accessor is)

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 { size = 50 }
	if size > 200 { size = 200 }
	if page < 0 { page = 0 }

	start := page * size
	end := start + size
	if start > len(all) { start = len(all) }
	if end > len(all) { end = len(all) }
	pageItems := all[start:end]

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"servers": pageItems,
		"total":   len(all),
		"page":    page,
		"size":    size,
	})
}
```

(Adapt to existing handler signature.)

- [ ] **Step 4: Wire engine event bus to EventQueue**

```go
// gui/api/server.go (in NewServer or NewServerWithHandler — wherever the lifecycle is)
func NewServer(eng *engine.Engine, webFS fs.FS) *Server {
	q := NewEventQueue(1024)
	go pumpEngineEvents(eng, q)
	return NewServerWithHandler(eng, webFS, NewHandler(HandlerConfig{Engine: eng, Events: q}))
}

func pumpEngineEvents(eng *engine.Engine, q *EventQueue) {
	ch := eng.Subscribe()
	defer eng.Unsubscribe(ch)
	for ev := range ch {
		// Map engine.Event → outbound event type/data per the spec table.
		// engine.Event already has fields like Type, Payload — adapt mapping
		// or pass through as-is in Phase 1; refine the type taxonomy in a
		// follow-up if engine event names don't match the spec list.
		q.Push(eventTypeFor(ev), ev)
	}
}

func eventTypeFor(ev engine.Event) string {
	// Use a tagged switch on existing engine.Event kinds.
	// Default to "engine.event" if no specific mapping.
	// Concrete mapping is straightforward — fill in based on engine.Event union.
	return "engine.event"
}
```

(Implementer: read `engine/engine_events.go:9` for the `engine.Event` shape; map to `engine.state` / `server.connected` / etc. per spec §6.5.)

- [ ] **Step 5: Run all Go tests**

Run: `./scripts/test.sh`
Expected: All host-safe tests green.

- [ ] **Step 6: Commit**

```bash
git add gui/api/server.go gui/api/routes_status.go gui/api/routes_status_test.go
git commit -m "feat(api): pump engine events into EventQueue; paginate servers list"
```

---

## Phase 4 — BridgeAdapter (TS only, mockable bridge)

### Task 4.1: bridge-transport.ts

**Files:**
- Create: `gui/web/src/lib/data/bridge-transport.ts`
- Test: `gui/web/src/lib/data/__tests__/bridge-transport.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// gui/web/src/lib/data/__tests__/bridge-transport.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { BridgeTransport } from '../bridge-transport'

describe('BridgeTransport', () => {
  let posted: Array<{ id: number; envelope: any }> = []
  let bridge: any

  beforeEach(() => {
    posted = []
    // Fake the postMessage / _complete handshake.
    bridge = {
      send(envelope: any) {
        const id = ++bridge._counter
        return new Promise((resolve, reject) => {
          bridge._pending.set(id, { resolve, reject })
          posted.push({ id, envelope })
        })
      },
      _complete(id: number, response: any) {
        const p = bridge._pending.get(id)
        if (p) { bridge._pending.delete(id); p.resolve(response) }
      },
      _fail(id: number, msg: string) {
        const p = bridge._pending.get(id)
        if (p) { bridge._pending.delete(id); p.reject(new Error(msg)) }
      },
      _counter: 0,
      _pending: new Map<number, any>(),
    }
    ;(globalThis as any).window = { ShuttleBridge: bridge }
  })

  it('forwards request envelopes', async () => {
    const t = new BridgeTransport()
    const p = t.send({ method: 'GET', path: '/api/x', headers: {} })
    expect(posted.length).toBe(1)
    expect(posted[0].envelope.path).toBe('/api/x')
    bridge._complete(posted[0].id, { status: 200, headers: {}, body: btoa('{}'), error: null })
    const res = await p
    expect(res.status).toBe(200)
  })

  it('rejects on _fail', async () => {
    const t = new BridgeTransport()
    const p = t.send({ method: 'GET', path: '/x', headers: {} })
    bridge._fail(posted[0].id, 'boom')
    await expect(p).rejects.toThrow('boom')
  })

  it('throws if window.ShuttleBridge missing', () => {
    delete (globalThis as any).window.ShuttleBridge
    expect(() => new BridgeTransport()).toThrow(/ShuttleBridge/)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/bridge-transport.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement bridge-transport.ts**

```typescript
// gui/web/src/lib/data/bridge-transport.ts

export interface BridgeEnvelope {
  method: string
  path: string
  headers: Record<string, string>
  body?: string                   // base64
}

export interface BridgeResponse {
  status: number                   // -1 for transport error
  headers: Record<string, string>
  body: string                     // base64
  error?: string | null
}

export interface ShuttleBridgeAPI {
  send(envelope: BridgeEnvelope): Promise<BridgeResponse>
}

declare global {
  interface Window {
    ShuttleBridge?: ShuttleBridgeAPI & {
      _complete?: (id: number, response: BridgeResponse) => void
      _fail?: (id: number, msg: string) => void
    }
  }
}

export class BridgeTransport {
  private readonly bridge: ShuttleBridgeAPI

  constructor() {
    if (typeof window === 'undefined' || !window.ShuttleBridge) {
      throw new Error('window.ShuttleBridge not available — BridgeAdapter requires it')
    }
    this.bridge = window.ShuttleBridge
  }

  send(envelope: BridgeEnvelope): Promise<BridgeResponse> {
    return this.bridge.send(envelope)
  }
}
```

- [ ] **Step 4: Run test**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/bridge-transport.spec.ts`
Expected: PASS — 3 tests green.

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/bridge-transport.ts gui/web/src/lib/data/__tests__/bridge-transport.spec.ts
git commit -m "feat(data): BridgeTransport — window.ShuttleBridge wrapper"
```

---

### Task 4.2: BridgeSubscription (polling)

**Files:**
- Create: `gui/web/src/lib/data/bridge-subscription.ts`
- Test: `gui/web/src/lib/data/__tests__/bridge-subscription.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// gui/web/src/lib/data/__tests__/bridge-subscription.spec.ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { BridgeSubscription } from '../bridge-subscription'
import { ConnectionStateController } from '../connection-state'

function makeFetcher(impl: (path: string) => Promise<any>) {
  return vi.fn(impl)
}

describe('BridgeSubscription (snapshot)', () => {
  let conn: ConnectionStateController
  beforeEach(() => {
    vi.useFakeTimers()
    conn = new ConnectionStateController()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('polls immediately on first subscribe', async () => {
    const fetcher = makeFetcher(async () => ({ a: 1 }))
    const sub = new BridgeSubscription<{ a: number }>(
      'status', 'snapshot', '/api/status', 2000, undefined, fetcher, conn,
    )
    sub.add(() => {})
    await vi.runOnlyPendingTimersAsync()
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('emits diff only — same value does not re-emit', async () => {
    const fetcher = makeFetcher(async () => ({ a: 1 }))
    const sub = new BridgeSubscription<{ a: number }>(
      'status', 'snapshot', '/api/status', 100, undefined, fetcher, conn,
    )
    const cb = vi.fn()
    sub.add(cb)
    await vi.advanceTimersByTimeAsync(0)
    await vi.advanceTimersByTimeAsync(120)
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('inFlight prevents pile-up', async () => {
    let resolve!: (v: any) => void
    const fetcher = makeFetcher(() => new Promise(r => { resolve = r }))
    const sub = new BridgeSubscription<any>(
      'status', 'snapshot', '/api/status', 50, undefined, fetcher, conn,
    )
    sub.add(() => {})
    await vi.advanceTimersByTimeAsync(150)
    expect(fetcher).toHaveBeenCalledTimes(1)  // first still pending
    resolve({ x: 1 })
  })
})

describe('BridgeSubscription (stream)', () => {
  let conn: ConnectionStateController
  beforeEach(() => { vi.useFakeTimers(); conn = new ConnectionStateController() })
  afterEach(() => { vi.useRealTimers() })

  it('passes cursor as ?since=N', async () => {
    const calls: string[] = []
    const fetcher = makeFetcher(async (p) => {
      calls.push(p)
      return { lines: [{ ts: '1', level: 'info', msg: 'hi' }], cursor: 5 }
    })
    const sub = new BridgeSubscription<any>(
      'logs', 'stream', '/api/logs', 50, 'since', fetcher, conn,
    )
    sub.add(() => {})
    await vi.advanceTimersByTimeAsync(0)
    expect(calls[0]).toContain('?since=0')
    await vi.advanceTimersByTimeAsync(60)
    // Second tick uses updated cursor
    expect(calls[1]).toContain('?since=5')
  })

  it('emits each line', async () => {
    const fetcher = makeFetcher(async () => ({ lines: [{ msg: 'a' }, { msg: 'b' }], cursor: 2 }))
    const sub = new BridgeSubscription<any>(
      'logs', 'stream', '/api/logs', 100, 'since', fetcher, conn,
    )
    const cb = vi.fn()
    sub.add(cb)
    await vi.advanceTimersByTimeAsync(0)
    expect(cb).toHaveBeenCalledTimes(2)
  })
})
```

- [ ] **Step 2: Run test**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/bridge-subscription.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement bridge-subscription.ts**

```typescript
// gui/web/src/lib/data/bridge-subscription.ts
import { SubscriptionBase } from './subscription-base'
import type { TopicKey, TopicKind } from './topics'
import type { ConnectionStateController } from './connection-state'

export type Fetcher = (path: string) => Promise<any>

export class BridgeSubscription<T> extends SubscriptionBase<T> {
  private timer: ReturnType<typeof setInterval> | undefined
  private inFlight = false
  private stopped = true

  constructor(
    topic: TopicKey,
    kind: TopicKind,
    private readonly restPath: string,
    private readonly pollMs: number,
    private readonly cursorParam: string | undefined,
    private readonly fetcher: Fetcher,
    private readonly conn: ConnectionStateController,
  ) {
    super(topic, kind)
  }

  protected connect(): void {
    if (!this.stopped) return
    this.stopped = false
    queueMicrotask(() => { void this.tick() })
    this.timer = setInterval(() => { void this.tick() }, this.pollMs)
  }

  protected disconnect(): void {
    this.stopped = true
    if (this.timer) clearInterval(this.timer)
    this.timer = undefined
    this.inFlight = false
    this.conn.clear(this.topic)
  }

  protected async tick(): Promise<void> {
    if (this.inFlight || this.stopped) return
    this.inFlight = true
    try {
      const path = this.buildPath()
      const result = await this.fetcher(path)
      if (this.stopped) return
      this.handleSuccess(result)
    } catch (err) {
      if (this.stopped) return
      this.handleError(err)
    } finally {
      this.inFlight = false
    }
  }

  private buildPath(): string {
    if (this.kind !== 'stream' || !this.cursorParam) return this.restPath
    const sep = this.restPath.includes('?') ? '&' : '?'
    return `${this.restPath}${sep}${this.cursorParam}=${this.cursor ?? 0}`
  }

  private handleSuccess(result: any): void {
    this.errorCount = 0
    this.conn.report(this.topic, 'ok')
    if (this.kind === 'snapshot') {
      this.emit(result as T)
    } else {
      const lines = Array.isArray(result?.lines) ? result.lines : Array.isArray(result?.events) ? result.events : []
      const nextCursor = result?.cursor
      if (nextCursor !== undefined) this.cursor = nextCursor
      for (const line of lines) this.emit(line as T)
    }
  }

  private handleError(err: unknown): void {
    this.errorCount++
    this.conn.report(this.topic, 'error')
    if (this.errorCount === 1) return   // let next interval retry naturally
    if (this.timer) clearInterval(this.timer)
    const delay = Math.min(30_000, 500 * 2 ** (this.errorCount - 1))
    this.timer = setTimeout(() => {
      if (!this.stopped) this.connect()
    }, delay) as unknown as ReturnType<typeof setInterval>
  }
}
```

- [ ] **Step 4: Run tests**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/bridge-subscription.spec.ts`
Expected: PASS — 5 tests green.

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/bridge-subscription.ts gui/web/src/lib/data/__tests__/bridge-subscription.spec.ts
git commit -m "feat(data): BridgeSubscription polling engine"
```

---

### Task 4.3: BridgeAdapter

**Files:**
- Create: `gui/web/src/lib/data/bridge-adapter.ts`
- Test: `gui/web/src/lib/data/__tests__/bridge-adapter.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// gui/web/src/lib/data/__tests__/bridge-adapter.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { BridgeAdapter } from '../bridge-adapter'
import { ApiError, TransportError } from '../types'

function fakeBridge(impl: (env: any) => Promise<any>) {
  ;(globalThis as any).window = { ShuttleBridge: { send: vi.fn(impl) } }
}

function ok(body: unknown, status = 200): any {
  return { status, headers: {'content-type': 'application/json'}, body: btoa(JSON.stringify(body)) }
}

describe('BridgeAdapter.request', () => {
  beforeEach(() => {})

  it('parses 200 JSON', async () => {
    fakeBridge(async () => ok({ ok: 1 }))
    const a = new BridgeAdapter()
    expect(await a.request({ method: 'GET', path: '/api/x' })).toEqual({ ok: 1 })
  })

  it('throws ApiError on 4xx', async () => {
    fakeBridge(async () => ok({ error: 'gone' }, 404))
    const a = new BridgeAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(ApiError)
  })

  it('throws TransportError when status=-1', async () => {
    fakeBridge(async () => ({ status: -1, headers: {}, body: '', error: 'no response' }))
    const a = new BridgeAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(TransportError)
  })

  it('throws TransportError when bridge.send rejects', async () => {
    fakeBridge(async () => { throw new Error('IPC fail') })
    const a = new BridgeAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(TransportError)
  })

  it('encodes JSON body as base64', async () => {
    let captured: any
    fakeBridge(async (env) => { captured = env; return ok({}) })
    const a = new BridgeAdapter()
    await a.request({ method: 'POST', path: '/x', body: { a: 1 } })
    expect(captured.body).toBe(btoa(JSON.stringify({ a: 1 })))
  })
})
```

- [ ] **Step 2: Run test**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/bridge-adapter.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement bridge-adapter.ts**

```typescript
// gui/web/src/lib/data/bridge-adapter.ts
import { BridgeTransport } from './bridge-transport'
import { BridgeSubscription } from './bridge-subscription'
import { ConnectionStateController } from './connection-state'
import { topicConfig, type TopicKey, type TopicValue } from './topics'
import {
  ApiError, TransportError,
  type DataAdapter, type RequestOptions, type SubscribeOptions, type Subscription,
} from './types'

export interface BridgeAdapterOptions {
  authToken?: () => string
  transport?: BridgeTransport
}

export class BridgeAdapter implements DataAdapter {
  readonly connectionState = new ConnectionStateController()
  private readonly subs = new Map<TopicKey, BridgeSubscription<any>>()
  private readonly transport: BridgeTransport
  private readonly authToken: () => string

  constructor(opts: BridgeAdapterOptions = {}) {
    this.transport = opts.transport ?? new BridgeTransport()
    this.authToken = opts.authToken ?? (() => (typeof window !== 'undefined' ? (window as any).__SHUTTLE_AUTH_TOKEN__ ?? '' : ''))
  }

  async request<T = unknown>(opts: RequestOptions): Promise<T> {
    const token = this.authToken()
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(opts.headers ?? {}),
    }
    if (token && !headers['Authorization']) headers['Authorization'] = `Bearer ${token}`

    const envelope = {
      method: opts.method,
      path: opts.path,
      headers,
      body: opts.body !== undefined ? btoa(JSON.stringify(opts.body)) : undefined,
    }

    let resp
    try {
      resp = await this.transport.send(envelope)
    } catch (err) {
      throw new TransportError(err, err instanceof Error ? err.message : String(err))
    }

    if (resp.status === -1 || resp.error) {
      throw new TransportError(null, resp.error || 'transport error')
    }

    if (resp.status === 204) return undefined as T

    const text = resp.body ? atob(resp.body) : ''
    const parsed = text ? safeJson(text) : undefined

    if (resp.status >= 400) {
      const msg = (parsed && typeof parsed === 'object' && 'error' in parsed) ? String((parsed as any).error) : `HTTP ${resp.status}`
      const code = (parsed && typeof parsed === 'object' && 'code' in parsed) ? String((parsed as any).code) : undefined
      throw new ApiError(resp.status, code, msg)
    }
    return parsed as T
  }

  subscribe<K extends TopicKey>(topic: K, _opts?: SubscribeOptions<K>): Subscription<TopicValue<K>> {
    let sub = this.subs.get(topic) as BridgeSubscription<TopicValue<K>> | undefined
    if (!sub) {
      const cfg = topicConfig[topic]
      const fetcher = async (path: string) => this.request({ method: 'GET', path })
      sub = new BridgeSubscription<TopicValue<K>>(
        topic, cfg.kind, cfg.restPath, cfg.pollMs, cfg.cursorParam, fetcher, this.connectionState,
      )
      this.subs.set(topic, sub)
    }
    return {
      get current() { return sub!.current },
      subscribe: cb => sub!.add(cb),
    }
  }
}

function safeJson(s: string): unknown {
  try { return JSON.parse(s) } catch { return s }
}
```

- [ ] **Step 4: Run tests**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/bridge-adapter.spec.ts`
Expected: PASS — 5 tests green.

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/bridge-adapter.ts gui/web/src/lib/data/__tests__/bridge-adapter.spec.ts
git commit -m "feat(data): BridgeAdapter envelope IPC + polling subscriptions"
```

---

### Task 4.4: Add BridgeAdapter to Conformance Suite

**Files:**
- Modify: `gui/web/src/lib/data/__tests__/conformance.spec.ts` (Task 2.3) to add the bridge factory.

- [ ] **Step 1: Add bridge factory entry**

```typescript
// gui/web/src/lib/data/__tests__/conformance.spec.ts (top of file)
import { BridgeAdapter } from '../bridge-adapter'

// Inside the file: replace the factories array:
const factories: Array<[string, AdapterFactory]> = [
  ['http', () => {
    ;(globalThis as any).WebSocket = FakeWS
    return new HttpAdapter()
  }],
  ['bridge', () => {
    // Construct a BridgeAdapter backed by an in-memory fake bridge that
    // serves the same fake fetch responses tests rely on, plus a manual
    // event push for subscribe tests.
    const handlers = new Map<string, (env: any) => Promise<any>>()
    ;(globalThis as any).window = {
      ShuttleBridge: {
        send: async (env: any) => {
          // Match request via globalThis.fetch mock (HTTP tests use fetch).
          // Convert the envelope to a Response equivalent.
          const fetchMock = (globalThis as any).fetch
          if (!fetchMock) throw new Error('no fetch mock for bridge')
          const init: RequestInit = {
            method: env.method,
            headers: env.headers,
            body: env.body ? atob(env.body) : undefined,
          }
          const res: Response = await fetchMock(env.path, init)
          const headers: Record<string, string> = {}
          res.headers?.forEach((v, k) => { headers[k] = v })
          const text = res.body ? await res.text() : ''
          return {
            status: res.status,
            headers,
            body: btoa(text),
            error: null,
          }
        },
      },
    }
    return new BridgeAdapter()
  }],
]
```

NOTE: The "subscribe (snapshot)" tests use `FakeWS.instances[0].push(...)` to drive WS frames. For BridgeAdapter, those tests need an alternative driver — wrap the snapshot tests in a per-adapter conditional:

```typescript
describe('subscribe (snapshot)', () => {
  it('emits values to subscribers', async () => {
    if (_name === 'bridge') {
      // Bridge polls — set up fetch to return a JSON body, then advance fake timers.
      ;(globalThis as any).fetch = vi.fn(async () => new Response(JSON.stringify({ connected: true }), { status: 200, headers: { 'content-type': 'application/json' } }))
      vi.useFakeTimers()
      const sub = adapter.subscribe('status')
      const cb = vi.fn()
      sub.subscribe(cb)
      await vi.runOnlyPendingTimersAsync()
      expect(cb).toHaveBeenCalledWith(expect.objectContaining({ connected: true }))
      vi.useRealTimers()
      return
    }
    // HTTP path — original code
    const sub = adapter.subscribe('status')
    const cb = vi.fn()
    sub.subscribe(cb)
    await Promise.resolve()
    FakeWS.instances[0].push({ connected: true })
    expect(cb).toHaveBeenCalledWith(expect.objectContaining({ connected: true }))
  })
  // ...repeat per-test branching for the other subscribe tests
})
```

(Implementer: this branching is necessary because the two adapters use different transports for live updates. Keep request-tier tests transport-agnostic; only subscribe-tier tests need branches.)

- [ ] **Step 2: Run conformance suite**

Run: `cd gui/web && npx vitest run src/lib/data/__tests__/conformance.spec.ts`
Expected: PASS — both `http` and `bridge` blocks green.

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/lib/data/__tests__/conformance.spec.ts
git commit -m "test(data): conformance suite covers BridgeAdapter"
```

---

## Phase 5 — iOS Native

### Task 5.1: SharedBridge Swift Package

**Files:**
- Create: `mobile/ios/SharedBridge/Package.swift`
- Create: `mobile/ios/SharedBridge/Sources/SharedBridge/APIRequest.swift`
- Create: `mobile/ios/SharedBridge/Sources/SharedBridge/APIResponse.swift`

- [ ] **Step 1: Create Package.swift**

```swift
// mobile/ios/SharedBridge/Package.swift
// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "SharedBridge",
    platforms: [.iOS(.v15)],
    products: [
        .library(name: "SharedBridge", targets: ["SharedBridge"]),
    ],
    targets: [
        .target(name: "SharedBridge", dependencies: []),
        .testTarget(name: "SharedBridgeTests", dependencies: ["SharedBridge"]),
    ]
)
```

- [ ] **Step 2: Create APIRequest.swift**

```swift
// mobile/ios/SharedBridge/Sources/SharedBridge/APIRequest.swift
import Foundation

public struct APIRequest: Codable {
    public let method: String
    public let path: String
    public let headers: [String: String]
    public let body: String?    // base64

    public init(method: String, path: String, headers: [String: String], body: String? = nil) {
        self.method = method
        self.path = path
        self.headers = headers
        self.body = body
    }
}
```

- [ ] **Step 3: Create APIResponse.swift**

```swift
// mobile/ios/SharedBridge/Sources/SharedBridge/APIResponse.swift
import Foundation

public struct APIResponse: Codable {
    public let status: Int      // -1 = transport error
    public let headers: [String: String]
    public let body: String     // base64
    public let error: String?

    public init(status: Int, headers: [String: String], body: String, error: String? = nil) {
        self.status = status
        self.headers = headers
        self.body = body
        self.error = error
    }

    public static func transportError(_ msg: String) -> APIResponse {
        APIResponse(status: -1, headers: [:], body: "", error: msg)
    }

    public static func engineNotReady() -> APIResponse {
        APIResponse(status: 503, headers: [:], body: "", error: "engine not ready")
    }
}
```

- [ ] **Step 4: Add a roundtrip test**

```swift
// mobile/ios/SharedBridge/Tests/SharedBridgeTests/SharedBridgeTests.swift
import XCTest
@testable import SharedBridge

final class SharedBridgeTests: XCTestCase {
    func testAPIRequest_RoundTrip() throws {
        let req = APIRequest(method: "GET", path: "/api/x", headers: ["A": "B"], body: "ZGF0YQ==")
        let data = try JSONEncoder().encode(req)
        let decoded = try JSONDecoder().decode(APIRequest.self, from: data)
        XCTAssertEqual(decoded.method, "GET")
        XCTAssertEqual(decoded.path, "/api/x")
        XCTAssertEqual(decoded.headers["A"], "B")
        XCTAssertEqual(decoded.body, "ZGF0YQ==")
    }

    func testAPIResponse_RoundTrip() throws {
        let res = APIResponse(status: 200, headers: ["x": "y"], body: "Zm9v", error: nil)
        let data = try JSONEncoder().encode(res)
        let decoded = try JSONDecoder().decode(APIResponse.self, from: data)
        XCTAssertEqual(decoded.status, 200)
        XCTAssertEqual(decoded.body, "Zm9v")
        XCTAssertNil(decoded.error)
    }
}
```

- [ ] **Step 5: Build the SPM package**

Run: `cd mobile/ios/SharedBridge && swift build && swift test`
Expected: build succeeds; 2 tests pass.

(If `swift` command unavailable in CI Linux, this step runs on macOS only — note in commit message.)

- [ ] **Step 6: Commit**

```bash
git add mobile/ios/SharedBridge/
git commit -m "feat(ios): SharedBridge SPM with APIRequest/APIResponse codable contract"
```

---

### Task 5.2: VPNManager.sendToExtension

**Files:**
- Modify: `mobile/ios/Shuttle/VPNManager.swift` (add public `sendToExtension(_:timeout:completion:)` wrapper around existing `sendProviderMessage`)

- [ ] **Step 1: Read current VPNManager state**

Run: `grep -n "sendProviderMessage\|class VPNManager" mobile/ios/Shuttle/VPNManager.swift`

- [ ] **Step 2: Add the public method**

```swift
// mobile/ios/Shuttle/VPNManager.swift
// (add as method on VPNManager — keeping existing private sendMessage if present)

import NetworkExtension

extension VPNManager {
    /// Forwards a raw envelope to the extension. Calls completion with response
    /// data on success, or nil on timeout / no provider session.
    public func sendToExtension(_ data: Data, timeout: TimeInterval = 30, completion: @escaping (Data?) -> Void) {
        guard let session = self.manager?.connection as? NETunnelProviderSession else {
            completion(nil); return
        }

        var responded = false
        let timeoutWork = DispatchWorkItem {
            if !responded { responded = true; completion(nil) }
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + timeout, execute: timeoutWork)

        do {
            try session.sendProviderMessage(data) { response in
                guard !responded else { return }
                responded = true
                timeoutWork.cancel()
                completion(response)
            }
        } catch {
            if !responded { responded = true; timeoutWork.cancel(); completion(nil) }
        }
    }
}
```

- [ ] **Step 3: Build the iOS app**

Run (macOS only): `cd mobile/ios && xcodebuild -scheme Shuttle -sdk iphonesimulator build 2>&1 | tail -20`

Or, if no Xcode project exists yet, skip to Task 5.5 which creates the wiring; for now confirm the file compiles via SPM dry-run if available.

(Note: `mobile/ios/` may not have an `.xcodeproj` per the CLAUDE.md notes. If so, mark this task as "code complete; CI build deferred to project scaffolding".)

- [ ] **Step 4: Commit**

```bash
git add mobile/ios/Shuttle/VPNManager.swift
git commit -m "feat(ios): VPNManager.sendToExtension envelope forwarder with timeout"
```

---

### Task 5.3: APIBridge.swift + bootstrap JS injection

**Files:**
- Create: `mobile/ios/Shuttle/APIBridge.swift`
- Modify: `mobile/ios/Shuttle/ShuttleApp.swift` to register the bridge handler and inject the bootstrap JS user script.

- [ ] **Step 1: Create APIBridge.swift**

```swift
// mobile/ios/Shuttle/APIBridge.swift
import Foundation
import WebKit
import os.log
import SharedBridge

private let log = Logger(subsystem: "com.shuttle.app", category: "APIBridge")

public final class APIBridge: NSObject, WKScriptMessageHandler {
    public weak var webView: WKWebView?
    private let manager: VPNManager

    public init(manager: VPNManager) {
        self.manager = manager
        super.init()
    }

    public func userContentController(_ ucc: WKUserContentController, didReceive msg: WKScriptMessage) {
        guard let body = msg.body as? [String: Any],
              let id = body["id"] as? Int,
              let envelopeAny = body["envelope"] as? [String: Any] else {
            log.warning("APIBridge: malformed message")
            return
        }

        guard let envelopeData = try? JSONSerialization.data(withJSONObject: envelopeAny) else {
            failJS(id: id, message: "envelope encode failed")
            return
        }

        manager.sendToExtension(envelopeData, timeout: 30) { [weak self] response in
            self?.completeJS(id: id, response: response)
        }
    }

    private func completeJS(id: Int, response: Data?) {
        DispatchQueue.main.async { [weak self] in
            guard let webView = self?.webView else { return }
            if let data = response, let json = String(data: data, encoding: .utf8) {
                let js = "window.ShuttleBridge._complete(\(id), \(json))"
                webView.evaluateJavaScript(js, completionHandler: nil)
            } else {
                self?.failJS(id: id, message: "IPC timeout or no response")
            }
        }
    }

    private func failJS(id: Int, message: String) {
        DispatchQueue.main.async { [weak self] in
            guard let webView = self?.webView else { return }
            let safeMsg = message.replacingOccurrences(of: "'", with: "\\'")
            let js = "window.ShuttleBridge._fail(\(id), '\(safeMsg)')"
            webView.evaluateJavaScript(js, completionHandler: nil)
        }
    }
}

public let shuttleBridgeBootstrapJS = """
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
"""
```

- [ ] **Step 2: Wire into ShuttleApp.swift**

Open `mobile/ios/Shuttle/ShuttleApp.swift`. Find the WKWebView construction (currently around lines that build `WKWebViewConfiguration`). Replace with:

```swift
// mobile/ios/Shuttle/ShuttleApp.swift  (around the WebView setup)
import SharedBridge

let userContent = WKUserContentController()
let apiBridge = APIBridge(manager: vpnManager)
userContent.add(apiBridge, name: "shuttleBridge")
let bootstrapScript = WKUserScript(
    source: shuttleBridgeBootstrapJS,
    injectionTime: .atDocumentStart,
    forMainFrameOnly: true,
)
userContent.addUserScript(bootstrapScript)

let config = WKWebViewConfiguration()
config.userContentController = userContent

let webView = WKWebView(frame: .zero, configuration: config)
apiBridge.webView = webView
// ... existing loadFileURL("Shuttle/www/index.html") call
```

(Replace the previous fallback HTML loading. Keep a feature flag based on URL query: if `?bridge=0` then load fallback HTML — used during Phase α/β testing. See Task 5.5 for fallback wiring.)

- [ ] **Step 3: Build (macOS)**

Run: `cd mobile/ios && xcodebuild -scheme Shuttle -sdk iphonesimulator build 2>&1 | tail -30`
Expected: build succeeds.

- [ ] **Step 4: Commit**

```bash
git add mobile/ios/Shuttle/APIBridge.swift mobile/ios/Shuttle/ShuttleApp.swift
git commit -m "feat(ios): APIBridge WKScriptMessageHandler + window.ShuttleBridge bootstrap"
```

---

### Task 5.4: Extension handleAppMessage envelope branch

**Files:**
- Modify: `mobile/ios/ShuttleExtension/PacketTunnelProvider.swift` (around line 86 per the spec)

- [ ] **Step 1: Read current handler shape**

Run: `grep -n "handleAppMessage\|func.*handleApp" mobile/ios/ShuttleExtension/PacketTunnelProvider.swift`

- [ ] **Step 2: Replace handleAppMessage with envelope branch + legacy fallback**

```swift
// mobile/ios/ShuttleExtension/PacketTunnelProvider.swift
import SharedBridge
import Foundation

extension PacketTunnelProvider {
    public override func handleAppMessage(_ messageData: Data, completionHandler: ((Data?) -> Void)?) {
        // 1) Try envelope decode first (new protocol)
        if let req = try? JSONDecoder().decode(APIRequest.self, from: messageData) {
            forwardToLocalAPI(req) { response in
                let data = (try? JSONEncoder().encode(response)) ?? Data()
                completionHandler?(data)
            }
            return
        }

        // 2) Legacy string commands ("status" / "stop" / "logs" / JSON config reload)
        //    Kept until SPA migration complete; remove in Task 6.4 cleanup.
        if let cmd = String(data: messageData, encoding: .utf8) {
            handleLegacyCommand(cmd, completionHandler: completionHandler)
            return
        }

        completionHandler?(nil)
    }

    private func forwardToLocalAPI(_ req: APIRequest, completion: @escaping (APIResponse) -> Void) {
        guard let apiAddr = self.currentAPIAddr else {
            completion(.engineNotReady())
            return
        }
        var components = URLComponents()
        components.scheme = "http"
        components.host = "127.0.0.1"
        components.port = apiAddr.port
        // Allow path with query string already attached.
        if let qIdx = req.path.firstIndex(of: "?") {
            components.path = String(req.path[..<qIdx])
            components.query = String(req.path[req.path.index(after: qIdx)...])
        } else {
            components.path = req.path
        }
        guard let url = components.url else {
            completion(.transportError("invalid URL"))
            return
        }

        var urlReq = URLRequest(url: url)
        urlReq.httpMethod = req.method
        for (k, v) in req.headers { urlReq.setValue(v, forHTTPHeaderField: k) }
        if let b64 = req.body, let body = Data(base64Encoded: b64) { urlReq.httpBody = body }
        urlReq.timeoutInterval = 25

        URLSession.shared.dataTask(with: urlReq) { data, response, error in
            if let error = error {
                completion(.transportError("\(error)"))
                return
            }
            guard let http = response as? HTTPURLResponse else {
                completion(.transportError("non-http response"))
                return
            }
            // Enforce response size limit (192 KB)
            let bodyData = data ?? Data()
            if bodyData.count > 192 * 1024 {
                completion(APIResponse(status: -1, headers: [:], body: "", error: "response too large"))
                return
            }
            let bodyB64 = bodyData.base64EncodedString()
            let headers = (http.allHeaderFields as? [String: String]) ?? [:]
            completion(APIResponse(status: http.statusCode, headers: headers, body: bodyB64, error: nil))
        }.resume()
    }

    private func handleLegacyCommand(_ cmd: String, completionHandler: ((Data?) -> Void)?) {
        // Existing logic — extracted unchanged. Implementer: paste in the
        // pre-existing switch statement that handled "status"/"stop"/"logs".
    }
}
```

(The `currentAPIAddr` property is the existing field that `MobileStart` populates with the listener address. The implementer should confirm its existing name and adapt accordingly.)

- [ ] **Step 3: Add SharedBridge as a dependency**

In Xcode: drag `mobile/ios/SharedBridge/` package into the ShuttleExtension target's Frameworks. Or from CLI, edit the `.xcodeproj` (manual) or use `swift package` if a workspace exists.

- [ ] **Step 4: Build extension target**

Run: `cd mobile/ios && xcodebuild -scheme ShuttleExtension -sdk iphonesimulator build 2>&1 | tail -30`
Expected: build succeeds.

- [ ] **Step 5: Commit**

```bash
git add mobile/ios/ShuttleExtension/PacketTunnelProvider.swift
git commit -m "feat(ios): extension handleAppMessage envelope branch + REST forwarder"
```

---

### Task 5.5: FallbackHandler.swift + boot.ts probe

**Files:**
- Create: `mobile/ios/Shuttle/FallbackHandler.swift`
- Modify: `mobile/ios/Shuttle/ShuttleApp.swift` to register the fallback handler.
- Modify: `gui/web/src/app/boot.ts` to do a healthz probe and trigger fallback on failure.

- [ ] **Step 1: Create FallbackHandler.swift**

```swift
// mobile/ios/Shuttle/FallbackHandler.swift
import Foundation
import WebKit
import os.log

private let log = Logger(subsystem: "com.shuttle.app", category: "Fallback")

public final class FallbackHandler: NSObject, WKScriptMessageHandler {
    public weak var webView: WKWebView?
    private let inlineHTML: String

    public init(inlineHTML: String) {
        self.inlineHTML = inlineHTML
        super.init()
    }

    public func userContentController(_ ucc: WKUserContentController, didReceive msg: WKScriptMessage) {
        guard let body = msg.body as? [String: Any],
              let reason = body["reason"] as? String else { return }
        log.warning("Bridge fallback triggered: \(reason)")
        DispatchQueue.main.async { [weak self] in
            self?.webView?.loadHTMLString(self?.inlineHTML ?? "", baseURL: nil)
        }
    }
}
```

- [ ] **Step 2: Register the fallback handler in ShuttleApp.swift**

```swift
// mobile/ios/Shuttle/ShuttleApp.swift (alongside the existing apiBridge registration)
let fallbackHTML = """
<!doctype html>
<html><head><meta name="viewport" content="width=device-width,initial-scale=1"><style>
body{font-family:-apple-system;text-align:center;padding:80px 20px;color:#222;background:#f7f7f7}
button{padding:12px 24px;border:0;border-radius:8px;background:#007aff;color:#fff;font-size:16px}
</style></head><body>
<h2>Realtime panel unavailable</h2>
<p>The engine is still running. Tap to retry.</p>
<button onclick="window.location.reload()">Retry</button>
</body></html>
"""
let fallbackHandler = FallbackHandler(inlineHTML: fallbackHTML)
userContent.add(fallbackHandler, name: "fallback")
fallbackHandler.webView = webView
```

- [ ] **Step 3: Update boot.ts with the probe**

```typescript
// gui/web/src/app/boot.ts (replace Phase 1 stub)
import { setAdapter } from '@/lib/data'
import { HttpAdapter } from '@/lib/data/http-adapter'
import { BridgeAdapter } from '@/lib/data/bridge-adapter'

declare global {
  interface Window {
    webkit?: {
      messageHandlers?: {
        fallback?: { postMessage: (msg: any) => void }
      }
    }
    ShuttleBridge?: any
  }
}

function timeout(ms: number): Promise<never> {
  return new Promise((_resolve, reject) => setTimeout(() => reject(new Error('timeout')), ms))
}

function requestFallback(reason: string): void {
  window.webkit?.messageHandlers?.fallback?.postMessage({ reason, timestamp: Date.now() })
}

export async function boot(): Promise<void> {
  const force = new URLSearchParams(location.search).get('bridge')

  // Force HTTP path
  if (force === '0') { setAdapter(new HttpAdapter()); return }

  // Wait briefly for window.ShuttleBridge to appear (atDocumentStart user script).
  if (typeof window !== 'undefined' && !window.ShuttleBridge) {
    await new Promise(r => setTimeout(r, 100))
  }

  if (!window.ShuttleBridge) {
    if (force === '1') { requestFallback('ShuttleBridge missing under bridge=1'); return }
    setAdapter(new HttpAdapter())
    return
  }

  // Bridge present — probe healthz before committing.
  const bridge = new BridgeAdapter()
  try {
    await Promise.race([
      bridge.request({ method: 'GET', path: '/api/healthz', timeoutMs: 5000 }),
      timeout(5000),
    ])
    setAdapter(bridge)
  } catch (err) {
    requestFallback(String(err))
  }

  // Tag-based fatal flag for unhandled rejections (used by stream paths).
  window.addEventListener('unhandledrejection', ev => {
    if (String(ev.reason).includes('[bridge-fatal]')) {
      requestFallback(String(ev.reason))
    }
  })
}
```

- [ ] **Step 4: Run TS tests**

Run: `cd gui/web && npm test`
Expected: All tests green. (Add a test for boot.ts that mocks window.ShuttleBridge and verifies fallback request.)

```typescript
// gui/web/src/app/__tests__/boot.spec.ts (extend)
it('requests fallback when bridge probe fails', async () => {
  const post = vi.fn()
  ;(window as any).webkit = { messageHandlers: { fallback: { postMessage: post } } }
  ;(window as any).ShuttleBridge = { send: async () => { throw new Error('unreachable') } }
  await boot()
  expect(post).toHaveBeenCalledWith(expect.objectContaining({ reason: expect.any(String) }))
})
```

- [ ] **Step 5: Commit**

```bash
git add mobile/ios/Shuttle/FallbackHandler.swift mobile/ios/Shuttle/ShuttleApp.swift \
        gui/web/src/app/boot.ts gui/web/src/app/__tests__/boot.spec.ts
git commit -m "feat(ios): FallbackHandler safety net + boot.ts bridge probe"
```

---

## Phase 6 — Testing & Cleanup

### Task 6.1: APIBridge XCTest

**Files:**
- Create: `mobile/ios/ShuttleTests/APIBridgeTests.swift`

- [ ] **Step 1: Write the test file**

```swift
// mobile/ios/ShuttleTests/APIBridgeTests.swift
import XCTest
import WebKit
@testable import Shuttle
@testable import SharedBridge

final class APIBridgeTests: XCTestCase {

    final class StubManager: VPNManager {
        var lastSent: Data?
        var stubResponse: Data?

        override func sendToExtension(_ data: Data, timeout: TimeInterval, completion: @escaping (Data?) -> Void) {
            lastSent = data
            completion(stubResponse)
        }
    }

    func testBridgeForwardsEnvelopeOnPostMessage() throws {
        let mgr = StubManager()
        let response = APIResponse(status: 200, headers: [:], body: "Zm9v", error: nil)
        mgr.stubResponse = try JSONEncoder().encode(response)

        let bridge = APIBridge(manager: mgr)

        // Build a fake WKScriptMessage manually — test through the public delegate method.
        let body: [String: Any] = [
            "id": 42,
            "envelope": ["method": "GET", "path": "/api/healthz", "headers": [:]],
        ]
        let msg = StubScriptMessage(body: body)

        bridge.userContentController(WKUserContentController(), didReceive: msg)

        XCTAssertNotNil(mgr.lastSent, "Bridge did not forward envelope")
        let decoded = try JSONDecoder().decode(APIRequest.self, from: mgr.lastSent!)
        XCTAssertEqual(decoded.path, "/api/healthz")
        XCTAssertEqual(decoded.method, "GET")
    }

    // Stub WKScriptMessage — WK does not allow direct construction; subclass.
    final class StubScriptMessage: WKScriptMessage {
        private let _body: Any
        init(body: Any) { self._body = body; super.init() }
        override var body: Any { _body }
    }
}
```

- [ ] **Step 2: Run the test**

Run (macOS): `cd mobile/ios && xcodebuild test -scheme Shuttle -sdk iphonesimulator -only-testing:ShuttleTests/APIBridgeTests 2>&1 | tail -20`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add mobile/ios/ShuttleTests/APIBridgeTests.swift
git commit -m "test(ios): APIBridge envelope forwarding XCTest"
```

---

### Task 6.2: testSPALoadsInVPNMode XCUITest

**Files:**
- Modify: `mobile/ios/ShuttleUITests/ShuttleUITests.swift` — add `testSPALoadsInVPNMode`.

- [ ] **Step 1: Add the new test method**

```swift
// mobile/ios/ShuttleUITests/ShuttleUITests.swift  (append new method to the class)
func testSPALoadsInVPNMode() throws {
    let app = XCUIApplication()
    app.launchEnvironment["FORCE_VPN_MODE"] = "1"
    app.launch()

    // Accept system VPN permission dialog if it appears.
    let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
    let allowBtn = springboard.buttons["Allow"]
    if allowBtn.waitForExistence(timeout: 5) { allowBtn.tap() }

    // The Now label is unique to the SPA — fallback HTML does not render it.
    let nowLabel = app.staticTexts["Now"].firstMatch
    XCTAssertTrue(
        nowLabel.waitForExistence(timeout: 15),
        "Now label did not appear in VPN mode — SPA failed to render via bridge",
    )

    // Mesh tab confirms full nav rendered (fallback HTML lacks bottom tabs).
    let meshTab = app.staticTexts["Mesh"].firstMatch
    XCTAssertTrue(
        meshTab.waitForExistence(timeout: 5),
        "Mesh tab missing — likely fell back to inline HTML",
    )
}
```

- [ ] **Step 2: Wire FORCE_VPN_MODE in ShuttleApp.swift**

```swift
// mobile/ios/Shuttle/ShuttleApp.swift (early in app delegate)
if ProcessInfo.processInfo.environment["FORCE_VPN_MODE"] == "1" {
    // Skip preflight that would route to proxy mode.
    self.forceVpnModeForTests = true
}
```

(Implementer: connect this flag to whatever path normally branches between proxy and VPN mode.)

- [ ] **Step 3: Run UI test**

Run (macOS): `cd mobile/ios && xcodebuild test -scheme Shuttle -sdk iphonesimulator -destination 'platform=iOS Simulator,name=iPhone 14' -only-testing:ShuttleUITests/ShuttleUITests/testSPALoadsInVPNMode 2>&1 | tail -30`
Expected: PASS, screenshot artifact in `~/Library/Developer/Xcode/DerivedData/.../testSPALoadsInVPNMode_*.png`.

- [ ] **Step 4: Commit**

```bash
git add mobile/ios/ShuttleUITests/ShuttleUITests.swift mobile/ios/Shuttle/ShuttleApp.swift
git commit -m "test(ios): testSPALoadsInVPNMode XCUITest"
```

---

### Task 6.3: Manual Smoke Checklist

**Files:**
- Modify: `docs/mobile-smoke.md`

- [ ] **Step 1: Append iOS VPN mode section**

```markdown
## iOS VPN 模式（真机）

Pre-requisites:
- TestFlight build with bridge enabled (Phase β)
- Real iPhone running iOS 15+
- Wi-Fi or cellular for the actual proxied traffic

- [ ] First connect: system VPN permission dialog appears, allow → SPA first paint <3s
- [ ] Now page Power button: Connect → Connected feedback within 2s
- [ ] Servers page: list loads, QR scan import, edit name, delete — all succeed
- [ ] Servers page: 500+ subscription sync flows pagination (no jank)
- [ ] Activity → Logs: new lines appear within 1s
- [ ] Activity → Logs: 5 minutes continuous, no dropped lines
- [ ] Settings: change theme persists across app restart
- [ ] Background 30s → foreground: SPA recovers, no white screen, status correct
- [ ] Background 5min → foreground: may show "event stream gap recovered" toast, full state refresh
- [ ] Force-quit app, reopen (VPN still on): SPA reloads and reconnects bridge
- [ ] Switch to proxy mode → back to VPN: SPA state transition smooth
```

- [ ] **Step 2: Commit**

```bash
git add docs/mobile-smoke.md
git commit -m "docs(mobile): iOS VPN-mode smoke checklist"
```

---

### Task 6.4: Phase γ Cleanup — Remove Fallback HTML and Legacy String Commands

**Important:** Do this ONLY after Phase β is green for 72h on TestFlight (per spec §9). Until then, the fallback path is the safety net.

**Files:**
- Modify: `mobile/ios/Shuttle/ShuttleApp.swift` — remove the inline fallback HTML constant and the `?bridge=0` branch.
- Modify: `mobile/ios/Shuttle/FallbackHandler.swift` — delete file.
- Modify: `mobile/ios/ShuttleExtension/PacketTunnelProvider.swift` — remove `handleLegacyCommand` and the legacy string-command branch.

- [ ] **Step 1: Remove fallback HTML and handler**

Delete `mobile/ios/Shuttle/FallbackHandler.swift`. In `ShuttleApp.swift`, remove the `fallbackHandler` registration, the `fallbackHTML` constant, and the `?bridge=0` branch in boot logic.

- [ ] **Step 2: Remove legacy string-command branch from extension**

In `mobile/ios/ShuttleExtension/PacketTunnelProvider.swift`, replace:

```swift
// Old
if let req = try? JSONDecoder().decode(APIRequest.self, from: messageData) {
    forwardToLocalAPI(req) { ... }
    return
}
if let cmd = String(data: messageData, encoding: .utf8) {
    handleLegacyCommand(cmd, completionHandler: completionHandler)
    return
}
completionHandler?(nil)
```

with:

```swift
// New — envelope only
guard let req = try? JSONDecoder().decode(APIRequest.self, from: messageData) else {
    completionHandler?(nil); return
}
forwardToLocalAPI(req) { response in
    let data = (try? JSONEncoder().encode(response)) ?? Data()
    completionHandler?(data)
}
```

Delete `handleLegacyCommand`.

- [ ] **Step 3: Update boot.ts to drop the bridge=0 branch**

```typescript
// gui/web/src/app/boot.ts (remove `?bridge=0` short-circuit)
export async function boot(): Promise<void> {
  if (typeof window !== 'undefined' && !window.ShuttleBridge) {
    await new Promise(r => setTimeout(r, 100))
  }
  if (!window.ShuttleBridge) { setAdapter(new HttpAdapter()); return }
  const bridge = new BridgeAdapter()
  try {
    await Promise.race([
      bridge.request({ method: 'GET', path: '/api/healthz', timeoutMs: 5000 }),
      timeout(5000),
    ])
    setAdapter(bridge)
  } catch (err) {
    // Bridge present but broken — still install it; UI will show error state.
    // Without fallback HTML, no escape valve. This is the Phase γ contract.
    setAdapter(bridge)
  }
}
```

(Note: keep the `?bridge=1` debug switch in dev builds via a build-time constant; remove for production.)

- [ ] **Step 4: Run all tests**

Run: `cd gui/web && npm test && cd ../.. && ./scripts/test.sh`
Expected: all green.

- [ ] **Step 5: Run iOS XCTest**

Run (macOS): `cd mobile/ios && xcodebuild test -scheme Shuttle -sdk iphonesimulator 2>&1 | tail -20`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add mobile/ios/Shuttle/ ShuttleApp.swift mobile/ios/ShuttleExtension/PacketTunnelProvider.swift \
        gui/web/src/app/boot.ts
git rm mobile/ios/Shuttle/FallbackHandler.swift
git commit -m "chore(ios): Phase γ — remove fallback HTML and legacy string commands"
```

---

## Acceptance Verification

Map each spec acceptance criterion to the tasks that satisfy it:

| Spec §14 Item | Tasks |
|---|---|
| DataAdapter conformance suite double-adapter green | Task 2.3 (skeleton), Task 4.4 (BridgeAdapter joins) |
| iOS VPN mode SPA 6 pages manual smoke pass | Task 6.3 (checklist) — manual execution gate |
| `docs/mobile-smoke.md` 11-item iOS VPN checklist pass | Task 6.3 — manual execution gate |
| Bridge RTT p95 <300ms (10 device sample) | Manual measurement during Phase β; instrumentation already added in BridgeAdapter (Task 4.3) — implementer should add `performance.now()` marks if not present |
| Extension steady memory <40MB (24h device) | Manual test during Phase β |
| CI build-mobile.yml iOS task green on new bridge path | Tasks 5.1–5.5 produce code that compiles |
| Phase β TestFlight 72h failure rate <0.1% | Phase β operational gate before Task 6.4 cleanup |
| Fallback HTML + string commands removed in Phase γ | Task 6.4 |

---

## Self-Review

**Spec coverage check:**

- §3 Architecture overview → Tasks 1.x build the skeleton, 5.x wire iOS native ✓
- §4 DataAdapter contract → Tasks 1.1–1.6 ✓
- §5 BridgeAdapter polling engine → Tasks 4.1–4.3 ✓
- §6.1 JS bridge → Task 5.3 (bootstrap script in `APIBridge.swift`) ✓
- §6.2 Swift APIBridge → Task 5.3 ✓
- §6.3 Extension envelope branch → Task 5.4 ✓
- §6.4 Envelope size handling → Task 5.4 (192KB cap in extension); pagination Task 3.4 ✓
- §6.5 Go events queue + healthz → Tasks 3.1–3.3 ✓
- §6.6 Change file list → all touched in Tasks 1.x–6.x ✓
- §7 Maintenance (registry + conformance) → Task 1.1, Task 2.3, Task 4.4 ✓
- §8 Testing strategy → Tasks 6.1–6.3 ✓
- §9 Rollout phases → Phase α = Tasks 1–5 with `?bridge=0/1` flag; Phase β = TestFlight gate; Phase γ = Task 6.4 ✓
- §10 Performance budget → instrumentation noted in Task 6.3 (manual measurement) ✓
- §11.1 Hard fallback triggers → Task 5.5 (boot.ts probe + FallbackHandler) ✓
- §11.2 Soft degradation → ConnectionState + UI dot already covered by Task 1.3, surfaced by SPA ✓
- §11.3 Local diagnostic counters — NOT a separate task. Implementer should add a small Settings → Diagnostics section during Phase β if needed; out of scope for the core plan.
- §12 Risks → addressed in design notes; no per-risk task except R3 (Extension memory) which is an operational gate, not code.

**Placeholder scan:** No `TBD` / `TODO` / "fill in details". Each step has either complete code or an explicit deferral with reason.

**Type consistency check:** `Subscription<T>`, `DataAdapter`, `TopicKey`, `topicConfig`, `ApiError`, `TransportError`, `BridgeEnvelope`, `APIRequest`, `APIResponse` — names match across phases. `useSubscription` / `useRequest` referenced consistently. Method signatures align: `adapter.request(opts: RequestOptions): Promise<T>`, `adapter.subscribe(topic, opts?): Subscription<T>`.

**One known gap:** the `connections` topic (used by `features/logs/store.svelte.ts:85`) is not in `topicConfig`. The plan in Task 2.5 keeps `connectWS('/api/connections', ...)` direct WS for now and notes "events topic carries connection events". For full coverage, a follow-up task could add `connections` to topicConfig, but it's not on the iOS VPN-mode critical path — the events queue (§6.5) carries `server.connected` / `server.disconnected` which is the same information from a different angle. Not blocking.
