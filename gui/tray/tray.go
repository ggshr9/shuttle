//go:build !android && !ios

package tray

import (
	"fyne.io/systray"
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
func Run(eng *engine.Engine, cb Callbacks) {
	systray.Run(func() {
		systray.SetTitle("Shuttle")
		systray.SetTooltip("Shuttle Proxy")

		mShow := systray.AddMenuItem("Show", "Show window")
		systray.AddSeparator()
		mConnect := systray.AddMenuItem("Connect", "Connect to server")
		mDisconnect := systray.AddMenuItem("Disconnect", "Disconnect from server")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Quit Shuttle")

		go func() {
			for {
				select {
				case <-mShow.ClickedCh:
					if cb.OnShow != nil {
						cb.OnShow()
					}
				case <-mConnect.ClickedCh:
					if cb.OnConnect != nil {
						cb.OnConnect()
					}
				case <-mDisconnect.ClickedCh:
					if cb.OnDisconnect != nil {
						cb.OnDisconnect()
					}
				case <-mQuit.ClickedCh:
					if cb.OnQuit != nil {
						cb.OnQuit()
					}
					systray.Quit()
					return
				}
			}
		}()
	}, func() {})
}
