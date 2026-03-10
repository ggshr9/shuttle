package config

import "fmt"

// Migration represents a config migration from one version to the next.
type Migration struct {
	FromVersion int
	Description string
	Migrate     func(raw map[string]any) error
}

// clientMigrations is the ordered list of client config migrations.
var clientMigrations = []Migration{
	// Future migrations go here. Example:
	// {FromVersion: 1, Description: "rename foo to bar", Migrate: func(raw map[string]any) error { ... }},
}

// serverMigrations is the ordered list of server config migrations.
var serverMigrations = []Migration{}

// MigrateClientConfig applies migrations to bring a raw config map up to the current version.
func MigrateClientConfig(raw map[string]any) (int, error) {
	return applyMigrations(raw, clientMigrations, CurrentClientConfigVersion)
}

// MigrateServerConfig applies migrations to bring a raw config map up to the current version.
func MigrateServerConfig(raw map[string]any) (int, error) {
	return applyMigrations(raw, serverMigrations, CurrentServerConfigVersion)
}

func applyMigrations(raw map[string]any, migrations []Migration, currentVersion int) (int, error) {
	version := 0
	if v, ok := raw["version"]; ok {
		switch vt := v.(type) {
		case int:
			version = vt
		case float64:
			version = int(vt)
		}
	}

	if version > currentVersion {
		return version, fmt.Errorf("config version %d is newer than supported version %d", version, currentVersion)
	}

	applied := 0
	for _, m := range migrations {
		if m.FromVersion >= version && m.FromVersion < currentVersion {
			if err := m.Migrate(raw); err != nil {
				return version, fmt.Errorf("migration from v%d failed: %w", m.FromVersion, err)
			}
			applied++
		}
	}

	raw["version"] = currentVersion
	return applied, nil
}
