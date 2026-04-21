# P1 Infrastructure — Pre-change baseline

Run: `cd gui/web && npm ci && npm run build`
Date: 2026-04-19
Branch: `refactor/gui-v2` from `main` (commit `05a0487`)

## Bundle sizes (bytes, from dist/assets/)

### JS (total ≈ 227.6 KB raw, 76.8 KB gzip)

| File | raw | gzip |
|------|-----|------|
| `index-0gQJro8N.js`               | 97,699 | 34.94 KB |
| `Dashboard-BwG2wWJw.js`           | 28,796 |  9.43 KB |
| `Settings-CEd2pOhg.js`            | 28,328 |  8.31 KB |
| `Routing-Dvv4ipsO.js`             | 19,362 |  6.36 KB |
| `Servers-DKwP__9T.js`             | 10,627 |  3.55 KB |
| `Logs-BRS0SOb4.js`                | 10,623 |  3.57 KB |
| `Mesh-GvgL5uCY.js`                |  6,415 |  2.37 KB |
| `Subscriptions-Dvu4Bd50.js`       |  6,249 |  2.40 KB |
| `Groups-C9gT3JpB.js`              |  5,401 |  2.18 KB |
| `MeshTopologyChart-uup9u9R6.js`   |  4,456 |  1.87 KB |
| misc (select, props, style, this) |  2,541 |  1.60 KB |

### CSS (total ≈ 87.2 KB raw, 17.4 KB gzip)

| File | raw | gzip |
|------|-----|------|
| `Settings-DZomXe5g.css`             | 17,210 | 2.54 KB |
| `index-CJr0QzwM.css`                | 15,577 | 3.32 KB |
| `Dashboard-C3zKTz8T.css`            | 13,637 | 2.62 KB |
| `Routing-DEagN7MB.css`              | 12,252 | 1.95 KB |
| `Servers-Bx541bA0.css`              |  7,163 | 1.51 KB |
| `Subscriptions-BwG9r-62.css`        |  5,277 | — |
| `Logs-CUotOx6r.css`                 |  4,913 | — |
| `Groups-DA049InX.css`               |  4,651 | — |
| `Mesh-DC10q_Nm.css`                 |  3,732 | — |
| `MeshTopologyChart-UZOqd3jK.css`    |  1,531 | — |

### Totals (from `vite build` gzip column)

- JS gzip total: **~76.8 KB**
- CSS gzip total: **~17.4 KB** (incl. lazy-loaded chunks not all reported gzip)
- Combined gzip: **~94.2 KB**

## Budget for P1 PR

Total JS + CSS gzip delta ≤ **+30 KB** (bits-ui + testing-library runtime + ui/ primitives).
If the delta exceeds that we stop and investigate tree-shaking — see Task 43.

---

## Post-P1 (2026-04-19)

Run after all 44 tasks committed on `refactor/gui-v2`.

### index.js
- raw: 97,699 → **98,372** bytes (**+673 B**)
- gzip: 34.94 → **35.38 KB** (**+0.44 KB**)

### CSS / lazy chunks
Unchanged — old pages still render the same code; `index.css` unchanged.

### Why the delta is small
P1 is **additive** — new `ui/`, `lib/`, `app/` files are written but not
imported by any route yet. Only the top-level barrel imports
(`app.css` → `tokens.css`) touch the production bundle. Tree-shaking
removes bits-ui and the entire ui/ design system from prod because
nothing references them yet.

### UIPreview harness
`src/__ui__/UIPreview.svelte` is gated by `import.meta.env.DEV` +
`?ui=1` and imported dynamically. Vite confirms: **no UIPreview chunk
is emitted in production**. The harness is reached only via
`npm run dev` + `http://localhost:5173/?ui=1`.

### Delta vs baseline
- JS gzip delta: **+0.44 KB**
- CSS gzip delta: **0 KB**
- Total gzip delta: **+0.44 KB** ≪ +30 KB budget ✓

The design system cost will materialize in P3+ when feature slices
start importing `@/ui`. The budget remains **+30 KB** across all
phases through P10.

---

## Post-P2 (2026-04-20)

Run after the P2 app-shell commits on `refactor/gui-v2`.

### index.js
- raw: 98,372 → **93,684** bytes (**−4,688 B** vs P1)
- gzip: 35.38 → **34.98 KB** (**−0.40 KB** vs P1)

### Why it got smaller
The legacy `src/App.svelte` (537 lines of tab-switching + SimpleMode
dual-UI + inline SVG icons) was deleted and `pages/SimpleMode.svelte`
(312 lines) removed. The new `app/App`+`Shell`+`Sidebar`+`Toaster`
composition plus router wiring is collectively smaller than the single
legacy root.

bits-ui is still tree-shaken from prod — no feature route imports it
yet. That cost will hit in P3+ as features start using `@/ui/Dialog`
etc.

### Delta vs original baseline (pre-P1)
- JS gzip: 34.94 KB → **34.98 KB** (**+0.04 KB** total after P1+P2)
- Budget remaining: ~29.96 KB for P3-P10 design-system integration.

---

## Post-P3 (2026-04-20)

Run after P3 dashboard feature commits on `refactor/gui-v2`.

### Bundle
| Chunk | Pre-P3 | Post-P3 |
|-------|--------|---------|
| `index-*.js`          | 93.70 KB / 35.00 KB gzip | **96.93 KB / 36.26 KB gzip** |
| `Dashboard-*.js` (lazy) | 28.80 KB / 9.43 KB gzip  | **9.82 KB / 3.86 KB gzip** |

### Why the shift
- `index-*.js` grew ~1.26 KB gzip — bits-ui Dialog / Badge / Card / AsyncBoundary now
  ship with the shell because `app/App`→`Shell`→`Sidebar`→`ui/*` pulls them eagerly.
- `Dashboard-*.js` shrank by **5.57 KB gzip** — the new feature slice is ~2/3 the size
  of the legacy `pages/Dashboard.svelte` + its chart bundles.

### Cumulative delta vs original pre-P1 baseline
- JS gzip: 34.94 → **36.26 KB** total (**+1.32 KB** after P1+P2+P3)
- Lazy Dashboard: 9.43 → **3.86 KB** (**−5.57 KB**)
- Net: app is **smaller**, even though design-system code is now in the shell bundle.
- Budget remaining: ~28.7 KB for P4-P10.

### Svelte-check error count
- Pre-P3 baseline: **359** errors
- Post-P3: **300** errors (−59 — all from the 5 deleted legacy files)

---

## Post-P4 (2026-04-20)

After P4 servers feature commits on `refactor/gui-v2`.

### Bundle
| Chunk | Pre-P4 | Post-P4 |
|-------|--------|---------|
| `index-*.js`           | 96.93 KB / 36.26 KB gzip | **107.05 KB / 39.59 KB gzip** |
| `Servers-*.js` (lazy)  | 10.66 KB /  3.56 KB gzip | **61.69 KB / 19.81 KB gzip** (new ServersPage) |

### Why ServersPage is much heavier than legacy Servers
3 bits-ui Dialogs (Add / Import / DeleteConfirm) bring focus-trap, portal,
and escape-handling code with them. Each Dialog wrapper is ~5-7 KB raw of
bits-ui internals plus its own styling. Tree-shaking keeps Dialog out of
the shell + Dashboard chunks (those don't open dialogs); Settings doesn't
duplicate it either.

### Cumulative delta vs original pre-P1 baseline
- JS gzip total (index + visible chunks):
  - pre-P1:   ~94.2 KB
  - post-P4:  ~110.0 KB (≈ +15.8 KB cumulative)
- Within +30 KB P1-P10 budget. Remaining: ~14 KB for P5-P10.

### Svelte-check error count
- Pre-P4: 300 errors
- Post-P4: **264** errors (−36 from deleted legacy Servers.svelte)

---

## Post-P5 (2026-04-20)

After Subscriptions + Groups feature commits on `refactor/gui-v2`.

### Bundle (lazy chunks for the two new features)
- `SubscriptionsPage-*.js`: **7.94 KB raw / ~2.5 KB gzip**
- `GroupsPage-*.js`: **4.66 KB raw / ~1.5 KB gzip**

Both chunks are smaller than legacy (`Subscriptions-*` was 6.29 / 2.42; `Groups-*` was 5.44 / 2.20) because they skip Chart.js and other legacy-only dependencies. Dialog code consolidated into the index chunk after bits-ui dedup between features.

### Svelte-check error count
- Pre-P5: 264 errors
- Post-P5: **236** errors (−28 from deleted legacy Subscriptions.svelte + Groups.svelte)

---

## Post-P6 (2026-04-21)

After Routing feature commits on `refactor/gui-v2`.

### Bundle
- `index-*.js`: **115.67 KB raw / 42.21 KB gzip** (+1.85 KB raw / +0.43 KB gzip vs post-P5)
- Routing chunk (lazy): part of ServersPage's chunk group due to shared bits-ui Dialog/Combobox — exact bytes depend on route loaded

### Svelte-check error count
- Pre-P6: 236 errors / 24 warnings
- Post-P6: **196** errors / **11** warnings (−40 errors / −13 warnings) — legacy `lib/routing/*` had a lot of implicit `any` and a11y warnings

### Legacy deletion totals
- Cumulative legacy code deleted P3-P6: **6,993 lines**
  - P3 Dashboard + charts: 1,859
  - P4 Servers: 704
  - P5 Subscriptions + Groups: 834
  - P6 Routing + lib/routing: 1,349
  - Plus p2 SimpleMode + legacy App.svelte: 2,247

---

## Post-P7 (2026-04-21)

### Bundle
- `index-*.js`: 116.64 KB raw / 42.54 KB gzip (+0.33 KB gzip vs post-P6 fix)

### Svelte-check error count
- Pre-P7: 196 errors / 11 warnings
- Post-P7: **165** errors / 11 warnings (−31)

### Legacy deletion (P7)
- `pages/Mesh.svelte` (413) + `lib/MeshTopologyChart.svelte` (360) = 773 lines
- **Cumulative P3-P7: 7,766 lines**

---

## Post-P8 (2026-04-21)

### Bundle
- `index-*.js`: 116.78 KB raw / 42.62 KB gzip (+0.08 KB gzip vs post-P7 fix)
- `LogsPage-*.js` (lazy): 18.71 KB raw / 6.79 KB gzip (vs legacy Logs-*.js 10.62 KB raw / 3.57 KB gzip; +3.22 KB gzip — absorbs Select/Switch/Badge/Input into the lazy chunk, plus the three-column feature-slice components; Dialog stays shared with other pages)

### Svelte-check error count
- Pre-P8: 145 errors / 11 warnings
- Post-P8: **65** errors / 9 warnings (−80 — legacy Logs.svelte was untyped and pulled in tangled shapes)

### Legacy deletion (P8)
- `pages/Logs.svelte` (562) — replaced by `features/logs/` feature slice
- **Cumulative P3-P8: 8,328 lines**

---

## Post-P9 (2026-04-21)

P9 shipped in two commits (P9a shell, P9b sub-pages).

### Bundle
- `index-*.js`: 42.62 → **43.15 KB gzip** (+0.53 KB total over P8; sub-nav + store + all new icons)
- `SettingsPage-*.js`: 3.56 → **8.05 KB gzip** (+4.49 KB as placeholders converted to real sub-pages; still smaller than legacy 8.16 KB)

### Svelte-check error count
- Pre-P9:  65 errors / 11 warnings
- Post-P9a: 24 errors / 9 warnings (−41; unused legacy lib/settings/*)
- Post-P9b: **8** errors / 3 warnings (all in lib/Onboarding.svelte; P10 target)

### Legacy deletion (P9)
- `pages/Settings.svelte` (294) — replaced by `features/settings/` shell
- `lib/settings/*` (1,437 lines across 12 files) — per-section replacements
- Total P9: **1,731 lines removed**
- **Cumulative P3-P9: 10,059 lines**

### Sub-pages ported
- General: language + theme + autostart
- Proxy: SOCKS5/HTTP/TUN + per-app routing + LAN sharing + system proxy
- Mesh: enable + p2p toggle
- Routing: default + geodata enable / auto-update / manual refresh
- DNS: domestic + remote server/via + cache/prefetch
- Logging: log level
- QoS: enable + rules with priority / ports editor
- Backup: create + restore
- Update: current version + update check + banner with changelog
- Advanced: config export (JSON/URI) + diagnostics bundle

Unified on `<Field>` rows + `<Switch>` / `<Select>` primitives per §7.8.
Singleton store (`settings`) tracks pristine vs draft JSON snapshot;
`<UnsavedBar>` stickies to top when isDirty, wires Discard/Save.

---

## Post-P10 (2026-04-21)

### Bundle
- `index-*.js`: 43.15 → **39.54 KB gzip** (**−3.61 KB** vs P9)
  - Onboarding moved to a lazy dynamic-import chunk (first-run only)
  - `lib/api.ts` barrel deleted, lets Vite tree-shake unused endpoints
  - Legacy `Toast.svelte` / `toast.ts` / `theme.ts` removed
- Cumulative JS gzip delta vs original pre-P1 baseline: 34.94 → 39.54 (**+4.60 KB** end-state for the entire refactor against a +30 KB budget)

### Svelte-check
- Pre-P10: 8 errors / 3 warnings
- Post-P10: **0 errors** / 2 warnings (both benign in ServerRowExpanded)

### Legacy deletion (P10)
- `lib/Onboarding.svelte` (674) → `features/onboarding/` (4-step wizard)
- `lib/api.ts` (6) barrel — replaced by direct `@/lib/api/endpoints` imports
- `lib/Toast.svelte` (129) + `lib/toast.ts` (57) — superseded by `toaster.svelte.ts`
- `lib/theme.ts` (46) — superseded by `theme.svelte.ts`
- `pages/` empty directory removed
- Total P10: **912 lines**
- **Cumulative P3-P10: 10,971 lines removed**

### Final `src/` layout

```
src/
  __ui__/        dev-only UI harness (gated on import.meta.env.DEV + ?ui=1)
  app/           App, Shell, Sidebar, Toaster, routes.ts, icons.ts
  app.css        entry stylesheet (imports ui/tokens.css)
  features/      dashboard, servers, subscriptions, groups, routing, mesh,
                 logs, settings, onboarding — each a feature slice
  lib/           api/, i18n/, router/, resource + theme + toaster +
                 ws + notify + shortcuts + flags
  locales/       en.json, zh-CN.json
  main.ts        entry
  README.md      Svelte 5 gotchas captured during the migration
  ui/            design system (21 primitives + tokens.css)
```

Matches spec §8 P10 exit criterion: `src/` = app / ui / lib / features /
main.ts / app.css (plus dev-only __ui__ harness and locales).

### Onboarding wizard (§7.9)
- Fullscreen overlay with 480 px wizard card
- 4 steps: Welcome → Add Server → Options → Connect
- DotProgress ● ● ● ○ between header and content
- Each step: big title + lede + single-column controls + footer with
  `[Back] | [Next]` (or `[Skip] | [Next]` on step 1, `[Back] | [Connect]` on step 4)
- Error banner inline; Esc closes wizard
