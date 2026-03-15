package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/shuttle-proxy/shuttle/connlog"
	"github.com/shuttle-proxy/shuttle/engine"
	"github.com/shuttle-proxy/shuttle/stats"
)

func registerStatsRoutes(mux *http.ServeMux, eng *engine.Engine, statsStore *stats.Storage, connStore *connlog.Storage) {
	mux.HandleFunc("GET /api/stats/history", func(w http.ResponseWriter, r *http.Request) {
		days := 7
		if d := r.URL.Query().Get("days"); d != "" {
			if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 90 {
				days = parsed
			}
		}

		if statsStore != nil {
			writeJSON(w, map[string]any{
				"history": statsStore.GetHistory(days),
				"total":   statsStore.GetTotal(),
			})
		} else {
			// Return empty history if stats storage not configured
			writeJSON(w, map[string]any{
				"history": []stats.DailyStats{},
				"total":   stats.DailyStats{},
			})
		}
	})

	mux.HandleFunc("GET /api/stats/weekly", func(w http.ResponseWriter, r *http.Request) {
		weeks := 4
		if v := r.URL.Query().Get("weeks"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 52 {
				weeks = parsed
			}
		}
		if statsStore != nil {
			writeJSON(w, statsStore.GetWeeklySummary(weeks))
		} else {
			writeJSON(w, []stats.PeriodStats{})
		}
	})

	mux.HandleFunc("GET /api/stats/monthly", func(w http.ResponseWriter, r *http.Request) {
		months := 6
		if v := r.URL.Query().Get("months"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 24 {
				months = parsed
			}
		}
		if statsStore != nil {
			writeJSON(w, statsStore.GetMonthlySummary(months))
		} else {
			writeJSON(w, []stats.PeriodStats{})
		}
	})

	mux.HandleFunc("GET /api/connections/history", func(w http.ResponseWriter, r *http.Request) {
		if connStore != nil {
			writeJSON(w, connStore.Recent(100))
		} else {
			writeJSON(w, []connlog.Entry{})
		}
	})

	// Streams by connection ID endpoint
	mux.HandleFunc("GET /api/connections/", func(w http.ResponseWriter, r *http.Request) {
		// Extract path: /api/connections/{id}/streams
		path := strings.TrimPrefix(r.URL.Path, "/api/connections/")
		parts := strings.Split(path, "/")
		if len(parts) != 2 || parts[1] != "streams" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		connID := parts[0]
		if connID == "" {
			writeError(w, http.StatusBadRequest, "connection id required")
			return
		}

		st := eng.StreamTracker()
		if st == nil {
			writeJSON(w, []any{})
			return
		}

		streams := st.ByConnID(connID)
		type streamInfo struct {
			StreamID      uint64 `json:"stream_id"`
			ConnID        string `json:"conn_id"`
			Target        string `json:"target"`
			Transport     string `json:"transport"`
			BytesSent     int64  `json:"bytes_sent"`
			BytesReceived int64  `json:"bytes_received"`
			Errors        int64  `json:"errors"`
			Closed        bool   `json:"closed"`
			DurationMs    int64  `json:"duration_ms"`
		}
		out := make([]streamInfo, 0, len(streams))
		for _, m := range streams {
			out = append(out, streamInfo{
				StreamID:      m.StreamID,
				ConnID:        m.ConnID,
				Target:        m.Target,
				Transport:     m.Transport,
				BytesSent:     m.BytesSent.Load(),
				BytesReceived: m.BytesReceived.Load(),
				Errors:        m.Errors.Load(),
				Closed:        m.Closed.Load(),
				DurationMs:    m.GetDuration().Milliseconds(),
			})
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("GET /api/transports/stats", func(w http.ResponseWriter, r *http.Request) {
		status := eng.Status()
		if status.TransportBreakdown != nil {
			writeJSON(w, status.TransportBreakdown)
		} else {
			writeJSON(w, []struct{}{})
		}
	})

	mux.HandleFunc("GET /api/multipath/stats", func(w http.ResponseWriter, r *http.Request) {
		mpStats := eng.MultipathStats()
		if mpStats == nil {
			writeJSON(w, []struct{}{})
			return
		}
		writeJSON(w, mpStats)
	})

	// WebSocket endpoints
	mux.HandleFunc("GET /api/logs", func(w http.ResponseWriter, r *http.Request) {
		handleEventWS(w, r, eng, engine.EventLog)
	})

	mux.HandleFunc("GET /api/speed", func(w http.ResponseWriter, r *http.Request) {
		handleEventWS(w, r, eng, engine.EventSpeedTick)
	})

	mux.HandleFunc("GET /api/connections", func(w http.ResponseWriter, r *http.Request) {
		handleEventWS(w, r, eng, engine.EventConnection)
	})
}
