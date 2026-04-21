# Mobile Unification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Consolidate Shuttle's 9-page SPA into a 6-page IA (Now / Servers / Traffic / Mesh / Activity / Settings) that renders identically across desktop (sidebar), tablet (rail), and phone (bottom tabs); wire the native WebView bridge so mobile Connect fires OS-level VPN permission prompts.

**Architecture:** Single responsive SPA — one `.svelte` file per page. AppShell selects nav component (Sidebar / Rail / BottomTabs) by viewport. New `lib/platform/` abstracts engine control so SPA calls `platform.*` regardless of runtime (web REST / native bridge / Wails binding). Old routes (`/dashboard`, `/subscriptions`, `/groups`, `/routing`, `/logs`) redirect client-side with a one-time toast. iOS VPN inline HTML is preserved untouched (follow-up spec).

**Tech Stack:** Svelte 5 + vite + vitest + Playwright + bits-ui + Wails (Go) + gomobile (Android AAR / iOS xcframework) + Kotlin + Swift.

**Reference:** `docs/superpowers/specs/2026-04-21-mobile-unification-design.md`

---

## File Map

### Created
```
gui/web/src/lib/
  viewport.svelte.ts                 responsive store
  viewport.svelte.test.ts
  platform/
    index.ts                         runtime detection + unified export
    types.ts                         Platform interface, capability types
    web.ts                           REST-based default
    web.test.ts
    native.ts                        iOS/Android WebView bridge
    native.test.ts
    wails.ts                         desktop Go bindings
    wails.test.ts
    detect.test.ts                   detect() across env shapes

gui/web/src/ui/
  Stack.svelte                       breakpoint-aware stack
  Stack.test.ts
  ResponsiveGrid.svelte              breakpoint columns grid
  ResponsiveGrid.test.ts

gui/web/src/app/
  nav.ts                             single nav source (6 items)
  nav.test.ts
  AppShell.svelte                    replaces Shell.svelte
  TopBar.svelte                      always-present header + gear
  TopBar.test.ts
  BottomTabs.svelte                  phone nav
  BottomTabs.test.ts
  Rail.svelte                        tablet nav
  Rail.test.ts
  route-migration.ts                 legacy → new URL mapping
  route-migration.test.ts

gui/web/src/features/
  now/
    Now.svelte                       power button page (replaces Dashboard hero)
    Now.test.ts
    PowerButton.svelte               circular 3-state toggle
    PowerButton.test.ts
    ServerChip.svelte                tap-to-picker pill
    ServerChip.test.ts
  activity/
    Activity.svelte                  speed + transports + logs
    Activity.test.ts
  traffic/                           rename from routing/ (git mv)
    RuleDetail.svelte                mobile rule form
    RuleDetail.test.ts

mobile/android/app/src/main/assets/
  shuttle-bridge.js                  shared JS bridge wrapper

build/scripts/
  build-android.sh
  build-ios.sh

.github/workflows/
  build-mobile.yml

mobile/ios/ShuttleUITests/
  ShuttleUITests.swift               launch + power button smoke
mobile/android/app/src/androidTest/java/com/shuttle/app/
  MainActivityTest.kt                launch + power button smoke

docs/mobile-smoke.md                 manual release checklist
```

### Modified
```
gui/web/src/app/
  routes.ts                          new 6 routes + legacy redirects
  App.svelte                         Shell → AppShell (behind VITE_USE_LEGACY_SHELL flag)
  Sidebar.svelte                     reads nav.ts (instead of routes.ts filter)
gui/web/src/features/servers/        absorb Subscriptions + Groups logic
gui/web/src/features/routing/        rename dir → traffic/, add tabs
gui/web/src/features/settings/       regroup Advanced collapse
gui/web/src/lib/i18n/locales/*.ts    add nav.{now,traffic,mesh,activity}
gui/web/playwright.config.ts         3-viewport matrix
gui/web/tests/                       add responsive.spec.ts, navigation.spec.ts, connect-flow.spec.ts
mobile/android/app/src/main/java/com/shuttle/app/MainActivity.kt
                                     extend bridge: requestPermission / scanQR / share / openExternal / subscribeStatus
mobile/ios/Shuttle/ShuttleApp.swift  extend message handler with same actions
build/scripts/build-all.sh           add --mobile flag
```

### Deleted (post-rollout, not this plan)
```
gui/web/src/features/subscriptions/  absorbed into servers/
gui/web/src/features/groups/         absorbed into servers/
gui/web/src/features/dashboard/      replaced by now/
gui/web/src/features/logs/           absorbed into activity/
gui/web/src/app/Shell.svelte         replaced by AppShell.svelte
```

---

## Phase 1 · Infrastructure

**Goal:** Ship `lib/viewport`, `lib/platform/*`, `ui/Stack`, `ui/ResponsiveGrid`, and the three nav components (TopBar, BottomTabs, Rail) plus `AppShell`. Nothing wired into user-visible routes yet. All engine calls migrated to `platform.engineStart/Stop/Status` with REST-equivalent behavior.

### Task 1.1: Viewport store

**Files:**
- Create: `gui/web/src/lib/viewport.svelte.ts`
- Test: `gui/web/src/lib/viewport.svelte.test.ts`

- [ ] **Step 1.1.1: Write the failing test**

File: `gui/web/src/lib/viewport.svelte.test.ts`

```ts
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { viewport, __reset } from './viewport.svelte'

describe('viewport', () => {
  beforeEach(() => { __reset() })
  afterEach(() => { vi.restoreAllMocks() })

  it('classifies widths into form buckets', () => {
    __reset(360)
    expect(viewport.form).toBe('xs')
    expect(viewport.isMobile).toBe(true)
    expect(viewport.isTablet).toBe(false)
    expect(viewport.isDesktop).toBe(false)

    __reset(600)
    expect(viewport.form).toBe('sm')
    expect(viewport.isMobile).toBe(true)

    __reset(820)
    expect(viewport.form).toBe('md')
    expect(viewport.isMobile).toBe(false)
    expect(viewport.isTablet).toBe(true)

    __reset(1280)
    expect(viewport.form).toBe('lg')
    expect(viewport.isDesktop).toBe(true)

    __reset(1600)
    expect(viewport.form).toBe('xl')
  })

  it('detects touch via pointer: coarse media query', () => {
    const matchMedia = vi.fn((q: string) => ({
      matches: q === '(pointer: coarse)',
      media: q, onchange: null,
      addEventListener: () => {}, removeEventListener: () => {},
      addListener: () => {}, removeListener: () => {}, dispatchEvent: () => false,
    }))
    Object.defineProperty(window, 'matchMedia', { value: matchMedia, writable: true })
    __reset(500)
    expect(viewport.isTouch).toBe(true)
  })
})
```

- [ ] **Step 1.1.2: Run test to verify it fails**

```bash
cd gui/web && npx vitest run src/lib/viewport.svelte.test.ts
```

Expected: FAIL — `Cannot find module './viewport.svelte'`.

- [ ] **Step 1.1.3: Implement the store**

File: `gui/web/src/lib/viewport.svelte.ts`

```ts
// Responsive viewport store. Single source of truth for viewport-driven
// layout decisions. One ResizeObserver on <html>, rAF-throttled.

export type Form = 'xs' | 'sm' | 'md' | 'lg' | 'xl'

interface Viewport {
  width: number
  form: Form
  isMobile: boolean    // xs | sm
  isTablet: boolean    // md
  isDesktop: boolean   // lg | xl
  isTouch: boolean
}

function classify(w: number): Form {
  if (w < 480)  return 'xs'
  if (w < 720)  return 'sm'
  if (w < 1024) return 'md'
  if (w < 1440) return 'lg'
  return 'xl'
}

function detectTouch(): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return false
  return window.matchMedia('(pointer: coarse)').matches
}

const initialWidth = typeof window !== 'undefined' ? window.innerWidth : 1440
const initialForm = classify(initialWidth)

export const viewport = $state<Viewport>({
  width: initialWidth,
  form: initialForm,
  isMobile: initialForm === 'xs' || initialForm === 'sm',
  isTablet: initialForm === 'md',
  isDesktop: initialForm === 'lg' || initialForm === 'xl',
  isTouch: detectTouch(),
})

function apply(w: number) {
  const f = classify(w)
  viewport.width = w
  viewport.form = f
  viewport.isMobile = f === 'xs' || f === 'sm'
  viewport.isTablet = f === 'md'
  viewport.isDesktop = f === 'lg' || f === 'xl'
  viewport.isTouch = detectTouch()
}

if (typeof window !== 'undefined' && typeof ResizeObserver !== 'undefined') {
  let pending = false
  const ro = new ResizeObserver((entries) => {
    if (pending) return
    pending = true
    requestAnimationFrame(() => {
      pending = false
      const e = entries[0]
      const w = e?.contentRect?.width ?? window.innerWidth
      apply(w)
    })
  })
  ro.observe(document.documentElement)
}

// Test helper
export function __reset(width: number = 1440): void {
  apply(width)
}
```

- [ ] **Step 1.1.4: Run test**

```bash
cd gui/web && npx vitest run src/lib/viewport.svelte.test.ts
```

Expected: PASS.

- [ ] **Step 1.1.5: Commit**

```bash
git add gui/web/src/lib/viewport.svelte.ts gui/web/src/lib/viewport.svelte.test.ts
git commit -m "feat(gui): add viewport store with breakpoint classification"
```

---

### Task 1.2: Platform types

**Files:**
- Create: `gui/web/src/lib/platform/types.ts`

- [ ] **Step 1.2.1: Write the types file** (no test — pure types)

File: `gui/web/src/lib/platform/types.ts`

```ts
import type { Status } from '../api/types'

export type PlatformName = 'web' | 'native' | 'wails'

export type CapResult<T> = T | 'unsupported'

export interface SharePayload {
  title?: string
  text?: string
  url?: string
}

export interface Platform {
  readonly name: PlatformName

  // Engine lifecycle
  engineStart(): Promise<void>
  engineStop(): Promise<void>
  engineStatus(): Promise<Status>

  // OS-level capabilities
  requestVpnPermission(): Promise<CapResult<'granted' | 'denied'>>
  scanQRCode(): Promise<CapResult<string>>
  share(payload: SharePayload): Promise<CapResult<'ok' | 'cancelled'>>
  openExternalUrl(url: string): Promise<CapResult<'ok'>>

  // Observation
  onStatusChange(cb: (s: Status) => void): () => void
}
```

- [ ] **Step 1.2.2: Commit**

```bash
git add gui/web/src/lib/platform/types.ts
git commit -m "feat(gui): platform types — Platform interface, CapResult"
```

---

### Task 1.3: Web platform runtime (default, REST)

**Files:**
- Create: `gui/web/src/lib/platform/web.ts`
- Test: `gui/web/src/lib/platform/web.test.ts`

- [ ] **Step 1.3.1: Write the failing test**

File: `gui/web/src/lib/platform/web.test.ts`

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { WebPlatform } from './web'
import * as endpoints from '../api/endpoints'

describe('WebPlatform', () => {
  let p: WebPlatform
  beforeEach(() => {
    p = new WebPlatform()
    vi.restoreAllMocks()
  })

  it('engineStart posts /api/connect', async () => {
    const spy = vi.spyOn(endpoints, 'connect').mockResolvedValue(undefined as any)
    await p.engineStart()
    expect(spy).toHaveBeenCalled()
  })

  it('requestVpnPermission returns unsupported on web', async () => {
    expect(await p.requestVpnPermission()).toBe('unsupported')
  })

  it('scanQRCode returns unsupported on web', async () => {
    expect(await p.scanQRCode()).toBe('unsupported')
  })

  it('share uses navigator.share when available', async () => {
    const share = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'share', { value: share, configurable: true })
    expect(await p.share({ title: 't', url: 'u' })).toBe('ok')
    expect(share).toHaveBeenCalledWith({ title: 't', url: 'u' })
  })

  it('share falls back to clipboard when navigator.share missing', async () => {
    Object.defineProperty(navigator, 'share', { value: undefined, configurable: true })
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    expect(await p.share({ url: 'https://x' })).toBe('ok')
    expect(writeText).toHaveBeenCalledWith('https://x')
  })

  it('openExternalUrl calls window.open', async () => {
    const open = vi.fn()
    vi.stubGlobal('open', open)
    expect(await p.openExternalUrl('https://x')).toBe('ok')
    expect(open).toHaveBeenCalledWith('https://x', '_blank')
  })

  it('name === web', () => { expect(p.name).toBe('web') })
})
```

- [ ] **Step 1.3.2: Run test to verify fail**

```bash
cd gui/web && npx vitest run src/lib/platform/web.test.ts
```

Expected: FAIL — module not found.

- [ ] **Step 1.3.3: Implement `web.ts`**

File: `gui/web/src/lib/platform/web.ts`

```ts
import type { Platform, PlatformName, CapResult, SharePayload } from './types'
import type { Status } from '../api/types'
import { connect, disconnect, status as getStatus } from '../api/endpoints'

export class WebPlatform implements Platform {
  readonly name: PlatformName = 'web'

  async engineStart(): Promise<void> { await connect() }
  async engineStop(): Promise<void> { await disconnect() }
  async engineStatus(): Promise<Status> { return getStatus() }

  async requestVpnPermission(): Promise<CapResult<'granted' | 'denied'>> {
    return 'unsupported'
  }
  async scanQRCode(): Promise<CapResult<string>> { return 'unsupported' }

  async share(payload: SharePayload): Promise<CapResult<'ok' | 'cancelled'>> {
    if (typeof navigator !== 'undefined' && typeof navigator.share === 'function') {
      try {
        await navigator.share(payload)
        return 'ok'
      } catch (e) {
        if ((e as Error).name === 'AbortError') return 'cancelled'
        // fall through to clipboard
      }
    }
    const text = payload.url ?? payload.text ?? payload.title ?? ''
    if (text && typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return 'ok'
    }
    return 'unsupported'
  }

  async openExternalUrl(url: string): Promise<CapResult<'ok'>> {
    if (typeof window !== 'undefined') {
      window.open(url, '_blank')
      return 'ok'
    }
    return 'unsupported'
  }

  onStatusChange(cb: (s: Status) => void): () => void {
    // WebSocket subscription reuses existing /api/events. Kept simple for now:
    // poll every 2s; upgrade to WS in Task 1.6.
    const timer = setInterval(async () => {
      try { cb(await this.engineStatus()) } catch {}
    }, 2000)
    return () => clearInterval(timer)
  }
}
```

- [ ] **Step 1.3.4: Run test — verify pass**

```bash
cd gui/web && npx vitest run src/lib/platform/web.test.ts
```

Expected: PASS.

- [ ] **Step 1.3.5: Commit**

```bash
git add gui/web/src/lib/platform/web.ts gui/web/src/lib/platform/web.test.ts
git commit -m "feat(gui): web platform runtime — REST + Web APIs"
```

---

### Task 1.4: Native platform runtime (iOS/Android bridge)

**Files:**
- Create: `gui/web/src/lib/platform/native.ts`
- Test: `gui/web/src/lib/platform/native.test.ts`

- [ ] **Step 1.4.1: Write failing test**

File: `gui/web/src/lib/platform/native.test.ts`

```ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { NativePlatform } from './native'

function mountBridge(methods: Record<string, any>) {
  (window as any).ShuttleVPN = methods
}

describe('NativePlatform', () => {
  let p: NativePlatform
  beforeEach(() => {
    p = new NativePlatform()
    delete (window as any).ShuttleVPN
  })
  afterEach(() => { vi.restoreAllMocks() })

  it('requestVpnPermission returns unsupported when method missing', async () => {
    mountBridge({})
    expect(await p.requestVpnPermission()).toBe('unsupported')
  })

  it('requestVpnPermission calls bridge when present', async () => {
    const fn = vi.fn().mockResolvedValue('granted')
    mountBridge({ requestPermission: fn })
    expect(await p.requestVpnPermission()).toBe('granted')
    expect(fn).toHaveBeenCalled()
  })

  it('scanQRCode returns unsupported when method missing', async () => {
    mountBridge({})
    expect(await p.scanQRCode()).toBe('unsupported')
  })

  it('scanQRCode returns scanned string when bridge present', async () => {
    mountBridge({ scanQR: vi.fn().mockResolvedValue('shuttle://abc') })
    expect(await p.scanQRCode()).toBe('shuttle://abc')
  })

  it('name === native', () => { expect(p.name).toBe('native') })
})
```

- [ ] **Step 1.4.2: Run test — verify fail**

```bash
cd gui/web && npx vitest run src/lib/platform/native.test.ts
```

Expected: FAIL — module not found.

- [ ] **Step 1.4.3: Implement `native.ts`**

File: `gui/web/src/lib/platform/native.ts`

```ts
import type { Platform, PlatformName, CapResult, SharePayload } from './types'
import type { Status } from '../api/types'
import { connect, disconnect, status as getStatus } from '../api/endpoints'

interface ShuttleBridge {
  start?: () => Promise<void> | void
  stop?: () => Promise<void> | void
  isRunning?: () => Promise<boolean> | boolean
  requestPermission?: () => Promise<'granted' | 'denied'>
  scanQR?: () => Promise<string>
  share?: (payload: SharePayload) => Promise<'ok' | 'cancelled'>
  openExternal?: (url: string) => Promise<void> | void
  subscribeStatus?: (cb: (s: Status) => void) => () => void
}

function bridge(): ShuttleBridge | null {
  if (typeof window === 'undefined') return null
  return (window as any).ShuttleVPN ?? null
}

function hasMethod<K extends keyof ShuttleBridge>(k: K): boolean {
  const b = bridge()
  return !!b && typeof b[k] === 'function'
}

export class NativePlatform implements Platform {
  readonly name: PlatformName = 'native'

  async engineStart(): Promise<void> {
    if (hasMethod('start')) {
      await bridge()!.start!()
      return
    }
    await connect()
  }

  async engineStop(): Promise<void> {
    if (hasMethod('stop')) {
      await bridge()!.stop!()
      return
    }
    await disconnect()
  }

  async engineStatus(): Promise<Status> { return getStatus() }

  async requestVpnPermission(): Promise<CapResult<'granted' | 'denied'>> {
    if (!hasMethod('requestPermission')) return 'unsupported'
    return await bridge()!.requestPermission!()
  }

  async scanQRCode(): Promise<CapResult<string>> {
    if (!hasMethod('scanQR')) return 'unsupported'
    return await bridge()!.scanQR!()
  }

  async share(payload: SharePayload): Promise<CapResult<'ok' | 'cancelled'>> {
    if (!hasMethod('share')) return 'unsupported'
    return await bridge()!.share!(payload)
  }

  async openExternalUrl(url: string): Promise<CapResult<'ok'>> {
    if (!hasMethod('openExternal')) return 'unsupported'
    await bridge()!.openExternal!(url)
    return 'ok'
  }

  onStatusChange(cb: (s: Status) => void): () => void {
    if (hasMethod('subscribeStatus')) {
      return bridge()!.subscribeStatus!(cb)
    }
    const timer = setInterval(async () => {
      try { cb(await this.engineStatus()) } catch {}
    }, 2000)
    return () => clearInterval(timer)
  }
}
```

- [ ] **Step 1.4.4: Run test — pass**

```bash
cd gui/web && npx vitest run src/lib/platform/native.test.ts
```

- [ ] **Step 1.4.5: Commit**

```bash
git add gui/web/src/lib/platform/native.ts gui/web/src/lib/platform/native.test.ts
git commit -m "feat(gui): native platform runtime — WebView bridge with capability checks"
```

---

### Task 1.5: Wails platform runtime

**Files:**
- Create: `gui/web/src/lib/platform/wails.ts`
- Test: `gui/web/src/lib/platform/wails.test.ts`

- [ ] **Step 1.5.1: Write failing test**

File: `gui/web/src/lib/platform/wails.test.ts`

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { WailsPlatform } from './wails'

function mountWails(api: Record<string, any>, runtime: Record<string, any> = {}) {
  (window as any).go = { main: { App: api } }
  ;(window as any).runtime = runtime
}

describe('WailsPlatform', () => {
  let p: WailsPlatform
  beforeEach(() => {
    delete (window as any).go
    delete (window as any).runtime
    p = new WailsPlatform()
  })

  it('engineStart calls Go binding when present', async () => {
    const fn = vi.fn().mockResolvedValue(undefined)
    mountWails({ EngineStart: fn })
    await p.engineStart()
    expect(fn).toHaveBeenCalled()
  })

  it('openExternalUrl uses Wails runtime BrowserOpenURL', async () => {
    const openURL = vi.fn()
    mountWails({}, { BrowserOpenURL: openURL })
    expect(await p.openExternalUrl('https://x')).toBe('ok')
    expect(openURL).toHaveBeenCalledWith('https://x')
  })

  it('scanQRCode returns unsupported on wails', async () => {
    mountWails({})
    expect(await p.scanQRCode()).toBe('unsupported')
  })

  it('name === wails', () => { expect(p.name).toBe('wails') })
})
```

- [ ] **Step 1.5.2: Run — fail**

```bash
cd gui/web && npx vitest run src/lib/platform/wails.test.ts
```

- [ ] **Step 1.5.3: Implement `wails.ts`**

File: `gui/web/src/lib/platform/wails.ts`

```ts
import type { Platform, PlatformName, CapResult, SharePayload } from './types'
import type { Status } from '../api/types'
import { connect, disconnect, status as getStatus } from '../api/endpoints'

function wailsApp(): Record<string, any> | null {
  if (typeof window === 'undefined') return null
  return ((window as any).go?.main?.App) ?? null
}

function wailsRuntime(): Record<string, any> | null {
  if (typeof window === 'undefined') return null
  return (window as any).runtime ?? null
}

export class WailsPlatform implements Platform {
  readonly name: PlatformName = 'wails'

  async engineStart(): Promise<void> {
    const app = wailsApp()
    if (app?.EngineStart) { await app.EngineStart(); return }
    await connect()
  }
  async engineStop(): Promise<void> {
    const app = wailsApp()
    if (app?.EngineStop) { await app.EngineStop(); return }
    await disconnect()
  }
  async engineStatus(): Promise<Status> {
    const app = wailsApp()
    if (app?.EngineStatus) return await app.EngineStatus()
    return getStatus()
  }

  async requestVpnPermission(): Promise<CapResult<'granted' | 'denied'>> { return 'unsupported' }
  async scanQRCode(): Promise<CapResult<string>> { return 'unsupported' }
  async share(): Promise<CapResult<'ok' | 'cancelled'>> { return 'unsupported' }

  async openExternalUrl(url: string): Promise<CapResult<'ok'>> {
    const r = wailsRuntime()
    if (r?.BrowserOpenURL) { r.BrowserOpenURL(url); return 'ok' }
    return 'unsupported'
  }

  onStatusChange(cb: (s: Status) => void): () => void {
    const timer = setInterval(async () => {
      try { cb(await this.engineStatus()) } catch {}
    }, 2000)
    return () => clearInterval(timer)
  }
}
```

- [ ] **Step 1.5.4: Run — pass**

```bash
cd gui/web && npx vitest run src/lib/platform/wails.test.ts
```

- [ ] **Step 1.5.5: Commit**

```bash
git add gui/web/src/lib/platform/wails.ts gui/web/src/lib/platform/wails.test.ts
git commit -m "feat(gui): wails platform runtime — Go bindings passthrough"
```

---

### Task 1.6: Platform detect + unified index

**Files:**
- Create: `gui/web/src/lib/platform/index.ts`
- Test: `gui/web/src/lib/platform/detect.test.ts`

- [ ] **Step 1.6.1: Write failing test**

File: `gui/web/src/lib/platform/detect.test.ts`

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { detect, __resetPlatform } from './index'

describe('detect', () => {
  beforeEach(() => {
    delete (window as any).go
    delete (window as any).ShuttleVPN
    __resetPlatform()
  })

  it('defaults to web', () => { expect(detect()).toBe('web') })

  it('detects wails when window.go.main.App present', () => {
    (window as any).go = { main: { App: {} } }
    expect(detect()).toBe('wails')
  })

  it('detects native when window.ShuttleVPN present', () => {
    (window as any).ShuttleVPN = {}
    expect(detect()).toBe('native')
  })

  it('wails takes precedence over native bridge', () => {
    (window as any).go = { main: { App: {} } }
    ;(window as any).ShuttleVPN = {}
    expect(detect()).toBe('wails')
  })
})
```

- [ ] **Step 1.6.2: Run — fail**

```bash
cd gui/web && npx vitest run src/lib/platform/detect.test.ts
```

- [ ] **Step 1.6.3: Implement `index.ts`**

File: `gui/web/src/lib/platform/index.ts`

```ts
import type { Platform, PlatformName } from './types'
import { WebPlatform } from './web'
import { NativePlatform } from './native'
import { WailsPlatform } from './wails'

export type { Platform, PlatformName, CapResult, SharePayload } from './types'

export function detect(): PlatformName {
  if (typeof window === 'undefined') return 'web'
  if ((window as any).go?.main?.App) return 'wails'
  if ((window as any).ShuttleVPN) return 'native'
  return 'web'
}

let _instance: Platform | null = null

export function getPlatform(): Platform {
  if (_instance) return _instance
  const name = detect()
  _instance = name === 'wails'  ? new WailsPlatform()
            : name === 'native' ? new NativePlatform()
            :                     new WebPlatform()
  return _instance
}

// Convenience: `platform` accessor — always returns the singleton.
export const platform = new Proxy({} as Platform, {
  get(_t, prop) { return (getPlatform() as any)[prop] },
})

// Test helper
export function __resetPlatform(): void { _instance = null }
```

- [ ] **Step 1.6.4: Run — pass**

```bash
cd gui/web && npx vitest run src/lib/platform/
```

Expected: all 4 platform test files pass.

- [ ] **Step 1.6.5: Commit**

```bash
git add gui/web/src/lib/platform/index.ts gui/web/src/lib/platform/detect.test.ts
git commit -m "feat(gui): platform detect + singleton — unified platform export"
```

---

### Task 1.7: Stack layout primitive

**Files:**
- Create: `gui/web/src/ui/Stack.svelte`
- Test: `gui/web/src/ui/Stack.test.ts`

- [ ] **Step 1.7.1: Write failing test**

File: `gui/web/src/ui/Stack.test.ts`

```ts
import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/svelte'
import Stack from './Stack.svelte'

describe('Stack', () => {
  it('defaults to column layout', () => {
    const { container } = render(Stack, { props: { gap: '3' } })
    const el = container.querySelector('.stack') as HTMLElement
    expect(el.dataset.direction).toBe('column')
  })

  it('applies breakAt for horizontal-above-breakpoint', () => {
    const { container } = render(Stack, { props: { breakAt: 'md' } })
    const el = container.querySelector('.stack') as HTMLElement
    expect(el.dataset.breakAt).toBe('md')
  })
})
```

- [ ] **Step 1.7.2: Run — fail**

```bash
cd gui/web && npx vitest run src/ui/Stack.test.ts
```

- [ ] **Step 1.7.3: Implement `Stack.svelte`**

File: `gui/web/src/ui/Stack.svelte`

```svelte
<script lang="ts">
  import type { Snippet } from 'svelte'
  type Form = 'xs' | 'sm' | 'md' | 'lg' | 'xl'
  interface Props {
    gap?: '1' | '2' | '3' | '4' | '5'
    direction?: 'row' | 'column'
    wrap?: boolean
    breakAt?: Form                 // when set, direction is 'row' at >= breakAt, 'column' below
    children?: Snippet
  }
  let { gap = '3', direction = 'column', wrap = false, breakAt, children }: Props = $props()
</script>

<div
  class="stack"
  data-direction={direction}
  data-gap={gap}
  data-wrap={wrap ? '1' : '0'}
  data-break-at={breakAt ?? ''}
>
  {@render children?.()}
</div>

<style>
  .stack {
    display: flex;
    flex-direction: var(--_dir, column);
    gap: var(--shuttle-space-3);
    flex-wrap: nowrap;
  }
  .stack[data-direction="row"] { --_dir: row; }
  .stack[data-gap="1"] { gap: var(--shuttle-space-1); }
  .stack[data-gap="2"] { gap: var(--shuttle-space-2); }
  .stack[data-gap="4"] { gap: var(--shuttle-space-4); }
  .stack[data-gap="5"] { gap: var(--shuttle-space-5); }
  .stack[data-wrap="1"] { flex-wrap: wrap; }

  /* breakAt variants — direction flips at the named breakpoint upward */
  @media (min-width: 480px)  { .stack[data-break-at="sm"]:not([data-direction="row"]) { flex-direction: row; } }
  @media (min-width: 720px)  { .stack[data-break-at="md"]:not([data-direction="row"]) { flex-direction: row; } }
  @media (min-width: 1024px) { .stack[data-break-at="lg"]:not([data-direction="row"]) { flex-direction: row; } }
</style>
```

- [ ] **Step 1.7.4: Run — pass**

- [ ] **Step 1.7.5: Commit**

```bash
git add gui/web/src/ui/Stack.svelte gui/web/src/ui/Stack.test.ts
git commit -m "feat(gui): Stack layout primitive with breakAt breakpoint flipping"
```

---

### Task 1.8: ResponsiveGrid primitive

**Files:**
- Create: `gui/web/src/ui/ResponsiveGrid.svelte`
- Test: `gui/web/src/ui/ResponsiveGrid.test.ts`

- [ ] **Step 1.8.1: Write failing test**

File: `gui/web/src/ui/ResponsiveGrid.test.ts`

```ts
import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/svelte'
import ResponsiveGrid from './ResponsiveGrid.svelte'

describe('ResponsiveGrid', () => {
  it('applies cols per breakpoint as CSS custom properties', () => {
    const { container } = render(ResponsiveGrid, {
      props: { cols: { xs: 1, md: 2, lg: 3 } },
    })
    const el = container.querySelector('.grid') as HTMLElement
    expect(el.style.getPropertyValue('--cols-xs')).toBe('1')
    expect(el.style.getPropertyValue('--cols-md')).toBe('2')
    expect(el.style.getPropertyValue('--cols-lg')).toBe('3')
  })
})
```

- [ ] **Step 1.8.2: Run — fail**

- [ ] **Step 1.8.3: Implement**

File: `gui/web/src/ui/ResponsiveGrid.svelte`

```svelte
<script lang="ts">
  import type { Snippet } from 'svelte'
  type Form = 'xs' | 'sm' | 'md' | 'lg' | 'xl'
  interface Props {
    cols?: Partial<Record<Form, number>>
    gap?: '1' | '2' | '3' | '4' | '5'
    children?: Snippet
  }
  let { cols = { xs: 1 }, gap = '3', children }: Props = $props()

  const styleVars = $derived.by(() => {
    const entries: string[] = []
    for (const [form, n] of Object.entries(cols)) {
      entries.push(`--cols-${form}: ${n}`)
    }
    return entries.join('; ')
  })
</script>

<div class="grid" data-gap={gap} style={styleVars}>
  {@render children?.()}
</div>

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(var(--cols-xs, 1), minmax(0, 1fr));
    gap: var(--shuttle-space-3);
  }
  .grid[data-gap="1"] { gap: var(--shuttle-space-1); }
  .grid[data-gap="2"] { gap: var(--shuttle-space-2); }
  .grid[data-gap="4"] { gap: var(--shuttle-space-4); }
  .grid[data-gap="5"] { gap: var(--shuttle-space-5); }

  @media (min-width: 480px)  { .grid { grid-template-columns: repeat(var(--cols-sm, var(--cols-xs, 1)), minmax(0, 1fr)); } }
  @media (min-width: 720px)  { .grid { grid-template-columns: repeat(var(--cols-md, var(--cols-sm, var(--cols-xs, 1))), minmax(0, 1fr)); } }
  @media (min-width: 1024px) { .grid { grid-template-columns: repeat(var(--cols-lg, var(--cols-md, var(--cols-sm, var(--cols-xs, 1)))), minmax(0, 1fr)); } }
  @media (min-width: 1440px) { .grid { grid-template-columns: repeat(var(--cols-xl, var(--cols-lg, var(--cols-md, var(--cols-sm, var(--cols-xs, 1))))), minmax(0, 1fr)); } }
</style>
```

- [ ] **Step 1.8.4: Run — pass**

- [ ] **Step 1.8.5: Commit**

```bash
git add gui/web/src/ui/ResponsiveGrid.svelte gui/web/src/ui/ResponsiveGrid.test.ts
git commit -m "feat(gui): ResponsiveGrid primitive — cols per breakpoint"
```

---

### Task 1.9: Nav source of truth

**Files:**
- Create: `gui/web/src/app/nav.ts`
- Test: `gui/web/src/app/nav.test.ts`

- [ ] **Step 1.9.1: Write failing test**

File: `gui/web/src/app/nav.test.ts`

```ts
import { describe, it, expect } from 'vitest'
import { nav, primaryNav, navById } from './nav'

describe('nav', () => {
  it('has 6 items', () => { expect(nav.length).toBe(6) })
  it('5 are primary (for bottom tabs)', () => {
    expect(primaryNav().length).toBe(5)
  })
  it('settings is not primary', () => {
    expect(navById('settings')?.primary).toBe(false)
  })
  it('all items have unique IDs', () => {
    const ids = nav.map(i => i.id)
    expect(new Set(ids).size).toBe(ids.length)
  })
  it('sections cover overview / network / system', () => {
    const sections = new Set(nav.map(i => i.section))
    expect(sections.has('overview')).toBe(true)
    expect(sections.has('network')).toBe(true)
    expect(sections.has('system')).toBe(true)
  })
})
```

- [ ] **Step 1.9.2: Run — fail**

- [ ] **Step 1.9.3: Implement**

File: `gui/web/src/app/nav.ts`

```ts
import type { IconName } from './icons'

export type NavSection = 'overview' | 'network' | 'system'

export interface NavItem {
  id: string
  path: string
  label: string      // i18n key
  icon: IconName
  section: NavSection
  order: number      // for sort within section
  primary: boolean   // true → appears in BottomTabs (else only TopBar gear / Sidebar)
}

export const nav: readonly NavItem[] = [
  { id: 'now',      path: '/',         label: 'nav.now',      icon: 'power',    section: 'overview', order: 10, primary: true  },
  { id: 'servers',  path: '/servers',  label: 'nav.servers',  icon: 'server',   section: 'network',  order: 20, primary: true  },
  { id: 'traffic',  path: '/traffic',  label: 'nav.traffic',  icon: 'traffic',  section: 'network',  order: 30, primary: true  },
  { id: 'mesh',     path: '/mesh',     label: 'nav.mesh',     icon: 'mesh',     section: 'network',  order: 40, primary: true  },
  { id: 'activity', path: '/activity', label: 'nav.activity', icon: 'activity', section: 'overview', order: 50, primary: true  },
  { id: 'settings', path: '/settings', label: 'nav.settings', icon: 'settings', section: 'system',   order: 60, primary: false },
]

export function primaryNav(): NavItem[] {
  return nav.filter(n => n.primary)
}

export function navById(id: string): NavItem | undefined {
  return nav.find(n => n.id === id)
}

export function navBySection(section: NavSection): NavItem[] {
  return nav.filter(n => n.section === section).sort((a, b) => a.order - b.order)
}
```

- [ ] **Step 1.9.4: Update icons.ts to include new icon names**

Open `gui/web/src/app/icons.ts`, add `power`, `traffic`, `activity`, `mesh` to the `IconName` union and icon map (reuse existing SVG paths if available; use sensible defaults otherwise). Run:

```bash
grep -n "IconName" gui/web/src/app/icons.ts
```

Add:
```ts
// in IconName union
| 'power' | 'traffic' | 'activity' | 'mesh'

// in the iconPaths map, add (example — use actual icon SVGs matching design system):
power:    'M12 2v10m0 0a7 7 0 1 0 7 7',
traffic:  'M4 6h16M4 12h10M4 18h16',
activity: 'M3 12h3l3-9 6 18 3-9h3',
mesh:     'M12 2l8 4v8l-8 4-8-4V6zm0 8v8m-8-4l8 4 8-4',
```

- [ ] **Step 1.9.5: Run — pass**

```bash
cd gui/web && npx vitest run src/app/nav.test.ts
```

- [ ] **Step 1.9.6: Commit**

```bash
git add gui/web/src/app/nav.ts gui/web/src/app/nav.test.ts gui/web/src/app/icons.ts
git commit -m "feat(gui): nav.ts — single 6-item source of truth + new icon keys"
```

---

### Task 1.10: i18n keys for new nav

**Files:**
- Modify: `gui/web/src/lib/i18n/locales/en.ts`, `gui/web/src/lib/i18n/locales/zh.ts` (and any others)

- [ ] **Step 1.10.1: Inspect existing locale files**

```bash
ls gui/web/src/lib/i18n/locales/
```

- [ ] **Step 1.10.2: Add keys**

In each locale file, add to `nav` namespace:

**en.ts**
```ts
nav: {
  ...existing,
  now: 'Now',
  servers: 'Servers',
  traffic: 'Traffic',
  mesh: 'Mesh',
  activity: 'Activity',
  settings: 'Settings',
  section: {
    ...existing.section,
    // reuse existing overview / network / system if already present
  },
},
```

**zh.ts**
```ts
nav: {
  ...existing,
  now: '此刻',
  servers: '服务器',
  traffic: '流量',
  mesh: '网格',
  activity: '活动',
  settings: '设置',
},
```

Keep old keys (`dashboard`, `routing`, `logs`, `subscriptions`, `groups`) intact — still used by legacy routes.

- [ ] **Step 1.10.3: Run check**

```bash
cd gui/web && npm run check
```

Expected: no TS errors from missing i18n keys.

- [ ] **Step 1.10.4: Commit**

```bash
git add gui/web/src/lib/i18n/locales/
git commit -m "i18n: add nav keys for 6-item IA (now/servers/traffic/mesh/activity/settings)"
```

---

### Task 1.11: TopBar component

**Files:**
- Create: `gui/web/src/app/TopBar.svelte`
- Test: `gui/web/src/app/TopBar.test.ts`

- [ ] **Step 1.11.1: Write failing test**

File: `gui/web/src/app/TopBar.test.ts`

```ts
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import TopBar from './TopBar.svelte'

describe('TopBar', () => {
  it('renders title passed via prop', () => {
    render(TopBar, { props: { title: 'Servers' } })
    expect(screen.getByText('Servers')).toBeTruthy()
  })
  it('renders settings gear with aria-label', () => {
    const { container } = render(TopBar, { props: { title: 'X' } })
    const gear = container.querySelector('[aria-label="Settings"]')
    expect(gear).toBeTruthy()
  })
})
```

- [ ] **Step 1.11.2: Run — fail**

- [ ] **Step 1.11.3: Implement**

File: `gui/web/src/app/TopBar.svelte`

```svelte
<script lang="ts">
  import { Icon } from '@/ui'
  import { navigate } from '@/lib/router'

  interface Props {
    title: string
    showMenuButton?: boolean
    onMenuClick?: () => void
  }
  let { title, showMenuButton = false, onMenuClick }: Props = $props()
</script>

<header class="topbar">
  {#if showMenuButton}
    <button class="icon-btn" aria-label="Menu" onclick={onMenuClick}>
      <Icon name="menu" size={18} />
    </button>
  {:else}
    <div class="brand">
      <div class="logo">S</div>
    </div>
  {/if}
  <h1 class="title">{title}</h1>
  <div class="spacer"></div>
  <button
    class="icon-btn"
    aria-label="Settings"
    onclick={() => navigate('/settings')}
  >
    <Icon name="settings" size={18} />
  </button>
</header>

<style>
  .topbar {
    display: flex; align-items: center; gap: var(--shuttle-space-3);
    height: 48px;
    padding: 0 var(--shuttle-space-3);
    padding-top: env(safe-area-inset-top);
    border-bottom: 1px solid var(--shuttle-border);
    background: var(--shuttle-bg-base);
    position: sticky; top: 0; z-index: 10;
  }
  .brand { display: flex; }
  .logo {
    width: 22px; height: 22px;
    background: var(--shuttle-accent); color: var(--shuttle-accent-fg);
    border-radius: var(--shuttle-radius-sm);
    display: flex; align-items: center; justify-content: center;
    font-weight: 600; font-size: 11px;
  }
  .title {
    font-size: var(--shuttle-text-base);
    font-weight: var(--shuttle-weight-semibold);
    margin: 0;
    color: var(--shuttle-fg-primary);
  }
  .spacer { flex: 1; }
  .icon-btn {
    background: transparent; border: 0; cursor: pointer;
    padding: var(--shuttle-space-1);
    color: var(--shuttle-fg-secondary);
    border-radius: var(--shuttle-radius-sm);
    min-height: 44px; min-width: 44px;
    display: flex; align-items: center; justify-content: center;
  }
  .icon-btn:hover { color: var(--shuttle-fg-primary); }
</style>
```

Also add `menu` icon key to `icons.ts` (hamburger: `M3 6h18M3 12h18M3 18h18`).

- [ ] **Step 1.11.4: Run — pass**

- [ ] **Step 1.11.5: Commit**

```bash
git add gui/web/src/app/TopBar.svelte gui/web/src/app/TopBar.test.ts gui/web/src/app/icons.ts
git commit -m "feat(gui): TopBar component — title + settings gear"
```

---

### Task 1.12: BottomTabs component

**Files:**
- Create: `gui/web/src/app/BottomTabs.svelte`
- Test: `gui/web/src/app/BottomTabs.test.ts`

- [ ] **Step 1.12.1: Write failing test**

File: `gui/web/src/app/BottomTabs.test.ts`

```ts
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import BottomTabs from './BottomTabs.svelte'
import { primaryNav } from './nav'

describe('BottomTabs', () => {
  it('renders one tab per primary nav item', () => {
    const { container } = render(BottomTabs)
    const tabs = container.querySelectorAll('[role="tab"]')
    expect(tabs.length).toBe(primaryNav().length)
  })
  it('has aria-label on the nav container', () => {
    const { container } = render(BottomTabs)
    const nav = container.querySelector('[aria-label="Primary navigation"]')
    expect(nav).toBeTruthy()
  })
})
```

- [ ] **Step 1.12.2: Run — fail**

- [ ] **Step 1.12.3: Implement**

File: `gui/web/src/app/BottomTabs.svelte`

```svelte
<script lang="ts">
  import { Link, useRoute } from '@/lib/router'
  import { Icon } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { primaryNav } from './nav'

  const route = useRoute()
  const items = primaryNav()

  function isActive(path: string): boolean {
    if (path === '/') return route.path === '/'
    return route.path === path || route.path.startsWith(path + '/')
  }
</script>

<nav class="tabs" aria-label="Primary navigation" role="tablist">
  {#each items as item}
    <Link
      to={item.path}
      class={'tab ' + (isActive(item.path) ? 'active' : '')}
      role="tab"
      aria-selected={isActive(item.path)}
    >
      <span class="icon"><Icon name={item.icon} size={20} /></span>
      <span class="label">{t(item.label)}</span>
    </Link>
  {/each}
</nav>

<style>
  .tabs {
    display: flex;
    height: 56px;
    padding-bottom: env(safe-area-inset-bottom);
    border-top: 1px solid var(--shuttle-border);
    background: var(--shuttle-bg-base);
    position: sticky; bottom: 0;
  }
  :global(a.tab) {
    flex: 1;
    display: flex; flex-direction: column;
    align-items: center; justify-content: center;
    gap: 2px;
    text-decoration: none;
    color: var(--shuttle-fg-muted);
    font-size: 10px;
    min-height: 44px;
    transition: color var(--shuttle-duration);
  }
  :global(a.tab.active) { color: var(--shuttle-accent); }
  .icon { width: 20px; height: 20px; display: inline-flex; }
  .label { font-weight: var(--shuttle-weight-medium); letter-spacing: 0.02em; }
</style>
```

- [ ] **Step 1.12.4: Run — pass**

- [ ] **Step 1.12.5: Commit**

```bash
git add gui/web/src/app/BottomTabs.svelte gui/web/src/app/BottomTabs.test.ts
git commit -m "feat(gui): BottomTabs — phone nav from nav.ts"
```

---

### Task 1.13: Rail component

**Files:**
- Create: `gui/web/src/app/Rail.svelte`
- Test: `gui/web/src/app/Rail.test.ts`

- [ ] **Step 1.13.1: Write failing test**

File: `gui/web/src/app/Rail.test.ts`

```ts
import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/svelte'
import Rail from './Rail.svelte'
import { nav } from './nav'

describe('Rail', () => {
  it('renders every nav item (including non-primary)', () => {
    const { container } = render(Rail)
    const items = container.querySelectorAll('.rail-item')
    expect(items.length).toBe(nav.length)
  })
})
```

- [ ] **Step 1.13.2: Run — fail**

- [ ] **Step 1.13.3: Implement**

File: `gui/web/src/app/Rail.svelte`

```svelte
<script lang="ts">
  import { Link, useRoute } from '@/lib/router'
  import { Icon } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { nav } from './nav'

  const route = useRoute()
  function isActive(path: string): boolean {
    if (path === '/') return route.path === '/'
    return route.path === path || route.path.startsWith(path + '/')
  }
</script>

<aside class="rail" aria-label="Primary navigation">
  {#each nav as item}
    <Link
      to={item.path}
      class={'rail-item ' + (isActive(item.path) ? 'active' : '')}
      aria-label={t(item.label)}
    >
      <span class="icon"><Icon name={item.icon} size={20} /></span>
      <span class="mini-label">{t(item.label)}</span>
    </Link>
  {/each}
</aside>

<style>
  .rail {
    width: 64px; min-width: 64px;
    display: flex; flex-direction: column; gap: 2px;
    padding: var(--shuttle-space-3) var(--shuttle-space-1);
    border-right: 1px solid var(--shuttle-border);
    background: var(--shuttle-bg-base);
  }
  :global(a.rail-item) {
    display: flex; flex-direction: column; align-items: center; gap: 2px;
    padding: var(--shuttle-space-2) var(--shuttle-space-1);
    border-radius: var(--shuttle-radius-sm);
    text-decoration: none;
    color: var(--shuttle-fg-muted);
    font-size: 9px;
    min-height: 44px;
  }
  :global(a.rail-item.active),
  :global(a.rail-item:hover) {
    color: var(--shuttle-fg-primary);
    background: var(--shuttle-bg-subtle);
  }
  .icon { width: 20px; height: 20px; display: inline-flex; }
  .mini-label { letter-spacing: 0.02em; }
</style>
```

- [ ] **Step 1.13.4: Run — pass**

- [ ] **Step 1.13.5: Commit**

```bash
git add gui/web/src/app/Rail.svelte gui/web/src/app/Rail.test.ts
git commit -m "feat(gui): Rail — tablet nav, 64px vertical icon rail"
```

---

### Task 1.14: AppShell component (not yet wired)

**Files:**
- Create: `gui/web/src/app/AppShell.svelte`

- [ ] **Step 1.14.1: Implement AppShell**

File: `gui/web/src/app/AppShell.svelte`

```svelte
<script lang="ts">
  import { Router } from '@/lib/router'
  import { viewport } from '@/lib/viewport.svelte'
  import Sidebar from './Sidebar.svelte'
  import Rail from './Rail.svelte'
  import BottomTabs from './BottomTabs.svelte'
  import TopBar from './TopBar.svelte'
  import { nav, navById } from './nav'
  import { useRoute } from '@/lib/router'
  import { t } from '@/lib/i18n/index'
  import { routes } from './routes'

  const route = useRoute()

  const currentTitle = $derived.by(() => {
    const item = nav.find(n => n.path === route.path || route.path.startsWith(n.path + '/'))
    return item ? t(item.label) : ''
  })
</script>

<svelte:body data-touch={viewport.isTouch ? '1' : '0'} />

<div class="shell" data-form={viewport.form}>
  {#if viewport.isMobile}
    <TopBar title={currentTitle} />
    <main>
      <Router {routes} />
    </main>
    <BottomTabs />
  {:else if viewport.isTablet}
    <Rail />
    <main>
      <Router {routes} />
    </main>
  {:else}
    <Sidebar {routes} />
    <main>
      <Router {routes} />
    </main>
  {/if}
</div>

<style>
  .shell {
    display: flex;
    min-height: 100vh;
    background: var(--shuttle-bg-base);
  }
  .shell[data-form="xs"],
  .shell[data-form="sm"] {
    flex-direction: column;
  }
  main {
    flex: 1; min-width: 0;
    overflow-y: auto;
    overscroll-behavior: contain;
    padding: var(--shuttle-space-5) var(--shuttle-space-6);
  }
  .shell[data-form="xs"] main,
  .shell[data-form="sm"] main {
    padding: var(--shuttle-space-3);
  }
  :global([data-touch="1"] button),
  :global([data-touch="1"] a) {
    min-height: 44px;
  }
  @media (hover: none) {
    :global(*:hover) { /* touch devices shouldn't trigger hover states */ }
  }
</style>
```

- [ ] **Step 1.14.2: Build check**

```bash
cd gui/web && npm run check
```

Expected: no TS errors. AppShell is not yet imported by `App.svelte`, so it's dead code at this point — compiles but isn't bundled.

- [ ] **Step 1.14.3: Commit**

```bash
git add gui/web/src/app/AppShell.svelte
git commit -m "feat(gui): AppShell — viewport-aware shell (not yet wired in App.svelte)"
```

---

### Task 1.15: Migrate Dashboard ConnectionHero to `platform.engineStart/Stop`

**Files:**
- Modify: `gui/web/src/features/dashboard/ConnectionHero.svelte`

- [ ] **Step 1.15.1: Add platform call path**

In `ConnectionHero.svelte`, replace `connect()` / `disconnect()` imports from `@/lib/api/endpoints` with `platform` from `@/lib/platform`. Update `toggle()`:

```svelte
<script lang="ts">
  // ...existing imports
  import { platform } from '@/lib/platform'

  async function toggle() {
    busy = true
    try {
      if (connected) await platform.engineStop()
      else await platform.engineStart()
      invalidate('dashboard.status')
    } catch (e) {
      toasts.error((e as Error).message)
    } finally {
      busy = false
    }
  }
</script>
```

Remove the direct `import { connect, disconnect } from '@/lib/api/endpoints'` line.

- [ ] **Step 1.15.2: Verify behavior unchanged (web runtime)**

Unit tests existing in dashboard feature should still pass:

```bash
cd gui/web && npx vitest run src/features/dashboard/
```

- [ ] **Step 1.15.3: Playwright smoke**

```bash
cd gui/web && npx playwright test tests/shell.spec.ts
```

Expected: pass.

- [ ] **Step 1.15.4: Commit**

```bash
git add gui/web/src/features/dashboard/ConnectionHero.svelte
git commit -m "refactor(gui): Dashboard uses platform.engineStart/Stop (REST-equivalent)"
```

---

### Task 1.16: Phase 1 PR gate — full test + build

- [ ] **Step 1.16.1: Run all unit tests**

```bash
cd gui/web && npm run test
```

Expected: all pass.

- [ ] **Step 1.16.2: Run svelte-check**

```bash
cd gui/web && npm run check
```

Expected: no errors.

- [ ] **Step 1.16.3: Run Playwright**

```bash
cd gui/web && npm run test:e2e
```

Expected: all desktop-viewport tests pass (viewport matrix not yet in config).

- [ ] **Step 1.16.4: Build**

```bash
cd gui/web && npm run build
```

Expected: success. Measure bundle size; expect < +5KB gzip delta vs baseline (new code is mostly dead-code-eliminated since AppShell isn't imported).

- [ ] **Step 1.16.5: Phase 1 merge-ready**

No extra commit needed. Phase 1 done.

---

## Phase 2 · IA & Routing Layer

**Goal:** New 6 routes live; legacy routes redirect with one-time toast; `AppShell` replaces `Shell` behind feature flag `VITE_USE_LEGACY_SHELL`; each new page shows a temporary "coming soon" card (actual implementations in Phase 3).

### Task 2.1: Route migration module

**Files:**
- Create: `gui/web/src/app/route-migration.ts`
- Test: `gui/web/src/app/route-migration.test.ts`

- [ ] **Step 2.1.1: Write failing test**

File: `gui/web/src/app/route-migration.test.ts`

```ts
import { describe, it, expect } from 'vitest'
import { resolveLegacyRoute } from './route-migration'

describe('resolveLegacyRoute', () => {
  it('returns null for new routes', () => {
    expect(resolveLegacyRoute('/', {})).toBeNull()
    expect(resolveLegacyRoute('/servers', {})).toBeNull()
    expect(resolveLegacyRoute('/traffic', {})).toBeNull()
  })
  it('maps /dashboard → /', () => {
    expect(resolveLegacyRoute('/dashboard', {})).toEqual({ path: '/', query: {} })
  })
  it('maps /subscriptions → /servers?source=subscriptions', () => {
    expect(resolveLegacyRoute('/subscriptions', {})).toEqual({
      path: '/servers', query: { source: 'subscriptions' },
    })
  })
  it('maps /subscriptions/:id → /servers?source=subscription:<id>', () => {
    expect(resolveLegacyRoute('/subscriptions/abc', {})).toEqual({
      path: '/servers', query: { source: 'subscription:abc' },
    })
  })
  it('maps /groups/:id → /servers?group=<id>', () => {
    expect(resolveLegacyRoute('/groups/pro', {})).toEqual({
      path: '/servers', query: { group: 'pro' },
    })
  })
  it('maps /routing → /traffic', () => {
    expect(resolveLegacyRoute('/routing', {})).toEqual({ path: '/traffic', query: {} })
  })
  it('maps /logs → /activity?tab=logs', () => {
    expect(resolveLegacyRoute('/logs', {})).toEqual({
      path: '/activity', query: { tab: 'logs' },
    })
  })
  it('preserves extra query params', () => {
    expect(resolveLegacyRoute('/subscriptions', { foo: 'bar' })).toEqual({
      path: '/servers', query: { source: 'subscriptions', foo: 'bar' },
    })
  })
})
```

- [ ] **Step 2.1.2: Run — fail**

- [ ] **Step 2.1.3: Implement**

File: `gui/web/src/app/route-migration.ts`

```ts
export interface Resolved {
  path: string
  query: Record<string, string>
}

export function resolveLegacyRoute(
  path: string,
  query: Record<string, string>,
): Resolved | null {
  if (path === '/dashboard')   return { path: '/',         query: { ...query } }
  if (path === '/routing')     return { path: '/traffic',  query: { ...query } }
  if (path === '/logs')        return { path: '/activity', query: { tab: 'logs', ...query } }
  if (path === '/subscriptions') {
    return { path: '/servers', query: { source: 'subscriptions', ...query } }
  }
  if (path.startsWith('/subscriptions/')) {
    const id = path.slice('/subscriptions/'.length)
    return { path: '/servers', query: { source: `subscription:${id}`, ...query } }
  }
  if (path === '/groups') {
    return { path: '/servers', query: { view: 'groups', ...query } }
  }
  if (path.startsWith('/groups/')) {
    const id = path.slice('/groups/'.length)
    return { path: '/servers', query: { group: id, ...query } }
  }
  return null
}

const TOAST_KEY = 'shuttle-route-migration-seen'
export function hasSeenMigrationToast(): boolean {
  try { return localStorage.getItem(TOAST_KEY) === '1' } catch { return false }
}
export function markMigrationToastSeen(): void {
  try { localStorage.setItem(TOAST_KEY, '1') } catch {}
}
```

- [ ] **Step 2.1.4: Run — pass**

- [ ] **Step 2.1.5: Commit**

```bash
git add gui/web/src/app/route-migration.ts gui/web/src/app/route-migration.test.ts
git commit -m "feat(gui): route-migration — legacy URL → new URL resolver"
```

---

### Task 2.2: Wire migration into router

**Files:**
- Modify: `gui/web/src/lib/router/router.svelte.ts`

- [ ] **Step 2.2.1: Extend `update()`**

In `router.svelte.ts`, import `resolveLegacyRoute`, `hasSeenMigrationToast`, `markMigrationToastSeen` from `@/app/route-migration`. In `update()`, after parsing hash, if legacy resolution returns non-null, replace location.hash with the new route and fire a one-time toast:

```ts
import { resolveLegacyRoute, hasSeenMigrationToast, markMigrationToastSeen } from '@/app/route-migration'
import { toasts } from '@/lib/toaster.svelte'

function update() {
  const parsed = parseHash(location.hash)
  const legacy = resolveLegacyRoute(parsed.path, parsed.query)
  if (legacy) {
    const qs = new URLSearchParams(legacy.query).toString()
    const newHash = '#' + legacy.path + (qs ? '?' + qs : '')
    history.replaceState(null, '', newHash)
    if (!hasSeenMigrationToast()) {
      toasts.info(`This page moved — now at ${legacy.path}.`)
      markMigrationToastSeen()
    }
    state.path = legacy.path
    state.query = legacy.query
    return
  }
  state.path = parsed.path
  state.query = parsed.query
}
```

- [ ] **Step 2.2.2: Add test**

File: `gui/web/src/lib/router/router.test.ts` — add:

```ts
it('redirects legacy /routing to /traffic', () => {
  location.hash = '#/routing'
  __resetRoute()
  const r = useRoute()
  expect(r.path).toBe('/traffic')
})
```

- [ ] **Step 2.2.3: Run**

```bash
cd gui/web && npx vitest run src/lib/router/
```

Expected: new test + existing tests pass.

- [ ] **Step 2.2.4: Commit**

```bash
git add gui/web/src/lib/router/router.svelte.ts gui/web/src/lib/router/router.test.ts
git commit -m "feat(gui): router — legacy route redirect with one-time toast"
```

---

### Task 2.3: Stub pages for new routes

**Files:**
- Create: `gui/web/src/features/now/Now.svelte`
- Create: `gui/web/src/features/now/index.ts`
- Create: `gui/web/src/features/traffic/Traffic.svelte`  (note: later absorbs `features/routing/`)
- Create: `gui/web/src/features/traffic/index.ts`
- Create: `gui/web/src/features/activity/Activity.svelte`
- Create: `gui/web/src/features/activity/index.ts`

- [ ] **Step 2.3.1: Create Now stub**

File: `gui/web/src/features/now/Now.svelte`

```svelte
<div class="stub"><h1>Now</h1><p>Coming in Phase 3a.</p></div>
<style>.stub { padding: var(--shuttle-space-5); }</style>
```

File: `gui/web/src/features/now/index.ts`

```ts
import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/',
  component: lazy(() => import('./Now.svelte')),
  nav: { label: 'nav.now', icon: 'power', order: 10 },
}
```

- [ ] **Step 2.3.2: Create Traffic stub**

File: `gui/web/src/features/traffic/Traffic.svelte`

```svelte
<div class="stub"><h1>Traffic</h1><p>Coming in Phase 3c.</p></div>
<style>.stub { padding: var(--shuttle-space-5); }</style>
```

File: `gui/web/src/features/traffic/index.ts`

```ts
import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/traffic',
  component: lazy(() => import('./Traffic.svelte')),
  nav: { label: 'nav.traffic', icon: 'traffic', order: 30 },
}
```

- [ ] **Step 2.3.3: Create Activity stub**

File: `gui/web/src/features/activity/Activity.svelte`

```svelte
<div class="stub"><h1>Activity</h1><p>Coming in Phase 3d.</p></div>
<style>.stub { padding: var(--shuttle-space-5); }</style>
```

File: `gui/web/src/features/activity/index.ts`

```ts
import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/activity',
  component: lazy(() => import('./Activity.svelte')),
  nav: { label: 'nav.activity', icon: 'activity', order: 50 },
}
```

- [ ] **Step 2.3.4: Commit**

```bash
git add gui/web/src/features/now gui/web/src/features/traffic gui/web/src/features/activity
git commit -m "feat(gui): stub Now/Traffic/Activity pages (placeholder content)"
```

---

### Task 2.4: Update routes.ts with new routes

**Files:**
- Modify: `gui/web/src/app/routes.ts`

- [ ] **Step 2.4.1: Replace routes list**

File: `gui/web/src/app/routes.ts`

```ts
import type { Component } from 'svelte'
import * as now from '@/features/now'
import * as servers from '@/features/servers'
import * as traffic from '@/features/traffic'
import * as mesh from '@/features/mesh'
import * as activity from '@/features/activity'
import * as settings from '@/features/settings'
// Legacy (still registered so redirects land somewhere; kept until Phase 3 completes):
import * as dashboard from '@/features/dashboard'
import * as routing from '@/features/routing'
import * as logs from '@/features/logs'
import * as subscriptions from '@/features/subscriptions'
import * as groups from '@/features/groups'

export interface NavMeta {
  label: string
  icon: string
  order: number
  hidden?: boolean
}
export interface AppRoute {
  path: string
  component: () => Promise<Component>
  nav?: NavMeta
  children?: AppRoute[]
}

export const routes: AppRoute[] = [
  // New 6 primary routes
  now.route,
  servers.route,
  traffic.route,
  mesh.route,
  activity.route,
  settings.route,
  // Legacy routes still exist in `routes` so router doesn't 404 while resolveLegacyRoute() is redirecting.
  // They'll redirect before mounting. Removed at Phase 3 end.
  dashboard.route,
  routing.route,
  logs.route,
  subscriptions.route,
  groups.route,
  groups.detailRoute,
]
```

- [ ] **Step 2.4.2: Run build**

```bash
cd gui/web && npm run build
```

Expected: success (all imports resolve).

- [ ] **Step 2.4.3: Commit**

```bash
git add gui/web/src/app/routes.ts
git commit -m "feat(gui): routes — add new 6 + keep legacy for redirect landing"
```

---

### Task 2.5: Feature flag + AppShell wiring

**Files:**
- Modify: `gui/web/src/app/App.svelte`

- [ ] **Step 2.5.1: Introduce flag**

File: `gui/web/src/app/App.svelte`

```svelte
<script lang="ts">
  import Shell from './Shell.svelte'
  import AppShell from './AppShell.svelte'

  // Escape hatch for emergency rollback. Default: new shell.
  const useLegacy = import.meta.env.VITE_USE_LEGACY_SHELL === '1'
</script>

{#if useLegacy}
  <Shell />
{:else}
  <AppShell />
{/if}
```

- [ ] **Step 2.5.2: Verify dev server**

```bash
cd gui/web && npm run dev
```

Open browser, resize window across 720 / 1024 px — verify BottomTabs / Rail / Sidebar switch. Navigate to `/#/dashboard` — expect redirect to `/` with toast.

- [ ] **Step 2.5.3: Playwright smoke across viewports**

Modify `gui/web/playwright.config.ts` to add viewport projects (also needed by Phase 5, but useful now):

```ts
import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  timeout: 30000,
  retries: 1,
  webServer: { command: 'npm run dev', port: 5173, reuseExistingServer: true },
  use: { baseURL: 'http://localhost:5173', actionTimeout: 10000 },
  projects: [
    { name: 'desktop', use: { viewport: { width: 1440, height: 900 } } },
    { name: 'tablet',  use: { viewport: { width: 820,  height: 1180 } } },
    { name: 'phone',   use: { ...devices['iPhone 14'] } },
  ],
})
```

- [ ] **Step 2.5.4: Write new Playwright test**

File: `gui/web/tests/responsive.spec.ts`

```ts
import { test, expect } from '@playwright/test'

test.describe('responsive shell', () => {
  test('mobile viewport shows BottomTabs', async ({ page, isMobile }) => {
    test.skip(!isMobile, 'only on phone project')
    await page.goto('/')
    await expect(page.locator('[aria-label="Primary navigation"][role="tablist"]')).toBeVisible()
  })
  test('desktop shows Sidebar', async ({ page, viewport }) => {
    test.skip((viewport?.width ?? 0) < 1024, 'desktop only')
    await page.goto('/')
    await expect(page.locator('aside.sidebar').or(page.locator('aside').first())).toBeVisible()
  })
})
```

- [ ] **Step 2.5.5: Run Playwright**

```bash
cd gui/web && npx playwright test tests/responsive.spec.ts
```

Expected: phone project shows BottomTabs; desktop shows Sidebar. Tablet project is covered implicitly (neither assertion fires).

- [ ] **Step 2.5.6: Commit**

```bash
git add gui/web/src/app/App.svelte gui/web/playwright.config.ts gui/web/tests/responsive.spec.ts
git commit -m "feat(gui): wire AppShell behind VITE_USE_LEGACY_SHELL flag + 3-viewport Playwright"
```

---

### Task 2.6: Sidebar reads nav.ts

**Files:**
- Modify: `gui/web/src/app/Sidebar.svelte`

- [ ] **Step 2.6.1: Replace routes-filtering with nav.ts**

In `Sidebar.svelte`, replace the `sections = $derived.by(...)` block. Replace imports:

```svelte
<script lang="ts">
  import { Link, useRoute } from '@/lib/router'
  import { Icon, Button } from '@/ui'
  import { theme } from '@/lib/theme.svelte'
  import { t } from '@/lib/i18n/index'
  import { navBySection } from './nav'

  interface Props {
    collapsed?: boolean
    onToggleCollapsed?: () => void
  }
  let { collapsed = false, onToggleCollapsed }: Props = $props()
  const route = useRoute()

  const sections = {
    overview: navBySection('overview'),
    network:  navBySection('network'),
    system:   navBySection('system'),
  }

  function isActive(path: string): boolean {
    if (path === '/') return route.path === '/'
    return route.path === path || route.path.startsWith(path + '/')
  }
</script>
```

Replace the nav rendering loops to iterate `sections.overview`, `sections.network`, `sections.system` using `item.label` / `item.icon` / `item.path`.

(Existing `routes` prop can be removed from Sidebar's Props since it no longer uses it.)

Update `AppShell.svelte`: remove the `{routes}` prop from `<Sidebar ... />` call.

- [ ] **Step 2.6.2: Run**

```bash
cd gui/web && npm run check && npx vitest run && npx playwright test
```

Expected: all pass.

- [ ] **Step 2.6.3: Commit**

```bash
git add gui/web/src/app/Sidebar.svelte gui/web/src/app/AppShell.svelte
git commit -m "refactor(gui): Sidebar reads from nav.ts instead of routes"
```

---

## Phase 3 · Page Implementation

Each sub-phase ships independently. Order by daily-use priority: 3a (Now) → 3b (Servers) → 3c (Traffic) → 3d (Activity) → 3e (Mesh) → 3f (Settings).

### Phase 3a · Now Page

#### Task 3a.1: PowerButton component

**Files:**
- Create: `gui/web/src/features/now/PowerButton.svelte`
- Test: `gui/web/src/features/now/PowerButton.test.ts`

- [ ] **Step 3a.1.1: Write failing test**

File: `gui/web/src/features/now/PowerButton.test.ts`

```ts
import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import PowerButton from './PowerButton.svelte'

describe('PowerButton', () => {
  it('renders disconnected state by default', () => {
    const { container } = render(PowerButton, { props: { state: 'disconnected' } })
    const btn = container.querySelector('[role="switch"]') as HTMLElement
    expect(btn.dataset.state).toBe('disconnected')
    expect(btn.getAttribute('aria-checked')).toBe('false')
  })

  it('renders connected state with aria-checked=true', () => {
    const { container } = render(PowerButton, { props: { state: 'connected' } })
    const btn = container.querySelector('[role="switch"]') as HTMLElement
    expect(btn.getAttribute('aria-checked')).toBe('true')
  })

  it('disables while connecting', () => {
    const { container } = render(PowerButton, { props: { state: 'connecting' } })
    const btn = container.querySelector('[role="switch"]') as HTMLButtonElement
    expect(btn.disabled).toBe(true)
  })

  it('invokes onToggle on click', async () => {
    const onToggle = vi.fn()
    const { container } = render(PowerButton, {
      props: { state: 'disconnected', onToggle },
    })
    const btn = container.querySelector('[role="switch"]') as HTMLButtonElement
    await fireEvent.click(btn)
    expect(onToggle).toHaveBeenCalled()
  })
})
```

- [ ] **Step 3a.1.2: Run — fail**

- [ ] **Step 3a.1.3: Implement PowerButton**

File: `gui/web/src/features/now/PowerButton.svelte`

```svelte
<script lang="ts">
  import { Icon } from '@/ui'
  type State = 'disconnected' | 'connecting' | 'connected'
  interface Props {
    state: State
    onToggle?: () => void
  }
  let { state, onToggle }: Props = $props()
  const isConnected = $derived(state === 'connected')
  const isConnecting = $derived(state === 'connecting')
  const label = $derived(isConnected ? 'Disconnect' : 'Connect')

  function handleClick() {
    if (isConnecting) return
    // haptic on touch devices (no-op on desktop)
    if (typeof navigator !== 'undefined' && 'vibrate' in navigator) navigator.vibrate?.(10)
    onToggle?.()
  }
</script>

<button
  class="power"
  data-state={state}
  role="switch"
  aria-checked={isConnected}
  aria-label={label}
  disabled={isConnecting}
  onclick={handleClick}
>
  {#if isConnecting}
    <span class="spinner" aria-hidden="true"></span>
  {/if}
  <Icon name="power" size={44} />
</button>

<style>
  .power {
    width: 120px; height: 120px;
    border-radius: 50%;
    border: 2px solid var(--shuttle-border);
    background: var(--shuttle-bg-subtle);
    color: var(--shuttle-fg-muted);
    display: flex; align-items: center; justify-content: center;
    cursor: pointer;
    position: relative;
    transition: background var(--shuttle-duration), border-color var(--shuttle-duration), color var(--shuttle-duration);
  }
  .power[data-state="connecting"] {
    border-color: var(--shuttle-warning, #d29922);
    color: var(--shuttle-warning, #d29922);
    background: color-mix(in srgb, var(--shuttle-warning, #d29922) 12%, transparent);
  }
  .power[data-state="connected"] {
    border-color: var(--shuttle-success, #3fb950);
    color: var(--shuttle-success, #3fb950);
    background: color-mix(in srgb, var(--shuttle-success, #3fb950) 12%, transparent);
  }
  .power:disabled { cursor: wait; }
  .power:focus-visible { outline: 2px solid var(--shuttle-accent); outline-offset: 3px; }

  .spinner {
    position: absolute; inset: -6px;
    border-radius: 50%;
    border: 2px dashed currentColor;
    opacity: 0.6;
    animation: spin 2s linear infinite;
  }
  @keyframes spin {
    from { transform: rotate(0deg); }
    to   { transform: rotate(360deg); }
  }
</style>
```

- [ ] **Step 3a.1.4: Run — pass**

- [ ] **Step 3a.1.5: Commit**

```bash
git add gui/web/src/features/now/PowerButton.svelte gui/web/src/features/now/PowerButton.test.ts
git commit -m "feat(gui): PowerButton — 3-state toggle with ARIA switch semantics"
```

---

#### Task 3a.2: ServerChip component

**Files:**
- Create: `gui/web/src/features/now/ServerChip.svelte`
- Test: `gui/web/src/features/now/ServerChip.test.ts`

- [ ] **Step 3a.2.1: Write test**

File: `gui/web/src/features/now/ServerChip.test.ts`

```ts
import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import ServerChip from './ServerChip.svelte'

describe('ServerChip', () => {
  it('renders server name + transport', () => {
    const { getByText } = render(ServerChip, {
      props: { serverName: 'my-server', transport: 'H3', state: 'connected' },
    })
    expect(getByText(/my-server/)).toBeTruthy()
    expect(getByText(/H3/)).toBeTruthy()
  })
  it('invokes onClick when tapped', async () => {
    const onClick = vi.fn()
    const { container } = render(ServerChip, {
      props: { serverName: 'x', transport: '', state: 'disconnected', onClick },
    })
    await fireEvent.click(container.querySelector('button')!)
    expect(onClick).toHaveBeenCalled()
  })
})
```

- [ ] **Step 3a.2.2: Run — fail**

- [ ] **Step 3a.2.3: Implement**

File: `gui/web/src/features/now/ServerChip.svelte`

```svelte
<script lang="ts">
  type State = 'disconnected' | 'connecting' | 'connected'
  interface Props {
    serverName: string
    transport: string
    state: State
    onClick?: () => void
  }
  let { serverName, transport, state, onClick }: Props = $props()
</script>

<button class="chip" data-state={state} onclick={() => onClick?.()} aria-label="Change server">
  <span class="dot"></span>
  <span class="text">
    {serverName}
    {#if transport}<span class="transport">· {transport}</span>{/if}
  </span>
  <span class="caret">▾</span>
</button>

<style>
  .chip {
    display: inline-flex; align-items: center; gap: var(--shuttle-space-2);
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border);
    border-radius: 999px;
    background: transparent;
    color: var(--shuttle-fg-secondary);
    font-size: var(--shuttle-text-sm);
    cursor: pointer;
    min-height: 44px;
  }
  .chip[data-state="connected"] .dot { background: var(--shuttle-success, #3fb950); }
  .chip[data-state="connecting"] .dot { background: var(--shuttle-warning, #d29922); }
  .dot {
    width: 6px; height: 6px; border-radius: 50%;
    background: var(--shuttle-fg-muted);
  }
  .transport { color: var(--shuttle-fg-muted); margin-left: var(--shuttle-space-1); }
  .caret { color: var(--shuttle-fg-muted); font-size: 10px; }
</style>
```

- [ ] **Step 3a.2.4: Run — pass**

- [ ] **Step 3a.2.5: Commit**

```bash
git add gui/web/src/features/now/ServerChip.svelte gui/web/src/features/now/ServerChip.test.ts
git commit -m "feat(gui): ServerChip — server + transport pill with tap-to-change"
```

---

#### Task 3a.3: Now page assembly

**Files:**
- Modify: `gui/web/src/features/now/Now.svelte`
- Test: `gui/web/src/features/now/Now.test.ts`

- [ ] **Step 3a.3.1: Write test**

File: `gui/web/src/features/now/Now.test.ts`

```ts
import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/svelte'
import Now from './Now.svelte'

vi.mock('@/lib/platform', () => ({
  platform: {
    name: 'web',
    engineStart: vi.fn(), engineStop: vi.fn(),
    engineStatus: vi.fn().mockResolvedValue({ connected: false, server: null }),
    onStatusChange: () => () => {},
  },
}))

describe('Now', () => {
  it('renders a power button', () => {
    const { container } = render(Now)
    expect(container.querySelector('[role="switch"]')).toBeTruthy()
  })
})
```

- [ ] **Step 3a.3.2: Run — fail (current Now.svelte is a stub)**

- [ ] **Step 3a.3.3: Implement**

File: `gui/web/src/features/now/Now.svelte`

```svelte
<script lang="ts">
  import PowerButton from './PowerButton.svelte'
  import ServerChip from './ServerChip.svelte'
  import { platform } from '@/lib/platform'
  import { toasts } from '@/lib/toaster.svelte'
  import { navigate } from '@/lib/router'
  import { useStatus, useSpeedStream } from '@/features/dashboard/resource.svelte'
  import { t } from '@/lib/i18n/index'
  import { AsyncBoundary } from '@/ui'
  import type { Status } from '@/lib/api/types'

  type PowerState = 'disconnected' | 'connecting' | 'connected'
  let busy = $state(false)

  const status = useStatus()
  const speed = useSpeedStream()

  function powerStateFor(s?: Status): PowerState {
    if (busy) return 'connecting'
    return s?.connected ? 'connected' : 'disconnected'
  }

  async function toggle(connected: boolean) {
    busy = true
    try {
      if (!connected) {
        if (platform.name === 'native') {
          const perm = await platform.requestVpnPermission()
          if (perm === 'denied') { toasts.error('VPN permission denied'); busy = false; return }
        }
        await platform.engineStart()
      } else {
        await platform.engineStop()
      }
    } catch (e) {
      toasts.error((e as Error).message)
    } finally {
      busy = false
    }
  }

  function formatUptime(s: number): string {
    if (s < 60) return `${s}s`
    const m = Math.floor(s / 60)
    if (m < 60) return `${m}m`
    const h = Math.floor(m / 60)
    return `${h}h ${m % 60}m`
  }

  function formatSpeed(bps: number): string {
    if (bps >= 1e6) return `${(bps / 1e6).toFixed(1)} MB/s`
    if (bps >= 1e3) return `${(bps / 1e3).toFixed(1)} KB/s`
    return `${bps} B/s`
  }
</script>

<AsyncBoundary resource={status}>
  {#snippet children(s: Status)}
    {@const ps = powerStateFor(s)}
    <div class="page">
      <div class="label" data-state={ps}>
        {#if ps === 'connected'}
          {t('now.connected')} · {formatUptime(s.uptime ?? 0)}
        {:else if ps === 'connecting'}
          {t('now.connecting')}
        {:else}
          {t('now.disconnected')}
        {/if}
      </div>

      <PowerButton state={ps} onToggle={() => toggle(!!s.connected)} />

      {#if ps === 'connected'}
        <div class="speeds">
          <span>↓ {formatSpeed(speed.data?.download ?? 0)}</span>
          <span>↑ {formatSpeed(speed.data?.upload ?? 0)}</span>
        </div>
      {/if}

      <ServerChip
        serverName={s.server?.name ?? s.server?.addr ?? '—'}
        transport={(s as any).transport ?? ''}
        state={ps}
        onClick={() => navigate('/servers')}
      />

      <button class="switch-link" onclick={() => navigate('/servers')}>
        {t('now.switchServer')} →
      </button>
    </div>
  {/snippet}
</AsyncBoundary>

<style>
  .page {
    display: flex; flex-direction: column; align-items: center;
    gap: var(--shuttle-space-4);
    padding: var(--shuttle-space-5);
    max-width: 420px; margin: 0 auto;
    min-height: 70vh; justify-content: center;
  }
  .label {
    font-size: var(--shuttle-text-xs);
    text-transform: uppercase; letter-spacing: 0.1em;
    color: var(--shuttle-fg-muted);
  }
  .label[data-state="connected"]  { color: var(--shuttle-success, #3fb950); }
  .label[data-state="connecting"] { color: var(--shuttle-warning, #d29922); }
  .speeds {
    display: flex; gap: var(--shuttle-space-5);
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-secondary);
    font-variant-numeric: tabular-nums;
  }
  .switch-link {
    background: transparent; border: 0; cursor: pointer;
    color: var(--shuttle-accent);
    font-size: var(--shuttle-text-sm);
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    min-height: 44px;
  }
</style>
```

- [ ] **Step 3a.3.4: Add i18n keys** (`en.ts` / `zh.ts`):

```ts
// en
now: {
  disconnected: 'Disconnected',
  connecting: 'Connecting…',
  connected: 'Connected',
  switchServer: 'Switch server',
}
// zh
now: {
  disconnected: '未连接',
  connecting: '连接中…',
  connected: '已连接',
  switchServer: '切换服务器',
}
```

- [ ] **Step 3a.3.5: Run — pass**

- [ ] **Step 3a.3.6: Playwright smoke**

File: `gui/web/tests/connect-flow.spec.ts`

```ts
import { test, expect } from '@playwright/test'

test('Now page shows power button', async ({ page }) => {
  await page.route('**/api/status', r => r.fulfill({ json: { connected: false, uptime: 0, server: null } }))
  await page.goto('/')
  const btn = page.getByRole('switch', { name: /Connect/ })
  await expect(btn).toBeVisible()
})
```

- [ ] **Step 3a.3.7: Commit**

```bash
git add gui/web/src/features/now/ gui/web/src/lib/i18n/locales/ gui/web/tests/connect-flow.spec.ts
git commit -m "feat(gui): Now page — PowerButton + ServerChip + speeds (Phase 3a)"
```

---

### Phase 3b · Servers Page (absorbs Subscriptions + Groups)

#### Task 3b.1: Source chip filter

**Files:**
- Modify: `gui/web/src/features/servers/Servers.svelte` (current file)
- New: `gui/web/src/features/servers/SourceFilter.svelte`

The existing Servers page has a server list. Add a source filter chip row above.

- [ ] **Step 3b.1.1: Write SourceFilter test**

File: `gui/web/src/features/servers/SourceFilter.test.ts`

```ts
import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import SourceFilter from './SourceFilter.svelte'

describe('SourceFilter', () => {
  it('renders All and given sources', () => {
    const { getByText } = render(SourceFilter, {
      props: {
        value: 'all',
        sources: [{ id: 'manual', label: 'Manual' }, { id: 'subscription:abc', label: 'sub:abc' }],
        groups: [],
        onChange: vi.fn(),
      },
    })
    expect(getByText('All')).toBeTruthy()
    expect(getByText('Manual')).toBeTruthy()
    expect(getByText('sub:abc')).toBeTruthy()
  })
  it('fires onChange when chip clicked', async () => {
    const onChange = vi.fn()
    const { getByText } = render(SourceFilter, {
      props: { value: 'all', sources: [{ id: 'manual', label: 'Manual' }], groups: [], onChange },
    })
    await fireEvent.click(getByText('Manual'))
    expect(onChange).toHaveBeenCalledWith('manual')
  })
})
```

- [ ] **Step 3b.1.2: Implement SourceFilter**

File: `gui/web/src/features/servers/SourceFilter.svelte`

```svelte
<script lang="ts">
  interface Source { id: string; label: string }
  interface Group { id: string; label: string }
  interface Props {
    value: string           // 'all' | 'manual' | `subscription:<id>` | `group:<id>`
    sources: Source[]
    groups: Group[]
    onChange: (v: string) => void
  }
  let { value, sources, groups, onChange }: Props = $props()
</script>

<div class="row" role="radiogroup" aria-label="Filter by source">
  <button class="chip" class:active={value === 'all'} onclick={() => onChange('all')}>All</button>
  {#each sources as s}
    <button class="chip" class:active={value === s.id} onclick={() => onChange(s.id)}>{s.label}</button>
  {/each}
  {#each groups as g}
    <button class="chip" class:active={value === `group:${g.id}`} onclick={() => onChange(`group:${g.id}`)}>
      {g.label}
    </button>
  {/each}
</div>

<style>
  .row {
    display: flex; gap: var(--shuttle-space-2);
    overflow-x: auto;
    padding-bottom: var(--shuttle-space-2);
    scrollbar-width: none;
  }
  .row::-webkit-scrollbar { display: none; }
  .chip {
    flex-shrink: 0;
    padding: var(--shuttle-space-1) var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border);
    border-radius: 999px;
    background: transparent;
    color: var(--shuttle-fg-secondary);
    font-size: var(--shuttle-text-sm);
    cursor: pointer;
    min-height: 36px;
  }
  .chip.active {
    background: var(--shuttle-accent);
    color: var(--shuttle-accent-fg);
    border-color: var(--shuttle-accent);
  }
</style>
```

- [ ] **Step 3b.1.3: Commit**

```bash
git add gui/web/src/features/servers/SourceFilter.svelte gui/web/src/features/servers/SourceFilter.test.ts
git commit -m "feat(gui): SourceFilter chip row for Servers page"
```

---

#### Task 3b.2: Subscription banner + refresh

**Files:**
- Create: `gui/web/src/features/servers/SubscriptionBanner.svelte`

- [ ] **Step 3b.2.1: Implement**

File: `gui/web/src/features/servers/SubscriptionBanner.svelte`

```svelte
<script lang="ts">
  import { Button } from '@/ui'
  import type { Subscription } from '@/lib/api/types'
  import { refreshSubscription, deleteSubscription } from '@/lib/api/endpoints'
  import { invalidate } from '@/lib/resource.svelte'
  import { toasts } from '@/lib/toaster.svelte'

  interface Props { sub: Subscription }
  let { sub }: Props = $props()
  let busy = $state(false)

  async function refresh() {
    busy = true
    try {
      await refreshSubscription(sub.id)
      invalidate('servers.list')
      toasts.success('Subscription refreshed')
    } catch (e) {
      toasts.error((e as Error).message)
    } finally { busy = false }
  }

  async function remove() {
    if (!confirm(`Delete subscription ${sub.name}?`)) return
    await deleteSubscription(sub.id)
    invalidate('servers.list')
  }
</script>

<div class="banner">
  <div class="meta">
    <div class="name">{sub.name}</div>
    <div class="url">{sub.url}</div>
    <div class="stats">
      {sub.server_count ?? 0} servers · last refresh
      {sub.last_refresh ? new Date(sub.last_refresh).toLocaleString() : 'never'}
    </div>
  </div>
  <div class="actions">
    <Button size="sm" loading={busy} onclick={refresh}>Refresh</Button>
    <Button size="sm" variant="ghost" onclick={remove}>Delete</Button>
  </div>
</div>

<style>
  .banner {
    display: flex; justify-content: space-between; align-items: flex-start;
    gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    margin-bottom: var(--shuttle-space-3);
  }
  .meta { min-width: 0; flex: 1; }
  .name { font-weight: var(--shuttle-weight-semibold); }
  .url { color: var(--shuttle-fg-muted); font-size: var(--shuttle-text-sm);
         white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .stats { color: var(--shuttle-fg-muted); font-size: var(--shuttle-text-xs); margin-top: var(--shuttle-space-1); }
  .actions { display: flex; gap: var(--shuttle-space-1); flex-shrink: 0; }
</style>
```

- [ ] **Step 3b.2.2: Commit**

```bash
git add gui/web/src/features/servers/SubscriptionBanner.svelte
git commit -m "feat(gui): SubscriptionBanner on Servers page"
```

---

#### Task 3b.3: Servers page wires filter + banners

**Files:**
- Modify: `gui/web/src/features/servers/Servers.svelte`

- [ ] **Step 3b.3.1: Read filter from route.query**

```svelte
<script lang="ts">
  import { useRoute, navigate } from '@/lib/router'
  import SourceFilter from './SourceFilter.svelte'
  import SubscriptionBanner from './SubscriptionBanner.svelte'
  import { getSubscriptions } from '@/lib/api/endpoints'
  // ... existing imports
  import { createResource } from '@/lib/resource.svelte'

  const route = useRoute()
  const subsResource = createResource('subscriptions.list', getSubscriptions)

  const currentFilter = $derived(route.query.source ?? (route.query.group ? `group:${route.query.group}` : 'all'))
  const activeSubId = $derived(
    currentFilter.startsWith('subscription:') ? currentFilter.slice('subscription:'.length) : null
  )
  const activeSub = $derived(
    activeSubId ? (subsResource.data?.find(s => s.id === activeSubId) ?? null) : null
  )

  function setFilter(v: string) {
    if (v === 'all')                            navigate('/servers')
    else if (v.startsWith('group:'))            navigate(`/servers?group=${v.slice('group:'.length)}`)
    else if (v === 'manual' || v === 'subscriptions' || v.startsWith('subscription:'))
                                                 navigate(`/servers?source=${v}`)
  }

  // ... existing server list logic; filter serverList by currentFilter
</script>
```

- [ ] **Step 3b.3.2: Apply filter to list**

In the server list rendering, filter servers:

```ts
const filteredServers = $derived.by(() => {
  const servers = serversResource.data ?? []
  if (currentFilter === 'all') return servers
  if (currentFilter === 'manual') return servers.filter(s => !s.subscription_id && !s.group_id)
  if (currentFilter === 'subscriptions') return servers.filter(s => !!s.subscription_id)
  if (currentFilter.startsWith('subscription:')) {
    const id = currentFilter.slice('subscription:'.length)
    return servers.filter(s => s.subscription_id === id)
  }
  if (currentFilter.startsWith('group:')) {
    const id = currentFilter.slice('group:'.length)
    return servers.filter(s => s.group_id === id)
  }
  return servers
})
```

(Where `subscription_id` / `group_id` come from existing Server type; add them if missing — see existing `features/subscriptions` code for truth of data shape.)

- [ ] **Step 3b.3.3: Render filter + banner**

```svelte
<SourceFilter
  value={currentFilter}
  sources={[
    { id: 'manual', label: 'Manual' },
    { id: 'subscriptions', label: 'Subscriptions' },
    ...(subsResource.data ?? []).map(s => ({ id: `subscription:${s.id}`, label: s.name })),
  ]}
  groups={[]}
  onChange={setFilter}
/>

{#if activeSub}
  <SubscriptionBanner sub={activeSub} />
{/if}

<!-- Existing server list, using filteredServers instead of raw list -->
```

- [ ] **Step 3b.3.4: Run smoke tests**

```bash
cd gui/web && npm run test && npx playwright test tests/subscriptions.spec.ts
```

Expected: existing subscriptions test passes via its legacy `/subscriptions` route redirect to `/servers?source=subscriptions`.

- [ ] **Step 3b.3.5: Commit**

```bash
git add gui/web/src/features/servers/Servers.svelte
git commit -m "feat(gui): Servers page — source filter + subscription banner"
```

---

#### Task 3b.4: Add-server sheet with method picker

**Files:**
- Create: `gui/web/src/features/servers/AddSheet.svelte`

- [ ] **Step 3b.4.1: Implement**

File: `gui/web/src/features/servers/AddSheet.svelte`

```svelte
<script lang="ts">
  import { Dialog, Button, Input } from '@/ui'
  import { platform } from '@/lib/platform'
  import { addServer, addSubscription, importConfig } from '@/lib/api/endpoints'
  import { invalidate } from '@/lib/resource.svelte'
  import { toasts } from '@/lib/toaster.svelte'

  interface Props { open: boolean; onClose: () => void }
  let { open, onClose }: Props = $props()

  type Method = 'manual' | 'paste' | 'subscribe' | 'qr'
  let method = $state<Method>('manual')
  let addr = $state(''); let password = $state(''); let name = $state('')
  let pasteData = $state(''); let subUrl = $state(''); let busy = $state(false)

  const canScan = $derived(platform.name === 'native')

  async function scan() {
    const r = await platform.scanQRCode()
    if (r === 'unsupported') { toasts.error('QR scan unavailable on this device'); return }
    pasteData = r; method = 'paste'
  }

  async function submit() {
    busy = true
    try {
      if (method === 'manual') {
        await addServer({ addr: addr.trim(), password: password.trim() || undefined, name: name.trim() || addr })
      } else if (method === 'paste') {
        await importConfig(pasteData)
      } else if (method === 'subscribe') {
        await addSubscription('', subUrl.trim())
      }
      invalidate('servers.list'); invalidate('subscriptions.list')
      toasts.success('Added')
      onClose()
    } catch (e) {
      toasts.error((e as Error).message)
    } finally { busy = false }
  }
</script>

<Dialog {open} onOpenChange={(v) => !v && onClose()}>
  <div class="sheet">
    <h2>Add server</h2>
    <div class="tabs">
      <button class:active={method === 'manual'} onclick={() => method = 'manual'}>Manual</button>
      <button class:active={method === 'paste'} onclick={() => method = 'paste'}>Paste</button>
      <button class:active={method === 'subscribe'} onclick={() => method = 'subscribe'}>Subscribe</button>
      {#if canScan}<button onclick={scan}>Scan QR</button>{/if}
    </div>

    {#if method === 'manual'}
      <Input bind:value={addr} placeholder="host:port" />
      <Input bind:value={password} type="password" placeholder="password (optional)" />
      <Input bind:value={name} placeholder="name (optional)" />
    {:else if method === 'paste'}
      <textarea bind:value={pasteData} placeholder="Paste shuttle:// URI or config JSON" />
    {:else if method === 'subscribe'}
      <Input bind:value={subUrl} placeholder="https://…subscription URL" />
    {/if}

    <div class="actions">
      <Button variant="ghost" onclick={onClose}>Cancel</Button>
      <Button variant="primary" loading={busy} onclick={submit}>Add</Button>
    </div>
  </div>
</Dialog>

<style>
  .sheet { display: flex; flex-direction: column; gap: var(--shuttle-space-3); padding: var(--shuttle-space-4); min-width: 320px; }
  .tabs { display: flex; gap: var(--shuttle-space-2); flex-wrap: wrap; }
  .tabs button {
    background: transparent; border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
    padding: var(--shuttle-space-1) var(--shuttle-space-3);
    color: var(--shuttle-fg-secondary); cursor: pointer; min-height: 36px;
  }
  .tabs button.active { background: var(--shuttle-accent); color: var(--shuttle-accent-fg); border-color: var(--shuttle-accent); }
  textarea {
    width: 100%; min-height: 120px;
    background: var(--shuttle-bg-subtle);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
    padding: var(--shuttle-space-2); color: var(--shuttle-fg-primary); font-family: var(--shuttle-font-mono);
  }
  .actions { display: flex; gap: var(--shuttle-space-2); justify-content: flex-end; margin-top: var(--shuttle-space-2); }
</style>
```

- [ ] **Step 3b.4.2: Wire into Servers page**

Add `AddSheet` mount in `Servers.svelte`:

```svelte
<script>
  let addOpen = $state(false)
</script>

<!-- toolbar -->
<Button onclick={() => addOpen = true}>+ Add</Button>

<AddSheet open={addOpen} onClose={() => addOpen = false} />
```

- [ ] **Step 3b.4.3: Commit**

```bash
git add gui/web/src/features/servers/AddSheet.svelte gui/web/src/features/servers/Servers.svelte
git commit -m "feat(gui): AddSheet — manual / paste / subscribe / scan QR unified"
```

---

#### Task 3b.5: Remove subscriptions / groups routes from active nav

**Files:**
- Modify: `gui/web/src/app/routes.ts`

- [ ] **Step 3b.5.1: Hide legacy `nav` metadata**

In `routes.ts`, the legacy `subscriptions.route`, `groups.route`, `groups.detailRoute`, `dashboard.route`, `routing.route`, `logs.route` remain in the array but `nav` metadata shouldn't appear in the new Sidebar (which reads from `nav.ts` anyway). The legacy routes are still needed as landing targets for redirects (the redirect path is `/servers?source=...` — but that's the new servers route, so actually the legacy routes don't need to stay).

Safe to remove legacy routes from `routes.ts` NOW, since all redirects land on the new routes:

```ts
export const routes: AppRoute[] = [
  now.route,
  servers.route,
  traffic.route,
  mesh.route,
  activity.route,
  settings.route,
]
```

- [ ] **Step 3b.5.2: Delete legacy feature directories**

```bash
cd gui/web/src/features
git rm -r subscriptions groups
```

(Don't delete `dashboard` / `routing` / `logs` yet — Phase 3a's Now reuses `dashboard/resource.svelte.ts`, Phase 3c will absorb routing content, Phase 3d will absorb logs.)

- [ ] **Step 3b.5.3: Run tests and build**

```bash
cd gui/web && npm run check && npm run test && npx playwright test
```

Fix any import breakage (likely: files that imported `@/features/subscriptions` or `@/features/groups` — should now use endpoints directly).

- [ ] **Step 3b.5.4: Commit**

```bash
git add -A gui/web/src/app/routes.ts gui/web/src/features/
git commit -m "refactor(gui): remove /subscriptions and /groups routes (absorbed into /servers)"
```

---

### Phase 3c · Traffic Page

#### Task 3c.1: Rename routing/ → traffic/ content files

**Files:**
- Move via `git mv`: `gui/web/src/features/routing/` → `gui/web/src/features/traffic/`

- [ ] **Step 3c.1.1: Rename**

```bash
cd gui/web/src/features
# traffic/ currently holds only Traffic.svelte + index.ts (stubs).
# First, move the stub out of the way:
mv traffic/Traffic.svelte traffic/Traffic.stub.svelte.bak
mv traffic/index.ts traffic/index.ts.bak

# Move routing contents:
git mv routing/* traffic/
```

- [ ] **Step 3c.1.2: Update traffic/index.ts**

File: `gui/web/src/features/traffic/index.ts`

```ts
import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/traffic',
  component: lazy(() => import('./Routing.svelte')),  // or whatever the main component is named
  nav: { label: 'nav.traffic', icon: 'traffic', order: 30 },
}
```

- [ ] **Step 3c.1.3: Remove stub/backup**

```bash
rm gui/web/src/features/traffic/Traffic.stub.svelte.bak gui/web/src/features/traffic/index.ts.bak
rm -rf gui/web/src/features/routing  # now empty
```

- [ ] **Step 3c.1.4: Fix imports inside traffic/**

Any `from '@/features/routing/...'` imports inside the moved files must become `from './...'`. Run:

```bash
grep -r "features/routing" gui/web/src --include="*.ts" --include="*.svelte"
```

Update each hit.

- [ ] **Step 3c.1.5: Run check**

```bash
cd gui/web && npm run check
```

- [ ] **Step 3c.1.6: Commit**

```bash
git add -A gui/web/src/features/
git commit -m "refactor(gui): rename routing/ → traffic/ (feature directory move)"
```

---

#### Task 3c.2: Add Traffic tabs

**Files:**
- Modify: `gui/web/src/features/traffic/Routing.svelte` (consider renaming file to `Traffic.svelte`)

- [ ] **Step 3c.2.1: Wrap existing content in Tabs**

Use `@/ui/Tabs.svelte`:

```svelte
<script lang="ts">
  import { Tabs } from '@/ui'
  import { useRoute, navigate } from '@/lib/router'
  import RulesTab from './RulesTab.svelte'
  // import TemplatesTab from './TemplatesTab.svelte'
  // etc. Split the existing Routing.svelte content into per-tab components.
  const route = useRoute()
  const tab = $derived(route.query.tab ?? 'rules')

  function setTab(t: string) {
    const q = new URLSearchParams({ ...route.query, tab: t }).toString()
    navigate(`/traffic?${q}`, { replace: true })
  }
</script>

<Tabs
  items={[
    { id: 'rules',    label: 'Rules' },
    { id: 'templates', label: 'Templates' },
    { id: 'dns',      label: 'DNS' },
    { id: 'split',    label: 'Split Tunnel' },
    { id: 'pac',      label: 'PAC' },
  ]}
  value={tab}
  onChange={setTab}
/>

{#if tab === 'rules'}<RulesTab />
{:else if tab === 'templates'}<TemplatesTab />
{:else if tab === 'dns'}<DnsTab />
{:else if tab === 'split'}<SplitTunnelTab />
{:else if tab === 'pac'}<PacTab />
{/if}
```

Split the current Routing.svelte body into RulesTab / TemplatesTab / etc. (Depending on how routing/ was structured, some or all of these sub-components may already exist — in which case, reuse.)

- [ ] **Step 3c.2.2: Commit**

```bash
git add gui/web/src/features/traffic/
git commit -m "feat(gui): Traffic page — 5 tabs (Rules/Templates/DNS/SplitTunnel/PAC)"
```

---

#### Task 3c.3: Mobile rule detail form

**Files:**
- Create: `gui/web/src/features/traffic/RuleDetail.svelte`
- Create: `gui/web/src/features/traffic/RuleDetail.test.ts`

- [ ] **Step 3c.3.1: Write test**

```ts
import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import RuleDetail from './RuleDetail.svelte'

describe('RuleDetail', () => {
  it('saves rule on submit', async () => {
    const onSave = vi.fn()
    const { getByLabelText, getByText } = render(RuleDetail, {
      props: { initial: { type: 'domain', value: '', action: 'proxy' }, onSave, onCancel: vi.fn() },
    })
    await fireEvent.input(getByLabelText('Value'), { target: { value: 'example.com' } })
    await fireEvent.click(getByText('Save'))
    expect(onSave).toHaveBeenCalledWith({ type: 'domain', value: 'example.com', action: 'proxy' })
  })
})
```

- [ ] **Step 3c.3.2: Implement**

File: `gui/web/src/features/traffic/RuleDetail.svelte`

```svelte
<script lang="ts">
  import { Button, Select, Input, Combobox } from '@/ui'
  import { getGeositeCategories } from '@/lib/api/endpoints'

  type RuleType = 'domain' | 'ip' | 'process' | 'geosite'
  type Action = 'proxy' | 'direct' | 'block'
  interface Rule { type: RuleType; value: string; action: Action }

  interface Props {
    initial: Rule
    onSave: (r: Rule) => void
    onCancel: () => void
  }
  let { initial, onSave, onCancel }: Props = $props()

  let type = $state<RuleType>(initial.type)
  let value = $state(initial.value)
  let action = $state<Action>(initial.action)

  let geositeOptions = $state<string[]>([])
  $effect(() => {
    if (type === 'geosite' && geositeOptions.length === 0) {
      getGeositeCategories().then(list => { geositeOptions = list }).catch(() => {})
    }
  })
</script>

<form onsubmit={(e) => { e.preventDefault(); onSave({ type, value: value.trim(), action }) }}>
  <label>
    <span>Type</span>
    <Select bind:value={type} options={[
      { value: 'domain', label: 'Domain' },
      { value: 'ip', label: 'IP / CIDR' },
      { value: 'process', label: 'Process' },
      { value: 'geosite', label: 'GeoSite' },
    ]} />
  </label>

  <label>
    <span>Value</span>
    {#if type === 'geosite'}
      <Combobox bind:value options={geositeOptions.map(o => ({ value: o, label: o }))} placeholder="e.g. google" />
    {:else}
      <Input bind:value placeholder={type === 'ip' ? '1.2.3.4/24' : type === 'domain' ? 'example.com' : 'chrome.exe'} />
    {/if}
  </label>

  <label>
    <span>Action</span>
    <Select bind:value={action} options={[
      { value: 'proxy', label: 'Proxy' },
      { value: 'direct', label: 'Direct' },
      { value: 'block', label: 'Block' },
    ]} />
  </label>

  <div class="actions">
    <Button variant="ghost" onclick={onCancel}>Cancel</Button>
    <Button type="submit" variant="primary">Save</Button>
  </div>
</form>

<style>
  form {
    display: flex; flex-direction: column; gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-4);
    max-width: 480px;
  }
  label {
    display: flex; flex-direction: column; gap: var(--shuttle-space-1);
  }
  label > span {
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase; letter-spacing: 0.08em;
  }
  .actions { display: flex; gap: var(--shuttle-space-2); justify-content: flex-end; margin-top: var(--shuttle-space-2); }
</style>
```

- [ ] **Step 3c.3.3: Expose `/traffic/rules/:id` route**

Add child route handling in `traffic/index.ts` or inline in `Routing.svelte` via `useParams` — the detail pattern depends on the existing routing code style. If existing code has no child-route pattern, render `RuleDetail` conditionally when query `?rule=<id>` is set.

- [ ] **Step 3c.3.4: Commit**

```bash
git add gui/web/src/features/traffic/RuleDetail.svelte gui/web/src/features/traffic/RuleDetail.test.ts
git commit -m "feat(gui): Traffic RuleDetail form — type/value/action with geosite autocomplete"
```

---

### Phase 3d · Activity Page

#### Task 3d.1: Compose speed + transports + logs

**Files:**
- Modify: `gui/web/src/features/activity/Activity.svelte`
- Reuse: `gui/web/src/features/dashboard/SpeedSparkline.svelte`, `gui/web/src/features/dashboard/TransportBreakdown.svelte`, `gui/web/src/features/logs/LogList.svelte`

- [ ] **Step 3d.1.1: Wire composed page**

File: `gui/web/src/features/activity/Activity.svelte`

```svelte
<script lang="ts">
  import { useRoute, navigate } from '@/lib/router'
  import { Tabs } from '@/ui'
  import SpeedSparkline from '@/features/dashboard/SpeedSparkline.svelte'
  import TransportBreakdown from '@/features/dashboard/TransportBreakdown.svelte'
  import LogList from '@/features/logs/LogList.svelte'
  import { useTransportStats } from '@/features/dashboard/resource.svelte'
  import { viewport } from '@/lib/viewport.svelte'

  const route = useRoute()
  const tab = $derived(route.query.tab ?? 'all')
  const transports = useTransportStats()

  function setTab(t: string) {
    const q = new URLSearchParams({ ...route.query, tab: t }).toString()
    navigate(`/activity?${q}`, { replace: true })
  }
</script>

<div class="page">
  <Tabs
    items={[
      { id: 'all', label: 'Overview' },
      { id: 'logs', label: 'Logs' },
    ]}
    value={tab === 'logs' ? 'logs' : 'all'}
    onChange={setTab}
  />

  {#if tab !== 'logs'}
    <section class="sticky-top">
      <SpeedSparkline />
    </section>
    <section>
      <TransportBreakdown transports={transports.data ?? []} />
    </section>
  {/if}

  <section>
    <LogList />
  </section>
</div>

<style>
  .page { display: flex; flex-direction: column; gap: var(--shuttle-space-4); }
  .sticky-top {
    position: sticky; top: 48px; /* accounts for TopBar on mobile */
    background: var(--shuttle-bg-base);
    z-index: 5;
    padding-bottom: var(--shuttle-space-2);
  }
</style>
```

- [ ] **Step 3d.1.2: Commit**

```bash
git add gui/web/src/features/activity/Activity.svelte
git commit -m "feat(gui): Activity page — speed + transports + logs combined"
```

---

#### Task 3d.2: Share logs action

**Files:**
- Modify: `gui/web/src/features/logs/LogsPage.svelte` (or LogList, wherever the top-right actions live)

- [ ] **Step 3d.2.1: Add share button using platform**

```svelte
<script>
  import { platform } from '@/lib/platform'
  import { exportLogs } from '@/lib/api/endpoints'
  import { toasts } from '@/lib/toaster.svelte'

  async function shareLogs() {
    const url = exportLogs()
    const r = await platform.share({ title: 'Shuttle logs', url })
    if (r === 'unsupported') {
      await navigator.clipboard?.writeText(url)
      toasts.info('Log export URL copied to clipboard')
    }
  }
</script>

<Button onclick={shareLogs}>Share</Button>
```

- [ ] **Step 3d.2.2: Commit**

```bash
git add gui/web/src/features/logs/
git commit -m "feat(gui): share logs via platform.share (native sheet or clipboard fallback)"
```

---

### Phase 3e · Mesh Page

#### Task 3e.1: Mesh tabs (Peers / Topology / Split Route)

**Files:**
- Modify: `gui/web/src/features/mesh/Mesh.svelte`

- [ ] **Step 3e.1.1: Structure the page with Tabs**

Refactor the existing Mesh page into three tabs. The current Mesh.svelte already has peer list + topology. Just wrap in `Tabs` and extract each into its own sub-component if not already.

```svelte
<script lang="ts">
  import { Tabs } from '@/ui'
  import { useRoute, navigate } from '@/lib/router'
  import PeersTab from './PeersTab.svelte'
  import TopologyTab from './TopologyTab.svelte'
  import SplitRouteTab from './SplitRouteTab.svelte'

  const route = useRoute()
  const tab = $derived(route.query.tab ?? 'peers')
  function setTab(t: string) {
    const q = new URLSearchParams({ ...route.query, tab: t }).toString()
    navigate(`/mesh?${q}`, { replace: true })
  }
</script>

<Tabs
  items={[
    { id: 'peers', label: 'Peers' },
    { id: 'topology', label: 'Topology' },
    { id: 'split', label: 'Split Route' },
  ]}
  value={tab}
  onChange={setTab}
/>

{#if tab === 'peers'}<PeersTab />
{:else if tab === 'topology'}<TopologyTab />
{:else if tab === 'split'}<SplitRouteTab />
{/if}
```

Extract Peers / Topology / Split Route content from existing Mesh.svelte into the three files.

- [ ] **Step 3e.1.2: Lazy-load topology canvas**

Topology tab uses dynamic import to avoid shipping the canvas code when user is on Peers tab:

```svelte
<!-- TopologyTab.svelte -->
<script>
  import { onMount } from 'svelte'
  let TopologyChart: any = $state(null)
  onMount(async () => {
    TopologyChart = (await import('./MeshTopologyChart.svelte')).default
  })
</script>
{#if TopologyChart}<TopologyChart />
{:else}<div>Loading topology…</div>
{/if}
```

- [ ] **Step 3e.1.3: Commit**

```bash
git add gui/web/src/features/mesh/
git commit -m "feat(gui): Mesh page — Peers/Topology/SplitRoute tabs, topology lazy-loaded"
```

---

### Phase 3f · Settings Page

#### Task 3f.1: Regroup with Advanced collapse

**Files:**
- Modify: `gui/web/src/features/settings/SettingsPage.svelte`
- Modify: `gui/web/src/features/settings/nav.ts`

- [ ] **Step 3f.1.1: Reorganize into 5 groups**

Group existing settings items into: General, Network, Advanced (collapsed), GeoData, About. Wrap Advanced in a `<details>`:

```svelte
<section>
  <h2>General</h2>
  <!-- theme, language, reset onboarding, check updates -->
</section>
<section>
  <h2>Network</h2>
  <!-- TUN, SOCKS5, HTTP port, listen addr -->
</section>
<details>
  <summary><h2>Advanced</h2></summary>
  <!-- congestion control, buffer sizes, transport prefs, 0-RTT -->
</details>
<section>
  <h2>GeoData</h2>
</section>
<section>
  <h2>About</h2>
</section>
```

- [ ] **Step 3f.1.2: Commit**

```bash
git add gui/web/src/features/settings/
git commit -m "feat(gui): Settings regrouped into General/Network/Advanced/GeoData/About"
```

---

### Phase 3 wrap-up: Delete orphaned feature directories

**Files:**
- Delete: `gui/web/src/features/dashboard/` (Now uses `resource.svelte.ts` — move those utilities to `lib/` first)
- Delete: `gui/web/src/features/logs/` (absorbed by Activity — keep until Activity is proven)

#### Task 3.7: Move dashboard resources into lib

- [ ] **Step 3.7.1: Move resource.svelte.ts**

```bash
git mv gui/web/src/features/dashboard/resource.svelte.ts gui/web/src/lib/resources/dashboard.svelte.ts
```

Update all imports (grep for `features/dashboard/resource`).

- [ ] **Step 3.7.2: Delete unused dashboard files**

```bash
git rm -r gui/web/src/features/dashboard/
```

- [ ] **Step 3.7.3: Delete features/logs/ if all usages moved to activity**

Leave for now if LogList is still used from `activity/Activity.svelte` via `@/features/logs/LogList`. Otherwise move into `features/activity/`.

- [ ] **Step 3.7.4: Run full test + build**

```bash
cd gui/web && npm run check && npm run test && npx playwright test && npm run build
```

- [ ] **Step 3.7.5: Commit**

```bash
git add -A gui/web/src/
git commit -m "chore(gui): remove orphaned dashboard feature dir post-Now migration"
```

---

## Phase 4 · Native Bridge Extension

**Goal:** Extend the Android `MainActivity` + iOS `ShuttleApp` bridge with `requestPermission / scanQR / share / openExternal / subscribeStatus`. The web SPA's capability fallback (`'unsupported'`) means SPA works without this phase — this phase unlocks the OS-native prompts.

### Task 4.1: Shared JS bridge wrapper

**Files:**
- Create: `gui/web/src/lib/platform/shuttle-bridge.ts` (runtime-agnostic helper for Promise-based message passing)

- [ ] **Step 4.1.1: Implement**

File: `gui/web/src/lib/platform/shuttle-bridge.ts`

```ts
// Promise-based wrapper for bidirectional WebView ↔ native messaging.
// Native side sends messages via window.ShuttleVPN method calls, and
// resolves pending requests by calling window._shuttleResolve(id, value).

let counter = 0
const pending = new Map<number, { resolve: (v: any) => void; reject: (e: any) => void }>()

if (typeof window !== 'undefined') {
  ;(window as any)._shuttleResolve = (id: number, value: any) => {
    const p = pending.get(id); if (!p) return
    pending.delete(id); p.resolve(value)
  }
  ;(window as any)._shuttleReject = (id: number, err: string) => {
    const p = pending.get(id); if (!p) return
    pending.delete(id); p.reject(new Error(err))
  }
}

export function callBridge<T>(action: string, payload?: any): Promise<T> {
  return new Promise((resolve, reject) => {
    const id = ++counter
    pending.set(id, { resolve, reject })
    try {
      const b = (window as any).ShuttleVPN
      // iOS uses postMessage; Android uses direct method invocation — both
      // named the same by convention.
      if (typeof b?.invoke === 'function') {
        b.invoke(JSON.stringify({ id, action, payload }))
      } else {
        // Fallback direct call (Android current style): methods each take
        // (id, ...) and write back via _shuttleResolve.
        if (typeof b?.[action] === 'function') {
          Promise.resolve(b[action](id, payload)).then(
            (v) => resolve(v as T),
            (e) => reject(e),
          )
        } else {
          reject(new Error(`bridge method ${action} not available`))
        }
      }
    } catch (e) {
      pending.delete(id); reject(e)
    }
  })
}
```

- [ ] **Step 4.1.2: Update native.ts to use callBridge for new methods**

Update `native.ts`:

```ts
import { callBridge } from './shuttle-bridge'

async requestVpnPermission() {
  if (!hasMethod('requestPermission') && !hasMethod('invoke')) return 'unsupported'
  return await callBridge<'granted' | 'denied'>('requestPermission')
}
// Similarly for scanQR, share, openExternal, subscribeStatus.
```

- [ ] **Step 4.1.3: Commit**

```bash
git add gui/web/src/lib/platform/shuttle-bridge.ts gui/web/src/lib/platform/native.ts
git commit -m "feat(gui): platform — Promise-based bridge helper (callBridge)"
```

---

### Task 4.2: Android bridge extension

**Files:**
- Modify: `mobile/android/app/src/main/java/com/shuttle/app/MainActivity.kt`

- [ ] **Step 4.2.1: Extend VpnBridge class**

In `MainActivity.kt`, extend the inner `VpnBridge` class with new `@JavascriptInterface` methods. Each accepts an `id: Int` parameter and calls `webView.evaluateJavascript("window._shuttleResolve(<id>, ...)", null)` on completion.

```kotlin
inner class VpnBridge {
    @JavascriptInterface fun isVpnRunning() = ShuttleVpnService.isRunning

    @JavascriptInterface
    fun requestPermission(id: Int) {
        runOnUiThread {
            val intent = VpnService.prepare(this@MainActivity)
            if (intent == null) {
                resolve(id, "\"granted\"")
            } else {
                pendingPermissionId = id
                startActivityForResult(intent, VPN_REQUEST_CODE)
            }
        }
    }

    @JavascriptInterface
    fun scanQR(id: Int) {
        runOnUiThread {
            // Launch ZXing Embedded scanner via an Intent (Zxing-android-embedded dep required in Gradle)
            val intent = Intent(this@MainActivity, QrScanActivity::class.java)
            pendingScanId = id
            startActivityForResult(intent, QR_REQUEST_CODE)
        }
    }

    @JavascriptInterface
    fun share(id: Int, payloadJson: String) {
        runOnUiThread {
            val json = JSONObject(payloadJson)
            val intent = Intent(Intent.ACTION_SEND).apply {
                type = "text/plain"
                putExtra(Intent.EXTRA_SUBJECT, json.optString("title"))
                putExtra(Intent.EXTRA_TEXT, json.optString("url").ifEmpty { json.optString("text") })
            }
            startActivity(Intent.createChooser(intent, null))
            resolve(id, "\"ok\"")
        }
    }

    @JavascriptInterface
    fun openExternal(id: Int, url: String) {
        runOnUiThread {
            startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
            resolve(id, "\"ok\"")
        }
    }

    @JavascriptInterface
    fun subscribeStatus(id: Int) {
        // Register listener for VPN status changes; call webView.evaluateJavascript
        // to invoke a callback with the latest status. Implementation detail.
    }

    private fun resolve(id: Int, jsonValue: String) {
        runOnUiThread {
            webView.evaluateJavascript("window._shuttleResolve($id, $jsonValue);", null)
        }
    }
}
```

Add `pendingPermissionId: Int?` and `pendingScanId: Int?` as private fields on `MainActivity`. In `onActivityResult`, resolve the pending id based on request code.

- [ ] **Step 4.2.2: Add QrScanActivity**

Create `mobile/android/app/src/main/java/com/shuttle/app/QrScanActivity.kt` using ZXing Embedded. Add to `build.gradle.kts`:

```kotlin
implementation("com.journeyapps:zxing-android-embedded:4.3.0")
```

Minimal QrScanActivity:

```kotlin
class QrScanActivity : AppCompatActivity() {
    override fun onCreate(b: Bundle?) {
        super.onCreate(b)
        val integrator = IntentIntegrator(this)
        integrator.setDesiredBarcodeFormats(IntentIntegrator.QR_CODE)
        integrator.setOrientationLocked(true)
        integrator.initiateScan()
    }
    override fun onActivityResult(rc: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(rc, resultCode, data)
        val result = IntentIntegrator.parseActivityResult(rc, resultCode, data)
        val code = result?.contents ?: ""
        val out = Intent().apply { putExtra("qr", code) }
        setResult(Activity.RESULT_OK, out)
        finish()
    }
}
```

- [ ] **Step 4.2.3: Commit**

```bash
git add mobile/android/app/
git commit -m "feat(android): extend bridge — requestPermission/scanQR/share/openExternal"
```

---

### Task 4.3: iOS bridge extension

**Files:**
- Modify: `mobile/ios/Shuttle/ShuttleApp.swift`
- Create: `mobile/ios/Shuttle/QrScannerViewController.swift`

- [ ] **Step 4.3.1: Extend message handler**

In `ShuttleApp.swift`, extend `userContentController(_:didReceive:)` to dispatch new actions by reading `message.body` as JSON with `{id, action, payload}`:

```swift
func userContentController(_ ucc: WKUserContentController, didReceive message: WKScriptMessage) {
    guard let body = message.body as? String,
          let data = body.data(using: .utf8),
          let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
          let id = json["id"] as? Int,
          let action = json["action"] as? String
    else { return }

    let payload = json["payload"] as? [String: Any]

    switch action {
    case "requestPermission":
        requestPermission(id: id)
    case "scanQR":
        scanQR(id: id)
    case "share":
        share(id: id, payload: payload ?? [:])
    case "openExternal":
        if let url = payload?["url"] as? String { openExternal(id: id, url: url) }
    case "subscribeStatus":
        subscribeStatus(id: id)
    // existing: isRunning, start, stop, status
    default: break
    }
}

private func requestPermission(id: Int) {
    NEVPNManager.shared().loadFromPreferences { [weak self] err in
        if err != nil { self?.resolve(id, "\"denied\""); return }
        NEVPNManager.shared().saveToPreferences { err2 in
            self?.resolve(id, err2 == nil ? "\"granted\"" : "\"denied\"")
        }
    }
}

private func scanQR(id: Int) {
    let vc = QrScannerViewController { [weak self] code in
        self?.resolve(id, "\"\(code.replacingOccurrences(of: "\"", with: "\\\""))\"")
    }
    present(vc, animated: true)
}

private func share(id: Int, payload: [String: Any]) {
    let items: [Any] = [payload["url"] ?? payload["text"] ?? payload["title"] ?? ""]
    let vc = UIActivityViewController(activityItems: items, applicationActivities: nil)
    vc.completionWithItemsHandler = { [weak self] _, completed, _, _ in
        self?.resolve(id, completed ? "\"ok\"" : "\"cancelled\"")
    }
    present(vc, animated: true)
}

private func openExternal(id: Int, url: String) {
    if let u = URL(string: url) {
        UIApplication.shared.open(u)
        resolve(id, "\"ok\"")
    } else { resolve(id, "\"ok\"") }
}

private func resolve(_ id: Int, _ jsonValue: String) {
    webView.evaluateJavaScript("window._shuttleResolve(\(id), \(jsonValue));")
}
```

Also update the WKUserScript injection — inject an `invoke` bridge on `window.ShuttleVPN`:

```swift
let bridgeScript = WKUserScript(source: """
    window.ShuttleVPN = {
        invoke: function(msg) {
            window.webkit.messageHandlers.shuttleNative.postMessage(msg);
        }
    };
""", injectionTime: .atDocumentStart, forMainFrameOnly: true)
```

- [ ] **Step 4.3.2: Implement QrScannerViewController**

File: `mobile/ios/Shuttle/QrScannerViewController.swift`

```swift
import UIKit
import AVFoundation

class QrScannerViewController: UIViewController, AVCaptureMetadataOutputObjectsDelegate {
    private let onResult: (String) -> Void
    private let session = AVCaptureSession()
    private var preview: AVCaptureVideoPreviewLayer?

    init(onResult: @escaping (String) -> Void) {
        self.onResult = onResult
        super.init(nibName: nil, bundle: nil)
    }
    required init?(coder: NSCoder) { fatalError() }

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .black
        guard let device = AVCaptureDevice.default(for: .video),
              let input = try? AVCaptureDeviceInput(device: device)
        else { onResult(""); dismiss(animated: true); return }
        session.addInput(input)
        let output = AVCaptureMetadataOutput()
        session.addOutput(output)
        output.setMetadataObjectsDelegate(self, queue: .main)
        output.metadataObjectTypes = [.qr]
        let preview = AVCaptureVideoPreviewLayer(session: session)
        preview.frame = view.layer.bounds
        preview.videoGravity = .resizeAspectFill
        view.layer.addSublayer(preview)
        self.preview = preview
        session.startRunning()
    }

    func metadataOutput(_ output: AVCaptureMetadataOutput,
                        didOutput metadataObjects: [AVMetadataObject],
                        from connection: AVCaptureConnection) {
        if let obj = metadataObjects.first as? AVMetadataMachineReadableCodeObject, let str = obj.stringValue {
            session.stopRunning()
            dismiss(animated: true) { [weak self] in self?.onResult(str) }
        }
    }
}
```

Add `NSCameraUsageDescription` to `Info.plist`:

```xml
<key>NSCameraUsageDescription</key>
<string>Scan QR codes to import Shuttle servers and subscriptions.</string>
```

- [ ] **Step 4.3.3: Commit**

```bash
git add mobile/ios/Shuttle/
git commit -m "feat(ios): extend bridge — requestPermission/scanQR/share/openExternal"
```

---

### Task 4.4: SPA capability rollout

**Files:**
- Modify: various SPA files that have action buttons now capable of native paths

- [ ] **Step 4.4.1: Show Scan QR in AddSheet when canScan**

Already wired in Task 3b.4 via `const canScan = $derived(platform.name === 'native')`. Verify:

```bash
grep -n "canScan" gui/web/src/features/servers/AddSheet.svelte
```

- [ ] **Step 4.4.2: Share logs uses platform.share**

Already wired in Task 3d.2.

- [ ] **Step 4.4.3: Smoke test by running `npm run dev` with bridge stub**

For local dev without an actual device, stub `window.ShuttleVPN` in a dev-only script:

```ts
// gui/web/src/dev-bridge.ts (loaded only in dev mode)
if (import.meta.env.DEV && window.location.search.includes('mockbridge=1')) {
  ;(window as any).ShuttleVPN = {
    requestPermission: (id: number) => { window._shuttleResolve(id, 'granted') },
    scanQR: (id: number) => setTimeout(() => window._shuttleResolve(id, 'shuttle://fake'), 500),
    // ...
  }
}
```

Import conditionally in `main.ts`:

```ts
if (import.meta.env.DEV) await import('./dev-bridge')
```

Open `http://localhost:5173/?mockbridge=1` — verify Scan QR button appears in AddSheet.

- [ ] **Step 4.4.4: Commit**

```bash
git add gui/web/src/dev-bridge.ts gui/web/src/main.ts
git commit -m "chore(gui): dev-mode mock bridge for local native-capability testing"
```

---

## Phase 5 · Build & CI

### Task 5.1: Android build script

**Files:**
- Create: `build/scripts/build-android.sh`

- [ ] **Step 5.1.1: Write script**

File: `build/scripts/build-android.sh`

```bash
#!/bin/bash
set -euo pipefail

VERSION="${1:-dev}"
DIST_DIR="${DIST_DIR:-dist}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

echo "Building Android app for version $VERSION..."

# 1. Build SPA and copy to assets
(cd "$ROOT/gui/web" && npm ci && npm run build)
mkdir -p "$ROOT/mobile/android/app/src/main/assets/web"
rm -rf "$ROOT/mobile/android/app/src/main/assets/web"/*
cp -r "$ROOT/gui/web/dist"/* "$ROOT/mobile/android/app/src/main/assets/web/"

# 2. Build AAR via gomobile
mkdir -p "$ROOT/mobile/android/app/libs"
(cd "$ROOT" && gomobile bind -target=android -androidapi=24 \
  -o "$ROOT/mobile/android/app/libs/shuttle.aar" \
  -ldflags="-s -w -X main.version=$VERSION" \
  ./mobile)

# 3. Gradle build
(cd "$ROOT/mobile/android" && ./gradlew assembleRelease)

# 4. Copy artifacts
mkdir -p "$ROOT/$DIST_DIR"
cp "$ROOT/mobile/android/app/build/outputs/apk/release/app-release-unsigned.apk" \
   "$ROOT/$DIST_DIR/shuttle-android-${VERSION}.apk"

echo "Done. Artifact: $DIST_DIR/shuttle-android-${VERSION}.apk"
```

- [ ] **Step 5.1.2: Make executable**

```bash
chmod +x build/scripts/build-android.sh
```

- [ ] **Step 5.1.3: Commit**

```bash
git add build/scripts/build-android.sh
git commit -m "build: add Android APK build script (gomobile bind + gradle)"
```

---

### Task 5.2: iOS build script

**Files:**
- Create: `build/scripts/build-ios.sh`

- [ ] **Step 5.2.1: Write script**

File: `build/scripts/build-ios.sh`

```bash
#!/bin/bash
set -euo pipefail

VERSION="${1:-dev}"
DIST_DIR="${DIST_DIR:-dist}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

echo "Building iOS app for version $VERSION..."

# 1. Build SPA
(cd "$ROOT/gui/web" && npm ci && npm run build)
mkdir -p "$ROOT/mobile/ios/Shuttle/www"
rm -rf "$ROOT/mobile/ios/Shuttle/www"/*
cp -r "$ROOT/gui/web/dist"/* "$ROOT/mobile/ios/Shuttle/www/"

# 2. gomobile bind → xcframework
(cd "$ROOT" && gomobile bind -target=ios,iossimulator \
  -o "$ROOT/mobile/ios/Shuttle.xcframework" \
  -ldflags="-s -w -X main.version=$VERSION" \
  ./mobile)

# 3. xcodebuild archive
mkdir -p "$ROOT/$DIST_DIR"
xcodebuild archive \
  -project "$ROOT/mobile/ios/Shuttle.xcodeproj" \
  -scheme Shuttle \
  -configuration Release \
  -archivePath "$ROOT/$DIST_DIR/Shuttle-${VERSION}.xcarchive" \
  CODE_SIGNING_ALLOWED=NO

echo "Done. Archive: $DIST_DIR/Shuttle-${VERSION}.xcarchive"
```

- [ ] **Step 5.2.2: Make executable**

```bash
chmod +x build/scripts/build-ios.sh
```

- [ ] **Step 5.2.3: Commit**

```bash
git add build/scripts/build-ios.sh
git commit -m "build: add iOS xcarchive build script (gomobile bind + xcodebuild)"
```

---

### Task 5.3: Wire --mobile flag into build-all.sh

**Files:**
- Modify: `build/scripts/build-all.sh`

- [ ] **Step 5.3.1: Add mobile flag handling**

Append at end of `build-all.sh`:

```bash
if [[ "${MOBILE:-}" == "1" || " $* " == *" --mobile "* ]]; then
    echo ""
    echo "Building mobile..."
    "$(dirname "$0")/build-android.sh" "$VERSION" || echo "Android build failed (non-fatal)"
    "$(dirname "$0")/build-ios.sh" "$VERSION" || echo "iOS build failed (non-fatal)"
fi
```

- [ ] **Step 5.3.2: Commit**

```bash
git add build/scripts/build-all.sh
git commit -m "build: build-all.sh --mobile triggers Android + iOS builds"
```

---

### Task 5.4: GitHub Actions mobile workflow

**Files:**
- Create: `.github/workflows/build-mobile.yml`

- [ ] **Step 5.4.1: Write workflow**

File: `.github/workflows/build-mobile.yml`

```yaml
name: Build Mobile

on:
  pull_request:
    paths:
      - 'mobile/**'
      - 'gui/web/**'
      - 'build/scripts/build-android.sh'
      - 'build/scripts/build-ios.sh'
      - '.github/workflows/build-mobile.yml'
  push:
    tags: [ 'v*' ]

jobs:
  android:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.24' }
      - uses: actions/setup-node@v4
        with: { node-version: '22' }
      - uses: actions/setup-java@v4
        with: { java-version: '17', distribution: 'temurin' }
      - uses: android-actions/setup-android@v3
      - name: Install gomobile
        run: go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init
      - name: Build APK
        run: ./build/scripts/build-android.sh ${{ github.ref_name }}
      - uses: actions/upload-artifact@v4
        with:
          name: shuttle-android
          path: dist/shuttle-android-*.apk

  ios:
    runs-on: macos-14
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.24' }
      - uses: actions/setup-node@v4
        with: { node-version: '22' }
      - name: Install gomobile
        run: go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init
      - name: Build xcarchive
        run: ./build/scripts/build-ios.sh ${{ github.ref_name }}
      - uses: actions/upload-artifact@v4
        with:
          name: shuttle-ios
          path: dist/Shuttle-*.xcarchive
```

- [ ] **Step 5.4.2: Commit**

```bash
git add .github/workflows/build-mobile.yml
git commit -m "ci: add build-mobile workflow (Android APK + iOS xcarchive)"
```

---

### Task 5.5: Playwright responsive + navigation smoke

**Files:**
- Create: `gui/web/tests/navigation.spec.ts`

- [ ] **Step 5.5.1: Write test**

File: `gui/web/tests/navigation.spec.ts`

```ts
import { test, expect } from '@playwright/test'

const paths = ['/', '/servers', '/traffic', '/mesh', '/activity', '/settings']

for (const path of paths) {
  test(`${path} loads without console errors`, async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', e => errors.push(String(e)))
    page.on('console', msg => { if (msg.type() === 'error') errors.push(msg.text()) })
    await page.goto(`/#${path}`)
    await page.waitForLoadState('networkidle')
    expect(errors.filter(e => !e.includes('Failed to fetch'))).toEqual([])
  })
}

test('legacy /dashboard redirects to /', async ({ page }) => {
  await page.goto('/#/dashboard')
  await page.waitForURL(/#\/$/)
})
test('legacy /subscriptions redirects to /servers?source=subscriptions', async ({ page }) => {
  await page.goto('/#/subscriptions')
  await page.waitForURL(/#\/servers\?source=subscriptions/)
})
```

- [ ] **Step 5.5.2: Run**

```bash
cd gui/web && npx playwright test tests/navigation.spec.ts
```

- [ ] **Step 5.5.3: Commit**

```bash
git add gui/web/tests/navigation.spec.ts
git commit -m "test(gui): navigation.spec — all 6 pages load + legacy redirects"
```

---

### Task 5.6: Native smoke tests (non-blocking)

**Files:**
- Create: `mobile/ios/ShuttleUITests/ShuttleUITests.swift`
- Create: `mobile/android/app/src/androidTest/java/com/shuttle/app/MainActivityTest.kt`

- [ ] **Step 5.6.1: iOS XCUITest**

File: `mobile/ios/ShuttleUITests/ShuttleUITests.swift`

```swift
import XCTest

final class ShuttleUITests: XCTestCase {
    func testAppLaunchesAndShowsNow() {
        let app = XCUIApplication()
        app.launch()
        // Wait for WebView to render the Now page label
        let nowLabel = app.staticTexts["Now"].firstMatch
        XCTAssertTrue(nowLabel.waitForExistence(timeout: 10))
    }
}
```

- [ ] **Step 5.6.2: Android Espresso**

File: `mobile/android/app/src/androidTest/java/com/shuttle/app/MainActivityTest.kt`

```kotlin
package com.shuttle.app

import androidx.test.espresso.Espresso.onView
import androidx.test.espresso.assertion.ViewAssertions.matches
import androidx.test.espresso.matcher.ViewMatchers.isDisplayed
import androidx.test.espresso.matcher.ViewMatchers.withId
import androidx.test.ext.junit.rules.ActivityScenarioRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class MainActivityTest {
    @get:Rule val rule = ActivityScenarioRule(MainActivity::class.java)

    @Test fun appLaunches() {
        // Placeholder: WebView content is hard to assert without instrumentation.
        // Just verify the MainActivity starts.
        rule.scenario.onActivity { assert(!it.isFinishing) }
    }
}
```

- [ ] **Step 5.6.3: Commit**

```bash
git add mobile/ios/ShuttleUITests mobile/android/app/src/androidTest
git commit -m "test(mobile): smoke tests for iOS XCUITest + Android instrumentation"
```

---

### Task 5.7: Manual smoke checklist

**Files:**
- Create: `docs/mobile-smoke.md`

- [ ] **Step 5.7.1: Write checklist**

File: `docs/mobile-smoke.md`

```markdown
# Mobile Smoke Test Checklist

Run before every release that includes mobile changes.

## iOS (14.0+)
- [ ] App installs and launches
- [ ] Now page renders: power button centered, "Disconnected" label above
- [ ] Tap power → system VPN permission dialog appears
- [ ] Grant permission → state transitions to Connecting → Connected (or error toast on network failure)
- [ ] Tap server chip → navigates to /servers
- [ ] On /servers, tap "+ Add" → sheet opens
- [ ] Scan QR works (test with any QR code)
- [ ] On /activity, logs list scrolls smoothly with > 500 entries
- [ ] Share logs → iOS share sheet appears
- [ ] Rotate portrait → landscape: layout does not break
- [ ] System dark mode toggle reflects immediately
- [ ] Background → foreground: connection state re-syncs
- [ ] Legacy URLs work: paste `shuttle://` link → opens app, imports server

## Android (7.0+)
(Same scenarios, replacing iOS-specific terminology)

## Notes
- iOS VPN-mode (full tunnel) still uses inline HTML fallback (follow-up spec covers app↔extension IPC).
- QR scan requires camera permission prompt on first use — accept once.
```

- [ ] **Step 5.7.2: Commit**

```bash
git add docs/mobile-smoke.md
git commit -m "docs: mobile smoke test checklist"
```

---

## Final integration: Phase 5 verification

### Task 5.8: End-to-end release gate

- [ ] **Step 5.8.1: Full test matrix**

```bash
cd gui/web
npm ci
npm run check
npm run test
npx playwright test
npm run build
```

All must pass.

- [ ] **Step 5.8.2: Build mobile artifacts (if macOS/Android toolchain present)**

```bash
cd ..  # repo root
./build/scripts/build-all.sh v0.2.0 --mobile
```

- [ ] **Step 5.8.3: Update CLAUDE.md if needed**

If the 6-page IA changes how developers discover features, add a note:

```markdown
## GUI IA (post 2026-04-21)

Six top-level pages: `/` (Now), `/servers`, `/traffic`, `/mesh`, `/activity`,
`/settings`. Subscriptions and Groups are filters on /servers; logs are a tab on
/activity. Legacy routes redirect client-side.
```

- [ ] **Step 5.8.4: Final commit**

```bash
git add CLAUDE.md
git commit -m "docs: CLAUDE.md — document 6-page IA after mobile unification"
```

---

## Plan complete.

Summary:
- **5 phases**, ~50 atomic tasks
- **Phase 1** (infrastructure): 16 tasks — viewport, platform runtimes, Stack/ResponsiveGrid, nav components, AppShell
- **Phase 2** (IA routing): 6 tasks — route-migration + AppShell wiring behind flag
- **Phase 3** (page impl): Sub-phases 3a–3f, ~16 tasks — each page implemented or consolidated
- **Phase 4** (bridge): 4 tasks — Android + iOS extension + JS wrapper + SPA rollout
- **Phase 5** (build/CI): 8 tasks — Android/iOS scripts + CI workflow + Playwright matrix + native smoke + docs

Each phase is independently mergeable. Feature flag `VITE_USE_LEGACY_SHELL=1` provides rollback until Phase 2 is stable.
