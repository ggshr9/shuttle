package connlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ggshr9/shuttle/internal/ringlog"
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
	store *ringlog.Store[Entry]
}

// NewStorage creates a new connection log storage. It creates logDir if it
// does not exist and opens today's log file for appending.
func NewStorage(logDir string, maxEntries int) (*Storage, error) {
	store, err := ringlog.New[Entry](ringlog.Config{
		LogDir:     logDir,
		MaxEntries: maxEntries,
		FilePrefix: "connections",
	})
	if err != nil {
		return nil, err
	}
	return &Storage{store: store}, nil
}

// Log appends an entry to the ring buffer and writes it as a JSON line to
// the current log file. If the date has changed since the file was opened,
// it rotates to a new file automatically.
func (s *Storage) Log(entry *Entry) {
	s.store.Log(entry)
}

// Recent returns the last n entries from the ring buffer, most recent first.
// If fewer than n entries exist, all entries are returned.
func (s *Storage) Recent(n int) []Entry {
	return s.store.Recent(n)
}

// Close closes the underlying log file. It is safe to call multiple times.
func (s *Storage) Close() error {
	return s.store.Close()
}

// CleanOldFiles removes JSONL files older than keepDays from the log directory.
func (s *Storage) CleanOldFiles(keepDays int) error {
	logDir := s.store.LogDir()
	cutoff := time.Now().AddDate(0, 0, -keepDays)

	entries, err := os.ReadDir(logDir)
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
			if err := os.Remove(filepath.Join(logDir, name)); err != nil {
				return fmt.Errorf("connlog: remove %s: %w", name, err)
			}
		}
	}
	return nil
}
