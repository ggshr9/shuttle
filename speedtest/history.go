package speedtest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// HistoryEntry records a single speed test result.
type HistoryEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	ServerAddr  string    `json:"server_addr"`
	ServerName  string    `json:"server_name,omitempty"`
	LatencyMs   int64     `json:"latency_ms"`
	DownloadBps int64     `json:"download_bps,omitempty"` // bits per second
	UploadBps   int64     `json:"upload_bps,omitempty"`   // bits per second
	Available   bool      `json:"available"`
}

// HistoryStorage persists speed test results to disk.
type HistoryStorage struct {
	mu         sync.Mutex
	path       string // file path for JSON storage
	entries    []HistoryEntry
	maxEntries int // default 1000
}

// NewHistoryStorage creates a new HistoryStorage that persists data in the given directory.
func NewHistoryStorage(dataDir string) *HistoryStorage {
	h := &HistoryStorage{
		path:       filepath.Join(dataDir, "speedtest_history.json"),
		maxEntries: 1000,
	}
	_ = h.load() // best-effort load from disk
	return h
}

// Record appends entries and persists to disk.
func (h *HistoryStorage) Record(entries []HistoryEntry) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.entries = append(h.entries, entries...)
	return h.save()
}

// GetHistory returns entries from the last N days. If days <= 0 it returns nil.
func (h *HistoryStorage) GetHistory(days int) []HistoryEntry {
	if days <= 0 {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -days)
	var result []HistoryEntry
	for _, e := range h.entries {
		if !e.Timestamp.Before(cutoff) {
			result = append(result, e)
		}
	}
	return result
}

// load reads history from disk.
func (h *HistoryStorage) load() error {
	data, err := os.ReadFile(h.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &h.entries)
}

// save persists history to disk, trimming to maxEntries.
func (h *HistoryStorage) save() error {
	// Trim oldest entries if over max
	if len(h.entries) > h.maxEntries {
		h.entries = h.entries[len(h.entries)-h.maxEntries:]
	}

	data, err := json.Marshal(h.entries)
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(h.path), 0700); err != nil {
		return err
	}

	return os.WriteFile(h.path, data, 0600)
}
