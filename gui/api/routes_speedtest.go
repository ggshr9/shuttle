package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/engine"
	"github.com/shuttleX/shuttle/speedtest"
)

func registerSpeedtestRoutes(mux *http.ServeMux, eng *engine.Engine, stHistory *speedtest.HistoryStorage) {
	mux.HandleFunc("POST /api/speedtest", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()

		// Collect all servers to test
		var servers []speedtest.Server
		if cfg.Server.Addr != "" {
			servers = append(servers, speedtest.Server{
				Addr:     cfg.Server.Addr,
				Name:     cfg.Server.Name,
				Password: cfg.Server.Password,
				SNI:      cfg.Server.SNI,
			})
		}
		for _, srv := range cfg.Servers {
			servers = append(servers, speedtest.Server{
				Addr:     srv.Addr,
				Name:     srv.Name,
				Password: srv.Password,
				SNI:      srv.SNI,
			})
		}

		if len(servers) == 0 {
			writeError(w, http.StatusBadRequest, "no servers configured")
			return
		}

		tester := speedtest.NewTester(nil)
		results := tester.TestAll(r.Context(), servers)
		speedtest.SortByLatency(results)

		// Persist results to history storage
		if stHistory != nil {
			now := time.Now()
			var histEntries []speedtest.HistoryEntry
			for _, r := range results {
				histEntries = append(histEntries, speedtest.HistoryEntry{
					Timestamp:  now,
					ServerAddr: r.ServerAddr,
					ServerName: r.ServerName,
					LatencyMs:  r.LatencyMs,
					Available:  r.Available,
				})
			}
			_ = stHistory.Record(histEntries)
		}

		writeJSON(w, map[string]any{
			"results": results,
		})
	})

	mux.HandleFunc("GET /api/speedtest/history", func(w http.ResponseWriter, r *http.Request) {
		days := 30
		if d := r.URL.Query().Get("days"); d != "" {
			if v, err := strconv.Atoi(d); err == nil && v > 0 {
				days = v
			}
		}
		var entries []speedtest.HistoryEntry
		if stHistory != nil {
			entries = stHistory.GetHistory(days)
		}
		if entries == nil {
			entries = []speedtest.HistoryEntry{}
		}
		writeJSON(w, entries)
	})

	mux.HandleFunc("POST /api/config/servers/auto-select", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()

		// Collect all servers to test
		var servers []speedtest.Server
		serverMap := make(map[string]config.ServerEndpoint)

		for _, srv := range cfg.Servers {
			servers = append(servers, speedtest.Server{
				Addr:     srv.Addr,
				Name:     srv.Name,
				Password: srv.Password,
				SNI:      srv.SNI,
			})
			serverMap[srv.Addr] = srv
		}

		if len(servers) == 0 {
			writeError(w, http.StatusBadRequest, "no servers to select from")
			return
		}

		tester := speedtest.NewTester(nil)
		results := tester.TestAll(r.Context(), servers)
		speedtest.SortByLatency(results)

		// Find the best available server
		var best *speedtest.TestResult
		for i := range results {
			if results[i].Available {
				best = &results[i]
				break
			}
		}

		if best == nil {
			writeError(w, http.StatusServiceUnavailable, "no available servers")
			return
		}

		// Switch to the best server
		bestServer := serverMap[best.ServerAddr]
		cfg.Server = bestServer
		if err := eng.Reload(&cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, map[string]any{
			"status":  "selected",
			"server":  bestServer,
			"latency": best.LatencyMs,
		})
	})

	mux.HandleFunc("GET /api/speedtest/stream", func(w http.ResponseWriter, r *http.Request) {
		handleSpeedtestWS(w, r, eng)
	})
}
