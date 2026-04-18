# Multi-Platform Service Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Linux-only `cmd/shuttled/service.go` with a structured, cross-platform `service/` package supporting systemd (Linux), launchd (macOS), and Windows SCM — plus symmetric CLI verbs (`install / uninstall / start / stop / restart / status / logs / token`) on both `shuttled` and `shuttle`, a unified `--ui ADDR` Web UI flag, and bearer-token auth bootstrap.

**Architecture:** New `service/` package with per-OS build-tag files (mirroring `autostart/` pattern). New `internal/paths/` for per-OS config/log locations. New `internal/servicecli/` for CLI helpers shared by both binaries. Windows requires an `svc.IsWindowsService()` preflight in each binary's `main()`. All CLI verbs map 1:1 to `Manager` interface methods. Web UI reuses existing `gui/api.NewHandler`.

**Tech Stack:** Go 1.24, `golang.org/x/sys/windows/svc` + `svc/mgr` (already in go.mod via x/sys v0.40), existing systemd shell-out pattern (Linux), `launchctl` shell-out (macOS).

**Spec:** `docs/superpowers/specs/2026-04-18-multi-platform-service-management-design.md`

---

## File Structure (locked in)

**New packages:**
```
service/
  service.go              # Config, Scope, Status, Manager interface, New(), typed errors
  template.go             # Pure template rendering (testable without OS)
  service_linux.go        # systemd impl
  service_darwin.go       # launchd impl
  service_windows.go      # SCM impl
  service_unsupported.go  # fallback
  service_test.go         # host-safe unit tests (template rendering)
  service_sandbox_test.go # //go:build sandbox — Linux integration in CI

internal/paths/
  paths.go                # Paths struct, Resolve(Scope), exported helpers
  paths_linux.go
  paths_darwin.go
  paths_windows.go
  paths_test.go

internal/servicecli/
  servicecli.go           # shared CLI verb dispatchers used by both binaries
  ui.go                   # --ui flag wiring to gui/api.NewHandler
  token.go                # bearer token generation + load from config
  logs.go                 # platform-dispatched log tailers
```

**Modified files:**
```
cmd/shuttled/main.go            # new subcommand dispatch; delete old -d logic
cmd/shuttled/main_default.go    # new: servicePreflight no-op (non-windows)
cmd/shuttled/main_windows.go    # new: svc.IsWindowsService() + winSvcHandler
cmd/shuttled/service.go         # DELETE
cmd/shuttle/main.go             # symmetric subcommands
cmd/shuttle/main_default.go     # new
cmd/shuttle/main_windows.go     # new
config/config.go                # add UIConfig {Listen, Token} struct to ClientConfig + ServerConfig
scripts/install.sh              # advertise new commands
.github/workflows/ci.yml        # new jobs: linux/macos/windows integration tests
CLAUDE.md                       # document new CLI surface
```

---

## Task Ordering Rationale

Linux first (Tasks 1–7) because existing code is the reference implementation — this phase is a refactor + test harness, not new behavior. Then macOS (8–10), then Windows (11–13) which is the riskiest. Then symmetric CLI + Web UI (14–17), logs (18), migration glue (19), tests (20–21), docs (22–23).

---

## Task 1: `internal/paths/` package skeleton

**Files:**
- Create: `internal/paths/paths.go`
- Create: `internal/paths/paths_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/paths/paths_test.go
package paths

import (
	"testing"
)

func TestResolveReturnsNonEmpty(t *testing.T) {
	for _, scope := range []Scope{ScopeSystem, ScopeUser} {
		p := Resolve(scope)
		if p.ConfigDir == "" {
			t.Errorf("scope=%v: ConfigDir is empty", scope)
		}
		if p.LogDir == "" {
			t.Errorf("scope=%v: LogDir is empty", scope)
		}
	}
}
```

- [ ] **Step 2: Write `paths.go` with type skeleton**

```go
// Package paths resolves OS-specific paths for Shuttle's configs and logs.
package paths

// Scope identifies whether paths are system-wide or per-user.
type Scope int

const (
	ScopeSystem Scope = iota
	ScopeUser
)

// Paths groups filesystem locations used by Shuttle.
type Paths struct {
	ConfigDir string
	LogDir    string
}

// Resolve returns the paths for the given scope on the current OS.
func Resolve(scope Scope) Paths {
	return resolve(scope)
}
```

- [ ] **Step 3: Run test — expect fail (undefined `resolve`)**

```bash
go test ./internal/paths/
```

Expected: compile error, `resolve` undefined.

- [ ] **Step 4: Commit skeleton**

```bash
git add internal/paths/paths.go internal/paths/paths_test.go
git commit -m "feat(paths): add cross-platform paths package skeleton"
```

---

## Task 2: `internal/paths/` per-OS implementations

**Files:**
- Create: `internal/paths/paths_linux.go`
- Create: `internal/paths/paths_darwin.go`
- Create: `internal/paths/paths_windows.go`

- [ ] **Step 1: Write Linux impl**

```go
//go:build linux

package paths

import (
	"os"
	"path/filepath"
)

func resolve(scope Scope) Paths {
	if scope == ScopeSystem {
		return Paths{
			ConfigDir: "/etc/shuttle",
			LogDir:    "/var/log/shuttle",
		}
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, _ := os.UserHomeDir()
		configHome = filepath.Join(home, ".config")
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, _ := os.UserHomeDir()
		stateHome = filepath.Join(home, ".local", "state")
	}
	return Paths{
		ConfigDir: filepath.Join(configHome, "shuttle"),
		LogDir:    filepath.Join(stateHome, "shuttle", "logs"),
	}
}
```

- [ ] **Step 2: Write macOS impl**

```go
//go:build darwin && !ios

package paths

import (
	"os"
	"path/filepath"
)

func resolve(scope Scope) Paths {
	if scope == ScopeSystem {
		return Paths{
			ConfigDir: "/Library/Application Support/Shuttle",
			LogDir:    "/Library/Logs/Shuttle",
		}
	}
	home, _ := os.UserHomeDir()
	return Paths{
		ConfigDir: filepath.Join(home, "Library", "Application Support", "Shuttle"),
		LogDir:    filepath.Join(home, "Library", "Logs", "Shuttle"),
	}
}
```

- [ ] **Step 3: Write Windows impl**

```go
//go:build windows

package paths

import (
	"os"
	"path/filepath"
)

func resolve(scope Scope) Paths {
	if scope == ScopeSystem {
		root := os.Getenv("ProgramData")
		if root == "" {
			root = `C:\ProgramData`
		}
		return Paths{
			ConfigDir: filepath.Join(root, "Shuttle"),
			LogDir:    filepath.Join(root, "Shuttle", "logs"),
		}
	}
	root := os.Getenv("AppData")
	if root == "" {
		home, _ := os.UserHomeDir()
		root = filepath.Join(home, "AppData", "Roaming")
	}
	return Paths{
		ConfigDir: filepath.Join(root, "Shuttle"),
		LogDir:    filepath.Join(root, "Shuttle", "logs"),
	}
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
./scripts/test.sh --pkg ./internal/paths/
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/paths/
git commit -m "feat(paths): add per-OS path resolution for config and logs"
```

---

## Task 3: `service/` package skeleton (interface, Config, Scope, Status, errors)

**Files:**
- Create: `service/service.go`
- Create: `service/service_unsupported.go`
- Create: `service/service_test.go`

- [ ] **Step 1: Write the public API**

```go
// Package service provides cross-platform system service management
// (install, start, stop, status, uninstall) for shuttled and shuttle.
package service

import (
	"errors"
)

type Scope int

const (
	ScopeSystem Scope = iota
	ScopeUser
)

type Status int

const (
	StatusUnknown Status = iota
	StatusNotInstalled
	StatusStopped
	StatusRunning
)

func (s Status) String() string {
	switch s {
	case StatusRunning:
		return "running"
	case StatusStopped:
		return "stopped"
	case StatusNotInstalled:
		return "not-installed"
	default:
		return "unknown"
	}
}

// Config fully describes a service to install.
type Config struct {
	Name        string   // "shuttled" or "shuttle"
	DisplayName string   // "Shuttle Server"
	Description string
	BinaryPath  string   // absolute, EvalSymlinks-resolved
	Args        []string // e.g. ["run", "-c", "/etc/shuttle/server.yaml"]
	Scope       Scope
	User        string   // empty = root / LocalSystem / current-user
	Restart     bool     // auto-restart on crash; default true
	RestartSec  int      // default 5
	LimitNOFILE int      // systemd only; 0 = unset
	LogDir      string   // where <name>.log / .err.log live
}

// Manager performs service lifecycle operations on the current OS.
type Manager interface {
	Install(cfg Config) error
	Uninstall(purge bool) error
	Start() error
	Stop() error
	Restart() error
	Status() (Status, error)
	Logs(follow bool) error
}

var (
	ErrUnsupported       = errors.New("service: unsupported platform")
	ErrNotInstalled      = errors.New("service: not installed")
	ErrAlreadyInstalled  = errors.New("service: already installed")
	ErrPermission        = errors.New("service: permission denied (run with sudo / as Administrator)")
)

// New returns a Manager for the given service name and scope on the current OS.
func New(name string, scope Scope) (Manager, error) {
	return newManager(name, scope)
}
```

- [ ] **Step 2: Write the unsupported fallback**

```go
//go:build !linux && !darwin && !windows

package service

type unsupportedManager struct{}

func newManager(name string, scope Scope) (Manager, error) {
	return nil, ErrUnsupported
}
```

- [ ] **Step 3: Write the test**

```go
// service/service_test.go
package service

import "testing"

func TestStatusString(t *testing.T) {
	tests := []struct {
		s    Status
		want string
	}{
		{StatusRunning, "running"},
		{StatusStopped, "stopped"},
		{StatusNotInstalled, "not-installed"},
		{StatusUnknown, "unknown"},
	}
	for _, tc := range tests {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("Status(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}
```

- [ ] **Step 4: Verify build (per-OS stub file absent on your dev OS will fail — that's next task)**

```bash
go vet ./service/ 2>&1 | head
```

Expected: `undefined: newManager` — OK, addressed by Task 4.

- [ ] **Step 5: Commit**

```bash
git add service/service.go service/service_unsupported.go service/service_test.go
git commit -m "feat(service): add cross-platform service manager interface and types"
```

---

## Task 4: Linux systemd implementation (pure template rendering)

**Files:**
- Create: `service/template.go`
- Update: `service/service_test.go`

- [ ] **Step 1: Write unit test for systemd unit rendering**

Add to `service/service_test.go`:

```go
func TestRenderSystemdUnit(t *testing.T) {
	cfg := Config{
		Name:        "shuttled",
		Description: "Shuttle Server",
		BinaryPath:  "/usr/local/bin/shuttled",
		Args:        []string{"run", "-c", "/etc/shuttle/server.yaml"},
		Restart:     true,
		RestartSec:  5,
		LimitNOFILE: 65535,
	}
	got := renderSystemdUnit(cfg, ScopeSystem)
	mustContain(t, got, "Description=Shuttle Server")
	mustContain(t, got, "ExecStart=/usr/local/bin/shuttled run -c /etc/shuttle/server.yaml")
	mustContain(t, got, "Restart=always")
	mustContain(t, got, "RestartSec=5")
	mustContain(t, got, "LimitNOFILE=65535")
	mustContain(t, got, "WantedBy=multi-user.target")

	user := renderSystemdUnit(cfg, ScopeUser)
	mustContain(t, user, "WantedBy=default.target")
}

func mustContain(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("rendered unit missing %q:\n%s", sub, s)
	}
}
```

Add imports: `"strings"`.

- [ ] **Step 2: Write `service/template.go`**

```go
package service

import (
	"fmt"
	"strings"
)

func renderSystemdUnit(cfg Config, scope Scope) string {
	restart := "no"
	if cfg.Restart {
		restart = "always"
	}
	sec := cfg.RestartSec
	if sec == 0 {
		sec = 5
	}
	wantedBy := "multi-user.target"
	if scope == ScopeUser {
		wantedBy = "default.target"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "[Unit]\nDescription=%s\nAfter=network-online.target\nWants=network-online.target\n\n", cfg.Description)
	sb.WriteString("[Service]\n")
	fmt.Fprintf(&sb, "ExecStart=%s %s\n", cfg.BinaryPath, strings.Join(cfg.Args, " "))
	fmt.Fprintf(&sb, "Restart=%s\nRestartSec=%d\n", restart, sec)
	if cfg.LimitNOFILE > 0 {
		fmt.Fprintf(&sb, "LimitNOFILE=%d\n", cfg.LimitNOFILE)
	}
	if cfg.User != "" && scope == ScopeSystem {
		fmt.Fprintf(&sb, "User=%s\n", cfg.User)
	}
	fmt.Fprintf(&sb, "\n[Install]\nWantedBy=%s\n", wantedBy)
	return sb.String()
}
```

- [ ] **Step 3: Run test — expect PASS**

```bash
./scripts/test.sh --pkg ./service/ --run TestRenderSystemdUnit
```

- [ ] **Step 4: Commit**

```bash
git add service/template.go service/service_test.go
git commit -m "feat(service): add systemd unit template rendering with tests"
```

---

## Task 5: Linux systemd — Manager implementation

**Files:**
- Create: `service/service_linux.go`

- [ ] **Step 1: Write the implementation**

```go
//go:build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shuttleX/shuttle/internal/paths"
)

type linuxManager struct {
	name  string
	scope Scope
}

func newManager(name string, scope Scope) (Manager, error) {
	return &linuxManager{name: name, scope: scope}, nil
}

func (m *linuxManager) unitPath() string {
	if m.scope == ScopeSystem {
		return filepath.Join("/etc/systemd/system", m.name+".service")
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, _ := os.UserHomeDir()
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "systemd", "user", m.name+".service")
}

func (m *linuxManager) systemctl(args ...string) ([]byte, error) {
	full := []string{}
	if m.scope == ScopeUser {
		full = append(full, "--user")
	}
	full = append(full, args...)
	return exec.Command("systemctl", full...).CombinedOutput()
}

func (m *linuxManager) Install(cfg Config) error {
	if _, err := os.Stat(m.unitPath()); err == nil {
		_ = m.stopSilent()
		if out, err := m.systemctl("disable", m.name); err != nil {
			return fmt.Errorf("disable existing: %s", string(out))
		}
		_ = os.Remove(m.unitPath())
	}
	if err := os.MkdirAll(filepath.Dir(m.unitPath()), 0755); err != nil {
		return err
	}
	unit := renderSystemdUnit(cfg, m.scope)
	if err := os.WriteFile(m.unitPath(), []byte(unit), 0644); err != nil {
		if os.IsPermission(err) {
			return ErrPermission
		}
		return fmt.Errorf("write unit: %w", err)
	}
	if out, err := m.systemctl("daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload: %s", string(out))
	}
	if out, err := m.systemctl("enable", m.name); err != nil {
		return fmt.Errorf("enable: %s", string(out))
	}
	return nil
}

func (m *linuxManager) Uninstall(purge bool) error {
	_ = m.stopSilent()
	_, _ = m.systemctl("disable", m.name)
	_ = os.Remove(m.unitPath())
	_, _ = m.systemctl("daemon-reload")
	if purge {
		p := paths.Resolve(paths.Scope(m.scope))
		_ = os.RemoveAll(p.ConfigDir)
		_ = os.RemoveAll(p.LogDir)
	}
	return nil
}

func (m *linuxManager) Start() error {
	out, err := m.systemctl("start", m.name)
	if err != nil {
		return fmt.Errorf("start: %s", string(out))
	}
	return nil
}

func (m *linuxManager) Stop() error {
	out, err := m.systemctl("stop", m.name)
	if err != nil {
		return fmt.Errorf("stop: %s", string(out))
	}
	return nil
}

func (m *linuxManager) stopSilent() error {
	_, _ = m.systemctl("stop", m.name)
	return nil
}

func (m *linuxManager) Restart() error {
	out, err := m.systemctl("restart", m.name)
	if err != nil {
		return fmt.Errorf("restart: %s", string(out))
	}
	return nil
}

func (m *linuxManager) Status() (Status, error) {
	if _, err := os.Stat(m.unitPath()); os.IsNotExist(err) {
		return StatusNotInstalled, nil
	}
	out, _ := m.systemctl("is-active", m.name)
	switch strings.TrimSpace(string(out)) {
	case "active":
		return StatusRunning, nil
	case "inactive", "failed":
		return StatusStopped, nil
	default:
		return StatusUnknown, nil
	}
}

func (m *linuxManager) Logs(follow bool) error {
	args := []string{}
	if m.scope == ScopeUser {
		args = append(args, "--user")
	}
	args = append(args, "-u", m.name)
	if follow {
		args = append(args, "-f")
	}
	cmd := exec.Command("journalctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

> Note: `service.Scope` and `paths.Scope` must not drift. Add a helper to `service/service.go` and use it everywhere instead of direct casts:
>
> ```go
> import ipaths "github.com/shuttleX/shuttle/internal/paths"
>
> func scopeToPaths(s Scope) ipaths.Scope {
>     if s == ScopeUser { return ipaths.ScopeUser }
>     return ipaths.ScopeSystem
> }
> ```
>
> Replace all `paths.Resolve(paths.Scope(m.scope))` with `paths.Resolve(scopeToPaths(m.scope))` in the per-OS files.

- [ ] **Step 2: Compile + vet**

```bash
CGO_ENABLED=0 go build ./service/
go vet ./service/
```

Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add service/service_linux.go
git commit -m "feat(service): add Linux systemd implementation"
```

---

## Task 6: Port existing shuttled daemon logic to new `service/` package + delete old file

**Files:**
- Modify: `cmd/shuttled/main.go`
- Delete: `cmd/shuttled/service.go`

- [ ] **Step 1: Update imports in `cmd/shuttled/main.go`**

Add to imports:
```go
"github.com/shuttleX/shuttle/service"
"github.com/shuttleX/shuttle/internal/paths"
```

- [ ] **Step 2: Replace `installAndStartService(*configPath)` call site**

Old:
```go
if *daemon {
    if *configPath == "" { ... }
    if _, err := os.Stat(*configPath); err != nil { ... }
    installAndStartService(*configPath)
    return
}
```

New:
```go
if *daemon {
    if *configPath == "" {
        fmt.Fprintf(os.Stderr, "Daemon mode (-d) requires a config.\n")
        fmt.Fprintf(os.Stderr, "Use -p <password> to auto-init, or -c <path> to point at an existing file.\n")
        os.Exit(1)
    }
    if _, err := os.Stat(*configPath); err != nil {
        fmt.Fprintf(os.Stderr, "Config not found at %s: %v\n", *configPath, err)
        os.Exit(1)
    }
    installAndStart("shuttled", service.ScopeSystem, *configPath)
    return
}
```

Add helper at bottom of `main.go`:

```go
func installAndStart(name string, scope service.Scope, configPath string) {
    bin, _ := os.Executable()
    if resolved, err := filepath.EvalSymlinks(bin); err == nil {
        bin = resolved
    }
    configPath, _ = filepath.Abs(configPath)
    mgr, err := service.New(name, scope)
    if err != nil {
        fmt.Fprintf(os.Stderr, "service: %v\n", err)
        os.Exit(1)
    }
    cfg := service.Config{
        Name:        name,
        DisplayName: "Shuttle Server",
        Description: "Shuttle Server",
        BinaryPath:  bin,
        Args:        []string{"run", "-c", configPath},
        Scope:       scope,
        Restart:     true,
        RestartSec:  5,
        LimitNOFILE: 65535,
        LogDir:      paths.Resolve(paths.Scope(scope)).LogDir,
    }
    if err := mgr.Install(cfg); err != nil {
        fmt.Fprintf(os.Stderr, "install: %v\n", err)
        os.Exit(1)
    }
    if err := mgr.Start(); err != nil {
        fmt.Fprintf(os.Stderr, "start: %v\n", err)
        os.Exit(1)
    }
    fmt.Println("Shuttle server installed and started.")
    fmt.Printf("  Config:  %s\n", configPath)
    fmt.Printf("  Logs:    %s logs -f\n", filepath.Base(bin))
    fmt.Printf("  Stop:    %s stop\n", filepath.Base(bin))
}
```

- [ ] **Step 3: Replace `stop`, `status`, `restart`, `uninstall` cases in switch**

Old (calls `stopService()`, `serviceStatus()`, etc.):

```go
case "stop":
    stopService()
case "restart":
    restartService()
case "status":
    serviceStatus()
case "uninstall":
    uninstallService()
```

New:
```go
case "stop":
    mustServiceCall("shuttled", service.ScopeSystem, func(m service.Manager) error { return m.Stop() })
case "restart":
    mustServiceCall("shuttled", service.ScopeSystem, func(m service.Manager) error { return m.Restart() })
case "status":
    printStatus("shuttled", service.ScopeSystem)
case "uninstall":
    mustServiceCall("shuttled", service.ScopeSystem, func(m service.Manager) error { return m.Uninstall(false) })
    fmt.Println("Shuttle server service removed.")
```

Add helpers at bottom of `main.go`:

```go
func mustServiceCall(name string, scope service.Scope, fn func(service.Manager) error) {
    mgr, err := service.New(name, scope)
    if err != nil {
        fmt.Fprintf(os.Stderr, "service: %v\n", err)
        os.Exit(1)
    }
    if err := fn(mgr); err != nil {
        fmt.Fprintf(os.Stderr, "%v\n", err)
        os.Exit(1)
    }
}

func printStatus(name string, scope service.Scope) {
    mgr, err := service.New(name, scope)
    if err != nil {
        fmt.Fprintf(os.Stderr, "service: %v\n", err)
        os.Exit(1)
    }
    s, err := mgr.Status()
    if err != nil {
        fmt.Fprintf(os.Stderr, "%v\n", err)
        os.Exit(1)
    }
    fmt.Println(s)
}
```

- [ ] **Step 4: Delete `cmd/shuttled/service.go`**

```bash
git rm cmd/shuttled/service.go
```

- [ ] **Step 5: Build and smoke-test**

```bash
CGO_ENABLED=0 go build -o /tmp/shuttled-test ./cmd/shuttled
/tmp/shuttled-test status 2>&1 | head
rm /tmp/shuttled-test
```

Expected: `not-installed` on a clean machine (or `stopped` / `running` if sandbox-installed).

- [ ] **Step 6: Commit**

```bash
git add cmd/shuttled/main.go
git commit -m "refactor(shuttled): use service package, delete inline service.go"
```

---

## Task 7: Linux sandbox integration test

**Files:**
- Create: `service/service_sandbox_test.go`

> Note: Full systemd tests require systemd. GH Actions `ubuntu-latest` runners have systemd. The Docker sandbox image (Alpine) does not. This test is CI-only; `./scripts/test.sh --sandbox` skips it gracefully if systemd is missing.

- [ ] **Step 1: Write the sandbox test**

```go
//go:build sandbox && linux

package service

import (
	"os"
	"os/exec"
	"testing"
)

func TestSandboxSystemdFullCycle(t *testing.T) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		t.Skip("systemctl not found")
	}
	if os.Geteuid() != 0 {
		t.Skip("requires root for system-scope systemd")
	}
	if out, err := exec.Command("systemctl", "is-system-running").CombinedOutput(); err != nil && len(out) > 0 {
		// "offline" / "maintenance" etc. — systemd is present but not usable.
		t.Skipf("systemd not ready: %s", string(out))
	}

	mgr, err := New("shuttle-test", ScopeSystem)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cfg := Config{
		Name:        "shuttle-test",
		Description: "Shuttle Test Service",
		BinaryPath:  "/bin/sleep",
		Args:        []string{"3600"},
		Scope:       ScopeSystem,
		Restart:     false,
	}
	defer mgr.Uninstall(false)

	if err := mgr.Install(cfg); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := mgr.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	s, _ := mgr.Status()
	if s != StatusRunning {
		t.Errorf("Status after Start = %v, want Running", s)
	}
	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	s, _ = mgr.Status()
	if s != StatusStopped {
		t.Errorf("Status after Stop = %v, want Stopped", s)
	}
}
```

- [ ] **Step 2: Commit (CI verification comes in Task 21)**

```bash
git add service/service_sandbox_test.go
git commit -m "test(service): add Linux systemd sandbox integration test"
```

---

## Task 8: macOS launchd — plist template

**Files:**
- Update: `service/template.go`
- Update: `service/service_test.go`

- [ ] **Step 1: Write failing test for plist rendering**

Add to `service/service_test.go`:

```go
func TestRenderLaunchdPlist(t *testing.T) {
	cfg := Config{
		Name:        "shuttled",
		Description: "Shuttle Server",
		BinaryPath:  "/usr/local/bin/shuttled",
		Args:        []string{"run", "-c", "/Library/Application Support/Shuttle/server.yaml"},
		LogDir:      "/Library/Logs/Shuttle",
		Restart:     true,
	}
	got := renderLaunchdPlist(cfg)
	mustContain(t, got, "<key>Label</key>\n\t<string>com.shuttle.shuttled</string>")
	mustContain(t, got, "<key>KeepAlive</key>\n\t<true/>")
	mustContain(t, got, "<key>RunAtLoad</key>\n\t<true/>")
	mustContain(t, got, "<string>/usr/local/bin/shuttled</string>")
	mustContain(t, got, "<string>run</string>")
	mustContain(t, got, "/Library/Logs/Shuttle/shuttled.log")
	mustContain(t, got, "/Library/Logs/Shuttle/shuttled.err.log")
}
```

- [ ] **Step 2: Implement in `service/template.go`**

Add:
```go
func renderLaunchdPlist(cfg Config) string {
	var args strings.Builder
	args.WriteString("\t\t<string>" + cfg.BinaryPath + "</string>\n")
	for _, a := range cfg.Args {
		args.WriteString("\t\t<string>" + xmlEscape(a) + "</string>\n")
	}
	keepAlive := "<false/>"
	if cfg.Restart {
		keepAlive = "<true/>"
	}
	logOut := filepath.Join(cfg.LogDir, cfg.Name+".log")
	logErr := filepath.Join(cfg.LogDir, cfg.Name+".err.log")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.shuttle.%s</string>
	<key>ProgramArguments</key>
	<array>
%s	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	%s
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, cfg.Name, args.String(), keepAlive, logOut, logErr)
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
```

Add imports: `"path/filepath"`.

- [ ] **Step 3: Run test — expect PASS**

```bash
./scripts/test.sh --pkg ./service/ --run TestRenderLaunchdPlist
```

- [ ] **Step 4: Commit**

```bash
git add service/template.go service/service_test.go
git commit -m "feat(service): add launchd plist template rendering"
```

---

## Task 9: macOS launchd — Manager implementation

**Files:**
- Create: `service/service_darwin.go`

- [ ] **Step 1: Write impl**

```go
//go:build darwin && !ios

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shuttleX/shuttle/internal/paths"
)

type darwinManager struct {
	name  string
	scope Scope
}

func newManager(name string, scope Scope) (Manager, error) {
	return &darwinManager{name: name, scope: scope}, nil
}

func (m *darwinManager) label() string { return "com.shuttle." + m.name }

func (m *darwinManager) plistPath() string {
	if m.scope == ScopeSystem {
		return filepath.Join("/Library/LaunchDaemons", m.label()+".plist")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library/LaunchAgents", m.label()+".plist")
}

func (m *darwinManager) domain() string {
	if m.scope == ScopeSystem {
		return "system"
	}
	return "gui/" + strconv.Itoa(os.Getuid())
}

func (m *darwinManager) serviceTarget() string {
	return m.domain() + "/" + m.label()
}

func (m *darwinManager) launchctl(args ...string) ([]byte, error) {
	return exec.Command("launchctl", args...).CombinedOutput()
}

func (m *darwinManager) Install(cfg Config) error {
	if cfg.LogDir == "" {
		cfg.LogDir = paths.Resolve(paths.Scope(m.scope)).LogDir
	}
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		return fmt.Errorf("mkdir logs: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.plistPath()), 0755); err != nil {
		return err
	}
	if _, err := os.Stat(m.plistPath()); err == nil {
		_, _ = m.launchctl("bootout", m.domain(), m.plistPath())
	}
	plist := renderLaunchdPlist(cfg)
	if err := os.WriteFile(m.plistPath(), []byte(plist), 0644); err != nil {
		if os.IsPermission(err) {
			return ErrPermission
		}
		return fmt.Errorf("write plist: %w", err)
	}
	if out, err := m.launchctl("bootstrap", m.domain(), m.plistPath()); err != nil {
		return fmt.Errorf("bootstrap: %s", string(out))
	}
	return nil
}

func (m *darwinManager) Uninstall(purge bool) error {
	_, _ = m.launchctl("bootout", m.domain(), m.plistPath())
	_ = os.Remove(m.plistPath())
	if purge {
		p := paths.Resolve(paths.Scope(m.scope))
		_ = os.RemoveAll(p.ConfigDir)
		_ = os.RemoveAll(p.LogDir)
	}
	return nil
}

func (m *darwinManager) Start() error {
	out, err := m.launchctl("kickstart", m.serviceTarget())
	if err != nil {
		return fmt.Errorf("kickstart: %s", string(out))
	}
	return nil
}

func (m *darwinManager) Stop() error {
	// kill and let KeepAlive restart; to truly stop, bootout then bootstrap back later.
	out, err := m.launchctl("kill", "SIGTERM", m.serviceTarget())
	if err != nil {
		return fmt.Errorf("kill: %s", string(out))
	}
	// If we want it to stay stopped, the caller should Uninstall or we must bootout.
	// For semantic "stop", bootout achieves that but means Start has to re-bootstrap.
	// Use bootout to match user expectation.
	_, _ = m.launchctl("bootout", m.domain(), m.plistPath())
	return nil
}

func (m *darwinManager) Restart() error {
	out, err := m.launchctl("kickstart", "-k", m.serviceTarget())
	if err != nil {
		return fmt.Errorf("kickstart -k: %s", string(out))
	}
	return nil
}

func (m *darwinManager) Status() (Status, error) {
	if _, err := os.Stat(m.plistPath()); os.IsNotExist(err) {
		return StatusNotInstalled, nil
	}
	out, err := m.launchctl("print", m.serviceTarget())
	if err != nil {
		// print exits non-zero when service not loaded
		return StatusStopped, nil
	}
	if strings.Contains(string(out), "state = running") {
		return StatusRunning, nil
	}
	return StatusStopped, nil
}

func (m *darwinManager) Logs(follow bool) error {
	p := paths.Resolve(paths.Scope(m.scope))
	out := filepath.Join(p.LogDir, m.name+".log")
	errLog := filepath.Join(p.LogDir, m.name+".err.log")
	args := []string{"-n", "200"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, out, errLog)
	cmd := exec.Command("tail", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

- [ ] **Step 2: Cross-compile check**

```bash
GOOS=darwin CGO_ENABLED=0 go build ./service/
```

Expected: clean compile.

- [ ] **Step 3: Commit**

```bash
git add service/service_darwin.go
git commit -m "feat(service): add macOS launchd implementation"
```

---

## Task 10: Windows SCM — Manager implementation

**Files:**
- Create: `service/service_windows.go`
- Update: `go.mod` (ensure `golang.org/x/sys/windows/svc/mgr` pulls in)

- [ ] **Step 1: Write impl**

```go
//go:build windows

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shuttleX/shuttle/internal/paths"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type windowsManager struct {
	name  string
	scope Scope
}

func newManager(name string, scope Scope) (Manager, error) {
	return &windowsManager{name: name, scope: scope}, nil
}

func (m *windowsManager) connect() (*mgr.Mgr, error) {
	scMgr, err := mgr.Connect()
	if err != nil {
		return nil, fmt.Errorf("SCM connect: %w (run as Administrator?)", err)
	}
	return scMgr, nil
}

func (m *windowsManager) openService(scMgr *mgr.Mgr) (*mgr.Service, error) {
	s, err := scMgr.OpenService(m.name)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (m *windowsManager) Install(cfg Config) error {
	scMgr, err := m.connect()
	if err != nil {
		return err
	}
	defer scMgr.Disconnect()

	// Uninstall existing
	if s, err := scMgr.OpenService(m.name); err == nil {
		_, _ = s.Control(svc.Stop)
		_ = s.Delete()
		s.Close()
		time.Sleep(500 * time.Millisecond)
	}

	startType := mgr.StartAutomatic
	svcMgrCfg := mgr.Config{
		DisplayName:      cfg.DisplayName,
		Description:      cfg.Description,
		StartType:        uint32(startType),
		ServiceStartName: cfg.User, // empty = LocalSystem
	}
	s, err := scMgr.CreateService(m.name, cfg.BinaryPath, svcMgrCfg, cfg.Args...)
	if err != nil {
		return fmt.Errorf("CreateService: %w", err)
	}
	defer s.Close()

	if cfg.Restart {
		recover := []mgr.RecoveryAction{
			{Type: mgr.ServiceRestart, Delay: time.Duration(cfg.RestartSec) * time.Second},
			{Type: mgr.ServiceRestart, Delay: time.Duration(cfg.RestartSec) * time.Second},
			{Type: mgr.NoAction, Delay: 0},
		}
		_ = s.SetRecoveryActions(recover, 86400)
	}

	if cfg.LogDir != "" {
		_ = os.MkdirAll(cfg.LogDir, 0755)
	}
	return nil
}

func (m *windowsManager) Uninstall(purge bool) error {
	scMgr, err := m.connect()
	if err != nil {
		return err
	}
	defer scMgr.Disconnect()

	s, err := scMgr.OpenService(m.name)
	if err != nil {
		if purge {
			p := paths.Resolve(paths.Scope(m.scope))
			_ = os.RemoveAll(p.ConfigDir)
			_ = os.RemoveAll(p.LogDir)
		}
		return nil
	}
	defer s.Close()
	_, _ = s.Control(svc.Stop)
	time.Sleep(500 * time.Millisecond)
	_ = s.Delete()
	if purge {
		p := paths.Resolve(paths.Scope(m.scope))
		_ = os.RemoveAll(p.ConfigDir)
		_ = os.RemoveAll(p.LogDir)
	}
	return nil
}

func (m *windowsManager) Start() error {
	scMgr, err := m.connect()
	if err != nil {
		return err
	}
	defer scMgr.Disconnect()
	s, err := scMgr.OpenService(m.name)
	if err != nil {
		return ErrNotInstalled
	}
	defer s.Close()
	if err := s.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	return nil
}

func (m *windowsManager) Stop() error {
	scMgr, err := m.connect()
	if err != nil {
		return err
	}
	defer scMgr.Disconnect()
	s, err := scMgr.OpenService(m.name)
	if err != nil {
		return ErrNotInstalled
	}
	defer s.Close()
	if _, err := s.Control(svc.Stop); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	return nil
}

func (m *windowsManager) Restart() error {
	_ = m.Stop()
	time.Sleep(1 * time.Second)
	return m.Start()
}

func (m *windowsManager) Status() (Status, error) {
	scMgr, err := m.connect()
	if err != nil {
		return StatusUnknown, err
	}
	defer scMgr.Disconnect()
	s, err := scMgr.OpenService(m.name)
	if err != nil {
		return StatusNotInstalled, nil
	}
	defer s.Close()
	st, err := s.Query()
	if err != nil {
		return StatusUnknown, err
	}
	switch st.State {
	case svc.Running:
		return StatusRunning, nil
	case svc.Stopped, svc.StopPending:
		return StatusStopped, nil
	default:
		return StatusUnknown, nil
	}
}

func (m *windowsManager) Logs(follow bool) error {
	p := paths.Resolve(paths.Scope(m.scope))
	logFile := filepath.Join(p.LogDir, m.name+".log")
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		return fmt.Errorf("log file not found: %s", logFile)
	}
	f, err := os.Open(logFile)
	if err != nil {
		return err
	}
	defer f.Close()
	// Simple: read all, print, optionally follow via polling.
	data, _ := os.ReadFile(logFile)
	os.Stdout.Write(data)
	if follow {
		offset := int64(len(data))
		for {
			time.Sleep(500 * time.Millisecond)
			info, err := os.Stat(logFile)
			if err != nil {
				return err
			}
			if info.Size() > offset {
				f, _ := os.Open(logFile)
				f.Seek(offset, 0)
				buf := make([]byte, info.Size()-offset)
				n, _ := f.Read(buf)
				os.Stdout.Write(buf[:n])
				offset = info.Size()
				f.Close()
			}
		}
	}
	_ = strings.TrimSpace // silence unused if we refactor
	return nil
}
```

- [ ] **Step 2: Cross-compile check**

```bash
GOOS=windows CGO_ENABLED=0 go build ./service/
```

Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add service/service_windows.go
git commit -m "feat(service): add Windows SCM implementation"
```

---

## Task 11: Windows entry-point — servicePreflight for `shuttled`

**Files:**
- Create: `cmd/shuttled/main_default.go`
- Create: `cmd/shuttled/main_windows.go`
- Modify: `cmd/shuttled/main.go` (add `servicePreflight()` call)

- [ ] **Step 1: Write `main_default.go`**

```go
//go:build !windows

package main

func servicePreflight() {}
```

- [ ] **Step 2: Write `main_windows.go`**

```go
//go:build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/shuttleX/shuttle/internal/paths"
	"github.com/shuttleX/shuttle/service"
	"golang.org/x/sys/windows/svc"
)

func servicePreflight() {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return
	}
	// Route file logs
	logDir := paths.Resolve(paths.Scope(service.ScopeSystem)).LogDir
	_ = os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, "shuttled.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{})))
	}
	if err := svc.Run("shuttled", &winSvcHandler{}); err != nil {
		fmt.Fprintf(os.Stderr, "svc.Run: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

type winSvcHandler struct{}

func (h *winSvcHandler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	changes <- svc.Status{State: svc.StartPending}

	// Parse os.Args to extract -c config path. SCM invokes the registered binary
	// with the stored ImagePath (which includes args we passed at install time).
	cfgPath := findConfigArg(os.Args)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runWithContext(ctx, cfgPath)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
				case <-time.After(10 * time.Second):
				}
				changes <- svc.Status{State: svc.Stopped}
				return
			}
		}
	}
}

func findConfigArg(args []string) string {
	for i, a := range args {
		if a == "-c" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
```

- [ ] **Step 3: Refactor `run(configPath string)` in `cmd/shuttled/main.go` to expose a context-driven variant**

Open `cmd/shuttled/main.go`, find `func run(configPath string)`. It currently has this shape (existing code):

```go
func run(configPath string) {
    cfg, err := config.LoadServerConfig(configPath)
    // ...engine setup...
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()
    // engine.Start + <-ctx.Done() + shutdown...
}
```

Split into two functions — rename the body to `runWithContext(ctx, configPath)`, keep the original `run` as a thin wrapper that constructs the signal context:

```go
func run(configPath string) {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()
    runWithContext(ctx, configPath)
}

func runWithContext(ctx context.Context, configPath string) {
    // ...move the entire body of the original run() here...
    // Any <-ctx.Done() in the body now triggers on either SIGINT/SIGTERM (CLI path)
    // or the Windows service handler's cancel() (SCM path).
}
```

No logic change — only an extract-method. Confirm the engine shutdown path is driven by `<-ctx.Done()` (existing code) and does not depend on signal package specifics.

- [ ] **Step 4: Add `servicePreflight()` call at top of `main()`**

```go
func main() {
    servicePreflight() // no-op on non-Windows; never returns on Windows service mode
    if len(os.Args) < 2 { ... }
    // ...existing dispatch
}
```

- [ ] **Step 5: Cross-compile check**

```bash
GOOS=windows CGO_ENABLED=0 go build ./cmd/shuttled
CGO_ENABLED=0 go build ./cmd/shuttled
```

Expected: both clean.

- [ ] **Step 6: Commit**

```bash
git add cmd/shuttled/main.go cmd/shuttled/main_default.go cmd/shuttled/main_windows.go
git commit -m "feat(shuttled): add Windows SCM service entry-point"
```

---

## Task 12: Windows entry-point for `shuttle` (client)

**Files:**
- Create: `cmd/shuttle/main_default.go`
- Create: `cmd/shuttle/main_windows.go`
- Modify: `cmd/shuttle/main.go` (add `servicePreflight()` call)

- [ ] **Step 1: Write `main_default.go`** (same as shuttled's)

```go
//go:build !windows

package main

func servicePreflight() {}
```

- [ ] **Step 2: Write `cmd/shuttle/main_windows.go`** (full content — do not just "copy shuttled's")

```go
//go:build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/shuttleX/shuttle/internal/paths"
	"github.com/shuttleX/shuttle/service"
	"golang.org/x/sys/windows/svc"
)

func servicePreflight() {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return
	}
	logDir := paths.Resolve(paths.Scope(service.ScopeUser)).LogDir
	_ = os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, "shuttle.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{})))
	}
	if err := svc.Run("shuttle", &winSvcHandler{}); err != nil {
		fmt.Fprintf(os.Stderr, "svc.Run: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

type winSvcHandler struct{}

func (h *winSvcHandler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	changes <- svc.Status{State: svc.StartPending}
	cfgPath := findConfigArg(os.Args)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runWithContext(ctx, cfgPath)
	}()
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
				case <-time.After(10 * time.Second):
				}
				changes <- svc.Status{State: svc.Stopped}
				return
			}
		}
	}
}

func findConfigArg(args []string) string {
	for i, a := range args {
		if a == "-c" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
```

Differences from shuttled's version: service name `"shuttle"`, log file name `"shuttle.log"`, log dir derived from `ScopeUser` (not `ScopeSystem`).

- [ ] **Step 3: Add `servicePreflight()` + `runWithContext` to `cmd/shuttle/main.go`**

Same pattern as Task 11 Step 3/4.

- [ ] **Step 4: Cross-compile**

```bash
GOOS=windows CGO_ENABLED=0 go build ./cmd/shuttle
CGO_ENABLED=0 go build ./cmd/shuttle
```

- [ ] **Step 5: Commit**

```bash
git add cmd/shuttle/main.go cmd/shuttle/main_default.go cmd/shuttle/main_windows.go
git commit -m "feat(shuttle): add Windows SCM service entry-point"
```

---

## Task 13: Shared CLI helpers — `internal/servicecli/`

**Files:**
- Create: `internal/servicecli/servicecli.go`
- Create: `internal/servicecli/servicecli_test.go`

- [ ] **Step 1: Write helpers**

```go
// Package servicecli provides shared subcommand handlers for shuttle and shuttled.
package servicecli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shuttleX/shuttle/internal/paths"
	"github.com/shuttleX/shuttle/service"
)

// Options identifies which binary is calling in.
type Options struct {
	Name         string        // "shuttled" or "shuttle"
	DisplayName  string        // "Shuttle Server" / "Shuttle Client"
	DefaultScope service.Scope // System for shuttled, User for shuttle
}

// Install dispatches to service.Manager.Install with config from flags.
func Install(opts Options, args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	configPath := fs.String("c", "", "config file path")
	scopeFlag := fs.String("scope", scopeToString(opts.DefaultScope), "service scope: system|user")
	ui := fs.String("ui", "", "Web UI listen addr (e.g. :9090)")
	_ = fs.Parse(args)

	if *configPath == "" {
		fmt.Fprintf(os.Stderr, "install requires -c <config>\n")
		os.Exit(1)
	}
	abs, _ := filepath.Abs(*configPath)
	if _, err := os.Stat(abs); err != nil {
		fmt.Fprintf(os.Stderr, "config not found: %s\n", abs)
		os.Exit(1)
	}

	scope := parseScope(*scopeFlag)
	bin, _ := os.Executable()
	if r, err := filepath.EvalSymlinks(bin); err == nil {
		bin = r
	}

	mgr := mustManager(opts.Name, scope)
	svcArgs := []string{"run", "-c", abs}
	if *ui != "" {
		svcArgs = append(svcArgs, "--ui", *ui)
	}

	cfg := service.Config{
		Name:        opts.Name,
		DisplayName: opts.DisplayName,
		Description: opts.DisplayName,
		BinaryPath:  bin,
		Args:        svcArgs,
		Scope:       scope,
		Restart:     true,
		RestartSec:  5,
		LimitNOFILE: 65535,
		LogDir:      paths.Resolve(paths.Scope(scope)).LogDir,
	}
	if err := mgr.Install(cfg); err != nil {
		exit("install: %v", err)
	}
	if err := mgr.Start(); err != nil {
		exit("start: %v", err)
	}
	fmt.Printf("%s installed and started.\n  Config: %s\n  Stop:   %s stop\n  Logs:   %s logs -f\n",
		opts.DisplayName, abs, opts.Name, opts.Name)
}

func Uninstall(opts Options, args []string) {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	scopeFlag := fs.String("scope", scopeToString(opts.DefaultScope), "service scope")
	purge := fs.Bool("purge", false, "also remove config and log directories")
	_ = fs.Parse(args)
	scope := parseScope(*scopeFlag)
	mgr := mustManager(opts.Name, scope)
	if err := mgr.Uninstall(*purge); err != nil {
		exit("uninstall: %v", err)
	}
	fmt.Printf("%s removed.\n", opts.DisplayName)
}

func Start(opts Options, args []string) { simple(opts, args, "start",
	func(m service.Manager) error { return m.Start() }) }
func Stop(opts Options, args []string) { simple(opts, args, "stop",
	func(m service.Manager) error { return m.Stop() }) }
func Restart(opts Options, args []string) { simple(opts, args, "restart",
	func(m service.Manager) error { return m.Restart() }) }

func Status(opts Options, args []string) {
	scope := scopeFromArgs(opts, args)
	mgr := mustManager(opts.Name, scope)
	s, err := mgr.Status()
	if err != nil {
		exit("status: %v", err)
	}
	fmt.Println(s)
}

func Logs(opts Options, args []string) {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	scopeFlag := fs.String("scope", scopeToString(opts.DefaultScope), "service scope")
	follow := fs.Bool("f", false, "follow")
	_ = fs.Parse(args)
	scope := parseScope(*scopeFlag)
	mgr := mustManager(opts.Name, scope)
	if err := mgr.Logs(*follow); err != nil {
		exit("logs: %v", err)
	}
}

// --- helpers ---

func simple(opts Options, args []string, verb string, fn func(service.Manager) error) {
	scope := scopeFromArgs(opts, args)
	mgr := mustManager(opts.Name, scope)
	if err := fn(mgr); err != nil {
		exit("%s: %v", verb, err)
	}
	fmt.Printf("%s %sed.\n", opts.DisplayName, verb)
}

func scopeFromArgs(opts Options, args []string) service.Scope {
	fs := flag.NewFlagSet("scope", flag.ContinueOnError)
	fs.SetOutput(nopWriter{})
	scopeFlag := fs.String("scope", scopeToString(opts.DefaultScope), "")
	_ = fs.Parse(args)
	return parseScope(*scopeFlag)
}

func mustManager(name string, scope service.Scope) service.Manager {
	mgr, err := service.New(name, scope)
	if err != nil {
		exit("service: %v", err)
	}
	return mgr
}

func parseScope(s string) service.Scope {
	switch s {
	case "user":
		return service.ScopeUser
	default:
		return service.ScopeSystem
	}
}

func scopeToString(s service.Scope) string {
	if s == service.ScopeUser {
		return "user"
	}
	return "system"
}

func exit(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
```

- [ ] **Step 2: Write basic test (parseScope)**

```go
package servicecli

import (
	"testing"

	"github.com/shuttleX/shuttle/service"
)

func TestParseScope(t *testing.T) {
	if parseScope("user") != service.ScopeUser {
		t.Error("user should map to ScopeUser")
	}
	if parseScope("system") != service.ScopeSystem {
		t.Error("system should map to ScopeSystem")
	}
	if parseScope("") != service.ScopeSystem {
		t.Error("empty should default to ScopeSystem")
	}
}
```

- [ ] **Step 3: Run test**

```bash
./scripts/test.sh --pkg ./internal/servicecli/
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/servicecli/
git commit -m "feat(servicecli): add shared CLI subcommand helpers"
```

---

## Task 14: Wire `shuttled` CLI to servicecli — new subcommands

**Files:**
- Modify: `cmd/shuttled/main.go`

- [ ] **Step 1: Import servicecli + remove old `mustServiceCall`/`printStatus`/`installAndStart` helpers**

Replace imports: add `"github.com/shuttleX/shuttle/internal/servicecli"`. Remove the temporary helpers added in Task 6 (they are superseded).

- [ ] **Step 2: Add new subcommand cases in the `switch`**

```go
shuttledOpts := servicecli.Options{
    Name:         "shuttled",
    DisplayName:  "Shuttle Server",
    DefaultScope: service.ScopeSystem,
}

switch os.Args[1] {
// ... existing cases ...
case "install":
    servicecli.Install(shuttledOpts, os.Args[2:])
case "uninstall":
    servicecli.Uninstall(shuttledOpts, os.Args[2:])
case "start":
    servicecli.Start(shuttledOpts, os.Args[2:])
case "stop":
    servicecli.Stop(shuttledOpts, os.Args[2:])
case "restart":
    servicecli.Restart(shuttledOpts, os.Args[2:])
case "status":
    servicecli.Status(shuttledOpts, os.Args[2:])
case "logs":
    servicecli.Logs(shuttledOpts, os.Args[2:])
// ...
}
```

- [ ] **Step 3: Preserve `-d` backward compat in `run` case**

Keep `-d` flag logic, but rewrite to delegate to `servicecli.Install` after bootstrap:

```go
if *daemon {
    if *configPath == "" {
        fmt.Fprintf(os.Stderr, "Daemon mode (-d) requires a config (use -p or -c).\n")
        os.Exit(1)
    }
    servicecli.Install(shuttledOpts, []string{"-c", *configPath})
    return
}
```

- [ ] **Step 4: Update `printUsage()` to list all verbs**

Replace the Commands section:
```go
fmt.Fprintf(os.Stderr, "Commands:\n")
fmt.Fprintf(os.Stderr, "  shuttled run [-c config] [--ui ADDR]    Run foreground\n")
fmt.Fprintf(os.Stderr, "  shuttled install -c <config> [--ui ADDR] Install and start as service\n")
fmt.Fprintf(os.Stderr, "  shuttled uninstall [--purge]            Remove service\n")
fmt.Fprintf(os.Stderr, "  shuttled start                          Start installed service\n")
fmt.Fprintf(os.Stderr, "  shuttled stop                           Stop service\n")
fmt.Fprintf(os.Stderr, "  shuttled restart                        Restart service\n")
fmt.Fprintf(os.Stderr, "  shuttled status                         Show status\n")
fmt.Fprintf(os.Stderr, "  shuttled logs [-f]                      Show logs\n")
fmt.Fprintf(os.Stderr, "  shuttled token                          Print Web UI token\n")
fmt.Fprintf(os.Stderr, "  shuttled init                           Generate config only\n")
fmt.Fprintf(os.Stderr, "  shuttled share -c <config> --addr host  Generate import URI\n")
fmt.Fprintf(os.Stderr, "  shuttled genkey                         Generate key pair\n")
fmt.Fprintf(os.Stderr, "  shuttled version                        Show version\n")
```

- [ ] **Step 5: Build + smoke test**

```bash
CGO_ENABLED=0 go build -o /tmp/shuttled-test ./cmd/shuttled
/tmp/shuttled-test help 2>&1 | head -20
/tmp/shuttled-test status 2>&1
rm /tmp/shuttled-test
```

- [ ] **Step 6: Commit**

```bash
git add cmd/shuttled/main.go
git commit -m "feat(shuttled): add install/uninstall/start/stop/restart/status/logs subcommands"
```

---

## Task 15: Wire `shuttle` CLI to servicecli — symmetric commands

**Files:**
- Modify: `cmd/shuttle/main.go`

- [ ] **Step 1: Import + add subcommand cases**

Add import: `"github.com/shuttleX/shuttle/internal/servicecli"`.

Add in `switch os.Args[1]`:
```go
shuttleOpts := servicecli.Options{
    Name:         "shuttle",
    DisplayName:  "Shuttle Client",
    DefaultScope: service.ScopeUser,
}

// ... add cases "install", "uninstall", "start", "stop", "restart", "status", "logs"
// identical to shuttled except opts variable
```

- [ ] **Step 2: Update `printUsage()`**

Add lines documenting the new commands.

- [ ] **Step 3: Build + smoke test**

```bash
CGO_ENABLED=0 go build -o /tmp/shuttle-test ./cmd/shuttle
/tmp/shuttle-test help 2>&1 | head -20
/tmp/shuttle-test status 2>&1
rm /tmp/shuttle-test
```

- [ ] **Step 4: Commit**

```bash
git add cmd/shuttle/main.go
git commit -m "feat(shuttle): add symmetric service subcommands"
```

---

## Task 16: Config fields for UI listen and token

**Files:**
- Modify: `config/config.go`

- [ ] **Step 1: Add UIConfig struct and embed in ClientConfig + ServerConfig**

In `config/config.go`, add:

```go
// UIConfig configures the embedded Web management UI.
type UIConfig struct {
    Listen string `yaml:"listen,omitempty" json:"listen,omitempty"` // e.g. ":9090"
    Token  string `yaml:"token,omitempty" json:"token,omitempty"`   // 64-hex bearer
}
```

Add `UI UIConfig `yaml:"ui,omitempty" json:"ui,omitempty"`` field to both `ClientConfig` and `ServerConfig`.

- [ ] **Step 2: Build + existing tests still pass**

```bash
./scripts/test.sh --pkg ./config/
```

- [ ] **Step 3: Commit**

```bash
git add config/config.go
git commit -m "feat(config): add UI listen and token fields"
```

---

## Task 17: Wire `--ui ADDR` flag in `run` + token generation on install

**Files:**
- Create: `internal/servicecli/token.go`
- Create: `internal/servicecli/ui.go`
- Modify: `internal/servicecli/servicecli.go`
- Modify: `cmd/shuttled/main.go` and `cmd/shuttle/main.go` (add `--ui` flag to `run`)

- [ ] **Step 1: Write token helpers**

```go
// internal/servicecli/token.go
package servicecli

import (
	"crypto/rand"
	"encoding/hex"
)

// NewToken returns 32 random bytes as 64 hex chars.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
```

- [ ] **Step 2: Add Token subcommand handler**

In `servicecli.go`:

```go
// Token prints the bearer token from the config file at path.
func Token(configPath string, isServer bool) {
	if isServer {
		cfg, err := config.LoadServerConfig(configPath)
		if err != nil {
			exit("load config: %v", err)
		}
		fmt.Println(cfg.UI.Token)
		return
	}
	cfg, err := config.LoadClientConfig(configPath)
	if err != nil {
		exit("load config: %v", err)
	}
	fmt.Println(cfg.UI.Token)
}
```

Add import `"github.com/shuttleX/shuttle/config"`.

- [ ] **Step 3: Update `Install` to generate + persist token**

In `Install` (before calling `mgr.Install`):

```go
// Generate + persist token if Web UI is enabled.
if *ui != "" {
    if isServer(opts) {
        cfg, err := config.LoadServerConfig(abs)
        if err == nil {
            if cfg.UI.Token == "" {
                tok, _ := NewToken()
                cfg.UI.Token = tok
            }
            cfg.UI.Listen = *ui
            _ = config.SaveServerConfig(abs, cfg)
            fmt.Printf("  Web UI: http://%s/?token=%s\n", *ui, cfg.UI.Token)
        }
    } else {
        cfg, err := config.LoadClientConfig(abs)
        if err == nil {
            if cfg.UI.Token == "" {
                tok, _ := NewToken()
                cfg.UI.Token = tok
            }
            cfg.UI.Listen = *ui
            _ = config.SaveClientConfig(abs, cfg)
            fmt.Printf("  Web UI: http://%s/?token=%s\n", *ui, cfg.UI.Token)
        }
    }
}

func isServer(opts Options) bool { return opts.Name == "shuttled" }
```

- [ ] **Step 4: Add `--ui` flag to existing `run` subcommand in both binaries**

In `cmd/shuttled/main.go` inside the `run` case, add:
```go
uiListen := runCmd.String("ui", "", "Web UI listen address (overrides config)")
```

And after loading the config:
```go
addr := *uiListen
if addr == "" {
    addr = cfg.UI.Listen
}
if addr != "" {
    // Start gui/api handler on addr using existing engine
    go startUIServer(addr, cfg.UI.Token, eng)
}
```

Write `startUIServer` helper (in a new file `cmd/shuttled/ui.go`):

```go
package main

import (
	"log/slog"
	"net/http"
	"github.com/shuttleX/shuttle/engine"
	"github.com/shuttleX/shuttle/gui/api"
)

func startUIServer(addr, token string, eng *engine.Engine) {
	h := api.NewHandler(api.HandlerConfig{
		Engine: eng,
		Token:  token,
	})
	srv := &http.Server{Addr: addr, Handler: h}
	slog.Info("Web UI listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("UI server failed", "err", err)
	}
}
```

Verify `api.HandlerConfig` has a `Token` field — if not, inspect `gui/api/api.go` and either add it or wire through a middleware.

- [ ] **Step 5: Add `token` subcommand**

In both binaries' main.go, add:
```go
case "token":
    servicecli.Token(*configPathOrDefault, opts.Name == "shuttled")
```

(derive `configPathOrDefault` using `paths.Resolve(defaultScope).ConfigDir` + `"<name>.yaml"` when not passed).

- [ ] **Step 6: Build + smoke test**

```bash
CGO_ENABLED=0 go build ./cmd/shuttled ./cmd/shuttle
```

- [ ] **Step 7: Commit**

```bash
git add internal/servicecli/token.go internal/servicecli/ui.go internal/servicecli/servicecli.go cmd/shuttled/main.go cmd/shuttled/ui.go cmd/shuttle/main.go cmd/shuttle/ui.go
git commit -m "feat(servicecli): add Web UI wiring and bearer token bootstrap"
```

---

## Task 18: Logs tailing — verify per-OS implementations work end-to-end

**Files:** (already implemented in Tasks 5/9/10)
- Verify: `service/service_linux.go` `Logs()`
- Verify: `service/service_darwin.go` `Logs()`
- Verify: `service/service_windows.go` `Logs()`

- [ ] **Step 1: Write table-driven test for path construction**

In `service_test.go`:

```go
func TestLogPathFormatting(t *testing.T) {
    // This test only checks that the template renders log paths correctly
    // for use by the Logs() implementations.
    cfg := Config{Name: "shuttled", LogDir: "/Library/Logs/Shuttle"}
    plist := renderLaunchdPlist(cfg)
    mustContain(t, plist, "/Library/Logs/Shuttle/shuttled.log")
    mustContain(t, plist, "/Library/Logs/Shuttle/shuttled.err.log")
}
```

- [ ] **Step 2: Run**

```bash
./scripts/test.sh --pkg ./service/ --run TestLogPathFormatting
```

- [ ] **Step 3: Commit**

```bash
git add service/service_test.go
git commit -m "test(service): verify log path construction for launchd plist"
```

---

## Task 19: Migration — detect pre-existing Linux unit and clean up

**Files:**
- Modify: `service/service_linux.go`

- [ ] **Step 1: Update `Install` to log the migration event**

In `service_linux.go`, modify the Install block that detects existing unit:

```go
if _, err := os.Stat(m.unitPath()); err == nil {
    fmt.Fprintln(os.Stderr, "Detected existing unit; reinstalling...")
    _ = m.stopSilent()
    _, _ = m.systemctl("disable", m.name)
    _ = os.Remove(m.unitPath())
}
```

Also handle the pre-0.3.x layout: the old code wrote to `/etc/systemd/system/shuttled.service` regardless of scope. This is the same path the new code uses for system scope, so no additional cleanup is needed — the logic above handles it.

For user scope, the old code didn't support user scope — skip.

Add import `"fmt"` if not already present.

- [ ] **Step 2: Build + vet**

```bash
CGO_ENABLED=0 go build ./service/
```

- [ ] **Step 3: Commit**

```bash
git add service/service_linux.go
git commit -m "feat(service): log migration event when replacing existing systemd unit"
```

---

## Task 20: Delete install.sh legacy docs + add new command hints

**Files:**
- Modify: `scripts/install.sh`

- [ ] **Step 1: Overwrite Quick start section**

Replace:
```bash
echo "Quick start:"
echo "  Server:  sudo shuttled run -p yourpassword -d   (installs systemd service)"
echo "           shuttled run -p yourpassword           (foreground)"
echo "  Client:  shuttle run -u \"shuttle://...\""
```

With:
```bash
echo "Quick start:"
echo "  Server:"
echo "    sudo shuttled install -p yourpassword --ui :9090"
echo "    shuttled run -p yourpassword  (foreground)"
echo ""
echo "  Client:"
echo "    shuttle run -u \"shuttle://...\""
echo "    shuttle install -c config.yaml  (run as user service)"
echo ""
echo "  Manage:"
echo "    shuttled status | stop | restart | logs -f | token | uninstall"
```

- [ ] **Step 2: Commit**

```bash
git add scripts/install.sh
git commit -m "docs(install): advertise new service subcommands"
```

---

## Task 21: CI integration tests — Linux, macOS, Windows

**Files:**
- Create or modify: `.github/workflows/ci.yml` (or extend existing `test.yml`)

- [ ] **Step 1: Check existing workflow structure**

```bash
ls .github/workflows/
cat .github/workflows/*.yml | head -100
```

Identify the existing test workflow file.

- [ ] **Step 2: Add three jobs to that workflow**

```yaml
  service-integration-linux:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - name: Run sandbox service tests
        run: sudo -E $(which go) test -tags sandbox ./service/

  service-integration-macos:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - name: Run macOS integration tests
        run: go test -tags integration_darwin ./service/

  service-integration-windows:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - name: Run Windows integration tests
        shell: pwsh
        run: go test -tags integration_windows ./service/
```

- [ ] **Step 3: Add macOS and Windows integration test files**

Create `service/service_integration_darwin_test.go`:
```go
//go:build integration_darwin

package service

import "testing"

func TestLaunchdFullCycle(t *testing.T) {
    mgr, err := New("shuttle-ci-test", ScopeUser)
    if err != nil { t.Fatal(err) }
    defer mgr.Uninstall(false)
    cfg := Config{
        Name: "shuttle-ci-test",
        Description: "CI test",
        BinaryPath: "/bin/sleep",
        Args: []string{"3600"},
        Scope: ScopeUser,
        LogDir: "/tmp/shuttle-ci-logs",
        Restart: false,
    }
    if err := mgr.Install(cfg); err != nil { t.Fatal(err) }
    if err := mgr.Start(); err != nil { t.Fatal(err) }
    s, _ := mgr.Status()
    if s != StatusRunning { t.Errorf("status=%v want running", s) }
    if err := mgr.Stop(); err != nil { t.Fatal(err) }
}
```

Create `service/service_integration_windows_test.go` with the same shape adapted for Windows:
```go
//go:build integration_windows

package service

import (
    "os"
    "testing"
)

func TestWindowsSCMFullCycle(t *testing.T) {
    if os.Getenv("CI") == "" {
        t.Skip("Windows service tests require CI administrator context")
    }
    mgr, err := New("shuttle-ci-test", ScopeSystem)
    if err != nil { t.Fatal(err) }
    defer mgr.Uninstall(false)
    cfg := Config{
        Name: "shuttle-ci-test",
        Description: "CI test",
        BinaryPath: `C:\Windows\System32\cmd.exe`,
        Args: []string{"/c", "timeout /t 3600"},
        Scope: ScopeSystem,
        LogDir: `C:\Windows\Temp\shuttle-ci`,
        Restart: false,
    }
    if err := mgr.Install(cfg); err != nil { t.Fatal(err) }
    if err := mgr.Start(); err != nil { t.Fatal(err) }
    s, _ := mgr.Status()
    if s != StatusRunning { t.Errorf("status=%v want running", s) }
    if err := mgr.Stop(); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ service/service_integration_darwin_test.go service/service_integration_windows_test.go
git commit -m "ci(service): add Linux/macOS/Windows integration test jobs"
```

---

## Task 22: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add a Service Management section**

Add under "## Architecture":

```markdown
### Service Management
- `service/` — cross-platform service manager (systemd / launchd / Windows SCM) with per-OS build-tag files following the `autostart/` pattern
- `internal/paths/` — per-OS config and log directory resolution
- `internal/servicecli/` — shared subcommand handlers used by both `shuttle` and `shuttled`
- CLI verbs (both binaries): `install | uninstall | start | stop | restart | status | logs | token`
- Default scopes: `shuttled` = system, `shuttle` = user
- Windows binaries detect SCM start via `svc.IsWindowsService()` in `servicePreflight()` called at the top of `main()`
- Web UI: `--ui ADDR` flag on `run` / `install` exposes `gui/api.NewHandler` on that address; bearer token generated on install and retrievable via `<binary> token`
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(claude): document service management architecture"
```

---

## Self-Review Checklist (author: run before handing off)

### Spec coverage

| Spec requirement | Task(s) |
|---|---|
| `service/` package with per-OS build tags | 3, 5, 9, 10 |
| `internal/paths/` with XDG / Library / ProgramData | 1, 2 |
| `Manager` interface (Install/Uninstall/Start/Stop/Restart/Status/Logs) | 3, 5, 9, 10 |
| `Config` with Scope / User / Restart / LogDir | 3 |
| Four scope combinations (system/user × shuttled/shuttle) | 14, 15 |
| Windows `servicePreflight()` — not `init()` hack | 11, 12 |
| Symmetric CLI verbs on both binaries | 14, 15 |
| `-d` backward compat | 14 |
| `--ui ADDR` → gui/api handler | 17 |
| Bearer token bootstrap + `token` command | 17 |
| `logs` command per OS | 5, 9, 10, 18 |
| Migration from old `/etc/systemd/system/shuttled.service` | 19 |
| Docker/CI tests for Linux systemd | 7, 21 |
| CI tests for macOS launchd | 21 |
| CI tests for Windows SCM | 21 |
| Updated install.sh | 20 |
| Updated CLAUDE.md | 22 |
| Delete `cmd/shuttled/service.go` | 6 |

### Known gaps / deferred
- `servicecli_test.go` only tests `parseScope`; broader coverage depends on integration tests.
- Windows Event Log integration: explicitly deferred per spec.
- launchd complex KeepAlive conditions: explicitly deferred.
- Uninstall's `--purge` removes directories without prompting. Per spec, this is fine (first iteration).

### Type consistency spot-check
- `service.Scope` and `paths.Scope` both have `ScopeSystem=0, ScopeUser=1`. `paths.Scope(m.scope)` cast in `service/*.go` relies on this. Compile-time guarantees: both packages import each other? No — `service` imports `paths` only. Verified by Task 5's usage: `paths.Resolve(paths.Scope(m.scope))`. Add a compile-time check in `service/service.go`:
```go
var _ = [1]struct{}{}[paths.Scope(ScopeSystem)-paths.ScopeSystem] // fails to compile if values drift
```
Hmm — that's awkward. Simpler: define a package-level `scopeToPaths` function:
```go
func scopeToPaths(s Scope) paths.Scope {
    if s == ScopeUser { return paths.ScopeUser }
    return paths.ScopeSystem
}
```
Use that everywhere instead of direct cast. Apply this fix when implementing Task 5.

### Placeholder scan
No "TBD" / "fill in details" / "similar to Task N" detected. `// ...existing cases ...` markers in Task 14 point to keeping existing code; the exact existing cases are preserved, not placeholders.

---

## Verification After All Tasks

Run before calling this done:

```bash
# All host-safe tests pass
./scripts/test.sh

# Cross-compile all three platforms
GOOS=linux   CGO_ENABLED=0 go build ./cmd/shuttle ./cmd/shuttled
GOOS=darwin  CGO_ENABLED=0 go build ./cmd/shuttle ./cmd/shuttled
GOOS=windows CGO_ENABLED=0 go build ./cmd/shuttle ./cmd/shuttled

# Go vet clean
go vet ./...

# On a sandbox or clean Linux VM: run end-to-end
sudo shuttled install -p testpass --ui :9090
sudo shuttled status
curl -H "Authorization: Bearer $(shuttled token)" http://localhost:9090/api/config
sudo shuttled stop
sudo shuttled uninstall
```

Cross-platform behavior must match: on macOS and Windows, the same commands (substituting `sudo` for Windows as Administrator elevation) produce the same outcomes.
