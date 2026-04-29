// gui/api/health_deep.go
//
// Deep health endpoints for the client GUI API: /api/health/live and
// /api/health/ready. These mirror the server-side admin probes (see
// server/admin/health.go) so external supervisors can drive the same
// liveness / readiness contract on either side.
//
// The legacy /api/healthz endpoint (gui/api/healthz.go) is preserved
// verbatim — it has different semantics (iOS BridgeAdapter shallow
// probe) and is wired via registerHealthzRoute.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/shuttleX/shuttle/internal/healthcheck"
)

const clientLivenessThreshold = 30 * time.Second

// engineProbe is the minimal surface of the engine needed by the deep
// readiness check. The real engine.Engine satisfies this via small
// adapter methods (StateName / HealthyOutbounds / ConfigValid).
type engineProbe interface {
	StateName() string
	HealthyOutbounds() int
	ConfigValid() bool
}

type clientCheckResult struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type clientHealthResponse struct {
	Status string                       `json:"status"`
	Checks map[string]clientCheckResult `json:"checks,omitempty"`
	TS     string                       `json:"ts"`
}

// registerDeepHealthRoutes mounts /api/health/live and /api/health/ready
// using the default liveness threshold.
func registerDeepHealthRoutes(mux *http.ServeMux, eng engineProbe, hb *healthcheck.Heartbeat) {
	registerDeepHealthRoutesWithThreshold(mux, eng, hb, clientLivenessThreshold)
}

// registerDeepHealthRoutesWithThreshold is the test-friendly entry point
// that allows callers to override the heartbeat freshness threshold.
func registerDeepHealthRoutesWithThreshold(mux *http.ServeMux, eng engineProbe, hb *healthcheck.Heartbeat, livenessThreshold time.Duration) {
	mux.HandleFunc("GET /api/health/live", func(w http.ResponseWriter, r *http.Request) {
		if hb != nil && !hb.IsAlive(livenessThreshold) {
			writeClientHealth(w, http.StatusServiceUnavailable, "unhealthy", map[string]clientCheckResult{
				"heartbeat": {Status: "fail", Error: "stale heartbeat"},
			})
			return
		}
		writeClientHealth(w, http.StatusOK, "ok", nil)
	})

	mux.HandleFunc("GET /api/health/ready", func(w http.ResponseWriter, r *http.Request) {
		checks := runClientReadiness(eng)
		status := http.StatusOK
		overall := "ok"
		for _, c := range checks {
			if c.Status == "fail" {
				status = http.StatusServiceUnavailable
				overall = "unhealthy"
				break
			}
		}
		writeClientHealth(w, status, overall, checks)
	})
}

func writeClientHealth(w http.ResponseWriter, status int, overall string, checks map[string]clientCheckResult) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(clientHealthResponse{
		Status: overall,
		Checks: checks,
		TS:     time.Now().UTC().Format(time.RFC3339),
	})
}

// runClientReadiness evaluates whether the client engine is ready to
// serve traffic. A check status of "fail" causes the overall response
// to be 503.
func runClientReadiness(eng engineProbe) map[string]clientCheckResult {
	out := map[string]clientCheckResult{}
	if eng == nil {
		out["engine"] = clientCheckResult{Status: "fail", Error: "engine not initialised"}
		return out
	}
	switch eng.StateName() {
	case "running", "starting":
		out["engine"] = clientCheckResult{Status: "ok", Detail: eng.StateName()}
	default:
		out["engine"] = clientCheckResult{Status: "fail", Error: "engine state: " + eng.StateName()}
	}
	if eng.ConfigValid() {
		out["config"] = clientCheckResult{Status: "ok"}
	} else {
		out["config"] = clientCheckResult{Status: "fail", Error: "config validation failed"}
	}
	if eng.HealthyOutbounds() > 0 {
		out["outbounds"] = clientCheckResult{Status: "ok"}
	} else {
		out["outbounds"] = clientCheckResult{Status: "fail", Error: "no healthy outbound"}
	}
	return out
}
