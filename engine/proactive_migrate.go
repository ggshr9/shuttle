package engine

import (
	"context"
	"log/slog"
	"time"
)

const defaultProbeTimeout = 3 * time.Second

// ProactiveMigratorConfig holds configuration for proactive network migration.
type ProactiveMigratorConfig struct {
	Enabled    bool
	ServerAddr string
	Timeout    time.Duration // default 3s
}

// ProactiveMigrator probes the new network on change and triggers transport
// migration if the probe succeeds. This enables zero-downtime network switching
// (e.g., WiFi to cellular).
type ProactiveMigrator struct {
	enabled   bool
	timeout   time.Duration
	logger    *slog.Logger
	emitFn    func(Event)
	probeFn   func(ctx context.Context) error // probe the server on the new network
	migrateFn func()                          // trigger transport migration
}

// NewProactiveMigrator creates a ProactiveMigrator. The probeFn should attempt
// a connection to the server and return nil on success. The migrateFn should
// trigger a transport migration (e.g., Selector.Migrate). Both functions are
// injectable for testability.
func NewProactiveMigrator(
	cfg ProactiveMigratorConfig,
	probeFn func(ctx context.Context) error,
	migrateFn func(),
	logger *slog.Logger,
	emitFn func(Event),
) *ProactiveMigrator {
	if logger == nil {
		logger = slog.Default()
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	return &ProactiveMigrator{
		enabled:   cfg.Enabled,
		timeout:   timeout,
		logger:    logger,
		emitFn:    emitFn,
		probeFn:   probeFn,
		migrateFn: migrateFn,
	}
}

// OnNetworkChange is called by netmon when a network interface change is
// detected. If enabled, it probes the server on the new network path and
// triggers migration on success.
func (pm *ProactiveMigrator) OnNetworkChange(ctx context.Context) {
	if !pm.enabled || pm.probeFn == nil {
		return
	}

	pm.logger.Info("proactive migration: probing new network")

	probeCtx, cancel := context.WithTimeout(ctx, pm.timeout)
	defer cancel()

	if err := pm.probeFn(probeCtx); err != nil {
		pm.logger.Warn("proactive migration: probe failed, keeping current path",
			"err", err)
		return
	}

	pm.logger.Info("proactive migration: probe succeeded, triggering migration")
	if pm.migrateFn != nil {
		pm.migrateFn()
	}
	if pm.emitFn != nil {
		pm.emitFn(Event{
			Type:    EventProactiveMigration,
			Message: "proactive migration triggered after network change",
		})
	}
}
