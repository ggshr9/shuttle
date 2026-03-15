package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/engine"
	"github.com/shuttle-proxy/shuttle/subscription"
)

// newTestEngine creates a stopped engine with default config for testing.
// The engine is never started, so no network listeners or system state changes occur.
func newTestEngine() *engine.Engine {
	cfg := config.DefaultClientConfig()
	return engine.New(cfg)
}

// newTestHandler creates an API handler backed by a stopped engine and fresh subscription manager.
func newTestHandler() (http.Handler, *engine.Engine, *subscription.Manager) {
	eng := newTestEngine()
	subMgr := subscription.NewManager()
	h := NewHandler(HandlerConfig{Engine: eng, SubMgr: subMgr})
	return h, eng, subMgr
}

// doRequest is a helper that performs an HTTP request against the handler and returns the recorder.
func doRequest(h http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestAPIStatus(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/status", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	state, ok := status["state"].(string)
	if !ok {
		t.Fatal("missing 'state' field in response")
	}
	if state != "stopped" {
		t.Fatalf("expected state 'stopped', got %q", state)
	}
}

func TestAPIConfig(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/config", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var cfg config.ClientConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("invalid config JSON: %v", err)
	}

	// Default config should have SOCKS5 and HTTP enabled
	if !cfg.Proxy.SOCKS5.Enabled {
		t.Error("expected SOCKS5 to be enabled in default config")
	}
	if !cfg.Proxy.HTTP.Enabled {
		t.Error("expected HTTP proxy to be enabled in default config")
	}
}

func TestAPIConfigServers_CRUD(t *testing.T) {
	h, _, _ := newTestHandler()

	// Initially no servers in the saved list
	rr := doRequest(h, "GET", "/api/config/servers", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET servers: expected 200, got %d", rr.Code)
	}
	var initial map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &initial)
	servers, _ := initial["servers"].([]interface{})
	if len(servers) != 0 {
		t.Fatalf("expected 0 servers initially, got %d", len(servers))
	}

	// Add a server via POST
	rr = doRequest(h, "POST", "/api/config/servers", `{"addr":"test.example.com:443","name":"Test Server"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST server: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify it appears in the list
	rr = doRequest(h, "GET", "/api/config/servers", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET servers: expected 200, got %d", rr.Code)
	}
	var afterAdd map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &afterAdd)
	servers, _ = afterAdd["servers"].([]interface{})
	if len(servers) != 1 {
		t.Fatalf("expected 1 server after add, got %d", len(servers))
	}

	// Adding duplicate should fail with 409
	rr = doRequest(h, "POST", "/api/config/servers", `{"addr":"test.example.com:443","name":"Dup"}`)
	if rr.Code != http.StatusConflict {
		t.Fatalf("POST duplicate: expected 409, got %d", rr.Code)
	}

	// Delete the server
	rr = doRequest(h, "DELETE", "/api/config/servers", `{"addr":"test.example.com:443"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE server: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify it's gone
	rr = doRequest(h, "GET", "/api/config/servers", "")
	var afterDel map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &afterDel)
	servers, _ = afterDel["servers"].([]interface{})
	if len(servers) != 0 {
		t.Fatalf("expected 0 servers after delete, got %d", len(servers))
	}

	// Deleting non-existent server should return 404
	rr = doRequest(h, "DELETE", "/api/config/servers", `{"addr":"nonexistent:443"}`)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("DELETE nonexistent: expected 404, got %d", rr.Code)
	}
}

func TestAPIConfigServers_PostMissingAddr(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "POST", "/api/config/servers", `{"name":"No Address"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing addr, got %d", rr.Code)
	}
}

func TestAPISubscriptions_CRUD(t *testing.T) {
	h, _, _ := newTestHandler()

	// Initially empty
	rr := doRequest(h, "GET", "/api/subscriptions", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET subscriptions: expected 200, got %d", rr.Code)
	}
	var initial []interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &initial)
	if len(initial) != 0 {
		t.Fatalf("expected 0 subscriptions initially, got %d", len(initial))
	}

	// Add a subscription (URL won't be fetched successfully, but the entry is created)
	rr = doRequest(h, "POST", "/api/subscriptions", `{"name":"Test Sub","url":"https://example.com/sub"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST subscription: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Extract the subscription ID from the response
	var addedSub map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &addedSub)
	subID, ok := addedSub["id"].(string)
	if !ok || subID == "" {
		t.Fatal("expected subscription ID in response")
	}

	// List should now contain 1 subscription
	rr = doRequest(h, "GET", "/api/subscriptions", "")
	var afterAdd []interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &afterAdd)
	if len(afterAdd) != 1 {
		t.Fatalf("expected 1 subscription after add, got %d", len(afterAdd))
	}

	// Delete the subscription
	rr = doRequest(h, "DELETE", "/api/subscriptions/"+subID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE subscription: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// List should be empty again
	rr = doRequest(h, "GET", "/api/subscriptions", "")
	var afterDel []interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &afterDel)
	if len(afterDel) != 0 {
		t.Fatalf("expected 0 subscriptions after delete, got %d", len(afterDel))
	}
}

func TestAPISubscriptions_PostMissingURL(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "POST", "/api/subscriptions", `{"name":"No URL"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing url, got %d", rr.Code)
	}
}

func TestAPIVersion(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/version", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := result["version"]; !ok {
		t.Fatal("missing 'version' field in response")
	}
}

func TestAPIRoutingRules(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/routing/rules", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var routing config.RoutingConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &routing); err != nil {
		t.Fatalf("invalid routing JSON: %v", err)
	}

	// Default routing should have "proxy" as default action
	if routing.Default != "proxy" {
		t.Fatalf("expected default routing action 'proxy', got %q", routing.Default)
	}
}

func TestAPIRoutingTemplates(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/routing/templates", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var templates []map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &templates); err != nil {
		t.Fatalf("invalid templates JSON: %v", err)
	}

	if len(templates) == 0 {
		t.Fatal("expected at least one routing template")
	}

	// Check known template IDs
	ids := make(map[string]bool)
	for _, tmpl := range templates {
		if id, ok := tmpl["id"].(string); ok {
			ids[id] = true
		}
	}
	for _, expected := range []string{"bypass-cn", "proxy-all", "direct-all", "block-ads"} {
		if !ids[expected] {
			t.Errorf("missing expected template %q", expected)
		}
	}
}

func TestAPIStatsHistory(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/stats/history", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should have history and total fields (even if empty)
	if _, ok := result["history"]; !ok {
		t.Error("missing 'history' field")
	}
	if _, ok := result["total"]; !ok {
		t.Error("missing 'total' field")
	}
}

func TestAPIStatsHistory_CustomDays(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/stats/history?days=30", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestAPIConnectDoubleStart(t *testing.T) {
	h, eng, _ := newTestHandler()

	// First connect should succeed.
	rr := doRequest(h, "POST", "/api/connect", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for first connect, got %d: %s", rr.Code, rr.Body.String())
	}
	defer func() { _ = eng.Stop() }()

	// Second connect on an already-running engine should return 409.
	rr = doRequest(h, "POST", "/api/connect", "")
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 for double connect, got %d: %s", rr.Code, rr.Body.String())
	}

	var result map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := result["error"]; !ok {
		t.Error("expected 'error' field in response")
	}
}

func TestAPIInvalidJSON(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "PUT", "/api/config", `{invalid json`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", rr.Code)
	}

	var result map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid error JSON: %v", err)
	}

	if _, ok := result["error"]; !ok {
		t.Error("expected 'error' field in response")
	}
}

func TestAPIInvalidJSON_Servers(t *testing.T) {
	h, _, _ := newTestHandler()

	// POST with bad JSON
	rr := doRequest(h, "POST", "/api/config/servers", `not json`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	// PUT with bad JSON
	rr = doRequest(h, "PUT", "/api/config/servers", `not json`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	// DELETE with bad JSON
	rr = doRequest(h, "DELETE", "/api/config/servers", `not json`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAPIConnectionsHistory(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/connections/history", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Should return an empty array since no connStore is configured
	var entries []interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &entries); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty connections history, got %d entries", len(entries))
	}
}

func TestAPIConfigExport_JSON(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/config/export?format=json", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	var cfg config.ClientConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("invalid exported config JSON: %v", err)
	}
}

func TestAPIConfigExport_UnsupportedFormat(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/config/export?format=xml", "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported format, got %d", rr.Code)
	}
}

func TestAPIBackup(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/backup", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var backup map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &backup); err != nil {
		t.Fatalf("invalid backup JSON: %v", err)
	}

	// Backup should have version, config, and subscriptions fields
	if v, ok := backup["version"].(float64); !ok || v != 1 {
		t.Errorf("expected version 1, got %v", backup["version"])
	}
	if _, ok := backup["config"]; !ok {
		t.Error("missing 'config' in backup")
	}
	if _, ok := backup["subscriptions"]; !ok {
		t.Error("missing 'subscriptions' in backup")
	}
}

func TestAPINetworkLAN(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/network/lan", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should have expected fields
	for _, key := range []string{"allow_lan", "addresses", "socks5", "http"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing %q field in LAN info", key)
		}
	}
}

func TestAPIGeodataStatus(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/geodata/status", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestAPIGeodataSources(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/geodata/sources", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Should return a non-empty list of presets
	var presets []interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &presets); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(presets) == 0 {
		t.Error("expected at least one geodata source preset")
	}
}

func TestAPILogsExport(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/logs/export", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("expected text/plain content type, got %q", ct)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Engine Status") {
		t.Error("expected log export to contain 'Engine Status'")
	}
	if !strings.Contains(body, "stopped") {
		t.Error("expected log export to reflect stopped state")
	}
}

func TestAPICORSHeaders(t *testing.T) {
	h, _, _ := newTestHandler()

	// Request with localhost origin should get CORS headers
	req := httptest.NewRequest("OPTIONS", "/api/status", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS, got %d", rr.Code)
	}

	origin := rr.Header().Get("Access-Control-Allow-Origin")
	if origin != "http://localhost:5173" {
		t.Fatalf("expected CORS origin http://localhost:5173, got %q", origin)
	}
}

func TestAPICORSHeaders_NonLocalhost(t *testing.T) {
	h, _, _ := newTestHandler()

	// Request with non-localhost origin should NOT get Access-Control-Allow-Origin
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	origin := rr.Header().Get("Access-Control-Allow-Origin")
	if origin != "" {
		t.Fatalf("expected no CORS origin for non-localhost, got %q", origin)
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestNormalizeListenAddr(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{":1080", "127.0.0.1:1080"},
		{"0.0.0.0:1080", "127.0.0.1:1080"},
		{":::1080", "127.0.0.1:1080"},
		{"192.168.1.1:1080", "192.168.1.1:1080"},
		{"127.0.0.1:8080", "127.0.0.1:8080"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeListenAddr(tc.input)
			if got != tc.expected {
				t.Fatalf("normalizeListenAddr(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestValidateProbeURL(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{"https://example.com", false},
		{"http://example.com/path", false},
		{"ftp://example.com", true},         // non-http scheme
		{"", true},                           // empty
		{"http://localhost/", true},          // localhost
		{"http://127.0.0.1/", true},         // loopback IP
		{"http://169.254.169.254/", true},   // metadata endpoint
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			err := validateProbeURL(tc.url)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.url, err)
			}
		})
	}
}

func TestAPIDeleteServers_EmptyAddr(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "DELETE", "/api/config/servers", `{"addr":""}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty addr, got %d", rr.Code)
	}
}

func TestAPIResponseContentType(t *testing.T) {
	h, _, _ := newTestHandler()

	// All JSON endpoints should return application/json
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/status"},
		{"GET", "/api/config"},
		{"GET", "/api/version"},
		{"GET", "/api/routing/rules"},
		{"GET", "/api/routing/templates"},
		{"GET", "/api/stats/history"},
		{"GET", "/api/subscriptions"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			rr := doRequest(h, ep.method, ep.path, "")
			ct := rr.Header().Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Fatalf("expected application/json, got %q", ct)
			}
		})
	}
}

func TestAPIDebugState(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/debug/state", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	for _, key := range []string{"engine_state", "circuit_breaker", "transport", "uptime_seconds", "goroutines"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing %q field in debug state", key)
		}
	}

	if state, ok := result["engine_state"].(string); !ok || state != "stopped" {
		t.Fatalf("expected engine_state 'stopped', got %v", result["engine_state"])
	}
	if g, ok := result["goroutines"].(float64); !ok || g <= 0 {
		t.Fatalf("expected goroutines > 0, got %v", result["goroutines"])
	}
}

func TestAPIConfigValidate(t *testing.T) {
	h, _, _ := newTestHandler()

	// Valid config — disable transports that require additional fields (domain, keys)
	validCfg := config.DefaultClientConfig()
	validCfg.Transport.CDN.Enabled = false
	validCfg.Transport.Reality.Enabled = false
	body, _ := json.Marshal(validCfg)
	rr := doRequest(h, "POST", "/api/config/validate", string(body))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var valid map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &valid); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if v, ok := valid["valid"].(bool); !ok || !v {
		t.Fatalf("expected valid:true for default config, got %v; errors: %v", valid["valid"], valid["errors"])
	}
	errs, _ := valid["errors"].([]interface{})
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors for valid config, got %d: %v", len(errs), errs)
	}

	// Invalid config — bad transport preference
	invalidCfg := config.DefaultClientConfig()
	invalidCfg.Transport.Preferred = "invalid_transport"
	body, _ = json.Marshal(invalidCfg)
	rr = doRequest(h, "POST", "/api/config/validate", string(body))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var invalid map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &invalid); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if v, ok := invalid["valid"].(bool); !ok || v {
		t.Fatalf("expected valid:false for invalid config, got %v", invalid["valid"])
	}
	errs, _ = invalid["errors"].([]interface{})
	if len(errs) == 0 {
		t.Fatal("expected at least 1 error for invalid config")
	}
}

func TestAPISystemResources(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/system/resources", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	for _, key := range []string{"goroutines", "mem_alloc_mb", "mem_sys_mb", "mem_gc_cycles", "num_cpu", "uptime_seconds"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing %q field in system resources", key)
		}
	}

	if g, ok := result["goroutines"].(float64); !ok || g <= 0 {
		t.Fatalf("expected goroutines > 0, got %v", result["goroutines"])
	}
	if cpu, ok := result["num_cpu"].(float64); !ok || cpu <= 0 {
		t.Fatalf("expected num_cpu > 0, got %v", result["num_cpu"])
	}
}

func TestAPIRoutingTest(t *testing.T) {
	h, _, _ := newTestHandler()

	// Test with a bare domain
	rr := doRequest(h, "POST", "/api/routing/test", `{"url":"example.com"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should have all expected fields
	for _, key := range []string{"domain", "action", "matched_by"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing %q field in routing test result", key)
		}
	}

	domain, _ := result["domain"].(string)
	if domain != "example.com" {
		t.Errorf("expected domain 'example.com', got %q", domain)
	}

	// Default config uses "proxy" as default action
	action, _ := result["action"].(string)
	if action != "proxy" {
		t.Errorf("expected action 'proxy' for default config, got %q", action)
	}

	matchedBy, _ := result["matched_by"].(string)
	if matchedBy != "default" {
		t.Errorf("expected matched_by 'default', got %q", matchedBy)
	}
}

func TestAPIRoutingTest_FullURL(t *testing.T) {
	h, _, _ := newTestHandler()

	// Test with a full URL — domain should be extracted
	rr := doRequest(h, "POST", "/api/routing/test", `{"url":"https://example.com/path?query=1"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	domain, _ := result["domain"].(string)
	if domain != "example.com" {
		t.Errorf("expected domain 'example.com' extracted from URL, got %q", domain)
	}
}

func TestAPIRoutingTest_MissingURL(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "POST", "/api/routing/test", `{"url":""}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty url, got %d", rr.Code)
	}
}

func TestAPITransportStats(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/transports/stats", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Engine is stopped, so transport breakdown is empty — should be a valid JSON array.
	var stats []interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &stats); err != nil {
		t.Fatalf("invalid JSON array: %v", err)
	}
}

func TestAPIConnectionStreams(t *testing.T) {
	h, _, _ := newTestHandler()

	// Engine is stopped so StreamTracker is nil — should return empty array.
	rr := doRequest(h, "GET", "/api/connections/test-conn-id/streams", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var streams []interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &streams); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(streams) != 0 {
		t.Fatalf("expected empty streams list, got %d", len(streams))
	}
}

func TestAPIConnectionStreams_BadPath(t *testing.T) {
	h, _, _ := newTestHandler()

	// Missing /streams suffix should return 404.
	rr := doRequest(h, "GET", "/api/connections/test-id/invalid", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// TestAPIWriteJSON_Encoding verifies the writeJSON helper produces valid JSON
// by checking a known endpoint's output is parseable.
func TestAPIWriteJSON_Encoding(t *testing.T) {
	h, _, _ := newTestHandler()

	rr := doRequest(h, "GET", "/api/status", "")
	if !json.Valid(bytes.TrimSpace(rr.Body.Bytes())) {
		t.Fatalf("response is not valid JSON: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Auth middleware tests
// ---------------------------------------------------------------------------

// newAuthHandler creates an authenticated API handler for testing.
func newAuthHandler(token string) (http.Handler, *engine.Engine) {
	eng := newTestEngine()
	h := NewHandler(HandlerConfig{Engine: eng, AuthToken: token})
	return h, eng
}

// doAuthRequest performs a request with an Authorization: Bearer header.
func doAuthRequest(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	token := "test-secret-token-12345"
	h, _ := newAuthHandler(token)

	rr := doAuthRequest(h, "GET", "/api/status", token, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid token, got %d: %s", rr.Code, rr.Body.String())
	}

	var status map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := status["state"]; !ok {
		t.Fatal("missing 'state' in response")
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	token := "correct-token"
	h, _ := newAuthHandler(token)

	rr := doAuthRequest(h, "GET", "/api/status", "wrong-token", "")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong token, got %d", rr.Code)
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	token := "correct-token"
	h, _ := newAuthHandler(token)

	// No Authorization header at all
	rr := doAuthRequest(h, "GET", "/api/status", "", "")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing token, got %d", rr.Code)
	}

	var result map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result["error"] != "unauthorized" {
		t.Fatalf("expected error 'unauthorized', got %q", result["error"])
	}
}

func TestAuthMiddleware_QueryParam(t *testing.T) {
	token := "ws-token-test"
	h, _ := newAuthHandler(token)

	// WebSocket-style ?token= query parameter (no Authorization header)
	rr := doAuthRequest(h, "GET", "/api/status?token="+token, "", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid query param token, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthMiddleware_StaticFiles(t *testing.T) {
	token := "secret-token"
	h, _ := newAuthHandler(token)

	// Non-API paths (e.g. static files) should be exempt from auth.
	// These will likely return 404 since there's no static file handler,
	// but the key assertion is they do NOT return 401.
	rr := doAuthRequest(h, "GET", "/index.html", "", "")
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("static file path should be exempt from auth, but got 401")
	}

	rr = doAuthRequest(h, "GET", "/assets/style.css", "", "")
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("static file path should be exempt from auth, but got 401")
	}
}
