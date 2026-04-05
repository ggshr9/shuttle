package engine

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestProactiveMigrator_ProbeSuccess(t *testing.T) {
	var migrated atomic.Bool
	var emitted atomic.Bool

	pm := NewProactiveMigrator(
		ProactiveMigratorConfig{
			Enabled: true,
			Timeout: 1 * time.Second,
		},
		func(ctx context.Context) error {
			return nil // probe succeeds
		},
		func() {
			migrated.Store(true)
		},
		slog.Default(),
		func(ev Event) {
			if ev.Type == EventProactiveMigration {
				emitted.Store(true)
			}
		},
	)

	pm.OnNetworkChange(context.Background())

	if !migrated.Load() {
		t.Error("expected migration to be triggered on successful probe")
	}
	if !emitted.Load() {
		t.Error("expected proactive migration event to be emitted")
	}
}

func TestProactiveMigrator_ProbeFailure(t *testing.T) {
	var migrated atomic.Bool

	pm := NewProactiveMigrator(
		ProactiveMigratorConfig{
			Enabled: true,
			Timeout: 1 * time.Second,
		},
		func(ctx context.Context) error {
			return errors.New("connection refused")
		},
		func() {
			migrated.Store(true)
		},
		slog.Default(),
		func(ev Event) {},
	)

	pm.OnNetworkChange(context.Background())

	if migrated.Load() {
		t.Error("expected migration NOT to be triggered on failed probe")
	}
}

func TestProactiveMigrator_Disabled(t *testing.T) {
	var migrated atomic.Bool

	pm := NewProactiveMigrator(
		ProactiveMigratorConfig{
			Enabled: false,
			Timeout: 1 * time.Second,
		},
		func(ctx context.Context) error {
			return nil
		},
		func() {
			migrated.Store(true)
		},
		slog.Default(),
		func(ev Event) {},
	)

	pm.OnNetworkChange(context.Background())

	if migrated.Load() {
		t.Error("disabled migrator should not trigger migration")
	}
}

func TestProactiveMigrator_NilProbeFn(t *testing.T) {
	// Should not panic when probeFn is nil.
	pm := NewProactiveMigrator(
		ProactiveMigratorConfig{Enabled: true},
		nil,
		func() {},
		slog.Default(),
		func(ev Event) {},
	)

	pm.OnNetworkChange(context.Background())
	// No panic = pass.
}

func TestProactiveMigrator_DefaultTimeout(t *testing.T) {
	pm := NewProactiveMigrator(
		ProactiveMigratorConfig{Enabled: true},
		func(ctx context.Context) error { return nil },
		func() {},
		nil,
		nil,
	)

	if pm.timeout != defaultProbeTimeout {
		t.Errorf("expected default timeout %v, got %v", defaultProbeTimeout, pm.timeout)
	}
}

func TestProactiveMigrator_ProbeRespectsTimeout(t *testing.T) {
	var migrated atomic.Bool

	pm := NewProactiveMigrator(
		ProactiveMigratorConfig{
			Enabled: true,
			Timeout: 50 * time.Millisecond,
		},
		func(ctx context.Context) error {
			// Simulate a slow probe that exceeds the timeout.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				return nil
			}
		},
		func() {
			migrated.Store(true)
		},
		slog.Default(),
		func(ev Event) {},
	)

	pm.OnNetworkChange(context.Background())

	if migrated.Load() {
		t.Error("expected migration NOT to be triggered when probe times out")
	}
}
