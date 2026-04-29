//go:build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ggshr9/shuttle/internal/paths"
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
		return filepath.Join("/etc/systemd/system", m.name+".service") //nolint:gocritic
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

func (m *linuxManager) Install(cfg *Config) error {
	if _, err := os.Stat(m.unitPath()); err == nil {
		fmt.Fprintln(os.Stderr, "Detected existing unit; reinstalling...")
		_, _ = m.systemctl("stop", m.name)
		_, _ = m.systemctl("disable", m.name)
		_ = os.Remove(m.unitPath())
	}
	if err := os.MkdirAll(filepath.Dir(m.unitPath()), 0755); err != nil {
		if os.IsPermission(err) {
			return ErrPermission
		}
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(m.unitPath()), err)
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
	_, _ = m.systemctl("stop", m.name)
	_, _ = m.systemctl("disable", m.name)
	_ = os.Remove(m.unitPath())
	_, _ = m.systemctl("daemon-reload")
	if purge {
		p := paths.Resolve(scopeToPaths(m.scope))
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
	args := []string{"--no-pager"}
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
