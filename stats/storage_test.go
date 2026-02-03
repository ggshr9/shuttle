package stats

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorageRecord(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer s.Close()

	// Record some data
	// First record: delta from 0, so all bytes counted
	s.Record(1000, 2000, 5)
	// Second record: delta is 500 sent, 1000 received
	s.Record(1500, 3000, 3)

	// Total = first delta (1000, 2000) + second delta (500, 1000)
	total := s.GetTotal()
	if total.BytesSent != 1500 {
		t.Errorf("BytesSent = %d, want 1500", total.BytesSent)
	}
	if total.BytesReceived != 3000 {
		t.Errorf("BytesReceived = %d, want 3000", total.BytesReceived)
	}
	if total.Connections != 8 {
		t.Errorf("Connections = %d, want 8", total.Connections)
	}
}

func TestStorageGetHistory(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer s.Close()

	// Record data for today
	s.Record(1000, 2000, 1)

	history := s.GetHistory(7)
	if len(history) != 7 {
		t.Errorf("GetHistory(7) len = %d, want 7", len(history))
	}

	// Last entry should be today with data
	today := time.Now().Format("2006-01-02")
	lastEntry := history[len(history)-1]
	if lastEntry.Date != today {
		t.Errorf("Last entry date = %q, want %q", lastEntry.Date, today)
	}
}

func TestStorageClean(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer s.Close()

	// Add old data manually
	s.mu.Lock()
	oldDate := time.Now().AddDate(0, 0, -100).Format("2006-01-02")
	s.stats[oldDate] = &DailyStats{
		Date:          oldDate,
		BytesSent:     1000,
		BytesReceived: 2000,
	}
	s.mu.Unlock()

	// Verify old data exists
	if _, ok := s.stats[oldDate]; !ok {
		t.Fatal("Old data not added")
	}

	// Clean keeping only 30 days
	s.Clean(30)

	// Verify old data removed
	s.mu.RLock()
	_, exists := s.stats[oldDate]
	s.mu.RUnlock()

	if exists {
		t.Error("Clean() should remove old data")
	}
}

func TestStoragePersistence(t *testing.T) {
	dir := t.TempDir()

	// Create and record
	s1, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	s1.Record(1000, 2000, 5)
	s1.Close()

	// Verify file exists
	filePath := filepath.Join(dir, "traffic_stats.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("Stats file not created")
	}

	// Reopen and verify data persisted
	s2, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage() second time error = %v", err)
	}
	defer s2.Close()

	total := s2.GetTotal()
	// Note: first record uses delta from 0, so all bytes are counted
	if total.BytesSent != 1000 {
		t.Errorf("Persisted BytesSent = %d, want 1000", total.BytesSent)
	}
}

func TestStorageHandleReset(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer s.Close()

	// First record
	s.Record(1000, 2000, 1)

	// Simulate counter reset (smaller value)
	s.Record(500, 1000, 1)

	total := s.GetTotal()
	// Should only count positive deltas
	if total.BytesSent != 1000 {
		t.Errorf("After reset BytesSent = %d, want 1000 (no negative delta)", total.BytesSent)
	}
}

func TestDailyStatsStruct(t *testing.T) {
	ds := DailyStats{
		Date:          "2024-01-15",
		BytesSent:     1024,
		BytesReceived: 2048,
		Connections:   10,
	}

	if ds.Date != "2024-01-15" {
		t.Errorf("Date = %q, want %q", ds.Date, "2024-01-15")
	}
}
