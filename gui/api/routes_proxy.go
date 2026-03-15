package api

import (
	"context"
	"net/http"

	"github.com/shuttle-proxy/shuttle/autostart"
	"github.com/shuttle-proxy/shuttle/engine"
	"github.com/shuttle-proxy/shuttle/sysproxy"
)

func registerProxyRoutes(mux *http.ServeMux, eng *engine.Engine) {
	mux.HandleFunc("POST /api/connect", func(w http.ResponseWriter, r *http.Request) {
		if err := eng.Start(context.Background()); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}

		// Set system proxy if enabled
		cfg := eng.Config()
		if cfg.Proxy.SystemProxy.Enabled {
			setSystemProxy(&cfg)
		}

		writeJSON(w, map[string]string{"status": "connected"})
	})

	mux.HandleFunc("POST /api/disconnect", func(w http.ResponseWriter, r *http.Request) {
		// Clear system proxy first
		cfg := eng.Config()
		if cfg.Proxy.SystemProxy.Enabled {
			sysproxy.Clear()
		}

		if err := eng.Stop(); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "disconnected"})
	})

	mux.HandleFunc("GET /api/autostart", func(w http.ResponseWriter, r *http.Request) {
		enabled, err := autostart.IsEnabled()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"enabled": enabled})
	})

	mux.HandleFunc("PUT /api/autostart", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()

		var err error
		if req.Enabled {
			err = autostart.Enable()
		} else {
			err = autostart.Disable()
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"enabled": req.Enabled})
	})

	mux.HandleFunc("GET /api/network/lan", func(w http.ResponseWriter, r *http.Request) {
		ips := getLANAddresses()
		cfg := eng.Config()
		writeJSON(w, map[string]any{
			"allow_lan": cfg.Proxy.AllowLAN,
			"addresses": ips,
			"socks5":    cfg.Proxy.SOCKS5.Listen,
			"http":      cfg.Proxy.HTTP.Listen,
		})
	})
}
