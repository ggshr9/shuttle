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

func (m *darwinManager) Install(cfg *Config) error {
	if cfg.LogDir == "" {
		cfg.LogDir = paths.Resolve(scopeToPaths(m.scope)).LogDir
	}
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		if os.IsPermission(err) {
			return ErrPermission
		}
		return fmt.Errorf("mkdir logs %s: %w", cfg.LogDir, err)
	}
	if err := os.MkdirAll(filepath.Dir(m.plistPath()), 0755); err != nil {
		if os.IsPermission(err) {
			return ErrPermission
		}
		return fmt.Errorf("mkdir plist dir: %w", err)
	}
	if _, err := os.Stat(m.plistPath()); err == nil {
		fmt.Fprintln(os.Stderr, "Detected existing plist; reinstalling...")
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
		p := paths.Resolve(scopeToPaths(m.scope))
		_ = os.RemoveAll(p.ConfigDir)
		_ = os.RemoveAll(p.LogDir)
	}
	return nil
}

func (m *darwinManager) Start() error {
	if _, err := os.Stat(m.plistPath()); os.IsNotExist(err) {
		return ErrNotInstalled
	}
	// If the job is not loaded, bootstrap re-registers and starts it (RunAtLoad=true).
	// If already loaded, bootstrap fails harmlessly and we fall through to kickstart.
	if _, err := m.launchctl("bootstrap", m.domain(), m.plistPath()); err == nil {
		return nil
	}
	// Already loaded — kickstart if stopped.
	out, err := m.launchctl("kickstart", m.serviceTarget())
	if err != nil {
		return fmt.Errorf("kickstart: %s", string(out))
	}
	return nil
}

// Stop requests a graceful shutdown via SIGTERM and then unloads the service
// so that KeepAlive does not restart it. Subsequent Start() must re-bootstrap
// the plist — Install already handles that flow.
func (m *darwinManager) Stop() error {
	_, _ = m.launchctl("kill", "SIGTERM", m.serviceTarget())
	_, _ = m.launchctl("bootout", m.domain(), m.plistPath())
	return nil
}

func (m *darwinManager) Restart() error {
	if _, err := os.Stat(m.plistPath()); os.IsNotExist(err) {
		return ErrNotInstalled
	}
	// If the job is not loaded (e.g. after Stop), bootstrap re-registers and starts it.
	if _, err := m.launchctl("bootstrap", m.domain(), m.plistPath()); err == nil {
		return nil
	}
	// Already loaded — force-kick the running service.
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
		// `launchctl print` exits non-zero if the service is not loaded.
		return StatusStopped, nil
	}
	if strings.Contains(string(out), "state = running") {
		return StatusRunning, nil
	}
	return StatusStopped, nil
}

func (m *darwinManager) Logs(follow bool) error {
	logDir := paths.Resolve(scopeToPaths(m.scope)).LogDir
	outLog := filepath.Join(logDir, m.name+".log")
	errLog := filepath.Join(logDir, m.name+".err.log")
	args := []string{"-n", "200"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, outLog, errLog)
	cmd := exec.Command("tail", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
