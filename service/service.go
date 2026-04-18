// Package service provides cross-platform system service management
// (install, start, stop, status, uninstall) for shuttled and shuttle.
package service

import (
	"errors"

	ipaths "github.com/shuttleX/shuttle/internal/paths"
)

// Scope identifies whether the service is system-wide or per-user.
type Scope int

const (
	// ScopeSystem installs a system service (root / LocalSystem).
	ScopeSystem Scope = iota
	// ScopeUser installs a per-user service (no elevation required).
	ScopeUser
)

// Status describes a service's current lifecycle state.
type Status int

const (
	StatusUnknown Status = iota
	StatusNotInstalled
	StatusStopped
	StatusRunning
)

// String returns a human-readable name for the status.
func (s Status) String() string {
	switch s {
	case StatusRunning:
		return "running"
	case StatusStopped:
		return "stopped"
	case StatusNotInstalled:
		return "not-installed"
	default:
		return "unknown"
	}
}

// Config fully describes a service to install.
type Config struct {
	Name        string   // "shuttled" or "shuttle"
	DisplayName string   // e.g. "Shuttle Server"
	Description string
	BinaryPath  string   // absolute, EvalSymlinks-resolved
	Args        []string // e.g. ["run", "-c", "/etc/shuttle/server.yaml"]
	Scope       Scope
	User        string // empty = root / LocalSystem / current-user
	Restart     bool   // auto-restart on crash; default true at caller
	RestartSec  int    // default 5 at caller
	LimitNOFILE int    // systemd only; 0 = unset
	LogDir      string // where <name>.log / .err.log live
}

// Manager performs service lifecycle operations on the current OS.
type Manager interface {
	Install(cfg *Config) error
	Uninstall(purge bool) error
	Start() error
	Stop() error
	Restart() error
	Status() (Status, error)
	Logs(follow bool) error
}

// Typed errors returned by Manager implementations.
var (
	ErrUnsupported      = errors.New("service: unsupported platform")
	ErrNotInstalled     = errors.New("service: not installed")
	ErrAlreadyInstalled = errors.New("service: already installed")
	ErrPermission       = errors.New("service: permission denied (run with sudo / as Administrator)")
)

// New returns a Manager for the given service name and scope on the current OS.
func New(name string, scope Scope) (Manager, error) {
	return newManager(name, scope)
}

// scopeToPaths maps the local Scope to internal/paths.Scope.
// Kept as a helper so downstream code never does direct type casts that could
// silently drift if either enum adds a value.
func scopeToPaths(s Scope) ipaths.Scope {
	if s == ScopeUser {
		return ipaths.ScopeUser
	}
	return ipaths.ScopeSystem
}
