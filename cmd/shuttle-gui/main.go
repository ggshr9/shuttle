package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/connlog"
	"github.com/shuttleX/shuttle/engine"
	"github.com/shuttleX/shuttle/gui"
	"github.com/shuttleX/shuttle/gui/api"
	"github.com/shuttleX/shuttle/gui/tray"
	"github.com/shuttleX/shuttle/speedtest"
	"github.com/shuttleX/shuttle/stats"
	"github.com/shuttleX/shuttle/subscription"
)

// App wraps the engine for Wails bindings.
type App struct {
	ctx        context.Context
	eng        *engine.Engine
	srv        *api.Server
	statsStore *stats.Storage
	connStore  *connlog.Storage
	apiHandler http.Handler
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Start internal API server using the shared handler for REST + WebSocket endpoints.
	a.srv = api.NewServerWithHandler(a.eng, nil, a.apiHandler)
	addr, err := a.srv.ListenAndServe("127.0.0.1:0")
	if err != nil {
		log.Printf("API server failed: %v", err)
		return
	}
	log.Printf("API server at http://%s", addr)
}

func (a *App) shutdown(ctx context.Context) {
	_ = a.eng.Stop()
	if a.srv != nil {
		a.srv.Close()
	}
	if a.statsStore != nil {
		a.statsStore.Close()
	}
	if a.connStore != nil {
		a.connStore.Close()
	}
}

// Connect starts the engine.
func (a *App) Connect() error {
	return a.eng.Start(a.ctx)
}

// Disconnect stops the engine.
func (a *App) Disconnect() error {
	return a.eng.Stop()
}

// Status returns the engine status.
func (a *App) Status() engine.EngineStatus {
	return a.eng.Status()
}

// GetConfig returns the current config.
func (a *App) GetConfig() config.ClientConfig {
	return a.eng.Config()
}

// SetConfig reloads with new config.
func (a *App) SetConfig(cfg config.ClientConfig) error { //nolint:gocritic // hugeParam: Wails binding requires value type
	return a.eng.Reload(&cfg)
}

func main() {
	var configPath string
	for i, arg := range os.Args[1:] {
		if arg == "-c" && i+1 < len(os.Args[1:]) {
			configPath = os.Args[i+2]
		}
	}

	var cfg *config.ClientConfig
	if configPath != "" {
		var err error
		cfg, err = config.LoadClientConfig(configPath)
		if err != nil {
			log.Printf("Failed to load config %s: %v, using defaults", configPath, err)
			cfg = config.DefaultClientConfig()
		}
	} else {
		cfg = config.DefaultClientConfig()
	}

	eng := engine.New(cfg)
	app := &App{eng: eng}

	// Initialize stats storage
	dataDir := filepath.Join(os.Getenv("HOME"), ".shuttle")
	statsStore, err := stats.NewStorage(dataDir)
	if err != nil {
		log.Printf("Failed to initialize stats storage: %v", err)
	} else {
		app.statsStore = statsStore
	}

	// Initialize connection log storage
	connStore, err := connlog.NewStorage(filepath.Join(dataDir, "connlog"), 512)
	if err != nil {
		log.Printf("Failed to initialize connlog storage: %v", err)
	} else {
		app.connStore = connStore
	}

	// Initialize speedtest history storage
	stHistory := speedtest.NewHistoryStorage(dataDir)

	// Initialize subscription manager
	subMgr := subscription.NewManager()
	if len(cfg.Subscriptions) > 0 {
		subMgr.LoadFromConfig(cfg.Subscriptions)
	}

	// Generate auth token for API security — all /api/ endpoints require Bearer auth.
	authToken, err := api.GenerateAuthToken()
	if err != nil {
		log.Fatalf("Failed to generate auth token: %v", err)
	}
	log.Printf("API auth token: %s", authToken)

	// Build one shared API handler for both the Wails asset handler and standalone server.
	sharedHandler := api.NewHandler(api.HandlerConfig{
		Engine:       eng,
		SubMgr:       subMgr,
		Stats:        statsStore,
		ConnLog:      connStore,
		SpeedHistory: stHistory,
		AuthToken:    authToken,
	})
	app.apiHandler = sharedHandler

	// Wire stats recording and connlog: subscribe to engine events.
	go recordEngineEvents(eng, statsStore, connStore)

	// Embedded SPA assets — fail fast if the bundle wasn't built before go build.
	webFS, err := fs.Sub(gui.WebAssets, "web/dist")
	if err != nil {
		log.Fatalf("Failed to load web assets: %v", err)
	}
	if err := verifyWebBundle(webFS); err != nil {
		log.Fatalf("Embedded GUI bundle is %v. "+
			"Run `cd gui/web && npm install && npm run build` before `go build ./cmd/shuttle-gui`.", err)
	}

	// System tray: enabled on Windows/Linux, macOS uses native dock behavior
	if runtime.GOOS != "darwin" {
		go tray.Run(eng, tray.Callbacks{
			OnShow: func() {
				if app.ctx != nil {
					wailsruntime.WindowShow(app.ctx)
				}
			},
			OnConnect: func() {
				_ = eng.Start(context.Background())
			},
			OnDisconnect: func() {
				_ = eng.Stop()
			},
			OnQuit: func() {
				if app.ctx != nil {
					wailsruntime.Quit(app.ctx)
				}
			},
		})
	}

	err = wails.Run(&options.App{
		Title:            "Shuttle",
		Width:            900,
		Height:           600,
		MinWidth:         600,
		MinHeight:        400,
		HideWindowOnClose: true, // Hide instead of close, allows tray/dock to restore
		AssetServer: &assetserver.Options{
			Assets:  webFS,
			Handler: sharedHandler,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   "Shuttle",
				Message: "Fast, secure proxy for unrestricted internet access",
			},
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

// verifyWebBundle confirms the embedded SPA has both index.html and at
// least one hashed file in assets/. A bundle missing either half embeds
// a broken page (a clean checkout ships only .gitkeep; a partial wipe
// can leave index.html pointing at absent assets). Fail fast with a
// single error the operator can act on.
func verifyWebBundle(webFS fs.FS) error {
	if _, err := fs.Stat(webFS, "index.html"); err != nil {
		return fmt.Errorf("missing (no web/dist/index.html)")
	}
	entries, err := fs.ReadDir(webFS, "assets")
	if err != nil || len(entries) == 0 {
		return fmt.Errorf("incomplete (web/dist/assets is missing or empty)")
	}
	return nil
}

// recordEngineEvents subscribes to engine events and records stats and
// connection log entries. It runs until the event channel is closed
// (engine unsubscription) and also periodically polls engine status to
// record cumulative byte counters to the stats store.
func recordEngineEvents(eng *engine.Engine, statsStore *stats.Storage, connStore *connlog.Storage) {
	ch := eng.Subscribe()
	defer eng.Unsubscribe(ch)

	// Periodic stats recording: poll engine status every 60 seconds so
	// that the cumulative byte counters get flushed to the daily stats
	// even when no speed-tick events fire (e.g. idle traffic).
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if ev.Type == engine.EventConnection {
				// Log connection events to connlog storage.
				if connStore != nil {
					connStore.Log(&connlog.Entry{
						ID:          ev.ConnID,
						Timestamp:   ev.Timestamp,
						Target:      ev.Target,
						Rule:        ev.Rule,
						Protocol:    ev.Protocol,
						ProcessName: ev.ProcessName,
						BytesIn:     ev.BytesIn,
						BytesOut:    ev.BytesOut,
						DurationMs:  ev.DurationMs,
						State:       ev.ConnState,
					})
				}
				// On connection close, record 1 new connection in stats.
				if statsStore != nil && ev.ConnState == "closed" {
					status := eng.Status()
					statsStore.Record(status.BytesSent, status.BytesReceived, 1)
				}
			}
		case <-ticker.C:
			// Periodic flush of byte counters to stats store.
			if statsStore != nil {
				status := eng.Status()
				statsStore.Record(status.BytesSent, status.BytesReceived, 0)
			}
		}
	}
}
