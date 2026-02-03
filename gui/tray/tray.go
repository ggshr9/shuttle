//go:build !android && !ios

package tray

import (
	_ "embed"
	"sync"

	"fyne.io/systray"
	"github.com/shuttle-proxy/shuttle/engine"
)

//go:embed icon_default.png
var iconDefault []byte

//go:embed icon_connected.png
var iconConnected []byte

//go:embed icon_disconnected.png
var iconDisconnected []byte

// Callbacks holds functions the tray can invoke.
type Callbacks struct {
	OnShow       func()
	OnConnect    func()
	OnDisconnect func()
	OnQuit       func()
}

// trayState holds the current tray state for updates.
type trayState struct {
	mu          sync.Mutex
	eng         *engine.Engine
	mConnect    *systray.MenuItem
	mDisconnect *systray.MenuItem
	mStatus     *systray.MenuItem
	running     bool
}

var state *trayState

// Run starts the system tray. It blocks until the tray exits.
func Run(eng *engine.Engine, cb Callbacks) {
	state = &trayState{eng: eng}

	systray.Run(func() {
		// Set initial icon
		if len(iconDefault) > 0 {
			systray.SetIcon(iconDefault)
		}
		systray.SetTitle("Shuttle")
		systray.SetTooltip("Shuttle Proxy - Disconnected")

		// Status display (disabled, just for display)
		state.mStatus = systray.AddMenuItem("⚪ Disconnected", "Current status")
		state.mStatus.Disable()
		systray.AddSeparator()

		mShow := systray.AddMenuItem("Show Window", "Show the main window")
		systray.AddSeparator()

		state.mConnect = systray.AddMenuItem("Connect", "Connect to proxy server")
		state.mDisconnect = systray.AddMenuItem("Disconnect", "Disconnect from server")
		state.mDisconnect.Hide()

		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit Shuttle", "Exit the application")

		// Start status monitor
		go monitorStatus(eng)

		// Event loop
		go func() {
			for {
				select {
				case <-mShow.ClickedCh:
					if cb.OnShow != nil {
						cb.OnShow()
					}
				case <-state.mConnect.ClickedCh:
					if cb.OnConnect != nil {
						cb.OnConnect()
					}
				case <-state.mDisconnect.ClickedCh:
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

// monitorStatus watches engine state and updates tray accordingly.
func monitorStatus(eng *engine.Engine) {
	ch := eng.Subscribe()
	defer eng.Unsubscribe(ch)

	for ev := range ch {
		switch ev.Type {
		case engine.EventConnected:
			updateTrayStatus("running")
		case engine.EventDisconnected:
			updateTrayStatus("stopped")
		}
	}
}

// updateTrayStatus updates tray icon and menu based on connection state.
func updateTrayStatus(status string) {
	if state == nil {
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	switch status {
	case "running":
		if len(iconConnected) > 0 {
			systray.SetIcon(iconConnected)
		}
		systray.SetTooltip("Shuttle Proxy - Connected")
		state.mStatus.SetTitle("🟢 Connected")
		state.mConnect.Hide()
		state.mDisconnect.Show()
		state.running = true

	case "stopped":
		if len(iconDisconnected) > 0 {
			systray.SetIcon(iconDisconnected)
		} else if len(iconDefault) > 0 {
			systray.SetIcon(iconDefault)
		}
		systray.SetTooltip("Shuttle Proxy - Disconnected")
		state.mStatus.SetTitle("⚪ Disconnected")
		state.mConnect.Show()
		state.mDisconnect.Hide()
		state.running = false

	case "starting":
		systray.SetTooltip("Shuttle Proxy - Connecting...")
		state.mStatus.SetTitle("🟡 Connecting...")
		state.mConnect.Hide()
		state.mDisconnect.Hide()

	case "stopping":
		systray.SetTooltip("Shuttle Proxy - Disconnecting...")
		state.mStatus.SetTitle("🟡 Disconnecting...")
		state.mConnect.Hide()
		state.mDisconnect.Hide()
	}
}

// SetConnected manually updates tray to connected state.
func SetConnected() {
	updateTrayStatus("running")
}

// SetDisconnected manually updates tray to disconnected state.
func SetDisconnected() {
	updateTrayStatus("stopped")
}
