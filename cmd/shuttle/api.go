//go:build !minimal

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/engine"
	"github.com/shuttleX/shuttle/gui/api"
)

func runAPI(configPath, listen string, autoConnect bool) {
	cfg, err := config.LoadClientConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	eng := engine.New(cfg)

	// Start API server
	srv := api.NewServer(eng, nil)
	addr, err := srv.ListenAndServe(listen)
	if err != nil {
		cancel()
		fmt.Fprintf(os.Stderr, "Failed to start API server: %v\n", err)
		os.Exit(1)
	}
	logger.Info("API server listening", "addr", addr)

	// Auto-connect if requested
	if autoConnect {
		if err := eng.Start(ctx); err != nil {
			logger.Error("auto-connect failed", "err", err)
		} else {
			logger.Info("auto-connected to server")
		}
	}

	<-ctx.Done()
	srv.Close()
	_ = eng.Stop()
}
