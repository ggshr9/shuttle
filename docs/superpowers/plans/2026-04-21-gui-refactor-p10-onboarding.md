# P10 Onboarding + final cleanup

Spec §7.9 Onboarding:
- Fullscreen overlay 4-step wizard
- Each step: big title + description + single-column actions + bottom `[Back] [Next]`
- Dot progress indicator (● ● ○ ○), not a progress bar

Spec §8 P10 also covers final cleanup: delete `pages/*` (empty now),
`lib/Onboarding.svelte`, legacy `lib/Toast.svelte` / `lib/toast.ts` /
`lib/theme.ts` shims, bridge files. End state: `src/` holds only
`app / ui / lib / features / main.ts / app.css`.

## Structure

```
features/onboarding/
  index.ts            exports { Onboarding }
  state.svelte.ts     singleton store: step, method, form data, addedServers, meshAvailable
  Onboarding.svelte   4-step wizard, fullscreen overlay, bottom Back/Next bar
  DotProgress.svelte  ● ● ○ ○ indicator
```

## Wizard steps

1. **Welcome** — brand title + one-liner value prop + three bullet
   features (speed / secure / global). `[Get started]` + `[Skip]` text link.
2. **Add server** — Tabs for subscription / import / manual. Single-column
   form for chosen tab. Error area + `[Back] [Next]`.
3. **Options** — Small preview of added servers, system-proxy toggle,
   mesh toggle (only if import returned mesh_enabled).
   `[Back] [Next]`.
4. **Done / Connect** — Final summary + `[Back] [Connect]`. Connecting
   state with spinner; on success `onComplete()`.

Dot progress on every step except maybe step 1 — include on all 4.

## Cleanup

- Replace `@/lib/api` (barrel) imports with `@/lib/api/endpoints` in
  `app/App.svelte`; delete `lib/api.ts`.
- Delete `lib/Onboarding.svelte`, `lib/Toast.svelte`, `lib/toast.ts`,
  `lib/theme.ts` (superseded by `theme.svelte.ts` + `toaster.svelte.ts`).
- Remove empty `pages/` directory.
- Remove `__ui__/` harness only if not needed (keep — it's dev-only).

## svelte-check target

Post-P10: 0 errors (all remaining 8 were in lib/Onboarding.svelte,
which gets deleted). Warnings allowed to remain (not strict goal).

## Bundle

- Onboarding chunk: new `OnboardingPage-*.js` (lazy) replaces part of
  current top-level `index.js` footprint (legacy Onboarding was
  statically imported by App). Expect index gzip to drop ~1-2 KB.
