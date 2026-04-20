# GUI Refactor P2 · App Shell Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the legacy monolithic `src/App.svelte` with a composable `app/` shell (Sidebar + Shell + App) that renders routes via the P1 hash router. Legacy pages continue to render via bridge routes; SimpleMode is removed.

**Architecture:** `app/App.svelte` is the new root — it checks first-run to overlay `<Onboarding>`, otherwise renders `<Shell>`. Shell composes `<Sidebar>` + the `<Router>` outlet. `Sidebar` reads `nav` metadata from each `AppRoute` in `routes.ts` and renders a three-section list (Overview / Network / System) with brand, theme toggle, and collapse. Each of the 8 legacy pages (`Dashboard`, `Servers`, `Subscriptions`, `Groups`, `Routing`, `Mesh`, `Logs`, `Settings`) gets a corresponding `AppRoute` whose `component` is a `lazy()` import of the existing `pages/*.svelte`. SimpleMode is deleted.

**Tech Stack:** Svelte 5 runes · P1 router (`@/lib/router`) · P1 theme/toaster (`@/lib/theme.svelte`, `@/lib/toaster.svelte`) · `@/ui` design system.

**Spec reference:** `docs/superpowers/specs/2026-04-19-gui-refactor-design.md` — §2 app/ directory, §6 router + URL table, §7.1 SimpleMode removal.

**P1 plan reference:** `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure.md`.

**Branch:** Continue on `refactor/gui-v2`. P2 lands as a single PR into `main` after P1 merges.

---

## Conventions

- Relative paths from repo root.
- Every task ends with a commit. One conventional-commit per task.
- **Section titles on the sidebar are i18n keys** — add to `lib/i18n/locales/en.ts` and `zh.ts` as needed (they already contain `nav.dashboard` etc., so the sidebar sections can use `nav.section.overview` / `nav.section.network` / `nav.section.system`).
- **Bridges don't rename files** — `app/routes.ts` points each URL to the *existing* legacy `pages/*.svelte`. P3+ replaces them in place.
- **No change to Playwright tests** unless they directly reference SimpleMode (only `simple-mode.spec.ts` does).

---

## Section A · App shell components (Tasks 1–4)

### Task 1: Create `app/Sidebar.svelte`

**Files:**
- Create: `gui/web/src/app/Sidebar.svelte`

**Context:** Three-section sidebar driven by route metadata. Sections chosen by order range:
- Overview: order 10-19 (Dashboard)
- Network: order 20-69 (Servers, Subscriptions, Groups, Routing, Mesh)
- System: order 70-99 (Logs, Settings)

Brand + version at top, theme toggle + collapse chevron at bottom.

- [ ] **Step 1: Create the file**

```svelte
<script lang="ts">
  import { Link, useRoute } from '@/lib/router'
  import { Icon, Button } from '@/ui'
  import { theme } from '@/lib/theme.svelte'
  import { t } from '@/lib/i18n/index'
  import type { AppRoute } from './routes'

  interface Props {
    routes: AppRoute[]
    collapsed?: boolean
    onToggleCollapsed?: () => void
  }

  let { routes, collapsed = false, onToggleCollapsed }: Props = $props()
  const route = useRoute()

  const sections = $derived.by(() => {
    const visible = routes
      .filter((r) => r.nav && !r.nav.hidden)
      .sort((a, b) => (a.nav!.order ?? 999) - (b.nav!.order ?? 999))
    return {
      overview: visible.filter((r) => r.nav!.order < 20),
      network:  visible.filter((r) => r.nav!.order >= 20 && r.nav!.order < 70),
      system:   visible.filter((r) => r.nav!.order >= 70),
    }
  })

  function isActive(path: string): boolean {
    return route.path === path || route.path.startsWith(path + '/')
  }
</script>

<aside class="sidebar" class:collapsed>
  <div class="brand">
    <div class="logo">S</div>
    {#if !collapsed}<span class="name">Shuttle</span>{/if}
  </div>

  {#snippet section(heading: string, items: AppRoute[])}
    {#if items.length > 0}
      {#if !collapsed}<div class="heading">{heading}</div>{/if}
      <nav>
        {#each items as r}
          <Link to={r.path} class={'item ' + (isActive(r.path) ? 'on' : '')}>
            <span class="ico"><Icon name={r.nav!.icon as any} size={16} /></span>
            {#if !collapsed}<span>{t(r.nav!.label)}</span>{/if}
          </Link>
        {/each}
      </nav>
    {/if}
  {/snippet}

  {@render section(t('nav.section.overview'), sections.overview)}
  {@render section(t('nav.section.network'), sections.network)}
  {@render section(t('nav.section.system'), sections.system)}

  <div class="footer">
    <Button size="sm" variant="ghost" onclick={() => theme.toggle()}>
      <Icon name={theme.current === 'dark' ? 'check' : 'check'} size={14} />
      {#if !collapsed}{theme.current}{/if}
    </Button>
    {#if onToggleCollapsed}
      <Button size="sm" variant="ghost" onclick={onToggleCollapsed}>
        <Icon name={collapsed ? 'chevronRight' : 'chevronLeft'} size={14} />
      </Button>
    {/if}
  </div>
</aside>

<style>
  .sidebar {
    width: 220px;
    min-width: 220px;
    background: var(--shuttle-bg-base);
    border-right: 1px solid var(--shuttle-border);
    display: flex;
    flex-direction: column;
    padding: var(--shuttle-space-4) var(--shuttle-space-2);
    transition: width var(--shuttle-duration) var(--shuttle-easing);
    font-family: var(--shuttle-font-sans);
  }
  .sidebar.collapsed { width: 60px; min-width: 60px; }

  .brand {
    display: flex; align-items: center; gap: var(--shuttle-space-2);
    padding: var(--shuttle-space-1) var(--shuttle-space-2) var(--shuttle-space-4);
  }
  .logo {
    width: 22px; height: 22px;
    background: var(--shuttle-accent); color: var(--shuttle-accent-fg);
    border-radius: var(--shuttle-radius-sm);
    display: flex; align-items: center; justify-content: center;
    font-weight: var(--shuttle-weight-semibold); font-size: 11px;
  }
  .name {
    font-size: var(--shuttle-text-base);
    font-weight: var(--shuttle-weight-semibold);
    letter-spacing: var(--shuttle-tracking-tight);
    color: var(--shuttle-fg-primary);
  }

  .heading {
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    padding: var(--shuttle-space-4) var(--shuttle-space-2) var(--shuttle-space-1);
  }

  nav { display: flex; flex-direction: column; gap: 1px; }

  :global(.item) {
    display: flex; align-items: center; gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-1) var(--shuttle-space-2);
    height: 30px;
    border-radius: var(--shuttle-radius-sm);
    color: var(--shuttle-fg-secondary);
    font-size: var(--shuttle-text-sm);
    text-decoration: none;
    transition: background var(--shuttle-duration), color var(--shuttle-duration);
  }
  :global(.item:hover) { background: var(--shuttle-bg-subtle); color: var(--shuttle-fg-primary); }
  :global(.item.on)    { background: var(--shuttle-bg-subtle); color: var(--shuttle-fg-primary); }

  .ico { width: 16px; height: 16px; flex-shrink: 0; display: inline-flex; }

  .footer {
    margin-top: auto;
    padding: var(--shuttle-space-2);
    display: flex; gap: var(--shuttle-space-1);
    justify-content: space-between;
    border-top: 1px solid var(--shuttle-border);
  }
</style>
```

- [ ] **Step 2: Add i18n section keys**

Open `gui/web/src/lib/i18n/index.ts` and locate the `en` / `zh` locale tables. Under the existing `nav.*` block, add:

```ts
'nav.section.overview': 'Overview',
'nav.section.network':  'Network',
'nav.section.system':   'System',
```

For `zh`:

```ts
'nav.section.overview': '总览',
'nav.section.network':  '网络',
'nav.section.system':   '系统',
```

*(If the locale file structure differs, adapt to the existing pattern — verify by grepping for `nav.dashboard`.)*

- [ ] **Step 3: Sanity check**

```bash
cd gui/web && npx svelte-check --threshold error 2>&1 | tail -1
```

Expected: error count unchanged (no new errors from Sidebar).

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/app/Sidebar.svelte gui/web/src/lib/i18n/
git commit -m "feat(app): add Sidebar with three-section route-driven nav"
```

---

### Task 2: Create `app/Toaster.svelte`

**Files:**
- Create: `gui/web/src/app/Toaster.svelte`

**Context:** Display component that reads the P1 `toaster` runes store and renders a stacked list of toasts in a fixed overlay. Replaces the role of `lib/Toast.svelte` (which is tied to the legacy `toast.ts` subscribe API).

- [ ] **Step 1: Create the file**

```svelte
<script lang="ts">
  import { toasts, dismiss } from '@/lib/toaster.svelte'
  import { Icon } from '@/ui'
</script>

<div class="stack" role="status" aria-live="polite">
  {#each toasts.items as t (t.id)}
    <div class="toast {t.type}" role="alert">
      <span class="ico">
        <Icon name={t.type === 'success' ? 'check' : t.type === 'error' ? 'x' : 'info'} size={14} />
      </span>
      <span class="msg">{t.message}</span>
      <button class="close" onclick={() => dismiss(t.id)} aria-label="Close">
        <Icon name="x" size={12} />
      </button>
    </div>
  {/each}
</div>

<style>
  .stack {
    position: fixed;
    bottom: var(--shuttle-space-5);
    right: var(--shuttle-space-5);
    display: flex; flex-direction: column; gap: var(--shuttle-space-2);
    z-index: 80;
    pointer-events: none;
  }
  .toast {
    pointer-events: auto;
    display: flex; align-items: center; gap: var(--shuttle-space-2);
    min-width: 240px; max-width: 360px;
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    box-shadow: var(--shuttle-shadow-md);
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-primary);
    font-family: var(--shuttle-font-sans);
  }
  .toast.success { border-left: 2px solid var(--shuttle-success); }
  .toast.error   { border-left: 2px solid var(--shuttle-danger); }
  .toast.warning { border-left: 2px solid var(--shuttle-warning); }
  .toast.info    { border-left: 2px solid var(--shuttle-info); }

  .ico { color: var(--shuttle-fg-secondary); display: inline-flex; }
  .msg { flex: 1; }
  .close {
    background: transparent; border: 0; padding: 2px;
    color: var(--shuttle-fg-muted); cursor: pointer;
    display: inline-flex;
  }
  .close:hover { color: var(--shuttle-fg-primary); }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/app/Toaster.svelte
git commit -m "feat(app): add Toaster overlay driven by toaster.svelte store"
```

---

### Task 3: Create `app/Shell.svelte`

**Files:**
- Create: `gui/web/src/app/Shell.svelte`

**Context:** Horizontal flex: Sidebar + main. Main renders the Router outlet. Shell manages the collapsed state (localStorage-persisted).

- [ ] **Step 1: Create the file**

```svelte
<script lang="ts">
  import { Router } from '@/lib/router'
  import Sidebar from './Sidebar.svelte'
  import { routes } from './routes'

  let collapsed = $state(
    typeof localStorage !== 'undefined' && localStorage.getItem('shuttle-sidebar-collapsed') === '1'
  )

  function toggleCollapsed() {
    collapsed = !collapsed
    try { localStorage.setItem('shuttle-sidebar-collapsed', collapsed ? '1' : '0') } catch {}
  }
</script>

<div class="shell">
  <Sidebar {routes} {collapsed} onToggleCollapsed={toggleCollapsed} />
  <main>
    <Router {routes} />
  </main>
</div>

<style>
  .shell {
    display: flex;
    min-height: 100vh;
    background: var(--shuttle-bg-base);
  }
  main {
    flex: 1;
    overflow-y: auto;
    padding: var(--shuttle-space-5) var(--shuttle-space-6);
  }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/app/Shell.svelte
git commit -m "feat(app): add Shell layout (Sidebar + Router outlet)"
```

---

### Task 4: Create `app/App.svelte`

**Files:**
- Create: `gui/web/src/app/App.svelte`

**Context:** New root. Checks first-run by calling `api.getConfig()`; shows Onboarding overlay if no servers, otherwise Shell. Always renders Toaster on top.

- [ ] **Step 1: Create the file**

```svelte
<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '@/lib/api'
  import Onboarding from '@/lib/Onboarding.svelte'
  import Shell from './Shell.svelte'
  import Toaster from './Toaster.svelte'

  let initialized = $state(false)
  let showOnboarding = $state(false)
  let apiError = $state(false)

  async function checkFirstRun() {
    try {
      const cfg = await api.getConfig()
      const hasServers = cfg.server?.addr || (cfg.servers && cfg.servers.length > 0)
      showOnboarding = !hasServers
      apiError = false
    } catch {
      showOnboarding = false
      apiError = true
    }
    initialized = true
  }

  onMount(() => { checkFirstRun() })

  function handleOnboardingComplete() {
    showOnboarding = false
  }
</script>

<Toaster />

{#if !initialized}
  <div class="center">
    <div class="spin" aria-label="Loading"></div>
  </div>
{:else if showOnboarding}
  <Onboarding onComplete={handleOnboardingComplete} />
{:else}
  {#if apiError}
    <div class="api-error">
      Backend unavailable — retrying will reload.
      <button onclick={() => location.reload()}>Retry</button>
    </div>
  {/if}
  <Shell />
{/if}

<style>
  .center {
    display: flex; align-items: center; justify-content: center;
    min-height: 100vh;
    background: var(--shuttle-bg-base);
  }
  .spin {
    width: 28px; height: 28px;
    border: 3px solid var(--shuttle-border);
    border-top-color: var(--shuttle-fg-primary);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }

  .api-error {
    background: color-mix(in oklab, var(--shuttle-danger) 10%, transparent);
    color: var(--shuttle-danger);
    border: 1px solid color-mix(in oklab, var(--shuttle-danger) 30%, transparent);
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    font-size: var(--shuttle-text-sm);
    display: flex; justify-content: space-between; align-items: center;
    font-family: var(--shuttle-font-sans);
  }
  .api-error button {
    background: transparent; border: 1px solid var(--shuttle-danger);
    color: var(--shuttle-danger); padding: 2px 8px;
    border-radius: var(--shuttle-radius-sm); cursor: pointer; font-size: var(--shuttle-text-xs);
  }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/app/App.svelte
git commit -m "feat(app): add App root (onboarding overlay + Shell + Toaster)"
```

---

## Section B · Routes populated with bridges (Task 5)

### Task 5: Populate `app/routes.ts`

**Files:**
- Modify: `gui/web/src/app/routes.ts`

**Context:** Each URL points to the existing legacy page via `lazy()`. Settings has no `onSwitchToSimple` in the new world; the prop must be removed in Task 8, but the bridge wraps the legacy component and passes no props so it Just Works.

The sidebar uses `order` to sort and section-group:
- 10 Dashboard
- 20 Servers, 30 Subscriptions, 40 Groups, 50 Routing, 60 Mesh
- 80 Logs, 90 Settings

- [ ] **Step 1: Replace file content**

```ts
import { lazy } from '@/lib/router'
import type { Component } from 'svelte'

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
  {
    path: '/',
    component: lazy(() => import('@/pages/Dashboard.svelte')),
    nav: { label: 'nav.dashboard', icon: 'dashboard', order: 10 },
  },
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

- [ ] **Step 2: Run `svelte-check`**

```bash
cd gui/web && npx svelte-check --threshold error 2>&1 | tail -1
```
Expected: unchanged baseline.

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/app/routes.ts
git commit -m "feat(app): populate routes with legacy-page bridges"
```

---

## Section C · Main swap (Task 6)

### Task 6: Update `main.ts` to mount new `app/App.svelte`

**Files:**
- Modify: `gui/web/src/main.ts`

**Context:** The dev-only UI preview gate stays. Production mount now uses the new App.

- [ ] **Step 1: Replace file content**

```ts
/// <reference types="vite/client" />
import './app.css'
import { mount } from 'svelte'
import App from './app/App.svelte'

const target = document.getElementById('app')!

const params = typeof location !== 'undefined' ? new URLSearchParams(location.search) : null
if (import.meta.env.DEV && params?.get('ui') === '1') {
  import('./__ui__/UIPreview.svelte').then((mod) => {
    mount(mod.default, { target })
  })
} else {
  mount(App, { target })
}
```

- [ ] **Step 2: Run dev server smoke**

```bash
cd gui/web && (timeout 8 npm run dev > /tmp/vite-p2.log 2>&1 || true)
grep -E "ready in|error" /tmp/vite-p2.log | head -3
```
Expected: `VITE vX.Y.Z  ready in ...` (no errors).

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/main.ts
git commit -m "feat(gui): mount new app/App.svelte; legacy src/App.svelte unused"
```

---

## Section D · SimpleMode removal (Tasks 7–9)

### Task 7: Delete `pages/SimpleMode.svelte`

**Files:**
- Delete: `gui/web/src/pages/SimpleMode.svelte`

- [ ] **Step 1: Delete**

```bash
rm gui/web/src/pages/SimpleMode.svelte
```

- [ ] **Step 2: Confirm no remaining imports**

```bash
grep -rEn "SimpleMode|simple-mode" "/Users/homebot/Library/Mobile Documents/com~apple~CloudDocs/shuttle/gui/web/src" 2>/dev/null || echo "✓ no references"
```

Expected: `✓ no references` OR only matches inside `App.svelte` (legacy, removed in Task 10).

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat(gui): remove SimpleMode — replaced by adaptive Dashboard (P3)"
```

---

### Task 8: Remove `onSwitchToSimple` prop from Settings.svelte

**Files:**
- Modify: `gui/web/src/pages/Settings.svelte`

**Context:** Settings had a "Switch to Simple Mode" button powered by a prop. SimpleMode no longer exists; remove the prop, the handler, and the button.

- [ ] **Step 1: Remove the prop declaration**

Open `gui/web/src/pages/Settings.svelte`. Find the top `<script>` block and locate:

```ts
  interface Props {
    onSwitchToSimple?: () => void
  }

  let { onSwitchToSimple }: Props = $props()
```

Replace with nothing (delete those lines). If `Props` interface has additional fields (verify by reading context), keep them; only drop the `onSwitchToSimple` field.

- [ ] **Step 2: Remove the button block**

Around line 257–260, find:

```svelte
  {#if onSwitchToSimple}
    <button class="btn-simple-mode" onclick={onSwitchToSimple}>
      ...
    </button>
  {/if}
```

Delete the entire `{#if}` block (including its CSS class `.btn-simple-mode` if it has a dedicated style rule — grep to confirm and remove matching style).

- [ ] **Step 3: Verify build**

```bash
cd gui/web && npm run build 2>&1 | tail -3
```

Expected: build succeeds.

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/pages/Settings.svelte
git commit -m "refactor(gui): drop onSwitchToSimple prop + button from Settings"
```

---

### Task 9: Delete Playwright SimpleMode spec

**Files:**
- Delete: `gui/web/tests/simple-mode.spec.ts`

- [ ] **Step 1: Delete**

```bash
rm gui/web/tests/simple-mode.spec.ts
```

- [ ] **Step 2: Commit**

```bash
git add -A
git commit -m "test(gui): remove simple-mode.spec.ts (SimpleMode deleted)"
```

---

## Section E · Legacy App removal (Task 10)

### Task 10: Delete legacy `src/App.svelte`

**Files:**
- Delete: `gui/web/src/App.svelte`

**Context:** With `main.ts` now mounting `app/App.svelte`, the legacy root is dead code. Its CSS `:global(body)` rules must be preserved — move them into `app.css` first.

- [ ] **Step 1: Move essential global styles from legacy App.svelte to app.css**

Open the legacy `gui/web/src/App.svelte` and find the `:global(body)`, `:global(*)`, and `:global(::-webkit-scrollbar*)` rules inside its `<style>` block. Append them to `gui/web/src/app.css` **without** the `:global(...)` wrapper (since app.css is already a plain global stylesheet). Adapt the rules to use `--shuttle-*` tokens where the old ones used `--bg-primary` etc.

Final `gui/web/src/app.css`:

```css
/* Global CSS: tokens + resets only. Component styles stay inside components. */
@import './ui/tokens.css';

/* Minimal reset */
*, *::before, *::after { box-sizing: border-box; }

body {
  margin: 0;
  font-family: var(--shuttle-font-sans);
  background: var(--shuttle-bg-base);
  color: var(--shuttle-fg-primary);
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

:focus-visible {
  outline: 2px solid var(--shuttle-accent);
  outline-offset: 2px;
}

::-webkit-scrollbar { width: 6px; height: 6px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb {
  background: var(--shuttle-border);
  border-radius: 3px;
}
::-webkit-scrollbar-thumb:hover { background: var(--shuttle-fg-muted); }
```

- [ ] **Step 2: Delete the legacy App.svelte**

```bash
rm gui/web/src/App.svelte
```

- [ ] **Step 3: Verify**

```bash
cd gui/web && npx svelte-check --threshold error 2>&1 | tail -1 && npm run build 2>&1 | tail -3
```

Expected: error count **decreases** (pre-existing App.svelte errors drop out); build succeeds.

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/app.css gui/web/src/App.svelte 2>/dev/null; git add -A
git commit -m "refactor(gui): delete legacy src/App.svelte; migrate body + scrollbar styles to app.css"
```

---

## Section F · Tests + verification (Tasks 11–13)

### Task 11: Sidebar unit test

**Files:**
- Create: `gui/web/src/app/Sidebar.test.ts`

- [ ] **Step 1: Create the test**

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { render } from '@testing-library/svelte'
import { __resetRoute } from '@/lib/router/router.svelte'
import Sidebar from '@/app/Sidebar.svelte'
import type { AppRoute } from '@/app/routes'

const mockRoutes: AppRoute[] = [
  { path: '/',        component: () => Promise.resolve(null as any), nav: { label: 'nav.dashboard', icon: 'dashboard', order: 10 } },
  { path: '/servers', component: () => Promise.resolve(null as any), nav: { label: 'nav.servers',   icon: 'servers',   order: 20 } },
  { path: '/logs',    component: () => Promise.resolve(null as any), nav: { label: 'nav.logs',      icon: 'logs',      order: 80 } },
]

describe('Sidebar', () => {
  beforeEach(() => {
    location.hash = ''
    __resetRoute()
  })

  it('renders a nav entry per route', () => {
    const { container } = render(Sidebar, { props: { routes: mockRoutes } })
    const links = container.querySelectorAll('a.item')
    expect(links.length).toBe(3)
  })

  it('groups entries into Overview / Network / System by order', () => {
    const { container } = render(Sidebar, { props: { routes: mockRoutes } })
    const headings = Array.from(container.querySelectorAll('.heading')).map((el) => el.textContent?.trim())
    // i18n falls back to keys in the default (en) table; at minimum all three
    // sections should render since each has at least one route.
    expect(headings.length).toBe(3)
  })
})
```

- [ ] **Step 2: Run**

```bash
cd gui/web && npm test -- Sidebar.test 2>&1 | tail -5
```
Expected: PASS (2 tests).

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/app/Sidebar.test.ts
git commit -m "test(app): add Sidebar nav-entry + section-grouping tests"
```

---

### Task 12: Playwright smoke against new URL structure

**Files:**
- Create: `gui/web/tests/shell.spec.ts`

**Context:** Verify the new shell mounts and each URL loads without runtime error. Doesn't check visual correctness (that's manual).

- [ ] **Step 1: Create the test**

```ts
import { test, expect } from '@playwright/test'

test.describe('P2 shell', () => {
  test('root URL renders Dashboard via bridge', async ({ page }) => {
    await page.goto('/')
    // Wait for the SPA to mount and either Onboarding OR Shell to render
    await page.waitForSelector('.shell, .onboarding', { timeout: 5000 })
  })

  test('hash navigation to /servers loads Servers page', async ({ page }) => {
    await page.goto('/#/servers')
    // Legacy Servers page has a recognizable piece of DOM; check for an
    // element common to the page (h1/h2 text or specific class).
    await page.waitForLoadState('networkidle')
    // A loose assertion: at least the sidebar "Servers" entry is highlighted
    const active = await page.locator('a.item.on').textContent()
    expect(active?.toLowerCase()).toContain('server')
  })
})
```

- [ ] **Step 2: Run the full Playwright suite**

```bash
cd gui/web && npx playwright test --reporter=list 2>&1 | tail -10
```

Expected: `shell.spec.ts` passes; other specs either pass or skip if they depend on a backend not running. If pre-existing tests break because they reference legacy DOM, note it and leave them — they'll be rewritten in P11. Do NOT mass-disable unrelated tests.

*(If the dev server isn't auto-started by Playwright, configure `playwright.config.ts` first by checking existing setup. The existing `mesh.spec.ts` and `subscriptions.spec.ts` must have some launcher — follow that pattern.)*

- [ ] **Step 3: Commit**

```bash
git add gui/web/tests/shell.spec.ts
git commit -m "test(gui): add P2 shell smoke — root + /servers via hash nav"
```

---

### Task 13: Final P2 gate run + PR

**Files:** none; verification only.

- [ ] **Step 1: All local gates**

```bash
cd gui/web
echo "=== svelte-check ===" && npx svelte-check --threshold error 2>&1 | tail -1
echo "=== vitest ==="        && npm test 2>&1 | tail -5
echo "=== i18n ==="           && ./scripts/check-i18n.sh
echo "=== build ==="          && npm run build 2>&1 | tail -3
```

Expected: all green. Error count should be **lower** than the P1 baseline (we deleted the 359-error-contributing legacy `App.svelte`).

- [ ] **Step 2: Bundle size check**

```bash
cd gui/web && du -b dist/assets/index*.js dist/assets/index*.css 2>/dev/null | sort -n
```

Compare to P1 post numbers recorded in `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md`. Expected delta: +5-15 KB gzip because we now import `@/ui`, `@/lib/router`, and `@/lib/toaster` from the route. Append to the baseline doc.

- [ ] **Step 3: Push and open PR**

```bash
git push origin refactor/gui-v2
gh pr create --title "refactor(gui): P2 app shell — Sidebar + Shell + App + bridged routes" --body "$(cat <<'EOF'
## Summary

P2 of the full GUI overhaul. Replaces the legacy `src/App.svelte` with a composable `app/` shell that renders routes via the P1 hash router.

- `app/Sidebar.svelte` — three-section navigation driven by route metadata (Overview / Network / System)
- `app/Shell.svelte` — flex layout: sidebar + router outlet
- `app/App.svelte` — root: Onboarding overlay on first-run, otherwise Shell; always renders Toaster
- `app/Toaster.svelte` — overlay driven by P1 `toaster.svelte` runes store
- `app/routes.ts` populated with 8 bridge routes; each URL lazy-loads the existing `pages/*.svelte` until P3+ replaces it in place
- **SimpleMode removed**: file, prop, Playwright spec
- **Legacy App.svelte removed**: its global body + scrollbar rules moved to `app.css`

After P2 the URL structure matches the spec: `#/`, `#/servers`, `#/subscriptions`, `#/groups`, `#/routing`, `#/mesh`, `#/logs`, `#/settings`. Subsequent PRs (P3+) swap each bridge for a redesigned feature.

Plan: `docs/superpowers/plans/2026-04-20-gui-refactor-p2-app-shell.md`

## Test plan
- [x] `npm run check` — new code clean; legacy errors decrease (App.svelte deleted)
- [x] `npm test` — all vitest pass including new Sidebar test
- [x] `./scripts/check-i18n.sh` — clean
- [x] `npm run build` — succeeds
- [x] Bundle gzip delta within budget
- [x] `shell.spec.ts` — Playwright smoke passes
- [ ] Manual smoke: `npm run dev` → click each sidebar entry, verify legacy page loads; verify hash URL updates

## Next

P3 — Dashboard feature redesign (§7.1): adaptive Hero + Stats grid + Throughput chart + Transport breakdown. Replaces the bridge for `#/`.
EOF
)"
```

---

## Self-review notes

**Spec coverage.**
- §2 `app/` directory → Tasks 1–4 create all 4 files (App, Shell, Sidebar, Toaster) + Task 5 populates routes.
- §6 URL table → Task 5 maps 8 URLs (SimpleMode not in the table; Onboarding is overlay-only, not a route yet — added in P10).
- §7.1 SimpleMode removal → Tasks 7–9.
- §7.10 "Toast only for async results" → Toaster component in place to receive such events.

**Placeholder scan.** No TBD/TODO/later in any step. Task 8 step 1 references "verify by reading context" for the `Props` block which is a *reading* step, not a placeholder — the engineer reads the file to locate the lines (standard practice). Acceptable.

**Type consistency.**
- `AppRoute` interface (Task 5) used in Sidebar (Task 1) and Shell (Task 3) — aligned.
- `NavMeta.order` is `number` everywhere.
- `lazy()` return type `() => Promise<Component>` matches `RouteDef.component` from `lib/router`.

**Explicit out-of-scope for P2.**
- Replacing any legacy page UX (that starts in P3).
- `#/groups/:id` child route (part of P5).
- `#/settings/:section` sub-routing (part of P9).
- a11y / axe audit (P11).
- Onboarding wizard redesign (P10).

**Known risks.**
- Pre-existing Playwright tests (`mesh.spec.ts`, `subscriptions.spec.ts`) might reference legacy DOM selectors that changed with the new Shell chrome. Since the legacy pages themselves are still rendered, content selectors should still match. If they break, fix them in Task 12 scope; don't disable.
- Legacy `lib/Toast.svelte` is now orphaned (only `toaster.svelte` is wired via Toaster). It'll be deleted in P10 cleanup, not here (kept to minimize P2 blast radius).

---

## Plan complete.

Plan complete and saved to `docs/superpowers/plans/2026-04-20-gui-refactor-p2-app-shell.md`.

Execution:
- **Subagent-Driven** — fresh agent per task, review between
- **Inline** — execute here in this session with checkpoints
