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

## Related

- Architecture spec: `docs/superpowers/specs/2026-04-19-gui-refactor-design.md`
- P1 plan: `docs/superpowers/plans/2026-04-19-gui-refactor-p1-infrastructure.md`
