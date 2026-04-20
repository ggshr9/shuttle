# GUI Refactor P4 · Servers Feature Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the legacy `pages/Servers.svelte` bridge with a redesigned feature slice per spec §7.2 — a dense 48-px-row table with in-place expand, multi-select + batch operations (delete, speedtest, auto-select), and bits-ui Dialogs for Add / Import / Delete-confirm. Active server is indicated via a 2-px left border, not a dedicated column.

**Architecture:** Feature-sliced under `features/servers/`. `resource.svelte.ts` exposes `useServers()` + mutation helpers (add / delete / setActive / speedtest / autoSelect). `ServerTable` owns selection state (Set of addresses) and renders `ServerRow` per entry with an inline `ServerRowExpanded` detail row when opened. Selection drives a top-floating `SelectionBar` for batch actions. Dialogs follow the P1 `ui/Dialog` pattern with `bind:open`.

**Tech Stack:** Svelte 5 runes · `@/lib/resource` · `@/lib/api` · `@/ui` (Card, Button, Input, Badge, Icon, Dialog, Empty, AsyncBoundary, Switch).

**Spec reference:** `docs/superpowers/specs/2026-04-19-gui-refactor-design.md` — §7.2 Servers page redesign.

**Branch:** Continue on `refactor/gui-v2`. P4 lands as new commits on the existing PR #8 (or a split PR once #8 merges).

---

## Design reference

Page layout top-to-bottom:

1. **Header row** — `Servers` title, subtitle count, right-aligned buttons: `Auto-select`, `Import`, `+ Add server`.
2. **Selection bar** (only when ≥1 row selected) — sticky horizontal strip at top showing `{N} selected · [Speed test]  [Delete]  [Cancel]`.
3. **Dense table** — column widths fixed, no wrapping:
   - Active indicator (2 px left border on row itself — no column)
   - Checkbox (32 px)
   - Status dot (16 px — green for tested-ok, red for failed, grey unknown)
   - Name (flex-grow 2)
   - Address (flex-grow 3, monospace)
   - Latency (80 px, tabular-nums, `— ms` when untested)
   - Protocol (80 px, inferred from address scheme: `shuttle`, `ss`, `vmess`, `—`)
   - Actions (per-row: expand ▸ / ▾, switch-to, delete)
4. **Expanded detail row** (when row clicked / expanded) — inline below the row, indented:
   - Edit name / password fields (`ui/Input`)
   - SNI / advanced fields
   - Per-server speedtest history (6 most recent)
   - Manual `Set as active` button

Click-to-expand is the primary detail-edit path; no modal for editing. Add uses `AddServerDialog`, not inline (first-time UX).

All colors from `--shuttle-*` tokens. No hardcoded hex / px.

---

## File structure

```
gui/web/src/features/servers/
├─ index.ts                    route + public API
├─ resource.svelte.ts          useServers + mutations
├─ ServersPage.svelte          header + selection-bar + table + dialogs
├─ ServerTable.svelte          table shell + selection state
├─ ServerRow.svelte            one row (always-visible summary)
├─ ServerRowExpanded.svelte    detail panel (shown on expand)
├─ AddServerDialog.svelte      ui/Dialog with Name/Address/Password/SNI fields
├─ ImportDialog.svelte         ui/Dialog, paste-area auto-detect
└─ DeleteConfirm.svelte        ui/Dialog with Cancel + Delete primary
```

9 files. Each ≤ ~150 lines.

Deletions after the feature ships:
- `pages/Servers.svelte` (704 lines)

---

## Conventions

- Relative paths from repo root.
- Every task ends with a commit.
- Tests live next to code: `<Name>.test.ts`.
- Follow the ConnectionHero pattern for mutations: `try { await api.X(); invalidate('servers.list') } catch (e) { toasts.error(...) }`.

---

## Section A · Resource layer (Tasks 1-2)

### Task 1: `features/servers/resource.svelte.ts`

**Files:**
- Create: `gui/web/src/features/servers/resource.svelte.ts`

**Context:** Centralizes all data concerns for the feature. `useServers()` returns the authoritative list (polled every 10s — active changes are rare). `useSpeedtestResults()` stores a transient Map<addr, SpeedtestResult> that the table uses to render Latency / Status columns; it's NOT from the backend, it's populated by the test action.

- [ ] **Step 1: Create the file**

```ts
import { createResource, invalidate, type Resource } from '@/lib/resource.svelte'
import {
  getServers,
  addServer as apiAddServer,
  setActiveServer as apiSetActiveServer,
  deleteServer as apiDeleteServer,
  speedtest as apiSpeedtest,
  autoSelectServer as apiAutoSelect,
  importConfig as apiImport,
} from '@/lib/api/endpoints'
import type { Server, ServersResponse, SpeedtestResult, AutoSelectResult, ImportResult } from '@/lib/api/types'
import { toasts } from '@/lib/toaster.svelte'

const LIST_KEY = 'servers.list'

// ── Read ─────────────────────────────────────────────────────
export function useServers(): Resource<ServersResponse> {
  return createResource(
    LIST_KEY,
    getServers,
    { poll: 10_000, initial: { active: { addr: '' }, servers: [] } },
  )
}

// ── Mutations ────────────────────────────────────────────────
export async function addServer(srv: Server): Promise<void> {
  try {
    await apiAddServer(srv)
    invalidate(LIST_KEY)
    toasts.success(`Added ${srv.name || srv.addr}`)
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}

export async function setActive(srv: Server): Promise<void> {
  try {
    await apiSetActiveServer(srv)
    invalidate(LIST_KEY)
    invalidate('dashboard.status')
    toasts.success(`Switched to ${srv.name || srv.addr}`)
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}

export async function removeServer(addr: string): Promise<void> {
  try {
    await apiDeleteServer(addr)
    invalidate(LIST_KEY)
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}

export async function removeMany(addrs: string[]): Promise<void> {
  for (const a of addrs) await apiDeleteServer(a)
  invalidate(LIST_KEY)
  toasts.success(`Deleted ${addrs.length} ${addrs.length === 1 ? 'server' : 'servers'}`)
}

export async function autoSelect(): Promise<AutoSelectResult | null> {
  try {
    const r = await apiAutoSelect()
    invalidate(LIST_KEY)
    invalidate('dashboard.status')
    toasts.success(`Auto-selected ${r.server.name || r.server.addr} (${r.latency} ms)`)
    return r
  } catch (e) {
    toasts.error((e as Error).message)
    return null
  }
}

export async function importServers(data: string): Promise<ImportResult | null> {
  try {
    const r = await apiImport(data)
    invalidate(LIST_KEY)
    if (r.error) {
      toasts.error(r.error)
    } else if (r.added > 0) {
      toasts.success(`Imported ${r.added} of ${r.total} servers`)
    } else {
      toasts.info('No new servers imported')
    }
    return r
  } catch (e) {
    toasts.error((e as Error).message)
    return null
  }
}

// ── Speedtest results — transient, in-memory only ────────────
const results = $state<{ map: Record<string, SpeedtestResult> }>({ map: {} })

export function useSpeedtestResult(addr: string): SpeedtestResult | undefined {
  return results.map[addr]
}

export function getAllResults(): Record<string, SpeedtestResult> {
  return results.map
}

export async function runSpeedtest(addrs: string[]): Promise<void> {
  if (addrs.length === 0) return
  try {
    const rs = await apiSpeedtest(addrs)
    for (const r of rs) results.map[r.server_addr] = r
    toasts.success(`Tested ${rs.length} ${rs.length === 1 ? 'server' : 'servers'}`)
  } catch (e) {
    toasts.error((e as Error).message)
  }
}

export function __resetResults(): void {
  results.map = {}
}
```

- [ ] **Step 2: svelte-check**

```bash
cd gui/web && npx svelte-check --threshold error 2>&1 | grep -E "ERROR.*features/servers" | head -5
```
Expected: no matches.

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/features/servers/resource.svelte.ts
git commit -m "feat(servers): resource layer (useServers + mutations + speedtest store)"
```

---

### Task 2: Resource test

**Files:**
- Create: `gui/web/src/features/servers/resource.test.ts`

**Context:** Focus on the non-trivial piece — `runSpeedtest` integrates with the toast store and the transient results map. Mock `apiSpeedtest` and verify results are stored per-addr.

- [ ] **Step 1: Create test**

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { __resetResults, getAllResults, useSpeedtestResult, runSpeedtest } from '@/features/servers/resource.svelte'

vi.mock('@/lib/api/endpoints', () => ({
  getServers:     vi.fn(async () => ({ active: { addr: '' }, servers: [] })),
  addServer:      vi.fn(async () => undefined),
  setActiveServer: vi.fn(async () => undefined),
  deleteServer:   vi.fn(async () => undefined),
  autoSelectServer: vi.fn(async () => ({ server: { addr: '' }, latency: 0 })),
  importConfig:   vi.fn(async () => ({ added: 0, total: 0 })),
  speedtest: vi.fn(async (addrs: string[]) => addrs.map((a, i) => ({
    server_addr: a,
    available: true,
    latency: 50 + i,
  }))),
}))

beforeEach(() => __resetResults())

describe('runSpeedtest', () => {
  it('stores a result per tested address', async () => {
    await runSpeedtest(['a:1', 'b:2'])
    expect(useSpeedtestResult('a:1')?.latency).toBe(50)
    expect(useSpeedtestResult('b:2')?.latency).toBe(51)
    expect(Object.keys(getAllResults())).toHaveLength(2)
  })

  it('is a no-op for empty input', async () => {
    await runSpeedtest([])
    expect(Object.keys(getAllResults())).toHaveLength(0)
  })
})
```

- [ ] **Step 2: Run**

```bash
cd gui/web && npm test -- features/servers/resource 2>&1 | tail -5
```
Expected: 2 pass.

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/features/servers/resource.test.ts
git commit -m "test(servers): speedtest results store per-addr"
```

---

## Section B · Dialogs (Tasks 3-5)

### Task 3: `AddServerDialog.svelte`

**Files:**
- Create: `gui/web/src/features/servers/AddServerDialog.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { Dialog, Input, Button } from '@/ui'
  import { addServer } from './resource.svelte'

  interface Props {
    open: boolean
  }
  let { open = $bindable(false) }: Props = $props()

  let name = $state('')
  let addr = $state('')
  let password = $state('')
  let sni = $state('')
  let submitting = $state(false)

  const canSubmit = $derived(addr.trim().length > 0 && password.length > 0)

  async function submit() {
    if (!canSubmit) return
    submitting = true
    try {
      await addServer({ name: name.trim() || undefined, addr: addr.trim(), password, sni: sni.trim() || undefined })
      name = ''; addr = ''; password = ''; sni = ''
      open = false
    } finally {
      submitting = false
    }
  }
</script>

<Dialog bind:open title="Add server" description="Enter server details. Address and password are required.">
  <div class="fields">
    <Input label="Name" placeholder="sg-hk-02" bind:value={name} />
    <Input label="Address" placeholder="example.com:443" bind:value={addr} />
    <Input label="Password" type="password" bind:value={password} />
    <Input label="SNI (optional)" placeholder="example.com" bind:value={sni} />
  </div>

  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>Cancel</Button>
    <Button variant="primary" disabled={!canSubmit} loading={submitting} onclick={submit}>
      Add
    </Button>
  {/snippet}
</Dialog>

<style>
  .fields { display: flex; flex-direction: column; gap: var(--shuttle-space-3); }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/servers/AddServerDialog.svelte
git commit -m "feat(servers): AddServerDialog"
```

---

### Task 4: `ImportDialog.svelte`

**Files:**
- Create: `gui/web/src/features/servers/ImportDialog.svelte`

**Context:** Paste any supported format (Clash YAML, Base64-SIP008, `shuttle://` URI). Backend auto-detects; UI only provides the textarea + format hint.

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { Dialog, Button } from '@/ui'
  import { importServers } from './resource.svelte'

  interface Props {
    open: boolean
  }
  let { open = $bindable(false) }: Props = $props()

  let data = $state('')
  let submitting = $state(false)

  async function submit() {
    if (!data.trim()) return
    submitting = true
    try {
      const r = await importServers(data)
      if (r && (r.added > 0 || !r.error)) {
        data = ''
        open = false
      }
    } finally {
      submitting = false
    }
  }
</script>

<Dialog bind:open title="Import servers" description="Paste a Clash YAML, Base64 SIP-008 subscription, or shuttle:// URI.">
  <textarea
    class="ta"
    placeholder={'shuttle://...  or  proxies:\\n  - name: ...  or  base64 SIP-008'}
    bind:value={data}
  ></textarea>

  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>Cancel</Button>
    <Button variant="primary" disabled={!data.trim()} loading={submitting} onclick={submit}>
      Import
    </Button>
  {/snippet}
</Dialog>

<style>
  .ta {
    width: 100%; min-height: 160px;
    padding: var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    color: var(--shuttle-fg-primary);
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-sm);
    resize: vertical;
    outline: none;
  }
  .ta:focus { border-color: var(--shuttle-border-strong); }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/servers/ImportDialog.svelte
git commit -m "feat(servers): ImportDialog with format-agnostic textarea"
```

---

### Task 5: `DeleteConfirm.svelte`

**Files:**
- Create: `gui/web/src/features/servers/DeleteConfirm.svelte`

**Context:** Reusable confirmation for both single-delete and batch-delete. Parent passes `count` and an `onConfirm` callback.

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { Dialog, Button } from '@/ui'

  interface Props {
    open: boolean
    count: number
    onConfirm: () => Promise<void> | void
  }
  let { open = $bindable(false), count, onConfirm }: Props = $props()

  let busy = $state(false)

  async function confirm() {
    busy = true
    try {
      await onConfirm()
      open = false
    } finally {
      busy = false
    }
  }

  const title = $derived(count === 1 ? 'Delete server?' : `Delete ${count} servers?`)
  const description = $derived('This cannot be undone.')
</script>

<Dialog bind:open {title} {description}>
  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>Cancel</Button>
    <Button variant="danger" loading={busy} onclick={confirm}>Delete</Button>
  {/snippet}
</Dialog>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/servers/DeleteConfirm.svelte
git commit -m "feat(servers): DeleteConfirm shared dialog"
```

---

## Section C · Table + rows (Tasks 6-8)

### Task 6: `ServerRowExpanded.svelte`

**Files:**
- Create: `gui/web/src/features/servers/ServerRowExpanded.svelte`

**Context:** Detail panel shown below a row when expanded. Edits are applied on blur (no "Save" button — simpler UX; mirrors the plan §7.8 unsaved-bar pattern, but that's Settings-specific, so inline apply here).

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { Input, Button, StatRow } from '@/ui'
  import { setActive, useSpeedtestResult, runSpeedtest } from './resource.svelte'
  import type { Server } from '@/lib/api/types'

  interface Props {
    server: Server
    isActive: boolean
  }
  let { server, isActive }: Props = $props()

  // Local drafts — pushed on blur.
  let name = $state(server.name ?? '')
  let sni = $state(server.sni ?? '')

  const result = $derived(useSpeedtestResult(server.addr))

  async function saveField() {
    // Mutation: re-add the server with updated fields. Backend dedup by addr
    // so this acts as upsert. For P4 we use the existing addServer endpoint;
    // a dedicated updateServer endpoint can come later.
    // In P4 we defer actual field edit to a future endpoint — just mark the
    // intent; changing name/sni doesn't break the server record key (addr).
    // No-op for now; the inputs are visible and editable but the change
    // travels via the regular setActive (which carries the updated fields).
  }

  async function test() {
    await runSpeedtest([server.addr])
  }

  async function makeActive() {
    await setActive({ ...server, name: name || undefined, sni: sni || undefined })
  }
</script>

<div class="pane">
  <div class="fields">
    <Input label="Name" bind:value={name} onchange={saveField} />
    <Input label="Server address" value={server.addr} disabled />
    <Input label="SNI" bind:value={sni} onchange={saveField} />
  </div>

  <div class="side">
    {#if result}
      <StatRow label="Latency" value={`${result.latency} ms`} mono />
      <StatRow label="Available" value={result.available ? 'yes' : 'no'} />
    {:else}
      <p class="hint">Not yet tested.</p>
    {/if}
    <div class="actions">
      <Button size="sm" variant="secondary" onclick={test}>Speed test</Button>
      {#if !isActive}
        <Button size="sm" variant="primary" onclick={makeActive}>Set as active</Button>
      {/if}
    </div>
  </div>
</div>

<style>
  .pane {
    display: grid; grid-template-columns: 1fr 240px;
    gap: var(--shuttle-space-5);
    padding: var(--shuttle-space-4) var(--shuttle-space-4) var(--shuttle-space-4) var(--shuttle-space-6);
    background: var(--shuttle-bg-subtle);
    border-top: 1px solid var(--shuttle-border);
  }
  .fields { display: flex; flex-direction: column; gap: var(--shuttle-space-3); }
  .side { display: flex; flex-direction: column; gap: var(--shuttle-space-2); }
  .hint { font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-muted); margin: 0; }
  .actions { margin-top: var(--shuttle-space-3); display: flex; gap: var(--shuttle-space-2); }
</style>
```

Note: the `saveField` no-op is a deliberate limitation of P4 — the backend currently has no `updateServer` endpoint distinct from `addServer`, and name/sni changes require re-registration. P5+ will add an edit endpoint. Leaving the visual affordance for familiarity.

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/servers/ServerRowExpanded.svelte
git commit -m "feat(servers): ServerRowExpanded detail pane"
```

---

### Task 7: `ServerRow.svelte`

**Files:**
- Create: `gui/web/src/features/servers/ServerRow.svelte`

**Context:** One dense summary row. Callback props for selection + expand toggle; the parent table owns the sets.

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { Button, Icon, Badge } from '@/ui'
  import { useSpeedtestResult, setActive, removeServer } from './resource.svelte'
  import type { Server } from '@/lib/api/types'

  interface Props {
    server: Server
    isActive: boolean
    selected: boolean
    expanded: boolean
    onSelectedChange: (v: boolean) => void
    onExpandedToggle: () => void
    onDelete: () => void
  }

  let {
    server, isActive, selected, expanded,
    onSelectedChange, onExpandedToggle, onDelete,
  }: Props = $props()

  const result = $derived(useSpeedtestResult(server.addr))

  // Simple protocol inference from the addr scheme prefix; backend may not
  // give us a dedicated field.
  const protocol = $derived(inferProtocol(server.addr))

  function inferProtocol(addr: string): string {
    if (addr.startsWith('ss://'))      return 'ss'
    if (addr.startsWith('vmess://'))   return 'vmess'
    if (addr.startsWith('trojan://'))  return 'trojan'
    if (addr.startsWith('shuttle://')) return 'shuttle'
    // Plain host:port defaults to shuttle.
    if (/^[^/:]+:\d+$/.test(addr))     return 'shuttle'
    return '—'
  }

  const statusClass = $derived(
    !result ? 'unknown' : result.available ? 'ok' : 'bad'
  )
</script>

<div class="row" class:active={isActive} class:selected>
  <span class="check">
    <input
      type="checkbox"
      checked={selected}
      onchange={(e) => onSelectedChange((e.target as HTMLInputElement).checked)}
      aria-label={`Select ${server.name || server.addr}`}
    />
  </span>
  <span class={`status ${statusClass}`} aria-label={statusClass}></span>
  <span class="name">{server.name || '—'}</span>
  <span class="addr">{server.addr}</span>
  <span class="lat">
    {#if result}{result.latency} ms{:else}— ms{/if}
  </span>
  <span class="proto">
    <Badge>{protocol}</Badge>
  </span>
  <span class="actions">
    <Button size="sm" variant="ghost" onclick={onExpandedToggle} class={`expand ${expanded ? 'open' : ''}`}>
      <Icon name={expanded ? 'chevronDown' : 'chevronRight'} size={14} />
    </Button>
    {#if !isActive}
      <Button size="sm" variant="ghost" onclick={() => setActive(server)} aria-label="Set active">
        <Icon name="check" size={14} />
      </Button>
    {/if}
    <Button size="sm" variant="ghost" onclick={onDelete} aria-label="Delete">
      <Icon name="trash" size={14} />
    </Button>
  </span>
</div>

<style>
  .row {
    display: grid;
    grid-template-columns: 32px 16px 2fr 3fr 80px 80px auto;
    align-items: center;
    gap: var(--shuttle-space-3);
    height: 48px;
    padding: 0 var(--shuttle-space-4);
    border-top: 1px solid var(--shuttle-border);
    border-left: 2px solid transparent;
    font-size: var(--shuttle-text-sm);
  }
  .row:first-child { border-top: 0; }
  .row.active   { border-left-color: var(--shuttle-accent); }
  .row.selected { background: var(--shuttle-bg-subtle); }

  .check { display: flex; }
  .check input { cursor: pointer; }

  .status {
    width: 8px; height: 8px; border-radius: 50%;
    background: var(--shuttle-fg-muted);
  }
  .status.ok  { background: var(--shuttle-success); }
  .status.bad { background: var(--shuttle-danger); }

  .name { font-weight: var(--shuttle-weight-medium); color: var(--shuttle-fg-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .addr { font-family: var(--shuttle-font-mono); font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .lat  { font-family: var(--shuttle-font-mono); color: var(--shuttle-fg-secondary); font-variant-numeric: tabular-nums; text-align: right; }
  .proto { }
  .actions { display: flex; gap: 2px; justify-content: flex-end; }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/servers/ServerRow.svelte
git commit -m "feat(servers): ServerRow — dense 48px summary"
```

---

### Task 8: `ServerTable.svelte`

**Files:**
- Create: `gui/web/src/features/servers/ServerTable.svelte`

**Context:** Owns selection (`Set<string>` of addresses), owns which row is expanded (`Set<string>` so multiple can expand). Exposes the selection upward so `ServersPage` can render its `SelectionBar`.

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { Empty, Card } from '@/ui'
  import ServerRow from './ServerRow.svelte'
  import ServerRowExpanded from './ServerRowExpanded.svelte'
  import type { Server } from '@/lib/api/types'

  interface Props {
    servers: Server[]
    activeAddr: string
    selected: Set<string>
    onSelectedChange: (next: Set<string>) => void
    onDelete: (addr: string) => void
  }

  let { servers, activeAddr, selected, onSelectedChange, onDelete }: Props = $props()

  const expanded = $state<Set<string>>(new Set())

  function toggleSelect(addr: string, v: boolean) {
    const next = new Set(selected)
    if (v) next.add(addr)
    else next.delete(addr)
    onSelectedChange(next)
  }

  function toggleExpanded(addr: string) {
    if (expanded.has(addr)) expanded.delete(addr)
    else expanded.add(addr)
  }

  function toggleAll(v: boolean) {
    onSelectedChange(v ? new Set(servers.map((s) => s.addr)) : new Set())
  }

  const allSelected = $derived(servers.length > 0 && selected.size === servers.length)
  const someSelected = $derived(selected.size > 0 && !allSelected)
</script>

{#if servers.length === 0}
  <Card><Empty icon="servers" title="No servers" description="Add one or import a subscription to get started." /></Card>
{:else}
  <div class="table">
    <div class="header">
      <span class="check">
        <input
          type="checkbox"
          checked={allSelected}
          indeterminate={someSelected}
          onchange={(e) => toggleAll((e.target as HTMLInputElement).checked)}
          aria-label="Select all"
        />
      </span>
      <span></span>
      <span>Name</span>
      <span>Address</span>
      <span class="lat">Latency</span>
      <span>Protocol</span>
      <span></span>
    </div>

    {#each servers as s (s.addr)}
      <ServerRow
        server={s}
        isActive={s.addr === activeAddr}
        selected={selected.has(s.addr)}
        expanded={expanded.has(s.addr)}
        onSelectedChange={(v) => toggleSelect(s.addr, v)}
        onExpandedToggle={() => toggleExpanded(s.addr)}
        onDelete={() => onDelete(s.addr)}
      />
      {#if expanded.has(s.addr)}
        <ServerRowExpanded server={s} isActive={s.addr === activeAddr} />
      {/if}
    {/each}
  </div>
{/if}

<style>
  .table {
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    overflow: hidden;
  }
  .header {
    display: grid;
    grid-template-columns: 32px 16px 2fr 3fr 80px 80px auto;
    align-items: center;
    gap: var(--shuttle-space-3);
    height: 36px;
    padding: 0 var(--shuttle-space-4);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    background: var(--shuttle-bg-subtle);
    border-bottom: 1px solid var(--shuttle-border);
  }
  .header .lat { text-align: right; }
  .check input { cursor: pointer; }
</style>
```

- [ ] **Step 2: svelte-check**

```bash
cd gui/web && npx svelte-check --threshold error 2>&1 | grep -E "ERROR.*features/servers" | head -5
```
Expected: no matches.

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/features/servers/ServerTable.svelte
git commit -m "feat(servers): ServerTable with selection + expand state"
```

---

## Section D · Page + route (Tasks 9-10)

### Task 9: `ServersPage.svelte` + selection bar

**Files:**
- Create: `gui/web/src/features/servers/ServersPage.svelte`

**Context:** Composes everything. Owns selection state (passed down to ServerTable). Renders SelectionBar above the table when ≥1 row is selected.

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { AsyncBoundary, Button, Icon, Section } from '@/ui'
  import { useServers, removeServer, removeMany, autoSelect, runSpeedtest } from './resource.svelte'
  import ServerTable from './ServerTable.svelte'
  import AddServerDialog from './AddServerDialog.svelte'
  import ImportDialog from './ImportDialog.svelte'
  import DeleteConfirm from './DeleteConfirm.svelte'

  const res = useServers()

  let selected = $state<Set<string>>(new Set())
  let addOpen = $state(false)
  let importOpen = $state(false)
  let deleteOpen = $state(false)
  let pendingDelete = $state<string[]>([])  // single or many

  function openSingleDelete(addr: string) {
    pendingDelete = [addr]
    deleteOpen = true
  }

  function openBatchDelete() {
    pendingDelete = Array.from(selected)
    deleteOpen = true
  }

  async function confirmDelete() {
    if (pendingDelete.length === 1) {
      await removeServer(pendingDelete[0])
    } else {
      await removeMany(pendingDelete)
    }
    selected = new Set()
  }

  async function testSelected() {
    await runSpeedtest(Array.from(selected))
  }

  async function testAll() {
    if (!res.data) return
    await runSpeedtest(res.data.servers.map((s) => s.addr))
  }
</script>

<Section
  title="Servers"
  description={res.data ? `${res.data.servers.length} configured` : undefined}
>
  {#snippet actions()}
    <Button variant="ghost" onclick={testAll}>Test all</Button>
    <Button variant="ghost" onclick={() => autoSelect()}>
      <Icon name="check" size={14} /> Auto-select
    </Button>
    <Button variant="ghost" onclick={() => (importOpen = true)}>
      Import
    </Button>
    <Button variant="primary" onclick={() => (addOpen = true)}>
      <Icon name="plus" size={14} /> Add server
    </Button>
  {/snippet}

  {#if selected.size > 0}
    <div class="sel-bar">
      <span class="count">{selected.size} selected</span>
      <Button size="sm" variant="secondary" onclick={testSelected}>Speed test</Button>
      <Button size="sm" variant="danger"    onclick={openBatchDelete}>Delete</Button>
      <Button size="sm" variant="ghost"     onclick={() => (selected = new Set())}>Cancel</Button>
    </div>
  {/if}

  <AsyncBoundary resource={res}>
    {#snippet children(list)}
      <ServerTable
        servers={list.servers}
        activeAddr={list.active.addr}
        {selected}
        onSelectedChange={(s) => (selected = s)}
        onDelete={openSingleDelete}
      />
    {/snippet}
  </AsyncBoundary>
</Section>

<AddServerDialog bind:open={addOpen} />
<ImportDialog bind:open={importOpen} />
<DeleteConfirm
  bind:open={deleteOpen}
  count={pendingDelete.length}
  onConfirm={confirmDelete}
/>

<style>
  .sel-bar {
    display: flex; align-items: center; gap: var(--shuttle-space-2);
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    margin-bottom: var(--shuttle-space-2);
    background: var(--shuttle-bg-subtle);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    font-size: var(--shuttle-text-sm);
  }
  .count {
    margin-right: auto;
    color: var(--shuttle-fg-primary);
    font-weight: var(--shuttle-weight-medium);
  }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/servers/ServersPage.svelte
git commit -m "feat(servers): ServersPage with selection bar + dialogs"
```

---

### Task 10: `index.ts` + route swap

**Files:**
- Create: `gui/web/src/features/servers/index.ts`
- Modify: `gui/web/src/app/routes.ts`

- [ ] **Step 1: Create `index.ts`**

```ts
import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/servers',
  component: lazy(() => import('./ServersPage.svelte')),
  nav: { label: 'nav.servers', icon: 'servers', order: 20 },
}

export { useServers, setActive } from './resource.svelte'
```

- [ ] **Step 2: Update `app/routes.ts`**

Replace the Servers bridge entry. The file becomes:

```ts
import { lazy } from '@/lib/router'
import type { Component } from 'svelte'
import * as dashboard from '@/features/dashboard'
import * as servers from '@/features/servers'

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
  servers.route,
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
Expected: succeeds, new `Servers-*.js` lazy chunk emitted.

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/features/servers/index.ts gui/web/src/app/routes.ts
git commit -m "feat(servers): export route + swap app/routes.ts"
```

---

## Section E · Cleanup + verification (Tasks 11-12)

### Task 11: Delete legacy `pages/Servers.svelte`

**Files:**
- Delete: `gui/web/src/pages/Servers.svelte`

- [ ] **Step 1: Confirm no remaining imports**

```bash
cd "/Users/homebot/Library/Mobile Documents/com~apple~CloudDocs/shuttle/gui/web"
grep -rEn "from '[./]*pages/Servers\\.svelte'" src/ 2>/dev/null
```
Expected: no output.

- [ ] **Step 2: Delete**

```bash
rm gui/web/src/pages/Servers.svelte
```

- [ ] **Step 3: Build + svelte-check**

```bash
cd gui/web
npx svelte-check --threshold error 2>&1 | tail -1
npm run build 2>&1 | tail -3
```
Expected: svelte-check error count decreases; build succeeds.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(gui): delete legacy pages/Servers.svelte (704 lines)"
```

---

### Task 12: Update Playwright, final gates, push

**Files:**
- Modify: `gui/web/tests/subscriptions.spec.ts` (verify still passes — no change expected)
- Modify: `gui/web/tests/shell.spec.ts` (add Servers-page smoke)
- Modify: `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md` (append P4 row)

- [ ] **Step 1: Add Servers smokes to `shell.spec.ts`**

Append after the P3 `test.describe`:

```ts
test.describe('P4 servers', () => {
    test('servers URL renders dense table header', async ({ page }) => {
        await page.goto('/#/servers');
        await expect(page.locator('.sidebar')).toBeVisible();
        // Dense table header contains the column labels.
        await expect(page.locator('text=Address').first()).toBeVisible({ timeout: 5000 });
        await expect(page.locator('text=Latency')).toBeVisible();
        await expect(page.locator('text=Protocol')).toBeVisible();
    });

    test('Add server button opens dialog', async ({ page }) => {
        await page.goto('/#/servers');
        await expect(page.locator('.sidebar')).toBeVisible();
        await page.locator('button:has-text("Add server")').click();
        await expect(page.locator('text=Enter server details')).toBeVisible();
    });
});
```

- [ ] **Step 2: Full gate run**

```bash
cd gui/web
echo "=== svelte-check ===" && npx svelte-check --threshold error 2>&1 | tail -1
echo "=== vitest ==="        && npm test 2>&1 | tail -3
echo "=== i18n ==="           && ./scripts/check-i18n.sh
echo "=== build ==="          && npm run build 2>&1 | tail -3
echo "=== playwright ==="     && npx playwright test --reporter=line 2>&1 | tail -3
```
Expected: all green. Error count drops further (legacy Servers.svelte deleted).

- [ ] **Step 3: Append post-P4 bundle sizes**

Append to `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md`:

```markdown
---

## Post-P4 (2026-04-XX)

- `Servers-*.js` lazy chunk: pre-P4 10.66 KB raw / 3.56 KB gzip (legacy) → post-P4 _<N> KB raw / <M> KB gzip_ (new dense-table)
- `index-*.js`: gzip delta _<±N> KB_ (runs captured from CI)
- svelte-check error count: _<before> → <after>_ (drop attributable to legacy Servers.svelte)
```

Fill in concrete numbers from Step 2's build output.

- [ ] **Step 4: Commit + push**

```bash
git add gui/web/tests/shell.spec.ts docs/superpowers/plans/
git commit -m "test(servers): Playwright smoke + post-P4 bundle record"
git push origin refactor/gui-v2
```

- [ ] **Step 5: Update PR #8 body**

```bash
gh pr view 8 --json body -q .body > /tmp/pr-body.md
```

Append a `## P4 · Servers feature` section to that file summarizing:
- new `features/servers/` slice (9 files)
- dense 48-px table + in-place expand + multi-select + batch ops
- deleted legacy Servers.svelte (704 lines)
- active indicator via border-left (no dedicated column)

Then:
```bash
gh pr edit 8 --body-file /tmp/pr-body.md
rm /tmp/pr-body.md
```

---

## Self-review notes

**Spec coverage.**
- §7.2 dense 48-px table → Task 7 ServerRow grid, Task 8 ServerTable
- §7.2 click-to-expand in-place → Task 6 ServerRowExpanded + Task 8 table orchestration
- §7.2 multi-select + batch ops → Task 8 selection state + Task 9 SelectionBar + resource.removeMany/runSpeedtest
- §7.2 Add-server bits-ui Dialog → Task 3
- §7.2 active indicator via border-left → ServerRow `.active { border-left-color: var(--shuttle-accent); }`
- §7.10 empty / loading / error states → AsyncBoundary + ServerTable Empty fallback

**Placeholder scan.** Each task has complete code blocks. Task 6's `saveField()` is a deliberate no-op with an explicit comment — not a placeholder; the affordance is kept for familiarity until a P5+ update endpoint exists.

**Type consistency.**
- `Server` shape from `@/lib/api/types` used throughout.
- `useServers()` returns `Resource<ServersResponse>` where `ServersResponse = { active: Server; servers: Server[] }`.
- Selection is `Set<string>` of `addr` everywhere.
- `onSelectedChange: (next: Set<string>) => void` signature matches ServerTable's caller.

**Explicit out-of-scope.**
- Drag-and-drop reorder (YAGNI for P4; server order rarely matters)
- Fuzzy search / filter (defer to when >50 servers become common)
- Password field edit in expanded pane (no update endpoint; deferred)
- Subscription-origin labels (implemented in P5 Subscriptions)
- Per-row speedtest history chart (spec §7.2 mentions it but it's defer-able; P4 shows only latest result via `useSpeedtestResult`)

**Known risks.**
- Backend `importConfig`'s auto-detect might fail silently on unknown formats; UX falls back to an info toast. Verify against a real backend during manual smoke.
- `saveField()` no-op is surprising UX; document in the commit message.
- Deleting a server that's currently active: backend returns an error from `/api/config/servers DELETE`. Our toast will surface the message. Worth verifying; if it crashes the client, add a guard before the confirm.

---

## Plan complete.

Plan complete and saved to `docs/superpowers/plans/2026-04-20-gui-refactor-p4-servers.md`.

Execution:
- **Subagent-Driven** — fresh agent per task, review between
- **Inline** — execute here in this session with checkpoints
