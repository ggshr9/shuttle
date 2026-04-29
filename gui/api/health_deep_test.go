package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/internal/healthcheck"
)

type fakeEngine struct {
	state            string
	outboundsHealthy int
	configValid      bool
}

func (f *fakeEngine) StateName() string     { return f.state }
func (f *fakeEngine) HealthyOutbounds() int { return f.outboundsHealthy }
func (f *fakeEngine) ConfigValid() bool     { return f.configValid }

func TestClientHealthLive_OK(t *testing.T) {
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	mux := http.NewServeMux()
	registerDeepHealthRoutes(mux, &fakeEngine{}, hb)

	req := httptest.NewRequest("GET", "/api/health/live", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
}

func TestClientHealthReady_OK(t *testing.T) {
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	mux := http.NewServeMux()
	registerDeepHealthRoutes(mux, &fakeEngine{
		state:            "running",
		outboundsHealthy: 1,
		configValid:      true,
	}, hb)

	req := httptest.NewRequest("GET", "/api/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d body=%s, want 200", w.Code, w.Body.String())
	}
}

func TestClientHealthReady_FailWhenStopping(t *testing.T) {
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	mux := http.NewServeMux()
	registerDeepHealthRoutes(mux, &fakeEngine{
		state:            "stopping",
		outboundsHealthy: 1,
		configValid:      true,
	}, hb)

	req := httptest.NewRequest("GET", "/api/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", w.Code)
	}
}

func TestClientHealthReady_FailWhenNoHealthyOutbound(t *testing.T) {
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	mux := http.NewServeMux()
	registerDeepHealthRoutes(mux, &fakeEngine{
		state:            "running",
		outboundsHealthy: 0,
		configValid:      true,
	}, hb)

	req := httptest.NewRequest("GET", "/api/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	checks, _ := body["checks"].(map[string]any)
	outbound, _ := checks["outbounds"].(map[string]any)
	if outbound["status"] != "fail" {
		t.Fatalf("outbounds check should be fail, got %v", outbound)
	}
}

func TestClientHealthLive_StaleHeartbeat(t *testing.T) {
	hb := healthcheck.NewHeartbeat()
	hb.Tick()
	time.Sleep(20 * time.Millisecond)

	mux := http.NewServeMux()
	registerDeepHealthRoutesWithThreshold(mux, &fakeEngine{}, hb, 10*time.Millisecond)

	req := httptest.NewRequest("GET", "/api/health/live", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", w.Code)
	}
}
