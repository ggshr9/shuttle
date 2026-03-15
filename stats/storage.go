package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// DailyStats represents traffic statistics for a single day.
type DailyStats struct {
	Date          string `json:"date"`           // YYYY-MM-DD
	BytesSent     int64  `json:"bytes_sent"`
	BytesReceived int64  `json:"bytes_received"`
	Connections   int64  `json:"connections"`
}

// Storage manages persistent traffic statistics.
type Storage struct {
	mu       sync.RWMutex
	stats    map[string]*DailyStats // date -> stats
	filePath string
	dirty    bool

	// Current session counters (reset on start)
	sessionStart  time.Time
	lastSent      int64
	lastReceived  int64
}

// NewStorage creates a new stats storage.
func NewStorage(dataDir string) (*Storage, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	s := &Storage{
		stats:        make(map[string]*DailyStats),
		filePath:     filepath.Join(dataDir, "traffic_stats.json"),
		sessionStart: time.Now(),
	}

	s.load()
	go s.autoSave()

	return s, nil
}

// Record adds traffic data to today's stats.
func (s *Storage) Record(bytesSent, bytesReceived, connections int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if s.stats[today] == nil {
		s.stats[today] = &DailyStats{Date: today}
	}

	// Calculate delta from last record
	deltaSent := bytesSent - s.lastSent
	deltaReceived := bytesReceived - s.lastReceived

	// Only add positive deltas (handle resets)
	if deltaSent > 0 {
		s.stats[today].BytesSent += deltaSent
	}
	if deltaReceived > 0 {
		s.stats[today].BytesReceived += deltaReceived
	}
	s.stats[today].Connections += connections

	s.lastSent = bytesSent
	s.lastReceived = bytesReceived
	s.dirty = true
}

// GetHistory returns stats for the last N days.
func (s *Storage) GetHistory(days int) []DailyStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Generate list of dates
	result := make([]DailyStats, 0, days)
	today := time.Now()

	for i := days - 1; i >= 0; i-- {
		date := today.AddDate(0, 0, -i).Format("2006-01-02")
		if stat, ok := s.stats[date]; ok {
			result = append(result, *stat)
		} else {
			result = append(result, DailyStats{Date: date})
		}
	}

	return result
}

// GetTotal returns total stats across all time.
func (s *Storage) GetTotal() DailyStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var total DailyStats
	for _, stat := range s.stats {
		total.BytesSent += stat.BytesSent
		total.BytesReceived += stat.BytesReceived
		total.Connections += stat.Connections
	}
	return total
}

// Clean removes stats older than the specified number of days.
func (s *Storage) Clean(keepDays int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -keepDays).Format("2006-01-02")

	for date := range s.stats {
		if date < cutoff {
			delete(s.stats, date)
			s.dirty = true
		}
	}
}

func (s *Storage) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return // File doesn't exist yet
	}

	var stats []DailyStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return
	}

	for _, stat := range stats {
		s.stats[stat.Date] = &DailyStats{
			Date:          stat.Date,
			BytesSent:     stat.BytesSent,
			BytesReceived: stat.BytesReceived,
			Connections:   stat.Connections,
		}
	}
}

func (s *Storage) save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.dirty {
		return nil
	}

	// Convert map to sorted slice
	stats := make([]DailyStats, 0, len(s.stats))
	for _, stat := range s.stats {
		stats = append(stats, *stat)
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Date < stats[j].Date
	})

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return err
	}

	return nil
}

func (s *Storage) autoSave() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.save()
	}
}

// PeriodStats represents aggregated stats for a time period.
type PeriodStats struct {
	Period      string `json:"period"`       // "2026-W11" or "2026-03"
	BytesSent   int64  `json:"bytes_sent"`
	BytesRecv   int64  `json:"bytes_recv"`
	Connections int64  `json:"connections"`
	Days        int    `json:"days"` // number of days with data in this period
}

// GetWeeklySummary returns stats aggregated by ISO week for the last N weeks.
func (s *Storage) GetWeeklySummary(weeks int) []PeriodStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if weeks <= 0 {
		return nil
	}

	// Build a map of ISO-week -> aggregated stats
	buckets := make(map[string]*PeriodStats)
	var orderedKeys []string

	now := time.Now()
	// Walk back from today far enough to cover N weeks
	cutoff := now.AddDate(0, 0, -weeks*7)

	for dateStr, stat := range s.stats {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil || t.Before(cutoff) {
			continue
		}
		year, week := t.ISOWeek()
		key := fmt.Sprintf("%04d-W%02d", year, week)
		if _, ok := buckets[key]; !ok {
			buckets[key] = &PeriodStats{Period: key}
			orderedKeys = append(orderedKeys, key)
		}
		buckets[key].BytesSent += stat.BytesSent
		buckets[key].BytesRecv += stat.BytesReceived
		buckets[key].Connections += stat.Connections
		buckets[key].Days++
	}

	sort.Strings(orderedKeys)
	result := make([]PeriodStats, 0, len(orderedKeys))
	for _, k := range orderedKeys {
		result = append(result, *buckets[k])
	}
	return result
}

// GetMonthlySummary returns stats aggregated by month for the last N months.
func (s *Storage) GetMonthlySummary(months int) []PeriodStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if months <= 0 {
		return nil
	}

	// Cutoff: first day of the month N months ago
	now := time.Now()
	cutoff := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -(months - 1), 0)

	buckets := make(map[string]*PeriodStats)
	var orderedKeys []string

	for dateStr, stat := range s.stats {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil || t.Before(cutoff) {
			continue
		}
		key := t.Format("2006-01")
		if _, ok := buckets[key]; !ok {
			buckets[key] = &PeriodStats{Period: key}
			orderedKeys = append(orderedKeys, key)
		}
		buckets[key].BytesSent += stat.BytesSent
		buckets[key].BytesRecv += stat.BytesReceived
		buckets[key].Connections += stat.Connections
		buckets[key].Days++
	}

	sort.Strings(orderedKeys)
	result := make([]PeriodStats, 0, len(orderedKeys))
	for _, k := range orderedKeys {
		result = append(result, *buckets[k])
	}
	return result
}

// Close saves and closes the storage.
func (s *Storage) Close() error {
	return s.save()
}
