# Shuttle Roadmap

## Phase 1: Core Stability & Testing (Current)

### P0 — Critical
- [x] **End-to-end proxy test in sandbox**: Verify full data flow (client → SOCKS5 → router → H3 → server → httpbin) with automated assertions
- [x] **TUN mode testing**: Bring up TUN device in Docker sandbox, route traffic, verify transparent proxying
- [x] **Reality transport sandbox test**: Verify Reality/TLS+Noise transport connects and relays correctly
- [x] **CDN transport sandbox test**: Verify CDN/HTTP2 transport works through sandbox (server on :8443, client switches via API, SOCKS5 proxy verification)

### P1 — Important
- [x] **Config hot-reload test**: Verify `Engine.Reload()` picks up config changes without restart
- [x] **Congestion control switching**: Test adaptive CC switching between BBR/Brutal under simulated packet loss
- [x] **GeoData auto-update persistence**: Persist `last_update` timestamp to disk so Settings UI shows correct state after restart
- [x] **Connection pool management**: Test multiplexed stream limits, reconnection, and graceful degradation

## Phase 2: Routing & Intelligence

### Smart Routing
- [x] **Per-app routing UI**: Settings page with mode selector (allow/deny), app list, process picker modal
- [x] **Rule-based routing UI**: Users can add/edit/delete domain/IP/process/geosite rules in Routing page
- [x] **GeoSite category picker**: Autocomplete datalist from `GET /api/geosite/categories`, `Categories()` method on GeoSiteDB
- [x] **DNS routing integration**: Split DNS with domestic/remote servers, cache, prefetch, anti-pollution
- [x] **PAC file generation**: `GeneratePAC()` generates PAC from routing rules, served via `GET /api/pac`

### GeoData Enhancements
- [x] **Custom rule lists**: Import/export JSON rules, merge with existing, routing templates (bypass-cn, proxy-all, etc.)
- [x] **Rule conflict detection**: `DetectConflicts()` warns on domain conflicts, exposed via `GET /api/routing/conflicts`
- [x] **GeoData source selection**: Presets (loyalsoldier, v2fly, custom) via `GET/POST /api/geodata/sources`

## Phase 3: Transport Hardening

- [x] **Transport fallback chain**: `dialFallback()` tries each transport in order, graceful migration via Migrator
- [x] **Connection quality metrics**: RTT, packet loss, availability exposed via `/api/status` per transport/path
- [x] **WebRTC DataChannel transport**: Full implementation — dual signaling (WS trickle ICE + HTTP), yamux mux, auto-reconnect, stats
- [x] **QUIC 0-RTT resumption**: `Allow0RTT: true` on H3 client/server, tested in quicfork integration tests
- [x] **Multi-path transport**: MultipathPool with 3 schedulers (weighted-latency, min-latency, load-balance), health checks, path metrics
- [x] **Bandwidth/throughput metrics**: Per-path `BytesSent`/`BytesReceived` tracked in `trackedStream`, exposed via `PathInfo` and `/api/status`

## Phase 4: Mesh VPN

- [x] **Hub relay stability**: Signaling hub with peer registration, message routing, broadcast, graceful disconnect handling
- [x] **P2P hole punching reliability**: Full ICE (host/srflx/relay/UPnP), multi-protocol NAT traversal (UPnP/NAT-PMP/PCP), ICE restart on quality degradation, trickle ICE
- [x] **Mesh topology visualization**: Canvas-based real-time topology with P2P/relay lines, RTT tooltips, state coloring
- [x] **Mesh DNS**: mDNS peer discovery (RFC 6762), service advertisement, TTL-based expiry, metadata exchange
- [x] **Advanced split tunnel**: `SplitRoute` config with per-subnet policies (mesh/direct/proxy), `RouteMesh()` method, 3 tests

## Phase 5: Platform & Distribution

### Desktop
- [x] **macOS .app bundle**: Info.plist, create-app.sh, URI handler, signing/notarize instructions
- [x] **Windows installer**: NSIS script with PATH, URI handler, shortcuts, uninstaller
- [x] **Linux packages**: nfpm.yaml (deb/rpm/apk), systemd services, postinstall/preremove scripts

### Mobile
- [x] **Android client**: Full VPN service with ShuttleVpnService, Gradle build, UI controls
- [x] **iOS client**: Network Extension VPN with PacketTunnelProvider, WebKit UI

### Server
- [x] **Admin web dashboard**: Embedded single-page dashboard at `/`, login, status cards, user CRUD, config reload
- [x] **Multi-user auth**: `UserStore` with add/remove/toggle, per-user traffic tracking, `GET/POST/PUT/DELETE /api/users`, 8 tests
- [x] **Server clustering**: Multiple server instances with load balancing

## Phase 6: Polish & UX

- [x] **Speed test integration**: SpeedChart.svelte with latency testing, upload/download graphs
- [x] **Traffic statistics**: Real-time speed charts via WebSocket, 5-min history, admin API metrics
- [x] **Notification system**: Toast component + browser notification API for connection state changes
- [x] **Theme support**: CSS variables with `data-theme` attribute, dark/light toggle in Settings, localStorage persistence
- [x] **Accessibility**: ARIA tablist/tab/tabpanel roles, arrow key navigation, `:focus-visible` outlines
- [x] **CLI completions**: `shuttle completion <bash|zsh|fish>` and `shuttled completion <bash|zsh|fish>`

## Phase 7: Security & Infrastructure

- [x] **Admin API rate limiting**: Token bucket rate limiter (1 req/sec, burst 5), IP-based, integrated into auth middleware, 6 tests
- [x] **Engine unit tests**: 16 tests covering state machine, events, config, validation, concurrent subscribe, streamConn
- [x] **TLS cert auto-renewal**: CertWatcher monitors expiry, auto-regenerates, callback on renewal, 7 tests
- [x] **Subscription auto-refresh**: Background goroutine with configurable interval, StartAutoRefresh/StopAutoRefresh, 3 tests
- [x] **Connection log persistence**: Ring buffer + JSONL file storage, GET /api/connections/history, 6 tests
- [x] **User quota enforcement**: QuotaExceeded/TotalBytes methods, TOKEN:target stream auth, countingReadWriter, 5 tests

## Phase 8: Production Readiness

- [x] **Server graceful shutdown**: Two-phase shutdown (drain → force), WaitGroup tracking, admin server Shutdown(), configurable DrainTimeout
- [x] **pprof debug endpoints**: Standalone pprof server (127.0.0.1:6060), gated by `debug.pprof_enabled` config
- [x] **Structured JSON logging**: `log.format: "json"` config, `logutil.NewLogger()` helper, applied to both server and client, 4 tests
- [x] **Release checksums**: SHA256 checksums.txt in GitHub release, SECURITY.md with verification instructions
- [x] **Config schema versioning**: Version field on configs, migration framework with ordered migrations, future-version rejection, 5 tests
- [x] **Prometheus metrics export**: Lightweight text exposition at GET /metrics (behind auth), server + per-user metrics, 3 tests
- [x] **Server audit log**: Ring buffer + JSONL file sink, per-stream audit entries, GET /api/audit endpoint, 5 tests
- [x] **Client retry with backoff**: Exponential backoff with jitter in proxy dialer, configurable attempts/backoff, 6 tests

## Phase 9: Anti-Censorship & Hardening

### Transport
- [x] **CDN server transport**: HTTP/2 ServerTransport with HMAC auth, yamux mux, bidirectional streaming, auto-cert, 5 tests
- [x] **Domain fronting**: FrontDomain config for TLS SNI manipulation in H2 and gRPC CDN clients, 4 tests

### DNS Security
- [x] **DNS leak prevention**: LeakPrevention mode blocks plaintext fallback, domestic DoH support, EDNS Client Subnet stripping, 4 tests

### Anti-DPI
- [x] **Traffic shaping**: Shaper wraps io.ReadWriter with random chunk sizes and inter-packet delays, configurable via ObfsConfig, 5 tests

### Server Hardening
- [x] **IP reputation system**: Auth failure tracking, escalating bans (1m→5m→30m→24h), success resets, cleanup, 7 tests
- [x] **Buffer pool relay**: io.CopyBuffer with pooled buffers in server relay (eliminates per-stream 32KB allocations)

### Client Resilience
- [x] **Network change detection**: Polling-based netmon.Monitor detects interface changes, fires callbacks, integrated into Engine with EventNetworkChange, 5 tests

### Test Coverage
- [x] **obfs/padding tests**: Pad/Unpad round-trip, min size, randomness, invalid frames, ReadWriteFrame, 5 tests
- [x] **internal/pool tests**: Get/Put, pool sizes, reuse, 3 tests
- [x] **server/cover tests**: Default handler HTML, static file serving, 2 tests
- [x] **transport/auth tests**: HMAC generate/verify, wrong password, tampered payload, 3 tests

## Phase 10: Integration & Wiring

- [x] **Plugin chain integration**: ConnTracker + Metrics + Logger wired into proxy pipeline via `wrapDialer()`, metricsConn byte tracking
- [x] **Stats/connlog wiring**: Engine event subscription → stats recording + connection logging in shuttle-gui
- [x] **Unified API handler**: Single shared handler with all options (subscriptions, connlog) in shuttle-gui
- [x] **GUI API tests**: 22+ httptest-based tests covering status, config, connect/disconnect, subscriptions, connections history, backup, geodata
- [x] **Proxy unit tests**: 4 additional tests (UnsupportedVersion, UnsupportedCommand, Config defaults for SOCKS5/HTTP)
- [x] **QoS classifier wiring**: QoSClassifier field on SOCKS5Server/HTTPServer, wired in engine.go
- [x] **Connection log rotation**: `CleanOldFiles(keepDays int)` method on connlog.Storage
- [x] **NewServerWithHandler**: Extracted GUI API server factory for handler reuse

---

## Recently Completed

- [x] GeoData `last_update` persistence to `status.json` (survives restart)
- [x] Config hot-reload sandbox test (transport switch via `Engine.Reload()`)
- [x] Adaptive CC unit tests (11 tests: BBR↔Brutal switching, cooldown, RTT trend, loss window)
- [x] Connection pool unit tests (19 tests: multipath, scheduler, stream tracking, fallback, double-close)
- [x] Fixed unsafe test files (`test/e2e_test.go`, `test/webrtc_test.go`) missing `//go:build sandbox` tag
- [x] E2E proxy sandbox tests (SOCKS5/HTTP, sequential, concurrent, multi-client)
- [x] Reality transport sandbox test (transport switch, proxy verification, auto-cert generation)
- [x] TUN mode sandbox test (enable/disable via API, probe verification)
- [x] Playwright browser-level API tests (status, config, probe, disconnect/reconnect, Reality switch)
- [x] Reality server auto-generates self-signed cert when CertFile/KeyFile not provided
- [x] GeoIP/GeoSite data management (download, cache, auto-update, engine integration)
- [x] Smart routing with community domain/CIDR lists (Loyalsoldier + chnroutes2)
- [x] Server admin API with auth
- [x] Self-signed TLS cert generation for bootstrap
- [x] shuttle:// URI import for one-click setup
- [x] Security hardening (race fixes, bounded allocs, error propagation)
- [x] Sandbox test infrastructure (sysproxy, autostart, P2P)
- [x] GUI geodata settings section with i18n
- [x] Server duplicate add/cascade delete bug fixes
- [x] Subscription management (SIP008, Base64, Shuttle JSON)
- [x] Onboarding flow with QR code support
- [x] PAC file generator (router/pac.go) with domain trie walk, CIDR→isInNet, 6 tests
- [x] Rule conflict detector (router/conflict.go) with geosite expansion, 7 tests
- [x] GeoData source presets (router/geodata/sources.go) — loyalsoldier, v2fly, custom, 5 tests
- [x] API endpoints: GET /api/pac, GET /api/routing/conflicts, GET/POST /api/geodata/sources
- [x] GeoSite category autocomplete: `Categories()` method + `GET /api/geosite/categories` + datalist in Routing.svelte
- [x] Per-path bandwidth tracking: `BytesSent`/`BytesReceived` in PathMetrics, exposed in PathInfo
- [x] Dark/light theme: CSS variables, `data-theme` attribute, all 8 pages/components converted
- [x] CLI completions: `shuttle completion` and `shuttled completion` for bash/zsh/fish
- [x] Per-app routing UI: Settings page with allow/deny mode, app list, process picker
- [x] Multi-user auth: UserStore (add/remove/toggle/traffic), 4 REST endpoints, 8 tests
- [x] Admin web dashboard: embedded HTML at `/`, login, status cards, user management, config reload
- [x] Advanced split tunnel: SplitRoute config, RouteMesh() method, IsMeshDestination respects policies, 3 tests
- [x] Accessibility: ARIA tablist roles, arrow key tab navigation, focus-visible outlines
- [x] Server clustering: `ClusterManager` with peer health checks, load shedding, least-loaded forwarding, cluster API endpoints, 11 tests
