package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shuttle-proxy/shuttle/config"
)

func testSetup() (*ServerInfo, *config.ServerConfig) {
	info := &ServerInfo{
		StartTime: time.Now(),
		Version:   "0.1.0-test",
	}
	info.ActiveConns.Store(5)
	info.TotalConns.Store(100)
	info.BytesSent.Store(1024)
	info.BytesRecv.Store(2048)

	cfg := config.DefaultServerConfig()
	cfg.Auth.Password = "testpass"
	cfg.Auth.PrivateKey = "deadbeef"
	cfg.Auth.PublicKey = "cafebabe"
	cfg.Admin.Enabled = true
	cfg.Admin.Listen = "127.0.0.1:9090"
	cfg.Admin.Token = "test-token-abc123"

	return info, cfg
}

func TestHealthNoAuth(t *testing.T) {
	info, cfg := testSetup()
	handler := Handler(info, cfg, "", NewUserStore(cfg.Admin.Users), nil)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/health status = %d, want 200", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want ok", resp["status"])
	}
}

func TestStatusRequiresAuth(t *testing.T) {
	info, cfg := testSetup()
	handler := Handler(info, cfg, "", NewUserStore(cfg.Admin.Users), nil)

	// No auth header
	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("no-auth status = %d, want 401", w.Code)
	}

	// Wrong token
	req = httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("wrong-token status = %d, want 401", w.Code)
	}
}

func TestStatusWithAuth(t *testing.T) {
	info, cfg := testSetup()
	handler := Handler(info, cfg, "", NewUserStore(cfg.Admin.Users), nil)

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer test-token-abc123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/status status = %d, want 200", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["version"] != "0.1.0-test" {
		t.Errorf("version = %v, want 0.1.0-test", resp["version"])
	}
	if resp["active_conns"] != float64(5) {
		t.Errorf("active_conns = %v, want 5", resp["active_conns"])
	}
	if resp["total_conns"] != float64(100) {
		t.Errorf("total_conns = %v, want 100", resp["total_conns"])
	}
}

func TestConfigRedactsSecrets(t *testing.T) {
	info, cfg := testSetup()
	handler := Handler(info, cfg, "", NewUserStore(cfg.Admin.Users), nil)

	req := httptest.NewRequest("GET", "/api/config", nil)
	req.Header.Set("Authorization", "Bearer test-token-abc123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/config status = %d, want 200", w.Code)
	}

	var resp config.ServerConfig
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Auth.Password != "***" {
		t.Errorf("password not redacted: %q", resp.Auth.Password)
	}
	if resp.Auth.PrivateKey != "***" {
		t.Errorf("private_key not redacted: %q", resp.Auth.PrivateKey)
	}
	if resp.Admin.Token != "***" {
		t.Errorf("admin token not redacted: %q", resp.Admin.Token)
	}
}

func TestShareURI(t *testing.T) {
	info, cfg := testSetup()
	cfg.Transport.H3.Enabled = true
	cfg.Transport.Reality.Enabled = true
	cfg.Transport.Reality.ShortIDs = []string{"abcd1234"}
	handler := Handler(info, cfg, "", NewUserStore(cfg.Admin.Users), nil)

	req := httptest.NewRequest("GET", "/api/share?addr=example.com:443", nil)
	req.Header.Set("Authorization", "Bearer test-token-abc123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/share status = %d, want 200", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["uri"] == "" {
		t.Error("uri is empty")
	}
	if resp["password"] != "testpass" {
		t.Errorf("password = %q, want testpass", resp["password"])
	}
}

func TestMetrics(t *testing.T) {
	info, cfg := testSetup()
	handler := Handler(info, cfg, "", NewUserStore(cfg.Admin.Users), nil)

	req := httptest.NewRequest("GET", "/api/metrics", nil)
	req.Header.Set("Authorization", "Bearer test-token-abc123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/metrics status = %d, want 200", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["bytes_sent"] != float64(1024) {
		t.Errorf("bytes_sent = %v, want 1024", resp["bytes_sent"])
	}
	if resp["goroutines"] == nil {
		t.Error("goroutines missing")
	}
}
