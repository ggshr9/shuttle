// Package observe provides test observability: event recording and
// timeline dump on failure.
package observe

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// Event represents a timestamped test event.
type Event struct {
	Time   time.Time
	Kind   string // "dial", "send", "recv", "drop", "fault", "close", etc.
	From   string // source node/component
	To     string // destination
	Detail string // human-readable detail
	Size   int    // bytes, if applicable
}

// Recorder collects events during a test and dumps them on failure.
type Recorder struct {
	mu     sync.Mutex
	events []Event
	start  time.Time
}

// NewRecorder creates a Recorder and registers a t.Cleanup that dumps
// the timeline if the test fails.
func NewRecorder(t testing.TB) *Recorder {
	r := &Recorder{start: time.Now()}
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(r.Format())
		}
	})
	return r
}

// NewRecorderManual creates a Recorder without automatic cleanup.
// Call Format() or Dump() manually.
func NewRecorderManual() *Recorder {
	return &Recorder{start: time.Now()}
}

// Record adds an event to the timeline.
func (r *Recorder) Record(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
}

// RecordF is a convenience method to record an event with formatted detail.
func (r *Recorder) RecordF(kind, from, to, format string, args ...any) {
	r.Record(Event{
		Kind:   kind,
		From:   from,
		To:     to,
		Detail: fmt.Sprintf(format, args...),
	})
}

// Events returns a copy of all recorded events.
func (r *Recorder) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]Event, len(r.events))
	copy(cp, r.events)
	return cp
}

// Len returns the number of recorded events.
func (r *Recorder) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

// Filter returns events matching the given kind. Pass "" to match all kinds.
func (r *Recorder) Filter(kind string) []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Event
	for _, e := range r.events {
		if kind == "" || e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// FilterFrom returns events matching the given kind and source node.
func (r *Recorder) FilterFrom(kind, from string) []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Event
	for _, e := range r.events {
		if (kind == "" || e.Kind == kind) && (from == "" || e.From == from) {
			out = append(out, e)
		}
	}
	return out
}

// Count returns the number of events matching the given kind.
func (r *Recorder) Count(kind string) int {
	return len(r.Filter(kind))
}

// TotalBytes returns the sum of Size across events matching the given kind.
func (r *Recorder) TotalBytes(kind string) int64 {
	var total int64
	for _, e := range r.Filter(kind) {
		total += int64(e.Size)
	}
	return total
}

// AssertCount fails the test if the event count for the given kind doesn't match.
func (r *Recorder) AssertCount(t testing.TB, kind string, want int) {
	t.Helper()
	got := r.Count(kind)
	if got != want {
		t.Errorf("event count for %q: got %d, want %d", kind, got, want)
	}
}

// AssertCountRange fails the test if the event count is outside [min, max].
func (r *Recorder) AssertCountRange(t testing.TB, kind string, min, max int) {
	t.Helper()
	got := r.Count(kind)
	if got < min || got > max {
		t.Errorf("event count for %q: got %d, want [%d, %d]", kind, got, min, max)
	}
}

// AssertHas fails the test if no events match the given kind.
func (r *Recorder) AssertHas(t testing.TB, kind string) {
	t.Helper()
	if r.Count(kind) == 0 {
		t.Errorf("expected at least one %q event, got none", kind)
	}
}

// AssertNone fails the test if any events match the given kind.
func (r *Recorder) AssertNone(t testing.TB, kind string) {
	t.Helper()
	if c := r.Count(kind); c > 0 {
		t.Errorf("expected no %q events, got %d", kind, c)
	}
}

// Format produces an ASCII timeline string.
func (r *Recorder) Format() string {
	r.mu.Lock()
	events := make([]Event, len(r.events))
	copy(events, r.events)
	r.mu.Unlock()

	if len(events) == 0 {
		return "=== Network Timeline (0 events) ==="
	}

	var b strings.Builder
	fmt.Fprintf(&b, "=== Network Timeline (%d events) ===\n", len(events))

	for _, e := range events {
		offset := e.Time.Sub(r.start)
		ms := float64(offset) / float64(time.Millisecond)

		arrow := ""
		if e.From != "" && e.To != "" {
			arrow = fmt.Sprintf("%-8s → %-8s", e.From, e.To)
		} else if e.From != "" {
			arrow = fmt.Sprintf("%-8s          ", e.From)
		} else {
			arrow = fmt.Sprintf("%-19s", "")
		}

		detail := e.Detail
		if e.Size > 0 && detail == "" {
			detail = fmt.Sprintf("%d bytes", e.Size)
		} else if e.Size > 0 {
			detail = fmt.Sprintf("%s (%d bytes)", detail, e.Size)
		}

		fmt.Fprintf(&b, "  %+10.3fms  [%-6s] %s  %s\n", ms, e.Kind, arrow, detail)
	}

	return b.String()
}
