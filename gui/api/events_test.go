package api

import (
	"context"
	"testing"
	"time"
)

func TestEventQueue_PushTail(t *testing.T) {
	q := NewEventQueue(8)
	q.Push("server.connected", map[string]any{"id": "a"})
	q.Push("server.connected", map[string]any{"id": "b"})

	events, latest, gap := q.Tail(0, 100)
	if gap {
		t.Fatal("gap should be false on initial fetch")
	}
	if latest != 2 {
		t.Fatalf("latest = %d, want 2", latest)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
}

func TestEventQueue_TailSince(t *testing.T) {
	q := NewEventQueue(8)
	q.Push("a", nil)
	q.Push("b", nil)
	q.Push("c", nil)

	events, latest, gap := q.Tail(1, 100)
	if gap {
		t.Fatal("gap should be false")
	}
	if latest != 3 {
		t.Fatalf("latest = %d, want 3", latest)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].Type != "b" || events[1].Type != "c" {
		t.Fatalf("events = %+v, want b,c", events)
	}
}

func TestEventQueue_GapWhenEventsEvicted(t *testing.T) {
	// Capacity 2: oldest two retained, anything older evicted.
	q := NewEventQueue(2)
	q.Push("a", nil) // cursor 1
	q.Push("b", nil) // cursor 2
	q.Push("c", nil) // cursor 3 — a evicted
	q.Push("d", nil) // cursor 4 — b evicted

	// Caller last saw cursor 1 (a). The next event they need is cursor 2 (b),
	// but b has been evicted; oldest retained is c (cursor 3). Real gap.
	_, _, gap := q.Tail(1, 100)
	if !gap {
		t.Fatal("expected gap=true when events between since+1 and oldest are evicted")
	}
}

func TestEventQueue_NoGapWhenOnlyCursorEvicted(t *testing.T) {
	// Capacity 2: cursor 1 (a) evicted but cursor 2 (b) — the caller's next
	// expected event — is still in the window. No actual data loss.
	q := NewEventQueue(2)
	q.Push("a", nil) // cursor 1
	q.Push("b", nil) // cursor 2
	q.Push("c", nil) // cursor 3 — a evicted

	events, _, gap := q.Tail(1, 100)
	if gap {
		t.Fatalf("expected gap=false when caller's next expected event is retained, got gap=true")
	}
	if len(events) != 2 || events[0].Type != "b" || events[1].Type != "c" {
		t.Fatalf("events = %+v, want [b, c]", events)
	}
}

func TestEventQueue_WaitBlocks(t *testing.T) {
	q := NewEventQueue(8)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		q.Push("late", nil)
	}()

	events, _, _, err := q.Wait(ctx, 0)
	if err != nil {
		t.Fatalf("Wait err: %v", err)
	}
	if len(events) == 0 || events[0].Type != "late" {
		t.Fatalf("events = %+v, want late", events)
	}
}

func TestEventQueue_WaitReturnsImmediately_WhenAvailable(t *testing.T) {
	q := NewEventQueue(8)
	q.Push("ready", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	events, _, _, err := q.Wait(ctx, 0)
	if err != nil {
		t.Fatalf("Wait err: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected immediate return")
	}
}
