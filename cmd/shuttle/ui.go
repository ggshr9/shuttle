package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/shuttleX/shuttle/engine"
	"github.com/shuttleX/shuttle/gui/api"
)

// startUIServer launches the Web management UI on addr using the shared
// gui/api handler. Runs until ctx is cancelled, then shuts down gracefully.
func startUIServer(ctx context.Context, addr, token string, eng *engine.Engine) {
	h := api.NewHandler(api.HandlerConfig{
		Engine:    eng,
		AuthToken: token,
	})
	srv := &http.Server{Addr: addr, Handler: h}
	slog.Info("Web UI listening", "addr", addr)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("UI server failed", "err", err)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
