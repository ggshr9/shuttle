package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/coder/websocket"
)

// registerEventsRoutes mounts /api/events (REST polling) and /ws/events
// (WebSocket long-poll) on the supplied mux. Both endpoints read from the
// shared EventQueue. If q is nil, no routes are registered.
func registerEventsRoutes(mux *http.ServeMux, q *EventQueue) {
	if q == nil {
		return
	}
	mux.Handle("/api/events", eventsRESTHandler(q))
	mux.Handle("/ws/events", eventsWSHandler(q))
}

func eventsRESTHandler(q *EventQueue) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		since, ok := parseInt64Param(r, "since", 0)
		if !ok {
			http.Error(w, `{"error":"invalid since"}`, http.StatusBadRequest)
			return
		}
		max, ok := parseIntParam(r, "max", 100)
		if !ok {
			http.Error(w, `{"error":"invalid max"}`, http.StatusBadRequest)
			return
		}
		events, latest, gap := q.Tail(since, max)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"events": events,
			"cursor": latest,
			"gap":    gap,
		})
	})
}

func eventsWSHandler(q *EventQueue) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		since, ok := parseInt64Param(r, "since", 0)
		if !ok {
			http.Error(w, `{"error":"invalid since"}`, http.StatusBadRequest)
			return
		}

		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		ctx := r.Context()
		for {
			if ctx.Err() != nil {
				return
			}
			events, latest, gap, err := q.Wait(ctx, since)
			if err != nil {
				return
			}
			payload := map[string]any{"events": events, "cursor": latest, "gap": gap}
			data, _ := json.Marshal(payload)
			if err := c.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
			since = latest
		}
	})
}

func parseInt64Param(r *http.Request, key string, def int64) (int64, bool) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def, true
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseIntParam(r *http.Request, key string, def int) (int, bool) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def, true
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}
