package main

import (
	"log/slog"
	"net/http"

	"github.com/shuttleX/shuttle/engine"
	"github.com/shuttleX/shuttle/gui/api"
)

// startUIServer launches the Web management UI on addr using the shared
// gui/api handler. Runs until the process exits.
func startUIServer(addr, token string, eng *engine.Engine) {
	h := api.NewHandler(api.HandlerConfig{
		Engine:    eng,
		AuthToken: token,
	})
	srv := &http.Server{Addr: addr, Handler: h}
	slog.Info("Web UI listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("UI server failed", "err", err)
	}
}
