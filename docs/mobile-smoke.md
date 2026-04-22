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

## Known gaps

- iOS VPN-mode full-SPA mode is not wired yet (Phase 4 left the inline
  HTML fallback). VPN mode on iOS continues to show the simplified
  Connect button rather than the full Svelte UI. The follow-up spec
  covers the extension↔app IPC needed to bridge this.
- `subscribeStatus` bridge action returns `unsupported` on both platforms
  — status polling via REST is adequate for Phase 4; a native push path
  is a later optimization.
- Navigation accessibility: chip-row `role="radio"` buttons on
  `SourceFilter` don't yet support arrow-key navigation — touch-only
  usable, keyboard users fall back to Tab focus.
