// gui/api/events.go
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// Event is one entry in the EventQueue.
type Event struct {
	Cursor int64           `json:"cursor"`
	Type   string          `json:"type"`
	Data   json.RawMessage `json:"data"`
	Time   time.Time       `json:"time"`
}

// EventQueue is a bounded ring buffer of engine events with monotonic cursors.
// Tail() returns events strictly after the supplied cursor. If the cursor
// predates the oldest retained event, gap=true is returned and the caller
// should refresh full state.
type EventQueue struct {
	mu     sync.RWMutex
	cap    int
	ring   []Event
	head   int   // next write index
	full   bool
	cursor int64 // monotonic, +1 per Push
	cond   *sync.Cond
}

func NewEventQueue(capacity int) *EventQueue {
	if capacity <= 0 {
		capacity = 1024
	}
	q := &EventQueue{cap: capacity, ring: make([]Event, capacity)}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *EventQueue) Push(typ string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		slog.Warn("event marshal failed", "type", typ, "err", err)
		raw = []byte("null")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.cursor++
	q.ring[q.head] = Event{
		Cursor: q.cursor,
		Type:   typ,
		Data:   raw,
		Time:   time.Now().UTC(),
	}
	q.head = (q.head + 1) % q.cap
	if q.head == 0 {
		q.full = true
	}
	q.cond.Broadcast()
}

// Tail returns events with cursor > since, up to max. gap=true means since
// predates the retained window.
func (q *EventQueue) Tail(since int64, max int) (events []Event, latest int64, gap bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.tailLocked(since, max)
}

func (q *EventQueue) tailLocked(since int64, max int) ([]Event, int64, bool) {
	if max <= 0 {
		max = 100
	}
	latest := q.cursor
	if since >= latest {
		return nil, latest, false
	}

	count := q.size()
	if count == 0 {
		return nil, latest, false
	}

	oldestCursor := latest - int64(count) + 1
	// gap: events the caller has not yet seen (anything from since+1 onward) have
	// been evicted. If since+1 is still in the window, the caller has continuous
	// data even though their reference cursor itself was evicted.
	gap := since > 0 && since+1 < oldestCursor

	startCursor := since + 1
	if startCursor < oldestCursor {
		startCursor = oldestCursor
	}
	startOffset := startCursor - oldestCursor // index from oldest

	// Cap the allocation by `max` — caller-supplied bound. Without this, a
	// long-quiet caller (since == 0) on a full ring would allocate the whole
	// window even though we'll only ever append `max` entries.
	wantBytes := latest - startCursor + 1
	if wantBytes > int64(max) {
		wantBytes = int64(max)
	}
	out := make([]Event, 0, wantBytes)
	for i := startOffset; i < int64(count) && len(out) < max; i++ {
		idx := (q.head - count + int(i) + q.cap) % q.cap
		out = append(out, q.ring[idx])
	}
	return out, latest, gap
}

func (q *EventQueue) size() int {
	if q.full {
		return q.cap
	}
	return q.head
}

// Wait blocks until events strictly after `since` are available or ctx is done.
func (q *EventQueue) Wait(ctx context.Context, since int64) ([]Event, int64, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.cursor > since {
		events, latest, gap := q.tailLocked(since, 100)
		return events, latest, gap, nil
	}

	// Register a one-shot wakeup on ctx cancellation. The callback runs in
	// its own goroutine — it must reacquire the mutex to safely Broadcast.
	stop := context.AfterFunc(ctx, func() {
		q.mu.Lock()
		q.cond.Broadcast()
		q.mu.Unlock()
	})
	defer stop()

	for q.cursor <= since {
		q.cond.Wait()
		if ctx.Err() != nil {
			return nil, q.cursor, false, ctx.Err()
		}
	}
	events, latest, gap := q.tailLocked(since, 100)
	return events, latest, gap, nil
}
