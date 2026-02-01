//go:build !android && !ios

package tray

import (
	"github.com/shuttle-proxy/shuttle/engine"
)

// Callbacks holds functions the tray can invoke.
type Callbacks struct {
	OnShow       func()
	OnConnect    func()
	OnDisconnect func()
	OnQuit       func()
}

// Run starts the system tray. It blocks until the tray exits.
// This is a placeholder — the actual systray integration requires
// github.com/getlantern/systray which needs CGO on some platforms.
// For now this provides the interface; the real implementation
// can be swapped in when the dependency is added.
func Run(eng *engine.Engine, cb Callbacks) {
	// Placeholder: in production, this would call systray.Run(onReady, onExit)
	// with menu items for Show/Hide, Connect/Disconnect, and Quit.
	//
	// Example with getlantern/systray:
	//   systray.Run(func() {
	//     systray.SetTitle("Shuttle")
	//     mShow := systray.AddMenuItem("Show", "Show window")
	//     mConn := systray.AddMenuItem("Connect", "Connect to server")
	//     mQuit := systray.AddMenuItem("Quit", "Quit Shuttle")
	//     go func() {
	//       for {
	//         select {
	//         case <-mShow.ClickedCh: cb.OnShow()
	//         case <-mConn.ClickedCh: cb.OnConnect()
	//         case <-mQuit.ClickedCh: cb.OnQuit(); systray.Quit()
	//         }
	//       }
	//     }()
	//   }, func() {})
}
