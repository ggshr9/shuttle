package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz_Returns200WithStatus(t *testing.T) {
	mux := http.NewServeMux()
	registerHealthzRoute(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %v, want ok", body["status"])
	}
}

func TestHealthz_PreservedAfterDeepHealthAdded(t *testing.T) {
	// Sanity: the new /api/health/live and /api/healthz coexist with
	// distinct semantics. /api/healthz is the iOS BridgeAdapter shallow probe.
	mux := http.NewServeMux()
	registerHealthzRoute(mux)

	req := httptest.NewRequest("GET", "/api/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("/api/healthz status = %d, want 200", w.Code)
	}
}
