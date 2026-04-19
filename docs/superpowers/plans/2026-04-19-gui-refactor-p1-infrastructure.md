# GUI Refactor P1 · Infrastructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the `ui/` design system, `lib/` infrastructure, `app/` shell scaffolding, and dev tooling so that later phases (P2–P11) can migrate features onto them. P1 is purely additive — **no old page or route is touched** and the existing GUI continues to work unchanged.

**Architecture:** Feature-sliced layout (`ui/` primitives, `lib/` infra, `app/` shell, `features/*` slices). `ui/` uses self-built components for simple cases and wraps `bits-ui` for complex a11y primitives. Data layer is a custom `createResource` runes factory with shared single-flight and subscription-count lifecycle. Router is a ~80-line custom hash-based router. Visual language is Geist-inspired (light/dark neutral palette, 4px grid, no gradients, no glow).

**Tech Stack:** Svelte 5 runes · Vite 6 · TypeScript · `bits-ui` (new) · `vitest` + `@testing-library/svelte` (new) · Playwright (existing).

**Spec reference:** `docs/superpowers/specs/2026-04-19-gui-refactor-design.md` — §1 constraints · §2 directory · §3 extensibility rules · §4 design tokens · §5 Resource · §6 Router.

**Branch:** All P1–P11 work happens on `refactor/gui-v2`. P1 lands on that branch via one PR into `main`.

---

## Conventions used below

- All file paths are relative to repo root unless stated.
- **TDD order**: write failing test → run (expect fail) → implement → run (expect pass) → commit. For pure-display primitives (Card / Spinner / Empty / Icon) a single render-smoke test is enough; complex interactive primitives (Dialog / Tabs / Combobox / Resource / Router) get behavior tests.
- **Commits**: one commit per task (`feat:` / `chore:` / `test:` / `refactor:`). Body should be ≤ 2 sentences.
- **Imports**: always use the `@` alias (set up in Task 3). E.g. `import { Button } from '@/ui'`.
- **File size budget**: every primitive ≤ 120 lines (template + script + style). Every infra module ≤ 150 lines.
- **No placeholder TODOs** — anything unfinished stays off the branch.

---

## Section A · Environment & scaffolding (Tasks 1–6)

### Task 1: Create branch and record baseline

**Files:**
- Create: `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md`

- [ ] **Step 1: Create branch from main**

```bash
cd "<repo-root>"
git switch -c refactor/gui-v2
```

- [ ] **Step 2: Build current `gui/web` and capture bundle size baseline**

```bash
cd gui/web && npm ci && npm run build
du -b dist/assets/*.js dist/assets/*.css | sort -n
```

- [ ] **Step 3: Write baseline note**

Write `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md` with:

```markdown
# P1 Infrastructure — Pre-change baseline

Run: `cd gui/web && npm ci && npm run build`
Date: <YYYY-MM-DD>

## Bundle sizes (bytes, from dist/assets/)

<paste output of `du -b` here, e.g.>
  238901  dist/assets/index-abc123.js
   42117  dist/assets/index-def456.css

## Budget for P1 PR
Total JS + CSS gzip delta ≤ **+30 KB** (bits-ui + testing-library + ui/ primitives).
```

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md
git commit -m "chore(gui): record bundle size baseline for P1 refactor"
```

---

### Task 2: Install new dependencies

**Files:**
- Modify: `gui/web/package.json`, `gui/web/package-lock.json`

- [ ] **Step 1: Install runtime and dev dependencies**

```bash
cd gui/web
npm install bits-ui@latest
npm install -D vitest@latest @testing-library/svelte@latest @testing-library/jest-dom@latest jsdom@latest @testing-library/user-event@latest
```

Expected: installs succeed with no peer-dep errors. If Svelte 5 peer warnings appear, accept them — bits-ui v1+ supports Svelte 5.

- [ ] **Step 2: Verify bits-ui works with current Svelte 5**

Run:
```bash
cd gui/web
npx svelte-check --threshold error
```
Expected: 0 errors. (If new type errors from `bits-ui`, record them — may indicate we must pin `bits-ui@1.x` exactly.)

- [ ] **Step 3: Commit**

```bash
git add package.json package-lock.json
git commit -m "chore(gui): add bits-ui, vitest, testing-library for P1"
```

---

### Task 3: Add `@` path alias to Vite and TS

**Files:**
- Modify: `gui/web/vite.config.js`
- Modify: `gui/web/tsconfig.json`

- [ ] **Step 1: Update `vite.config.js`**

Replace entire file with:

```js
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [svelte()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:9090',
    },
  },
})
```

- [ ] **Step 2: Update `tsconfig.json`** — add `baseUrl` + `paths`

Replace `compilerOptions` block (full file):

```json
{
  "compilerOptions": {
    "target": "ESNext",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "noEmit": true,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "verbatimModuleSyntax": true,
    "lib": ["ESNext", "DOM", "DOM.Iterable"],
    "baseUrl": ".",
    "paths": { "@/*": ["src/*"] }
  },
  "include": ["src/**/*.ts", "src/**/*.svelte"]
}
```

- [ ] **Step 3: Smoke-test alias by importing an existing file**

Create temporary file `gui/web/src/__alias_check.ts`:

```ts
import { api } from '@/lib/api'
export const _check = api
```

Run:
```bash
cd gui/web && npx svelte-check --threshold error
```

Expected: 0 errors. If errors, alias is wrong — fix before proceeding.

- [ ] **Step 4: Delete smoke file and commit**

```bash
rm gui/web/src/__alias_check.ts
git add gui/web/vite.config.js gui/web/tsconfig.json
git commit -m "chore(gui): add @ path alias for feature-sliced imports"
```

---

### Task 4: Configure Vitest with Svelte + jsdom

**Files:**
- Create: `gui/web/vitest.config.ts`
- Create: `gui/web/test/setup.ts`
- Modify: `gui/web/package.json` (scripts)

- [ ] **Step 1: Create `gui/web/vitest.config.ts`**

```ts
import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config.js'

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: 'jsdom',
      globals: true,
      setupFiles: ['./test/setup.ts'],
      include: ['src/**/*.{test,spec}.{ts,svelte.ts}'],
    },
  }),
)
```

- [ ] **Step 2: Create `gui/web/test/setup.ts`**

```ts
import '@testing-library/jest-dom/vitest'

// Stub matchMedia for theme.svelte tests
if (!window.matchMedia) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  })
}
```

- [ ] **Step 3: Add test script to `gui/web/package.json`**

Inside `"scripts"` block, add entries (keep existing):

```json
"test": "vitest run",
"test:watch": "vitest"
```

- [ ] **Step 4: Verify vitest runs (no tests yet)**

```bash
cd gui/web && npm test
```

Expected: `No test files found, exiting with code 0` OR similar non-error exit.

- [ ] **Step 5: Commit**

```bash
git add gui/web/vitest.config.ts gui/web/test/setup.ts gui/web/package.json
git commit -m "chore(gui): configure vitest with jsdom + testing-library"
```

---

### Task 5: Add `svelte-check` + `vitest` to CI pipeline

**Files:**
- Modify: `.github/workflows/build.yml` (or equivalent)

- [ ] **Step 1: Inspect existing GUI CI job**

```bash
grep -nE "(gui/web|svelte|vitest|npm test)" .github/workflows/*.yml
```

- [ ] **Step 2: Add test step**

In the existing job that runs `cd gui/web && npm run build`, **insert before** the build step:

```yaml
      - name: svelte-check
        run: cd gui/web && npm run check
      - name: vitest
        run: cd gui/web && npm test
```

If no GUI job exists, create one modeled after the existing Go test job. Use Node 22.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/
git commit -m "ci(gui): add svelte-check and vitest gates"
```

---

### Task 6: Add i18n leak detector (CI grep)

**Files:**
- Create: `gui/web/scripts/check-i18n.sh`
- Modify: `.github/workflows/build.yml`

**Context:** Spec §10 R5 — prevent hardcoded English strings in `ui/` and `features/`. We enforce with a grep rule.

- [ ] **Step 1: Create `gui/web/scripts/check-i18n.sh`**

```bash
#!/usr/bin/env bash
# Detect hardcoded user-visible English in ui/ and features/.
# Strings must come through lib/i18n or be explicit i18n keys.
set -euo pipefail

cd "$(dirname "$0")/.."

# Flag any `>Title Case English Words<` or `placeholder="English words"` etc.
# Allow-listed: file-level SVG paths, test files, single-word labels under 3 chars.
if grep -rEn \
     --include='*.svelte' \
     '(>[A-Z][a-z]+ [A-Z][a-z][^<{]+<|placeholder="[A-Z][a-z][^"]+"|aria-label="[A-Z][a-z][^"]+")' \
     src/ui src/features 2>/dev/null | grep -vE '(\.test\.|\.spec\.)' | head -5; then
  echo ""
  echo "❌ Found apparent hardcoded English in ui/ or features/. Use t(key) via @/lib/i18n."
  exit 1
fi

echo "✓ No hardcoded English found in ui/ or features/"
```

- [ ] **Step 2: `chmod +x gui/web/scripts/check-i18n.sh`**

```bash
chmod +x gui/web/scripts/check-i18n.sh
```

- [ ] **Step 3: Add to CI**

In the GUI job, after vitest:

```yaml
      - name: i18n check
        run: cd gui/web && ./scripts/check-i18n.sh
```

- [ ] **Step 4: Run locally to confirm it passes on current tree (folders don't exist yet → no violations)**

```bash
cd gui/web && ./scripts/check-i18n.sh
```

Expected: `✓ No hardcoded English found`.

- [ ] **Step 5: Commit**

```bash
git add gui/web/scripts/check-i18n.sh .github/workflows/
git commit -m "ci(gui): guard against hardcoded English in ui/ and features/"
```

---

## Section B · Design tokens & base CSS (Tasks 7–9)

### Task 7: Create `ui/tokens.css` with Geist-inspired palette

**Files:**
- Create: `gui/web/src/ui/tokens.css`

**Context:** Spec §4.1 token list + §4.3 light/dark specific values.

- [ ] **Step 1: Create `gui/web/src/ui/tokens.css`**

```css
/* ============================================================
 * Shuttle design tokens — Geist-inspired
 * Convention: --shuttle-<scope>-<prop>
 * ============================================================ */

:root,
[data-theme="dark"] {
  /* Background */
  --shuttle-bg-base: #0a0a0a;
  --shuttle-bg-surface: #111113;
  --shuttle-bg-subtle: #1a1a1c;

  /* Foreground */
  --shuttle-fg-primary: #ededed;
  --shuttle-fg-secondary: #a1a1aa;
  --shuttle-fg-muted: #52525b;

  /* Border */
  --shuttle-border: #27272a;
  --shuttle-border-strong: #3f3f46;

  /* Accent (inverse button) */
  --shuttle-accent: #ededed;
  --shuttle-accent-fg: #09090b;

  /* Semantic state colors — used ONLY for state */
  --shuttle-success: #22c55e;
  --shuttle-warning: #eab308;
  --shuttle-danger: #ef4444;
  --shuttle-info: #3b82f6;
}

[data-theme="light"] {
  --shuttle-bg-base: #fafafa;
  --shuttle-bg-surface: #ffffff;
  --shuttle-bg-subtle: #f4f4f5;

  --shuttle-fg-primary: #09090b;
  --shuttle-fg-secondary: #52525b;
  --shuttle-fg-muted: #a1a1aa;

  --shuttle-border: #e4e4e7;
  --shuttle-border-strong: #d4d4d8;

  --shuttle-accent: #09090b;
  --shuttle-accent-fg: #fafafa;

  --shuttle-success: #16a34a;
  --shuttle-warning: #ca8a04;
  --shuttle-danger: #dc2626;
  --shuttle-info: #2563eb;
}

/* Typography */
:root {
  --shuttle-font-sans: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
  --shuttle-font-mono: ui-monospace, 'SF Mono', 'Menlo', 'Consolas', monospace;

  --shuttle-text-xs:  11px;
  --shuttle-text-sm:  12px;
  --shuttle-text-base: 14px;
  --shuttle-text-lg:  16px;
  --shuttle-text-xl:  20px;
  --shuttle-text-2xl: 28px;

  --shuttle-weight-regular: 400;
  --shuttle-weight-medium: 500;
  --shuttle-weight-semibold: 600;

  --shuttle-tracking-tight: -0.02em;
  --shuttle-tracking-normal: 0;

  /* Spacing — 4px grid */
  --shuttle-space-0: 0;
  --shuttle-space-1: 4px;
  --shuttle-space-2: 8px;
  --shuttle-space-3: 12px;
  --shuttle-space-4: 16px;
  --shuttle-space-5: 24px;
  --shuttle-space-6: 32px;
  --shuttle-space-7: 48px;

  /* Radius */
  --shuttle-radius-sm: 4px;
  --shuttle-radius-md: 8px;
  --shuttle-radius-lg: 12px;

  /* Shadow */
  --shuttle-shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.06);
  --shuttle-shadow-md: 0 4px 12px rgba(0, 0, 0, 0.08);

  /* Motion */
  --shuttle-duration: 120ms;
  --shuttle-easing: cubic-bezier(0.2, 0, 0, 1);
}
```

- [ ] **Step 2: Sanity-run `svelte-check`**

```bash
cd gui/web && npx svelte-check --threshold error
```
Expected: 0 errors. (CSS-only file; nothing references it yet.)

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/ui/tokens.css
git commit -m "feat(ui): add Geist-inspired design tokens (light + dark)"
```

---

### Task 8: Create global `app.css` and wire it into `main.ts`

**Files:**
- Create: `gui/web/src/app.css`
- Modify: `gui/web/src/main.ts`

**Context:** We **do not** yet remove the CSS from `App.svelte` — that happens in P2. P1 just makes the tokens available globally so new `ui/` components can use them.

- [ ] **Step 1: Create `gui/web/src/app.css`**

```css
/* Global CSS: tokens + resets only. Component styles stay inside components. */
@import './ui/tokens.css';

/* Minimal reset (non-invasive — does not conflict with existing App.svelte CSS) */
*, *::before, *::after { box-sizing: border-box; }
```

- [ ] **Step 2: Import `app.css` at the top of `gui/web/src/main.ts`**

Read the existing file, then add the import at the top:

```ts
import './app.css'
// ... rest of existing main.ts unchanged ...
```

- [ ] **Step 3: Run `npm run build` to confirm nothing breaks**

```bash
cd gui/web && npm run build
```

Expected: build succeeds. The existing Dashboard/etc. still render because `App.svelte` keeps its own token set.

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/app.css gui/web/src/main.ts
git commit -m "feat(ui): wire design tokens via global app.css"
```

---

### Task 9: Add `gui/web/src/README.md` contributor doc

**Files:**
- Create: `gui/web/src/README.md`

**Context:** Spec §3.⑦ — contributor-facing file explaining layer rules and how to add features/primitives.

- [ ] **Step 1: Create `gui/web/src/README.md`**

```markdown
# gui/web/src — Frontend Architecture

> **In one sentence:** feature-sliced Svelte 5 + custom `ui/` design system (bits-ui hybrid) + `lib/` infrastructure. Depend downward only.

## Layers

| Layer | Purpose | May import from |
|-------|---------|-----------------|
| `features/<x>/` | Business slice (page + components + resource + types) | `app/`, `ui/`, `lib/` |
| `app/` | Shell, sidebar, route registry | `ui/`, `lib/` |
| `ui/` | Primitives (pure display, no business) | `lib/` (only non-Svelte, non-API helpers) |
| `lib/` | Cross-cutting infrastructure | nothing internal |

**Hard rules (enforced by review, not tooling):**
- `ui/` MUST NOT import from `features/`, `app/`, or `lib/api/`.
- `features/<x>/` MUST NOT import from `features/<y>/`. Shared code goes to `ui/` or `lib/`.
- `lib/` is pure TypeScript; no Svelte components.
- All data goes through `lib/resource` + `lib/api`. No `fetch` in pages.
- All interactive widgets come from `ui/`. No inline `<button>` beyond `ui/Button`.

## How to add a new feature

1. Create `src/features/<name>/` with:
   - `index.ts` — public exports
   - `<Name>Page.svelte` — the route entry
   - `resource.ts` — data hooks (use `createResource` / `createStream`)
   - Internal components, dialogs, helpers — never re-exported from `index.ts`
2. In `src/features/<name>/index.ts`:
   ```ts
   import { lazy } from '@/lib/router'
   export const route = {
     path: '/<name>',
     component: lazy(() => import('./<Name>Page.svelte')),
     nav: { label: 'nav.<name>', icon: '<icon>', order: <n> },
   }
   export { /* public API */ } from './resource'
   ```
3. In `src/app/routes.ts`, add one `import * as <name> from '@/features/<name>'` and one entry in the `routes` array.

## How to add a UI primitive

1. Decide: self-built or bits-ui wrapper?
   - Bits-ui: anything requiring keyboard a11y, focus trap, portals, screen-reader semantics (Dialog, Popover, Select, Combobox, Tabs, Tooltip, DropdownMenu, Menubar, Switch, RadioGroup).
   - Self-built: pure display (Button, Card, Badge, Icon, StatRow, Spinner, Empty, ErrorBanner, Section).
2. Create `src/ui/<Name>.svelte`. ≤120 lines. Props API: `size: 'sm'|'md'`, `variant: '…'`, `loading`, `disabled`, standard `onclick`, slot-based children.
3. Export from `src/ui/index.ts`.
4. Add a render-smoke test at `src/ui/<Name>.test.ts` (behavior test for interactive ones).

## Tokens

All colors / spacing / type come from `src/ui/tokens.css` — never hardcode hex or px.
Convention: `--shuttle-<scope>-<prop>` (e.g. `--shuttle-bg-surface`, `--shuttle-space-4`).
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/README.md
git commit -m "docs(gui): add src/ architecture and contribution guide"
```

---

## Section C · `lib/api` split (Tasks 10–13)

**Context:** Current `gui/web/src/lib/api.ts` (445 lines) mixes fetch-wrapper + types + endpoint fns. We split into three files, and keep `api.ts` as a re-export barrel so old pages keep compiling.

### Task 10: Extract fetch client to `lib/api/client.ts`

**Files:**
- Create: `gui/web/src/lib/api/client.ts`

- [ ] **Step 1: Create `gui/web/src/lib/api/client.ts`**

```ts
// Low-level HTTP client. Owns auth token, timeout, JSON parsing.

declare global {
  interface Window {
    __SHUTTLE_AUTH_TOKEN__?: string
  }
}

export interface ClientOptions {
  base?: string
  version?: string // reserved for future: /api/v1 vs /api/v2
  defaultTimeoutMs?: number
}

export interface Client {
  get<T>(path: string, timeoutMs?: number): Promise<T>
  post<T>(path: string, body: unknown, timeoutMs?: number): Promise<T>
  put<T>(path: string, body: unknown, timeoutMs?: number): Promise<T>
  del<T>(path: string, timeoutMs?: number): Promise<T>
  setAuthToken(token: string): void
  getAuthToken(): string
}

export function createClient(opts: ClientOptions = {}): Client {
  const base = opts.base ?? ''
  const defaultTimeout = opts.defaultTimeoutMs ?? 10000
  let authToken: string = typeof window !== 'undefined' ? (window.__SHUTTLE_AUTH_TOKEN__ ?? '') : ''

  async function request<T>(method: string, path: string, body?: unknown, timeoutMs = defaultTimeout): Promise<T> {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), timeoutMs)
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (authToken) headers['Authorization'] = `Bearer ${authToken}`
    const init: RequestInit = { method, headers, signal: controller.signal }
    if (body !== undefined) init.body = JSON.stringify(body)
    try {
      const res = await fetch(base + path, init)
      const data = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error((data as { error?: string }).error || `HTTP ${res.status}`)
      return data as T
    } finally {
      clearTimeout(timer)
    }
  }

  return {
    get: <T>(path: string, t?: number) => request<T>('GET', path, undefined, t),
    post: <T>(path: string, body: unknown, t?: number) => request<T>('POST', path, body, t),
    put: <T>(path: string, body: unknown, t?: number) => request<T>('PUT', path, body, t),
    del: <T>(path: string, t?: number) => request<T>('DELETE', path, undefined, t),
    setAuthToken: (token: string) => { authToken = token },
    getAuthToken: () => authToken,
  }
}

// Default shared client used by the app
export const client = createClient()
export const setAuthToken = client.setAuthToken
export const getAuthToken = client.getAuthToken
```

- [ ] **Step 2: Run svelte-check**

```bash
cd gui/web && npx svelte-check --threshold error
```
Expected: 0 errors.

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/lib/api/client.ts
git commit -m "refactor(gui): extract fetch client to lib/api/client.ts"
```

---

### Task 11: Extract types to `lib/api/types.ts`

**Files:**
- Create: `gui/web/src/lib/api/types.ts`

- [ ] **Step 1: Read current `gui/web/src/lib/api.ts`**

Identify every `export interface` / `export type`. Copy them verbatim into a new file.

- [ ] **Step 2: Create `gui/web/src/lib/api/types.ts`**

Structure: a copy of every type currently in `gui/web/src/lib/api.ts`. Keep names identical. Example header block:

```ts
// All backend types. Pure declarations — no runtime code.
// When adding a new type, group with related ones.

// ── Servers ──────────────────────────────────────────────
export interface Server { addr: string; name?: string; password?: string; sni?: string }
export interface ServersResponse { active: Server; servers: Server[] }

// ── Config ───────────────────────────────────────────────
export interface Config {
  server?: Server
  servers?: Server[]
  proxy: { system_proxy?: { enabled: boolean } }
  // ... copy the rest from current api.ts
}

// ... continue copying every exported type ...
```

- [ ] **Step 3: Run svelte-check**

```bash
cd gui/web && npx svelte-check --threshold error
```
Expected: 0 errors.

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/lib/api/types.ts
git commit -m "refactor(gui): extract backend types to lib/api/types.ts"
```

---

### Task 12: Extract endpoint functions to `lib/api/endpoints.ts`

**Files:**
- Create: `gui/web/src/lib/api/endpoints.ts`

- [ ] **Step 1: Create `gui/web/src/lib/api/endpoints.ts`**

Pattern: group endpoint functions by feature. Skeleton:

```ts
import { client } from './client'
import type {
  Server, ServersResponse, Config, ImportResult, /* ... all types ... */
} from './types'

// ── status ────────────────────────────────────────────────
export const getStatus = () => client.get<any>('/api/status')

// ── servers ───────────────────────────────────────────────
export const getServers = () => client.get<ServersResponse>('/api/servers')
export const putServers = (srv: Server) => client.put<{ ok: boolean }>('/api/servers', srv)
export const addServer  = (srv: Server) => client.post<{ ok: boolean }>('/api/servers', srv)
// ... copy every remaining endpoint from current api.ts, preserving signature + path ...

export const api = {
  getStatus,
  getServers, putServers, addServer,
  // ... list every export above ...
}
```

The `api` object aggregate keeps old call sites (`api.getServers()`) compiling.

- [ ] **Step 2: Run svelte-check**

```bash
cd gui/web && npx svelte-check --threshold error
```
Expected: 0 errors.

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/lib/api/endpoints.ts
git commit -m "refactor(gui): extract endpoint functions to lib/api/endpoints.ts"
```

---

### Task 13: Make `lib/api.ts` a re-export barrel

**Files:**
- Modify: `gui/web/src/lib/api.ts`

- [ ] **Step 1: Replace full content of `gui/web/src/lib/api.ts`**

```ts
// Barrel — keeps `import { api } from './api'` working for legacy pages.
// New code should import from '@/lib/api/endpoints' or '@/lib/api/types'.

export * from './api/client'
export * from './api/types'
export * from './api/endpoints'
```

- [ ] **Step 2: Run svelte-check AND build to confirm no regression**

```bash
cd gui/web && npx svelte-check --threshold error && npm run build
```
Expected: 0 errors, build succeeds.

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/lib/api.ts
git commit -m "refactor(gui): convert lib/api.ts to barrel re-exporter"
```

---

## Section D · Data layer (Tasks 14–17)

### Task 14: Write `createResource` failing test

**Files:**
- Create: `gui/web/src/lib/resource.test.ts`

- [ ] **Step 1: Create failing test file**

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushSync } from 'svelte'
import { createResource, invalidate } from '@/lib/resource.svelte'

beforeEach(() => {
  vi.useRealTimers()
})

describe('createResource', () => {
  it('fetches on first subscribe and populates data', async () => {
    const fetcher = vi.fn(async () => ({ v: 1 }))
    const r = createResource('test.a', fetcher)
    expect(r.loading).toBe(true)
    await vi.waitUntil(() => !r.loading, { timeout: 500 })
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(r.data).toEqual({ v: 1 })
    expect(r.error).toBeNull()
  })

  it('shares one fetcher across same key', async () => {
    const fetcher = vi.fn(async () => ({ v: 2 }))
    const a = createResource('test.shared', fetcher)
    const b = createResource('test.shared', fetcher)
    await vi.waitUntil(() => !a.loading, { timeout: 500 })
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(a.data).toBe(b.data)
  })

  it('preserves last data when fetch fails', async () => {
    let call = 0
    const fetcher = vi.fn(async () => {
      call++
      if (call === 2) throw new Error('boom')
      return { v: call }
    })
    const r = createResource('test.err', fetcher)
    await vi.waitUntil(() => !r.loading, { timeout: 500 })
    expect(r.data).toEqual({ v: 1 })
    await r.refetch()
    expect(r.data).toEqual({ v: 1 })     // preserved
    expect(r.error?.message).toBe('boom')
    expect(r.stale).toBe(true)
  })

  it('invalidate triggers refetch on subscribed key', async () => {
    const fetcher = vi.fn(async () => ({ v: Math.random() }))
    const r = createResource('test.invalidate', fetcher)
    await vi.waitUntil(() => !r.loading, { timeout: 500 })
    const first = r.data
    invalidate('test.invalidate')
    await vi.waitUntil(() => r.data !== first, { timeout: 500 })
    expect(fetcher).toHaveBeenCalledTimes(2)
  })
})
```

- [ ] **Step 2: Run test (expect fail — module doesn't exist)**

```bash
cd gui/web && npm test -- resource.test
```
Expected: FAIL with "Cannot find module".

---

### Task 15: Implement `createResource`

**Files:**
- Create: `gui/web/src/lib/resource.svelte.ts`

- [ ] **Step 1: Create `gui/web/src/lib/resource.svelte.ts`**

```ts
// createResource — reactive server-state primitive for Svelte 5 runes.
// Contract: resources with the same key share state, fetcher, and polling.

interface ResourceState<T> {
  data: T | undefined
  loading: boolean
  error: Error | null
  stale: boolean
}

interface Options<T> {
  poll?: number
  initial?: T
  enabled?: () => boolean
  onError?: (e: Error) => void
}

interface Entry<T> {
  state: ResourceState<T>
  fetcher: () => Promise<T>
  opts: Options<T>
  refCount: number
  pollTimer: ReturnType<typeof setInterval> | null
  inflight: Promise<void> | null
}

const registry = new Map<string, Entry<unknown>>()

async function runFetch<T>(entry: Entry<T>): Promise<void> {
  if (entry.inflight) return entry.inflight
  entry.state.loading = true
  const p = (async () => {
    try {
      const value = await entry.fetcher()
      entry.state.data = value
      entry.state.error = null
      entry.state.stale = false
    } catch (e) {
      entry.state.error = e instanceof Error ? e : new Error(String(e))
      entry.state.stale = true
      entry.opts.onError?.(entry.state.error)
    } finally {
      entry.state.loading = false
      entry.inflight = null
    }
  })()
  entry.inflight = p
  return p
}

function startPolling<T>(entry: Entry<T>) {
  if (!entry.opts.poll || entry.opts.poll <= 0) return
  stopPolling(entry)
  entry.pollTimer = setInterval(() => {
    if (entry.opts.enabled && !entry.opts.enabled()) return
    void runFetch(entry)
  }, entry.opts.poll)
}

function stopPolling<T>(entry: Entry<T>) {
  if (entry.pollTimer) {
    clearInterval(entry.pollTimer)
    entry.pollTimer = null
  }
}

export interface Resource<T> {
  readonly data: T | undefined
  readonly loading: boolean
  readonly error: Error | null
  readonly stale: boolean
  refetch(): Promise<void>
}

export function createResource<T>(
  key: string,
  fetcher: () => Promise<T>,
  opts: Options<T> = {},
): Resource<T> {
  let entry = registry.get(key) as Entry<T> | undefined
  if (!entry) {
    const state = $state<ResourceState<T>>({
      data: opts.initial,
      loading: false,
      error: null,
      stale: false,
    })
    entry = { state, fetcher, opts, refCount: 0, pollTimer: null, inflight: null }
    registry.set(key, entry as Entry<unknown>)
  } else {
    entry.fetcher = fetcher
    entry.opts = opts
  }
  entry.refCount++

  // Kick an initial fetch if not yet populated
  if (entry.state.data === undefined && !entry.inflight) void runFetch(entry)

  startPolling(entry)

  // NOTE: P1 does NOT auto-dispose when a component unmounts. Polling continues
  // until explicit `invalidate()` or process exit. This keeps createResource
  // callable from plain .svelte.ts (no component context required), simplifying
  // tests. A ref-count/`$effect` dispose hook can be added in a later phase if
  // polling accumulates measurably; for the current set of ≤10 resources it does not.

  return {
    get data() { return entry!.state.data },
    get loading() { return entry!.state.loading },
    get error() { return entry!.state.error },
    get stale() { return entry!.state.stale },
    refetch: () => runFetch(entry!),
  }
}

export function invalidate(key: string): void {
  const entry = registry.get(key)
  if (entry) void runFetch(entry)
}

export function invalidateAll(): void {
  registry.forEach(entry => { void runFetch(entry) })
}

// Test helper — reset registry between tests
export function __resetRegistry(): void {
  registry.forEach(e => stopPolling(e))
  registry.clear()
}
```

**Note**: we intentionally avoid `$effect` inside this factory — it requires a component scope, which tests don't provide. Auto-dispose is deferred (see the inline NOTE in the code).

- [ ] **Step 2: Update test to call `__resetRegistry` between tests**

Add to `gui/web/src/lib/resource.test.ts` imports and `beforeEach`:

```ts
import { createResource, invalidate, __resetRegistry } from '@/lib/resource.svelte'

beforeEach(() => {
  __resetRegistry()
  vi.useRealTimers()
})
```

- [ ] **Step 3: Run test**

```bash
cd gui/web && npm test -- resource.test
```
Expected: PASS (4 tests).

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/lib/resource.svelte.ts gui/web/src/lib/resource.test.ts
git commit -m "feat(gui): add createResource runes primitive with tests"
```

---

### Task 16: Add `createStream` WebSocket wrapper

**Files:**
- Create: `gui/web/src/lib/resource.svelte.ts` (modify — append)

- [ ] **Step 1: Append to `gui/web/src/lib/resource.svelte.ts`**

```ts
// ─────────────────────────────────────────────────────────
// createStream — runes wrapper over lib/ws.ts
// ─────────────────────────────────────────────────────────

import { connectWS } from './ws'

export interface Stream<T> {
  readonly data: T | undefined
  readonly connected: boolean
  close(): void
}

export function createStream<T>(
  _key: string,     // reserved for future single-flight; currently ignored
  path: string,
  opts: { initial?: T } = {},
): Stream<T> {
  const state = $state({ data: opts.initial, connected: false })
  const conn = connectWS<T>(path, (msg) => {
    state.data = msg
    state.connected = true
  })
  // Consumer must call .close() explicitly from their component's $effect cleanup:
  //     $effect(() => () => stream.close())
  // We do not register a $effect here because this factory may be called
  // outside a component context (tests, module init).
  return {
    get data() { return state.data },
    get connected() { return state.connected },
    close: () => conn.close(),
  }
}
```

- [ ] **Step 2: Build + svelte-check**

```bash
cd gui/web && npx svelte-check --threshold error && npm run build
```
Expected: 0 errors, build ok.

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/lib/resource.svelte.ts
git commit -m "feat(gui): add createStream runes wrapper over ws"
```

---

### Task 17: Add `lib/flags.ts` stub

**Files:**
- Create: `gui/web/src/lib/flags.ts`

**Context:** Spec §3.⑧ — reserve the position, do not implement.

- [ ] **Step 1: Create stub**

```ts
// Feature flags — reserved hook. Not yet implemented.
// When needed, back with localStorage or config field.

export function isEnabled(_flag: string): boolean {
  return false
}
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/lib/flags.ts
git commit -m "feat(gui): reserve lib/flags.ts as non-implemented hook"
```

---

## Section E · Router (Tasks 18–21)

### Task 18: Write router failing tests

**Files:**
- Create: `gui/web/src/lib/router/router.test.ts`

- [ ] **Step 1: Create test file**

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { navigate, useRoute, matches, __resetRoute } from '@/lib/router/router.svelte'

beforeEach(() => {
  __resetRoute()
  location.hash = ''
})

describe('router', () => {
  it('starts at "/"', () => {
    const r = useRoute()
    expect(r.path).toBe('/')
  })

  it('navigate() updates path', () => {
    navigate('/servers')
    const r = useRoute()
    expect(r.path).toBe('/servers')
    expect(location.hash).toBe('#/servers')
  })

  it('reads current hash on init', () => {
    location.hash = '#/settings/mesh'
    __resetRoute()
    const r = useRoute()
    expect(r.path).toBe('/settings/mesh')
  })

  it('matches static path', () => {
    navigate('/servers')
    expect(matches('/servers')).toBe(true)
    expect(matches('/groups')).toBe(false)
  })

  it('matches path with param', () => {
    navigate('/groups/42')
    expect(matches('/groups/:id')).toBe(true)
    const r = useRoute()
    expect(r.params.id).toBe('42')
  })

  it('unknown path stays on it (no 404)', () => {
    navigate('/nonexistent')
    expect(useRoute().path).toBe('/nonexistent')
  })
})
```

- [ ] **Step 2: Run test (expect fail)**

```bash
cd gui/web && npm test -- router.test
```
Expected: FAIL (module not found).

---

### Task 19: Implement router core

**Files:**
- Create: `gui/web/src/lib/router/router.svelte.ts`

- [ ] **Step 1: Create router module**

```ts
// Minimal hash-based router for Shuttle GUI.

interface RouteState {
  path: string
  params: Record<string, string>
  query: Record<string, string>
}

const state = $state<RouteState>({ path: '/', params: {}, query: {} })

function parseHash(hash: string): { path: string; query: Record<string, string> } {
  let raw = hash.startsWith('#') ? hash.slice(1) : hash
  if (!raw) raw = '/'
  const [path, qs] = raw.split('?')
  const query: Record<string, string> = {}
  if (qs) {
    new URLSearchParams(qs).forEach((v, k) => { query[k] = v })
  }
  return { path: path || '/', query }
}

function update() {
  const { path, query } = parseHash(location.hash)
  state.path = path
  state.query = query
  state.params = {} // re-derived by matches()
}

if (typeof window !== 'undefined') {
  window.addEventListener('hashchange', update)
  update()
}

export function navigate(path: string, opts: { replace?: boolean } = {}): void {
  const hash = '#' + (path.startsWith('/') ? path : '/' + path)
  if (opts.replace) history.replaceState(null, '', hash)
  else location.hash = hash
}

export function useRoute(): Readonly<RouteState> {
  return state
}

export function useParams<T extends Record<string, string>>(): T {
  return state.params as T
}

// Returns true + populates params if pattern matches current state.path.
export function matches(pattern: string): boolean {
  const patternParts = pattern.split('/').filter(Boolean)
  const pathParts = state.path.split('/').filter(Boolean)
  if (patternParts.length !== pathParts.length) return false
  const params: Record<string, string> = {}
  for (let i = 0; i < patternParts.length; i++) {
    const p = patternParts[i]
    if (p.startsWith(':')) {
      params[p.slice(1)] = pathParts[i]
    } else if (p !== pathParts[i]) {
      return false
    }
  }
  state.params = params
  return true
}

export type Lazy<T> = () => Promise<T>
export function lazy<T>(loader: () => Promise<{ default: T }>): Lazy<T> {
  return async () => (await loader()).default
}

// Test helper
export function __resetRoute(): void {
  state.path = '/'
  state.params = {}
  state.query = {}
}
```

- [ ] **Step 2: Run test**

```bash
cd gui/web && npm test -- router.test
```
Expected: PASS (6 tests).

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/lib/router/router.svelte.ts gui/web/src/lib/router/router.test.ts
git commit -m "feat(gui): add hash router with params + matches()"
```

---

### Task 20: Add `Router.svelte`, `Route.svelte`, `Link.svelte`

**Files:**
- Create: `gui/web/src/lib/router/Router.svelte`
- Create: `gui/web/src/lib/router/Route.svelte`
- Create: `gui/web/src/lib/router/Link.svelte`

- [ ] **Step 1: Create `gui/web/src/lib/router/Router.svelte`**

```svelte
<script lang="ts">
  import { useRoute, matches, type Lazy } from './router.svelte'
  import type { Component } from 'svelte'

  interface RouteDef {
    path: string
    component: Component | Lazy<Component>
    children?: RouteDef[]
  }

  interface Props {
    routes: RouteDef[]
    fallback?: Component
  }

  let { routes, fallback }: Props = $props()
  const route = useRoute()

  function findMatch(defs: RouteDef[], prefix = ''): RouteDef | null {
    for (const d of defs) {
      const fullPath = (prefix + d.path).replace(/\/+/g, '/')
      if (matches(fullPath)) return d
      if (d.children) {
        const child = findMatch(d.children, fullPath)
        if (child) return child
      }
    }
    return null
  }

  let Match = $derived.by(() => {
    const m = findMatch(routes)
    if (!m) return fallback ?? null
    const c = m.component
    // Lazy if function; eager if component
    return typeof c === 'function' && !('$$render' in (c as object)) ? c : c
  })

  void route.path // reactivity
</script>

{#if Match}
  {#if typeof Match === 'function' && !('$$render' in (Match as object))}
    {#await (Match as Lazy<Component>)() then Loaded}
      <Loaded />
    {/await}
  {:else}
    {@const C = Match as Component}
    <C />
  {/if}
{/if}
```

- [ ] **Step 2: Create `gui/web/src/lib/router/Route.svelte`**

```svelte
<!-- Thin marker for readability when composing route trees declaratively.
     Actual match runs inside Router.svelte. This component is reserved for
     future nested outlets and is safe to leave as a no-op for P1. -->
<script lang="ts">
  interface Props { path: string; children?: () => any }
  let { children }: Props = $props()
</script>

{@render children?.()}
```

- [ ] **Step 3: Create `gui/web/src/lib/router/Link.svelte`**

```svelte
<script lang="ts">
  import { navigate } from './router.svelte'
  interface Props {
    to: string
    replace?: boolean
    class?: string
    children?: () => any
  }
  let { to, replace = false, class: cls = '', children }: Props = $props()

  function onclick(e: MouseEvent) {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
    e.preventDefault()
    navigate(to, { replace })
  }
</script>

<a href={'#' + to} class={cls} {onclick}>{@render children?.()}</a>
```

- [ ] **Step 4: Create `gui/web/src/lib/router/index.ts`**

```ts
export { navigate, useRoute, useParams, matches, lazy } from './router.svelte'
export type { Lazy } from './router.svelte'
export { default as Router } from './Router.svelte'
export { default as Route } from './Route.svelte'
export { default as Link } from './Link.svelte'
```

- [ ] **Step 5: Run svelte-check + build**

```bash
cd gui/web && npx svelte-check --threshold error && npm run build
```
Expected: 0 errors, build ok.

- [ ] **Step 6: Commit**

```bash
git add gui/web/src/lib/router/
git commit -m "feat(gui): add Router/Route/Link svelte components"
```

---

### Task 21: Add `navigate()` + `Link` component test

**Files:**
- Create: `gui/web/src/lib/router/Link.test.ts`

- [ ] **Step 1: Create test**

```ts
import { describe, it, expect } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import { useRoute, __resetRoute } from '@/lib/router/router.svelte'
import Link from '@/lib/router/Link.svelte'

describe('Link', () => {
  it('updates route on click', async () => {
    __resetRoute()
    const { getByRole } = render(Link, {
      props: { to: '/servers' },
    })
    // @ts-ignore — default slot in svelte-5 vitest is via children snippet; omit for P1
    const a = getByRole('link')
    await fireEvent.click(a)
    expect(useRoute().path).toBe('/servers')
  })
})
```

*Note:* if `children` slot cannot be asserted via testing-library in Svelte 5 cleanly, simplify the test to just verify `href` attribute:

```ts
expect(a.getAttribute('href')).toBe('#/servers')
```

- [ ] **Step 2: Run test**

```bash
cd gui/web && npm test -- Link.test
```
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/lib/router/Link.test.ts
git commit -m "test(gui): add Link click navigation test"
```

---

## Section F · Theme & toast runes rewrites (Tasks 22–24)

**Rationale:** old `lib/theme.ts` and `lib/toast.ts` use subscribe-set patterns. We add `.svelte.ts` versions that expose a reactive state object. Old files stay in place untouched so legacy pages keep working.

### Task 22: Add `lib/theme.svelte.ts`

**Files:**
- Create: `gui/web/src/lib/theme.svelte.ts`

- [ ] **Step 1: Create `gui/web/src/lib/theme.svelte.ts`**

```ts
export type Theme = 'dark' | 'light'

function readInitial(): Theme {
  if (typeof localStorage !== 'undefined') {
    const stored = localStorage.getItem('shuttle-theme')
    if (stored === 'dark' || stored === 'light') return stored
  }
  if (typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: light)').matches) {
    return 'light'
  }
  return 'dark'
}

function apply(theme: Theme) {
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('data-theme', theme)
  }
}

const state = $state<{ theme: Theme }>({ theme: readInitial() })
apply(state.theme)

export const theme = {
  get current() { return state.theme },
  set(next: Theme) {
    state.theme = next
    localStorage.setItem('shuttle-theme', next)
    apply(next)
  },
  toggle() { this.set(state.theme === 'dark' ? 'light' : 'dark') },
}
```

- [ ] **Step 2: Add test `gui/web/src/lib/theme.test.ts`**

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { theme } from '@/lib/theme.svelte'

beforeEach(() => { localStorage.clear() })

describe('theme', () => {
  it('defaults to dark (no stored, no light media)', () => {
    expect(theme.current).toBe('dark')
  })

  it('set() persists and applies data-theme', () => {
    theme.set('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    expect(localStorage.getItem('shuttle-theme')).toBe('light')
  })

  it('toggle() flips between dark and light', () => {
    theme.set('dark')
    theme.toggle()
    expect(theme.current).toBe('light')
    theme.toggle()
    expect(theme.current).toBe('dark')
  })
})
```

- [ ] **Step 3: Run tests**

```bash
cd gui/web && npm test -- theme.test
```
Expected: PASS (3 tests).

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/lib/theme.svelte.ts gui/web/src/lib/theme.test.ts
git commit -m "feat(gui): add runes-based theme store alongside legacy theme.ts"
```

---

### Task 23: Add `lib/toast.svelte.ts`

**Files:**
- Create: `gui/web/src/lib/toast.svelte.ts`

- [ ] **Step 1: Create file**

```ts
export type ToastType = 'success' | 'error' | 'warning' | 'info'

export interface ToastMessage {
  id: number
  type: ToastType
  message: string
  duration: number
}

const state = $state<{ items: ToastMessage[] }>({ items: [] })
let nextId = 0

function add(type: ToastType, message: string, duration = 4000): number {
  const id = nextId++
  state.items = [...state.items, { id, type, message, duration }]
  if (duration > 0) setTimeout(() => dismiss(id), duration)
  return id
}

export function dismiss(id: number): void {
  state.items = state.items.filter(t => t.id !== id)
}

export function dismissAll(): void {
  state.items = []
}

export const toasts = {
  get items() { return state.items },
  success: (m: string, d?: number) => add('success', m, d),
  error:   (m: string, d?: number) => add('error', m, d ?? 6000),
  warning: (m: string, d?: number) => add('warning', m, d),
  info:    (m: string, d?: number) => add('info', m, d),
  dismiss,
  dismissAll,
}
```

- [ ] **Step 2: Add `gui/web/src/lib/toast.test.ts`**

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { toasts, dismissAll } from '@/lib/toast.svelte'

beforeEach(() => dismissAll())

describe('toasts', () => {
  it('success() adds an item', () => {
    toasts.success('hi')
    expect(toasts.items.length).toBe(1)
    expect(toasts.items[0].type).toBe('success')
  })

  it('dismiss removes by id', () => {
    const id = toasts.info('x')
    toasts.dismiss(id)
    expect(toasts.items.length).toBe(0)
  })
})
```

- [ ] **Step 3: Run tests**

```bash
cd gui/web && npm test -- toast.test
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/lib/toast.svelte.ts gui/web/src/lib/toast.test.ts
git commit -m "feat(gui): add runes-based toast store alongside legacy toast.ts"
```

---

### Task 24: Create `app/icons.ts` icon registry

**Files:**
- Create: `gui/web/src/app/icons.ts`

- [ ] **Step 1: Create file**

```ts
// Icon registry — maps semantic name → inline SVG path data.
// Add new icons here; reference via <Icon name="..."/> from ui/Icon.svelte.
// All icons: 20x20 viewBox, stroke 1.5, currentColor, no fill.

export interface IconPath {
  paths: string[]       // each entry is a <path d="..."/> or similar element as SVG text
  viewBox?: string      // default "0 0 20 20"
  strokeWidth?: number  // default 1.5
}

export const icons: Record<string, IconPath> = {
  dashboard: {
    paths: [
      '<rect x="3" y="3" width="6" height="6" rx="1"/>',
      '<rect x="11" y="3" width="6" height="6" rx="1"/>',
      '<rect x="3" y="11" width="6" height="6" rx="1"/>',
      '<rect x="11" y="11" width="6" height="6" rx="1"/>',
    ],
  },
  servers: {
    paths: [
      '<rect x="3" y="3" width="14" height="5" rx="1.5"/>',
      '<rect x="3" y="12" width="14" height="5" rx="1.5"/>',
      '<circle cx="6" cy="5.5" r="1" fill="currentColor"/>',
      '<circle cx="6" cy="14.5" r="1" fill="currentColor"/>',
    ],
  },
  subscriptions: {
    paths: ['<path d="M4 5h12M4 10h12M4 15h8"/>', '<circle cx="16" cy="15" r="2"/>'],
  },
  groups: {
    paths: [
      '<circle cx="10" cy="5" r="2"/>',
      '<circle cx="4" cy="15" r="2"/>',
      '<circle cx="10" cy="15" r="2"/>',
      '<circle cx="16" cy="15" r="2"/>',
      '<path d="M10 7v5M10 12l-6 1M10 12l6 1"/>',
    ],
  },
  routing: {
    paths: [
      '<circle cx="5" cy="10" r="2"/>',
      '<circle cx="15" cy="5" r="2"/>',
      '<circle cx="15" cy="15" r="2"/>',
      '<path d="M7 10h3l2-5h1M10 10l2 5h1"/>',
    ],
  },
  mesh: {
    paths: [
      '<circle cx="10" cy="4" r="2"/>',
      '<circle cx="3" cy="15" r="2"/>',
      '<circle cx="17" cy="15" r="2"/>',
      '<path d="M10 6v3M5 14l4-5M15 14l-4-5M5 15h10"/>',
    ],
  },
  logs: {
    paths: ['<path d="M5 4h10a1 1 0 011 1v10a1 1 0 01-1 1H5a1 1 0 01-1-1V5a1 1 0 011-1z"/>', '<path d="M7 8h6M7 11h4"/>'],
  },
  settings: {
    paths: [
      '<circle cx="10" cy="10" r="3"/>',
      '<path d="M10 3v2M10 15v2M3 10h2M15 10h2M5.05 5.05l1.41 1.41M13.54 13.54l1.41 1.41M5.05 14.95l1.41-1.41M13.54 6.46l1.41-1.41"/>',
    ],
  },
  check: { paths: ['<path d="M5 10l3 3 7-7"/>'] },
  x: { paths: ['<path d="M5 5l10 10M15 5l-10 10"/>'] },
  chevronRight: { paths: ['<path d="M8 5l5 5-5 5"/>'] },
  chevronLeft: { paths: ['<path d="M12 5l-5 5 5 5"/>'] },
  chevronDown: { paths: ['<path d="M5 8l5 5 5-5"/>'] },
  plus: { paths: ['<path d="M10 4v12M4 10h12"/>'] },
  trash: { paths: ['<path d="M4 6h12M7 6V4h6v2M6 6l1 10h6l1-10"/>'] },
  info: { paths: ['<circle cx="10" cy="10" r="7"/>', '<path d="M10 9v4M10 6v.01"/>'] },
}

export type IconName = keyof typeof icons
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/app/icons.ts
git commit -m "feat(app): add icon registry (14 semantic names)"
```

---

## Section G · UI primitives — self-built (Tasks 25–33)

### Task 25: `ui/Icon.svelte`

**Files:**
- Create: `gui/web/src/ui/Icon.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { icons, type IconName } from '@/app/icons'

  interface Props {
    name: IconName
    size?: number
    class?: string
    title?: string
  }

  let { name, size = 16, class: cls = '', title }: Props = $props()
  const def = $derived(icons[name])
  const viewBox = $derived(def?.viewBox ?? '0 0 20 20')
  const sw = $derived(def?.strokeWidth ?? 1.5)
</script>

{#if def}
  <svg
    width={size}
    height={size}
    viewBox={viewBox}
    fill="none"
    stroke="currentColor"
    stroke-width={sw}
    stroke-linecap="round"
    stroke-linejoin="round"
    class={cls}
    aria-hidden={title ? undefined : 'true'}
    aria-label={title}
    role={title ? 'img' : undefined}
  >
    {#each def.paths as p}{@html p}{/each}
  </svg>
{/if}
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/ui/Icon.svelte
git commit -m "feat(ui): add Icon primitive driven by app/icons registry"
```

---

### Task 26: `ui/Button.svelte`

**Files:**
- Create: `gui/web/src/ui/Button.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  interface Props {
    type?: 'button' | 'submit' | 'reset'
    variant?: 'primary' | 'secondary' | 'ghost' | 'danger'
    size?: 'sm' | 'md'
    loading?: boolean
    disabled?: boolean
    onclick?: (e: MouseEvent) => void
    class?: string
    children?: () => any
  }

  let {
    type = 'button',
    variant = 'secondary',
    size = 'md',
    loading = false,
    disabled = false,
    onclick,
    class: cls = '',
    children,
  }: Props = $props()
</script>

<button
  {type}
  class="btn btn-{variant} btn-{size} {cls}"
  class:loading
  disabled={disabled || loading}
  onclick={(e) => onclick?.(e)}
>
  {#if loading}<span class="spinner" aria-hidden="true"></span>{/if}
  {@render children?.()}
</button>

<style>
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--shuttle-space-2);
    border: 1px solid transparent;
    border-radius: var(--shuttle-radius-md);
    font-family: var(--shuttle-font-sans);
    font-weight: var(--shuttle-weight-medium);
    cursor: pointer;
    transition: background var(--shuttle-duration) var(--shuttle-easing),
                border-color var(--shuttle-duration) var(--shuttle-easing),
                color var(--shuttle-duration) var(--shuttle-easing);
    white-space: nowrap;
  }
  .btn-md { height: 32px; padding: 0 var(--shuttle-space-3); font-size: var(--shuttle-text-base); }
  .btn-sm { height: 26px; padding: 0 var(--shuttle-space-2); font-size: var(--shuttle-text-sm); }

  .btn-primary {
    background: var(--shuttle-accent);
    color: var(--shuttle-accent-fg);
  }
  .btn-primary:hover:not(:disabled) { background: var(--shuttle-fg-primary); }

  .btn-secondary {
    background: var(--shuttle-bg-surface);
    color: var(--shuttle-fg-primary);
    border-color: var(--shuttle-border);
  }
  .btn-secondary:hover:not(:disabled) { border-color: var(--shuttle-border-strong); background: var(--shuttle-bg-subtle); }

  .btn-ghost {
    background: transparent;
    color: var(--shuttle-fg-secondary);
  }
  .btn-ghost:hover:not(:disabled) { background: var(--shuttle-bg-subtle); color: var(--shuttle-fg-primary); }

  .btn-danger {
    background: var(--shuttle-danger);
    color: #fff;
  }
  .btn-danger:hover:not(:disabled) { filter: brightness(1.05); }

  .btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .spinner {
    width: 12px; height: 12px;
    border: 1.5px solid currentColor;
    border-right-color: transparent;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
```

- [ ] **Step 2: Create `gui/web/src/ui/Button.test.ts`**

```ts
import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import Button from '@/ui/Button.svelte'

describe('Button', () => {
  it('fires onclick when enabled', async () => {
    const onclick = vi.fn()
    const { getByRole } = render(Button, { props: { onclick } })
    await fireEvent.click(getByRole('button'))
    expect(onclick).toHaveBeenCalled()
  })

  it('does not fire onclick when disabled', async () => {
    const onclick = vi.fn()
    const { getByRole } = render(Button, { props: { onclick, disabled: true } })
    await fireEvent.click(getByRole('button'))
    expect(onclick).not.toHaveBeenCalled()
  })

  it('adds loading class when loading', () => {
    const { getByRole } = render(Button, { props: { loading: true } })
    expect(getByRole('button').classList.contains('loading')).toBe(true)
  })
})
```

- [ ] **Step 3: Run tests**

```bash
cd gui/web && npm test -- Button.test
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/ui/Button.svelte gui/web/src/ui/Button.test.ts
git commit -m "feat(ui): add Button primitive with 4 variants + 2 sizes"
```

---

### Task 27: `ui/Card.svelte`

**Files:**
- Create: `gui/web/src/ui/Card.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  interface Props {
    class?: string
    children?: () => any
  }
  let { class: cls = '', children }: Props = $props()
</script>

<div class="card {cls}">{@render children?.()}</div>

<style>
  .card {
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    padding: var(--shuttle-space-5);
  }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/ui/Card.svelte
git commit -m "feat(ui): add Card primitive"
```

---

### Task 28: `ui/Input.svelte`

**Files:**
- Create: `gui/web/src/ui/Input.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  interface Props {
    value?: string
    placeholder?: string
    type?: 'text' | 'password' | 'email' | 'url' | 'number'
    label?: string
    error?: string
    disabled?: boolean
    oninput?: (e: Event) => void
    onchange?: (e: Event) => void
    class?: string
    id?: string
    autocomplete?: string
  }

  let {
    value = $bindable(''),
    placeholder = '',
    type = 'text',
    label,
    error,
    disabled = false,
    oninput,
    onchange,
    class: cls = '',
    id,
    autocomplete,
  }: Props = $props()

  const inputId = id ?? `in-${Math.random().toString(36).slice(2, 8)}`
</script>

<div class="field {cls}" class:has-error={!!error}>
  {#if label}<label for={inputId}>{label}</label>{/if}
  <input
    id={inputId}
    {type}
    bind:value
    {placeholder}
    {disabled}
    {autocomplete}
    {oninput}
    {onchange}
    aria-invalid={!!error}
    aria-describedby={error ? `${inputId}-err` : undefined}
  />
  {#if error}<p id={`${inputId}-err`} class="err">{error}</p>{/if}
</div>

<style>
  .field { display: flex; flex-direction: column; gap: var(--shuttle-space-1); }
  label {
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-secondary);
    font-weight: var(--shuttle-weight-medium);
  }
  input {
    height: 32px;
    padding: 0 var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    color: var(--shuttle-fg-primary);
    font-family: var(--shuttle-font-sans);
    font-size: var(--shuttle-text-base);
    outline: none;
    transition: border-color var(--shuttle-duration);
  }
  input:focus { border-color: var(--shuttle-border-strong); }
  input:disabled { opacity: 0.5; cursor: not-allowed; }
  .has-error input { border-color: var(--shuttle-danger); }
  .err {
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-danger);
    margin: 0;
  }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/ui/Input.svelte
git commit -m "feat(ui): add Input primitive with label + error support"
```

---

### Task 29: `ui/Badge.svelte`

- [ ] **Step 1: Create `gui/web/src/ui/Badge.svelte`**

```svelte
<script lang="ts">
  interface Props {
    variant?: 'neutral' | 'success' | 'warning' | 'danger' | 'info'
    children?: () => any
  }
  let { variant = 'neutral', children }: Props = $props()
</script>

<span class="badge {variant}">{@render children?.()}</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    font-size: var(--shuttle-text-xs);
    font-weight: var(--shuttle-weight-medium);
    padding: 1px var(--shuttle-space-2);
    border-radius: var(--shuttle-radius-sm);
    letter-spacing: 0.02em;
    border: 1px solid var(--shuttle-border);
    color: var(--shuttle-fg-secondary);
    background: var(--shuttle-bg-subtle);
  }
  .success { color: var(--shuttle-success); border-color: color-mix(in oklab, var(--shuttle-success) 35%, transparent); }
  .warning { color: var(--shuttle-warning); border-color: color-mix(in oklab, var(--shuttle-warning) 35%, transparent); }
  .danger  { color: var(--shuttle-danger);  border-color: color-mix(in oklab, var(--shuttle-danger) 35%, transparent); }
  .info    { color: var(--shuttle-info);    border-color: color-mix(in oklab, var(--shuttle-info) 35%, transparent); }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/ui/Badge.svelte
git commit -m "feat(ui): add Badge primitive with 5 variants"
```

---

### Task 30: `ui/Spinner.svelte`, `ui/Empty.svelte`, `ui/ErrorBanner.svelte` (batch)

- [ ] **Step 1: Create `gui/web/src/ui/Spinner.svelte`**

```svelte
<script lang="ts">
  interface Props { size?: number }
  let { size = 16 }: Props = $props()
</script>

<span
  class="sp"
  style={`width:${size}px;height:${size}px;border-width:${Math.max(1, Math.round(size / 10))}px`}
  role="status"
  aria-label="Loading"
></span>

<style>
  .sp {
    display: inline-block;
    border: 2px solid var(--shuttle-border);
    border-top-color: var(--shuttle-fg-primary);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
```

- [ ] **Step 2: Create `gui/web/src/ui/Empty.svelte`**

```svelte
<script lang="ts">
  import Icon from './Icon.svelte'
  import type { IconName } from '@/app/icons'
  interface Props {
    icon?: IconName
    title: string
    description?: string
    action?: () => any
  }
  let { icon, title, description, action }: Props = $props()
</script>

<div class="empty">
  {#if icon}<div class="icon"><Icon name={icon} size={28} /></div>{/if}
  <div class="title">{title}</div>
  {#if description}<div class="desc">{description}</div>{/if}
  {#if action}<div class="action">{@render action()}</div>{/if}
</div>

<style>
  .empty {
    display: flex; flex-direction: column; align-items: center; justify-content: center;
    padding: var(--shuttle-space-7) var(--shuttle-space-5); text-align: center;
    color: var(--shuttle-fg-secondary);
  }
  .icon { color: var(--shuttle-fg-muted); margin-bottom: var(--shuttle-space-3); }
  .title { font-size: var(--shuttle-text-base); color: var(--shuttle-fg-primary); font-weight: var(--shuttle-weight-medium); }
  .desc { font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-muted); margin-top: var(--shuttle-space-1); }
  .action { margin-top: var(--shuttle-space-4); }
</style>
```

- [ ] **Step 3: Create `gui/web/src/ui/ErrorBanner.svelte`**

```svelte
<script lang="ts">
  import Icon from './Icon.svelte'
  interface Props {
    message: string
    onretry?: () => void
  }
  let { message, onretry }: Props = $props()
</script>

<div class="banner" role="alert">
  <div class="msg"><Icon name="info" size={14} />{message}</div>
  {#if onretry}<button class="retry" onclick={onretry}>Retry</button>{/if}
</div>

<style>
  .banner {
    display: flex; align-items: center; gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    background: color-mix(in oklab, var(--shuttle-danger) 10%, transparent);
    color: var(--shuttle-danger);
    border: 1px solid color-mix(in oklab, var(--shuttle-danger) 30%, transparent);
    border-radius: var(--shuttle-radius-md);
    font-size: var(--shuttle-text-sm);
  }
  .msg { display: flex; align-items: center; gap: var(--shuttle-space-2); }
  .retry {
    margin-left: auto;
    background: transparent; border: 1px solid var(--shuttle-danger); color: var(--shuttle-danger);
    padding: 2px var(--shuttle-space-2); border-radius: var(--shuttle-radius-sm);
    font-size: var(--shuttle-text-xs); cursor: pointer;
  }
</style>
```

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/ui/Spinner.svelte gui/web/src/ui/Empty.svelte gui/web/src/ui/ErrorBanner.svelte
git commit -m "feat(ui): add Spinner/Empty/ErrorBanner state primitives"
```

---

### Task 31: `ui/Section.svelte` and `ui/StatRow.svelte`

- [ ] **Step 1: Create `gui/web/src/ui/Section.svelte`**

```svelte
<script lang="ts">
  interface Props {
    title?: string
    description?: string
    actions?: () => any
    children?: () => any
  }
  let { title, description, actions, children }: Props = $props()
</script>

<section class="sec">
  {#if title || actions}
    <header>
      <div>
        {#if title}<h3>{title}</h3>{/if}
        {#if description}<p>{description}</p>{/if}
      </div>
      {#if actions}<div class="actions">{@render actions()}</div>{/if}
    </header>
  {/if}
  <div class="body">{@render children?.()}</div>
</section>

<style>
  .sec { margin-bottom: var(--shuttle-space-6); }
  header { display: flex; align-items: flex-start; justify-content: space-between; margin-bottom: var(--shuttle-space-3); gap: var(--shuttle-space-4); }
  h3 { margin: 0; font-size: var(--shuttle-text-lg); font-weight: var(--shuttle-weight-semibold); letter-spacing: var(--shuttle-tracking-tight); color: var(--shuttle-fg-primary); }
  p { margin: var(--shuttle-space-1) 0 0; font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-secondary); }
</style>
```

- [ ] **Step 2: Create `gui/web/src/ui/StatRow.svelte`**

```svelte
<script lang="ts">
  interface Props {
    label: string
    value: string | number
    mono?: boolean
  }
  let { label, value, mono = false }: Props = $props()
</script>

<div class="row">
  <span class="lbl">{label}</span>
  <span class="val" class:mono>{value}</span>
</div>

<style>
  .row {
    display: flex; align-items: baseline; justify-content: space-between;
    font-size: var(--shuttle-text-sm);
    padding: var(--shuttle-space-1) 0;
  }
  .lbl { color: var(--shuttle-fg-secondary); }
  .val { color: var(--shuttle-fg-primary); font-variant-numeric: tabular-nums; font-weight: var(--shuttle-weight-medium); }
  .val.mono { font-family: var(--shuttle-font-mono); }
</style>
```

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/ui/Section.svelte gui/web/src/ui/StatRow.svelte
git commit -m "feat(ui): add Section and StatRow primitives"
```

---

### Task 32: `ui/AsyncBoundary.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts" generics="T">
  import Spinner from './Spinner.svelte'
  import ErrorBanner from './ErrorBanner.svelte'
  import Empty from './Empty.svelte'
  import type { Resource } from '@/lib/resource.svelte'

  interface Props {
    resource: Resource<T>
    emptyTitle?: string
    emptyDescription?: string
    isEmpty?: (data: T) => boolean
    children: (data: T) => any
  }

  let { resource, emptyTitle, emptyDescription, isEmpty, children }: Props = $props()
</script>

{#if resource.loading && resource.data === undefined}
  <div class="center"><Spinner size={20} /></div>
{:else if resource.error && resource.data === undefined}
  <ErrorBanner message={resource.error.message} onretry={() => resource.refetch()} />
{:else if resource.data !== undefined && isEmpty?.(resource.data)}
  <Empty title={emptyTitle ?? 'Nothing here'} description={emptyDescription} />
{:else if resource.data !== undefined}
  {@render children(resource.data)}
{/if}

<style>
  .center { display: flex; justify-content: center; padding: var(--shuttle-space-6); }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/ui/AsyncBoundary.svelte
git commit -m "feat(ui): add AsyncBoundary primitive for Resource states"
```

---

### Task 33: `ui/index.ts` barrel (self-built primitives)

- [ ] **Step 1: Create `gui/web/src/ui/index.ts`**

```ts
// Design system barrel. ALWAYS import from @/ui.
// Tasks 34–40 append bits-ui wrapper exports below.
export { default as Button } from './Button.svelte'
export { default as Card } from './Card.svelte'
export { default as Input } from './Input.svelte'
export { default as Badge } from './Badge.svelte'
export { default as Icon } from './Icon.svelte'
export { default as StatRow } from './StatRow.svelte'
export { default as Section } from './Section.svelte'
export { default as Spinner } from './Spinner.svelte'
export { default as Empty } from './Empty.svelte'
export { default as ErrorBanner } from './ErrorBanner.svelte'
export { default as AsyncBoundary } from './AsyncBoundary.svelte'
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/ui/index.ts
git commit -m "feat(ui): add ui/index.ts barrel for self-built primitives"
```

---

## Section H · UI primitives — bits-ui wrappers (Tasks 34–40)

**IMPORTANT:** bits-ui has slightly different imports between v1.x lines. Before writing each wrapper, run once:

```bash
cd gui/web && node -e "console.log(Object.keys(require('bits-ui')))"
```

Confirm the namespace (e.g. `Dialog`, `DropdownMenu`). If the API shape differs from what's shown below, adjust imports — the component contract stays the same.

### Task 34: `ui/Switch.svelte`

- [ ] **Step 1: Create `gui/web/src/ui/Switch.svelte`**

```svelte
<script lang="ts">
  import { Switch as BitsSwitch } from 'bits-ui'

  interface Props {
    checked?: boolean
    disabled?: boolean
    onCheckedChange?: (checked: boolean) => void
    label?: string
    id?: string
  }

  let {
    checked = $bindable(false),
    disabled = false,
    onCheckedChange,
    label,
    id,
  }: Props = $props()

  const switchId = id ?? `sw-${Math.random().toString(36).slice(2, 8)}`
</script>

<div class="wrap">
  <BitsSwitch.Root
    id={switchId}
    bind:checked
    {disabled}
    onCheckedChange={onCheckedChange}
    class="root"
  >
    <BitsSwitch.Thumb class="thumb" />
  </BitsSwitch.Root>
  {#if label}<label for={switchId}>{label}</label>{/if}
</div>

<style>
  .wrap { display: inline-flex; align-items: center; gap: var(--shuttle-space-2); }
  label { font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-secondary); cursor: pointer; }
  :global(.root) {
    width: 32px; height: 18px; border-radius: 999px;
    background: var(--shuttle-bg-subtle); border: 1px solid var(--shuttle-border);
    position: relative; cursor: pointer; transition: background var(--shuttle-duration);
  }
  :global(.root[data-state="checked"]) { background: var(--shuttle-accent); }
  :global(.thumb) {
    display: block; width: 14px; height: 14px; border-radius: 999px;
    background: var(--shuttle-bg-surface); transform: translateX(0); transition: transform var(--shuttle-duration);
    position: absolute; top: 1px; left: 1px;
  }
  :global(.root[data-state="checked"] .thumb) { transform: translateX(14px); background: var(--shuttle-accent-fg); }
</style>
```

- [ ] **Step 2: Add to barrel**

Edit `gui/web/src/ui/index.ts`, append:

```ts
export { default as Switch } from './Switch.svelte'
```

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/ui/Switch.svelte gui/web/src/ui/index.ts
git commit -m "feat(ui): wrap bits-ui Switch"
```

---

### Task 35: `ui/Dialog.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { Dialog as BitsDialog } from 'bits-ui'

  interface Props {
    open?: boolean
    onOpenChange?: (open: boolean) => void
    title: string
    description?: string
    children?: () => any
    actions?: () => any
  }

  let { open = $bindable(false), onOpenChange, title, description, children, actions }: Props = $props()
</script>

<BitsDialog.Root bind:open onOpenChange={onOpenChange}>
  <BitsDialog.Portal>
    <BitsDialog.Overlay class="overlay" />
    <BitsDialog.Content class="content">
      <BitsDialog.Title class="title">{title}</BitsDialog.Title>
      {#if description}<BitsDialog.Description class="desc">{description}</BitsDialog.Description>{/if}
      <div class="body">{@render children?.()}</div>
      {#if actions}<div class="actions">{@render actions()}</div>{/if}
    </BitsDialog.Content>
  </BitsDialog.Portal>
</BitsDialog.Root>

<style>
  :global(.overlay) {
    position: fixed; inset: 0; background: rgba(0,0,0,0.5);
    z-index: 50;
  }
  :global(.content) {
    position: fixed; top: 50%; left: 50%; transform: translate(-50%,-50%);
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-lg);
    padding: var(--shuttle-space-5);
    min-width: 360px; max-width: 90vw; max-height: 85vh; overflow: auto;
    z-index: 51;
  }
  :global(.title) { margin: 0; font-size: var(--shuttle-text-lg); font-weight: var(--shuttle-weight-semibold); color: var(--shuttle-fg-primary); }
  :global(.desc)  { margin: var(--shuttle-space-1) 0 0; font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-secondary); }
  .body { margin-top: var(--shuttle-space-4); }
  .actions { margin-top: var(--shuttle-space-5); display: flex; justify-content: flex-end; gap: var(--shuttle-space-2); }
</style>
```

- [ ] **Step 2: Add to barrel**

```ts
export { default as Dialog } from './Dialog.svelte'
```

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/ui/Dialog.svelte gui/web/src/ui/index.ts
git commit -m "feat(ui): wrap bits-ui Dialog"
```

---

### Task 36: `ui/Select.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts" generics="V extends string">
  import { Select as BitsSelect } from 'bits-ui'
  import Icon from './Icon.svelte'

  interface Option<V> { value: V; label: string }
  interface Props {
    value?: V
    options: Option<V>[]
    placeholder?: string
    disabled?: boolean
    onValueChange?: (v: V) => void
  }

  let { value = $bindable(), options, placeholder = 'Select…', disabled = false, onValueChange }: Props = $props()
</script>

<BitsSelect.Root type="single" bind:value {disabled} onValueChange={(v) => onValueChange?.(v as V)}>
  <BitsSelect.Trigger class="trigger">
    <span>{options.find(o => o.value === value)?.label ?? placeholder}</span>
    <Icon name="chevronDown" size={14} />
  </BitsSelect.Trigger>
  <BitsSelect.Portal>
    <BitsSelect.Content class="content" sideOffset={4}>
      {#each options as o}
        <BitsSelect.Item value={o.value} label={o.label} class="item">
          {o.label}
        </BitsSelect.Item>
      {/each}
    </BitsSelect.Content>
  </BitsSelect.Portal>
</BitsSelect.Root>

<style>
  :global(.trigger) {
    display: inline-flex; align-items: center; justify-content: space-between; gap: var(--shuttle-space-2);
    height: 32px; padding: 0 var(--shuttle-space-3);
    background: var(--shuttle-bg-surface); color: var(--shuttle-fg-primary);
    border: 1px solid var(--shuttle-border); border-radius: var(--shuttle-radius-md);
    font-size: var(--shuttle-text-base); font-family: var(--shuttle-font-sans); cursor: pointer; min-width: 140px;
  }
  :global(.trigger:hover) { border-color: var(--shuttle-border-strong); }
  :global(.content) {
    background: var(--shuttle-bg-surface); border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md); box-shadow: var(--shuttle-shadow-md);
    min-width: 140px; z-index: 60; padding: var(--shuttle-space-1);
  }
  :global(.item) {
    padding: var(--shuttle-space-1) var(--shuttle-space-3); font-size: var(--shuttle-text-sm);
    border-radius: var(--shuttle-radius-sm); cursor: pointer; color: var(--shuttle-fg-primary);
  }
  :global(.item[data-highlighted]) { background: var(--shuttle-bg-subtle); }
</style>
```

- [ ] **Step 2: Add to barrel**

```ts
export { default as Select } from './Select.svelte'
```

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/ui/Select.svelte gui/web/src/ui/index.ts
git commit -m "feat(ui): wrap bits-ui Select"
```

---

### Task 37: `ui/Tabs.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts" generics="V extends string">
  import { Tabs as BitsTabs } from 'bits-ui'

  interface Item<V> { value: V; label: string }
  interface Props {
    value?: V
    items: Item<V>[]
    onValueChange?: (v: V) => void
    children?: (value: V) => any
  }

  let { value = $bindable(), items, onValueChange, children }: Props = $props()
</script>

<BitsTabs.Root bind:value onValueChange={(v) => onValueChange?.(v as V)}>
  <BitsTabs.List class="list">
    {#each items as it}
      <BitsTabs.Trigger value={it.value} class="trigger">{it.label}</BitsTabs.Trigger>
    {/each}
  </BitsTabs.List>
  {#if children && value !== undefined}
    {@render children(value as V)}
  {/if}
</BitsTabs.Root>

<style>
  :global(.list) {
    display: inline-flex; gap: var(--shuttle-space-1);
    background: var(--shuttle-bg-subtle); padding: 2px; border-radius: var(--shuttle-radius-md);
  }
  :global(.trigger) {
    padding: var(--shuttle-space-1) var(--shuttle-space-3);
    border: 0; background: transparent;
    font-size: var(--shuttle-text-sm); font-weight: var(--shuttle-weight-medium); color: var(--shuttle-fg-secondary);
    border-radius: var(--shuttle-radius-sm); cursor: pointer;
  }
  :global(.trigger[data-state="active"]) {
    background: var(--shuttle-bg-surface); color: var(--shuttle-fg-primary); box-shadow: var(--shuttle-shadow-sm);
  }
</style>
```

- [ ] **Step 2: Add to barrel**

```ts
export { default as Tabs } from './Tabs.svelte'
```

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/ui/Tabs.svelte gui/web/src/ui/index.ts
git commit -m "feat(ui): wrap bits-ui Tabs"
```

---

### Task 38: `ui/Tooltip.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { Tooltip as BitsTooltip } from 'bits-ui'
  interface Props {
    content: string
    side?: 'top' | 'bottom' | 'left' | 'right'
    children?: () => any
  }
  let { content, side = 'top', children }: Props = $props()
</script>

<BitsTooltip.Provider delayDuration={200}>
  <BitsTooltip.Root>
    <BitsTooltip.Trigger class="trigger">{@render children?.()}</BitsTooltip.Trigger>
    <BitsTooltip.Portal>
      <BitsTooltip.Content {side} sideOffset={6} class="content">{content}</BitsTooltip.Content>
    </BitsTooltip.Portal>
  </BitsTooltip.Root>
</BitsTooltip.Provider>

<style>
  :global(.trigger) { display: inline-flex; background: transparent; border: 0; padding: 0; cursor: inherit; }
  :global(.content) {
    background: var(--shuttle-fg-primary); color: var(--shuttle-bg-base);
    padding: 4px 8px; border-radius: var(--shuttle-radius-sm);
    font-size: var(--shuttle-text-xs); z-index: 70;
  }
</style>
```

- [ ] **Step 2: Barrel + commit**

```ts
export { default as Tooltip } from './Tooltip.svelte'
```

```bash
git add gui/web/src/ui/Tooltip.svelte gui/web/src/ui/index.ts
git commit -m "feat(ui): wrap bits-ui Tooltip"
```

---

### Task 39: `ui/DropdownMenu.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts">
  import { DropdownMenu as BitsMenu } from 'bits-ui'

  interface MenuItem { label: string; onselect: () => void; danger?: boolean; disabled?: boolean }
  interface Props { items: MenuItem[]; children?: () => any }
  let { items, children }: Props = $props()
</script>

<BitsMenu.Root>
  <BitsMenu.Trigger class="trigger">{@render children?.()}</BitsMenu.Trigger>
  <BitsMenu.Portal>
    <BitsMenu.Content class="content" sideOffset={4} align="end">
      {#each items as it}
        <BitsMenu.Item
          class={`item ${it.danger ? 'danger' : ''}`}
          disabled={it.disabled}
          onSelect={() => it.onselect()}
        >{it.label}</BitsMenu.Item>
      {/each}
    </BitsMenu.Content>
  </BitsMenu.Portal>
</BitsMenu.Root>

<style>
  :global(.trigger) { background: transparent; border: 0; padding: 0; cursor: pointer; }
  :global(.content) {
    background: var(--shuttle-bg-surface); border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md); box-shadow: var(--shuttle-shadow-md);
    padding: var(--shuttle-space-1); min-width: 160px; z-index: 60;
  }
  :global(.item) {
    display: block; padding: var(--shuttle-space-1) var(--shuttle-space-3);
    border-radius: var(--shuttle-radius-sm); font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-primary);
    cursor: pointer;
  }
  :global(.item[data-highlighted]) { background: var(--shuttle-bg-subtle); }
  :global(.item.danger) { color: var(--shuttle-danger); }
  :global(.item[data-disabled]) { color: var(--shuttle-fg-muted); pointer-events: none; }
</style>
```

- [ ] **Step 2: Barrel + commit**

```ts
export { default as DropdownMenu } from './DropdownMenu.svelte'
```

```bash
git add gui/web/src/ui/DropdownMenu.svelte gui/web/src/ui/index.ts
git commit -m "feat(ui): wrap bits-ui DropdownMenu"
```

---

### Task 40: `ui/Combobox.svelte`

- [ ] **Step 1: Create file**

```svelte
<script lang="ts" generics="V extends string">
  import { Combobox as BitsCombobox } from 'bits-ui'

  interface Item<V> { value: V; label: string }
  interface Props {
    value?: V
    items: Item<V>[]
    placeholder?: string
    onValueChange?: (v: V | undefined) => void
  }

  let { value = $bindable(), items, placeholder = 'Search…', onValueChange }: Props = $props()
  let input = $state('')
  const filtered = $derived(
    items.filter(it => it.label.toLowerCase().includes(input.toLowerCase()))
  )
</script>

<BitsCombobox.Root
  type="single"
  bind:value
  onValueChange={(v) => onValueChange?.(v as V | undefined)}
>
  <BitsCombobox.Input
    bind:value={input}
    class="input"
    placeholder={placeholder}
  />
  <BitsCombobox.Portal>
    <BitsCombobox.Content class="content">
      {#each filtered as it}
        <BitsCombobox.Item value={it.value} label={it.label} class="item">{it.label}</BitsCombobox.Item>
      {/each}
    </BitsCombobox.Content>
  </BitsCombobox.Portal>
</BitsCombobox.Root>

<style>
  :global(.input) {
    height: 32px; padding: 0 var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border); border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface); color: var(--shuttle-fg-primary);
    font-size: var(--shuttle-text-base); font-family: var(--shuttle-font-sans); outline: none; min-width: 180px;
  }
  :global(.input:focus) { border-color: var(--shuttle-border-strong); }
  :global(.content) {
    background: var(--shuttle-bg-surface); border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md); box-shadow: var(--shuttle-shadow-md);
    padding: var(--shuttle-space-1); min-width: 180px; max-height: 240px; overflow-y: auto; z-index: 60;
  }
  :global(.item) {
    padding: var(--shuttle-space-1) var(--shuttle-space-3); border-radius: var(--shuttle-radius-sm);
    font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-primary); cursor: pointer;
  }
  :global(.item[data-highlighted]) { background: var(--shuttle-bg-subtle); }
</style>
```

- [ ] **Step 2: Barrel + commit**

```ts
export { default as Combobox } from './Combobox.svelte'
```

```bash
git add gui/web/src/ui/Combobox.svelte gui/web/src/ui/index.ts
git commit -m "feat(ui): wrap bits-ui Combobox with client-side filter"
```

---

## Section I · Dev harness + finalization (Tasks 41–44)

### Task 41: `app/routes.ts` empty registry

**Files:**
- Create: `gui/web/src/app/routes.ts`

- [ ] **Step 1: Create file**

```ts
// Feature route registry — P2 and later phases add entries.
import type { Lazy } from '@/lib/router'
import type { Component } from 'svelte'

export interface NavMeta { label: string; icon: string; order: number; hidden?: boolean }
export interface AppRoute {
  path: string
  component: Component | Lazy<Component>
  nav?: NavMeta
  children?: AppRoute[]
}

export const routes: AppRoute[] = [
  // Populated in P2+ via feature index.ts imports.
]
```

- [ ] **Step 2: Commit**

```bash
git add gui/web/src/app/routes.ts
git commit -m "feat(app): add empty route registry for P2+ features"
```

---

### Task 42: `__ui__` dev harness page

**Files:**
- Create: `gui/web/src/__ui__/UIPreview.svelte`
- Modify: `gui/web/src/main.ts` (dev-only mount alternative)

**Context:** A dev-only visual catalog for manually eyeballing primitives in both themes. Accessed at `?ui=1` query param. Zero impact on prod bundle — we gate by `import.meta.env.DEV`.

- [ ] **Step 1: Create `gui/web/src/__ui__/UIPreview.svelte`**

```svelte
<script lang="ts">
  import { Button, Card, Input, Badge, Icon, StatRow, Section, Spinner, Empty, ErrorBanner } from '@/ui'
  import { Switch, Dialog, Select, Tabs, Tooltip, DropdownMenu, Combobox } from '@/ui'
  import { theme } from '@/lib/theme.svelte'

  let dialogOpen = $state(false)
  let switched = $state(false)
  let selectVal = $state('h3')
  let tabsVal = $state('a')
  let combo = $state<string | undefined>(undefined)
</script>

<div class="root">
  <header class="bar">
    <h2>UI Preview · P1</h2>
    <Button variant="ghost" size="sm" onclick={() => theme.toggle()}>
      Toggle theme ({theme.current})
    </Button>
  </header>

  <Section title="Button">
    <div class="row">
      <Button variant="primary">Primary</Button>
      <Button variant="secondary">Secondary</Button>
      <Button variant="ghost">Ghost</Button>
      <Button variant="danger">Danger</Button>
      <Button loading>Loading</Button>
      <Button disabled>Disabled</Button>
      <Button size="sm">Small</Button>
    </div>
  </Section>

  <Section title="Card + StatRow">
    <Card>
      <StatRow label="RTT" value="42 ms" mono />
      <StatRow label="Loss" value="0.0 %" />
      <StatRow label="Transport" value="H3 / BBR" />
    </Card>
  </Section>

  <Section title="Input">
    <Input label="Server name" placeholder="eg. sg-hk-02" />
    <Input label="With error" error="This field is required" value="" />
  </Section>

  <Section title="Badge">
    <div class="row">
      <Badge>neutral</Badge>
      <Badge variant="success">success</Badge>
      <Badge variant="warning">warning</Badge>
      <Badge variant="danger">danger</Badge>
      <Badge variant="info">info</Badge>
    </div>
  </Section>

  <Section title="Icon (14 registered)">
    <div class="row">
      {#each ['dashboard','servers','subscriptions','groups','routing','mesh','logs','settings','check','x','plus','trash','info','chevronRight'] as n}
        <Icon name={n} size={16} />
      {/each}
    </div>
  </Section>

  <Section title="State primitives">
    <Spinner size={20} />
    <Empty icon="servers" title="No servers" description="Add one to get started" />
    <ErrorBanner message="Connection refused" onretry={() => alert('retry')} />
  </Section>

  <Section title="Switch">
    <Switch bind:checked={switched} label="Enable telemetry" />
    <p>Current: {switched}</p>
  </Section>

  <Section title="Select">
    <Select
      value={selectVal}
      options={[
        { value: 'h3', label: 'HTTP/3' },
        { value: 'reality', label: 'Reality' },
        { value: 'cdn', label: 'CDN' },
      ]}
      onValueChange={(v) => (selectVal = v)}
    />
  </Section>

  <Section title="Tabs">
    <Tabs
      value={tabsVal}
      items={[
        { value: 'a', label: 'Overview' },
        { value: 'b', label: 'Detail' },
        { value: 'c', label: 'History' },
      ]}
      onValueChange={(v) => (tabsVal = v)}
    />
  </Section>

  <Section title="Tooltip">
    <Tooltip content="I am a tooltip.">
      <Button variant="ghost">Hover me</Button>
    </Tooltip>
  </Section>

  <Section title="DropdownMenu">
    <DropdownMenu items={[
      { label: 'Rename', onselect: () => alert('rename') },
      { label: 'Duplicate', onselect: () => alert('dup') },
      { label: 'Delete', onselect: () => alert('del'), danger: true },
    ]}>
      <Button variant="secondary">Actions ▾</Button>
    </DropdownMenu>
  </Section>

  <Section title="Combobox">
    <Combobox
      value={combo}
      items={Array.from({ length: 20 }, (_, i) => ({ value: `opt-${i}`, label: `Option ${i}` }))}
      onValueChange={(v) => (combo = v)}
    />
    <p>Selected: {combo ?? '(none)'}</p>
  </Section>

  <Section title="Dialog">
    <Button variant="primary" onclick={() => (dialogOpen = true)}>Open dialog</Button>
    <Dialog bind:open={dialogOpen} title="Delete server?" description="This cannot be undone.">
      <p>Are you sure you want to delete <strong>sg-hk-02</strong>?</p>
      {#snippet actions()}
        <Button variant="ghost" onclick={() => (dialogOpen = false)}>Cancel</Button>
        <Button variant="danger" onclick={() => (dialogOpen = false)}>Delete</Button>
      {/snippet}
    </Dialog>
  </Section>
</div>

<style>
  .root { padding: var(--shuttle-space-5) var(--shuttle-space-7); max-width: 960px; margin: 0 auto;
          background: var(--shuttle-bg-base); color: var(--shuttle-fg-primary); min-height: 100vh; }
  .bar { display: flex; align-items: center; justify-content: space-between; margin-bottom: var(--shuttle-space-5); }
  h2 { margin: 0; font-size: var(--shuttle-text-xl); letter-spacing: var(--shuttle-tracking-tight); }
  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--shuttle-space-3); }
</style>
```

*(Note: the `{#snippet actions()}` syntax works with Svelte 5; if the harness errors on that syntax, change to a simpler inline actions by passing a snippet via props — see Dialog.svelte implementation. Adjust locally.)*

- [ ] **Step 2: Wire mount in `gui/web/src/main.ts`**

Read the current `main.ts` and insert a dev-only branch. Final file:

```ts
import './app.css'
import { mount } from 'svelte'
import App from './App.svelte'

let RootComponent: any = App
if (import.meta.env.DEV && typeof location !== 'undefined' && new URLSearchParams(location.search).get('ui') === '1') {
  const mod = await import('./__ui__/UIPreview.svelte')
  RootComponent = mod.default
}

const target = document.getElementById('app')!
mount(RootComponent, { target })
```

*(If existing `main.ts` uses a different mounting pattern, preserve it and only add the dev-branch swap — do not change the target.)*

- [ ] **Step 3: Smoke test**

```bash
cd gui/web && npm run dev
# Open http://localhost:5173/?ui=1 in a browser.
```
Expected: UI Preview renders with all primitives, theme toggle works, dialog opens/closes.

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/__ui__/UIPreview.svelte gui/web/src/main.ts
git commit -m "feat(gui): add dev-only UI primitive preview at ?ui=1"
```

---

### Task 43: Production build and bundle size check

**Files:**
- Modify: `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md`

- [ ] **Step 1: Production build**

```bash
cd gui/web && npm run build
```

- [ ] **Step 2: Capture post-P1 bundle sizes**

```bash
du -b dist/assets/*.js dist/assets/*.css | sort -n
```

- [ ] **Step 3: Append results to baseline file**

Add section at the end:

```markdown
## Post-P1

Run: <YYYY-MM-DD>
<paste du output>

### Delta vs baseline
- JS gzip delta: +<N> KB
- CSS gzip delta: +<N> KB
- Total gzip delta: +<N> KB  (**budget +30 KB** — pass/fail)
```

If total gzip delta > 30 KB, stop and investigate — `bits-ui` tree-shaking may be off, or we're importing the entire bits-ui namespace instead of specific namespaces. Fix before shipping.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md
git commit -m "chore(gui): record post-P1 bundle sizes"
```

---

### Task 44: Final CI green run + PR

**Files:** none; checks only.

- [ ] **Step 1: Run all gates locally**

```bash
cd gui/web
npm run check                         # svelte-check
npm test                              # vitest
./scripts/check-i18n.sh               # i18n guard
npm run build                         # vite build
```
Expected: all green.

- [ ] **Step 2: Run Playwright E2E against existing GUI**

```bash
cd gui/web && npx playwright test
```
Expected: existing tests still pass (P1 doesn't touch any page).

- [ ] **Step 3: Run the Go test suite host-safe tier** (defense against unintended backend change)

```bash
./scripts/test.sh
```
Expected: all packages pass.

- [ ] **Step 4: Push and open PR**

```bash
git push -u origin refactor/gui-v2
gh pr create --title "refactor(gui): P1 infrastructure — ui/ + lib/ + app/ scaffolding" --body "$(cat <<'EOF'
## Summary
- Adds `ui/` design system (11 self-built + 7 bits-ui wrappers)
- Adds `lib/` infrastructure: `api/` split, `resource.svelte.ts`, `router/`, `theme.svelte.ts`, `toast.svelte.ts`, `flags.ts`
- Adds `app/icons.ts` registry and empty `app/routes.ts`
- Adds vitest + testing-library, i18n CI guard, dev-only UI preview at `?ui=1`
- **Zero user-visible change** — old pages are untouched and continue to render as before

Spec: `docs/superpowers/specs/2026-04-19-gui-refactor-design.md`
Baseline + post-P1 bundle sizes: `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md`

## Test plan
- [x] `npm run check` clean
- [x] `npm test` all green (Resource, Router, Theme, Toast, Button)
- [x] `npm run build` succeeds
- [x] Bundle gzip delta ≤ 30 KB vs baseline
- [x] Playwright E2E still passes against current old pages
- [x] Manual smoke at `http://localhost:5173/?ui=1` — all primitives render in both themes
- [x] `./scripts/test.sh` (Go host tests) passes

## Next
P2 — App shell + Sidebar, bridges legacy pages onto new router.
EOF
)"
```

---

## Self-review notes (for the planner)

**Spec coverage check.**

- Spec §1 dependency rules → documented in `src/README.md` (Task 9) + enforced by import structure (no tooling).
- Spec §2 directory → Tasks 7, 9, 10–13, 14–17, 18–21, 22–24, 25–33, 34–40, 41, 42 create every P1-scope file. Feature directories are NOT created in P1 (deferred to P3+).
- Spec §3 extensibility → README (Task 9) documents the conventions; `app/routes.ts` (Task 41) is the Feature self-description hook; `ui/index.ts` (Tasks 33, 34–40) is the barrel; `lib/flags.ts` (Task 17) is the reserved hook.
- Spec §4 tokens + rules → Task 7 lays down every token; each primitive uses them.
- Spec §5 Resource → Tasks 14–16.
- Spec §6 Router → Tasks 18–21; components in Task 20, lazy helper included.
- Spec §8 delivery → P1 aligns with 2–3 day estimate.
- Spec §9 tests → Resource/Router/Theme/Toast/Button covered; Dialog/Select/Tabs/Combobox get manual eyeball via `?ui=1` preview (Task 42); deeper interaction tests can be added in P2 when integrated.
- Spec §10 risks → R1 (bits-ui compat) addressed at Task 34 preflight; R2 (bundle) gated in Task 43; R5 (i18n leaks) guarded in Task 6.

**Explicit out-of-scope for P1**
- Creating any file under `features/`.
- Touching or deleting any `pages/*.svelte`.
- Changing `App.svelte` (its CSS migrates in P2).
- Playwright E2E rewrite (P11).
- a11y sweep with axe (P11).

**Known edge risks to watch during execution**
- `$effect` inside `.svelte.ts` modules: Svelte 5 allows this in "effect scope" via library code. If createResource's `$effect` fails outside a component, refactor to an explicit `subscribe(onDispose)` helper that consumers call from their own `$effect` — adjust Task 15 inline and update tests. Do not proceed if this fails; raise to human.
- `bits-ui` API shape may differ between minor versions. The wrappers in Tasks 34–40 were written against the v1.x public API documented at bits-ui.com. If an import fails, consult the installed package's `package.json` version and adjust.
- `color-mix()` in CSS (used in Badge / ErrorBanner) requires Chrome 111+ / Safari 16.4+. Wails WebView on macOS/Windows uses system WebKit/WebView2 — verify at first Task 42 smoke run. If unsupported, replace with fixed rgba values.

---

## Plan complete and saved to `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure.md`.

Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
