package admin

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/internal/healthcheck"
	"github.com/shuttleX/shuttle/server/metrics"
)

const defaultLivenessThreshold = 30 * time.Second

type checkResult struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Addr   string `json:"addr,omitempty"`
}

type healthResponse struct {
	Status string                 `json:"status"`
	Checks map[string]checkResult `json:"checks,omitempty"`
	TS     string                 `json:"ts"`
}

// registerHealthRoutes mounts /api/health/live and /api/health/ready on mux
// using the default liveness threshold.
func registerHealthRoutes(mux *http.ServeMux, info *ServerInfo, cfg *config.ServerConfig, mc *metrics.Collector, hb *healthcheck.Heartbeat) {
	registerHealthRoutesWithThreshold(mux, info, cfg, mc, hb, defaultLivenessThreshold)
}

// registerHealthRoutesWithThreshold is the test-friendly entry point that
// allows callers to override the heartbeat freshness threshold.
func registerHealthRoutesWithThreshold(mux *http.ServeMux, info *ServerInfo, cfg *config.ServerConfig, mc *metrics.Collector, hb *healthcheck.Heartbeat, livenessThreshold time.Duration) {
	mux.HandleFunc("GET /api/health/live", func(w http.ResponseWriter, r *http.Request) {
		if hb != nil && !hb.IsAlive(livenessThreshold) {
			writeHealth(w, http.StatusServiceUnavailable, "unhealthy", map[string]checkResult{
				"heartbeat": {Status: "fail", Error: "stale heartbeat"},
			})
			return
		}
		writeHealth(w, http.StatusOK, "ok", nil)
	})

	mux.HandleFunc("GET /api/health/ready", func(w http.ResponseWriter, r *http.Request) {
		checks := runReadinessChecks(info, cfg, mc)
		status := http.StatusOK
		overall := "ok"
		for _, c := range checks {
			if c.Status == "fail" {
				status = http.StatusServiceUnavailable
				overall = "unhealthy"
				break
			}
		}
		writeHealth(w, status, overall, checks)
	})
}

func writeHealth(w http.ResponseWriter, status int, overall string, checks map[string]checkResult) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(healthResponse{
		Status: overall,
		Checks: checks,
		TS:     time.Now().UTC().Format(time.RFC3339),
	})
}

// runReadinessChecks evaluates whether the server is ready to accept traffic.
// A check status of "fail" causes the overall response to be 503.
//
// Note on field names (deviates from spec):
//   - cfg.Transport is ServerTransportConfig (not TransportConfig).
//   - ServerH3Config and ServerRealityConfig have no Listen field; both
//     transports bind to the main cfg.Listen address.
//   - ServerCDNConfig.Listen is optional; falls back to cfg.Listen when empty.
//   - ServerConfig has no Metrics field, so we cannot key the metrics check on
//     a config flag — the check is omitted (Task 5 may revisit this).
func runReadinessChecks(info *ServerInfo, cfg *config.ServerConfig, mc *metrics.Collector) map[string]checkResult {
	out := map[string]checkResult{}
	if cfg == nil {
		out["config"] = checkResult{Status: "fail", Error: "config not loaded"}
		return out
	}
	out["config"] = checkResult{Status: "ok"}

	if cfg.Transport.H3.Enabled {
		if info != nil && info.IsListenerReady("h3") {
			out["listener_h3"] = checkResult{Status: "ok", Addr: cfg.Listen}
		} else {
			out["listener_h3"] = checkResult{Status: "fail", Error: "not bound"}
		}
	}
	if cfg.Transport.Reality.Enabled {
		if info != nil && info.IsListenerReady("reality") {
			out["listener_reality"] = checkResult{Status: "ok", Addr: cfg.Listen}
		} else {
			out["listener_reality"] = checkResult{Status: "fail", Error: "not bound"}
		}
	}
	if cfg.Transport.CDN.Enabled {
		addr := cfg.Transport.CDN.Listen
		if addr == "" {
			addr = cfg.Listen
		}
		if info != nil && info.IsListenerReady("cdn") {
			out["listener_cdn"] = checkResult{Status: "ok", Addr: addr}
		} else {
			out["listener_cdn"] = checkResult{Status: "fail", Error: "not bound"}
		}
	}

	_ = mc // metrics readiness check is not config-gated (no cfg.Metrics block)

	return out
}
