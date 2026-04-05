# Sub-2: Smart Connection Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** When network changes (WiFi→cellular), proactively dial on the new network before switching, achieving zero-downtime migration. Feature is opt-in (default off).

**Architecture:** ProactiveMigrator listens to netmon.OnChange, immediately attempts a probe dial on the new network. If successful, triggers Selector migration to drain old connections gracefully. If probe fails, stays on current connection.

**Tech Stack:** Go 1.24+, existing netmon.Monitor, Selector.Migrate(), transport probing

---

## File Structure

- Create: `engine/proactive_migrate.go` — ProactiveMigrator logic
- Create: `engine/proactive_migrate_test.go` — tests
- Modify: `config/config_transport.go` — add migration config fields
- Modify: `engine/engine_lifecycle.go` — wire ProactiveMigrator into netmon callback
- Modify: `engine/engine.go` — store ProactiveMigrator

---

## Task 1: Config Fields

**Files:**
- Modify: `config/config_transport.go`

- [ ] **Step 1: Add migration fields**

```go
type TransportConfig struct {
    // ... existing fields ...
    ProactiveMigration   bool   `yaml:"proactive_migration" json:"proactive_migration"`       // default false
    MigrationProbeTimeout string `yaml:"migration_probe_timeout" json:"migration_probe_timeout"` // default "3s"
}
```

- [ ] **Step 2: Commit**

Commit: `feat(config): add proactive_migration transport config fields`

---

## Task 2: ProactiveMigrator

**Files:**
- Create: `engine/proactive_migrate.go`
- Create: `engine/proactive_migrate_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestProactiveMigrator_ProbeSuccess(t *testing.T) {
    // Mock selector that succeeds on probe
    // Call OnNetworkChange
    // Verify migrate was called
}

func TestProactiveMigrator_ProbeFailure(t *testing.T) {
    // Mock selector that fails on probe
    // Call OnNetworkChange
    // Verify migrate was NOT called
}

func TestProactiveMigrator_Disabled(t *testing.T) {
    // Create with enabled=false
    // OnNetworkChange should be no-op
}
```

- [ ] **Step 2: Implement ProactiveMigrator**

```go
// engine/proactive_migrate.go
package engine

import (
    "context"
    "log/slog"
    "time"

    "github.com/shuttleX/shuttle/transport/selector"
)

// ProactiveMigrator performs proactive connection migration on network changes.
// When enabled, it immediately probes the new network and migrates if successful.
type ProactiveMigrator struct {
    enabled    bool
    sel        *selector.Selector
    serverAddr string
    timeout    time.Duration
    logger     *slog.Logger
    emitFn     func(Event)
}

// ProactiveMigratorConfig configures the proactive migrator.
type ProactiveMigratorConfig struct {
    Enabled    bool
    ServerAddr string
    Timeout    time.Duration // default 3s
}

// NewProactiveMigrator creates a ProactiveMigrator.
func NewProactiveMigrator(sel *selector.Selector, cfg ProactiveMigratorConfig, logger *slog.Logger, emitFn func(Event)) *ProactiveMigrator {
    timeout := cfg.Timeout
    if timeout == 0 {
        timeout = 3 * time.Second
    }
    return &ProactiveMigrator{
        enabled:    cfg.Enabled,
        sel:        sel,
        serverAddr: cfg.ServerAddr,
        timeout:    timeout,
        logger:     logger,
        emitFn:     emitFn,
    }
}

// OnNetworkChange is called by netmon.Monitor when a network change is detected.
// If enabled, it probes the new network and triggers migration on success.
func (pm *ProactiveMigrator) OnNetworkChange(ctx context.Context) {
    if !pm.enabled {
        return
    }
    if pm.sel == nil {
        return
    }

    pm.logger.Info("proactive migration: network change detected, probing new network")

    probeCtx, cancel := context.WithTimeout(ctx, pm.timeout)
    defer cancel()

    // Probe dial on the new network
    conn, err := pm.sel.Dial(probeCtx, pm.serverAddr)
    if err != nil {
        pm.logger.Info("proactive migration: probe failed, keeping current connection", "err", err)
        if pm.emitFn != nil {
            pm.emitFn(Event{Type: EventNetworkChange, Message: "migration probe failed: " + err.Error()})
        }
        return
    }
    conn.Close() // probe succeeded, close probe connection

    // Trigger migration
    pm.logger.Info("proactive migration: probe succeeded, migrating to new network")
    pm.sel.Migrate()

    if pm.emitFn != nil {
        pm.emitFn(Event{Type: EventNetworkChange, Message: "proactive migration: switched to new network"})
    }
}
```

- [ ] **Step 3: Run tests, commit**

Run: `./scripts/test.sh --run TestProactiveMigrator --pkg ./engine/`
Commit: `feat(engine): add ProactiveMigrator for zero-downtime network switching`

---

## Task 3: Wire into Engine

**Files:**
- Modify: `engine/engine.go` — add proactiveMigrator field
- Modify: `engine/engine_lifecycle.go` — create and wire into netmon callback

- [ ] **Step 1: Add field to Engine**

```go
proactiveMigrator *ProactiveMigrator
```

- [ ] **Step 2: Wire in startInternal**

After selector and netmon are created, build ProactiveMigrator and register as netmon callback:

```go
// Parse migration timeout
migrateTimeout := 3 * time.Second
if cfgSnap.Transport.MigrationProbeTimeout != "" {
    if d, err := time.ParseDuration(cfgSnap.Transport.MigrationProbeTimeout); err == nil {
        migrateTimeout = d
    }
}

pm := NewProactiveMigrator(sel, ProactiveMigratorConfig{
    Enabled:    cfgSnap.Transport.ProactiveMigration,
    ServerAddr: cfgSnap.Server.Addr,
    Timeout:    migrateTimeout,
}, e.logger, e.obs.Emit)

e.proactiveMigrator = pm

// Update netmon callback to include proactive migration
nm.OnChange(func() {
    e.logger.Info("network change detected")
    e.obs.Emit(Event{Type: EventNetworkChange, Message: "network change detected"})
    pm.OnNetworkChange(ctx)
})
```

- [ ] **Step 3: Add Selector.Migrate() method if missing**

Check if `Selector` has a `Migrate()` method. If not, add one that calls `migrator.Migrate()`.

- [ ] **Step 4: Run all tests, commit**

Run: `./scripts/test.sh`
Commit: `feat(engine): wire ProactiveMigrator into network change detection`

---

## Config Example

```yaml
transport:
  proactive_migration: true    # opt-in, default false
  migration_probe_timeout: 3s  # probe dial timeout
```
