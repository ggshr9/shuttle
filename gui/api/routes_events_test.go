package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
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

func TestEventsHandler_WS_StreamsEvents(t *testing.T) {
	q := NewEventQueue(8)
	mux := http.NewServeMux()
	registerEventsRoutes(mux, q)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Convert http://... → ws://...
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/events?since=0"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, resp, err := websocket.Dial(ctx, wsURL, nil)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	// Push after connection established to verify live streaming.
	q.Push("hello", map[string]any{"x": 1})

	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var payload struct {
		Events []Event `json:"events"`
		Cursor int64   `json:"cursor"`
		Gap    bool    `json:"gap"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Events) != 1 || payload.Events[0].Type != "hello" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Cursor != 1 {
		t.Fatalf("cursor = %d, want 1", payload.Cursor)
	}
}

func TestEventsHandler_WS_BadSince_400(t *testing.T) {
	q := NewEventQueue(8)
	mux := http.NewServeMux()
	registerEventsRoutes(mux, q)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// websocket.Dial returns the underlying response on handshake failure.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/events?since=abc"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected dial to fail with 400")
	}
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %v, want 400", resp)
	}
}

func TestEventsHandler_WS_ClientCancel_ServerExits(t *testing.T) {
	q := NewEventQueue(8)
	mux := http.NewServeMux()
	registerEventsRoutes(mux, q)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/events?since=0"
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer dialCancel()

	c, resp, err := websocket.Dial(dialCtx, wsURL, nil)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// CloseNow tears down the connection immediately without waiting for a
	// bidirectional close handshake. The server handler is blocked in q.Wait,
	// so it won't echo the close frame; using CloseNow avoids a 2s timeout.
	c.CloseNow()

	// Push after client disconnect — should not block forever or panic.
	done := make(chan struct{})
	go func() {
		q.Push("post-close", nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("push blocked after client disconnect")
	}
}
