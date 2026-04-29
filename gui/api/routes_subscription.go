package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/ggshr9/shuttle/config"
	"github.com/ggshr9/shuttle/engine"
	"github.com/ggshr9/shuttle/subscription"
)

// subscriptionDTO is the wire shape returned to the frontend. It
// mirrors the TS `Subscription` interface in gui/web/src/lib/api/types.ts
// — keep them in sync. Notable differences from subscription.Subscription:
//   - servers carry only address-level fields; passwords/secrets are
//     redacted because the UI never reads them.
//   - updated_at is an RFC3339 string, optional (omitted when zero).
type subscriptionDTO struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	URL       string      `json:"url"`
	Servers   []serverDTO `json:"servers,omitempty"`
	UpdatedAt string      `json:"updated_at,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// serverDTO mirrors the TS `Server` interface — addr/name/sni only.
// password, type, options are intentionally omitted: they are config
// internals, not UI data.
type serverDTO struct {
	Addr string `json:"addr"`
	Name string `json:"name,omitempty"`
	SNI  string `json:"sni,omitempty"`
}

func toServerDTO(s config.ServerEndpoint) serverDTO {
	return serverDTO{Addr: s.Addr, Name: s.Name, SNI: s.SNI}
}

func toSubscriptionDTO(sub *subscription.Subscription) subscriptionDTO {
	if sub == nil {
		return subscriptionDTO{}
	}
	dto := subscriptionDTO{
		ID:    sub.ID,
		Name:  sub.Name,
		URL:   sub.URL,
		Error: sub.Error,
	}
	if !sub.UpdatedAt.IsZero() {
		dto.UpdatedAt = sub.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if len(sub.Servers) > 0 {
		dto.Servers = make([]serverDTO, len(sub.Servers))
		for i, s := range sub.Servers {
			dto.Servers[i] = toServerDTO(s)
		}
	}
	return dto
}

func toSubscriptionListDTO(subs []*subscription.Subscription) []subscriptionDTO {
	out := make([]subscriptionDTO, 0, len(subs))
	for _, s := range subs {
		out = append(out, toSubscriptionDTO(s))
	}
	return out
}

func registerSubscriptionRoutes(mux *http.ServeMux, eng *engine.Engine, subMgr *subscription.Manager) {
	mux.HandleFunc("GET /api/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, toSubscriptionListDTO(subMgr.List()))
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

		writeJSON(w, toSubscriptionDTO(sub))
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
		writeJSON(w, toSubscriptionDTO(sub))
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
