package subscription

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/config"
)

func TestManagerAddRemove(t *testing.T) {
	m := NewManager()

	// Add subscription
	sub, err := m.Add("Test", "https://example.com/sub")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if sub.ID == "" {
		t.Error("Add() returned subscription with empty ID")
	}
	if sub.Name != "Test" {
		t.Errorf("Add() name = %q, want %q", sub.Name, "Test")
	}

	// Get subscription
	got, err := m.Get(sub.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.URL != "https://example.com/sub" {
		t.Errorf("Get() URL = %q, want %q", got.URL, "https://example.com/sub")
	}

	// List subscriptions
	list := m.List()
	if len(list) != 1 {
		t.Errorf("List() len = %d, want 1", len(list))
	}

	// Remove subscription
	if err := m.Remove(sub.ID); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	// Verify removed
	if _, err := m.Get(sub.ID); err == nil {
		t.Error("Get() after Remove() should return error")
	}
}

func TestManagerRemoveNotFound(t *testing.T) {
	m := NewManager()
	err := m.Remove("nonexistent")
	if err == nil {
		t.Error("Remove() nonexistent should return error")
	}
}

func TestManagerRefresh(t *testing.T) {
	// Create a test server
	servers := []config.ServerEndpoint{
		{Addr: "server1.example.com:443", Name: "Server 1", Password: "pass1"},
		{Addr: "server2.example.com:443", Name: "Server 2", Password: "pass2"},
	}
	data, _ := json.Marshal(servers)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer ts.Close()

	m := NewManager()
	m.SetAllowPrivateNetworks(true) // httptest uses 127.0.0.1
	sub, _ := m.Add("Test", ts.URL)

	// Refresh
	refreshed, err := m.Refresh(context.Background(), sub.ID)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	if len(refreshed.Servers) != 2 {
		t.Errorf("Refresh() servers count = %d, want 2", len(refreshed.Servers))
	}
	if refreshed.UpdatedAt.IsZero() {
		t.Error("Refresh() should set UpdatedAt")
	}
}

func TestManagerRefreshError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	m := NewManager()
	m.SetAllowPrivateNetworks(true)
	sub, _ := m.Add("Test", ts.URL)

	refreshed, err := m.Refresh(context.Background(), sub.ID)
	if err == nil {
		t.Error("Refresh() with server error should return error")
	}
	if refreshed.Error == "" {
		t.Error("Refresh() should set Error field on failure")
	}
}

func TestManagerToConfig(t *testing.T) {
	m := NewManager()
	m.Add("Sub1", "https://example.com/1")
	m.Add("Sub2", "https://example.com/2")

	configs := m.ToConfig()
	if len(configs) != 2 {
		t.Errorf("ToConfig() len = %d, want 2", len(configs))
	}
}

func TestManagerLoadFromConfig(t *testing.T) {
	m := NewManager()
	m.LoadFromConfig([]config.SubscriptionConfig{
		{ID: "id1", Name: "Sub1", URL: "https://example.com/1"},
		{Name: "Sub2", URL: "https://example.com/2"}, // No ID, should generate
	})

	list := m.List()
	if len(list) != 2 {
		t.Errorf("LoadFromConfig() len = %d, want 2", len(list))
	}

	// Check ID was generated for second one
	for _, sub := range list {
		if sub.ID == "" {
			t.Error("LoadFromConfig() should generate ID if missing")
		}
	}
}

func TestParseSIP008(t *testing.T) {
	sip008 := `{
		"version": 1,
		"servers": [
			{"server": "server1.com", "server_port": 443, "password": "pass1", "method": "aes-256-gcm", "remarks": "Server 1"},
			{"server": "server2.com", "server_port": 8443, "password": "pass2", "method": "chacha20", "remarks": "Server 2"}
		]
	}`

	servers, err := parseSIP008(sip008)
	if err != nil {
		t.Fatalf("parseSIP008() error = %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("parseSIP008() len = %d, want 2", len(servers))
	}
	if servers[0].Addr != "server1.com:443" {
		t.Errorf("parseSIP008() addr = %q, want %q", servers[0].Addr, "server1.com:443")
	}
	if servers[0].Name != "Server 1" {
		t.Errorf("parseSIP008() name = %q, want %q", servers[0].Name, "Server 1")
	}
}

func TestParseSubscriptionBase64(t *testing.T) {
	// JSON servers encoded in base64
	servers := []config.ServerEndpoint{
		{Addr: "test.com:443", Name: "Test", Password: "pass"},
	}
	data, _ := json.Marshal(servers)
	encoded := base64.StdEncoding.EncodeToString(data)

	result, err := ParseSubscription(encoded)
	if err != nil {
		t.Fatalf("ParseSubscription() error = %v", err)
	}
	if len(result) != 1 {
		t.Errorf("ParseSubscription() len = %d, want 1", len(result))
	}
}

func TestParseSubscriptionEmpty(t *testing.T) {
	_, err := ParseSubscription("")
	if err == nil {
		t.Error("ParseSubscription() empty should return error")
	}

	_, err = ParseSubscription("   ")
	if err == nil {
		t.Error("ParseSubscription() whitespace should return error")
	}
}

func TestStartStopAutoRefresh(t *testing.T) {
	m := NewManager()

	ctx := context.Background()
	m.StartAutoRefresh(ctx, 1*time.Hour)

	m.autoMu.Lock()
	running := m.autoRunning
	m.autoMu.Unlock()
	if !running {
		t.Error("StartAutoRefresh() should set autoRunning = true")
	}

	// Stop should not error.
	m.StopAutoRefresh()

	// Give goroutine time to exit.
	time.Sleep(50 * time.Millisecond)

	m.autoMu.Lock()
	running = m.autoRunning
	m.autoMu.Unlock()
	if running {
		t.Error("StopAutoRefresh() should set autoRunning = false")
	}

	// Idempotent stop — should not panic.
	m.StopAutoRefresh()
}

func TestAutoRefreshDoubleStart(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	m.StartAutoRefresh(ctx, 1*time.Hour)

	// Second start should replace the first without panic or deadlock.
	m.StartAutoRefresh(ctx, 1*time.Hour)

	// Give goroutine time to set autoRunning.
	deadline := time.Now().Add(500 * time.Millisecond)
	var running bool
	for time.Now().Before(deadline) {
		m.autoMu.Lock()
		running = m.autoRunning
		m.autoMu.Unlock()
		if running {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if !running {
		t.Error("second StartAutoRefresh() should keep autoRunning = true")
	}

	m.StopAutoRefresh()

	// Give goroutines time to exit.
	time.Sleep(50 * time.Millisecond)
}

func TestAutoRefreshCallsRefreshAll(t *testing.T) {
	var hitCount atomic.Int64
	servers := []config.ServerEndpoint{
		{Addr: "server1.example.com:443", Name: "Server 1", Password: "pass1"},
	}
	data, _ := json.Marshal(servers)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		_, _ = w.Write(data)
	}))
	defer ts.Close()

	m := NewManager()
	m.SetAllowPrivateNetworks(true)
	_, _ = m.Add("Sub1", ts.URL)
	_, _ = m.Add("Sub2", ts.URL)

	ctx := context.Background()
	m.StartAutoRefresh(ctx, 50*time.Millisecond)

	// Wait long enough for at least one tick (interval waits before first tick).
	time.Sleep(200 * time.Millisecond)

	m.StopAutoRefresh()

	hits := hitCount.Load()
	// We have 2 subscriptions; at least one tick should have fired (2+ hits).
	if hits < 2 {
		t.Errorf("expected at least 2 HTTP hits (2 subs x 1+ tick), got %d", hits)
	}

	// Verify subscriptions were actually updated with servers.
	for _, sub := range m.List() {
		if len(sub.Servers) == 0 {
			t.Errorf("subscription %q was not refreshed", sub.Name)
		}
	}
}

func TestParseSubscription_ClashAutoDetect(t *testing.T) {
	data := "proxies:\n  - name: test\n    type: ss\n    server: 1.2.3.4\n    port: 443\n    password: pass\n    cipher: aes-256-gcm\n"
	servers, err := ParseSubscription(data)
	if err != nil {
		t.Fatalf("ParseSubscription() Clash error = %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("ParseSubscription() Clash len = %d, want 1", len(servers))
	}
	if servers[0].Name != "test" {
		t.Errorf("ParseSubscription() Clash name = %q, want %q", servers[0].Name, "test")
	}
	if servers[0].Addr != "1.2.3.4:443" {
		t.Errorf("ParseSubscription() Clash addr = %q, want %q", servers[0].Addr, "1.2.3.4:443")
	}
}

func TestParseSubscription_SingboxAutoDetect(t *testing.T) {
	data := `{"outbounds":[{"type":"shadowsocks","tag":"test","server":"1.2.3.4","server_port":443,"password":"pass","method":"aes-256-gcm"}]}`
	servers, err := ParseSubscription(data)
	if err != nil {
		t.Fatalf("ParseSubscription() sing-box error = %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("ParseSubscription() sing-box len = %d, want 1", len(servers))
	}
	if servers[0].Name != "test" {
		t.Errorf("ParseSubscription() sing-box name = %q, want %q", servers[0].Name, "test")
	}
	if servers[0].Addr != "1.2.3.4:443" {
		t.Errorf("ParseSubscription() sing-box addr = %q, want %q", servers[0].Addr, "1.2.3.4:443")
	}
}

func TestParseSubscription_Base64ClashAutoDetect(t *testing.T) {
	clash := "proxies:\n  - name: base64server\n    type: ss\n    server: 5.6.7.8\n    port: 8443\n    password: secret\n    cipher: chacha20-ietf-poly1305\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(clash))
	servers, err := ParseSubscription(encoded)
	if err != nil {
		t.Fatalf("ParseSubscription() base64 Clash error = %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("ParseSubscription() base64 Clash len = %d, want 1", len(servers))
	}
	if servers[0].Name != "base64server" {
		t.Errorf("ParseSubscription() base64 Clash name = %q, want %q", servers[0].Name, "base64server")
	}
	if servers[0].Addr != "5.6.7.8:8443" {
		t.Errorf("ParseSubscription() base64 Clash addr = %q, want %q", servers[0].Addr, "5.6.7.8:8443")
	}
}

func TestGetAllServers(t *testing.T) {
	m := NewManager()

	// Add two subscriptions with servers
	sub1, _ := m.Add("Sub1", "https://example.com/1")
	sub2, _ := m.Add("Sub2", "https://example.com/2")

	m.mu.Lock()
	m.subscriptions[sub1.ID].Servers = []config.ServerEndpoint{
		{Addr: "s1.com:443"},
		{Addr: "s2.com:443"},
	}
	m.subscriptions[sub2.ID].Servers = []config.ServerEndpoint{
		{Addr: "s3.com:443"},
	}
	m.mu.Unlock()

	all := m.GetAllServers()
	if len(all) != 3 {
		t.Errorf("GetAllServers() len = %d, want 3", len(all))
	}
}
