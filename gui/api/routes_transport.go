package api

import (
	"net/http"

	"github.com/ggshr9/shuttle/engine"
)

func registerTransportRoutes(mux *http.ServeMux, eng *engine.Engine) {
	// POST /api/transport/strategy — switch the active transport strategy at runtime.
	mux.HandleFunc("POST /api/transport/strategy", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Strategy string `json:"strategy"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()

		switch req.Strategy {
		case "auto", "priority", "latency", "multipath":
			// valid
		default:
			writeError(w, http.StatusBadRequest, "invalid strategy: must be one of auto, priority, latency, multipath")
			return
		}

		if err := eng.SetTransportStrategy(r.Context(), req.Strategy); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}

		writeJSON(w, map[string]any{"ok": true, "strategy": req.Strategy})
	})
}
