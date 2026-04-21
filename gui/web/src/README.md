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

## Svelte 5 gotchas (things review has caught)

These are Svelte 5 runes quirks that several reviews flagged as P2 bugs. Read
once, save time later.

1. **Reactive collections need `svelte/reactivity`, not `$state<Set>`**
   ```ts
   // ❌ mutation NOT tracked — .add / .delete won't re-render
   const expanded = $state<Set<string>>(new Set())
   expanded.add(id)

   // ✅ SvelteSet tracks mutations
   import { SvelteSet } from 'svelte/reactivity'
   const expanded = new SvelteSet<string>()
   expanded.add(id)
   ```
   Same applies to Map → use `SvelteMap`. A plain `$state({...})` object IS
   tracked for property access; only collection mutations need the helpers.

2. **Runes in non-component files need a `.svelte.ts` extension**
   Files that call `$state` / `$derived` / `$effect` at top level must be named
   `foo.svelte.ts` (not `foo.ts`). Imports go without the `.ts` suffix
   (`import { x } from './foo.svelte'`).

3. **Don't call `$effect` from a factory function if callers may use it
   outside a component context** (e.g. tests, module init). We keep
   `createResource` callable from tests by having consumers drive disposal
   explicitly rather than registering `$effect` inside the factory.

4. **Module-level `$state` + multiple runtime exports + sibling `.ts` file
   with the same basename breaks exports.** Hit this in P1 with
   `lib/toast.svelte.ts` coexisting with the legacy `lib/toast.ts` — the
   build compiled the runes module as a Svelte *component* and collapsed
   named exports to `default`. Workaround: pick a unique basename
   (we renamed to `lib/toaster.svelte.ts`).

5. **Pure functions stay pure — don't write state from `$derived`**
   ```ts
   // ❌ violates state_unsafe_mutation
   const loader = $derived.by(() => {
     for (const r of routes) {
       if (matches(r.path)) { state.params = r.params; return r.component }  // writes inside derived
     }
   })

   // ✅ pure match + sync params via $effect if needed
   const result = $derived.by(() => findMatch(routes, state.path))
   $effect(() => { if (result) state.params = result.params })
   ```

6. **Checkbox `indeterminate` is a DOM property, not an HTML attribute.**
   Setting it declaratively via Svelte does nothing. Use `bind:this` + an
   action (or a one-line `$effect` that sets `el.indeterminate = ...`).

## Router gotchas

1. **Every URL pattern must be registered explicitly** — `RouterOutlet` does
   strict-length matchPath, no prefix / wildcard fallback. A detail route
   like `/groups/:tag` must be a separate `AppRoute` entry (can share a
   lazy component with the index route; set `nav.hidden: true` so it
   doesn't show up in the sidebar).

2. **`matchPath` is pure; `matches()` reads live state** — both safe inside
   `$derived`. `useParams(pattern)` returns params without mutation.

## Resource + Stream gotchas

1. **Same-key means same instance** — `createResource('x', fetcher1)` +
   `createResource('x', fetcher2)` keep the *first* fetcher; second caller
   subscribes to the existing entry. To swap behavior, use a different key.

2. **`createStream` dedupes by key with refcount close** — Same-key callers
   share one WebSocket; each caller must `.close()` to decrement. The
   socket drops only when the last subscriber closes.

3. **Polling keeps running until the process exits** (P1 design choice). No
   auto-dispose on component unmount. Call `invalidate(key)` or let the
   registry grow; it's bounded by the finite set of resources the app
   registers.

## Build gotchas

1. **`gui/web/dist/.gitkeep` must exist before `go build ./gui`** — the
   `//go:embed all:web/dist` pattern requires at least one matching file.
   Every `npm run build` wipes the placeholder (vite `emptyOutDir: true`),
   so `scripts/keep-embed-placeholder.mjs` runs as a post-build hook to
   restore it. Don't delete the script or the `.gitkeep`.

2. **Release workflow runs `npm run build` before `go build`** — so
   production binaries always embed fresh assets. A local developer who
   builds `cmd/shuttle-gui` without running `npm run build` first will
   *not* get a silent blank window: `cmd/shuttle-gui/main.go` calls
   `fs.Stat(webFS, "index.html")` at startup and `log.Fatalf`s with a
   pointer to the build step if the bundle is missing. CLAUDE.md
   documents the full local sequence.

## Settings shell (multi-URL pattern)

`features/settings/` is the one slice that multiplexes several URLs into
a single visual frame:

- `SettingsPage` is registered as a `/settings` route with 10 `children`
  in `features/settings/index.ts`; every sub-path maps to the same
  component because persistent draft state lives in
  `features/settings/config.svelte.ts`, not in the component.
- The component dispatches on the last URL segment into one of 10
  sub-page components (`sub/<Name>.svelte`).
- `UnsavedBar.svelte` is sticky at the top of the content area and only
  renders when `settings.isDirty`.

When another feature needs sub-routing, copy this shape rather than
registering individual top-level routes.

## Related

- Architecture spec: `docs/superpowers/specs/2026-04-19-gui-refactor-design.md`
- Phase plans: `docs/superpowers/plans/2026-04-*-gui-refactor-p*.md`
- Post-phase bundle + error baselines: `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure-baseline.md`
