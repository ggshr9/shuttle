# Multi-Platform Service Management Design

**Date:** 2026-04-18
**Status:** Draft

## Problem

`shuttled` currently only supports daemon mode on Linux with systemd (`cmd/shuttled/service.go`). `shuttle` (client) has no daemon mode at all. macOS and Windows users have no first-class way to run Shuttle as a persistent background service; error paths today are "this is only supported on Linux."

This breaks Shuttle's cross-platform positioning. A unified daemon story is needed so that:

- Users on macOS and Windows can deploy `shuttled` as a real, always-on service without hand-writing launchd plists or SCM configurations.
- `shuttle` (CLI client) can run headless in the background on any platform, distinct from the GUI login-item auto-start already handled by `autostart/`.
- The CLI surface (`install / uninstall / start / stop / restart / status / logs`) is identical across Linux / macOS / Windows.
- The Web UI at a single port is the primary cross-platform control plane; platform-specific details of the underlying service manager are hidden from the user.

## Non-Goals

- FreeBSD rc.d integration (deferred; `autostart_freebsd.go` exists as precedent, but not in this iteration).
- systemd socket activation, drop-ins, timer units.
- Windows Event Log registration — first iteration uses file logs under `%ProgramData%\Shuttle\logs\`.
- launchd complex KeepAlive conditions (`NetworkState`, `PathState`, etc.).
- Windows service recovery action sequences beyond simple `Restart=true`.
- Replacing `autostart/` (login-item GUI auto-start) — it remains for the GUI, with clear boundary documentation.

## Design

### Architectural Layers

```
┌──────────────────────────────────────────────────────────┐
│  Control Plane (cross-platform identical)                │
│  • Web UI @ ADDR (shuttled/shuttle, gui/api handler)     │
│  • CLI verbs: install/start/stop/restart/status/logs     │
└──────────────────────┬───────────────────────────────────┘
                       │
┌──────────────────────▼───────────────────────────────────┐
│  service/ package (per-OS implementation, unified API)   │
│  ┌──────────────┬──────────────┬──────────────────────┐  │
│  │ Linux        │ macOS        │ Windows              │  │
│  │ systemd      │ launchd      │ SCM                  │  │
│  │ systemctl    │ launchctl    │ x/sys/windows/svc    │  │
│  │ system+user  │ Daemon+Agent │ SYSTEM+user          │  │
│  └──────────────┴──────────────┴──────────────────────┘  │
└──────────────────────────────────────────────────────────┘
```

### Boundary: `service/` vs `autostart/`

Two different packages, zero overlap:

| Package | Purpose | Target binary | Triggered by | Mechanism |
|---|---|---|---|---|
| `autostart/` | GUI login auto-start (user convenience) | `shuttle-gui` | User login | LaunchAgent (`KeepAlive=false`) / XDG autostart / registry Run key |
| `service/` | System/user daemon (persistent service) | `shuttled`, `shuttle` (CLI) | OS boot / session start | systemd unit / LaunchDaemon or LaunchAgent (`KeepAlive=true`) / Windows Service |

The CLI binaries never touch `autostart/`; the GUI binary never touches `service/`.

### Package: `service/`

```
service/
  service.go              # Manager interface, Config, Scope, Status, New()
  paths.go                # BinaryPath resolution, arg normalization
  errors.go               # ErrUnsupported, ErrAlreadyInstalled, ...
  service_linux.go        # //go:build linux
  service_darwin.go       # //go:build darwin && !ios
  service_windows.go      # //go:build windows
  service_unsupported.go  # //go:build !linux && !darwin && !windows
  service_sandbox_test.go # //go:build sandbox — Docker integration
```

**Public API (`service/service.go`):**

```go
type Scope int
const (
    ScopeSystem Scope = iota  // root/LocalSystem — shuttled default
    ScopeUser                 // current user     — shuttle default
)

type Status int
const (
    StatusUnknown Status = iota
    StatusNotInstalled
    StatusStopped
    StatusRunning
)

type Config struct {
    Name        string   // "shuttled" or "shuttle"
    DisplayName string   // "Shuttle Server"
    Description string
    BinaryPath  string   // absolute, EvalSymlinks-resolved
    Args        []string // ["run", "-c", "/etc/shuttle/server.yaml"]
    Scope       Scope
    User        string   // only meaningful if Scope=System and non-root run desired

    Restart     bool // default true
    RestartSec  int  // default 5

    LimitNOFILE int  // systemd only; ignored elsewhere

    LogDir      string // per-OS default via internal/paths, overridable
}

type Manager interface {
    Install(Config) error       // idempotent: uninstall-then-install if present
    Uninstall(purge bool) error // purge removes logs + config
    Start() error
    Stop() error
    Restart() error
    Status() (Status, error)
    Logs(follow bool) error     // streams to stdout; per-OS backend
}

func New(name string, scope Scope) (Manager, error)
```

**Linux implementation (`service_linux.go`):**

- `ScopeSystem`: writes `/etc/systemd/system/<name>.service`, uses `systemctl ...`.
- `ScopeUser`: writes `$XDG_CONFIG_HOME/systemd/user/<name>.service`, uses `systemctl --user ...`.
- `Logs()`: wraps `journalctl -u <name>` or `journalctl --user -u <name>`.

Unit template:
```
[Unit]
Description={{.Description}}
After=network-online.target
Wants=network-online.target

[Service]
ExecStart={{.BinaryPath}} {{.Args}}
Restart={{if .Restart}}always{{else}}no{{end}}
RestartSec={{.RestartSec}}
LimitNOFILE={{.LimitNOFILE}}
{{if .User}}User={{.User}}{{end}}

[Install]
WantedBy={{if system}}multi-user.target{{else}}default.target{{end}}
```

**macOS implementation (`service_darwin.go`):**

- `ScopeSystem`: writes `/Library/LaunchDaemons/com.shuttle.<name>.plist`, uses `launchctl bootstrap system` / `bootout` / `kickstart` / `print`.
- `ScopeUser`: writes `~/Library/LaunchAgents/com.shuttle.<name>.plist`, uses `launchctl bootstrap gui/$UID`.
- `Logs()`: tails `{LogDir}/<name>.log` and `<name>.err.log` (the plist sets `StandardOutPath` / `StandardErrorPath` to these).

Plist template includes `RunAtLoad=true`, `KeepAlive=true`, `StandardOutPath`, `StandardErrorPath`, `ProgramArguments` array.

**Windows implementation (`service_windows.go`):**

- Uses `golang.org/x/sys/windows/svc/mgr` for Install/Uninstall/Start/Stop/Status.
- Service name = `Config.Name`; display name, description, start type (`mgr.StartAutomatic`), `ServiceStartName` (account), and recovery actions (`RestartIfNonzero=true`, reset period) set via `mgr.Config`.
- `Logs()`: tails `{LogDir}\<name>.log` (populated by the Go binary's slog file handler when `svc.IsWindowsService()==true`).

The Windows binary itself must cooperate with SCM — see "Windows Entry-Point Structure" below.

### Windows Entry-Point Structure (no `init()` hack)

Each binary splits its entry-point into OS-conditional files:

```go
// cmd/shuttled/main_default.go   //go:build !windows
package main
func servicePreflight() {}

// cmd/shuttled/main_windows.go   //go:build windows
package main

import (
    "golang.org/x/sys/windows/svc"
    "log"
    "os"
)

func servicePreflight() {
    isService, err := svc.IsWindowsService()
    if err != nil || !isService { return }
    // Started by SCM — run the service handler, never return to CLI logic.
    if err := svc.Run("shuttled", &winSvcHandler{}); err != nil {
        log.Fatal(err)
    }
    os.Exit(0)
}
```

```go
// main.go (all OS)
func main() {
    servicePreflight() // no-op on non-windows; may not return on windows
    // ... existing CLI dispatch
}
```

**`winSvcHandler.Execute`** implements `svc.Handler`:
1. Parses `os.Args` via the normal CLI dispatch path to extract `-c CONFIG` and other flags. SCM invokes the binary with the configured `BinaryPathName` + `ServiceArgs`; these appear in `os.Args` as usual.
2. Accepts `svc.AcceptStop | svc.AcceptShutdown`.
3. Launches `run(ctx, configPath)` in a goroutine with a cancellable `context.Context`.
4. Blocks on `r` (ChangeRequest channel); on `Stop` / `Shutdown`, cancels the context and waits for the goroutine to return.
5. Reports `StartPending → Running → StopPending → Stopped` via `changes` channel.

Same file-split pattern applied to `cmd/shuttle/`.

### CLI Surface (symmetric across binaries)

```
<binary> install [--scope system|user] [--ui ADDR] [-c CONFIG] [-p PASSWORD]
<binary> uninstall [--purge]
<binary> start
<binary> stop
<binary> restart
<binary> status
<binary> logs [-f]
<binary> token                       # print Web UI bearer token
<binary> run [-c CONFIG] [--ui ADDR] # foreground
<binary> run -p PASSWORD -d          # backward-compat alias for install+start
```

**Defaults:**
- `shuttled` → `--scope system`
- `shuttle` → `--scope user`

**Deletion:** `cmd/shuttled/service.go` is deleted. All logic moves to `service/` + a thin CLI dispatch in `cmd/shuttled/main.go`.

**Backward compat:**
- `shuttled run -p PW -d` continues to work; internally: bootstrap config (`config.Bootstrap` with password, `Force=true`) → `Install(Scope=System, ...)` → `Start()`. If a config already exists, `-p` overwrites it (matching today's behavior); if `-p` is absent and config exists, it is reused unchanged.
- First time `install` runs on a host with a pre-existing `/etc/systemd/system/shuttled.service` (from the old code), the Linux implementation detects it, issues `systemctl stop; systemctl disable; rm`, and reinstalls using the canonical path (noisy one-time message).

### Web UI Integration (`--ui ADDR`)

- New flag `--ui ADDR` on `<binary> run` and `<binary> install` (e.g., `--ui :9090` or `--ui 0.0.0.0:9090`).
- When set, the run loop starts an HTTP server using `gui/api.NewHandler(HandlerConfig{Engine, SubMgr, ...})` on that address — the exact same handler used by the Wails GUI.
- `install` persists the address into config (`ui.listen: ADDR`), so `start` / `restart` respects it without re-passing the flag.
- **Flag resolution order for `run`:** explicit `--ui` flag > `config.ui.listen` > empty (no Web UI).
- Bearer token: `install` generates 32 random bytes, hex-encodes (64 hex chars), writes to config (`ui.token: "..."`, file mode 0600). Printed once to stdout during install; retrievable later via `<binary> token`.
- First-line hint on start: `Web UI: http://<host>:<port>/?token=<token>` written to stdout (or log file under service mode).

### Config Path Conventions (`internal/paths/`)

New package centralizes per-OS paths:

```
internal/paths/
  paths.go          # type Paths, Resolve(scope) function
  paths_linux.go    # XDG-aware
  paths_darwin.go
  paths_windows.go
```

```go
type Paths struct {
    ConfigDir string // where *.yaml live
    LogDir    string
    StateDir  string // cache, temp, runtime state
    Binary    string // system bin install dir (for completeness)
}

func Resolve(scope Scope) Paths
```

| Platform | system | user |
|---|---|---|
| Linux config | `/etc/shuttle/` | `${XDG_CONFIG_HOME:-~/.config}/shuttle/` |
| Linux logs   | `/var/log/shuttle/` | `${XDG_STATE_HOME:-~/.local/state}/shuttle/logs/` |
| macOS config | `/Library/Application Support/Shuttle/` | `~/Library/Application Support/Shuttle/` |
| macOS logs   | `/Library/Logs/Shuttle/` | `~/Library/Logs/Shuttle/` |
| Windows config | `%ProgramData%\Shuttle\` | `%AppData%\Shuttle\` |
| Windows logs   | `%ProgramData%\Shuttle\logs\` | `%AppData%\Shuttle\logs\` |

The `service.Config.LogDir` defaults from `paths.Resolve(scope).LogDir`. Callers may override.

### Logs Command

`<binary> logs [-f]` per OS:

| Platform | Implementation |
|---|---|
| Linux system | `journalctl -u <name> [--follow]` |
| Linux user   | `journalctl --user -u <name> [--follow]` |
| macOS        | `tail [-f]` the `.log` and `.err.log` files in `LogDir` |
| Windows      | `tail [-f]` the file log under `LogDir\<name>.log` |

File logs on Windows are written by the Go binary's slog handler when running under SCM; path set via `LogDir` in service config passed as an env var or arg.

### Data / Control Flow

```
User: `shuttled install -p yourpass --ui :9090`
  │
  ├── Bootstrap config (existing): generate server.yaml in paths.Resolve(system).ConfigDir
  ├── Generate bearer token, store in config.ui.token
  ├── service.New("shuttled", ScopeSystem) returns per-OS Manager
  ├── Manager.Install(Config{
  │       BinaryPath: /usr/local/bin/shuttled,
  │       Args:       [run, -c, /etc/shuttle/server.yaml],
  │       Scope:      System, ... })
  │   └── Linux:   write unit file → systemctl daemon-reload && enable
  │   └── macOS:   write plist      → launchctl bootstrap system
  │   └── Windows: mgr.Connect + mgr.CreateService + SetConfig
  ├── Manager.Start()
  └── Print Web UI URL + token
```

```
SCM (Windows) starts shuttled.exe
  │
  ├── main() calls servicePreflight()
  ├── servicePreflight sees svc.IsWindowsService() == true
  ├── svc.Run("shuttled", handler)
  │   └── handler.Execute:
  │         starts run(cfg) in goroutine with ctx
  │         reports Running to SCM
  │         waits for Stop/Shutdown on r
  │         on stop: cancel ctx, await goroutine, report Stopped
  └── os.Exit(0) — never falls through to CLI dispatch
```

### Error Handling

- All `Manager` operations return typed errors (`ErrUnsupported`, `ErrNotInstalled`, `ErrAlreadyInstalled`, `ErrPermission`).
- CLI maps typed errors to user-friendly messages (e.g., `ErrPermission` → "Try with sudo on Linux/macOS, or run as Administrator on Windows").
- `Install` on non-Linux/macOS/Windows OS returns `ErrUnsupported` with guidance pointing to manual docs.
- Transient failures (e.g., `systemctl start` fails because port already bound) propagate with output captured, not swallowed.

### Migration from Current State

1. **Users running `shuttled run -d` today (Linux)**: new `shuttled run -d` detects pre-existing `/etc/systemd/system/shuttled.service`, uninstalls it, reinstalls via new path. Idempotent.
2. **No data migration needed** — service unit files are regenerated on each install.
3. **Config files unchanged** in location (already at `/etc/shuttle/server.yaml` for system scope on Linux).

## Testing Strategy

Two-tier testing matches existing `//go:build sandbox` convention plus CI-only runners for macOS/Windows.

### Tier 1 — Unit tests (host-safe)
- `service/service_test.go`: `Config` validation, path resolution, template rendering (no OS calls).
- All host tests remain safe for `./scripts/test.sh` default run.

### Tier 2 — Integration tests

**Linux (Docker sandbox, `//go:build sandbox`):**
- Extend `sandbox/` with a systemd-enabled container.
- `service/service_sandbox_test.go` runs install → start → status → stop → uninstall full cycle.
- Exercises both system and user scopes (`systemd --user` in container).

**macOS (GitHub Actions, `//go:build integration_darwin`):**
- CI job on `macos-latest` runner.
- Runs `launchctl`-backed full cycle; teardown ensures no leftover plist.
- Not run by `./scripts/test.sh` locally (documented).

**Windows (GitHub Actions, `//go:build integration_windows`):**
- CI job on `windows-latest` runner, requires Administrator (default for Actions).
- Uses SCM API full cycle.
- Also not run locally.

Add three workflow jobs to `.github/workflows/` (extend existing test workflow, not the release one).

### Regression guarantees
- `./scripts/test.sh` stays fast and safe (no system mutations on host).
- `./scripts/test.sh --sandbox` exercises Linux systemd.
- Full macOS/Windows coverage only on CI push to main / PR.

## Deliverables / Scope

**In scope (this spec):**
1. New `service/` package with Linux/macOS/Windows implementations.
2. New `internal/paths/` package.
3. `cmd/shuttled/main.go` refactored; `cmd/shuttled/service.go` deleted.
4. `cmd/shuttle/main.go` refactored; symmetric CLI surface added.
5. `cmd/shuttled/main_windows.go` + `cmd/shuttle/main_windows.go` SCM handlers.
6. `--ui ADDR` flag wired to `gui/api.NewHandler` in both binaries.
7. Bearer token bootstrap + `<binary> token` command.
8. `<binary> logs [-f]` command.
9. Migration logic for pre-existing Linux systemd unit.
10. Sandbox tests + CI jobs for macOS/Windows.
11. Updated `install.sh`, `README`, `CLAUDE.md` to reflect new commands.

**Out of scope:**
- GUI binary changes (`shuttle-gui` continues using `autostart/`).
- FreeBSD service support.
- Windows Event Log (file logs only).
- systemd socket activation / drop-in support.
- Advanced launchd conditions (NetworkState, etc.).

## Open Questions — None

All prior open questions resolved:
- ✅ `service/` is separate from `autostart/`.
- ✅ Windows entry-point via explicit `servicePreflight()`, no `init()`.
- ✅ Client default scope=user, server default scope=system.
- ✅ Windows logs: file-based, Event Log deferred.
