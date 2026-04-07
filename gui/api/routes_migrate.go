package api

import (
	"io"
	"net/http"

	"github.com/shuttleX/shuttle/subscription"
)

func registerMigrateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/migrate/validate", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody))
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to read body")
			return
		}
		report := subscription.ValidateClashMigration(body)
		writeJSON(w, report)
	})
}
