package speedtest

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryStorageRecordAndGet(t *testing.T) {
	dir := t.TempDir()
	h := NewHistoryStorage(dir)

	now := time.Now()
	entries := []HistoryEntry{
		{Timestamp: now, ServerAddr: "a:443", LatencyMs: 50, Available: true},
		{Timestamp: now, ServerAddr: "b:443", LatencyMs: 100, Available: true},
	}

	if err := h.Record(entries); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got := h.GetHistory(1)
	if len(got) != 2 {
		t.Fatalf("GetHistory(1) returned %d entries, want 2", len(got))
	}
	if got[0].ServerAddr != "a:443" {
		t.Errorf("got[0].ServerAddr = %q, want %q", got[0].ServerAddr, "a:443")
	}
	if got[1].LatencyMs != 100 {
		t.Errorf("got[1].LatencyMs = %d, want 100", got[1].LatencyMs)
	}
}

func TestHistoryStoragePersistence(t *testing.T) {
	dir := t.TempDir()

	// Write with first instance
	h1 := NewHistoryStorage(dir)
	entries := []HistoryEntry{
		{Timestamp: time.Now(), ServerAddr: "persist:443", LatencyMs: 42, Available: true},
	}
	if err := h1.Record(entries); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "speedtest_history.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("history file not found: %v", err)
	}

	// Load with second instance
	h2 := NewHistoryStorage(dir)
	got := h2.GetHistory(1)
	if len(got) != 1 {
		t.Fatalf("GetHistory(1) returned %d entries after reload, want 1", len(got))
	}
	if got[0].ServerAddr != "persist:443" {
		t.Errorf("ServerAddr = %q, want %q", got[0].ServerAddr, "persist:443")
	}
	if got[0].LatencyMs != 42 {
		t.Errorf("LatencyMs = %d, want 42", got[0].LatencyMs)
	}
}

func TestHistoryStorageMaxEntries(t *testing.T) {
	dir := t.TempDir()
	h := NewHistoryStorage(dir)
	h.maxEntries = 5 // small limit for testing

	now := time.Now()
	var entries []HistoryEntry
	for i := 0; i < 8; i++ {
		entries = append(entries, HistoryEntry{
			Timestamp:  now,
			ServerAddr: "s:443",
			LatencyMs:  int64(i),
			Available:  true,
		})
	}
	if err := h.Record(entries); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got := h.GetHistory(1)
	if len(got) != 5 {
		t.Fatalf("GetHistory returned %d entries, want 5 (maxEntries)", len(got))
	}
	// Should keep the last 5 (indices 3..7)
	if got[0].LatencyMs != 3 {
		t.Errorf("oldest entry LatencyMs = %d, want 3", got[0].LatencyMs)
	}
	if got[4].LatencyMs != 7 {
		t.Errorf("newest entry LatencyMs = %d, want 7", got[4].LatencyMs)
	}
}

func TestHistoryStorageEmptyDays(t *testing.T) {
	dir := t.TempDir()
	h := NewHistoryStorage(dir)

	entries := []HistoryEntry{
		{Timestamp: time.Now(), ServerAddr: "x:443", LatencyMs: 10, Available: true},
	}
	if err := h.Record(entries); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Zero days
	got := h.GetHistory(0)
	if len(got) != 0 {
		t.Errorf("GetHistory(0) returned %d entries, want 0", len(got))
	}

	// Negative days
	got = h.GetHistory(-5)
	if len(got) != 0 {
		t.Errorf("GetHistory(-5) returned %d entries, want 0", len(got))
	}
}

func TestHistoryStorageGetHistoryFilter(t *testing.T) {
	dir := t.TempDir()
	h := NewHistoryStorage(dir)

	now := time.Now()
	entries := []HistoryEntry{
		{Timestamp: now.AddDate(0, 0, -60), ServerAddr: "old:443", LatencyMs: 200, Available: true},
		{Timestamp: now.AddDate(0, 0, -10), ServerAddr: "mid:443", LatencyMs: 100, Available: true},
		{Timestamp: now, ServerAddr: "new:443", LatencyMs: 50, Available: true},
	}
	if err := h.Record(entries); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// 7 days should only include "new"
	got := h.GetHistory(7)
	if len(got) != 1 {
		t.Fatalf("GetHistory(7) returned %d entries, want 1", len(got))
	}
	if got[0].ServerAddr != "new:443" {
		t.Errorf("got[0].ServerAddr = %q, want %q", got[0].ServerAddr, "new:443")
	}

	// 30 days should include "mid" and "new"
	got = h.GetHistory(30)
	if len(got) != 2 {
		t.Fatalf("GetHistory(30) returned %d entries, want 2", len(got))
	}

	// 90 days should include all 3
	got = h.GetHistory(90)
	if len(got) != 3 {
		t.Fatalf("GetHistory(90) returned %d entries, want 3", len(got))
	}
}
