package geodata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStatusPersistence(t *testing.T) {
	dir := t.TempDir()

	// Create manager and simulate an update timestamp
	m := NewManager(&ManagerConfig{
		DataDir: dir,
	}, nil)

	// Initially lastUpdate is zero
	status := m.Status()
	if !status.LastUpdate.IsZero() {
		t.Fatalf("expected zero lastUpdate initially, got %v", status.LastUpdate)
	}

	// Set a timestamp and save
	now := time.Now().Truncate(time.Second) // JSON round-trips to second precision
	m.mu.Lock()
	m.lastUpdate = now
	m.mu.Unlock()
	m.saveStatus()

	// Verify status.json exists
	statusPath := filepath.Join(dir, "status.json")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("status.json not found: %v", err)
	}

	var ps persistedStatus
	if err := json.Unmarshal(data, &ps); err != nil {
		t.Fatalf("invalid status.json: %v", err)
	}
	if !ps.LastUpdate.Equal(now) {
		t.Fatalf("persisted time mismatch: got %v, want %v", ps.LastUpdate, now)
	}

	// Create a new manager from the same directory — should load persisted timestamp
	m2 := NewManager(&ManagerConfig{
		DataDir: dir,
	}, nil)

	status2 := m2.Status()
	if !status2.LastUpdate.Equal(now) {
		t.Fatalf("new manager did not load persisted timestamp: got %v, want %v", status2.LastUpdate, now)
	}
}

func TestStatusPersistenceMissingFile(t *testing.T) {
	dir := t.TempDir()

	// No status.json exists — should not error
	m := NewManager(&ManagerConfig{
		DataDir: dir,
	}, nil)

	status := m.Status()
	if !status.LastUpdate.IsZero() {
		t.Fatalf("expected zero lastUpdate when no status.json, got %v", status.LastUpdate)
	}
}

func TestStatusPersistenceCorruptFile(t *testing.T) {
	dir := t.TempDir()

	// Write corrupt status.json
	statusPath := filepath.Join(dir, "status.json")
	_ = os.WriteFile(statusPath, []byte("not json"), 0o600)

	// Should not error, just ignore corrupt data
	m := NewManager(&ManagerConfig{
		DataDir: dir,
	}, nil)

	status := m.Status()
	if !status.LastUpdate.IsZero() {
		t.Fatalf("expected zero lastUpdate with corrupt status.json, got %v", status.LastUpdate)
	}
}

func TestManagerNextUpdate(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Truncate(time.Second)

	m := NewManager(&ManagerConfig{
		DataDir:        dir,
		AutoUpdate:     true,
		UpdateInterval: "1h",
	}, nil)

	m.mu.Lock()
	m.lastUpdate = now
	m.mu.Unlock()

	status := m.Status()
	expected := now.Add(1 * time.Hour)
	if !status.NextUpdate.Equal(expected) {
		t.Fatalf("expected next_update=%v, got %v", expected, status.NextUpdate)
	}
}

func TestManagerHasAllFiles(t *testing.T) {
	dir := t.TempDir()

	m := NewManager(&ManagerConfig{
		DataDir: dir,
	}, nil)

	// No files → false
	if m.hasAllFiles() {
		t.Fatal("expected hasAllFiles=false with empty dir")
	}

	// Create the required files
	_ = os.WriteFile(filepath.Join(dir, FileDirectList), []byte("example.com"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, FileCNCidr), []byte("1.0.0.0/8"), 0o600)

	if !m.hasAllFiles() {
		t.Fatal("expected hasAllFiles=true after creating required files")
	}
}

func TestManagerBuildSources(t *testing.T) {
	m := NewManager(&ManagerConfig{
		DirectListURL:  "https://example.com/direct.txt",
		ProxyListURL:   "https://example.com/proxy.txt",
		RejectListURL:  "https://example.com/reject.txt",
		GFWListURL:     "https://example.com/gfw.txt",
		CNCidrURL:      "https://example.com/cn.txt",
		PrivateCidrURL: "https://example.com/private.txt",
	}, nil)

	sources := m.buildSources()
	if len(sources) != 6 {
		t.Fatalf("expected 6 sources, got %d", len(sources))
	}

	// Without optional URLs
	m2 := NewManager(&ManagerConfig{
		DirectListURL: "https://example.com/direct.txt",
		ProxyListURL:  "https://example.com/proxy.txt",
		RejectListURL: "https://example.com/reject.txt",
		GFWListURL:    "https://example.com/gfw.txt",
	}, nil)
	sources2 := m2.buildSources()
	if len(sources2) != 4 {
		t.Fatalf("expected 4 sources without optional URLs, got %d", len(sources2))
	}
}
