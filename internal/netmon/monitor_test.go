package netmon

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestMonitorStartStop(t *testing.T) {
	m := New(100 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)
	// Should not panic or block.
	time.Sleep(50 * time.Millisecond)
	m.Stop()
}

func TestMonitorOnChange(t *testing.T) {
	m := New(time.Second)

	var called int32
	m.OnChange(func() {
		atomic.AddInt32(&called, 1)
	})

	m.mu.Lock()
	if len(m.callbacks) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(m.callbacks))
	}
	m.mu.Unlock()
}

func TestMonitorDoubleStart(t *testing.T) {
	m := New(100 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)
	// Second start should cancel the first and not panic.
	m.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	m.Stop()
}

func TestGetAddressSnapshot(t *testing.T) {
	snap := getAddressSnapshot()
	if snap == "" {
		t.Skip("no network interfaces available")
	}
	// Should contain at least one semicolon (separator).
	found := false
	for _, c := range snap {
		if c == ';' {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("snapshot missing separator: %q", snap)
	}
}

func TestMonitorDetectsChange(t *testing.T) {
	m := New(50 * time.Millisecond)

	var notified atomic.Int32
	m.OnChange(func() {
		notified.Add(1)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)

	// Simulate a network change by modifying lastAddrs to something different.
	m.mu.Lock()
	m.lastAddrs = "fake-old-snapshot"
	m.mu.Unlock()

	// Wait for the poll loop to detect the change.
	deadline := time.After(2 * time.Second)
	for {
		if notified.Load() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for change callback")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	m.Stop()
}
