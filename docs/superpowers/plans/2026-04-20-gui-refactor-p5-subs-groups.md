# GUI Refactor P5 · Subscriptions + Groups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `pages/Subscriptions.svelte` and `pages/Groups.svelte` bridges with redesigned feature slices per spec §7.3 (dense table, like Servers) and §7.4 (card grid + `#/groups/:id` detail sub-route). Bundle the two features in one PR because they share conventions and the nav section they belong to.

**Architecture:**
- **Subscriptions** mirrors the P4 Servers pattern: table + row + expanded detail + AddSubscriptionDialog + DeleteConfirm + resource. No multi-select (rarely useful for subscriptions).
- **Groups** is new ground: 4-column card grid of `GroupCard`s; click a card → hash navigates to `#/groups/:tag` → `GroupDetail` renders inline via the P1 Router child-routes mechanism. A trailing dashed "+ New group" card opens an empty-state notice because the backend has no create endpoint yet.

**Tech Stack:** Svelte 5 runes · `@/lib/resource` · `@/lib/api` · `@/ui` · P1 hash router with child routes.

**Spec reference:** `docs/superpowers/specs/2026-04-19-gui-refactor-design.md` §7.3 (Subscriptions), §7.4 (Groups).

**Branch:** Continue on `refactor/gui-v2`. P5 lands as commits on the existing PR #8.

---

## Design reference

### Subscriptions page layout

Header: `Subscriptions` title, `N sources` subtitle, right-aligned `+ Add subscription` button.

Table columns (48 px rows):
- Status dot (16 px — green if last refresh OK, red if error, grey if never refreshed)
- Name (flex 2)
- URL (flex 4, monospace, truncated)
- Servers count (80 px, tabular-nums)
- Last updated (120 px, relative "3h ago" / "never")
- Actions (expand / refresh / delete). Refresh button only appears on hover to reduce visual noise (spec §7.3).

Expanded detail: edit name + URL inputs, "force refresh" button, imported servers list (first 5 + "… X more"), last error (if any).

### Groups page layout

Header: `Outbound groups` title, `N groups` subtitle. No action buttons (create not supported by backend yet).

4-column grid (CSS `grid-template-columns: repeat(4, 1fr)`, gap 12 px). Last cell: dashed-border empty card labeled `+ New group (coming soon)`.

Each `GroupCard`:
- Group tag (mono, large)
- Strategy badge (`failover` / `round-robin` / `quality`)
- `{N} members`
- Currently selected: member tag (truncated if long)
- 24-h quality sparkline (stub for P5 — renders flat line with tooltip "quality history: coming soon"; full impl in a later phase when backend exposes per-member history)
- Click anywhere → `navigate('/groups/:tag')`

### Group detail page (`#/groups/:tag`)

Sub-route mounted under Groups. Shown when path matches `/groups/:tag`. Replaces the card grid view when active.

Layout:
- Breadcrumb header: `Groups / <tag>` with back button
- Strategy dropdown (Select) — change strategy (wired but backend may or may not accept; fall back to toast error)
- Members table: member addr, current-selected flag, last test latency, manual `Pick` button
- `Test all` button at top — runs `testGroup` and updates member latencies

---

## File structure

```
gui/web/src/features/subscriptions/
├─ index.ts
├─ SubscriptionsPage.svelte
├─ SubscriptionTable.svelte
├─ SubscriptionRow.svelte
├─ SubscriptionRowExpanded.svelte
├─ AddSubscriptionDialog.svelte
└─ resource.svelte.ts

gui/web/src/features/groups/
├─ index.ts
├─ GroupsPage.svelte              grid of cards
├─ GroupCard.svelte               one card
├─ GroupDetail.svelte             sub-route page (#/groups/:tag)
└─ resource.svelte.ts
```

**Reuses** the P4 `features/servers/DeleteConfirm.svelte` — we'll either re-export it from `@/features/servers` or copy once to `@/ui/` if it grows broader. For P5, re-export is fine since both features live under `features/` and aren't peers (it's acceptable to cross-import for well-known utilities). If cross-feature import feels wrong, we move it to `@/ui/` in the same commit; decided at Task 5.

**Deletions** after ship:
- `pages/Subscriptions.svelte` (445 lines)
- `pages/Groups.svelte` (389 lines)

---

## Conventions

- Relative paths from repo root.
- Every task ends with a commit.
- New user-visible strings go through i18n: add keys to `locales/{en,zh-CN}.json` alongside feature code.
- Tests live next to code: `<Name>.test.ts`.

---

## Section A · Subscriptions resource (Task 1)

### Task 1: `features/subscriptions/resource.svelte.ts`

**Files:**
- Create: `gui/web/src/features/subscriptions/resource.svelte.ts`

- [ ] **Step 1: Create directory + file**

```bash
mkdir -p gui/web/src/features/subscriptions
```

```ts
import { createResource, invalidate, type Resource } from '@/lib/resource.svelte'
import {
  getSubscriptions,
  addSubscription as apiAdd,
  refreshSubscription as apiRefresh,
  deleteSubscription as apiDelete,
} from '@/lib/api/endpoints'
import type { Subscription } from '@/lib/api/types'
import { toasts } from '@/lib/toaster.svelte'
import { t } from '@/lib/i18n/index'

const LIST_KEY = 'subscriptions.list'

export function useSubscriptions(): Resource<Subscription[]> {
  return createResource(LIST_KEY, getSubscriptions, {
    poll: 30_000,
    initial: [],
  })
}

export async function addSubscription(name: string, url: string): Promise<void> {
  try {
    await apiAdd(name, url)
    invalidate(LIST_KEY)
    toasts.success(t('subscriptions.toast.added', { name: name || url }))
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}

export async function refreshSubscription(id: string): Promise<void> {
  try {
    await apiRefresh(id)
    invalidate(LIST_KEY)
    toasts.success(t('subscriptions.toast.refreshed'))
  } catch (e) {
    toasts.error((e as Error).message)
  }
}

export async function deleteSubscription(id: string): Promise<void> {
  try {
    await apiDelete(id)
    invalidate(LIST_KEY)
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}
```

- [ ] **Step 2: svelte-check**

```bash
cd gui/web && npx svelte-check --threshold error 2>&1 | grep -E "ERROR.*features/subscriptions" | head -5
```
Expected: no matches (the new i18n keys don't yet exist but that only fails at runtime, not compile).

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/features/subscriptions/resource.svelte.ts
git commit -m "feat(subscriptions): resource layer (list + add/refresh/delete)"
```

---

## Section B · Subscriptions UI (Tasks 2-5)

### Task 2: `SubscriptionRow.svelte`

**Files:**
- Create: `gui/web/src/features/subscriptions/SubscriptionRow.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { Button, Icon, Badge } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { refreshSubscription } from './resource.svelte'
  import type { Subscription } from '@/lib/api/types'

  interface Props {
    sub: Subscription
    expanded: boolean
    onExpandedToggle: () => void
    onDelete: () => void
  }
  let { sub, expanded, onExpandedToggle, onDelete }: Props = $props()

  let refreshing = $state(false)
  async function refresh() {
    refreshing = true
    try { await refreshSubscription(sub.id) } finally { refreshing = false }
  }

  // Infer format by URL prefix — same idea as ServerRow's protocol.
  function inferFormat(url: string): string {
    if (/\.ya?ml(\?|$)/i.test(url)) return 'clash'
    if (/sip008|\.json(\?|$)/i.test(url)) return 'sip008'
    return 'auto'
  }

  function relative(ts: string | undefined): string {
    if (!ts) return t('subscriptions.never')
    const diff = Date.now() - new Date(ts).getTime()
    if (diff < 60_000)      return t('subscriptions.justNow')
    if (diff < 3_600_000)   return t('subscriptions.minutesAgo', { n: Math.floor(diff / 60_000) })
    if (diff < 86_400_000)  return t('subscriptions.hoursAgo',   { n: Math.floor(diff / 3_600_000) })
    return t('subscriptions.daysAgo', { n: Math.floor(diff / 86_400_000) })
  }

  const count = $derived(sub.servers?.length ?? 0)
  const status = $derived(sub.error ? 'bad' : sub.updated_at ? 'ok' : 'unknown')
</script>

<div class="row">
  <span class={`status ${status}`}></span>
  <span class="name">{sub.name || sub.url}</span>
  <span class="url">{sub.url}</span>
  <span class="count">{count}</span>
  <span class="ago">{relative(sub.updated_at)}</span>
  <span class="fmt"><Badge>{inferFormat(sub.url)}</Badge></span>
  <span class="actions">
    <Button size="sm" variant="ghost" onclick={onExpandedToggle}>
      <Icon name={expanded ? 'chevronDown' : 'chevronRight'} size={14} />
    </Button>
    <Button size="sm" variant="ghost" class="hover-only" loading={refreshing} onclick={refresh}>
      <Icon name="check" size={14} title={t('subscriptions.refresh')} />
    </Button>
    <Button size="sm" variant="ghost" onclick={onDelete}>
      <Icon name="trash" size={14} title={t('common.delete')} />
    </Button>
  </span>
</div>

<style>
  .row {
    display: grid;
    grid-template-columns: 16px 2fr 4fr 72px 120px 72px auto;
    align-items: center;
    gap: var(--shuttle-space-3);
    height: 48px;
    padding: 0 var(--shuttle-space-4);
    border-top: 1px solid var(--shuttle-border);
    font-size: var(--shuttle-text-sm);
  }
  .row:first-child { border-top: 0; }
  .status {
    width: 8px; height: 8px; border-radius: 50%;
    background: var(--shuttle-fg-muted);
  }
  .status.ok  { background: var(--shuttle-success); }
  .status.bad { background: var(--shuttle-danger); }
  .name { color: var(--shuttle-fg-primary); font-weight: var(--shuttle-weight-medium); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .url  { font-family: var(--shuttle-font-mono); color: var(--shuttle-fg-secondary); font-size: var(--shuttle-text-xs); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .count, .ago { color: var(--shuttle-fg-secondary); font-variant-numeric: tabular-nums; }
  .ago { font-size: var(--shuttle-text-xs); }
  .actions { display: flex; gap: 2px; justify-content: flex-end; }

  /* Hover-only refresh button per §7.3 */
  :global(.hover-only) { opacity: 0; transition: opacity var(--shuttle-duration); }
  .row:hover :global(.hover-only) { opacity: 1; }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/subscriptions/SubscriptionRow.svelte
git commit -m "feat(subscriptions): SubscriptionRow — dense summary + hover-only refresh"
```

---

### Task 3: `SubscriptionRowExpanded.svelte`

**Files:**
- Create: `gui/web/src/features/subscriptions/SubscriptionRowExpanded.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { StatRow, Badge } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import type { Subscription } from '@/lib/api/types'

  interface Props { sub: Subscription }
  let { sub }: Props = $props()

  const shown = $derived((sub.servers ?? []).slice(0, 5))
  const more  = $derived(Math.max(0, (sub.servers?.length ?? 0) - 5))
</script>

<div class="pane">
  <div class="fields">
    <StatRow label={t('subscriptions.columns.url')} value={sub.url} mono />
    <StatRow label={t('subscriptions.columns.servers')} value={String(sub.servers?.length ?? 0)} />
    <StatRow label={t('subscriptions.columns.updated')} value={sub.updated_at ?? '—'} mono />
    {#if sub.error}
      <div class="err">{sub.error}</div>
    {/if}
  </div>

  {#if shown.length > 0}
    <div class="servers">
      <h4>{t('subscriptions.importedServers')}</h4>
      <ul>
        {#each shown as s}
          <li>
            <span class="sname">{s.name || s.addr}</span>
            <span class="saddr">{s.addr}</span>
          </li>
        {/each}
        {#if more > 0}
          <li class="more">{t('subscriptions.andMore', { n: more })}</li>
        {/if}
      </ul>
    </div>
  {/if}
</div>

<style>
  .pane {
    display: grid; grid-template-columns: 1fr 280px;
    gap: var(--shuttle-space-5);
    padding: var(--shuttle-space-4) var(--shuttle-space-4) var(--shuttle-space-4) var(--shuttle-space-6);
    background: var(--shuttle-bg-subtle);
    border-top: 1px solid var(--shuttle-border);
  }
  .fields { display: flex; flex-direction: column; gap: var(--shuttle-space-2); }
  .err {
    padding: var(--shuttle-space-2);
    background: color-mix(in oklab, var(--shuttle-danger) 10%, transparent);
    color: var(--shuttle-danger);
    border-radius: var(--shuttle-radius-sm);
    font-size: var(--shuttle-text-xs);
    font-family: var(--shuttle-font-mono);
  }
  .servers h4 {
    margin: 0 0 var(--shuttle-space-2);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  ul { list-style: none; margin: 0; padding: 0; }
  li {
    display: flex; justify-content: space-between;
    padding: var(--shuttle-space-1) 0;
    font-size: var(--shuttle-text-xs);
  }
  .sname { color: var(--shuttle-fg-primary); }
  .saddr { color: var(--shuttle-fg-muted); font-family: var(--shuttle-font-mono); }
  .more  { color: var(--shuttle-fg-muted); font-style: italic; }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/subscriptions/SubscriptionRowExpanded.svelte
git commit -m "feat(subscriptions): RowExpanded with imported-servers preview"
```

---

### Task 4: `SubscriptionTable.svelte`

**Files:**
- Create: `gui/web/src/features/subscriptions/SubscriptionTable.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { Empty, Card } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import SubscriptionRow from './SubscriptionRow.svelte'
  import SubscriptionRowExpanded from './SubscriptionRowExpanded.svelte'
  import type { Subscription } from '@/lib/api/types'

  interface Props {
    items: Subscription[]
    onDelete: (id: string) => void
  }
  let { items, onDelete }: Props = $props()

  const expanded = $state<Set<string>>(new Set())

  function toggle(id: string) {
    if (expanded.has(id)) expanded.delete(id)
    else expanded.add(id)
  }
</script>

{#if items.length === 0}
  <Card>
    <Empty
      icon="subscriptions"
      title={t('subscriptions.empty.title')}
      description={t('subscriptions.empty.desc')}
    />
  </Card>
{:else}
  <div class="table">
    <div class="header">
      <span></span>
      <span>{t('subscriptions.columns.name')}</span>
      <span>{t('subscriptions.columns.url')}</span>
      <span>{t('subscriptions.columns.servers')}</span>
      <span>{t('subscriptions.columns.updated')}</span>
      <span>{t('subscriptions.columns.format')}</span>
      <span></span>
    </div>
    {#each items as s (s.id)}
      <SubscriptionRow
        sub={s}
        expanded={expanded.has(s.id)}
        onExpandedToggle={() => toggle(s.id)}
        onDelete={() => onDelete(s.id)}
      />
      {#if expanded.has(s.id)}
        <SubscriptionRowExpanded sub={s} />
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
    grid-template-columns: 16px 2fr 4fr 72px 120px 72px auto;
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
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/subscriptions/SubscriptionTable.svelte
git commit -m "feat(subscriptions): SubscriptionTable with expand state"
```

---

### Task 5: `AddSubscriptionDialog.svelte`

**Files:**
- Create: `gui/web/src/features/subscriptions/AddSubscriptionDialog.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { Dialog, Input, Button } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { addSubscription } from './resource.svelte'

  interface Props { open: boolean }
  let { open = $bindable(false) }: Props = $props()

  let name = $state('')
  let url = $state('')
  let submitting = $state(false)

  const canSubmit = $derived(url.trim().length > 0)

  async function submit() {
    if (!canSubmit) return
    submitting = true
    try {
      await addSubscription(name.trim(), url.trim())
      name = ''; url = ''
      open = false
    } finally {
      submitting = false
    }
  }
</script>

<Dialog bind:open title={t('subscriptions.dialog.add.title')} description={t('subscriptions.dialog.add.desc')}>
  <div class="fields">
    <Input label={t('subscriptions.name')} bind:value={name} />
    <Input label={t('subscriptions.url')} placeholder="https://example.com/sub.yaml" bind:value={url} />
  </div>

  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>{t('common.cancel')}</Button>
    <Button variant="primary" disabled={!canSubmit} loading={submitting} onclick={submit}>
      {t('subscriptions.add')}
    </Button>
  {/snippet}
</Dialog>

<style>
  .fields { display: flex; flex-direction: column; gap: var(--shuttle-space-3); }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/subscriptions/AddSubscriptionDialog.svelte
git commit -m "feat(subscriptions): AddSubscriptionDialog"
```

---

## Section C · Subscriptions page + route (Task 6)

### Task 6: `SubscriptionsPage.svelte` + `index.ts` + route swap

**Files:**
- Create: `gui/web/src/features/subscriptions/SubscriptionsPage.svelte`
- Create: `gui/web/src/features/subscriptions/index.ts`
- Modify: `gui/web/src/app/routes.ts`

- [ ] **Step 1: Create `SubscriptionsPage.svelte`**

```svelte
<script lang="ts">
  import { AsyncBoundary, Button, Icon, Section } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { useSubscriptions, deleteSubscription } from './resource.svelte'
  import SubscriptionTable from './SubscriptionTable.svelte'
  import AddSubscriptionDialog from './AddSubscriptionDialog.svelte'
  import DeleteConfirm from '@/features/servers/DeleteConfirm.svelte'

  const res = useSubscriptions()

  let addOpen = $state(false)
  let delOpen = $state(false)
  let pending = $state<string | null>(null)

  function openDelete(id: string) {
    pending = id
    delOpen = true
  }

  async function confirmDelete() {
    if (pending) await deleteSubscription(pending)
    pending = null
  }
</script>

<Section
  title={t('nav.subscriptions')}
  description={res.data ? t('subscriptions.count', { n: res.data.length }) : undefined}
>
  {#snippet actions()}
    <Button variant="primary" onclick={() => (addOpen = true)}>
      <Icon name="plus" size={14} /> {t('subscriptions.add')}
    </Button>
  {/snippet}

  <AsyncBoundary resource={res}>
    {#snippet children(items)}
      <SubscriptionTable {items} onDelete={openDelete} />
    {/snippet}
  </AsyncBoundary>
</Section>

<AddSubscriptionDialog bind:open={addOpen} />
<DeleteConfirm bind:open={delOpen} count={1} onConfirm={confirmDelete} />
```

- [ ] **Step 2: Create `index.ts`**

```ts
import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/subscriptions',
  component: lazy(() => import('./SubscriptionsPage.svelte')),
  nav: { label: 'nav.subscriptions', icon: 'subscriptions', order: 30 },
}

export { useSubscriptions } from './resource.svelte'
```

- [ ] **Step 3: Update `app/routes.ts`** — replace the Subscriptions bridge line with `subscriptions.route`

```ts
import * as subscriptions from '@/features/subscriptions'
// ...
export const routes: AppRoute[] = [
  dashboard.route,
  servers.route,
  subscriptions.route,
  // ... rest unchanged
]
```

Specifically, delete the `{ path: '/subscriptions', component: lazy(() => import('@/pages/Subscriptions.svelte')), ... }` block and replace with `subscriptions.route` in the array literal.

- [ ] **Step 4: Build**

```bash
cd gui/web && npm run build 2>&1 | tail -3
```
Expected: succeeds.

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/features/subscriptions/ gui/web/src/app/routes.ts
git commit -m "feat(subscriptions): page + index + route swap"
```

---

## Section D · Subscriptions i18n keys (Task 7)

### Task 7: Add Subscriptions i18n keys

**Files:**
- Modify: `gui/web/src/locales/en.json`
- Modify: `gui/web/src/locales/zh-CN.json`

- [ ] **Step 1: Extend locales**

Run a small Python snippet to deep-merge new keys into both files:

```bash
cd gui/web/src/locales && python3 <<'PY'
import json
def merge(a, b):
    for k, v in b.items():
        if isinstance(v, dict) and isinstance(a.get(k), dict): merge(a[k], v)
        else: a[k] = v
    return a

EN = {
  "subscriptions": {
    "count": "{n} sources",
    "empty": {
      "title": "No subscriptions",
      "desc":  "Add one to auto-import servers from a remote list."
    },
    "dialog": {
      "add": {
        "title": "Add subscription",
        "desc":  "Paste a subscription URL (Clash YAML, SIP-008, or shuttle:// list)."
      }
    },
    "columns": {
      "name":     "Name",
      "url":      "URL",
      "servers":  "Servers",
      "updated":  "Updated",
      "format":   "Format"
    },
    "refresh":   "Refresh now",
    "never":     "never",
    "justNow":   "just now",
    "minutesAgo": "{n} min ago",
    "hoursAgo":   "{n} h ago",
    "daysAgo":    "{n} d ago",
    "importedServers": "Imported servers",
    "andMore": "… and {n} more",
    "toast": {
      "added":     "Added {name}",
      "refreshed": "Refreshed"
    }
  }
}
ZH = {
  "subscriptions": {
    "count": "{n} 个订阅",
    "empty": {
      "title": "暂无订阅",
      "desc":  "添加一个订阅从远端列表自动导入服务器。"
    },
    "dialog": {
      "add": {
        "title": "添加订阅",
        "desc":  "粘贴订阅链接(Clash YAML、SIP-008 或 shuttle:// 列表)。"
      }
    },
    "columns": {
      "name":    "名称",
      "url":     "链接",
      "servers": "服务器",
      "updated": "更新",
      "format":  "格式"
    },
    "refresh":   "立即刷新",
    "never":     "从未",
    "justNow":   "刚刚",
    "minutesAgo": "{n} 分钟前",
    "hoursAgo":   "{n} 小时前",
    "daysAgo":    "{n} 天前",
    "importedServers": "已导入的服务器",
    "andMore": "……还有 {n} 个",
    "toast": {
      "added":     "已添加 {name}",
      "refreshed": "已刷新"
    }
  }
}

for fname, add in [('en.json', EN), ('zh-CN.json', ZH)]:
  data = json.load(open(fname))
  merge(data, add)
  json.dump(data, open(fname, 'w'), indent=2, ensure_ascii=False)
print('Done')
PY
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/locales/
git commit -m "i18n(subscriptions): add keys for new feature strings"
```

---

## Section E · Groups resource + detail route (Tasks 8-12)

### Task 8: `features/groups/resource.svelte.ts`

**Files:**
- Create: `gui/web/src/features/groups/resource.svelte.ts`

- [ ] **Step 1: Create directory + file**

```bash
mkdir -p gui/web/src/features/groups
```

```ts
import { createResource, invalidate, type Resource } from '@/lib/resource.svelte'
import {
  getGroups,
  getGroup as apiGetGroup,
  testGroup as apiTestGroup,
  selectGroupMember as apiSelectMember,
} from '@/lib/api/endpoints'
import type { GroupInfo, GroupTestResult } from '@/lib/api/types'
import { toasts } from '@/lib/toaster.svelte'
import { t } from '@/lib/i18n/index'

const LIST_KEY = 'groups.list'

export function useGroups(): Resource<GroupInfo[]> {
  return createResource(LIST_KEY, getGroups, {
    poll: 15_000,
    initial: [],
  })
}

// Single-group resource keyed by tag — each tag gets its own cached entry.
export function useGroup(tag: string): Resource<GroupInfo> {
  return createResource(
    `groups.item.${tag}`,
    () => apiGetGroup(tag),
    { poll: 10_000 },
  )
}

// Test results are transient per-group; stored in a small module map.
const testResults = $state<{ map: Record<string, GroupTestResult[]> }>({ map: {} })

export function useGroupTestResults(tag: string): GroupTestResult[] {
  return testResults.map[tag] ?? []
}

export async function testGroup(tag: string): Promise<void> {
  try {
    const rs = await apiTestGroup(tag)
    testResults.map[tag] = rs
    toasts.success(t('groups.toast.tested', { n: rs.length }))
  } catch (e) {
    toasts.error((e as Error).message)
  }
}

export async function selectMember(groupTag: string, member: string): Promise<void> {
  try {
    await apiSelectMember(groupTag, member)
    invalidate(LIST_KEY)
    invalidate(`groups.item.${groupTag}`)
    toasts.success(t('groups.toast.selected', { name: member }))
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}

export function __resetGroupResults(): void {
  testResults.map = {}
}
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/groups/resource.svelte.ts
git commit -m "feat(groups): resource layer (list + single + test + select)"
```

---

### Task 9: `GroupCard.svelte`

**Files:**
- Create: `gui/web/src/features/groups/GroupCard.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { Card, Badge } from '@/ui'
  import { navigate } from '@/lib/router'
  import { t } from '@/lib/i18n/index'
  import type { GroupInfo } from '@/lib/api/types'

  interface Props { group: GroupInfo }
  let { group }: Props = $props()

  function open() {
    navigate(`/groups/${encodeURIComponent(group.tag)}`)
  }
</script>

<button class="card" onclick={open} aria-label={group.tag}>
  <Card>
    <div class="top">
      <span class="tag">{group.tag}</span>
      <Badge>{group.strategy}</Badge>
    </div>
    <div class="meta">
      <div>
        <div class="label">{t('groups.members')}</div>
        <div class="val">{group.members.length}</div>
      </div>
      <div>
        <div class="label">{t('groups.selected')}</div>
        <div class="val mono">{group.selected ?? '—'}</div>
      </div>
    </div>
    <div class="spark">
      <span class="hint">{t('groups.qualityComingSoon')}</span>
    </div>
  </Card>
</button>

<style>
  .card {
    background: transparent; border: 0; padding: 0; text-align: left;
    cursor: pointer; font: inherit; color: inherit;
    display: block; width: 100%;
  }
  .top { display: flex; align-items: center; justify-content: space-between; margin-bottom: var(--shuttle-space-3); }
  .tag { font-family: var(--shuttle-font-mono); font-size: var(--shuttle-text-base); color: var(--shuttle-fg-primary); font-weight: var(--shuttle-weight-semibold); }
  .meta { display: grid; grid-template-columns: 1fr 1fr; gap: var(--shuttle-space-3); margin-bottom: var(--shuttle-space-3); }
  .label { font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-muted); text-transform: uppercase; letter-spacing: 0.06em; }
  .val { font-size: var(--shuttle-text-lg); color: var(--shuttle-fg-primary); font-variant-numeric: tabular-nums; margin-top: var(--shuttle-space-1); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .val.mono { font-family: var(--shuttle-font-mono); font-size: var(--shuttle-text-sm); }
  .spark {
    height: 40px; display: flex; align-items: center; justify-content: center;
    background: var(--shuttle-bg-subtle);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
  }
  .hint { font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-muted); font-style: italic; }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/groups/GroupCard.svelte
git commit -m "feat(groups): GroupCard summary"
```

---

### Task 10: `GroupDetail.svelte`

**Files:**
- Create: `gui/web/src/features/groups/GroupDetail.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { AsyncBoundary, Button, Icon, Section, StatRow, Badge } from '@/ui'
  import { useParams, navigate } from '@/lib/router'
  import { t } from '@/lib/i18n/index'
  import { useGroup, testGroup, selectMember, useGroupTestResults } from './resource.svelte'

  const params = $derived(useParams<{ tag: string }>('/groups/:tag'))
  const tag = $derived(decodeURIComponent(params.tag ?? ''))
  const res = $derived(useGroup(tag))

  const testRs = $derived(useGroupTestResults(tag))

  function latencyFor(member: string): string {
    const r = testRs.find((x) => x.tag === member)
    if (!r) return '— ms'
    if (!r.available) return t('groups.failed')
    return `${r.latency_ms} ms`
  }
</script>

<Section
  title={t('nav.groups')}
  description={tag}
>
  {#snippet actions()}
    <Button variant="ghost" onclick={() => navigate('/groups')}>
      <Icon name="chevronLeft" size={14} /> {t('groups.backToGroups')}
    </Button>
    <Button variant="secondary" onclick={() => testGroup(tag)}>{t('groups.testAll')}</Button>
  {/snippet}

  <AsyncBoundary resource={res}>
    {#snippet children(g)}
      <div class="summary">
        <StatRow label={t('groups.strategy')} value={g.strategy} mono />
        <StatRow label={t('groups.members')}  value={String(g.members.length)} />
        <StatRow label={t('groups.selected')} value={g.selected ?? '—'} mono />
      </div>

      <div class="members">
        {#each g.members as m}
          <div class="mrow">
            <span class="mname">{m}</span>
            <span class="mlat">{latencyFor(m)}</span>
            {#if g.selected === m}
              <Badge variant="success">{t('groups.active')}</Badge>
            {:else}
              <Button size="sm" variant="ghost" onclick={() => selectMember(tag, m)}>
                {t('groups.pick')}
              </Button>
            {/if}
          </div>
        {/each}
      </div>
    {/snippet}
  </AsyncBoundary>
</Section>

<style>
  .summary {
    display: grid; grid-template-columns: repeat(3, 1fr);
    gap: var(--shuttle-space-3);
    margin-bottom: var(--shuttle-space-5);
  }
  .members {
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    overflow: hidden;
  }
  .mrow {
    display: grid; grid-template-columns: 1fr 100px auto;
    align-items: center; gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-2) var(--shuttle-space-4);
    border-top: 1px solid var(--shuttle-border);
    font-size: var(--shuttle-text-sm);
  }
  .mrow:first-child { border-top: 0; }
  .mname { font-family: var(--shuttle-font-mono); color: var(--shuttle-fg-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .mlat  { font-family: var(--shuttle-font-mono); color: var(--shuttle-fg-secondary); font-variant-numeric: tabular-nums; text-align: right; }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/groups/GroupDetail.svelte
git commit -m "feat(groups): GroupDetail page with member table + select"
```

---

### Task 11: `GroupsPage.svelte` (grid)

**Files:**
- Create: `gui/web/src/features/groups/GroupsPage.svelte`

**Context:** The page branches internally: if the current route is `/groups/:tag`, render `<GroupDetail>`; else render the grid. We use `matches('/groups/:tag')` from the router to decide.

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { AsyncBoundary, Empty, Section } from '@/ui'
  import { matches, useRoute } from '@/lib/router'
  import { t } from '@/lib/i18n/index'
  import { useGroups } from './resource.svelte'
  import GroupCard from './GroupCard.svelte'
  import GroupDetail from './GroupDetail.svelte'

  const res = useGroups()
  const route = useRoute()

  // Decide between detail (when path matches /groups/:tag) and list view.
  const showDetail = $derived.by(() => {
    void route.path
    return matches('/groups/:tag')
  })
</script>

{#if showDetail}
  <GroupDetail />
{:else}
  <Section
    title={t('nav.groups')}
    description={res.data ? t('groups.count', { n: res.data.length }) : undefined}
  >
    <AsyncBoundary resource={res}>
      {#snippet children(groups)}
        {#if groups.length === 0}
          <Empty icon="groups" title={t('groups.empty.title')} description={t('groups.empty.desc')} />
        {:else}
          <div class="grid">
            {#each groups as g (g.tag)}
              <GroupCard group={g} />
            {/each}
            <div class="stub">
              <div class="stub-inner">
                <span>+ {t('groups.newStub')}</span>
                <span class="hint">{t('groups.newStubHint')}</span>
              </div>
            </div>
          </div>
        {/if}
      {/snippet}
    </AsyncBoundary>
  </Section>
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
    gap: var(--shuttle-space-3);
  }
  .stub {
    display: flex; align-items: center; justify-content: center;
    min-height: 180px;
    border: 2px dashed var(--shuttle-border-strong);
    border-radius: var(--shuttle-radius-md);
    color: var(--shuttle-fg-muted);
  }
  .stub-inner { text-align: center; display: flex; flex-direction: column; gap: var(--shuttle-space-1); font-size: var(--shuttle-text-sm); }
  .hint { font-size: var(--shuttle-text-xs); }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/groups/GroupsPage.svelte
git commit -m "feat(groups): GroupsPage — grid + internal detail routing"
```

---

### Task 12: Groups `index.ts` + route swap + i18n

**Files:**
- Create: `gui/web/src/features/groups/index.ts`
- Modify: `gui/web/src/app/routes.ts`
- Modify: `gui/web/src/locales/en.json`, `zh-CN.json`

- [ ] **Step 1: Create `index.ts`**

```ts
import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/groups',
  component: lazy(() => import('./GroupsPage.svelte')),
  nav: { label: 'nav.groups', icon: 'groups', order: 40 },
  // Child path — GroupsPage itself branches on the route, so we don't need
  // a declarative child route entry. The Router's findMatch walks children
  // but we lean on matches('/groups/:tag') inside the page for simplicity.
}

export { useGroups } from './resource.svelte'
```

- [ ] **Step 2: Update `app/routes.ts`** — replace the Groups bridge with `groups.route`

```ts
import * as groups from '@/features/groups'
// ...
export const routes: AppRoute[] = [
  dashboard.route,
  servers.route,
  subscriptions.route,
  groups.route,
  // ... rest unchanged
]
```

- [ ] **Step 3: Add Groups i18n keys**

```bash
cd gui/web/src/locales && python3 <<'PY'
import json
def merge(a, b):
    for k, v in b.items():
        if isinstance(v, dict) and isinstance(a.get(k), dict): merge(a[k], v)
        else: a[k] = v
    return a

EN = {
  "groups": {
    "count": "{n} groups",
    "members": "Members",
    "selected": "Selected",
    "strategy": "Strategy",
    "qualityComingSoon": "quality history: coming soon",
    "backToGroups": "Back",
    "testAll": "Test all",
    "active": "ACTIVE",
    "pick": "Pick",
    "failed": "failed",
    "empty": { "title": "No groups", "desc": "Outbound groups are defined in your config file." },
    "newStub": "New group",
    "newStubHint": "create via config; UI editing coming soon",
    "toast": {
      "tested": "Tested {n} members",
      "selected": "Selected {name}"
    }
  }
}
ZH = {
  "groups": {
    "count": "{n} 个组",
    "members": "成员",
    "selected": "已选",
    "strategy": "策略",
    "qualityComingSoon": "质量历史:即将上线",
    "backToGroups": "返回",
    "testAll": "测试全部",
    "active": "活跃",
    "pick": "选用",
    "failed": "失败",
    "empty": { "title": "暂无出站组", "desc": "出站组通过配置文件定义。" },
    "newStub": "新建组",
    "newStubHint": "通过配置创建;UI 编辑即将上线",
    "toast": {
      "tested": "已测试 {n} 个成员",
      "selected": "已选用 {name}"
    }
  }
}

for fname, add in [('en.json', EN), ('zh-CN.json', ZH)]:
  data = json.load(open(fname))
  merge(data, add)
  json.dump(data, open(fname, 'w'), indent=2, ensure_ascii=False)
print('Done')
PY
```

- [ ] **Step 4: Build**

```bash
cd gui/web && npm run build 2>&1 | tail -3
```
Expected: succeeds; new `GroupsPage-*.js` chunk emitted.

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/features/groups/index.ts gui/web/src/app/routes.ts gui/web/src/locales/
git commit -m "feat(groups): index + route swap + i18n keys"
```

---

## Section F · Cleanup + verification (Tasks 13-15)

### Task 13: Delete legacy pages

**Files:**
- Delete: `gui/web/src/pages/Subscriptions.svelte`
- Delete: `gui/web/src/pages/Groups.svelte`

- [ ] **Step 1: Confirm no remaining imports**

```bash
cd "/Users/homebot/Library/Mobile Documents/com~apple~CloudDocs/shuttle/gui/web"
grep -rEn "from '[./]*pages/(Subscriptions|Groups)\\.svelte'" src/ 2>/dev/null
```
Expected: no output.

- [ ] **Step 2: Delete**

```bash
rm gui/web/src/pages/Subscriptions.svelte gui/web/src/pages/Groups.svelte
```

- [ ] **Step 3: Build + svelte-check**

```bash
cd gui/web && npx svelte-check --threshold error 2>&1 | tail -1 && npm run build 2>&1 | tail -3
```
Expected: error count drops further; build succeeds.

- [ ] **Step 4: Commit**

```bash
git add -A gui/web/src/pages/Subscriptions.svelte gui/web/src/pages/Groups.svelte
git commit -m "refactor(gui): delete legacy Subscriptions + Groups pages (834 lines)"
```

---

### Task 14: Playwright smoke + bundle record

**Files:**
- Modify: `gui/web/tests/subscriptions.spec.ts` (rewrite for new page)
- Modify: `gui/web/tests/shell.spec.ts` (add Groups smoke)
- Modify: `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md` (append)

- [ ] **Step 1: Rewrite `subscriptions.spec.ts`**

Replace the file contents:

```ts
import { test, expect } from '@playwright/test';

test.describe('P5 subscriptions', () => {
    test('subscriptions URL renders page chrome', async ({ page }) => {
        await page.goto('/#/subscriptions');
        await expect(page.locator('.sidebar')).toBeVisible();
        await expect(page.locator('h3:has-text("Subscriptions")')).toBeVisible({ timeout: 5000 });
        await expect(page.locator('button:has-text("Add subscription")')).toBeVisible();
    });

    test('Add subscription button opens dialog', async ({ page }) => {
        await page.goto('/#/subscriptions');
        await expect(page.locator('.sidebar')).toBeVisible();
        await page.locator('button:has-text("Add subscription")').click();
        await expect(page.locator('text=Paste a subscription URL')).toBeVisible({ timeout: 5000 });
    });
});
```

- [ ] **Step 2: Append Groups smoke to `shell.spec.ts`**

```ts
test.describe('P5 groups', () => {
    test('groups URL renders page chrome', async ({ page }) => {
        await page.goto('/#/groups');
        await expect(page.locator('.sidebar')).toBeVisible();
        await expect(page.locator('h3:has-text("Groups")').first()).toBeVisible({ timeout: 5000 });
    });
});
```

- [ ] **Step 3: Run all tests**

```bash
cd gui/web
npx svelte-check --threshold error 2>&1 | tail -1
npm test 2>&1 | tail -3
./scripts/check-i18n.sh
npm run build 2>&1 | tail -3
npx playwright test --reporter=line 2>&1 | tail -3
```

Expected: all green.

- [ ] **Step 4: Append post-P5 bundle record to the baseline doc**

Open `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md` and append a new `## Post-P5` section summarizing:
- New lazy chunks: `SubscriptionsPage-*.js`, `GroupsPage-*.js` sizes
- `index-*.js` delta
- svelte-check error count change

- [ ] **Step 5: Commit**

```bash
git add gui/web/tests/subscriptions.spec.ts gui/web/tests/shell.spec.ts docs/superpowers/plans/
git commit -m "test(p5): rewrite subscriptions spec + add groups smoke + record bundle"
```

---

### Task 15: Push + update PR body

- [ ] **Step 1: Push**

```bash
git push origin refactor/gui-v2
```

- [ ] **Step 2: Update PR #8 body — append P5 section**

```bash
gh pr view 8 --json body -q .body > /tmp/pr-body.md
```

Append a section describing P5. Example body snippet:

```markdown
---

## P5 · Subscriptions + Groups (15 commits)

Plan: `docs/superpowers/plans/2026-04-20-gui-refactor-p5-subs-groups.md`

`#/subscriptions` and `#/groups` (+ `#/groups/:tag` detail) are now feature slices.

### Subscriptions
- `features/subscriptions/` (7 files): resource + table + row + expanded + AddDialog + page + index
- Dense 48-px table, hover-only refresh button per spec §7.3, reuse `DeleteConfirm` from features/servers
- Deleted legacy `pages/Subscriptions.svelte` (445 lines)

### Groups
- `features/groups/` (5 files): resource + card + detail + page + index
- 4-col auto-fill card grid with dashed "+ New group" stub per spec §7.4
- Group detail at `#/groups/:tag` — `matches('/groups/:tag')` inside `GroupsPage` branches to `GroupDetail`, avoiding a separate route registration
- Deleted legacy `pages/Groups.svelte` (389 lines)

### Results
svelte-check errors: **N → M** (−K from deleted legacy)
vitest: ** all pass** across X files
Playwright: ** all pass** across Y tests
Cumulative bundle gzip delta: still within +30 KB budget
```

Then:

```bash
gh pr edit 8 --body-file /tmp/pr-body.md
rm /tmp/pr-body.md
```

---

## Self-review notes

**Spec coverage.**
- §7.3 Subscriptions dense table + hover refresh → Tasks 2, 4
- §7.3 Row expand → Tasks 3, 4
- §7.3 Import format detection → client-side URL-based inference in Task 2 (conservative — backend auto-detects for real)
- §7.4 4-col card grid → Task 11
- §7.4 Strategy badge + member count + selected tag → Task 9 GroupCard
- §7.4 Click into `#/groups/:id` → Task 9 navigate + Task 10 GroupDetail + Task 11 routing
- §7.4 Dashed stub → Task 11
- §7.4 24-h quality sparkline — stub only ("coming soon"); backend doesn't expose history yet. Documented limitation.

**Placeholder scan.** All tasks have complete code blocks. Task 15 step 2 asks the engineer to hand-append a markdown section — the template is provided with placeholders `N → M` etc. that must be replaced with actual numbers from Task 14.

**Type consistency.**
- `Subscription`, `GroupInfo`, `GroupTestResult` all from `@/lib/api/types`.
- `useSubscriptions()` / `useGroups()` / `useGroup(tag)` signatures consistent.
- `onDelete` callbacks use `id: string` for subscriptions, matching the backend API.
- `Resource<T>` inherited from P1 `lib/resource.svelte.ts`.

**Explicit out-of-scope.**
- Edit subscription name/URL after add (backend has no update endpoint; deferred).
- Delete-many / batch ops on subscriptions (rare use case; deferred).
- Group create/edit UI (backend has no create endpoint; stub card documents).
- Real 24-h quality sparkline in GroupCard (backend doesn't expose per-member history).
- i18n polish for newly-added `groups.*` and `subscriptions.*` keys beyond what's in Tasks 7 + 12.

**Known risks.**
- `features/subscriptions/SubscriptionsPage.svelte` cross-imports `@/features/servers/DeleteConfirm.svelte`. This violates the convention that features don't import from other features. If this bothers review, move `DeleteConfirm` to `@/ui/` first. P4 chose not to promote because it seemed servers-specific; with reuse now confirmed, the right move is to promote. Plan currently takes the shortcut — flag in review.
- `useGroup(tag)` creates a new resource per tag via `createResource('groups.item.${tag}', ...)`. If many tags are visited, registry grows without eviction. Acceptable for P5 since groups count is usually small (<20); add eviction in P11 polish.
- `matches('/groups/:tag')` inside `GroupsPage.svelte` is called from a `$derived.by` — matchPath is pure so this is safe (per P1 fix). But when the URL changes from `/groups/X` to `/groups/Y`, `GroupsPage` does NOT remount — `GroupDetail` re-renders with the new params. Verify via Playwright when possible.

---

## Plan complete.

Plan complete and saved to `docs/superpowers/plans/2026-04-20-gui-refactor-p5-subs-groups.md`.

Execution:
- **Subagent-Driven** — fresh agent per task, review between
- **Inline** — execute here in this session with checkpoints
