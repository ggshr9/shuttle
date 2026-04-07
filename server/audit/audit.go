package audit

import (
	"time"

	"github.com/shuttleX/shuttle/internal/ringlog"
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
	store *ringlog.Store[Entry]
}

// NewLogger creates an audit logger.
// If logDir is empty, only in-memory ring buffer is used.
func NewLogger(logDir string, maxEntries int) (*Logger, error) {
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	store, err := ringlog.New[Entry](ringlog.Config{
		LogDir:     logDir,
		MaxEntries: maxEntries,
		FilePrefix: "audit",
	})
	if err != nil {
		return nil, err
	}
	return &Logger{store: store}, nil
}

// Log records an audit entry.
func (l *Logger) Log(e *Entry) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	l.store.Log(e)
}

// Recent returns the last n entries (most recent first).
func (l *Logger) Recent(n int) []Entry {
	return l.store.Recent(n)
}

// Close closes the file sink.
func (l *Logger) Close() error {
	return l.store.Close()
}
