package api

import (
	"encoding/json"
	"net/http"

	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/engine"
)

// Handler creates the HTTP handler for the shuttle API.
func Handler(eng *engine.Engine) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, eng.Status())
	})

	mux.HandleFunc("POST /api/connect", func(w http.ResponseWriter, r *http.Request) {
		if err := eng.Start(r.Context()); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "connected"})
	})

	mux.HandleFunc("POST /api/disconnect", func(w http.ResponseWriter, r *http.Request) {
		if err := eng.Stop(); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "disconnected"})
	})

	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, eng.Config())
	})

	mux.HandleFunc("PUT /api/config", func(w http.ResponseWriter, r *http.Request) {
		var cfg config.ClientConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := eng.Reload(&cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "reloaded"})
	})

	mux.HandleFunc("GET /api/config/servers", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()
		writeJSON(w, map[string]any{
			"active":  cfg.Server,
			"servers": cfg.Servers,
		})
	})

	mux.HandleFunc("PUT /api/config/servers", func(w http.ResponseWriter, r *http.Request) {
		var srv config.ServerEndpoint
		if err := json.NewDecoder(r.Body).Decode(&srv); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cfg := *eng.Config()
		cfg.Server = srv
		if err := eng.Reload(&cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "updated"})
	})

	mux.HandleFunc("POST /api/config/servers", func(w http.ResponseWriter, r *http.Request) {
		var srv config.ServerEndpoint
		if err := json.NewDecoder(r.Body).Decode(&srv); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cfg := *eng.Config()
		cfg.Servers = append(cfg.Servers, srv)
		eng.SetConfig(&cfg)
		writeJSON(w, map[string]string{"status": "added"})
	})

	mux.HandleFunc("DELETE /api/config/servers", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Addr string `json:"addr"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cfg := *eng.Config()
		filtered := cfg.Servers[:0]
		for _, s := range cfg.Servers {
			if s.Addr != req.Addr {
				filtered = append(filtered, s)
			}
		}
		cfg.Servers = filtered
		eng.SetConfig(&cfg)
		writeJSON(w, map[string]string{"status": "deleted"})
	})

	mux.HandleFunc("GET /api/routing/rules", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()
		writeJSON(w, cfg.Routing)
	})

	mux.HandleFunc("PUT /api/routing/rules", func(w http.ResponseWriter, r *http.Request) {
		var routing config.RoutingConfig
		if err := json.NewDecoder(r.Body).Decode(&routing); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cfg := *eng.Config()
		cfg.Routing = routing
		if err := eng.Reload(&cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "updated"})
	})

	// WebSocket endpoints
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		handleEventWS(w, r, eng, engine.EventLog)
	})

	mux.HandleFunc("/api/speed", func(w http.ResponseWriter, r *http.Request) {
		handleEventWS(w, r, eng, engine.EventSpeedTick)
	})

	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
