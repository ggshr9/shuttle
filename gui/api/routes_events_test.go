package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEventsHandler_GET_ReturnsEvents(t *testing.T) {
	q := NewEventQueue(8)
	q.Push("ping", map[string]any{"x": 1})

	mux := http.NewServeMux()
	registerEventsRoutes(mux, q)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/events?since=0&max=10")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	var body struct {
		Events []Event `json:"events"`
		Cursor int64   `json:"cursor"`
		Gap    bool    `json:"gap"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Events) != 1 || body.Events[0].Type != "ping" {
		t.Fatalf("got %+v", body.Events)
	}
	if body.Cursor != 1 {
		t.Fatalf("cursor = %d, want 1", body.Cursor)
	}
}

func TestEventsHandler_BadSince_400(t *testing.T) {
	q := NewEventQueue(8)
	mux := http.NewServeMux()
	registerEventsRoutes(mux, q)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/events?since=abc")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status %d, want 400", res.StatusCode)
	}
	body := make([]byte, 256)
	n, _ := res.Body.Read(body)
	if !strings.Contains(string(body[:n]), "since") {
		t.Fatalf("body should mention 'since': %s", body[:n])
	}
}

func TestEventsHandler_NilQueue_404(t *testing.T) {
	// When EventQueue is nil, routes are NOT registered. Sanity check.
	mux := http.NewServeMux()
	registerEventsRoutes(mux, nil)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("status %d, want 404 when queue nil", res.StatusCode)
	}
}
