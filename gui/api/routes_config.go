package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/engine"
)

// modifyServers is a shared helper for POST/DELETE /api/config/servers handlers.
// It loads the current config, applies fn to mutate the Servers slice, persists
// the config via SetConfig, and writes cfg.Servers as JSON on success.
func modifyServers(w http.ResponseWriter, eng *engine.Engine, fn func([]config.ServerEndpoint) ([]config.ServerEndpoint, string, int)) {
	cfg := eng.Config()
	servers, errMsg, code := fn(cfg.Servers)
	if errMsg != "" {
		writeError(w, code, errMsg)
		return
	}
	cfg.Servers = servers
	eng.SetConfig(&cfg)
	writeJSON(w, map[string]string{"status": "ok"})
}

func registerConfigRoutes(mux *http.ServeMux, eng *engine.Engine) {
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()
		redactClientConfig(&cfg)
		writeJSON(w, &cfg)
	})

	mux.HandleFunc("PUT /api/config", func(w http.ResponseWriter, r *http.Request) {
		var cfg config.ClientConfig
		if err := decodeJSON(r, &cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()
		if err := eng.Reload(&cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "reloaded"})
	})

	mux.HandleFunc("GET /api/config/servers", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()
		servers := cfg.Servers

		// Optional pagination — preserves backward compatibility:
		// without ?size, the full list is returned as before.
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if size > 0 {
			if size > 200 {
				size = 200
			}
			if page < 0 {
				page = 0
			}
			start := page * size
			end := start + size
			if start > len(servers) {
				start = len(servers)
			}
			if end > len(servers) {
				end = len(servers)
			}
			writeJSON(w, map[string]any{
				"active":  cfg.Server,
				"servers": servers[start:end],
				"total":   len(servers),
				"page":    page,
				"size":    size,
			})
			return
		}

		writeJSON(w, map[string]any{
			"active":  cfg.Server,
			"servers": servers,
		})
	})

	mux.HandleFunc("PUT /api/config/servers", func(w http.ResponseWriter, r *http.Request) {
		var srv config.ServerEndpoint
		if err := decodeJSON(r, &srv); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()
		if srv.Addr == "" {
			writeError(w, http.StatusBadRequest, "addr is required")
			return
		}
		cfg := eng.Config()
		cfg.Server = srv
		if err := eng.Reload(&cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "updated"})
	})

	mux.HandleFunc("POST /api/config/servers", func(w http.ResponseWriter, r *http.Request) {
		var srv config.ServerEndpoint
		if err := decodeJSON(r, &srv); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()
		if srv.Addr == "" {
			writeError(w, http.StatusBadRequest, "addr is required")
			return
		}
		modifyServers(w, eng, func(servers []config.ServerEndpoint) ([]config.ServerEndpoint, string, int) {
			for _, s := range servers {
				if s.Addr == srv.Addr {
					return nil, "server with this address already exists", http.StatusConflict
				}
			}
			return append(servers, srv), "", 0
		})
	})

	mux.HandleFunc("DELETE /api/config/servers", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Addr string `json:"addr"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()
		if req.Addr == "" {
			writeError(w, http.StatusBadRequest, "addr is required")
			return
		}
		modifyServers(w, eng, func(servers []config.ServerEndpoint) ([]config.ServerEndpoint, string, int) {
			filtered := make([]config.ServerEndpoint, 0, len(servers))
			found := false
			for _, s := range servers {
				if !found && s.Addr == req.Addr {
					found = true
					continue
				}
				filtered = append(filtered, s)
			}
			if !found {
				return nil, "server not found", http.StatusNotFound
			}
			return filtered, "", 0
		})
	})

	mux.HandleFunc("POST /api/config/validate", func(w http.ResponseWriter, r *http.Request) {
		var cfg config.ClientConfig
		if err := decodeJSON(r, &cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()

		var errs []string
		if err := cfg.Validate(); err != nil {
			errs = append(errs, err.Error())
		}
		writeJSON(w, map[string]any{
			"valid":  len(errs) == 0,
			"errors": errs,
		})
	})

	mux.HandleFunc("GET /api/config/export", func(w http.ResponseWriter, r *http.Request) {
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "json"
		}

		cfg := eng.Config()

		// Redact secrets unless explicitly requested
		if r.URL.Query().Get("include_secrets") != "true" {
			redactClientConfig(&cfg)
		}

		switch format {
		case "json":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", "attachment; filename=shuttle-config.json")
			_ = json.NewEncoder(w).Encode(&cfg)
		case "yaml":
			data, err := config.ExportConfig(&cfg, "yaml")
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.Header().Set("Content-Type", "text/yaml")
			w.Header().Set("Content-Disposition", "attachment; filename=shuttle-config.yaml")
			_, _ = w.Write(data)
		case "uri":
			data, err := config.ExportConfig(&cfg, "uri")
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", "attachment; filename=shuttle-servers.txt")
			_, _ = w.Write(data)
		default:
			writeError(w, http.StatusBadRequest, "unsupported format: "+format)
		}
	})

	mux.HandleFunc("POST /api/config/import", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Data string `json:"data"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()

		result, err := config.ImportConfig(req.Data)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		// Add imported servers to config
		cfg := eng.Config()
		existingAddrs := make(map[string]bool)
		for _, s := range cfg.Servers {
			existingAddrs[s.Addr] = true
		}

		added := 0
		for _, srv := range result.Servers {
			if !existingAddrs[srv.Addr] {
				cfg.Servers = append(cfg.Servers, srv)
				existingAddrs[srv.Addr] = true
				added++
			}
		}
		eng.SetConfig(&cfg)

		// If the imported config has mesh enabled, apply it
		if result.MeshEnabled {
			cfg.Mesh.Enabled = true
			cfg.Mesh.P2PEnabled = true
			eng.SetConfig(&cfg)
		}

		writeJSON(w, map[string]any{
			"status":       "imported",
			"added":        added,
			"total":        len(result.Servers),
			"servers":      result.Servers,
			"errors":       result.Errors,
			"mesh_enabled": result.MeshEnabled,
		})
	})
}
