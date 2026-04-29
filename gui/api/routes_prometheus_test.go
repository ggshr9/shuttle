package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/engine"
)

func TestPrometheusEndpoint(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/prometheus", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected text/plain Content-Type, got %q", ct)
	}

	body := rr.Body.String()

	expectedMetrics := []string{
		"# HELP shuttle_active_connections",
		"# TYPE shuttle_active_connections gauge",
		"shuttle_active_connections ",
		"# HELP shuttle_total_connections",
		"# TYPE shuttle_total_connections counter",
		"shuttle_total_connections ",
		"# HELP shuttle_bytes_sent",
		"# TYPE shuttle_bytes_sent counter",
		"shuttle_bytes_sent ",
		"# HELP shuttle_bytes_received",
		"# TYPE shuttle_bytes_received counter",
		"shuttle_bytes_received ",
		"# HELP shuttle_upload_speed_bytes",
		"# TYPE shuttle_upload_speed_bytes gauge",
		"shuttle_upload_speed_bytes ",
		"# HELP shuttle_download_speed_bytes",
		"# TYPE shuttle_download_speed_bytes gauge",
		"shuttle_download_speed_bytes ",
		"# HELP shuttle_circuit_breaker_state",
		"# TYPE shuttle_circuit_breaker_state gauge",
		"shuttle_circuit_breaker_state ",
		"# HELP shuttle_draining_connections",
		"# TYPE shuttle_draining_connections gauge",
		"shuttle_draining_connections ",
	}

	for _, want := range expectedMetrics {
		if !strings.Contains(body, want) {
			t.Errorf("response body missing %q\nfull body:\n%s", want, body)
		}
	}
}

// TestPrometheus_NewMetricsEmitted verifies that the new labelled metrics
// derived from the engine MetricsSnapshot (router decisions, per-outbound
// CB state, subscription refresh, handshake durations) are emitted by the
// /api/prometheus endpoint.
//
// Seeds the engine via SeedMetricsForTest because driving real handshakes
// / decisions / subscription refreshes would require network I/O (which
// the host-safe test tier disallows).
func TestPrometheus_NewMetricsEmitted(t *testing.T) {
	eng := newTestEngine()

	eng.SeedMetricsForTest(engine.MetricsSnapshot{
		RoutingDecisions: map[string]int64{"proxy/geosite": 4},
		CircuitBreakers:  map[string]string{"out-primary": "open"},
		Subscriptions: map[string]engine.SubscriptionStats{
			"sub-1": {OK: 3, Fail: 1, LastRefresh: time.Unix(1700000000, 0)},
		},
		HandshakeDurationsNanos: map[string][]int64{
			"h3": {int64(50 * time.Millisecond), int64(75 * time.Millisecond)},
		},
	})

	mux := http.NewServeMux()
	registerPrometheusRoutes(mux, eng)

	req := httptest.NewRequest("GET", "/api/prometheus", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	for _, want := range []string{
		"shuttle_routing_decisions_total",
		"shuttle_circuit_breaker_state{outbound=",
		"shuttle_subscription_refresh_total",
		"shuttle_subscription_last_refresh_timestamp",
		"shuttle_handshake_duration_seconds",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in /api/prometheus body\nfull body:\n%s", want, body)
		}
	}
}
