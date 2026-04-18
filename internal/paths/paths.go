// Package paths resolves OS-specific paths for Shuttle's configs and logs.
package paths

// Scope identifies whether paths are system-wide or per-user.
type Scope int

const (
	ScopeSystem Scope = iota
	ScopeUser
)

// String returns a human-readable name for the scope.
func (s Scope) String() string {
	switch s {
	case ScopeSystem:
		return "system"
	case ScopeUser:
		return "user"
	default:
		return "unknown"
	}
}

// Paths groups filesystem locations used by Shuttle.
type Paths struct {
	ConfigDir string
	LogDir    string
}

// Resolve returns the paths for the given scope on the current OS.
func Resolve(scope Scope) Paths {
	return resolve(scope)
}
