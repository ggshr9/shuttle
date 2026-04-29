package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ggshr9/shuttle/config"
	"github.com/ggshr9/shuttle/engine"
)

func TestServersList_Pagination(t *testing.T) {
	eng := newTestEngine()

	// Seed 120 servers.
	cfg := eng.Config()
	for i := 0; i < 120; i++ {
		cfg.Servers = append(cfg.Servers, config.ServerEndpoint{
			Addr: "host" + string(rune('a'+i%26)) + ":1234",
			Name: "s",
		})
	}
	eng.SetConfig(&cfg)

	h := newTestHandlerFromEngine(eng)

	// Page 0 of size 50 — expect 50 servers, total=120.
	rr := doRequest(h, "GET", "/api/config/servers?page=0&size=50", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Servers []config.ServerEndpoint `json:"servers"`
		Total   int                     `json:"total"`
		Page    int                     `json:"page"`
		Size    int                     `json:"size"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Servers) != 50 {
		t.Fatalf("page 0 size 50: got %d servers, want 50", len(body.Servers))
	}
	if body.Total != 120 {
		t.Fatalf("total = %d, want 120", body.Total)
	}
	if body.Page != 0 {
		t.Fatalf("page = %d, want 0", body.Page)
	}
	if body.Size != 50 {
		t.Fatalf("size = %d, want 50", body.Size)
	}

	// Page 2 of size 50 — expect 20 servers (120 mod 50 = 20).
	rr = doRequest(h, "GET", "/api/config/servers?page=2&size=50", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Servers) != 20 {
		t.Fatalf("page 2 size 50: got %d servers, want 20", len(body.Servers))
	}
	if body.Total != 120 {
		t.Fatalf("total = %d, want 120", body.Total)
	}
}

func TestServersList_PaginationOutOfRange(t *testing.T) {
	eng := newTestEngine()
	cfg := eng.Config()
	cfg.Servers = []config.ServerEndpoint{{Addr: "a:1", Name: "x"}}
	eng.SetConfig(&cfg)

	h := newTestHandlerFromEngine(eng)

	// Page way beyond end — expect empty servers slice, not 404.
	rr := doRequest(h, "GET", "/api/config/servers?page=99&size=50", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for out-of-range page, got %d", rr.Code)
	}
	var body struct {
		Servers []config.ServerEndpoint `json:"servers"`
		Total   int                     `json:"total"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Servers) != 0 {
		t.Fatalf("expected empty servers for out-of-range page, got %d", len(body.Servers))
	}
	if body.Total != 1 {
		t.Fatalf("total = %d, want 1", body.Total)
	}
}

func TestServersList_SizeCappedAt200(t *testing.T) {
	eng := newTestEngine()
	cfg := eng.Config()
	for i := 0; i < 250; i++ {
		cfg.Servers = append(cfg.Servers, config.ServerEndpoint{Addr: "h:1", Name: "s"})
	}
	eng.SetConfig(&cfg)

	h := newTestHandlerFromEngine(eng)

	// Request size=500 — should be capped to 200.
	rr := doRequest(h, "GET", "/api/config/servers?page=0&size=500", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body struct {
		Servers []config.ServerEndpoint `json:"servers"`
		Size    int                     `json:"size"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Servers) != 200 {
		t.Fatalf("expected 200 servers (capped), got %d", len(body.Servers))
	}
	if body.Size != 200 {
		t.Fatalf("size field = %d, want 200", body.Size)
	}
}

func TestServersList_LegacyNoParams(t *testing.T) {
	eng := newTestEngine()
	cfg := eng.Config()
	cfg.Servers = []config.ServerEndpoint{{Addr: "a:1", Name: "x"}, {Addr: "b:2", Name: "y"}}
	eng.SetConfig(&cfg)

	h := newTestHandlerFromEngine(eng)

	rr := doRequest(h, "GET", "/api/config/servers", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body struct {
		Servers []config.ServerEndpoint `json:"servers"`
		Total   int                     `json:"total"` // should be zero when not paginated
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Servers) != 2 {
		t.Fatalf("legacy: got %d servers, want 2", len(body.Servers))
	}
	// Total field must be absent (decodes as zero) in legacy mode.
	if body.Total != 0 {
		t.Fatalf("legacy: total field should be absent (0), got %d", body.Total)
	}
}

// newTestHandlerFromEngine creates an API handler backed by the given engine.
func newTestHandlerFromEngine(eng *engine.Engine) http.Handler {
	return NewHandler(HandlerConfig{Engine: eng})
}

// Ensure the test file is parsed correctly when the httptest server variant
// is needed (pagination uses a real ServeMux so doRequest works fine here).
var _ = httptest.NewRecorder
