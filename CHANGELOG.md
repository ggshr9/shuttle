# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed — GUI refactor (branch `refactor/gui-v2`)

End-to-end rewrite of the desktop GUI frontend. Same Go backend, same
REST/WebSocket API contract, same Wails shell — every Svelte page
rebuilt.

- **Architecture**: flat `src/` tree of `app / ui / lib / features /
  main.ts / app.css`. Each feature is a self-contained slice
  (`features/<name>/` with resource store, components, page, route
  export) — no more `pages/` + `lib/settings/` split.
- **Design system**: 21 UI primitives under `ui/` built on bits-ui for
  Dialog / Select / Switch / Combobox / DropdownMenu / Tabs, plus
  custom Badge / Button / Card / Empty / ErrorBanner / Icon / Input /
  Section / Spinner / StatRow / Tooltip / AsyncBoundary. Geist-inspired
  `--shuttle-*` token layer replaces ad-hoc CSS variables.
- **Reactivity**: Svelte 5 runes throughout. Custom `createResource`
  primitive with singleton registry + refcount `createStream` for
  WebSockets, replacing a tangle of one-off `$state` + `onMount`
  polling. Hash-based router (`matchPath` + `useParams(pattern)`) under
  `lib/router/`.
- **Pages rewritten**: Dashboard (single-screen hero + stats + live
  throughput + activity), Servers (dense table + inline expand + batch
  delete + speedtest), Subscriptions, Groups (+ `/groups/:tag`
  detail), Routing (draft/commit editor + rule preview), Mesh
  (topology canvas + peer table), Logs (3-column filter / virtual-
  scroll list / detail panel), Settings (left sub-nav + 10 sub-routes
  + draft store + sticky unsaved-changes bar), Onboarding (4-step
  wizard, dot progress, fullscreen overlay).
- **SimpleMode removed**: legacy dual-UI branch deleted. Dashboard is
  the one-screen default; everything else lives behind the sidebar.
- **Bundle**: JS gzip 34.94 → 39.54 KB (+4.60 KB) for the entire
  refactor including a net-new design system and a second router; well
  under the +30 KB budget. Heavy feature pages (Mesh, Routing,
  Settings, Logs) are lazy-loaded and each <11 KB gzip.
- **Testing**: Playwright specs per feature (shell, dashboard,
  servers, subscriptions, groups, routing, mesh, logs, settings,
  onboarding). Svelte-check: 0 errors on the final branch.
- **Deleted**: 10,971 lines of legacy GUI code across
  `pages/Dashboard.svelte`, `pages/Servers.svelte`,
  `pages/Subscriptions.svelte`, `pages/Groups.svelte`,
  `pages/Routing.svelte`, `pages/Mesh.svelte`, `pages/Logs.svelte`,
  `pages/Settings.svelte`, `pages/SimpleMode.svelte`, legacy
  `App.svelte`, `lib/settings/*` (12 files), `lib/Onboarding.svelte`,
  `lib/MeshTopologyChart.svelte`, `lib/routing/*`, `lib/api.ts`
  barrel, `lib/Toast.svelte`, `lib/toast.ts`, `lib/theme.ts`.

## [0.1.0] - 2026-03-16

### Added
- **Multi-transport proxy**: H3/QUIC, Reality/TLS+Noise, CDN/HTTP2, and WebRTC transports
- **Adaptive congestion control**: BBR, Brutal, and auto-switching based on packet loss & RTT
- **Intelligent routing**: Domain trie matching, GeoIP/GeoSite rules, DNS-over-HTTPS with caching and prefetch
- **Proxy listeners**: SOCKS5, HTTP CONNECT, TUN device with per-app routing
- **Mesh VPN**: Hub-and-spoke relay with P2P NAT traversal via STUN/hole-punching
- **Desktop GUI**: Wails + Svelte SPA with system tray support
- **Server features**: Prometheus metrics, audit logging, admin API, graceful two-phase shutdown
- **Security**: Config encryption at rest, HMAC transport auth, post-quantum KEM support, SSRF prevention
- **Deployment**: Docker one-click deploy, OpenWrt package, multi-platform release builds (Linux/macOS/Windows/FreeBSD)
- **Diagnostics**: Connection tracing, speed test, diagnostics bundle export
- **CI/CD**: GitHub Actions with lint, test, build, coverage, and automated release with SHA256 checksums
