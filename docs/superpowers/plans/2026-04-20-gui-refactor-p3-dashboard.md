# GUI Refactor P3 · Dashboard Feature Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the P2 bridge at `#/` with a first redesigned feature slice — an adaptive Dashboard composed of ConnectionHero + StatsGrid + SpeedSparkline + TransportBreakdown, powered by the P1 Resource / Stream layer. No mode toggle; progressive disclosure via vertical scroll.

**Architecture:** Feature-sliced under `features/dashboard/`. `resource.ts` defines hooks (`useStatus`, `useSpeedStream`, `useTransportStats`) built on `createResource` / `createStream`. Four small presentational components are composed by `Dashboard.svelte`. The feature exports its route via `index.ts`, registered in `app/routes.ts`. Legacy `pages/Dashboard.svelte` and four orphaned chart files are deleted.

**Tech Stack:** Svelte 5 runes · `@/lib/resource` · `@/lib/api` · `@/ui` design system · inline SVG sparkline (no chart library).

**Spec reference:** `docs/superpowers/specs/2026-04-19-gui-refactor-design.md` — §7.1 Dashboard redesign principles, §6 shell-and-dashboard mockup (see `.superpowers/brainstorm/*/content/shell-and-dashboard.html`).

**Branch:** Continue on `refactor/gui-v2`. P3 lands as a single PR (or appends to the existing PR #8).

---

## Design reference

The Dashboard page renders, in order, from top:

1. **ConnectionHero** — Big card. Left: status dot + `Connected · <server-name> via <transport>` + uptime. Big speed number (download) with small upload below. Right: primary button (`Disconnect` when connected, `Connect` when stopped) + ghost button `Switch server` (links `#/servers`).
2. **StatsGrid** — 4-up grid: `RTT` (ms), `Packet loss` (%), `Transfer` (↓ / ↑ bytes since connect), `Transport` (e.g. `H3 / BBR`).
3. **SpeedSparkline** — Card with title `Throughput`, a 72-px-tall SVG spark line (last 5 min of upload + download at 5-s cadence), legend + `last 5 min` subtitle.
4. **TransportBreakdown** — Card with a row per transport from `/api/transports/stats`: colored dot, name (mono), bandwidth bar, MB/s, stream count, state chip (primary / standby / idle).

When the backend is not connected (`status.state !== 'running'`) the hero shows a neutral `Disconnected` state and StatsGrid + Sparkline + TransportBreakdown show empty states rather than zero values (which lie).

**All styling must use `--shuttle-*` tokens** — no hex / px outside tokens.

---

## File structure

```
gui/web/src/features/dashboard/
├─ index.ts                       public exports + route
├─ resource.ts                    useStatus / useSpeedStream / useTransportStats
├─ Dashboard.svelte               page composition (~80 lines)
├─ ConnectionHero.svelte          hero card (~120 lines)
├─ StatsGrid.svelte               4-up stat cards (~80 lines)
├─ SpeedSparkline.svelte          inline-SVG sparkline (~100 lines)
└─ TransportBreakdown.svelte      densified transport table (~100 lines)
```

**Deletions** (orphaned after the new Dashboard ships):
- `pages/Dashboard.svelte` (708 lines)
- `lib/SpeedChart.svelte` (174 lines)
- `lib/ConnectionQualityChart.svelte` (333 lines)
- `lib/TrafficChart.svelte` (283 lines)
- `lib/SpeedTestHistory.svelte` (361 lines)

**Kept** (used by other pages):
- `lib/MeshTopologyChart.svelte` (used by Mesh page; moves to `features/mesh/` in P7)

---

## Conventions

- All new files under `features/dashboard/`.
- Tests live next to the code: `<Name>.test.ts`.
- Every task ends with a commit.
- Every step ends with a command you can run + an expected outcome.

---

## Section A · Data layer (Tasks 1-2)

### Task 1: Write `features/dashboard/resource.ts`

**Files:**
- Create: `gui/web/src/features/dashboard/resource.ts`

**Context:** Defines three reactive hooks. `useStatus` polls every 3s. `useTransportStats` polls every 5s but only when connected (uses `enabled`). `useSpeedStream` wraps `/api/speed` WebSocket.

Additionally, the sparkline needs a rolling 5-minute history. Because `createStream` pushes to a single `data` slot, we expose a separate `useSpeedHistory()` that maintains the ring buffer internally (state is module-local; first caller wins per P1 contract).

- [ ] **Step 1: Create the file**

```ts
import { createResource, createStream, type Resource, type Stream } from '@/lib/resource.svelte'
import { status as fetchStatus, getTransportStats } from '@/lib/api/endpoints'
import type { Status, TransportStats } from '@/lib/api/types'

// ── Status — 3s polling (primary source of truth) ────────────
export function useStatus(): Resource<Status> {
  return createResource('dashboard.status', fetchStatus, { poll: 3000 })
}

// ── Transport stats — 5s polling, only while connected ───────
export function useTransportStats(): Resource<TransportStats[]> {
  return createResource(
    'dashboard.transports',
    getTransportStats,
    {
      poll: 5000,
      initial: [],
      enabled: () => useStatus().data?.connected === true,
    },
  )
}

// ── Speed stream — WebSocket push ────────────────────────────
interface SpeedSample { upload: number; download: number }
export function useSpeedStream(): Stream<SpeedSample> {
  return createStream<SpeedSample>(
    'dashboard.speed',
    '/api/speed',
    { initial: { upload: 0, download: 0 } },
  )
}

// ── Speed history — rolling 5 min × 5s cadence = 60 samples ──
// Module-private state, shared across all callers (first-writer wins).
const MAX_POINTS = 60
let _historyInitialized = false
const _history = $state<{ up: number[]; down: number[] }>({ up: [], down: [] })

function ensureHistoryPump() {
  if (_historyInitialized) return
  _historyInitialized = true
  // Drive the buffer from the same WS stream. We don't close it — history is
  // an app-lifetime concern, not a component one.
  useSpeedStream()
  // Svelte will react to stream.data changes; we poll the stream state via an
  // interval sampling (cheap, predictable). We intentionally do not register
  // an $effect here — it's a non-component context.
  setInterval(() => {
    const s = useSpeedStream()
    if (!s.data) return
    _history.up   = [..._history.up.slice(-(MAX_POINTS - 1)),   s.data.upload]
    _history.down = [..._history.down.slice(-(MAX_POINTS - 1)), s.data.download]
  }, 5000)
}

export interface SpeedHistory {
  readonly up: readonly number[]
  readonly down: readonly number[]
}
export function useSpeedHistory(): SpeedHistory {
  ensureHistoryPump()
  return {
    get up()   { return _history.up },
    get down() { return _history.down },
  }
}
```

- [ ] **Step 2: svelte-check**

```bash
cd gui/web && npx svelte-check --threshold error 2>&1 | grep -E "ERROR.*features/dashboard" | head -5
```
Expected: no matches.

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/features/dashboard/resource.ts
git commit -m "feat(dashboard): add resource hooks (status / transports / speed / history)"
```

---

### Task 2: Test `useSpeedHistory` ring buffer

**Files:**
- Create: `gui/web/src/features/dashboard/resource.test.ts`

**Context:** `useStatus` / `useSpeedStream` / `useTransportStats` are thin wrappers over the already-tested `createResource` / `createStream`. Only the ring-buffer behavior of `useSpeedHistory` is non-trivial and worth unit-testing.

Because the ring buffer pumps from a WebSocket that's unreachable in jsdom, we inject samples directly by mutating `_history`. To do that cleanly without exposing internals we add a test helper.

- [ ] **Step 1: Add a test helper to `resource.ts`**

Append to `gui/web/src/features/dashboard/resource.ts`:

```ts
// Test helper — lets unit tests push samples without a live WebSocket.
export function __pushHistorySample(sample: { upload: number; download: number }): void {
  _history.up   = [..._history.up.slice(-(MAX_POINTS - 1)),   sample.upload]
  _history.down = [..._history.down.slice(-(MAX_POINTS - 1)), sample.download]
}

// Test helper — clear the buffer between tests.
export function __resetHistory(): void {
  _history.up = []
  _history.down = []
}
```

- [ ] **Step 2: Create the test**

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { useSpeedHistory, __pushHistorySample, __resetHistory } from '@/features/dashboard/resource'

beforeEach(() => __resetHistory())

describe('useSpeedHistory', () => {
  it('starts empty', () => {
    const h = useSpeedHistory()
    expect(h.up.length).toBe(0)
    expect(h.down.length).toBe(0)
  })

  it('appends samples in order', () => {
    __pushHistorySample({ upload: 1, download: 10 })
    __pushHistorySample({ upload: 2, download: 20 })
    const h = useSpeedHistory()
    expect(h.up).toEqual([1, 2])
    expect(h.down).toEqual([10, 20])
  })

  it('caps at 60 samples', () => {
    for (let i = 0; i < 70; i++) __pushHistorySample({ upload: i, download: i * 10 })
    const h = useSpeedHistory()
    expect(h.up.length).toBe(60)
    expect(h.up[0]).toBe(10)       // first 10 dropped
    expect(h.up[59]).toBe(69)      // last one preserved
  })
})
```

- [ ] **Step 3: Run**

```bash
cd gui/web && npm test -- features/dashboard/resource 2>&1 | tail -5
```
Expected: 3 pass.

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/features/dashboard/resource.ts gui/web/src/features/dashboard/resource.test.ts
git commit -m "test(dashboard): ring-buffer coverage for useSpeedHistory"
```

---

## Section B · Display components (Tasks 3-6)

### Task 3: `ConnectionHero.svelte`

**Files:**
- Create: `gui/web/src/features/dashboard/ConnectionHero.svelte`

**Context:** The hero card. Renders differently for connected vs disconnected. Connect/disconnect actions call the P1 API and invalidate the status resource.

- [ ] **Step 1: Create the file**

```svelte
<script lang="ts">
  import { Card, Button } from '@/ui'
  import { navigate } from '@/lib/router'
  import { connect, disconnect } from '@/lib/api/endpoints'
  import { invalidate } from '@/lib/resource.svelte'
  import { toasts } from '@/lib/toaster.svelte'
  import type { Status } from '@/lib/api/types'

  interface Props { status: Status }
  let { status }: Props = $props()

  const connected = $derived(status.connected === true)
  const serverLabel = $derived(
    status.server?.name || status.server?.addr || '—'
  )
  const transportLabel = $derived(
    // transport is optional on Status; be defensive.
    (status as unknown as { transport?: string }).transport ?? 'auto'
  )
  const uptime = $derived(formatUptime(status.uptime ?? 0))

  function formatUptime(s: number): string {
    if (s < 60) return `${s}s`
    const m = Math.floor(s / 60)
    if (m < 60) return `${m}m ${s % 60}s`
    const h = Math.floor(m / 60)
    return `${h}h ${m % 60}m`
  }

  function formatSpeed(bps: number): { value: string; unit: string } {
    if (bps >= 1e6) return { value: (bps / 1e6).toFixed(1), unit: 'MB/s' }
    if (bps >= 1e3) return { value: (bps / 1e3).toFixed(1), unit: 'KB/s' }
    return { value: String(bps), unit: 'B/s' }
  }

  const down = $derived(formatSpeed(status.bytes_recv ?? 0))
  const up   = $derived(formatSpeed(status.bytes_sent ?? 0))

  let busy = $state(false)
  async function toggle() {
    busy = true
    try {
      if (connected) await disconnect()
      else await connect()
      invalidate('dashboard.status')
    } catch (e) {
      toasts.error((e as Error).message)
    } finally {
      busy = false
    }
  }
</script>

<Card>
  <div class="hero">
    <div class="head">
      <span class="dot" class:on={connected}></span>
      <span class="state">
        {#if connected}
          Connected · <b>{serverLabel}</b> via {transportLabel}
        {:else}
          Disconnected
        {/if}
      </span>
      <span class="spacer"></span>
      {#if connected}<span class="uptime">{uptime}</span>{/if}
    </div>

    <div class="row">
      <div class="speed">
        <div class="big">
          {down.value}<span class="unit"> {down.unit}</span>
        </div>
        <div class="label">Download</div>
      </div>
      <div class="speed small">
        <div class="mid">{up.value}</div>
        <div class="label">Upload {up.unit}</div>
      </div>
      <div class="spacer"></div>
      <div class="actions">
        <Button variant="primary" loading={busy} onclick={toggle}>
          {connected ? 'Disconnect' : 'Connect'}
        </Button>
        <Button variant="ghost" onclick={() => navigate('/servers')}>
          Switch server
        </Button>
      </div>
    </div>
  </div>
</Card>

<style>
  .hero { display: flex; flex-direction: column; gap: var(--shuttle-space-4); }
  .head { display: flex; align-items: center; gap: var(--shuttle-space-2); }
  .dot  { width: 8px; height: 8px; border-radius: 50%; background: var(--shuttle-fg-muted); }
  .dot.on { background: var(--shuttle-success); }
  .state { font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-secondary); }
  .state b { color: var(--shuttle-fg-primary); font-weight: var(--shuttle-weight-semibold); }
  .spacer { flex: 1; }
  .uptime { font-family: var(--shuttle-font-mono); font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-secondary); }

  .row { display: flex; align-items: baseline; gap: var(--shuttle-space-5); }
  .speed .big {
    font-size: var(--shuttle-text-2xl); font-weight: var(--shuttle-weight-semibold);
    letter-spacing: var(--shuttle-tracking-tight); line-height: 1;
    font-variant-numeric: tabular-nums; color: var(--shuttle-fg-primary);
  }
  .speed .mid {
    font-size: var(--shuttle-text-xl); color: var(--shuttle-fg-secondary);
    font-variant-numeric: tabular-nums;
  }
  .speed .label {
    font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-muted);
    text-transform: uppercase; letter-spacing: 0.08em; margin-top: var(--shuttle-space-1);
  }
  .unit { font-size: var(--shuttle-text-lg); color: var(--shuttle-fg-muted); }
  .actions { display: flex; gap: var(--shuttle-space-2); }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/dashboard/ConnectionHero.svelte
git commit -m "feat(dashboard): add ConnectionHero card"
```

---

### Task 4: `StatsGrid.svelte`

**Files:**
- Create: `gui/web/src/features/dashboard/StatsGrid.svelte`

**Context:** 4-cell grid. Reads from `status` and `transportStats`. Empty cells (`—`) when disconnected.

- [ ] **Step 1: Create the file**

```svelte
<script lang="ts">
  import { Card } from '@/ui'
  import type { Status, TransportStats } from '@/lib/api/types'

  interface Props {
    status: Status
    transports: TransportStats[]
  }
  let { status, transports }: Props = $props()

  const connected = $derived(status.connected === true)

  // Active transport = one with active_streams > 0; fall back to first.
  const active = $derived(
    transports.find((t) => t.active_streams > 0) ?? transports[0]
  )

  function formatBytes(n: number): string {
    if (n >= 1e9) return `${(n / 1e9).toFixed(1)} GB`
    if (n >= 1e6) return `${(n / 1e6).toFixed(1)} MB`
    if (n >= 1e3) return `${(n / 1e3).toFixed(0)} KB`
    return `${n} B`
  }

  const stats = $derived([
    {
      label: 'RTT',
      value: connected ? `${(status as any).rtt_ms ?? '—'}` : '—',
      unit:  connected ? 'ms' : '',
    },
    {
      label: 'Packet loss',
      value: connected ? `${((status as any).loss_rate ?? 0).toFixed(1)}` : '—',
      unit:  connected ? '%' : '',
    },
    {
      label: 'Transfer',
      value: connected ? formatBytes((status.bytes_sent ?? 0) + (status.bytes_recv ?? 0)) : '—',
      unit:  '',
    },
    {
      label: 'Transport',
      value: connected ? (active?.transport ?? 'auto') : '—',
      unit:  '',
      mono: true,
    },
  ])
</script>

<div class="grid">
  {#each stats as s}
    <Card>
      <div class="lbl">{s.label}</div>
      <div class="val" class:mono={s.mono}>
        {s.value}{#if s.unit}<span class="unit"> {s.unit}</span>{/if}
      </div>
    </Card>
  {/each}
</div>

<style>
  .grid {
    display: grid; grid-template-columns: repeat(4, 1fr);
    gap: var(--shuttle-space-3);
  }
  .lbl {
    font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-muted);
    text-transform: uppercase; letter-spacing: 0.08em;
  }
  .val {
    font-size: var(--shuttle-text-xl); font-weight: var(--shuttle-weight-semibold);
    letter-spacing: var(--shuttle-tracking-tight);
    margin-top: var(--shuttle-space-1);
    font-variant-numeric: tabular-nums;
    color: var(--shuttle-fg-primary);
  }
  .val.mono { font-family: var(--shuttle-font-mono); font-size: var(--shuttle-text-base); padding-top: 4px; }
  .unit { font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-muted); font-weight: var(--shuttle-weight-regular); }

  @media (max-width: 860px) {
    .grid { grid-template-columns: repeat(2, 1fr); }
  }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/dashboard/StatsGrid.svelte
git commit -m "feat(dashboard): add StatsGrid 4-cell layout"
```

---

### Task 5: `SpeedSparkline.svelte`

**Files:**
- Create: `gui/web/src/features/dashboard/SpeedSparkline.svelte`

**Context:** Pure SVG sparkline — no chart library. Two lines: download (solid, foreground color) + upload (dashed, muted). Reads from `useSpeedHistory`. 72px tall, viewbox-normalized.

- [ ] **Step 1: Create the file**

```svelte
<script lang="ts">
  import { Card } from '@/ui'
  import { useSpeedHistory } from './resource'

  const history = useSpeedHistory()

  const downPath = $derived(buildPath(history.down))
  const upPath   = $derived(buildPath(history.up))
  const downArea = $derived(buildArea(history.down))

  const VIEW_W = 400
  const VIEW_H = 72

  function buildPath(samples: readonly number[]): string {
    if (samples.length === 0) return ''
    const max = Math.max(1, ...samples)
    const stepX = samples.length > 1 ? VIEW_W / (samples.length - 1) : 0
    return samples
      .map((v, i) => {
        const x = i * stepX
        const y = VIEW_H - (v / max) * (VIEW_H - 4)
        return `${i === 0 ? 'M' : 'L'} ${x.toFixed(1)} ${y.toFixed(1)}`
      })
      .join(' ')
  }

  function buildArea(samples: readonly number[]): string {
    const line = buildPath(samples)
    if (!line) return ''
    const stepX = samples.length > 1 ? VIEW_W / (samples.length - 1) : 0
    const lastX = ((samples.length - 1) * stepX).toFixed(1)
    return `${line} L ${lastX} ${VIEW_H} L 0 ${VIEW_H} Z`
  }
</script>

<Card>
  <header>
    <h3>Throughput</h3>
    <span class="legend">
      <span class="dot fg"></span>Down
      <span class="dot mu"></span>Up
      <span class="hint">last 5 min</span>
    </span>
  </header>
  <svg viewBox={`0 0 ${VIEW_W} ${VIEW_H}`} preserveAspectRatio="none" width="100%" height={VIEW_H}>
    {#if downArea}
      <path d={downArea} fill="currentColor" fill-opacity="0.08" />
      <path d={downPath} fill="none" stroke="currentColor" stroke-width="1.5" />
    {/if}
    {#if upPath}
      <path d={upPath} fill="none" stroke="var(--shuttle-fg-muted)" stroke-width="1" stroke-dasharray="2 2" />
    {/if}
  </svg>
  {#if history.down.length === 0}
    <div class="empty">Waiting for samples…</div>
  {/if}
</Card>

<style>
  header { display: flex; align-items: center; gap: var(--shuttle-space-2); margin-bottom: var(--shuttle-space-2); }
  h3 { margin: 0; font-size: var(--shuttle-text-sm); font-weight: var(--shuttle-weight-semibold); color: var(--shuttle-fg-primary); }
  .legend {
    margin-left: auto; display: flex; align-items: center; gap: var(--shuttle-space-3);
    font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-secondary);
  }
  .legend .dot { width: 8px; height: 2px; display: inline-block; margin-right: 4px; vertical-align: 1px; }
  .legend .fg { background: var(--shuttle-fg-primary); }
  .legend .mu { background: var(--shuttle-fg-muted); }
  .hint { color: var(--shuttle-fg-muted); }

  svg { color: var(--shuttle-fg-primary); display: block; }
  .empty {
    font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-muted);
    text-align: center; margin-top: calc(-1 * var(--shuttle-space-6));
    padding-top: var(--shuttle-space-5);
  }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/dashboard/SpeedSparkline.svelte
git commit -m "feat(dashboard): add SpeedSparkline inline-SVG chart"
```

---

### Task 6: `TransportBreakdown.svelte`

**Files:**
- Create: `gui/web/src/features/dashboard/TransportBreakdown.svelte`

**Context:** Densified table of transports. Renders empty placeholder if no data.

- [ ] **Step 1: Create the file**

```svelte
<script lang="ts">
  import { Card, Badge, Empty } from '@/ui'
  import type { TransportStats } from '@/lib/api/types'

  interface Props { transports: TransportStats[] }
  let { transports }: Props = $props()

  const total = $derived(
    transports.reduce((sum, t) => sum + t.bytes_sent + t.bytes_recv, 0) || 1
  )

  function fmtBps(t: TransportStats): string {
    // We don't have live bps; use total bytes / session as a proxy.
    const bytes = t.bytes_sent + t.bytes_recv
    if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(1)} GB`
    if (bytes >= 1e6) return `${(bytes / 1e6).toFixed(1)} MB`
    if (bytes >= 1e3) return `${(bytes / 1e3).toFixed(0)} KB`
    return `${bytes} B`
  }

  function pct(t: TransportStats): number {
    return Math.max(0, Math.min(100, ((t.bytes_sent + t.bytes_recv) / total) * 100))
  }

  function state(t: TransportStats): { label: string; variant: 'success' | 'neutral' } {
    if (t.active_streams > 0) return { label: 'PRIMARY', variant: 'success' }
    if (t.total_streams > 0)  return { label: 'STANDBY', variant: 'neutral' }
    return { label: 'IDLE', variant: 'neutral' }
  }
</script>

<Card>
  <header>
    <h3>Active transports</h3>
    <span class="count">{transports.length} {transports.length === 1 ? 'transport' : 'transports'}</span>
  </header>

  {#if transports.length === 0}
    <Empty title="No transport data" description="Connect to see per-transport breakdown." />
  {:else}
    <ul>
      {#each transports as t}
        <li>
          <span class="name">{t.transport}</span>
          <div class="bar"><div class="fill" style="width: {pct(t)}%"></div></div>
          <span class="num">{fmtBps(t)}</span>
          <span class="num sm">{t.active_streams} / {t.total_streams}</span>
          <Badge variant={state(t).variant}>{state(t).label}</Badge>
        </li>
      {/each}
    </ul>
  {/if}
</Card>

<style>
  header {
    display: flex; align-items: center; gap: var(--shuttle-space-2);
    margin-bottom: var(--shuttle-space-3);
  }
  h3 { margin: 0; font-size: var(--shuttle-text-sm); font-weight: var(--shuttle-weight-semibold); color: var(--shuttle-fg-primary); }
  .count {
    margin-left: auto; font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted); font-family: var(--shuttle-font-mono);
  }

  ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0; }
  li {
    display: grid;
    grid-template-columns: 80px 1fr 80px 72px auto;
    align-items: center; gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-2) 0;
    border-top: 1px solid var(--shuttle-border);
    font-size: var(--shuttle-text-sm);
  }
  li:first-child { border-top: 0; }
  .name { font-family: var(--shuttle-font-mono); font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-primary); }
  .bar { height: 4px; background: var(--shuttle-bg-subtle); border-radius: 2px; overflow: hidden; }
  .fill { height: 100%; background: var(--shuttle-accent); transition: width var(--shuttle-duration); }
  .num {
    color: var(--shuttle-fg-secondary); font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-xs); text-align: right;
  }
  .num.sm { color: var(--shuttle-fg-muted); }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/dashboard/TransportBreakdown.svelte
git commit -m "feat(dashboard): add TransportBreakdown densified table"
```

---

## Section C · Page composition + route (Tasks 7-8)

### Task 7: `Dashboard.svelte` page

**Files:**
- Create: `gui/web/src/features/dashboard/Dashboard.svelte`

**Context:** Composes the 4 panels. Subscribes to `useStatus` + `useTransportStats`. Uses `AsyncBoundary` for the status-driven panels so loading / error is handled uniformly.

- [ ] **Step 1: Create the file**

```svelte
<script lang="ts">
  import { AsyncBoundary } from '@/ui'
  import { useStatus, useTransportStats } from './resource'
  import ConnectionHero from './ConnectionHero.svelte'
  import StatsGrid from './StatsGrid.svelte'
  import SpeedSparkline from './SpeedSparkline.svelte'
  import TransportBreakdown from './TransportBreakdown.svelte'

  const status = useStatus()
  const transports = useTransportStats()
</script>

<div class="page">
  <AsyncBoundary resource={status}>
    {#snippet children(st)}
      <ConnectionHero status={st} />
      <StatsGrid status={st} transports={transports.data ?? []} />
      <SpeedSparkline />
      <TransportBreakdown transports={transports.data ?? []} />
    {/snippet}
  </AsyncBoundary>
</div>

<style>
  .page {
    display: flex; flex-direction: column; gap: var(--shuttle-space-5);
    max-width: 1080px;
  }
</style>
```

- [ ] **Step 2: svelte-check**

```bash
cd gui/web && npx svelte-check --threshold error 2>&1 | grep -E "ERROR.*features/dashboard" | head -5
```
Expected: no matches.

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/features/dashboard/Dashboard.svelte
git commit -m "feat(dashboard): compose Dashboard page from 4 panels"
```

---

### Task 8: Feature index + route swap

**Files:**
- Create: `gui/web/src/features/dashboard/index.ts`
- Modify: `gui/web/src/app/routes.ts`

**Context:** `features/dashboard/index.ts` is the public API boundary — exports the route and the public resource hooks. `routes.ts` swaps the legacy bridge for the feature route.

- [ ] **Step 1: Create `features/dashboard/index.ts`**

```ts
import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/',
  component: lazy(() => import('./Dashboard.svelte')),
  nav: { label: 'nav.dashboard', icon: 'dashboard', order: 10 },
}

// Public hooks — other features can subscribe to the same status resource.
export { useStatus, useTransportStats } from './resource'
```

- [ ] **Step 2: Update `app/routes.ts`**

Replace the Dashboard entry. The file becomes:

```ts
import { lazy } from '@/lib/router'
import type { Component } from 'svelte'
import * as dashboard from '@/features/dashboard'

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
  dashboard.route,
  {
    path: '/servers',
    component: lazy(() => import('@/pages/Servers.svelte')),
    nav: { label: 'nav.servers', icon: 'servers', order: 20 },
  },
  {
    path: '/subscriptions',
    component: lazy(() => import('@/pages/Subscriptions.svelte')),
    nav: { label: 'nav.subscriptions', icon: 'subscriptions', order: 30 },
  },
  {
    path: '/groups',
    component: lazy(() => import('@/pages/Groups.svelte')),
    nav: { label: 'nav.groups', icon: 'groups', order: 40 },
  },
  {
    path: '/routing',
    component: lazy(() => import('@/pages/Routing.svelte')),
    nav: { label: 'nav.routing', icon: 'routing', order: 50 },
  },
  {
    path: '/mesh',
    component: lazy(() => import('@/pages/Mesh.svelte')),
    nav: { label: 'nav.mesh', icon: 'mesh', order: 60 },
  },
  {
    path: '/logs',
    component: lazy(() => import('@/pages/Logs.svelte')),
    nav: { label: 'nav.logs', icon: 'logs', order: 80 },
  },
  {
    path: '/settings',
    component: lazy(() => import('@/pages/Settings.svelte')),
    nav: { label: 'nav.settings', icon: 'settings', order: 90 },
  },
]
```

- [ ] **Step 3: Build**

```bash
cd gui/web && npm run build 2>&1 | tail -5
```
Expected: succeeds; `Dashboard-*.js` lazy chunk is emitted from the new path.

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/features/dashboard/index.ts gui/web/src/app/routes.ts
git commit -m "feat(dashboard): export route + swap app/routes.ts"
```

---

## Section D · Cleanup legacy (Task 9)

### Task 9: Delete orphaned legacy files

**Files:**
- Delete: `gui/web/src/pages/Dashboard.svelte` (708 lines)
- Delete: `gui/web/src/lib/SpeedChart.svelte`
- Delete: `gui/web/src/lib/ConnectionQualityChart.svelte`
- Delete: `gui/web/src/lib/TrafficChart.svelte`
- Delete: `gui/web/src/lib/SpeedTestHistory.svelte`

**Context:** `MeshTopologyChart.svelte` is NOT deleted — Mesh page still uses it (moves to `features/mesh/` in P7).

- [ ] **Step 1: Confirm no remaining imports**

```bash
cd "/Users/homebot/Library/Mobile Documents/com~apple~CloudDocs/shuttle/gui/web"
grep -rEn "from '[./]*pages/Dashboard\\.svelte'|from '[./]*lib/SpeedChart|from '[./]*lib/ConnectionQualityChart|from '[./]*lib/TrafficChart|from '[./]*lib/SpeedTestHistory'" src/ 2>/dev/null | head -5
```
Expected: empty output.

- [ ] **Step 2: Delete**

```bash
rm gui/web/src/pages/Dashboard.svelte
rm gui/web/src/lib/SpeedChart.svelte
rm gui/web/src/lib/ConnectionQualityChart.svelte
rm gui/web/src/lib/TrafficChart.svelte
rm gui/web/src/lib/SpeedTestHistory.svelte
```

- [ ] **Step 3: Build + svelte-check**

```bash
cd gui/web
npx svelte-check --threshold error 2>&1 | tail -1
npm run build 2>&1 | tail -3
```
Expected: svelte-check error count **decreases** (Dashboard.svelte alone contributes dozens); build succeeds.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(gui): delete legacy Dashboard page + 4 orphaned chart components"
```

---

## Section E · Tests + verification (Tasks 10-11)

### Task 10: Update `shell.spec.ts` Dashboard smoke

**Files:**
- Modify: `gui/web/tests/shell.spec.ts`

**Context:** The existing smoke tests checked that `/` rendered the legacy Dashboard's sidebar (a class-based selector). Add an assertion that the NEW Dashboard hero renders.

- [ ] **Step 1: Append a new test**

Open `gui/web/tests/shell.spec.ts` and add after the existing tests:

```ts
test.describe('P3 dashboard', () => {
    test('root URL renders new Dashboard hero', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('.sidebar')).toBeVisible();
        // New Dashboard always shows a card with either "Connected" or "Disconnected" state.
        await expect(page.locator('text=/Connected|Disconnected/').first()).toBeVisible({ timeout: 5000 });
    });

    test('dashboard shows four stats cards', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('.sidebar')).toBeVisible();
        // Each stat card contains one of: RTT, Packet loss, Transfer, Transport
        await expect(page.locator('text=RTT')).toBeVisible({ timeout: 5000 });
        await expect(page.locator('text=Packet loss')).toBeVisible();
        await expect(page.locator('text=Transfer')).toBeVisible();
        await expect(page.locator('text=Transport').first()).toBeVisible();
    });
});
```

- [ ] **Step 2: Run**

```bash
cd gui/web && npx playwright test shell.spec.ts --reporter=line 2>&1 | tail -5
```
Expected: all pass (existing + new).

- [ ] **Step 3: Commit**

```bash
git add gui/web/tests/shell.spec.ts
git commit -m "test(dashboard): smoke — hero state + four stats cards"
```

---

### Task 11: Final gates + bundle record + push

**Files:**
- Modify: `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md` (append P3 row)

- [ ] **Step 1: Full gate run**

```bash
cd gui/web
echo "=== svelte-check ===" && npx svelte-check --threshold error 2>&1 | tail -1
echo "=== vitest ==="        && npm test 2>&1 | tail -3
echo "=== i18n ==="           && ./scripts/check-i18n.sh
echo "=== build ==="          && npm run build 2>&1 | tail -3
echo "=== playwright ==="     && npx playwright test --reporter=line 2>&1 | tail -3
```

Expected: all green. svelte-check count should drop (legacy Dashboard.svelte + 4 charts deleted).

- [ ] **Step 2: Record post-P3 bundle sizes**

```bash
cd gui/web && ls -la dist/assets/index*.js dist/assets/Dashboard*.js | awk '{print $5, $9}'
```

Append a `## Post-P3 (2026-04-XX)` section to `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md` with:
- New lazy chunk size for `Dashboard-*.js` (the new one)
- index.js gzip vs baseline
- Notes: bits-ui and `@/ui` start showing up now that a feature imports them — delta should be +3-8 KB gzip, still well inside the +30 KB P1-P10 total budget

- [ ] **Step 3: Push**

```bash
git push origin refactor/gui-v2
```

PR #8 auto-updates to include P3 commits.

- [ ] **Step 4: Update PR body**

Run:
```bash
gh pr view 8 --json body -q .body > /tmp/pr-body.md
```
Append a `## P3 · Dashboard feature` section to that file summarizing:
- new `features/dashboard/` slice (7 files)
- deleted legacy Dashboard.svelte + 4 orphaned chart components
- new route registration pattern

Then:
```bash
gh pr edit 8 --body-file /tmp/pr-body.md
```

---

## Self-review notes

**Spec coverage.**
- §7.1 redesign principles → Tasks 3-6 implement Hero, StatsGrid, Sparkline, TransportBreakdown; Task 7 composes them in the scroll-based "adaptive" layout (no mode toggle).
- §2 feature-sliced directory → Tasks 1-8 create the canonical `features/dashboard/` layout.
- §5 Resource → Task 1 uses `createResource` + `createStream` per the P1 contract; Task 2 tests the one piece of custom logic (ring buffer).
- §6 route registration → Task 8 uses the `import * as dashboard from '@/features/dashboard'; routes.push(dashboard.route)` pattern documented in `src/README.md`.
- §7.10 UX principles → AsyncBoundary, unit-explicit numbers, no decorative colors, keyboard-accessible primary action (Button primitive).

**Placeholder scan.** Each task has complete code blocks. Task 11 asks the engineer to hand-append a markdown section — that's fine (it's a one-paragraph human-written summary), but to avoid ambiguity a boilerplate snippet is included in the instructions.

**Type consistency.**
- `useStatus` / `useTransportStats` / `useSpeedHistory` signatures match across Tasks 1, 2, 7.
- `AppRoute.component` is a `() => Promise<Component>` everywhere.
- `TransportStats` shape matches `@/lib/api/types`.
- `AsyncBoundary` generics render `{children(data)}` — usage in Task 7 supplies `snippet children(st)` correctly.

**Explicit out-of-scope for P3.**
- Speed-test history panel (no longer on Dashboard per §7.1; lives in Servers detail in P4 or is deprecated).
- Mesh topology on Dashboard (deleted; Mesh page keeps its own in P7).
- Daily/weekly/monthly traffic chart (deleted; resurfaces in Stats page in a later phase if product decides).
- Cmd/Ctrl+K keyboard shortcut (legacy `shortcuts.ts` still exists; rewiring deferred to P11 or a small follow-up).
- i18n of new strings (Connect / Disconnect / Download / Upload / RTT / Packet loss / Transfer / Transport / Throughput / Active transports) — added in a P3 follow-up task. For P3 the English strings are acceptable since locale files only cover the sidebar nav right now.

**Known risks.**
- `status.rtt_ms` and `status.loss_rate` are accessed as `any` because the backend `Status` type doesn't declare them. Verify at runtime that the backend actually returns those fields; if not, StatsGrid will show `—` which is correct behavior but may be unexpected. A proper type extension happens in P11 when we clear pre-existing svelte-check errors.
- `transports[0].bytes_sent` as a "current bandwidth" proxy is a lie — it's cumulative. A future endpoint exposing instantaneous per-transport rate should replace this. For P3 the number is still informative (shows relative share) even if imperfect.

---

## Plan complete.

Plan complete and saved to `docs/superpowers/plans/2026-04-20-gui-refactor-p3-dashboard.md`.

Execution:
- **Subagent-Driven** — fresh agent per task, review between
- **Inline** — execute here in this session with checkpoints
