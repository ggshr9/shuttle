package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/config"
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
	handler := Handler(info, cfg, "", NewUserStore(cfg.Admin.Users), nil, nil, nil)

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
	handler := Handler(info, cfg, "", NewUserStore(cfg.Admin.Users), nil, nil, nil)

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
	handler := Handler(info, cfg, "", NewUserStore(cfg.Admin.Users), nil, nil, nil)

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
	handler := Handler(info, cfg, "", NewUserStore(cfg.Admin.Users), nil, nil, nil)

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
	handler := Handler(info, cfg, "", NewUserStore(cfg.Admin.Users), nil, nil, nil)

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
	handler := Handler(info, cfg, "", NewUserStore(cfg.Admin.Users), nil, nil, nil)

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

func TestBackup(t *testing.T) {
	info, cfg := testSetup()
	users := NewUserStore([]config.User{
		{Name: "alice", Token: "tok-alice", MaxBytes: 1000, Enabled: true},
		{Name: "bob", Token: "tok-bob", MaxBytes: 0, Enabled: false},
	})
	handler := Handler(info, cfg, "", users, nil, nil, nil)

	req := httptest.NewRequest("GET", "/api/backup", nil)
	req.Header.Set("Authorization", "Bearer test-token-abc123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/backup status = %d, want 200", w.Code)
	}

	var backup BackupPayload
	if err := json.NewDecoder(w.Body).Decode(&backup); err != nil {
		t.Fatalf("decode backup: %v", err)
	}

	if backup.Version != 1 {
		t.Errorf("version = %d, want 1", backup.Version)
	}
	if backup.Timestamp == "" {
		t.Error("timestamp is empty")
	}
	if len(backup.Users) != 2 {
		t.Fatalf("users count = %d, want 2", len(backup.Users))
	}
	if backup.Config == nil {
		t.Fatal("config is nil")
	}
	// Secrets should be redacted
	if backup.Config.Auth.Password != "***" {
		t.Errorf("password not redacted: %q", backup.Config.Auth.Password)
	}
	if backup.Config.Auth.PrivateKey != "***" {
		t.Errorf("private_key not redacted: %q", backup.Config.Auth.PrivateKey)
	}
	if backup.Config.Admin.Token != "***" {
		t.Errorf("admin token not redacted: %q", backup.Config.Admin.Token)
	}
}

func TestRestoreUsers(t *testing.T) {
	info, cfg := testSetup()
	users := NewUserStore([]config.User{
		{Name: "old-user", Token: "tok-old", MaxBytes: 0, Enabled: true},
	})
	handler := Handler(info, cfg, "", users, nil, nil, nil)

	body := `{
		"version": 1,
		"users": [
			{"name": "alice", "token": "tok-alice", "max_bytes": 5000, "enabled": true},
			{"name": "bob", "token": "tok-bob", "max_bytes": 0, "enabled": false}
		]
	}`
	req := httptest.NewRequest("POST", "/api/restore", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token-abc123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/restore status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "restored" {
		t.Errorf("status = %q, want restored", resp["status"])
	}
	if resp["count"] != "2" {
		t.Errorf("count = %q, want 2", resp["count"])
	}

	// Verify old user is gone and new users exist
	listed := users.List()
	if len(listed) != 2 {
		t.Fatalf("user count = %d, want 2", len(listed))
	}

	// Check that alice is authenticated
	if u := users.Authenticate("tok-alice"); u == nil {
		t.Error("alice should be authenticated")
	}
	// Check that bob is disabled
	if u := users.Authenticate("tok-bob"); u != nil {
		t.Error("bob should not be authenticated (disabled)")
	}
	// Old user should be gone
	if u := users.Authenticate("tok-old"); u != nil {
		t.Error("old-user should have been removed")
	}
}

func TestRestoreInvalidJSON(t *testing.T) {
	info, cfg := testSetup()
	users := NewUserStore(cfg.Admin.Users)
	handler := Handler(info, cfg, "", users, nil, nil, nil)

	req := httptest.NewRequest("POST", "/api/restore", strings.NewReader("{not valid json"))
	req.Header.Set("Authorization", "Bearer test-token-abc123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /api/restore with bad JSON status = %d, want 400", w.Code)
	}
}
