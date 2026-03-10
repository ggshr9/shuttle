package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry records a completed stream for audit purposes.
type Entry struct {
	Timestamp  time.Time `json:"timestamp"`
	User       string    `json:"user,omitempty"`
	Target     string    `json:"target"`
	BytesIn    int64     `json:"bytes_in"`
	BytesOut   int64     `json:"bytes_out"`
	DurationMs int64     `json:"duration_ms"`
	Transport  string    `json:"transport,omitempty"`
}

// Logger is a thread-safe audit logger with ring buffer and optional file sink.
type Logger struct {
	mu         sync.Mutex
	entries    []Entry
	maxEntries int
	head       int
	count      int
	writer     *os.File
	logDir     string
	currentDay string
}

// NewLogger creates an audit logger.
// If logDir is empty, only in-memory ring buffer is used.
func NewLogger(logDir string, maxEntries int) (*Logger, error) {
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	l := &Logger{
		entries:    make([]Entry, maxEntries),
		maxEntries: maxEntries,
		logDir:     logDir,
	}
	if logDir != "" {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("create audit log dir: %w", err)
		}
		if err := l.openLogFile(); err != nil {
			return nil, err
		}
	}
	return l, nil
}

func (l *Logger) openLogFile() error {
	day := time.Now().Format("2006-01-02")
	l.currentDay = day
	path := filepath.Join(l.logDir, "audit-"+day+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	l.writer = f
	return nil
}

// Log records an audit entry.
func (l *Logger) Log(e *Entry) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	// Ring buffer
	l.entries[l.head] = *e
	l.head = (l.head + 1) % l.maxEntries
	if l.count < l.maxEntries {
		l.count++
	}

	// File write with date rotation
	if l.writer != nil {
		day := e.Timestamp.Format("2006-01-02")
		if day != l.currentDay {
			l.writer.Close()
			_ = l.openLogFile()
		}
		data, _ := json.Marshal(e)
		_, _ = l.writer.Write(data)
		_, _ = l.writer.WriteString("\n")
	}
}

// Recent returns the last n entries (most recent first).
func (l *Logger) Recent(n int) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()

	if n > l.count {
		n = l.count
	}
	if n <= 0 {
		return []Entry{}
	}

	result := make([]Entry, n)
	for i := 0; i < n; i++ {
		idx := (l.head - 1 - i + l.maxEntries) % l.maxEntries
		result[i] = l.entries[idx]
	}
	return result
}

// Close closes the file sink.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.writer != nil {
		err := l.writer.Close()
		l.writer = nil
		return err
	}
	return nil
}
