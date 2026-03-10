package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewLogger_MemoryOnly(t *testing.T) {
	l, err := NewLogger("", 10)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	l.Log(&Entry{Target: "example.com:443", BytesIn: 100, BytesOut: 200, DurationMs: 50})
	l.Log(&Entry{Target: "test.com:80", BytesIn: 300, BytesOut: 400, DurationMs: 100})

	entries := l.Recent(5)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Most recent first
	if entries[0].Target != "test.com:80" {
		t.Errorf("expected most recent entry first, got %q", entries[0].Target)
	}
	if entries[1].Target != "example.com:443" {
		t.Errorf("expected oldest entry second, got %q", entries[1].Target)
	}
}

func TestNewLogger_WithFile(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir, 10)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	l.Log(&Entry{Target: "example.com:443", BytesIn: 100, BytesOut: 200, DurationMs: 50})
	l.Log(&Entry{Target: "test.com:80", BytesIn: 300, BytesOut: 400, DurationMs: 100})
	l.Close()

	// Check that a JSONL file was written
	day := time.Now().Format("2006-01-02")
	path := filepath.Join(dir, "audit-"+day+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var e Entry
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("unmarshal line 0: %v", err)
	}
	if e.Target != "example.com:443" {
		t.Errorf("expected target example.com:443, got %q", e.Target)
	}
}

func TestRingBuffer(t *testing.T) {
	l, err := NewLogger("", 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	// Log 5 entries into a buffer of size 3
	for i := 0; i < 5; i++ {
		l.Log(&Entry{Target: string(rune('A' + i)), DurationMs: int64(i)})
	}

	entries := l.Recent(10) // ask for more than available
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Should have the last 3 entries (C=2, D=3, E=4), most recent first
	if entries[0].DurationMs != 4 {
		t.Errorf("expected duration 4, got %d", entries[0].DurationMs)
	}
	if entries[1].DurationMs != 3 {
		t.Errorf("expected duration 3, got %d", entries[1].DurationMs)
	}
	if entries[2].DurationMs != 2 {
		t.Errorf("expected duration 2, got %d", entries[2].DurationMs)
	}
}

func TestRecentEmpty(t *testing.T) {
	l, err := NewLogger("", 10)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	entries := l.Recent(5)
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
	if entries == nil {
		t.Fatal("expected non-nil empty slice")
	}
}

func TestClose(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir, 10)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	// Close twice should not panic
	if err := l.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}
