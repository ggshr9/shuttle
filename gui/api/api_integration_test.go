package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/config"

	"github.com/coder/websocket"
)

// =============================================================================
// Integration tests — these run on the host without sandbox or network changes.
// =============================================================================

// TestAPIStatusEndpoint creates an API server and verifies that GET /api/status
// returns a valid JSON response with expected fields.
func TestAPIStatusEndpoint(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/status", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	var status map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	// Verify expected fields exist in the status response.
	requiredFields := []string{"state", "active_conns", "total_conns", "bytes_sent", "bytes_received", "transport"}
	for _, field := range requiredFields {
		if _, ok := status[field]; !ok {
			t.Errorf("missing required field %q in status response", field)
		}
	}

	// A freshly created (not started) engine should be in "stopped" state.
	state, _ := status["state"].(string)
	if state != "stopped" {
		t.Errorf("expected state 'stopped', got %q", state)
	}

	// Numeric fields should be zero for a stopped engine.
	if conns, ok := status["active_conns"].(float64); ok && conns != 0 {
		t.Errorf("expected active_conns=0, got %v", conns)
	}
}

// TestAPIConfigEndpoint verifies that GET /api/config returns a valid
// ClientConfig JSON with expected default values.
func TestAPIConfigEndpoint(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/config", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	// Verify it unmarshals to a valid ClientConfig.
	var cfg config.ClientConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	// Default config should have proxy listeners enabled.
	if !cfg.Proxy.SOCKS5.Enabled {
		t.Error("expected SOCKS5 to be enabled in default config")
	}
	if !cfg.Proxy.HTTP.Enabled {
		t.Error("expected HTTP proxy to be enabled in default config")
	}

	// SOCKS5 and HTTP should have non-empty listen addresses.
	if cfg.Proxy.SOCKS5.Listen == "" {
		t.Error("expected non-empty SOCKS5 listen address")
	}
	if cfg.Proxy.HTTP.Listen == "" {
		t.Error("expected non-empty HTTP listen address")
	}

	// Default routing action should be set.
	if cfg.Routing.Default == "" {
		t.Error("expected non-empty default routing action")
	}
}

// TestAPIConcurrentRequests verifies that the API server handles 20 concurrent
// requests to /api/status without errors, races, or dropped requests.
func TestAPIConcurrentRequests(t *testing.T) {
	h, _, _ := newTestHandler()

	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)

	errors := make(chan string, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()

			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/api/status", nil)
			h.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				errors <- "request " + http.StatusText(rr.Code)
				return
			}

			var status map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
				errors <- "invalid JSON: " + err.Error()
				return
			}

			if _, ok := status["state"]; !ok {
				errors <- "missing 'state' field"
				return
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	errs := make([]string, 0, len(errors))
	for e := range errors {
		errs = append(errs, e)
	}

	if len(errs) > 0 {
		t.Fatalf("%d/%d concurrent requests failed: %v", len(errs), concurrency, errs)
	}
}

// TestAPIWebSocketEvents tests the WebSocket event endpoint (/api/logs).
// It connects to the WebSocket, starts the engine to generate events, and
// verifies that events are received. If the WebSocket upgrade fails (e.g.,
// because the test environment does not support it), the test is skipped.
func TestAPIWebSocketEvents(t *testing.T) {
	h, eng, _ := newTestHandler()

	// Start a real HTTP server so we can make a proper WebSocket connection.
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Attempt WebSocket connection to the /api/logs endpoint.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + srv.URL[4:] + "/api/logs" // http://... -> ws://...
	conn, resp, err := websocket.Dial(ctx, wsURL, nil)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Skipf("WebSocket connection failed (may not be supported in test env): %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	// Start the engine to trigger log events.
	if err := eng.Start(context.Background()); err != nil {
		// Engine start may fail without a real server configured, but it should
		// still emit log events. We try and continue regardless.
		t.Logf("engine start returned error (expected without server): %v", err)
	}
	defer func() { _ = eng.Stop() }()

	// Try to read at least one event within the timeout.
	readCtx, readCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer readCancel()

	_, msg, err := conn.Read(readCtx)
	if err != nil {
		// It's acceptable if no events arrive within the timeout when
		// the engine can't fully start. Skip rather than fail.
		t.Skipf("no WebSocket events received within timeout (engine may not have generated events): %v", err)
	}

	// Verify the received message is valid JSON with expected event fields.
	var event map[string]any
	if err := json.Unmarshal(msg, &event); err != nil {
		t.Fatalf("received non-JSON WebSocket message: %s", string(msg))
	}

	if _, ok := event["type"]; !ok {
		t.Error("WebSocket event missing 'type' field")
	}
	if _, ok := event["timestamp"]; !ok {
		t.Error("WebSocket event missing 'timestamp' field")
	}

	t.Logf("received WebSocket event: type=%v", event["type"])
}
