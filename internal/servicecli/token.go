package servicecli

import (
	"fmt"
	"os"

	"github.com/ggshr9/shuttle/config"
	"github.com/ggshr9/shuttle/gui/api"
)

// Token prints the bearer token from the appropriate config field.
// isServer selects: server → cfg.Admin.Token; client → cfg.UI.Token.
func Token(configPath string, isServer bool) {
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "token requires a config path (use -c)")
		os.Exit(1)
	}
	if isServer {
		cfg, err := config.LoadServerConfig(configPath)
		if err != nil {
			exit("load config: %v", err)
		}
		fmt.Println(cfg.Admin.Token)
		return
	}
	cfg, err := config.LoadClientConfig(configPath)
	if err != nil {
		exit("load config: %v", err)
	}
	fmt.Println(cfg.UI.Token)
}

// ensureUIToken enables the web management UI for the given config.
// Server:  sets cfg.Admin.{Enabled=true, Listen, Token}.
// Client:  sets cfg.UI.{Listen, Token}.
// Generates a fresh token if none exists. Persists only if fields changed.
// Returns the token so the caller can print the URL.
func ensureUIToken(configPath, listen string, isServer bool) (string, error) {
	if isServer {
		cfg, err := config.LoadServerConfig(configPath)
		if err != nil {
			return "", fmt.Errorf("load: %w", err)
		}
		changed := false
		if !cfg.Admin.Enabled {
			cfg.Admin.Enabled = true
			changed = true
		}
		if cfg.Admin.Listen != listen {
			cfg.Admin.Listen = listen
			changed = true
		}
		if cfg.Admin.Token == "" {
			tok, err := api.GenerateAuthToken()
			if err != nil {
				return "", fmt.Errorf("generate token: %w", err)
			}
			cfg.Admin.Token = tok
			changed = true
		}
		if changed {
			if err := config.SaveServerConfig(configPath, cfg); err != nil {
				return "", fmt.Errorf("save: %w", err)
			}
		}
		return cfg.Admin.Token, nil
	}
	// Client path
	cfg, err := config.LoadClientConfig(configPath)
	if err != nil {
		return "", fmt.Errorf("load: %w", err)
	}
	changed := false
	if cfg.UI.Listen != listen {
		cfg.UI.Listen = listen
		changed = true
	}
	if cfg.UI.Token == "" {
		tok, err := api.GenerateAuthToken()
		if err != nil {
			return "", fmt.Errorf("generate token: %w", err)
		}
		cfg.UI.Token = tok
		changed = true
	}
	if changed {
		if err := config.SaveClientConfig(configPath, cfg); err != nil {
			return "", fmt.Errorf("save: %w", err)
		}
	}
	return cfg.UI.Token, nil
}
