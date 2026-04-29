package api

import (
	"net/http"
	"strings"

	"github.com/ggshr9/shuttle/engine"
	"github.com/ggshr9/shuttle/subscription"
)

func registerSubscriptionRoutes(mux *http.ServeMux, eng *engine.Engine, subMgr *subscription.Manager) {
	mux.HandleFunc("GET /api/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, subMgr.List())
	})

	mux.HandleFunc("POST /api/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()

		if req.URL == "" {
			writeError(w, http.StatusBadRequest, "url is required")
			return
		}

		sub, err := subMgr.Add(req.Name, req.URL)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Auto-refresh after adding
		_, _ = subMgr.Refresh(r.Context(), sub.ID)
		sub, _ = subMgr.Get(sub.ID)

		// Save to config
		cfg := eng.Config()
		cfg.Subscriptions = subMgr.ToConfig()
		eng.SetConfig(&cfg)

		writeJSON(w, sub)
	})

	mux.HandleFunc("PUT /api/subscriptions/", func(w http.ResponseWriter, r *http.Request) {
		// Extract ID from path: /api/subscriptions/{id}/refresh
		path := strings.TrimPrefix(r.URL.Path, "/api/subscriptions/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 || parts[1] != "refresh" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		id := parts[0]

		sub, err := subMgr.Refresh(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, sub)
	})

	mux.HandleFunc("DELETE /api/subscriptions/", func(w http.ResponseWriter, r *http.Request) {
		// Extract ID from path: /api/subscriptions/{id}
		id := strings.TrimPrefix(r.URL.Path, "/api/subscriptions/")
		if id == "" {
			writeError(w, http.StatusBadRequest, "id is required")
			return
		}

		if err := subMgr.Remove(id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		// Save to config
		cfg := eng.Config()
		cfg.Subscriptions = subMgr.ToConfig()
		eng.SetConfig(&cfg)

		writeJSON(w, map[string]string{"status": "deleted"})
	})
}
