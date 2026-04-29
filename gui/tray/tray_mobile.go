//go:build android || ios

package tray

import "github.com/ggshr9/shuttle/engine"

// Callbacks holds functions the tray can invoke.
// On mobile, the tray is a no-op.
type Callbacks struct {
	OnShow       func()
	OnConnect    func()
	OnDisconnect func()
	OnQuit       func()
}

// Run is a no-op on mobile platforms.
func Run(_ *engine.Engine, _ Callbacks) {}
