package api

import (
	"net/http"
	"strings"

	"github.com/ggshr9/shuttle/engine"
)

func registerProviderRoutes(mux *http.ServeMux, eng *engine.Engine) {
	// GET /api/providers/proxy — list all proxy providers.
	mux.HandleFunc("GET /api/providers/proxy", func(w http.ResponseWriter, r *http.Request) {
		providers := eng.ListProxyProviders()
		writeJSON(w, providers)
	})

	// POST /api/providers/proxy/{name}/refresh — manual refresh of a proxy provider.
	mux.HandleFunc("POST /api/providers/proxy/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/providers/proxy/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 || parts[1] != "refresh" || parts[0] == "" {
			writeError(w, http.StatusNotFound, "expected /api/providers/proxy/{name}/refresh")
			return
		}
		name := parts[0]

		if err := eng.RefreshProxyProvider(r.Context(), name); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /api/providers/rule — list all rule providers.
	mux.HandleFunc("GET /api/providers/rule", func(w http.ResponseWriter, r *http.Request) {
		providers := eng.ListRuleProviders()
		writeJSON(w, providers)
	})

	// POST /api/providers/rule/{name}/refresh — manual refresh of a rule provider.
	mux.HandleFunc("POST /api/providers/rule/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/providers/rule/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 || parts[1] != "refresh" || parts[0] == "" {
			writeError(w, http.StatusNotFound, "expected /api/providers/rule/{name}/refresh")
			return
		}
		name := parts[0]

		if err := eng.RefreshRuleProvider(r.Context(), name); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
