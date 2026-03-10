package connlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewStorage(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStorage(filepath.Join(dir, "logs"), 100)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	defer s.Close()

	// Directory should have been created.
	if _, err := os.Stat(filepath.Join(dir, "logs")); err != nil {
		t.Fatalf("log dir not created: %v", err)
	}
}

func TestLogAndRecent(t *testing.T) {
	s, err := NewStorage(t.TempDir(), 100)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	defer s.Close()

	entries := []Entry{
		{ID: "1", Timestamp: time.Now(), Target: "a.com:443", Rule: "proxy", State: "opened"},
		{ID: "2", Timestamp: time.Now(), Target: "b.com:80", Rule: "direct", State: "opened"},
		{ID: "3", Timestamp: time.Now(), Target: "c.com:443", Rule: "proxy", State: "closed"},
	}
	for _, e := range entries {
		s.Log(e)
	}

	recent := s.Recent(3)
	if len(recent) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(recent))
	}
	// Most recent first.
	if recent[0].ID != "3" {
		t.Errorf("expected most recent entry ID=3, got %s", recent[0].ID)
	}
	if recent[1].ID != "2" {
		t.Errorf("expected second entry ID=2, got %s", recent[1].ID)
	}
	if recent[2].ID != "1" {
		t.Errorf("expected third entry ID=1, got %s", recent[2].ID)
	}

	// Request fewer than available.
	recent2 := s.Recent(1)
	if len(recent2) != 1 || recent2[0].ID != "3" {
		t.Errorf("Recent(1) should return most recent entry")
	}

	// Request more than available.
	recent3 := s.Recent(50)
	if len(recent3) != 3 {
		t.Errorf("Recent(50) should return all 3 entries, got %d", len(recent3))
	}
}

func TestRingBuffer(t *testing.T) {
	maxEntries := 5
	s, err := NewStorage(t.TempDir(), maxEntries)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	defer s.Close()

	// Log more entries than maxEntries.
	for i := 0; i < 12; i++ {
		s.Log(Entry{
			ID:        string(rune('a' + i)),
			Timestamp: time.Now(),
			Target:    "host.com:443",
			State:     "opened",
		})
	}

	recent := s.Recent(maxEntries)
	if len(recent) != maxEntries {
		t.Fatalf("expected %d entries, got %d", maxEntries, len(recent))
	}

	// The last 5 entries logged had IDs corresponding to i=7..11 (rune 'h'..'l').
	// Most recent should be i=11 (rune 'l').
	if recent[0].ID != string(rune('a'+11)) {
		t.Errorf("expected most recent ID=%s, got %s", string(rune('a'+11)), recent[0].ID)
	}
}

func TestLogWritesFile(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStorage(dir, 100)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	e := Entry{
		ID:          "test-1",
		Timestamp:   time.Now(),
		Target:      "example.com:443",
		Rule:        "proxy",
		Protocol:    "h3",
		ProcessName: "curl",
		BytesIn:     1024,
		BytesOut:    512,
		DurationMs:  150,
		State:       "closed",
	}
	s.Log(e)
	s.Close()

	// Find the log file.
	today := time.Now().Format("2006-01-02")
	logFile := filepath.Join(dir, "connections-"+today+".jsonl")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var decoded Entry
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ID != "test-1" {
		t.Errorf("expected ID=test-1, got %s", decoded.ID)
	}
	if decoded.Target != "example.com:443" {
		t.Errorf("expected Target=example.com:443, got %s", decoded.Target)
	}
	if decoded.BytesIn != 1024 {
		t.Errorf("expected BytesIn=1024, got %d", decoded.BytesIn)
	}
}

func TestRecentEmpty(t *testing.T) {
	s, err := NewStorage(t.TempDir(), 100)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	defer s.Close()

	recent := s.Recent(10)
	if len(recent) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(recent))
	}

	recent0 := s.Recent(0)
	if len(recent0) != 0 {
		t.Errorf("Recent(0) should return empty slice, got %d", len(recent0))
	}
}

func TestCleanOldFiles(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStorage(dir, 100)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	defer s.Close()

	// Create fake old log files.
	oldDates := []string{
		time.Now().AddDate(0, 0, -10).Format("2006-01-02"),
		time.Now().AddDate(0, 0, -20).Format("2006-01-02"),
		time.Now().AddDate(0, 0, -30).Format("2006-01-02"),
	}
	recentDate := time.Now().AddDate(0, 0, -2).Format("2006-01-02")

	for _, d := range oldDates {
		name := filepath.Join(dir, "connections-"+d+".jsonl")
		if err := os.WriteFile(name, []byte(`{"id":"old"}`+"\n"), 0o644); err != nil {
			t.Fatalf("create old file: %v", err)
		}
	}
	recentFile := filepath.Join(dir, "connections-"+recentDate+".jsonl")
	if err := os.WriteFile(recentFile, []byte(`{"id":"recent"}`+"\n"), 0o644); err != nil {
		t.Fatalf("create recent file: %v", err)
	}

	// Also create a non-matching file that should be left alone.
	otherFile := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(otherFile, []byte("keep"), 0o644); err != nil {
		t.Fatalf("create other file: %v", err)
	}

	// Clean files older than 7 days.
	if err := s.CleanOldFiles(7); err != nil {
		t.Fatalf("CleanOldFiles: %v", err)
	}

	// Old files should be gone.
	for _, d := range oldDates {
		name := filepath.Join(dir, "connections-"+d+".jsonl")
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed, but it still exists", d)
		}
	}

	// Recent file should still exist.
	if _, err := os.Stat(recentFile); err != nil {
		t.Errorf("recent file should still exist: %v", err)
	}

	// Today's file (created by NewStorage) should still exist.
	todayFile := filepath.Join(dir, "connections-"+time.Now().Format("2006-01-02")+".jsonl")
	if _, err := os.Stat(todayFile); err != nil {
		t.Errorf("today's file should still exist: %v", err)
	}

	// Non-matching file should still exist.
	if _, err := os.Stat(otherFile); err != nil {
		t.Errorf("non-matching file should still exist: %v", err)
	}
}

func TestClose(t *testing.T) {
	s, err := NewStorage(t.TempDir(), 100)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	// First close should succeed.
	if err := s.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}

	// Second close should be idempotent (no error).
	if err := s.Close(); err != nil {
		t.Errorf("second Close should be nil, got: %v", err)
	}
}
