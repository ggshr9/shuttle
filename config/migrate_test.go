package config

import (
	"testing"
)

func TestMigrateClientConfig_NoMigrations(t *testing.T) {
	raw := map[string]any{
		"server": map[string]any{"addr": "example.com:443"},
	}
	applied, err := MigrateClientConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied != 0 {
		t.Fatalf("expected 0 migrations applied, got %d", applied)
	}
	if raw["version"] != CurrentClientConfigVersion {
		t.Fatalf("expected version %d, got %v", CurrentClientConfigVersion, raw["version"])
	}
}

func TestMigrateServerConfig_NoMigrations(t *testing.T) {
	raw := map[string]any{
		"listen": ":443",
	}
	applied, err := MigrateServerConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied != 0 {
		t.Fatalf("expected 0 migrations applied, got %d", applied)
	}
	if raw["version"] != CurrentServerConfigVersion {
		t.Fatalf("expected version %d, got %v", CurrentServerConfigVersion, raw["version"])
	}
}

func TestMigrateConfig_FutureVersion(t *testing.T) {
	raw := map[string]any{
		"version": 999,
	}
	_, err := MigrateClientConfig(raw)
	if err == nil {
		t.Fatal("expected error for future version, got nil")
	}
}

func TestMigrateConfig_MissingVersion(t *testing.T) {
	raw := map[string]any{}
	applied, err := MigrateClientConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied != 0 {
		t.Fatalf("expected 0 migrations applied, got %d", applied)
	}
	if raw["version"] != CurrentClientConfigVersion {
		t.Fatalf("expected version %d, got %v", CurrentClientConfigVersion, raw["version"])
	}
}

func TestApplyMigrations_WithMigration(t *testing.T) {
	migrationRan := false
	migrations := []Migration{
		{
			FromVersion: 0,
			Description: "test migration: add default_field",
			Migrate: func(raw map[string]any) error {
				migrationRan = true
				raw["default_field"] = "migrated"
				return nil
			},
		},
	}

	raw := map[string]any{}
	applied, err := applyMigrations(raw, migrations, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !migrationRan {
		t.Fatal("expected migration to run")
	}
	if applied != 1 {
		t.Fatalf("expected 1 migration applied, got %d", applied)
	}
	if raw["version"] != 1 {
		t.Fatalf("expected version 1, got %v", raw["version"])
	}
	if raw["default_field"] != "migrated" {
		t.Fatalf("expected default_field to be 'migrated', got %v", raw["default_field"])
	}
}
