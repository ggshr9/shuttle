package engine

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestObservabilityManager_Subscribe(t *testing.T) {
	om := NewObservabilityManager(nil)

	ch := om.Subscribe()

	// Emit an event and verify it arrives.
	om.Emit(Event{Type: EventLog, Message: "hello"})

	select {
	case ev := <-ch:
		if ev.Type != EventLog {
			t.Fatalf("expected EventLog, got %v", ev.Type)
		}
		if ev.Message != "hello" {
			t.Fatalf("expected message 'hello', got %q", ev.Message)
		}
		if ev.Timestamp.IsZero() {
			t.Fatal("expected non-zero timestamp")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}

	// Unsubscribe and verify the channel is closed.
	om.Unsubscribe(ch)

	// Channel should be drained/closed — reading should eventually yield zero value.
	select {
	case _, ok := <-ch:
		if ok {
			// Might get a buffered event, drain again.
			for range ch {
			}
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed after Unsubscribe")
	}
}

func TestObservabilityManager_SubscribeMultiple(t *testing.T) {
	om := NewObservabilityManager(nil)

	ch1 := om.Subscribe()
	ch2 := om.Subscribe()

	om.Emit(Event{Type: EventConnected, Message: "up"})

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Type != EventConnected {
				t.Fatalf("subscriber %d: expected EventConnected, got %v", i, ev.Type)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("subscriber %d: timed out", i)
		}
	}

	om.Unsubscribe(ch1)
	om.Unsubscribe(ch2)
}

func TestObservabilityManager_EmitNonBlocking(t *testing.T) {
	om := NewObservabilityManager(nil)
	ch := om.Subscribe()

	// Fill the channel buffer.
	for i := 0; i < observabilityChannelBuffer; i++ {
		om.Emit(Event{Type: EventLog, Message: "fill"})
	}

	// This emit should not block even though the buffer is full.
	done := make(chan struct{})
	go func() {
		om.Emit(Event{Type: EventLog, Message: "overflow"})
		close(done)
	}()

	select {
	case <-done:
		// Good — non-blocking.
	case <-time.After(2 * time.Second):
		t.Fatal("Emit blocked on full channel")
	}

	om.Unsubscribe(ch)
}

func TestObservabilityManager_ConcurrentEmitUnsubscribe(t *testing.T) {
	om := NewObservabilityManager(nil)

	done := make(chan struct{})
	go func() {
		defer close(done)

		ch := om.Subscribe()
		// Drain so sends don't block.
		go func() {
			for range ch {
			}
		}()

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				om.Emit(Event{Type: EventSpeedTick})
			}
		}()

		go func() {
			defer wg.Done()
			time.Sleep(time.Millisecond)
			om.Unsubscribe(ch)
		}()

		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock detected")
	}
}

func TestObservabilityManager_EmitConnectionEvent(t *testing.T) {
	om := NewObservabilityManager(nil)
	ch := om.Subscribe()

	om.EmitConnectionEvent("abc123", "opened", "example.com:443", "direct", "tcp", "curl", 100, 200, 500)

	select {
	case ev := <-ch:
		if ev.Type != EventConnection {
			t.Fatalf("expected EventConnection, got %v", ev.Type)
		}
		if ev.ConnID != "abc123" {
			t.Fatalf("expected connID 'abc123', got %q", ev.ConnID)
		}
		if ev.ConnState != "opened" {
			t.Fatalf("expected state 'opened', got %q", ev.ConnState)
		}
		if ev.Target != "example.com:443" {
			t.Fatalf("expected target 'example.com:443', got %q", ev.Target)
		}
		if ev.BytesIn != 100 || ev.BytesOut != 200 || ev.DurationMs != 500 {
			t.Fatalf("unexpected byte/duration values: in=%d out=%d dur=%d", ev.BytesIn, ev.BytesOut, ev.DurationMs)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for connection event")
	}

	om.Unsubscribe(ch)
}

func TestObservabilityManager_PluginChain(t *testing.T) {
	om := NewObservabilityManager(nil)

	// Metrics should be available immediately.
	if om.Metrics() == nil {
		t.Fatal("expected non-nil Metrics")
	}

	// Chain should be nil before BuildChain.
	if om.Chain() != nil {
		t.Fatal("expected nil Chain before BuildChain")
	}

	// Build the chain.
	ctx := context.Background()
	if err := om.BuildChain(ctx, om); err != nil {
		t.Fatalf("BuildChain failed: %v", err)
	}

	if om.Chain() == nil {
		t.Fatal("expected non-nil Chain after BuildChain")
	}

	// CloseChain should nil out the chain.
	om.CloseChain()
	if om.Chain() != nil {
		t.Fatal("expected nil Chain after CloseChain")
	}
}

func TestObservabilityManager_SpeedLoop(t *testing.T) {
	om := NewObservabilityManager(nil)
	ch := om.Subscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	om.StartSpeedLoop(ctx)

	// We should receive an EventSpeedTick within ~2 seconds (ticker fires every 1s).
	select {
	case ev := <-ch:
		if ev.Type != EventSpeedTick {
			t.Fatalf("expected EventSpeedTick, got %v", ev.Type)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for speed tick event")
	}

	// Cancel and wait for the goroutine to exit.
	cancel()
	waitDone := make(chan struct{})
	go func() {
		om.WaitBackground()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(3 * time.Second):
		t.Fatal("WaitBackground did not return after context cancellation")
	}

	om.Unsubscribe(ch)
}
