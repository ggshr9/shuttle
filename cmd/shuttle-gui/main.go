package main

import (
	"context"
	"io/fs"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/engine"
	"github.com/shuttle-proxy/shuttle/gui"
	"github.com/shuttle-proxy/shuttle/gui/api"
	"github.com/shuttle-proxy/shuttle/gui/tray"
)

// App wraps the engine for Wails bindings.
type App struct {
	ctx context.Context
	eng *engine.Engine
	srv *api.Server
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Start internal API server for REST + WebSocket endpoints.
	a.srv = api.NewServer(a.eng, nil)
	addr, err := a.srv.ListenAndServe("127.0.0.1:0")
	if err != nil {
		log.Printf("API server failed: %v", err)
		return
	}
	log.Printf("API server at http://%s", addr)
}

func (a *App) shutdown(ctx context.Context) {
	a.eng.Stop()
	if a.srv != nil {
		a.srv.Close()
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
func (a *App) SetConfig(cfg config.ClientConfig) error {
	return a.eng.Reload(&cfg)
}

func main() {
	configPath := "config/client.example.yaml"
	for i, arg := range os.Args[1:] {
		if arg == "-c" && i+1 < len(os.Args[1:]) {
			configPath = os.Args[i+2]
		}
	}

	cfg, err := config.LoadClientConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	eng := engine.New(cfg)
	app := &App{eng: eng}

	// Embedded SPA assets
	webFS, err := fs.Sub(gui.WebAssets, "web/dist")
	if err != nil {
		log.Fatalf("Failed to load web assets: %v", err)
	}

	// System tray runs alongside Wails
	go tray.Run(eng, tray.Callbacks{
		OnShow: func() {
			if app.ctx != nil {
				wailsruntime.WindowShow(app.ctx)
			}
		},
		OnConnect: func() {
			eng.Start(context.Background())
		},
		OnDisconnect: func() {
			eng.Stop()
		},
		OnQuit: func() {
			if app.ctx != nil {
				wailsruntime.Quit(app.ctx)
			}
		},
	})

	err = wails.Run(&options.App{
		Title:     "Shuttle",
		Width:     900,
		Height:    600,
		MinWidth:  600,
		MinHeight: 400,
		AssetServer: &assetserver.Options{
			Assets:  webFS,
			Handler: api.Handler(eng),
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
