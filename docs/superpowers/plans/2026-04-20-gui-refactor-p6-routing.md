# GUI Refactor P6 · Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate `pages/Routing.svelte` + `lib/routing/*` (7 files, 1271 lines) into a feature slice at `features/routing/` per spec §7.5 — preserve feature parity (rule CRUD, templates, import/export, test URL, GeoSite autocomplete, per-app picker), swap to new `@/ui` primitives where it lowers code, and add a rule-hit visualization bar.

**Architecture:** Structural migration (move + port), not a full redesign. Legacy files move to `features/routing/` and switch their CSS vars from `--bg-*` / `--text-*` / `--border` to `--shuttle-*` while keeping layout. The big primitives (Dialog, Combobox) replace the custom modal / autocomplete constructs where the existing shape maps cleanly; the deeper list editor (RuleList) keeps its current structure, only cosmetic tightening. The new visualization (RuleHitBar) is a stub: backend exposes no cumulative rule-hit counts yet, so the bar renders from rule type/action distribution for now — documented in the PR as a placeholder.

**Tech Stack:** Svelte 5 runes · `@/lib/resource` · `@/lib/api` · `@/ui` (Card, Button, Input, Select, Dialog, Combobox, Badge, AsyncBoundary).

**Spec reference:** `docs/superpowers/specs/2026-04-19-gui-refactor-design.md` §7.5.

**Branch:** Continue on `refactor/gui-v2`. P6 lands as commits on PR #8.

---

## Scope discipline

**In scope:**
- All 7 legacy files move into `features/routing/` and are renamed per the feature-slice convention (`Routing<Thing>.svelte` → `<Thing>.svelte` inside the feature dir).
- `--bg-*` / `--text-*` / etc. → `--shuttle-*` token migration inside the moved files.
- `RoutingConfirmModal` + `RoutingTemplateModal` + `RoutingImportExport`'s confirm → replaced by `@/ui/Dialog`.
- `GeoSite` picker switches to `@/ui/Combobox` for autocomplete.
- New `RuleHitBar.svelte` — a thin visual strip at top of page showing per-rule type distribution.
- New `features/routing/resource.svelte.ts` — wraps `getRouting` / `putRouting` / `getRoutingTemplates` / `getGeositeCategories` / `getProcesses`.
- Rule list operations (add / edit / delete / reorder) keep their current logic, wrapped with `@/ui/Button` / `Input` / `Select`.
- `RoutingTestPanel` stays structurally; styling touches only where trivial.

**Out of scope (deferred to P11 polish or a later PR):**
- Deep redesign of the rule editor row (it's already compact and works).
- Server-side rule-hit counters — need backend work; bar is visual-only for P6.
- Drag-and-drop reorder (existing uses ↑/↓ buttons; keep).
- Rule conflict viewer from `/api/routing/conflicts` — exists in backend but not in current UI; defer.
- Per-subscription or per-path rule overrides — out of current scope entirely.

---

## File structure

```
gui/web/src/features/routing/
├─ index.ts                      route + public API
├─ resource.svelte.ts            all routing data + mutations
├─ RoutingPage.svelte            page orchestration (was pages/Routing.svelte)
├─ RuleList.svelte               rule table (from lib/routing/RuleList.svelte)
├─ RuleHitBar.svelte             NEW — visual distribution strip
├─ TestPanel.svelte              test-URL panel (from lib/routing/RoutingTestPanel.svelte)
├─ TemplateDialog.svelte         template picker (from lib/routing/RoutingTemplateModal.svelte)
├─ ImportExportDialog.svelte     import/export (from lib/routing/RoutingImportExport.svelte)
├─ ProcessPicker.svelte          process autocomplete (from lib/routing/RoutingProcessPicker.svelte)
└─ ConfirmDialog.svelte          small confirm (from lib/routing/RoutingConfirmModal.svelte)
```

10 files. Big — accept this because the legacy surface is big.

**Reuses** `features/servers/DeleteConfirm.svelte` where single-confirm is enough (not Templates' "this replaces all rules" — that's a different semantic, use its own).

**Deletions** after ship: `pages/Routing.svelte` (289) + `lib/routing/*` (1060) = 1349 lines.

---

## Conventions

- Relative paths from repo root.
- Each task ends with a commit.
- Migrations preserve behavior first, cosmetic second.
- Add i18n keys alongside any new user-visible strings.

---

## Section A · Data layer (Task 1)

### Task 1: `features/routing/resource.svelte.ts`

**Files:**
- Create: `gui/web/src/features/routing/resource.svelte.ts`

- [ ] **Step 1: Create**

```bash
mkdir -p gui/web/src/features/routing
```

```ts
import { createResource, invalidate, type Resource } from '@/lib/resource.svelte'
import {
  getRouting,
  putRouting as apiPut,
  getRoutingTemplates,
  applyRoutingTemplate as apiApplyTpl,
  getGeositeCategories,
  getProcesses,
  importRouting as apiImport,
  testRouting as apiTest,
  exportRouting,
} from '@/lib/api/endpoints'
import type {
  RoutingRules, RoutingTemplate, Process, DryRunResult,
} from '@/lib/api/types'
import { toasts } from '@/lib/toaster.svelte'
import { t } from '@/lib/i18n/index'

const RULES_KEY = 'routing.rules'

// Rules live resource — 30 s poll is enough; rules rarely change outside the
// page itself.
export function useRules(): Resource<RoutingRules> {
  return createResource(RULES_KEY, getRouting, {
    poll: 30_000,
    initial: { rules: [], default: 'proxy' },
  })
}

export async function saveRules(rules: RoutingRules): Promise<void> {
  try {
    await apiPut(rules)
    invalidate(RULES_KEY)
    toasts.success(t('routing.toast.saved'))
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}

// Templates + GeoSite categories + Processes: fetched on demand, small lists.
export function useTemplates(): Resource<RoutingTemplate[]> {
  return createResource('routing.templates', getRoutingTemplates, { initial: [] })
}

export function useCategories(): Resource<string[]> {
  return createResource('routing.geosite.categories', getGeositeCategories, {
    initial: [],
  })
}

export function useProcesses(): Resource<Process[]> {
  return createResource('routing.processes', getProcesses, { initial: [] })
}

// Mutations that don't fit the polled-resource pattern.
export async function applyTemplate(id: string): Promise<void> {
  try {
    await apiApplyTpl(id)
    invalidate(RULES_KEY)
    toasts.success(t('routing.toast.templateApplied'))
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}

export async function importRules(
  rules: RoutingRules,
  mode: 'merge' | 'replace',
): Promise<{ added: number; total: number } | null> {
  try {
    const r = await apiImport(rules, mode)
    invalidate(RULES_KEY)
    toasts.success(t('routing.toast.imported', { added: r.added, total: r.total }))
    return r
  } catch (e) {
    toasts.error((e as Error).message)
    return null
  }
}

export async function testUrl(url: string): Promise<DryRunResult | null> {
  try {
    return await apiTest(url)
  } catch (e) {
    toasts.error((e as Error).message)
    return null
  }
}

export const exportHref = exportRouting
```

- [ ] **Step 2: svelte-check**

```bash
cd gui/web && npx svelte-check --threshold error 2>&1 | grep -E "ERROR.*features/routing" | head -5
```

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/features/routing/resource.svelte.ts
git commit -m "feat(routing): resource layer (rules + templates + categories + processes)"
```

---

## Section B · Port sub-components (Tasks 2-8)

Each task below is a rename-and-port: take a legacy file from `lib/routing/`, copy into `features/routing/` under the shorter name, and replace CSS vars + inline custom widgets with the equivalent from `@/ui`. Keep the logic untouched where possible.

### Task 2: `RuleList.svelte`

**Files:**
- Read: `gui/web/src/lib/routing/RuleList.svelte`
- Create: `gui/web/src/features/routing/RuleList.svelte`

- [ ] **Step 1: Read the legacy file and port**

Read the legacy `gui/web/src/lib/routing/RuleList.svelte` in full, then create the new file at `gui/web/src/features/routing/RuleList.svelte` with the same props + events but:
- Replace legacy `<input>` elements with `<Input>` from `@/ui` where they're labeled user inputs. For the inline cell inputs (compact), keep native `<input>` but apply our style conventions.
- Replace buttons with `<Button size="sm" variant="ghost">`.
- Replace the `--bg-*` / `--text-*` / `--border*` CSS vars with the `--shuttle-*` equivalents (the legacy compat layer in `app.css` still resolves old vars, so a verbatim copy would work; but migrate to be explicit about the token surface).
- Replace the `GeoSite` autocomplete `<datalist>` with `@/ui/Combobox`.
- Replace the `process` field autocomplete the same way.

The component contract stays the same: emits rule add / edit / delete via callbacks.

Full file available for reference at the given path. Port line-by-line, keep the same structure; don't re-architect.

- [ ] **Step 2: svelte-check**

```bash
cd gui/web && npx svelte-check --threshold error 2>&1 | grep -E "ERROR.*features/routing/RuleList" | head -5
```

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/features/routing/RuleList.svelte
git commit -m "feat(routing): port RuleList to @/ui primitives"
```

---

### Task 3: `ConfirmDialog.svelte`

**Files:**
- Read: `gui/web/src/lib/routing/RoutingConfirmModal.svelte`
- Create: `gui/web/src/features/routing/ConfirmDialog.svelte`

- [ ] **Step 1: Create**

Rewrite as a thin `@/ui/Dialog` wrapper. The legacy component is 80 lines of custom modal; the new one is ~30 lines matching `DeleteConfirm.svelte` from P4's servers feature:

```svelte
<script lang="ts">
  import { Dialog, Button } from '@/ui'
  import { t } from '@/lib/i18n/index'

  interface Props {
    open: boolean
    title: string
    description?: string
    confirmLabel?: string
    cancelLabel?: string
    danger?: boolean
    onConfirm: () => Promise<void> | void
  }
  let {
    open = $bindable(false),
    title,
    description,
    confirmLabel,
    cancelLabel,
    danger = false,
    onConfirm,
  }: Props = $props()

  let busy = $state(false)

  async function confirm() {
    busy = true
    try { await onConfirm(); open = false } finally { busy = false }
  }
</script>

<Dialog bind:open {title} {description}>
  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>
      {cancelLabel ?? t('common.cancel')}
    </Button>
    <Button variant={danger ? 'danger' : 'primary'} loading={busy} onclick={confirm}>
      {confirmLabel ?? t('common.ok', { default: 'OK' })}
    </Button>
  {/snippet}
</Dialog>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/routing/ConfirmDialog.svelte
git commit -m "feat(routing): port ConfirmModal to ui/Dialog wrapper"
```

---

### Task 4: `TemplateDialog.svelte`

**Files:**
- Read: `gui/web/src/lib/routing/RoutingTemplateModal.svelte`
- Create: `gui/web/src/features/routing/TemplateDialog.svelte`

- [ ] **Step 1: Create**

```svelte
<script lang="ts">
  import { Dialog, Button } from '@/ui'
  import { AsyncBoundary } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { useTemplates, applyTemplate } from './resource.svelte'

  interface Props {
    open: boolean
    onConfirm?: (id: string) => void
  }
  let { open = $bindable(false), onConfirm }: Props = $props()

  const tpls = useTemplates()
  let selectedId = $state<string | null>(null)
  let busy = $state(false)

  async function apply() {
    if (!selectedId) return
    busy = true
    try {
      await applyTemplate(selectedId)
      onConfirm?.(selectedId)
      open = false
    } finally {
      busy = false
    }
  }
</script>

<Dialog
  bind:open
  title={t('routing.templates.title')}
  description={t('routing.templates.desc')}
>
  <AsyncBoundary resource={tpls}>
    {#snippet children(list)}
      {#if list.length === 0}
        <p class="empty">{t('routing.templates.empty')}</p>
      {:else}
        <ul class="list">
          {#each list as tpl}
            <li>
              <label>
                <input
                  type="radio"
                  name="tpl"
                  value={tpl.id}
                  checked={selectedId === tpl.id}
                  onchange={() => (selectedId = tpl.id)}
                />
                <span class="name">{tpl.name}</span>
                <span class="desc">{tpl.description}</span>
              </label>
            </li>
          {/each}
        </ul>
      {/if}
    {/snippet}
  </AsyncBoundary>

  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>{t('common.cancel')}</Button>
    <Button variant="primary" disabled={!selectedId} loading={busy} onclick={apply}>
      {t('routing.templates.apply')}
    </Button>
  {/snippet}
</Dialog>

<style>
  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--shuttle-space-2); }
  label {
    display: grid; grid-template-columns: 16px 1fr; gap: var(--shuttle-space-2);
    padding: var(--shuttle-space-2);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
    cursor: pointer;
  }
  label:has(input:checked) {
    border-color: var(--shuttle-accent);
    background: var(--shuttle-bg-subtle);
  }
  .name {
    font-weight: var(--shuttle-weight-medium); color: var(--shuttle-fg-primary);
    grid-row: 1; grid-column: 2;
  }
  .desc {
    font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-muted);
    grid-row: 2; grid-column: 2;
  }
  .empty { color: var(--shuttle-fg-muted); text-align: center; }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/routing/TemplateDialog.svelte
git commit -m "feat(routing): port TemplateModal to Dialog + AsyncBoundary"
```

---

### Task 5: `ImportExportDialog.svelte`

**Files:**
- Read: `gui/web/src/lib/routing/RoutingImportExport.svelte`
- Create: `gui/web/src/features/routing/ImportExportDialog.svelte`

- [ ] **Step 1: Create**

The legacy file is 424 lines — drag-drop zone + parse + merge/replace. Port verbatim but wrap the whole thing in `<Dialog>` (instead of the custom modal shell) and replace the drag-drop zone's CSS vars with `--shuttle-*`. Keep the parsing logic identical. The new file is expected to be ~350 lines (save the modal chrome).

```svelte
<script lang="ts">
  import { Dialog, Button, Icon } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { importRules, exportHref } from './resource.svelte'
  import type { RoutingRules } from '@/lib/api/types'

  interface Props {
    open: boolean
    currentRules: RoutingRules
  }
  let { open = $bindable(false), currentRules }: Props = $props()

  let mode = $state<'merge' | 'replace'>('merge')
  let parsed = $state<RoutingRules | null>(null)
  let parseError = $state<string | null>(null)
  let dragOver = $state(false)
  let submitting = $state(false)

  function parseJson(text: string) {
    parseError = null
    parsed = null
    if (!text.trim()) return
    try {
      const obj = JSON.parse(text)
      if (!obj || !Array.isArray(obj.rules)) {
        parseError = t('routing.importExport.invalidFile')
        return
      }
      parsed = obj as RoutingRules
    } catch (e) {
      parseError = (e as Error).message
    }
  }

  async function onFile(f: File) {
    parseJson(await f.text())
  }

  function onPaste(ev: Event) {
    parseJson((ev.target as HTMLTextAreaElement).value)
  }

  async function doImport() {
    if (!parsed) return
    submitting = true
    try {
      await importRules(parsed, mode)
      open = false
    } finally {
      submitting = false
    }
  }

  function doExport() {
    window.location.href = exportHref()
  }
</script>

<Dialog bind:open title={t('routing.importExport.title')} description={t('routing.importExport.desc')}>
  <div class="col">
    <div
      class="dropzone"
      class:over={dragOver}
      ondragover={(e) => { e.preventDefault(); dragOver = true }}
      ondragleave={() => (dragOver = false)}
      ondrop={(e) => {
        e.preventDefault()
        dragOver = false
        const f = e.dataTransfer?.files?.[0]
        if (f) onFile(f)
      }}
      role="region"
      aria-label={t('routing.importExport.dropHint')}
    >
      <Icon name="plus" size={20} />
      <span>{t('routing.importExport.dropHint')}</span>
    </div>

    <textarea
      oninput={onPaste}
      placeholder={t('routing.importExport.pastePlaceholder')}
      rows="5"
    ></textarea>

    {#if parseError}
      <div class="err">{parseError}</div>
    {:else if parsed}
      <div class="ok">{t('routing.importExport.parsed', { n: parsed.rules.length })}</div>
    {/if}

    <div class="mode">
      <label><input type="radio" name="mode" value="merge" checked={mode === 'merge'} onchange={() => (mode = 'merge')} />{t('routing.importExport.merge')}</label>
      <label><input type="radio" name="mode" value="replace" checked={mode === 'replace'} onchange={() => (mode = 'replace')} />{t('routing.importExport.replace')}</label>
    </div>

    <div class="export">
      <Button variant="secondary" onclick={doExport}>{t('routing.importExport.export')}</Button>
      <span class="hint">{t('routing.importExport.exportHint')}</span>
    </div>
  </div>

  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>{t('common.cancel')}</Button>
    <Button variant="primary" disabled={!parsed} loading={submitting} onclick={doImport}>
      {t('routing.importExport.import')}
    </Button>
  {/snippet}
</Dialog>

<style>
  .col { display: flex; flex-direction: column; gap: var(--shuttle-space-3); }
  .dropzone {
    display: flex; flex-direction: column; align-items: center; justify-content: center;
    padding: var(--shuttle-space-5);
    border: 2px dashed var(--shuttle-border-strong);
    border-radius: var(--shuttle-radius-md);
    color: var(--shuttle-fg-muted);
    font-size: var(--shuttle-text-sm);
    gap: var(--shuttle-space-2);
    transition: border-color var(--shuttle-duration);
  }
  .dropzone.over { border-color: var(--shuttle-accent); color: var(--shuttle-fg-primary); }
  textarea {
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    color: var(--shuttle-fg-primary);
    padding: var(--shuttle-space-3);
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-xs);
    outline: none;
    resize: vertical;
  }
  textarea:focus { border-color: var(--shuttle-border-strong); }
  .err { color: var(--shuttle-danger); font-size: var(--shuttle-text-sm); }
  .ok  { color: var(--shuttle-success); font-size: var(--shuttle-text-sm); }
  .mode { display: flex; gap: var(--shuttle-space-3); font-size: var(--shuttle-text-sm); }
  .export { display: flex; align-items: center; gap: var(--shuttle-space-2); border-top: 1px solid var(--shuttle-border); padding-top: var(--shuttle-space-3); }
  .hint { font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-muted); }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/routing/ImportExportDialog.svelte
git commit -m "feat(routing): port ImportExport to Dialog + drop zone"
```

---

### Task 6: `TestPanel.svelte`

**Files:**
- Read: `gui/web/src/lib/routing/RoutingTestPanel.svelte`
- Create: `gui/web/src/features/routing/TestPanel.svelte`

- [ ] **Step 1: Create**

Port verbatim with @/ui Button + Input, keep the rest:

```svelte
<script lang="ts">
  import { Card, Input, Button, Badge } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { testUrl } from './resource.svelte'
  import type { DryRunResult } from '@/lib/api/types'

  let url = $state('')
  let busy = $state(false)
  let result = $state<DryRunResult | null>(null)

  async function run() {
    if (!url.trim()) return
    busy = true
    try {
      result = await testUrl(url.trim())
    } finally {
      busy = false
    }
  }

  const variant = $derived(
    !result ? 'neutral'
      : result.action === 'proxy'  ? 'info'
      : result.action === 'direct' ? 'success'
      : 'danger'
  )
</script>

<Card>
  <h3>{t('routing.test.title')}</h3>
  <div class="row">
    <Input placeholder={t('routing.test.placeholder')} bind:value={url} />
    <Button variant="primary" loading={busy} onclick={run}>{t('routing.test.test')}</Button>
  </div>

  {#if result}
    <div class="result">
      <Badge variant={variant as 'success' | 'info' | 'danger'}>{result.action}</Badge>
      <span class="domain">{result.domain}</span>
      <span class="matched">
        {t('routing.test.matchedBy', { rule: result.matched_by, detail: result.rule ?? '' })}
      </span>
    </div>
  {/if}
</Card>

<style>
  h3 {
    margin: 0 0 var(--shuttle-space-3);
    font-size: var(--shuttle-text-sm);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-primary);
  }
  .row { display: grid; grid-template-columns: 1fr auto; gap: var(--shuttle-space-2); align-items: end; }
  .result {
    display: flex; align-items: center; gap: var(--shuttle-space-2);
    margin-top: var(--shuttle-space-3); font-size: var(--shuttle-text-sm);
  }
  .domain { font-family: var(--shuttle-font-mono); color: var(--shuttle-fg-primary); }
  .matched { color: var(--shuttle-fg-muted); font-size: var(--shuttle-text-xs); margin-left: auto; }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/routing/TestPanel.svelte
git commit -m "feat(routing): port TestPanel with inline result badge"
```

---

### Task 7: `ProcessPicker.svelte`

**Files:**
- Read: `gui/web/src/lib/routing/RoutingProcessPicker.svelte`
- Create: `gui/web/src/features/routing/ProcessPicker.svelte`

- [ ] **Step 1: Create**

Same shape as legacy; wrap in `<Dialog>` + use `<Combobox>` + `<Button>`:

```svelte
<script lang="ts">
  import { Dialog, Button, Combobox } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { useProcesses } from './resource.svelte'

  interface Props {
    open: boolean
    onPick: (name: string) => void
  }
  let { open = $bindable(false), onPick }: Props = $props()

  const procs = useProcesses()
  let picked = $state<string | undefined>(undefined)

  function confirm() {
    if (picked) {
      onPick(picked)
      open = false
      picked = undefined
    }
  }
</script>

<Dialog bind:open title={t('routing.process.title')} description={t('routing.process.desc')}>
  {#if procs.data && procs.data.length > 0}
    <Combobox
      value={picked}
      items={procs.data.map((p) => ({ value: p.name, label: `${p.name} (${p.conns})` }))}
      onValueChange={(v) => (picked = v ?? undefined)}
    />
  {:else}
    <p class="hint">{t('routing.process.none')}</p>
  {/if}

  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>{t('common.cancel')}</Button>
    <Button variant="primary" disabled={!picked} onclick={confirm}>{t('routing.process.pick')}</Button>
  {/snippet}
</Dialog>

<style>
  .hint { color: var(--shuttle-fg-muted); font-size: var(--shuttle-text-sm); text-align: center; }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/routing/ProcessPicker.svelte
git commit -m "feat(routing): port ProcessPicker to Dialog + Combobox"
```

---

### Task 8: `RuleHitBar.svelte`

**Files:**
- Create: `gui/web/src/features/routing/RuleHitBar.svelte`

**Context:** New per-spec §7.5. Backend doesn't expose cumulative hit counts, so the bar is rule-type-distributed for P6 — each rule gets a segment proportional to its kind's visual weight (geosite = 3, geoip = 2, domain = 1, process = 1). Colored by `action` (proxy / direct / reject). Hover shows `rule[i]` identifier + action. Real hit counts arrive in a later PR when the backend endpoint exists.

- [ ] **Step 1: Create**

```svelte
<script lang="ts">
  import { t } from '@/lib/i18n/index'
  import type { RoutingRule } from '@/lib/api/types'

  interface Props { rules: RoutingRule[] }
  let { rules }: Props = $props()

  // Visual weight per rule kind (larger = more "visible" in the bar).
  function weight(r: RoutingRule): number {
    if (r.geosite) return 3
    if (r.geoip) return 2
    if (r.domain) return 1
    if (r.process) return 1
    return 1
  }

  function color(action: string): string {
    switch (action) {
      case 'proxy':   return 'var(--shuttle-info)'
      case 'direct':  return 'var(--shuttle-success)'
      case 'reject':  return 'var(--shuttle-danger)'
      default:        return 'var(--shuttle-fg-muted)'
    }
  }

  function label(r: RoutingRule, i: number): string {
    const kind = r.geosite ? `geosite:${r.geosite}`
      : r.geoip ? `geoip:${r.geoip}`
      : r.domain ? `domain:${r.domain}`
      : r.process ? `proc:${r.process}`
      : 'fallthrough'
    return `#${i + 1} ${kind} → ${r.action}`
  }

  const total = $derived(rules.reduce((s, r) => s + weight(r), 0) || 1)
</script>

<div class="bar" aria-label={t('routing.hitBar.label')}>
  {#each rules as r, i}
    <div
      class="seg"
      style="flex: {weight(r)}; background: {color(r.action)}"
      title={label(r, i)}
    ></div>
  {/each}
</div>
{#if rules.length === 0}
  <div class="empty">{t('routing.hitBar.empty')}</div>
{/if}

<style>
  .bar {
    display: flex; width: 100%; height: 10px;
    border-radius: var(--shuttle-radius-sm);
    overflow: hidden;
    background: var(--shuttle-bg-subtle);
    border: 1px solid var(--shuttle-border);
    margin-bottom: var(--shuttle-space-3);
  }
  .seg {
    transition: flex var(--shuttle-duration);
    cursor: help;
  }
  .seg:hover { filter: brightness(1.15); }
  .empty {
    height: 10px; width: 100%; margin-bottom: var(--shuttle-space-3);
    border: 1px dashed var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
  }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/routing/RuleHitBar.svelte
git commit -m "feat(routing): add RuleHitBar — visual rule distribution strip"
```

---

## Section C · Page composition + route (Tasks 9-10)

### Task 9: `RoutingPage.svelte`

**Files:**
- Read: `gui/web/src/pages/Routing.svelte`
- Create: `gui/web/src/features/routing/RoutingPage.svelte`

- [ ] **Step 1: Create**

Port the page from `pages/Routing.svelte`. Replace:
- `import { api } from '../lib/api'` → direct imports from `@/lib/api/endpoints` (actually resource-style — use `useRules`).
- `import { toast } from '../lib/toast'` → `import { toasts } from '@/lib/toaster.svelte'`.
- Sub-component imports from `../lib/routing/...` → from `./...` (features local).
- Loading state → `AsyncBoundary`.
- Action buttons at top → `@/ui/Button`.

Structure:

```svelte
<script lang="ts">
  import { AsyncBoundary, Button, Icon, Section, Select } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { useRules, saveRules } from './resource.svelte'
  import RuleList from './RuleList.svelte'
  import RuleHitBar from './RuleHitBar.svelte'
  import TemplateDialog from './TemplateDialog.svelte'
  import ImportExportDialog from './ImportExportDialog.svelte'
  import TestPanel from './TestPanel.svelte'
  import type { RoutingRules, RoutingRule } from '@/lib/api/types'

  const res = useRules()

  let draft = $state<RoutingRules | null>(null)
  let saving = $state(false)
  let tplOpen = $state(false)
  let ioOpen = $state(false)

  // Initialize draft from remote state when it arrives / changes.
  $effect(() => {
    if (res.data && !draft) draft = structuredClone(res.data)
  })

  async function save() {
    if (!draft) return
    saving = true
    try {
      await saveRules(draft)
    } finally {
      saving = false
    }
  }

  function onRulesChange(rules: RoutingRule[]) {
    if (draft) draft = { ...draft, rules }
  }

  function onDefaultChange(v: string) {
    if (draft) draft = { ...draft, default: v }
  }
</script>

<Section
  title={t('nav.routing')}
  description={res.data ? t('routing.count', { n: res.data.rules.length }) : undefined}
>
  {#snippet actions()}
    <Button variant="ghost" onclick={() => (tplOpen = true)}>{t('routing.applyTemplate')}</Button>
    <Button variant="ghost" onclick={() => (ioOpen = true)}>{t('routing.importExport.open')}</Button>
    <Button variant="primary" loading={saving} onclick={save}>{t('common.save')}</Button>
  {/snippet}

  <AsyncBoundary resource={res}>
    {#snippet children(_remote)}
      {#if draft}
        <RuleHitBar rules={draft.rules} />

        <div class="default-row">
          <span class="label">{t('routing.default')}</span>
          <Select
            value={draft.default}
            options={[
              { value: 'proxy',  label: t('routing.action.proxy') },
              { value: 'direct', label: t('routing.action.direct') },
              { value: 'reject', label: t('routing.action.reject') },
            ]}
            onValueChange={onDefaultChange}
          />
        </div>

        <RuleList rules={draft.rules} onChange={onRulesChange} />

        <TestPanel />
      {/if}
    {/snippet}
  </AsyncBoundary>
</Section>

<TemplateDialog bind:open={tplOpen} />
{#if draft}
  <ImportExportDialog bind:open={ioOpen} currentRules={draft} />
{/if}

<style>
  .default-row {
    display: flex; align-items: center; gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    margin-bottom: var(--shuttle-space-3);
  }
  .label {
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-secondary);
  }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/features/routing/RoutingPage.svelte
git commit -m "feat(routing): compose RoutingPage with hit bar + draft + save"
```

---

### Task 10: `index.ts` + route swap + i18n keys

**Files:**
- Create: `gui/web/src/features/routing/index.ts`
- Modify: `gui/web/src/app/routes.ts`
- Modify: `gui/web/src/locales/en.json`, `zh-CN.json`

- [ ] **Step 1: Create `index.ts`**

```ts
import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/routing',
  component: lazy(() => import('./RoutingPage.svelte')),
  nav: { label: 'nav.routing', icon: 'routing', order: 50 },
}

export { useRules } from './resource.svelte'
```

- [ ] **Step 2: Update `app/routes.ts`**

Replace the Routing bridge line with `routing.route`:

```ts
import * as routing from '@/features/routing'
// ... in the array:
groups.route,
groups.detailRoute,
routing.route,
// remove the old { path: '/routing', component: lazy(() => import('@/pages/Routing.svelte')), ... }
```

- [ ] **Step 3: Add Routing i18n keys**

```bash
cd gui/web/src/locales && python3 <<'PY'
import json
def merge(a, b):
    for k, v in b.items():
        if isinstance(v, dict) and isinstance(a.get(k), dict): merge(a[k], v)
        else: a[k] = v
    return a

EN = {
  "routing": {
    "count": "{n} rules",
    "default": "Default action",
    "applyTemplate": "Apply template",
    "action": { "proxy": "Proxy", "direct": "Direct", "reject": "Reject" },
    "hitBar": {
      "label": "Rule distribution",
      "empty": "No rules defined"
    },
    "templates": {
      "title": "Apply routing template",
      "desc": "This will replace the current rules with the template's ruleset.",
      "empty": "No templates available.",
      "apply": "Apply"
    },
    "importExport": {
      "title": "Import / export rules",
      "desc":  "Paste JSON, drop a file, or export the current ruleset.",
      "open":  "Import / export",
      "dropHint": "Drop a .json file here, or paste JSON below.",
      "pastePlaceholder": "{ \"rules\": [...], \"default\": \"proxy\" }",
      "invalidFile": "Invalid JSON — expected an object with a \"rules\" array.",
      "parsed": "Parsed {n} rules.",
      "merge": "Merge with existing",
      "replace": "Replace all",
      "import": "Import",
      "export": "Download current rules",
      "exportHint": "JSON, downloads immediately."
    },
    "test": {
      "title": "Test URL",
      "placeholder": "https://example.com/some/path",
      "test": "Test",
      "matchedBy": "matched by {rule} {detail}"
    },
    "process": {
      "title": "Pick a process",
      "desc":  "Choose a running process to use as a rule target.",
      "none":  "No processes with active connections.",
      "pick":  "Use"
    },
    "toast": {
      "saved":   "Routing saved",
      "templateApplied": "Template applied",
      "imported": "Imported {added} of {total} rules"
    }
  }
}
ZH = {
  "routing": {
    "count": "{n} 条规则",
    "default": "默认行为",
    "applyTemplate": "应用模板",
    "action": { "proxy": "代理", "direct": "直连", "reject": "拒绝" },
    "hitBar": {
      "label": "规则分布",
      "empty": "尚未定义规则"
    },
    "templates": {
      "title": "应用路由模板",
      "desc":  "这将用模板规则替换当前所有规则。",
      "empty": "暂无可用模板。",
      "apply": "应用"
    },
    "importExport": {
      "title": "导入 / 导出规则",
      "desc":  "粘贴 JSON、拖放文件,或导出当前规则集。",
      "open":  "导入 / 导出",
      "dropHint": "拖放 .json 文件到此处,或在下方粘贴 JSON。",
      "pastePlaceholder": "{ \"rules\": [...], \"default\": \"proxy\" }",
      "invalidFile": "无效 JSON — 需要包含 \"rules\" 数组的对象。",
      "parsed": "已解析 {n} 条规则。",
      "merge": "与现有规则合并",
      "replace": "替换全部",
      "import": "导入",
      "export": "下载当前规则",
      "exportHint": "JSON 格式,立即下载。"
    },
    "test": {
      "title": "测试 URL",
      "placeholder": "https://example.com/some/path",
      "test": "测试",
      "matchedBy": "匹配规则:{rule} {detail}"
    },
    "process": {
      "title": "选择进程",
      "desc":  "选择一个正在运行的进程作为规则目标。",
      "none":  "没有找到有活动连接的进程。",
      "pick":  "使用"
    },
    "toast": {
      "saved":   "路由已保存",
      "templateApplied": "模板已应用",
      "imported": "已导入 {added}/{total} 条规则"
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

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/features/routing/index.ts gui/web/src/app/routes.ts gui/web/src/locales/
git commit -m "feat(routing): index + route swap + i18n keys"
```

---

## Section D · Cleanup + verification (Tasks 11-12)

### Task 11: Delete legacy files

**Files:**
- Delete: `gui/web/src/pages/Routing.svelte`
- Delete: `gui/web/src/lib/routing/RoutingConfirmModal.svelte`
- Delete: `gui/web/src/lib/routing/RoutingImportExport.svelte`
- Delete: `gui/web/src/lib/routing/RoutingProcessPicker.svelte`
- Delete: `gui/web/src/lib/routing/RoutingTemplateModal.svelte`
- Delete: `gui/web/src/lib/routing/RoutingTestPanel.svelte`
- Delete: `gui/web/src/lib/routing/RuleList.svelte`

(entire `lib/routing/` directory goes)

- [ ] **Step 1: Confirm no remaining imports**

```bash
cd "/Users/homebot/Library/Mobile Documents/com~apple~CloudDocs/shuttle/gui/web"
grep -rEn "from '[./]*(pages/Routing\\.svelte|lib/routing/)" src/ 2>/dev/null
```

Expected: no output.

- [ ] **Step 2: Delete**

```bash
rm -rf gui/web/src/lib/routing
rm gui/web/src/pages/Routing.svelte
```

- [ ] **Step 3: Build + svelte-check**

```bash
cd gui/web
npx svelte-check --threshold error 2>&1 | tail -1
npm run build 2>&1 | tail -3
```

Expected: svelte-check error count drops meaningfully (the legacy Routing.svelte had the largest number of `any`/implicit type issues).

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(gui): delete legacy Routing page + lib/routing (1349 lines)"
```

---

### Task 12: Playwright smoke + bundle record + push

**Files:**
- Modify: `gui/web/tests/shell.spec.ts` (add routing smoke)
- Modify: `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md` (append P6)

- [ ] **Step 1: Add routing smoke**

Append to `shell.spec.ts`:

```ts
test.describe('P6 routing', () => {
    test('routing URL renders page chrome', async ({ page }) => {
        await page.goto('/#/routing');
        await expect(page.locator('.sidebar')).toBeVisible();
        await expect(page.locator('h3:has-text("Routing")')).toBeVisible({ timeout: 5000 });
        await expect(page.locator('button:has-text("Save")')).toBeVisible();
    });
});
```

- [ ] **Step 2: Full gate run**

```bash
cd gui/web
echo "=== svelte-check ===" && npx svelte-check --threshold error 2>&1 | tail -1
echo "=== vitest ===" && npm test 2>&1 | tail -3
echo "=== i18n ===" && ./scripts/check-i18n.sh
echo "=== build ===" && npm run build 2>&1 | tail -3
echo "=== playwright ===" && npx playwright test --reporter=line 2>&1 | tail -3
```

- [ ] **Step 3: Append post-P6 bundle record** to the baseline doc.

- [ ] **Step 4: Push and update PR #8 body.**

```bash
git push origin refactor/gui-v2
gh pr view 8 --json body -q .body > /tmp/pr-body.md
# Append a P6 section similar to P5's.
gh pr edit 8 --body-file /tmp/pr-body.md
rm /tmp/pr-body.md
```

---

## Self-review notes

**Spec coverage.**
- §7.5 "保留整体结构，换组件层" → Tasks 2-8 port legacy files to `@/ui` primitives.
- §7.5 GeoSite Combobox → RuleList (Task 2) uses `@/ui/Combobox` for category selection.
- §7.5 Dialog for all modals → Tasks 3, 4, 5, 7.
- §7.5 Rule-hit visualization bar → Task 8 RuleHitBar (stub; real counts need backend work, documented).
- §7.10 AsyncBoundary / tokens / single-accent → followed throughout.

**Placeholder scan.** Every task has concrete code (or explicit "port from file X with these changes" with an inline example). Task 5's ImportExportDialog description says "~350 lines expected" — not a placeholder, it's a size hint. Actual output can be shorter.

**Type consistency.**
- `RoutingRules`, `RoutingRule`, `RoutingTemplate`, `Process`, `DryRunResult` all from `@/lib/api/types`.
- `useRules` / `saveRules` signatures consistent across tasks.
- `onRulesChange: (rules: RoutingRule[]) => void` callback shape in RuleList ↔ RoutingPage.

**Explicit out-of-scope.**
- Rule conflict viewer (`/api/routing/conflicts` backend exists, UI deferred).
- Drag-and-drop rule reorder (legacy uses ↑/↓; keep).
- GeoSite *category group* sections (legacy flattened them; keep flat).
- Real rule-hit counters (need backend endpoint).
- Per-subscription overrides.

**Known risks.**
- ImportExportDialog (Task 5) is the biggest porting task — ~350 lines of custom drop-zone + parse logic. Regressions in file parsing would break a real user flow. Manual-smoke this against a sample `.json` export after the port.
- RuleList (Task 2) is where most review comments will land — the inline editor cells are tricky to port without breaking focus flow or Enter-submit behavior. Port conservatively.
- `structuredClone(res.data)` in RoutingPage (Task 9) needs ES2022 target; Vite's default is fine. If a Safari < 15 user hits this it fails — our Wails WebView is recent enough. No concern.

---

## Plan complete.

Plan complete and saved to `docs/superpowers/plans/2026-04-20-gui-refactor-p6-routing.md`.

Execution:
- **Subagent-Driven** — fresh agent per task, review between
- **Inline** — execute here in this session with checkpoints
