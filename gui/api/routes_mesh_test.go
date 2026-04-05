package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestMeshStatusDisabled(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/mesh/status", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if resp["enabled"] != false {
		t.Errorf("expected enabled=false, got %v", resp["enabled"])
	}
	if resp["peer_count"] != float64(0) {
		t.Errorf("expected peer_count=0, got %v", resp["peer_count"])
	}
}

func TestMeshPeersEmpty(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/mesh/peers", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var peers []any
	if err := json.Unmarshal(rr.Body.Bytes(), &peers); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("expected empty peers, got %d", len(peers))
	}
}

func TestMeshConnectNotFound(t *testing.T) {
	h, _, _ := newTestHandler()

	// Missing /connect suffix should 404.
	rr := doRequest(h, "POST", "/api/mesh/peers/10.7.0.2/something", "")
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestMeshConnectMeshDisabled(t *testing.T) {
	h, _, _ := newTestHandler()

	// Mesh is not enabled on default test engine, so connect should fail.
	rr := doRequest(h, "POST", "/api/mesh/peers/10.7.0.2/connect", "")
	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409 (mesh not enabled), got %d", rr.Code)
	}
}
