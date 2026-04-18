package servicecli

import (
	"fmt"
	"os"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/gui/api"
)

// Token prints the bearer token from the config file at configPath.
// isServer selects between ServerConfig and ClientConfig loaders.
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
		fmt.Println(cfg.UI.Token)
		return
	}
	cfg, err := config.LoadClientConfig(configPath)
	if err != nil {
		exit("load config: %v", err)
	}
	fmt.Println(cfg.UI.Token)
}

// ensureUIToken sets cfg.UI.Listen and cfg.UI.Token on the given config file at path.
// Generates a new token if none exists. Persists changes if either field was updated.
// isServer selects loader/saver; returns the token string (for printing).
func ensureUIToken(configPath, listen string, isServer bool) (string, error) {
	if isServer {
		cfg, err := config.LoadServerConfig(configPath)
		if err != nil {
			return "", fmt.Errorf("load: %w", err)
		}
		changed := false
		if cfg.UI.Token == "" {
			tok, err := api.GenerateAuthToken()
			if err != nil {
				return "", fmt.Errorf("generate token: %w", err)
			}
			cfg.UI.Token = tok
			changed = true
		}
		if cfg.UI.Listen != listen {
			cfg.UI.Listen = listen
			changed = true
		}
		if changed {
			if err := config.SaveServerConfig(configPath, cfg); err != nil {
				return "", fmt.Errorf("save: %w", err)
			}
		}
		return cfg.UI.Token, nil
	}
	cfg, err := config.LoadClientConfig(configPath)
	if err != nil {
		return "", fmt.Errorf("load: %w", err)
	}
	changed := false
	if cfg.UI.Token == "" {
		tok, err := api.GenerateAuthToken()
		if err != nil {
			return "", fmt.Errorf("generate token: %w", err)
		}
		cfg.UI.Token = tok
		changed = true
	}
	if cfg.UI.Listen != listen {
		cfg.UI.Listen = listen
		changed = true
	}
	if changed {
		if err := config.SaveClientConfig(configPath, cfg); err != nil {
			return "", fmt.Errorf("save: %w", err)
		}
	}
	return cfg.UI.Token, nil
}
