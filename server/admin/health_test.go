package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/config"
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

func TestHealthReady_OKWhenAllListenersBound(t *testing.T) {
	cfg := &config.ServerConfig{}
	cfg.Transport.H3.Enabled = true
	// Note: ServerH3Config has no Listen field; H3 binds to cfg.Listen.

	info := &ServerInfo{}
	info.MarkListenerReady("h3")

	mux := http.NewServeMux()
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	registerHealthRoutes(mux, info, cfg, nil, hb)

	req := httptest.NewRequest("GET", "/api/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
}

func TestHealthReady_503WhenListenerNotBound(t *testing.T) {
	cfg := &config.ServerConfig{}
	cfg.Transport.H3.Enabled = true
	cfg.Transport.Reality.Enabled = true

	info := &ServerInfo{}
	info.MarkListenerReady("h3") // reality NOT marked

	mux := http.NewServeMux()
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	registerHealthRoutes(mux, info, cfg, nil, hb)

	req := httptest.NewRequest("GET", "/api/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	var body healthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Checks["listener_reality"].Status != "fail" {
		t.Fatalf("listener_reality should be fail, got %v", body.Checks["listener_reality"])
	}
	if body.Checks["listener_h3"].Status != "ok" {
		t.Fatalf("listener_h3 should be ok, got %v", body.Checks["listener_h3"])
	}
}

func TestHealthReady_503WhenConfigNil(t *testing.T) {
	mux := http.NewServeMux()
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	registerHealthRoutes(mux, &ServerInfo{}, nil, nil, hb)

	req := httptest.NewRequest("GET", "/api/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}
