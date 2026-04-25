// gui/api/healthz.go
package api

import (
	"encoding/json"
	"net/http"
)

// registerHealthzRoute mounts /api/healthz, a minimal liveness endpoint used
// primarily by the iOS BridgeAdapter probe (boot.ts) to verify the
// app↔extension envelope path is functioning before installing as the
// active DataAdapter. Returns {"status":"ok"} on success.
func registerHealthzRoute(mux *http.ServeMux) {
	mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
}
