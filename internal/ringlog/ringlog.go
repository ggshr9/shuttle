// Package ringlog provides a generic ring-buffer logger with optional JSONL
// file persistence and daily rotation. It is used by connlog and server/audit.
package ringlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store is a thread-safe ring buffer of entries with optional JSONL file sink.
// T must be a value type suitable for json.Marshal.
type Store[T any] struct {
	mu         sync.Mutex
	entries    []T
	maxEntries int
	head       int
	count      int
	writer     *os.File
	logDir     string
	currentDay string
	filePrefix string // e.g. "connections" or "audit"
}

// Config holds configuration for creating a new Store.
type Config struct {
	// LogDir is the directory for JSONL files. If empty, only in-memory
	// ring buffer is used (no file writes).
	LogDir string

	// MaxEntries is the ring buffer capacity. If <= 0, defaults to 256.
	MaxEntries int

	// FilePrefix is the filename prefix before the date, e.g. "connections"
	// produces "connections-2006-01-02.jsonl". If empty and LogDir is set,
	// defaults to "log".
	FilePrefix string
}

// New creates a Store. If cfg.LogDir is non-empty, it creates the directory
// and opens today's log file.
func New[T any](cfg Config) (*Store[T], error) {
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 256
	}
	if cfg.FilePrefix == "" {
		cfg.FilePrefix = "log"
	}

	s := &Store[T]{
		entries:    make([]T, cfg.MaxEntries),
		maxEntries: cfg.MaxEntries,
		logDir:     cfg.LogDir,
		filePrefix: cfg.FilePrefix,
	}

	if cfg.LogDir != "" {
		if err := os.MkdirAll(cfg.LogDir, 0o755); err != nil {
			return nil, fmt.Errorf("ringlog: create dir: %w", err)
		}
		if err := s.openWriter(time.Now()); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Log appends an entry to the ring buffer and, if a file sink is configured,
// writes it as a JSON line. The file is rotated daily.
func (s *Store[T]) Log(entry *T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store in ring buffer.
	s.entries[s.head] = *entry
	s.head = (s.head + 1) % s.maxEntries
	if s.count < s.maxEntries {
		s.count++
	}

	// File write with date rotation.
	if s.writer != nil {
		today := time.Now().Format("2006-01-02")
		if today != s.currentDay {
			s.writer.Close()
			_ = s.openWriter(time.Now())
		}
		data, err := json.Marshal(entry)
		if err == nil {
			_, _ = s.writer.Write(data)
			_, _ = s.writer.WriteString("\n")
		}
	}
}

// Recent returns the last n entries, most recent first. If fewer than n
// entries exist, all entries are returned. Always returns a non-nil slice.
func (s *Store[T]) Recent(n int) []T {
	s.mu.Lock()
	defer s.mu.Unlock()

	if n <= 0 || s.count == 0 {
		return []T{}
	}
	if n > s.count {
		n = s.count
	}

	result := make([]T, n)
	for i := 0; i < n; i++ {
		idx := (s.head - 1 - i + s.maxEntries) % s.maxEntries
		result[i] = s.entries[idx]
	}
	return result
}

// Close closes the file sink. It is safe to call multiple times.
func (s *Store[T]) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writer != nil {
		err := s.writer.Close()
		s.writer = nil
		return err
	}
	return nil
}

// LogDir returns the configured log directory (may be empty).
func (s *Store[T]) LogDir() string {
	return s.logDir
}

// FilePrefix returns the configured file prefix.
func (s *Store[T]) FilePrefix() string {
	return s.filePrefix
}

// openWriter opens (or rotates to) the log file for the given time.
// Caller must hold s.mu.
func (s *Store[T]) openWriter(t time.Time) error {
	if s.writer != nil {
		s.writer.Close()
		s.writer = nil
	}

	day := t.Format("2006-01-02")
	s.currentDay = day
	path := filepath.Join(s.logDir, s.filePrefix+"-"+day+".jsonl")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("ringlog: open %s: %w", path, err)
	}
	s.writer = f
	return nil
}
