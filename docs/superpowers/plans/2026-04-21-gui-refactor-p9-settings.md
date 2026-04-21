# P9 Settings — implementation plan

Spec: §7.8 — left sub-nav (not tabs) + independent URL per sub-page +
single-topic pages + Switch/Select primitives + unsaved-changes bar.

## Architecture

```
features/settings/
  index.ts              route exports (10 sub-routes → one SettingsPage shell)
  config.svelte.ts      singleton store: load once, draft vs pristine, dirty, save/discard
  nav.ts                sub-nav metadata (10 entries)
  types.ts              Config type extensions used across sub-pages
  SettingsPage.svelte   shell: sub-nav + unsaved bar + outlet dispatching on sub path
  UnsavedBar.svelte     sticky top bar when isDirty
  sub/
    General.svelte      language / theme / autostart
    Proxy.svelte        SOCKS / HTTP / TUN / LAN sharing / system proxy / per-app
    Mesh.svelte         mesh enable + p2p toggle
    Routing.svelte      default mode + geodata update/sources
    Dns.svelte          DNS servers + cache
    Logging.svelte      log level + destinations
    Qos.svelte          QoS rules
    Backup.svelte       import / export / diagnostics bundle
    Update.svelte       version + update check
    Advanced.svelte     export PAC + per-app picker + misc
```

## Router wiring

The router supports `children` with path composition. Register 10
sub-routes as children of `/settings`; all 10 point to the same
`SettingsPage` component (it internally dispatches on the last path
segment). Only the parent `/settings` carries `nav` metadata so the
main sidebar still shows one entry. `isActive` in Sidebar already does
prefix matching so the Settings item stays highlighted on sub-routes.

Default path `/settings` redirects to `/settings/general` on mount.

## Store design

```ts
class SettingsStore {
  pristine = $state<Config | null>(null)
  draft    = $state<Config | null>(null)
  loading  = $state(true)
  saving   = $state(false)
  message  = $state<{ kind: 'ok' | 'err'; text: string } | null>(null)

  isDirty  = $derived(!deepEqual(this.pristine, this.draft))

  async load(): Promise<void>      { ...getConfig + normalize defaults }
  async save(): Promise<void>      { putConfig(draft); pristine = clone(draft) }
  discard(): void                  { draft = clone(pristine) }
}
```

Sub-pages bind directly to `store.draft.<section>`. A top-level
normalizer ensures required nested objects (`mesh`, `qos`, `dns`,
`routing.geodata`, `proxy.tun.app_list`, …) exist after load so
components can `bind:` without null guards.

## Sub-page structure

Each sub-page is a flat grid of `<Field>` rows. A `Field` component
(label left, control right) provides consistent rhythm. One screen
per page; scroll only if absolutely necessary.

Business logic (autostart toggle, geodata update, version check) stays
on sub-pages that need it via ad-hoc resources/local effects. The
store only owns the config draft.

## Phased delivery

- **P9a** (this commit): shell + store + sub-nav + unsaved bar +
  `General`, `Mesh`, `Logging`, `Advanced`. Other sub-pages stub out
  with "coming next" placeholder so routes don't 404.
- **P9b**: port `Proxy`, `Routing`, `Dns`, `Qos`, `Backup`, `Update`.
- **P9c**: delete legacy `pages/Settings.svelte` +
  `lib/settings/*` + `lib/Onboarding.svelte` (handled in P10).

## Bundle budget

Expect `Settings-*.js` lazy chunk to shrink as it drops Onboarding and
legacy styling; `index-*.js` gains ≤0.5 KB gzip from sub-nav/unsaved-bar.
