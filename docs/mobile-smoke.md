# Mobile smoke test checklist

Run this manually before every release that includes a mobile change. The
CI workflow `build-mobile.yml` only verifies that the Android APK and iOS
xcarchive **compile** — it cannot exercise runtime behavior on a device.
Each tick below is one thing a human with a device has to verify.

If a line fails, file a bug with the device model, OS version, Shuttle
version, and repro steps.

## Build artifacts

- [ ] `npm run build --prefix gui/web` completes without errors
- [ ] `./build/scripts/build-android.sh $VERSION` produces
      `dist/shuttle-android-$VERSION.apk`
- [ ] `./build/scripts/build-ios.sh $VERSION` produces
      `dist/Shuttle-$VERSION.xcarchive` (macOS only)
- [ ] `gomobile bind` ran without warnings about missing symbols

## First run (both platforms)

- [ ] App installs and launches on a real device
- [ ] No blocking crash within 30 seconds of launch
- [ ] The `Now` screen renders: power button visible + "Disconnected" label
- [ ] Bottom tabs show 5 items in order: Now / Servers / Traffic / Mesh / Activity
- [ ] Settings gear in the top-right navigates to `/settings`

## VPN permission flow (the real payoff of Phase 4)

- [ ] Tap power button on a fresh install → **system VPN permission dialog
      appears** (iOS uses the "Shuttle would like to add VPN
      Configurations" sheet; Android shows the "Connection request" dialog)
- [ ] Accept → app state transitions to Connecting → Connected or shows
      a clear error toast if the backend is unreachable
- [ ] Deny → error toast "VPN permission denied"; state returns to
      Disconnected; retry re-prompts correctly

## Servers page

- [ ] `/servers` shows the saved-servers list (or Empty state if new)
- [ ] `[+ Add]` button opens the AddSheet dialog
- [ ] `Manual` tab: address + password + name fields work; Add succeeds
- [ ] `Paste` tab: paste a `shuttle://` URI; Add imports the server
- [ ] `Subscribe` tab: paste a subscription URL; Add registers it
- [ ] `Scan QR` button appears on native (iOS + Android only, hidden on web)
- [ ] Tap `Scan QR` → camera permission prompt → camera view opens
- [ ] Scan a test QR (e.g. a `shuttle://` URI encoded in a phone QR generator)
      → AddSheet flips to Paste tab with the scanned URI populated
- [ ] Source filter chips work (`All` / `Manual` / `Subscriptions` / per-sub)
- [ ] Subscription banner appears when a specific sub is filtered;
      Refresh + Delete work

## Activity page

- [ ] Tap the Activity tab → Overview sub-tab shows throughput sparkline +
      transport breakdown (data may be empty if not connected)
- [ ] Sub-tab switch to `Logs` → virtual-scroll list renders; filter chips
      work (level multi-select, search)
- [ ] Scrolling 500+ log rows stays smooth (no dropped frames)
- [ ] Tap `Share` button on the Logs tab → system share sheet appears
      (iOS: UIActivityViewController; Android: share chooser)
- [ ] Share via "Copy" / "Notes" → log content matches formatter output

## Traffic, Mesh, Settings

- [ ] `/traffic` renders the existing routing editor; rules can be added / saved
- [ ] `/mesh` shows Peers tab by default when mesh is enabled
- [ ] `/mesh?tab=topology` → topology chart loads (first time: spinner;
      subsequent switches: instant). If chunk fails to load → error banner
      with Retry button; Retry kicks another import that succeeds on
      restored network
- [ ] `/settings` → 4 section headers on desktop (Basics / Network /
      Diagnostics / Advanced); horizontal scrollable chip row on mobile
- [ ] Each settings sub-page saves without error

## Cross-cutting

- [ ] Rotate portrait → landscape on both pages: layout does not break
- [ ] System dark mode toggle reflects in the app immediately
      (AppShell + Sidebar respect `prefers-color-scheme`)
- [ ] Put app in background for 30s → return to foreground → connection
      state re-syncs within 3s (no stale "Disconnected" when VPN is still up)
- [ ] Open a deep link (`shuttle://…` URI handler) → app focuses and
      shows the add-server confirmation
- [ ] Legacy URL in the WebView (visit `#/dashboard`) → redirects to `#/`
      with a one-time toast; second visit is silent

## iOS VPN mode (real device, TestFlight Phase β build)

These items only exist in iOS VPN mode. On other runtimes the same
interactions are exercised by the cross-cutting checklist above —
unification means a tester reading this section should see no
behavioural difference from desktop or Android, only different
performance characteristics where polling stands in for WebSockets.

Pre-requisites:
- TestFlight build with bridge enabled (Phase β), real iPhone iOS 15+
- Wi-Fi or cellular for the actual proxied traffic
- `?bridge=auto` (default) — for debugging swap to `?bridge=0` to force
  HTTP fallback or `?bridge=1` to force bridge install regardless of
  probe outcome

- [ ] First connect: system VPN-permission dialog appears, allow → SPA
      first paint <3s
- [ ] Now page Power button: Connect → Connected feedback within 2s
      (matches Android VPN; perceptibly equivalent to desktop)
- [ ] Servers page: list loads, QR scan import, edit name, delete —
      identical flows to desktop / Android
- [ ] Servers page with 500+ entries: pagination is smooth, no jank
      (paginated requests stay under the 192 KB envelope cap)
- [ ] Activity → Logs: new lines appear within 1s (poll cadence; user
      should not perceive a difference from desktop's WS-pushed feed)
- [ ] Activity → Logs continuous 5 min: no dropped lines, no duplicate
      lines, scroll auto-follow stays in sync
- [ ] Settings: change theme persists across cold-restart of the app
- [ ] Background 30s → foreground: SPA recovers without white-screen,
      connection-state dot returns to green within 3s
- [ ] Background 5min → foreground: may show one-time "event stream
      gap recovered" toast; servers/status data refreshes to current truth
- [ ] Force-quit app while VPN is up → relaunch: SPA reloads, bridge
      reconnects, no Connect/Disconnect toggle desync
- [ ] Switch config from VPN → proxy mode → back to VPN: each transition
      reloads the WebView smoothly, no stuck state
- [ ] (Diagnostic only) `?bridge=0` URL — confirms inline-HTML fallback
      still loads if needed for triage during Phase β

### Phase γ acceptance gate (Task 6.4 — removes fallback HTML)

Task 6.4 lands once these hold. The criteria are deliberately what we
can actually measure — Shuttle is a privacy-sensitive VPN tool and does
not phone home (spec §11.3 *不打远程上报*), so cohort metrics aren't
available. Instead, gate on direct evidence:

- [ ] iOS VPN-mode smoke checklist (the section above) passes on ≥3
      developer TestFlight devices, run by ≥2 different testers
- [ ] No bridge-failure crash reports in 7 days of TestFlight (Apple's
      Crashes dashboard — opt-in but visible to the publisher)
- [ ] No GitHub issues / Telegram / direct feedback reporting iOS VPN
      regressions during the same 7-day window
- [ ] On each test device, Settings → Diagnostics shows <1 fallback
      trigger per 24 h (per-device counter, no aggregation needed —
      see spec §11.3)
- [ ] Manual perf check: a test device running for 24 h shows extension
      memory <40 MB via Xcode Instruments / Console.app (spot-check, not
      cohort)

## Known gaps

- `subscribeStatus` bridge action returns `unsupported` on both platforms
  — status polling via REST is adequate; a native push path is a later
  optimization.
- Navigation accessibility: chip-row `role="radio"` buttons on
  `SourceFilter` don't yet support arrow-key navigation — touch-only
  usable, keyboard users fall back to Tab focus.
