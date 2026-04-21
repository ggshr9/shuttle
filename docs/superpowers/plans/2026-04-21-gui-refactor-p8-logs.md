# P8 Logs — implementation plan

Spec: §7.7 — three columns (filter / list / detail) + monospace + level color
+ top fixed search + virtual scroll.

## File structure

```
features/logs/
  index.ts           route export
  store.svelte.ts    ring buffer + filter state + WS subs (singleton)
  types.ts           LogEntry, ConnDetails
  LogsPage.svelte    3-column grid composition
  LogFilters.svelte  left column: level chips + protocol/action + show-conn
  LogList.svelte     middle column: virtual scroll + monospace rows
  LogDetail.svelte   right column: selected row detail or hint
```

## Data model

Unified entry:
```ts
interface LogEntry {
  id: string
  time: number          // ms epoch
  level: 'debug'|'info'|'warn'|'error'
  msg: string
  kind: 'log' | 'conn-open' | 'conn-close'
  details?: ConnDetails // present for conn-*
}
```

Store exposes:
- `entries: LogEntry[]` (ring of 500)
- `filters: { levels: Set<Level>, text: string, protocol, action, showConn }`
- `filtered: LogEntry[]` ($derived)
- `selectedId: string | null`
- `activeConnectionCount: number` ($derived — count of open conns with no close)
- `subscribe()` / `unsubscribe()` — attach/detach WS streams

Virtual scroll: pure JS, fixed row height 22px, render only visible window
± 10 overscan, based on container scrollTop and clientHeight. No dep.

## Tasks

1. Types + store — ring buffer, filter derives, WS wiring.
2. LogFilters — left rail.
3. LogList — virtual scroll list, click to select, auto-scroll toggle.
4. LogDetail — right panel, shows kind-specific fields.
5. LogsPage — grid composition, top search+controls, wires subscribe on mount.
6. index.ts — route export.
7. Wire into app/routes.ts; delete pages/Logs.svelte.
8. Build + svelte-check; record post-P8 baseline.
9. Commit.

## Bundle budget

Current pre-P8 index gzip: 42.54 KB. No new dep (virtual scroll hand-rolled).
Expect lazy `LogsPage-*.js` ~ 4 KB gzip (current is 3.56 KB).
