//go:build windows

package service

import (
	"fmt"
	"os"
	"path/filepath"
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

func (m *windowsManager) Install(cfg *Config) error {
	scMgr, err := m.connect()
	if err != nil {
		return err
	}
	defer scMgr.Disconnect()

	// Remove existing service if present
	if s, err := scMgr.OpenService(m.name); err == nil {
		fmt.Fprintln(os.Stderr, "Detected existing service; reinstalling...")
		_, _ = s.Control(svc.Stop)
		_ = s.Delete()
		s.Close()
		time.Sleep(500 * time.Millisecond) // let SCM settle
	}

	svcMgrCfg := mgr.Config{
		DisplayName:      cfg.DisplayName,
		Description:      cfg.Description,
		StartType:        mgr.StartAutomatic,
		ServiceStartName: cfg.User, // empty = LocalSystem
	}
	s, err := scMgr.CreateService(m.name, cfg.BinaryPath, svcMgrCfg, cfg.Args...)
	if err != nil {
		return fmt.Errorf("CreateService: %w", err)
	}
	defer s.Close()

	if cfg.Restart {
		sec := cfg.RestartSec
		if sec == 0 {
			sec = 5
		}
		recover := []mgr.RecoveryAction{
			{Type: mgr.ServiceRestart, Delay: time.Duration(sec) * time.Second},
			{Type: mgr.ServiceRestart, Delay: time.Duration(sec) * time.Second},
			{Type: mgr.NoAction, Delay: 0},
		}
		if err := s.SetRecoveryActions(recover, 86400); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not configure auto-restart: %v\n", err)
		}
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

	if s, err := scMgr.OpenService(m.name); err == nil {
		_, _ = s.Control(svc.Stop)
		time.Sleep(500 * time.Millisecond)
		_ = s.Delete()
		s.Close()
	}

	if purge {
		p := paths.Resolve(scopeToPaths(m.scope))
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
	if err := m.Stop(); err != nil && err != ErrNotInstalled {
		return fmt.Errorf("restart: stop failed: %w", err)
	}
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

// Logs streams the file log written by the service handler.
// On Windows we tail the file manually rather than shelling out to a pager.
func (m *windowsManager) Logs(follow bool) error {
	logDir := paths.Resolve(scopeToPaths(m.scope)).LogDir
	logFile := filepath.Join(logDir, m.name+".log")
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		return fmt.Errorf("log file not found: %s", logFile)
	}
	data, _ := os.ReadFile(logFile)
	os.Stdout.Write(data)
	if !follow {
		return nil
	}
	offset := int64(len(data))
	for {
		time.Sleep(500 * time.Millisecond)
		info, err := os.Stat(logFile)
		if err != nil {
			return err
		}
		if info.Size() > offset {
			f, err := os.Open(logFile)
			if err != nil {
				return err
			}
			_, _ = f.Seek(offset, 0)
			buf := make([]byte, info.Size()-offset)
			n, _ := f.Read(buf)
			f.Close()
			os.Stdout.Write(buf[:n])
			offset = info.Size()
		}
	}
}
