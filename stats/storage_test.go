package stats

import (
	"os"
	"path/filepath"
	"sync"
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

func TestGetWeeklySummary(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer s.Close()

	// Seed 14 days of data ending today
	now := time.Now()
	s.mu.Lock()
	for i := 13; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		s.stats[date] = &DailyStats{
			Date:          date,
			BytesSent:     1000,
			BytesReceived: 2000,
			Connections:   5,
		}
	}
	s.mu.Unlock()

	result := s.GetWeeklySummary(4)
	if len(result) == 0 {
		t.Fatal("GetWeeklySummary returned empty slice")
	}

	// Verify total across all weeks matches 14 days of data
	var totalSent, totalRecv, totalConns int64
	var totalDays int
	for _, p := range result {
		totalSent += p.BytesSent
		totalRecv += p.BytesRecv
		totalConns += p.Connections
		totalDays += p.Days
		if p.Period == "" {
			t.Error("Period should not be empty")
		}
	}

	if totalDays != 14 {
		t.Errorf("Total days = %d, want 14", totalDays)
	}
	if totalSent != 14000 {
		t.Errorf("Total BytesSent = %d, want 14000", totalSent)
	}
	if totalRecv != 28000 {
		t.Errorf("Total BytesRecv = %d, want 28000", totalRecv)
	}
	if totalConns != 70 {
		t.Errorf("Total Connections = %d, want 70", totalConns)
	}

	// Verify periods are sorted
	for i := 1; i < len(result); i++ {
		if result[i].Period < result[i-1].Period {
			t.Errorf("Periods not sorted: %q before %q", result[i-1].Period, result[i].Period)
		}
	}
}

func TestGetMonthlySummary(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer s.Close()

	// Seed 60 days of data ending today, spanning at least 2 months
	now := time.Now()
	s.mu.Lock()
	for i := 59; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		s.stats[date] = &DailyStats{
			Date:          date,
			BytesSent:     500,
			BytesReceived: 1000,
			Connections:   2,
		}
	}
	s.mu.Unlock()

	result := s.GetMonthlySummary(3)
	if len(result) < 2 {
		t.Fatalf("GetMonthlySummary returned %d periods, want at least 2", len(result))
	}

	// Verify total across all months matches 60 days
	var totalSent, totalRecv, totalConns int64
	var totalDays int
	for _, p := range result {
		totalSent += p.BytesSent
		totalRecv += p.BytesRecv
		totalConns += p.Connections
		totalDays += p.Days
		// Verify period format "YYYY-MM"
		if len(p.Period) != 7 || p.Period[4] != '-' {
			t.Errorf("Invalid period format: %q", p.Period)
		}
	}

	if totalDays != 60 {
		t.Errorf("Total days = %d, want 60", totalDays)
	}
	if totalSent != 30000 {
		t.Errorf("Total BytesSent = %d, want 30000", totalSent)
	}
	if totalRecv != 60000 {
		t.Errorf("Total BytesRecv = %d, want 60000", totalRecv)
	}

	// Verify periods are sorted
	for i := 1; i < len(result); i++ {
		if result[i].Period < result[i-1].Period {
			t.Errorf("Periods not sorted: %q before %q", result[i-1].Period, result[i].Period)
		}
	}
}

func TestEmptyPeriods(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer s.Close()

	weekly := s.GetWeeklySummary(4)
	if len(weekly) != 0 {
		t.Errorf("GetWeeklySummary with no data returned %d items, want 0", len(weekly))
	}

	monthly := s.GetMonthlySummary(6)
	if len(monthly) != 0 {
		t.Errorf("GetMonthlySummary with no data returned %d items, want 0", len(monthly))
	}

	// Zero/negative input
	if result := s.GetWeeklySummary(0); result != nil {
		t.Error("GetWeeklySummary(0) should return nil")
	}
	if result := s.GetMonthlySummary(-1); result != nil {
		t.Error("GetMonthlySummary(-1) should return nil")
	}
}

func TestConcurrentRecordSave(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}

	var wg sync.WaitGroup
	// Writer goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				s.Record(int64(j), int64(j), 1)
			}
		}(i)
	}
	wg.Wait()
	s.Close()

	// Verify no data corruption by loading
	s2, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage() reopen error = %v", err)
	}
	summary := s2.GetMonthlySummary(1)
	if len(summary) == 0 {
		t.Error("expected data after concurrent writes")
	}
	s2.Close()
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
