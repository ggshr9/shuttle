package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/internal/healthcheck"
)

func TestHealthLive_OK(t *testing.T) {
	hb := healthcheck.NewHeartbeat()
	hb.Tick()

	mux := http.NewServeMux()
	registerHealthRoutes(mux, &ServerInfo{}, nil, nil, hb)

	req := httptest.NewRequest("GET", "/api/health/live", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %v, want ok", body["status"])
	}
}

func TestHealthLive_503WhenStale(t *testing.T) {
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	time.Sleep(20 * time.Millisecond)

	mux := http.NewServeMux()
	// staleThreshold of 10ms means the heartbeat is now stale
	registerHealthRoutesWithThreshold(mux, &ServerInfo{}, nil, nil, hb, 10*time.Millisecond)

	req := httptest.NewRequest("GET", "/api/health/live", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}
