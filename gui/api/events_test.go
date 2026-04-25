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

func TestEventQueue_GapWhenSinceTooOld(t *testing.T) {
	q := NewEventQueue(2) // tiny ring
	q.Push("a", nil)
	q.Push("b", nil)
	q.Push("c", nil) // a evicted

	_, _, gap := q.Tail(1, 100) // since=1 means "after event #1", but a is gone
	if !gap {
		t.Fatal("expected gap=true when since predates ring buffer")
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
