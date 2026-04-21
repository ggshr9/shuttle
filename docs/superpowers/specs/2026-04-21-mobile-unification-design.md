# Mobile Unification · Design Spec

- **Date**: 2026-04-21
- **Scope**: Shuttle GUI (`gui/web` + `mobile/ios` + `mobile/android`)
- **Out of scope**: iOS VPN-mode inline-HTML replacement (app↔extension IPC) — follow-up spec
- **Status**: draft, awaiting user review

## 1. Problem

Shuttle's GUI ships on macOS, Windows, Linux (Wails), and in stub form on iOS/Android (WebView + gomobile). The Go runtime, REST API, and config schema are already unified — but the user-facing shell is not:

1. The Svelte SPA in `gui/web/` has **no responsive breakpoints**. `Sidebar.svelte` is hard-coded to 220px. `Dashboard.svelte` caps at 1080px in a desktop-oriented grid. On a phone the app does not fit.
2. The native bridges (`window.ShuttleVPN` in `ShuttleApp.swift` / `MainActivity.kt`) are injected but **no SPA code ever calls them**. The only path to trigger an iOS/Android VPN permission prompt is the native bridge — the REST API cannot do this. Today, tapping Connect on mobile cannot request VPN permission.
3. The IA has **9 top-level nav items** (Dashboard, Servers, Subscriptions, Groups, Routing, Mesh, Logs, Settings, + Groups detail). Three of these (Subscriptions, Groups, Logs-vs-Dashboard-stats) reflect code module boundaries, not user mental models. This shape is too wide for phones and muddles the desktop UX as well.
4. iOS VPN mode falls back to a hardcoded inline "Connect" HTML page in `ShuttleApp.swift:152-194`, visibly diverging from the SPA on every other surface.

The goal stated by the project lead: **users should experience the same product on every platform.** "Same" means: same information architecture, same labels, same primary action, same interaction metaphors — allowing each form factor to render those with native-appropriate chrome.

## 2. Goals

1. One unified information architecture (6 nav items) shared by desktop, tablet, phone.
2. Every page is fully editable on every form factor — no "please use desktop" dead-ends.
3. Dashboard's central act is a single power toggle with three states (disconnected / connecting / connected).
4. iOS/Android tap-to-Connect fires the correct OS-level VPN permission prompt.
5. Desktop experience does not regress visually or functionally.
6. No backend / REST API / config schema changes.

## 3. Non-goals (this spec)

- Replacing iOS VPN-mode inline HTML with the real SPA (requires app↔extension IPC, separate spec).
- Visual-regression baseline for the SPA.
- Play Store / App Store release pipeline.
- F-Droid packaging.
- Rewriting any Go engine or transport code.

## 4. Information Architecture

### 4.1 Six top-level areas

| ID | Label | URL | Purpose |
|---|---|---|---|
| now | Now | `/` | Current connection state, power button, current server chip, instant speed |
| servers | Servers | `/servers` | All servers incl. subscription-sourced + groups; add / edit / switch / subscription management |
| traffic | Traffic | `/traffic` | Routing rules, templates, PAC, DNS, split-tunnel, conflict detection |
| mesh | Mesh | `/mesh` | Peer list, add/remove, mDNS rediscover, split-route; topology view as a sub-tab |
| activity | Activity | `/activity` | Real-time speed chart, transport breakdown, connection log stream, filter |
| settings | Settings | `/settings` | General / Network / Advanced (collapsed) / GeoData / About |

### 4.2 Nav rendering per form factor

- **Phone (< 720px)**: bottom Tab Bar with 5 items `Now / Servers / Traffic / Mesh / Activity`; `Settings` reachable via gear icon in the global `TopBar`.
- **Tablet (720–1024px)**: left 56px Navigation Rail, 6 items vertical icon + mini label.
- **Desktop (≥ 1024px)**: left 220px Sidebar, 6 items grouped (`Overview: Now, Activity` / `Network: Servers, Traffic, Mesh` / `System: Settings`).

All three render from one `app/nav.ts` source of truth — same labels, same icons, same order.

### 4.3 Route migration

Old → new, with client-side router redirects and a one-time dismissable toast:

| Old | New |
|---|---|
| `/dashboard` | `/` |
| `/subscriptions` | `/servers?source=subscriptions` |
| `/subscriptions/:id` | `/servers?source=subscription:<id>` |
| `/groups` | `/servers?view=groups` |
| `/groups/:id` | `/servers?group=<id>` |
| `/routing` | `/traffic` |
| `/logs` | `/activity?tab=logs` |

Dismiss state in `localStorage.shuttle-route-migration-seen`. Old routes supported for two minor versions, removed at the next major.

## 5. Page Internals

### 5.1 Now · `/`

- Single-column centered layout used on every form factor.
- Status label top: `Disconnected` / `Connecting…` / `Connected · 2h 14m`.
- 120px circular power button, three states: gray idle / orange spinner ring / green steady. Haptic tap on mobile (`navigator.vibrate(10)`).
- Below button: current-server chip (tap opens server quick-picker sheet, does not navigate away).
- When connected: two small speed figures (`↓ 3.2 MB/s`, `↑ 180 KB/s`). No chart — chart lives in Activity.
- Single tertiary action: `Switch server →` routes to `/servers`.
- Desktop renders identically, with extra whitespace — does not try to fill 1920px.

### 5.2 Servers · `/servers`

- Top bar: search + `[+ Add]` sheet (Manual / Paste URL / QR scan / Subscribe to URL).
- Filter chip row (horizontally scrollable): `All · Manual · Subscription: X · Group: Y`.
- When "Subscription" filter active: per-subscription banner atop the list showing URL, last-refresh timestamp, server count, `[Refresh] [Edit] [Delete]`.
- Row: name / address / transport / latency / source chip. Actions: tap to select active; swipe or overflow menu to delete; detail icon → `/servers/:id`.
- Desktop `≥ md`: container-query two-column (list 50% + detail 50%); right pane swaps on row click, no route change.
- Mobile: single-column; row click routes to full-screen `/servers/:id`.

### 5.3 Traffic · `/traffic`

- Tabs: `Rules · Templates · DNS · Split Tunnel · PAC`.
- **Rules**: summary card `N rules · M conflicts [review]`; active template `Applying: Bypass CN [change]`; rule list (type icon + value + action chip); tap → `/traffic/rules/:id` form page (type dropdown / value input with geosite autocomplete / action dropdown). FAB `+` on mobile, top-right button on desktop.
- **Templates**: card list (bypass-cn / proxy-all / direct-lan), one-tap apply.
- **DNS**: domestic / remote server inputs, cache TTL, prefetch toggle.
- **Split Tunnel**: subnet + policy form.
- **PAC**: generated URL + `[Copy]` + `[Download]`.
- Conflict banner appears above tabs when any conflict exists; tap → full-screen conflict list.

### 5.4 Mesh · `/mesh`

- Tabs: `Peers · Topology · Split Route`.
- **Peers**: summary chip (`3 online / 5 total · P2P: 2, Relay: 1`); row = name / online state / mode / RTT / last seen. Tap → `/mesh/peers/:id` (disconnect, rename note, remove). Top-level actions: `[+ Add peer]` (QR / paste token), `[Rediscover]`.
- **Topology**: full-width canvas (reuse current component); mobile supports pinch-zoom + single-finger pan.
- **Split Route**: subnet + policy form.

### 5.5 Activity · `/activity`

- Top sticky: real-time speed sparkline (5-min rolling window).
- Middle: transport breakdown (right-aligned on desktop, stacked below on mobile).
- Bottom: log stream — filter bar (level multi-chip / tag / time range), virtual-scroll list, tap row inline-expands JSON; `[Share]` top-right uses Web Share API on mobile, clipboard on desktop.

### 5.6 Settings · `/settings`

Sections:

- **General**: theme / language / reset onboarding / check for updates.
- **Network**: TUN toggle / SOCKS5 port / HTTP port / listen addresses.
- **Advanced** (collapsed by default): congestion control / buffer sizes / transport preferences / 0-RTT.
- **GeoData**: source dropdown / auto-update toggle / `[Update now]`.
- **About**: version / license / log path / OSS notices.

All fields use a common `<Field>` component (label + control + description). Mobile stacks full-bleed; desktop two columns.

## 6. Responsive Layout System

### 6.1 Breakpoints

| Range | Name |
|---|---|
| `< 480px` | `xs` |
| `480–720px` | `sm` |
| `720–1024px` | `md` |
| `1024–1440px` | `lg` |
| `≥ 1440px` | `xl` |

Shell switches at `720px` (phone ↔ tablet) and `1024px` (tablet ↔ desktop). Page-internal layout uses CSS container queries.

### 6.2 Viewport store

`gui/web/src/lib/viewport.svelte.ts`:

- Exports reactive object `viewport = { width, form, isMobile, isTablet, isDesktop, isTouch }`.
- `form` ∈ `'xs' | 'sm' | 'md' | 'lg' | 'xl'`.
- `isTouch = matchMedia('(pointer: coarse)').matches`.
- One `ResizeObserver` on `document.documentElement`, rAF-throttled. Only place the app reads `window.innerWidth`.

### 6.3 App Shell

`app/AppShell.svelte` replaces `Shell.svelte`:

```
AppShell
├── TopBar           (always; logo + page title + Settings gear)
├── Sidebar | Rail   (viewport.form >= 'md')
├── <main><Router/></main>
└── BottomTabs       (viewport.form in {xs, sm})
```

Three nav components (`Sidebar` / `Rail` / `BottomTabs`) all read from `app/nav.ts`:

```ts
export const nav = [
  { id: 'now',      path: '/',         label, icon, section: 'overview', primary: true  },
  { id: 'servers',  path: '/servers',  label, icon, section: 'network',  primary: true  },
  { id: 'traffic',  path: '/traffic',  label, icon, section: 'network',  primary: true  },
  { id: 'mesh',     path: '/mesh',     label, icon, section: 'network',  primary: true  },
  { id: 'activity', path: '/activity', label, icon, section: 'overview', primary: true  },
  { id: 'settings', path: '/settings', label, icon, section: 'system',   primary: false },
]
```

### 6.4 Layout primitives

- `ui/Stack.svelte` — `gap`, `direction`, `wrap`, `breakAt` props; default vertical, horizontal above `breakAt`.
- `ui/ResponsiveGrid.svelte` — `cols={{ xs: 1, md: 2, lg: 3 }}` prop.

Together they cover ~80% of current ad-hoc `@media` needs.

### 6.5 Container queries

Page-internal column switching uses CSS container queries (Safari 16 / Chrome 105 / Firefox 110 — all supported in WKWebView and Android WebView in target versions).

Fallback: on load, `CSS.supports('container-type', 'inline-size')` check; if false, log a warning and degrade to media query (layout may not be optimal at intermediate widths but remains functional).

### 6.6 Safe-area and touch

- `TopBar` → `padding-top: env(safe-area-inset-top)`.
- `BottomTabs` → `padding-bottom: env(safe-area-inset-bottom)`.
- Touch-active: global rule `[data-touch] button, [data-touch] a { min-height: 44px }`.
- `<main>` has `overscroll-behavior: contain`.
- All `:hover` styles wrapped in `@media (hover: hover)`.

### 6.7 Anti-patterns (forbidden)

- **No** `Dashboard.mobile.svelte` sibling files. One `.svelte` per page, responsive via CSS and primitives.
- **No** `{#if viewport.isMobile}` branching of page-level markup; only acceptable at the AppShell boundary where the nav component itself must change.
- **No** custom scroll containers.

## 7. Native Bridge Abstraction

### 7.1 Goal

SPA code calls `platform.*`, unaware of runtime. Adding a future runtime (iOS VPN-mode IPC) touches only one module.

### 7.2 Module layout

```
gui/web/src/lib/platform/
├── index.ts    exports detect() + unified Platform API
├── types.ts    Platform / Capability types
├── web.ts      default: REST + Web APIs
├── native.ts   iOS/Android WebView: window.ShuttleVPN + REST mix
└── wails.ts    Desktop Wails: direct Go bindings + REST
```

### 7.3 Detection

```ts
function detect(): PlatformName {
  if (typeof window === 'undefined') return 'web'
  if ((window as any).go?.main?.App) return 'wails'
  if ((window as any).ShuttleVPN) return 'native'
  return 'web'
}
```

Result cached as singleton; no runtime switching.

### 7.4 Capability interface

```ts
export interface Platform {
  name: 'web' | 'native' | 'wails'

  engineStart(): Promise<void>
  engineStop(): Promise<void>
  engineStatus(): Promise<Status>

  requestVpnPermission(): Promise<'granted' | 'denied' | 'unsupported'>
  scanQRCode(): Promise<string | 'unsupported'>
  share(payload: { title?: string; text?: string; url?: string }): Promise<'ok' | 'cancelled' | 'unsupported'>
  openExternalUrl(url: string): Promise<'ok' | 'unsupported'>

  onStatusChange(cb: (s: Status) => void): () => void
}
```

### 7.5 Runtime behaviors

**`web.ts`** (desktop browsers, default):
- Engine APIs → REST `/api/engine/*`.
- `requestVpnPermission` → `'unsupported'`.
- `scanQRCode` → `'unsupported'` (future: `BarcodeDetector`).
- `share` → `navigator.share?.()` else copy-to-clipboard + toast.
- `openExternalUrl` → `window.open(url, '_blank')`.
- `onStatusChange` → WebSocket `/api/events`.

**`native.ts`** (iOS/Android WebView):
- `engineStart` / `engineStop`: if TUN mode → bridge `start` / `stop`; else REST.
- `engineStatus`: REST (always reachable; engine in-app or in VPN service).
- `requestVpnPermission`: bridge → native `VpnService.prepare()` (Android) / `NEVPNManager.save` (iOS).
- `scanQRCode`: bridge → native camera scan activity/view controller.
- `share`: bridge → system share sheet.
- `openExternalUrl`: bridge → `Intent.ACTION_VIEW` / `UIApplication.open`.
- `onStatusChange`: bridge subscribe; fallback REST WebSocket.

Each capability that relies on a bridge method MUST first check `typeof window.ShuttleVPN.<method> === 'function'` and return `'unsupported'` if missing. This lets Phase 1 ship `platform.ts` before Phase 4 extends the bridge — on today's mobile builds, new capabilities simply report `'unsupported'` and the UI falls back (QR scan button hidden, share uses clipboard, etc.). Each phase expands capability coverage without requiring simultaneous ship of SPA + native binary.

**`wails.ts`** (desktop GUI):
- Engine APIs → Wails-generated Go bindings (low latency).
- `requestVpnPermission`: `'unsupported'`.
- `scanQRCode`: `'unsupported'`.
- `share`: `'unsupported'`.
- `openExternalUrl`: Wails `runtime.BrowserOpenURL`.

### 7.6 UI-level capability checks

```svelte
<script>
  import { platform } from '@/lib/platform'

  async function onScanClick() {
    const result = await platform.scanQRCode()
    if (result === 'unsupported') { fallbackToPaste = true; return }
    importFromQR(result)
  }
  $: canScan = platform.name === 'native'
</script>

{#if canScan}<Button onclick={onScanClick}>Scan QR</Button>{/if}
<Button onclick={pasteURL}>Paste URL</Button>
```

### 7.7 Native bridge extension

Current bridge exposes `isRunning / start / stop / getStatus`. Extended to:

- `requestPermission()`
- `scanQR()`
- `share(payload)`
- `openExternal(url)`
- `subscribeStatus(callback)`

Request/response uses a shared JS helper (`ShuttleVPN.js`) that wraps messages in `{ requestId, action, payload }` and resolves pending promises from `window._shuttlePending[id]`. Android and iOS native sides only receive/send JSON messages.

### 7.8 Initialization timing

- `platform.ts` runs `detect()` synchronously at module load.
- Bridge must be injected **before** WebView loads the SPA URL.
  - iOS: `WKUserScript` with `injectionTime: .atDocumentStart` (already correct).
  - Android: `addJavascriptInterface` is sync-safe but current code at `MainActivity.kt:53-58` waits for `onPageFinished` to inject the JS wrapper, which races with SPA boot. Fix: inject wrapper via `WebViewClient.shouldInterceptRequest` returning the wrapper as a virtual script asset, or inline the wrapper into the HTML shell at build time.
- SPA boot retries `window.ShuttleVPN` presence 3× over 300ms as a safety net.

## 8. Build & Test Strategy

### 8.1 Build matrix

**Android** (`build/scripts/build-android.sh`):
1. `gomobile bind -target=android -androidapi=24 -o mobile/android/app/libs/shuttle.aar ./mobile`
2. `cd gui/web && npm run build` → copy to `mobile/android/app/src/main/assets/web/`
3. `cd mobile/android && ./gradlew assembleRelease`
4. Output: `dist/shuttle-android-<version>.apk` + `.aab`

**iOS** (`build/scripts/build-ios.sh`):
1. `gomobile bind -target=ios,iossimulator -o mobile/ios/Shuttle.xcframework ./mobile`
2. SPA build output copied to `mobile/ios/Shuttle/www/`
3. `xcodebuild archive` → `dist/Shuttle-<version>.xcarchive`

`build-all.sh --mobile` triggers both; default skips to keep local dev fast.

### 8.2 CI

`.github/workflows/build-mobile.yml` — new:
- Android job: macOS or Ubuntu runner, Go + JDK + Android SDK, `build-android.sh`, upload APK artifact.
- iOS job: macOS-14 runner with Xcode 15, `build-ios.sh`, upload xcarchive.
- PR builds only; tag pushes run a release job.
- First version non-blocking (not a required check).

### 8.3 Unit tests (vitest)

New files:
- `lib/platform/*.test.ts` — three runtimes, detect, capability fallback.
- `lib/viewport.svelte.test.ts` — breakpoints, resize throttle.
- `ui/Stack.test.ts` / `ui/ResponsiveGrid.test.ts` — breakpoint-driven output.
- One test per nav component (`Sidebar` / `Rail` / `BottomTabs`) covering item rendering, active state, click.

### 8.4 Playwright — three-viewport matrix

```ts
// gui/web/playwright.config.ts
projects: [
  { name: 'desktop', use: { viewport: { width: 1440, height: 900 } } },
  { name: 'tablet',  use: { viewport: { width: 820,  height: 1180 } } },
  { name: 'phone',   use: { ...devices['iPhone 14'] } },
]
```

Smoke suites run in all three:
- `responsive.spec.ts` — correct nav component (Sidebar / Rail / BottomTabs).
- `navigation.spec.ts` — visit all 6 URLs, no console errors.
- `onboarding.spec.ts` — wizard completes.
- `connect-flow.spec.ts` — Now power button with mocked REST.

### 8.5 Native smoke tests (opt-in CI)

**iOS** (`mobile/ios/ShuttleUITests/`) XCUITest: launch → WebView loads → Now label visible → tap power → system VPN permission dialog handled via `springboardApp`.

**Android** (`mobile/android/app/src/androidTest/`) Espresso + Web UI matcher: same scenario.

Both `continue-on-error: true` in first release.

### 8.6 Manual acceptance checklist

File: `docs/mobile-smoke.md`. Run every release:

- [ ] iOS 14.0+ / Android 7.0+ device install + launch.
- [ ] Power button three-state colors correct.
- [ ] First tap fires system VPN permission dialog.
- [ ] Paste subscription URL + manual refresh → servers list populates.
- [ ] QR scan to add a server.
- [ ] Activity log stream scrolls smoothly with >1000 rows.
- [ ] Rotate portrait→landscape, layout unbroken.
- [ ] System dark mode switch reflects immediately.
- [ ] Background → foreground, connection state re-syncs.

## 9. Migration & Rollout

### 9.1 Phases

Each phase is an independently mergeable, revertable PR group.

**Phase 1 · Infrastructure** (user-invisible)
- `lib/viewport.svelte.ts`, `lib/platform/*`, `ui/Stack.svelte`, `ui/ResponsiveGrid.svelte`.
- New nav components (`TopBar`, `BottomTabs`, `Rail`) built but not wired.
- `AppShell.svelte` prepared but does not replace `Shell.svelte`.
- All engine calls migrated to `platform.engineStart/Stop/Status` (REST-equivalent behavior, regression-neutral).

**Phase 2 · IA · routing layer**
- Add 6 new routes + client-side redirects with one-time toast.
- Swap `AppShell` in for `Shell`; nav renders per viewport.
- Each new page stubbed with a "coming soon" card.
- User-visible: nav shape changes; URLs change; page contents unchanged.

**Phase 3 · Page implementation** (ordered by daily-use priority)
- 3a · Now (Power button, replaces Dashboard)
- 3b · Servers (absorbs Subscriptions + Groups — biggest business consolidation)
- 3c · Traffic (renamed from Routing + mobile rule form)
- 3d · Activity (absorbs Logs + stats)
- 3e · Mesh (Peers first, Topology lazy-loaded)
- 3f · Settings (regrouped)
- Each sub-phase shippable independently.

**Phase 4 · Native bridge extension**
- Android `MainActivity.kt` adds new handlers; iOS `ShuttleApp.swift` adds new message actions.
- `ShuttleVPN.js` helper shipped.
- SPA capability-check reveals QR scan / native share where supported.
- VPN permission prompts fire correctly on both mobile platforms.

**Phase 5 · Mobile CI & packaging**
- `build-android.sh`, `build-ios.sh`, `build-mobile.yml`.
- Playwright three-viewport matrix.
- Native smoke tests (continue-on-error).
- `docs/mobile-smoke.md`.

### 9.2 Backward-compat surface

- Backend REST endpoints unchanged.
- `config/config.go` unchanged.
- Old frontend routes redirect for 2 minor versions (toast first version, silent second), removed at next major.
- Old i18n keys (`nav.dashboard`, `nav.routing`, `nav.logs`, `nav.subscriptions`, `nav.groups`) retained this release, deleted next minor.

### 9.3 Feature flags

- `VITE_USE_LEGACY_SHELL=1` — swap back to old `Shell.svelte` if Phase 2 rollout regresses. Removed 2 versions after Phase 2 ships clean.

### 9.4 Accessibility

- All three nav components implement ARIA `tablist` / `tab` / `tabpanel` + arrow-key navigation (Sidebar already has it; reused).
- `BottomTabs` gets `aria-label="Primary navigation"`.
- Power button is `role="switch"` + `aria-checked` + `aria-label={connected ? 'Disconnect' : 'Connect'}`.
- `:focus-visible` outlines retained globally.

### 9.5 Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Users can't find "Subscriptions" | Med | Med | Route redirect + toast; FAQ entry |
| Container queries unsupported in old WebView | Low | High | `CSS.supports` check at boot, fallback to media query |
| Android JS interface race vs SPA boot | Med | High | 3×100ms retry for `window.ShuttleVPN`; switch to sync injection |
| Desktop users dislike visual change | Med | Med | Collect feedback pre-GA; `VITE_USE_LEGACY_SHELL` escape hatch |
| `gomobile bind` not reproducible locally | High | Low | CI doesn't depend on local reproducibility; document in `docs/mobile-smoke.md` |

## 10. Open Questions

*(None at spec freeze — all design decisions resolved during brainstorming. Add future discoveries here as implementation proceeds.)*

## 11. Follow-up Specs

- **iOS VPN-mode SPA unification**: replace inline HTML (`ShuttleApp.swift:152-194`) with app↔extension IPC so the full SPA loads even when `PacketTunnelProvider` owns the engine. Requires XPC or Unix-socket proxy for REST + status events.
- **Mobile app-store pipelines**: signing, notarization, TestFlight, Play Store listings.
- **Visual regression baseline**: Playwright `toHaveScreenshot` coverage across the three viewports.

---

*Spec authored during brainstorming session on 2026-04-21. Approved sections: IA, responsive system, page internals, bridge abstraction, test strategy, rollout.*
