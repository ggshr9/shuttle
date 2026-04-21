# GUI Refactor P7 · Mesh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate `pages/Mesh.svelte` + `lib/MeshTopologyChart.svelte` (2 files, 773 lines) into `features/mesh/` per spec §7.6 — keep the existing topology chart (already the most visually mature part of the legacy GUI), densify the peer list into the Servers-style table, and trim descriptive paragraphs by moving context into `ui/Tooltip`.

**Architecture:** Small feature slice. `resource.svelte.ts` polls `mesh/status` + `mesh/peers`. The existing canvas-based `MeshTopologyChart` moves verbatim (only CSS token rename) into the feature directory; spec acknowledges it's "already mature". A new `PeerRow` / `PeerTable` replace the legacy card list with a dense 48-px table matching P4 Servers. The "mesh not enabled" empty state and hub-IP / self-IP summary stay but tighten visually.

**Tech Stack:** Svelte 5 runes · `@/lib/resource` · `@/lib/api` · `@/ui` (Card, Button, Badge, Icon, Empty, Tooltip, AsyncBoundary).

**Spec reference:** `docs/superpowers/specs/2026-04-19-gui-refactor-design.md` §7.6.

**Branch:** Continue on `refactor/gui-v2`. P7 lands as commits on PR #8.

---

## File structure

```
gui/web/src/features/mesh/
├─ index.ts                   route + public API
├─ resource.svelte.ts         useStatus / usePeers + connectPeer action
├─ MeshPage.svelte            page orchestration
├─ TopologyChart.svelte       moved from lib/MeshTopologyChart.svelte (token-rename only)
├─ PeerTable.svelte           dense table shell
└─ PeerRow.svelte             48-px row per peer
```

6 files. Delete legacy `pages/Mesh.svelte` (413) + `lib/MeshTopologyChart.svelte` (360) = 773 lines.

---

## Conventions

- Relative paths from repo root.
- Each task ends with a commit.
- Follow P4/P5 patterns; no multi-select on peers (rarely useful).

---

## Section A · Data layer (Task 1)

### Task 1: `resource.svelte.ts`

**Files:**
- Create: `gui/web/src/features/mesh/resource.svelte.ts`

- [ ] **Step 1: Create**

```bash
mkdir -p gui/web/src/features/mesh
```

```ts
import { createResource, invalidate, type Resource } from '@/lib/resource.svelte'
import {
  meshStatus as apiStatus,
  meshPeers as apiPeers,
  meshConnectPeer as apiConnect,
} from '@/lib/api/endpoints'
import type { MeshStatus, MeshPeer } from '@/lib/api/types'
import { toasts } from '@/lib/toaster.svelte'
import { t } from '@/lib/i18n/index'

const STATUS_KEY = 'mesh.status'
const PEERS_KEY  = 'mesh.peers'

export function useStatus(): Resource<MeshStatus> {
  return createResource(STATUS_KEY, apiStatus, {
    poll: 10_000,
    initial: { enabled: false },
  })
}

export function usePeers(): Resource<MeshPeer[]> {
  return createResource(PEERS_KEY, apiPeers, {
    poll: 10_000,
    initial: [],
    enabled: () => useStatus().data?.enabled === true,
  })
}

export async function connectPeer(vip: string): Promise<void> {
  try {
    await apiConnect(vip)
    invalidate(PEERS_KEY)
    toasts.success(t('mesh.toast.connecting', { vip }))
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}
```

- [ ] **Step 2: svelte-check**

```bash
cd gui/web && npx svelte-check --threshold error 2>&1 | grep -E "ERROR.*features/mesh" | head -5
```

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/features/mesh/resource.svelte.ts
git commit -m "feat(mesh): resource layer (status + peers + connectPeer)"
```

---

## Section B · Components (Tasks 2-4)

### Task 2: Move `TopologyChart.svelte`

**Files:**
- Read: `gui/web/src/lib/MeshTopologyChart.svelte`
- Create: `gui/web/src/features/mesh/TopologyChart.svelte`

**Context:** Spec §7.6 calls this "already mature" — preserve the drawing logic. Port only: CSS vars (`--bg-*` / `--text-*` → `--shuttle-*`) and prop names if they use legacy types. The canvas rendering code stays byte-identical.

- [ ] **Step 1: Copy + token-rename**

Read the legacy file. Copy verbatim to the new path, then replace:
- `var(--bg-primary)` → `var(--shuttle-bg-base)`
- `var(--bg-secondary)` → `var(--shuttle-bg-surface)`
- `var(--bg-tertiary)` → `var(--shuttle-bg-subtle)`
- `var(--text-primary)` → `var(--shuttle-fg-primary)`
- `var(--text-secondary)` → `var(--shuttle-fg-secondary)`
- `var(--text-muted)` → `var(--shuttle-fg-muted)`
- `var(--border)` → `var(--shuttle-border)`
- `var(--accent)` → `var(--shuttle-accent)`
- `var(--accent-green)` → `var(--shuttle-success)`
- `var(--accent-red)` → `var(--shuttle-danger)`
- `var(--accent-yellow)` → `var(--shuttle-warning)`
- `var(--radius-sm)` → `var(--shuttle-radius-sm)`

Imports need updating if any are from `../lib/...`; swap to absolute paths under `@/lib/...`. The legacy file imports from `./i18n/index` — change to `@/lib/i18n/index`.

- [ ] **Step 2: svelte-check**

```bash
cd gui/web && npx svelte-check --threshold error 2>&1 | grep -E "ERROR.*features/mesh/TopologyChart" | head -5
```

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/features/mesh/TopologyChart.svelte
git commit -m "feat(mesh): move TopologyChart to feature slice with token migration"
```

---

### Task 3: `PeerRow.svelte`

**Files:**
- Create: `gui/web/src/features/mesh/PeerRow.svelte`

**Context:** 48-px dense row matching P4 Servers. Columns: state dot | VIP | connection method | avg RTT | packet loss | quality score | action (Connect if disconnected).

- [ ] **Step 1: Create**

```svelte
<script lang="ts">
  import { Button, Icon, Badge } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { connectPeer } from './resource.svelte'
  import type { MeshPeer } from '@/lib/api/types'

  interface Props { peer: MeshPeer }
  let { peer }: Props = $props()

  let busy = $state(false)

  async function doConnect() {
    busy = true
    try { await connectPeer(peer.virtual_ip) } finally { busy = false }
  }

  const stateClass = $derived(
    peer.state === 'connected' ? 'ok'
      : peer.state === 'connecting' ? 'warn'
      : peer.state === 'failed' ? 'bad'
      : 'unknown'
  )

  const stateVariant = $derived<'success' | 'warning' | 'danger' | 'neutral'>(
    peer.state === 'connected' ? 'success'
      : peer.state === 'connecting' ? 'warning'
      : peer.state === 'failed' ? 'danger'
      : 'neutral'
  )
</script>

<div class="row">
  <span class={`dot ${stateClass}`}></span>
  <span class="vip">{peer.virtual_ip}</span>
  <Badge variant={stateVariant}>{peer.state}</Badge>
  <span class="method">{peer.method ?? '—'}</span>
  <span class="rtt">{peer.avg_rtt_ms != null ? `${peer.avg_rtt_ms} ms` : '—'}</span>
  <span class="loss">{peer.packet_loss != null ? `${(peer.packet_loss * 100).toFixed(1)} %` : '—'}</span>
  <span class="score">{peer.score != null ? peer.score.toFixed(0) : '—'}</span>
  <span class="action">
    {#if peer.state !== 'connected'}
      <Button size="sm" variant="ghost" loading={busy} onclick={doConnect}>
        <Icon name="check" size={14} title={t('mesh.connect')} />
      </Button>
    {/if}
  </span>
</div>

<style>
  .row {
    display: grid;
    grid-template-columns: 16px 160px 90px 80px 80px 80px 60px auto;
    align-items: center;
    gap: var(--shuttle-space-3);
    height: 48px;
    padding: 0 var(--shuttle-space-4);
    border-top: 1px solid var(--shuttle-border);
    font-size: var(--shuttle-text-sm);
  }
  .row:first-child { border-top: 0; }
  .dot {
    width: 8px; height: 8px; border-radius: 50%;
    background: var(--shuttle-fg-muted);
  }
  .dot.ok   { background: var(--shuttle-success); }
  .dot.warn { background: var(--shuttle-warning); }
  .dot.bad  { background: var(--shuttle-danger); }
  .vip { font-family: var(--shuttle-font-mono); color: var(--shuttle-fg-primary); font-weight: var(--shuttle-weight-medium); }
  .method, .rtt, .loss, .score {
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-secondary);
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  .action { justify-self: end; }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/mesh/PeerRow.svelte
git commit -m "feat(mesh): dense 48px peer row"
```

---

### Task 4: `PeerTable.svelte`

**Files:**
- Create: `gui/web/src/features/mesh/PeerTable.svelte`

- [ ] **Step 1: Create**

```svelte
<script lang="ts">
  import { Empty, Card } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import PeerRow from './PeerRow.svelte'
  import type { MeshPeer } from '@/lib/api/types'

  interface Props { peers: MeshPeer[] }
  let { peers }: Props = $props()
</script>

{#if peers.length === 0}
  <Card>
    <Empty
      icon="mesh"
      title={t('mesh.empty.title')}
      description={t('mesh.empty.desc')}
    />
  </Card>
{:else}
  <div class="table">
    <div class="header">
      <span></span>
      <span>{t('mesh.columns.vip')}</span>
      <span>{t('mesh.columns.state')}</span>
      <span class="num">{t('mesh.columns.method')}</span>
      <span class="num">{t('mesh.columns.rtt')}</span>
      <span class="num">{t('mesh.columns.loss')}</span>
      <span class="num">{t('mesh.columns.score')}</span>
      <span></span>
    </div>
    {#each peers as p (p.virtual_ip)}
      <PeerRow peer={p} />
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
    grid-template-columns: 16px 160px 90px 80px 80px 80px 60px auto;
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
  .header .num { text-align: right; }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/mesh/PeerTable.svelte
git commit -m "feat(mesh): PeerTable with 7-column grid"
```

---

## Section C · Page + route (Task 5)

### Task 5: `MeshPage.svelte` + `index.ts` + route swap + i18n

**Files:**
- Create: `gui/web/src/features/mesh/MeshPage.svelte`
- Create: `gui/web/src/features/mesh/index.ts`
- Modify: `gui/web/src/app/routes.ts`
- Modify: `gui/web/src/locales/en.json`, `zh-CN.json`

- [ ] **Step 1: Create `MeshPage.svelte`**

```svelte
<script lang="ts">
  import { AsyncBoundary, Button, Card, Section, StatRow, Icon, Tooltip } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { useStatus, usePeers } from './resource.svelte'
  import TopologyChart from './TopologyChart.svelte'
  import PeerTable from './PeerTable.svelte'

  const statusRes = useStatus()
  const peersRes = usePeers()
</script>

<Section
  title={t('nav.mesh')}
  description={statusRes.data?.enabled ? t('mesh.count', { n: peersRes.data?.length ?? 0 }) : undefined}
>
  <AsyncBoundary resource={statusRes}>
    {#snippet children(status)}
      {#if !status.enabled}
        <Card>
          <div class="disabled">
            <h3>{t('mesh.disabled.title')}</h3>
            <p>{t('mesh.disabled.desc')}</p>
          </div>
        </Card>
      {:else}
        <Card>
          <div class="summary">
            <StatRow label={t('mesh.virtualIp')} value={status.virtual_ip ?? '—'} mono />
            <StatRow label={t('mesh.cidr')}      value={status.cidr ?? '—'} mono />
            <StatRow label={t('mesh.peerCount')} value={String(status.peer_count ?? 0)} />
          </div>
        </Card>

        <h3 class="section-head">
          {t('mesh.topology')}
          <Tooltip content={t('mesh.topologyTooltip')}>
            <Icon name="info" size={12} />
          </Tooltip>
        </h3>
        <Card>
          <TopologyChart peers={peersRes.data ?? []} selfIP={status.virtual_ip} />
        </Card>

        <h3 class="section-head">{t('mesh.peers')}</h3>
        <AsyncBoundary resource={peersRes}>
          {#snippet children(peers)}
            <PeerTable peers={peers} />
          {/snippet}
        </AsyncBoundary>
      {/if}
    {/snippet}
  </AsyncBoundary>
</Section>

<style>
  .disabled { text-align: center; padding: var(--shuttle-space-5); }
  .disabled h3 {
    margin: 0 0 var(--shuttle-space-2);
    font-size: var(--shuttle-text-base);
    color: var(--shuttle-fg-primary);
  }
  .disabled p {
    margin: 0;
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-muted);
  }
  .summary { display: grid; grid-template-columns: repeat(3, 1fr); gap: var(--shuttle-space-3); }
  .section-head {
    display: flex; align-items: center; gap: var(--shuttle-space-2);
    margin: var(--shuttle-space-5) 0 var(--shuttle-space-3);
    font-size: var(--shuttle-text-sm);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-primary);
  }
  .section-head :global(button) { color: var(--shuttle-fg-muted); }
</style>
```

**Note:** verify `TopologyChart`'s prop names after Task 2 — legacy uses `peers`, `selfIP`, and possibly `hubIP`. Adjust the pass-through accordingly.

- [ ] **Step 2: Create `index.ts`**

```ts
import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/mesh',
  component: lazy(() => import('./MeshPage.svelte')),
  nav: { label: 'nav.mesh', icon: 'mesh', order: 60 },
}

export { useStatus, usePeers } from './resource.svelte'
```

- [ ] **Step 3: Update `app/routes.ts`**

Add import + entry; remove the `/mesh` bridge:

```ts
import * as mesh from '@/features/mesh'
// ... in array, replace the Mesh bridge block with:
mesh.route,
```

- [ ] **Step 4: Add i18n keys**

```bash
cd gui/web/src/locales && python3 <<'PY'
import json
def merge(a, b):
    for k, v in b.items():
        if isinstance(v, dict) and isinstance(a.get(k), dict): merge(a[k], v)
        else: a[k] = v
    return a

EN = {
  "mesh": {
    "count": "{n} peers",
    "virtualIp": "Virtual IP",
    "cidr": "CIDR",
    "peerCount": "Peers",
    "topology": "Topology",
    "topologyTooltip": "Lines show active P2P links; dashed = relay fallback.",
    "peers": "Peers",
    "connect": "Connect",
    "disabled": {
      "title": "Mesh is not enabled",
      "desc": "Enable Mesh in Settings to join the overlay network."
    },
    "empty": {
      "title": "No peers",
      "desc":  "Waiting for peer discovery… make sure at least one peer is online."
    },
    "columns": {
      "vip":    "VIP",
      "state":  "State",
      "method": "Method",
      "rtt":    "RTT",
      "loss":   "Loss",
      "score":  "Score"
    },
    "toast": {
      "connecting": "Connecting to {vip}…"
    }
  }
}
ZH = {
  "mesh": {
    "count": "{n} 个对等节点",
    "virtualIp": "虚拟 IP",
    "cidr": "CIDR",
    "peerCount": "对等数",
    "topology": "拓扑",
    "topologyTooltip": "实线为活跃 P2P 连接;虚线为中继回退。",
    "peers": "对等节点",
    "connect": "连接",
    "disabled": {
      "title": "Mesh 未启用",
      "desc": "在设置中启用 Mesh 加入覆盖网络。"
    },
    "empty": {
      "title": "暂无对等节点",
      "desc":  "等待对等发现…确保至少有一个对等节点在线。"
    },
    "columns": {
      "vip":    "虚拟 IP",
      "state":  "状态",
      "method": "方式",
      "rtt":    "往返延迟",
      "loss":   "丢包",
      "score":  "质量"
    },
    "toast": {
      "connecting": "连接到 {vip}…"
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

- [ ] **Step 5: Build**

```bash
cd gui/web && npm run build 2>&1 | tail -3
```

- [ ] **Step 6: Commit**

```bash
git add gui/web/src/features/mesh/MeshPage.svelte gui/web/src/features/mesh/index.ts gui/web/src/app/routes.ts gui/web/src/locales/
git commit -m "feat(mesh): page composition + route swap + i18n"
```

---

## Section D · Cleanup + verification (Tasks 6-7)

### Task 6: Delete legacy

**Files:**
- Delete: `gui/web/src/pages/Mesh.svelte`
- Delete: `gui/web/src/lib/MeshTopologyChart.svelte`

- [ ] **Step 1: Confirm no remaining imports**

```bash
cd "/Users/homebot/Library/Mobile Documents/com~apple~CloudDocs/shuttle/gui/web"
grep -rEn "from '[./]*pages/Mesh\\.svelte|from '[./]*lib/MeshTopologyChart" src/ 2>/dev/null
```
Expected: no output.

- [ ] **Step 2: Delete + verify**

```bash
rm gui/web/src/pages/Mesh.svelte gui/web/src/lib/MeshTopologyChart.svelte
cd gui/web
npx svelte-check --threshold error 2>&1 | tail -1
npm run build 2>&1 | tail -3
```

- [ ] **Step 3: Commit**

```bash
git add -A gui/web/src/pages/Mesh.svelte gui/web/src/lib/MeshTopologyChart.svelte
git commit -m "refactor(gui): delete legacy Mesh page + MeshTopologyChart (773 lines)"
```

---

### Task 7: Playwright + bundle record + push + PR update

**Files:**
- Modify: `gui/web/tests/mesh.spec.ts` (rewrite)
- Modify: `gui/web/tests/shell.spec.ts` (add peer-list smoke, optional)
- Modify: `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md`

- [ ] **Step 1: Rewrite `mesh.spec.ts`** for new selectors

The legacy Mesh tests look for `h3:has-text("Peers")` and `h3:has-text("Network Topology")`. New page has `h3:has-text("Peers")` + `h3:has-text("Topology")` (dropped the "Network" prefix for spec §7.6 trim). Update:

```ts
import { test, expect } from '@playwright/test';

test.describe('Mesh Page', () => {
    test.beforeEach(async ({ page }) => {
        await page.goto('/#/mesh');
        await expect(page.locator('.sidebar')).toBeVisible();
    });

    test('mesh tab is visible in navigation', async ({ page }) => {
        const meshTab = page.locator('a.item:has-text("Mesh")');
        await expect(meshTab).toBeVisible();
    });

    test('mesh page renders', async ({ page }) => {
        // Backend is unreachable in tests, so the initial status load
        // fails; the page falls back to the AsyncBoundary error state.
        // Just verify the section title renders.
        await expect(page.locator('h3:has-text("Mesh")').first()).toBeVisible({ timeout: 5000 });
    });
});
```

- [ ] **Step 2: Full gate run**

```bash
cd gui/web
npx svelte-check --threshold error 2>&1 | tail -1
npm test 2>&1 | tail -3
./scripts/check-i18n.sh
npm run build 2>&1 | tail -3
npx playwright test --reporter=line 2>&1 | tail -3
```

- [ ] **Step 3: Append post-P7 bundle record** to the baseline doc with the new error/warning counts.

- [ ] **Step 4: Push + update PR body**

```bash
git push origin refactor/gui-v2
gh pr view 8 --json body -q .body > /tmp/pr-body.md
# Append ## P7 · Mesh section summarizing the feature + deletions + counts
gh pr edit 8 --body-file /tmp/pr-body.md
rm /tmp/pr-body.md
```

---

## Self-review notes

**Spec coverage.**
- §7.6 "保留拓扑图" → Task 2 moves TopologyChart verbatim, only token migration.
- §7.6 "对等节点改成密集表" → Tasks 3, 4 new PeerRow + PeerTable.
- §7.6 "多余说明 block 移到 Tooltip" → MeshPage (Task 5) wraps topology section header with `<Tooltip>` for the explanatory sentence.

**Placeholder scan.** Every task has concrete code. Task 2 refers the engineer to read the legacy file — expected; the port is mechanical.

**Type consistency.**
- `MeshStatus`, `MeshPeer` from `@/lib/api/types`.
- `useStatus() / usePeers() / connectPeer()` consistent across tasks.

**Out of scope.**
- Manual peer removal (backend doesn't support; peers disappear on TTL).
- Per-peer traffic stats (backend doesn't expose yet).
- Topology chart redesign — spec says keep as-is.
- i18n of legacy mesh strings that existed prior to this plan; we migrate only what the new feature uses.

**Known risks.**
- TopologyChart's prop names might differ from my pass-through in MeshPage — the legacy file is the source of truth. Verify at Task 5 step 1.
- Legacy `mesh.spec.ts` asserts `h3:has-text("Network Topology")` which changes to "Topology". Update in Task 7.

---

## Plan complete.

Plan saved to `docs/superpowers/plans/2026-04-21-gui-refactor-p7-mesh.md`.

Execution:
- **Subagent-Driven** — fresh agent per task, review between
- **Inline** — execute here in this session with checkpoints
