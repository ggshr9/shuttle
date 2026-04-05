package api

import (
	"net/http"
	"strings"
	"testing"
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
	}

	for _, want := range expectedMetrics {
		if !strings.Contains(body, want) {
			t.Errorf("response body missing %q\nfull body:\n%s", want, body)
		}
	}
}
