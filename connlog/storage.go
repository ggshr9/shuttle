package connlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Entry represents a single connection event.
type Entry struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Target      string    `json:"target"`
	Rule        string    `json:"rule"`
	Protocol    string    `json:"protocol"`
	ProcessName string    `json:"process_name,omitempty"`
	BytesIn     int64     `json:"bytes_in"`
	BytesOut    int64     `json:"bytes_out"`
	DurationMs  int64     `json:"duration_ms"`
	State       string    `json:"state"` // "opened" or "closed"
}

// Storage persists connection log entries to JSONL files and keeps
// a ring buffer of recent entries in memory for fast access.
type Storage struct {
	logDir     string
	mu         sync.Mutex
	maxEntries int
	entries    []Entry
	head       int // write position in ring buffer
	count      int // number of valid entries (<= maxEntries)
	writer     *os.File
	writerDate string // YYYY-MM-DD of current writer
}

// NewStorage creates a new connection log storage. It creates logDir if it
// does not exist and opens today's log file for appending.
func NewStorage(logDir string, maxEntries int) (*Storage, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("connlog: create dir: %w", err)
	}
	if maxEntries <= 0 {
		maxEntries = 256
	}

	s := &Storage{
		logDir:     logDir,
		maxEntries: maxEntries,
		entries:    make([]Entry, maxEntries),
	}

	if err := s.openWriter(time.Now()); err != nil {
		return nil, err
	}
	return s, nil
}

// Log appends an entry to the ring buffer and writes it as a JSON line to
// the current log file. If the date has changed since the file was opened,
// it rotates to a new file automatically.
func (s *Storage) Log(entry *Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store in ring buffer.
	s.entries[s.head] = *entry
	s.head = (s.head + 1) % s.maxEntries
	if s.count < s.maxEntries {
		s.count++
	}

	// Rotate log file if the date changed.
	today := time.Now().Format("2006-01-02")
	if today != s.writerDate {
		_ = s.openWriter(time.Now())
	}

	// Write JSON line.
	if s.writer != nil {
		data, err := json.Marshal(entry)
		if err == nil {
			_, _ = s.writer.Write(data)
			_, _ = s.writer.WriteString("\n")
		}
	}
}

// Recent returns the last n entries from the ring buffer, most recent first.
// If fewer than n entries exist, all entries are returned.
func (s *Storage) Recent(n int) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()

	if n <= 0 || s.count == 0 {
		return []Entry{}
	}
	if n > s.count {
		n = s.count
	}

	result := make([]Entry, n)
	for i := 0; i < n; i++ {
		// Walk backwards from head.
		idx := (s.head - 1 - i + s.maxEntries) % s.maxEntries
		result[i] = s.entries[idx]
	}
	return result
}

// Close closes the underlying log file. It is safe to call multiple times.
func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writer != nil {
		err := s.writer.Close()
		s.writer = nil
		return err
	}
	return nil
}

// CleanOldFiles removes JSONL files older than keepDays from the log directory.
func (s *Storage) CleanOldFiles(keepDays int) error {
	cutoff := time.Now().AddDate(0, 0, -keepDays)

	entries, err := os.ReadDir(s.logDir)
	if err != nil {
		return fmt.Errorf("connlog: read dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Match connections-YYYY-MM-DD.jsonl
		if !strings.HasPrefix(name, "connections-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		dateStr := strings.TrimPrefix(name, "connections-")
		dateStr = strings.TrimSuffix(dateStr, ".jsonl")

		fileDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue // skip files with unparseable dates
		}

		if fileDate.Before(cutoff) {
			if err := os.Remove(filepath.Join(s.logDir, name)); err != nil {
				return fmt.Errorf("connlog: remove %s: %w", name, err)
			}
		}
	}
	return nil
}

// openWriter opens (or rotates to) the log file for the given time.
// Caller must hold s.mu.
func (s *Storage) openWriter(t time.Time) error {
	if s.writer != nil {
		s.writer.Close()
		s.writer = nil
	}

	date := t.Format("2006-01-02")
	name := filepath.Join(s.logDir, "connections-"+date+".jsonl")

	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("connlog: open %s: %w", name, err)
	}
	s.writer = f
	s.writerDate = date
	return nil
}
