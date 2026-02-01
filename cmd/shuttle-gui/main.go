package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/engine"
	"github.com/shuttle-proxy/shuttle/gui"
	"github.com/shuttle-proxy/shuttle/gui/api"
)

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

	// Serve embedded SPA
	webFS, err := fs.Sub(gui.WebAssets, "web/dist")
	if err != nil {
		log.Fatalf("Failed to load web assets: %v", err)
	}

	srv := api.NewServer(eng, webFS)
	addr, err := srv.ListenAndServe("127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to start API server: %v", err)
	}
	defer srv.Close()

	fmt.Printf("Shuttle GUI running at http://%s\n", addr)

	// In a full Wails integration, this would be:
	//   wails.Run(&options.App{
	//     Title:  "Shuttle",
	//     Width:  900,
	//     Height: 600,
	//     URL:    "http://" + addr,
	//     OnStartup: func(ctx context.Context) { eng.Start(ctx) },
	//     OnShutdown: func(ctx context.Context) { eng.Stop() },
	//   })
	//
	// For now, we run as a standalone HTTP server that can be accessed
	// in a browser or embedded in a WebView.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	<-ctx.Done()
	eng.Stop()
}
