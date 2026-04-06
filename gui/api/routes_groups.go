package api

import (
	"net/http"
	"strings"

	"github.com/shuttleX/shuttle/engine"
)

func registerGroupRoutes(mux *http.ServeMux, eng *engine.Engine) {
	// GET /api/groups — list all strategy groups with status.
	mux.HandleFunc("GET /api/groups", func(w http.ResponseWriter, r *http.Request) {
		groups := eng.ListGroups()
		writeJSON(w, groups)
	})

	// GET /api/groups/{tag} — get group details (members, selected, latencies).
	// PUT /api/groups/{tag}/selected — select node in select-strategy group.
	// POST /api/groups/{tag}/test — trigger health check for group.
	mux.HandleFunc("/api/groups/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/groups/")
		if path == "" {
			writeError(w, http.StatusNotFound, "missing group tag")
			return
		}

		// Check for sub-resources: {tag}/selected or {tag}/test
		parts := strings.SplitN(path, "/", 2)
		tag := parts[0]

		if len(parts) == 1 {
			// GET /api/groups/{tag}
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			group, err := eng.GetGroup(tag)
			if err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeJSON(w, group)
			return
		}

		switch parts[1] {
		case "selected":
			// PUT /api/groups/{tag}/selected
			if r.Method != http.MethodPut {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			var body struct {
				Selected string `json:"selected"`
			}
			if err := decodeJSON(r, &body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if body.Selected == "" {
				writeError(w, http.StatusBadRequest, "selected is required")
				return
			}
			if err := eng.SelectGroupOutbound(tag, body.Selected); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case "test":
			// POST /api/groups/{tag}/test
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			results, err := eng.TestGroup(tag)
			if err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeJSON(w, results)

		default:
			writeError(w, http.StatusNotFound, "unknown sub-resource")
		}
	})
}
