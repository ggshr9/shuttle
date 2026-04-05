package server

import (
	"sync"
	"testing"
	"time"
)

func TestServerEventBus_SubscribeEmit(t *testing.T) {
	bus := NewServerEventBus()

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	bus.Emit(ServerEvent{Type: "started", Message: "server started"})

	select {
	case ev := <-ch:
		if ev.Type != "started" {
			t.Fatalf("expected type 'started', got %q", ev.Type)
		}
		if ev.Message != "server started" {
			t.Fatalf("expected message 'server started', got %q", ev.Message)
		}
		if ev.Timestamp.IsZero() {
			t.Fatal("expected non-zero timestamp")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestServerEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewServerEventBus()

	ch1 := bus.Subscribe()
	ch2 := bus.Subscribe()
	defer bus.Unsubscribe(ch1)
	defer bus.Unsubscribe(ch2)

	bus.Emit(ServerEvent{Type: "connected", ConnID: "abc123"})

	for i, ch := range []<-chan ServerEvent{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Type != "connected" {
				t.Fatalf("subscriber %d: expected type 'connected', got %q", i, ev.Type)
			}
			if ev.ConnID != "abc123" {
				t.Fatalf("subscriber %d: expected conn_id 'abc123', got %q", i, ev.ConnID)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out waiting for event", i)
		}
	}
}

func TestServerEventBus_Unsubscribe(t *testing.T) {
	bus := NewServerEventBus()

	ch := bus.Subscribe()
	bus.Unsubscribe(ch)

	// Emit after unsubscribe should not panic
	bus.Emit(ServerEvent{Type: "test"})

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed")
		}
	default:
		// Channel is closed and drained — also fine
	}
}

func TestServerEventBus_NonBlocking(t *testing.T) {
	bus := NewServerEventBus()

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	// Fill the channel buffer
	for i := 0; i < eventChannelBuffer; i++ {
		bus.Emit(ServerEvent{Type: "fill"})
	}

	// This should not block even though the channel is full
	done := make(chan struct{})
	go func() {
		bus.Emit(ServerEvent{Type: "overflow"})
		close(done)
	}()

	select {
	case <-done:
		// OK — Emit returned without blocking
	case <-time.After(time.Second):
		t.Fatal("Emit blocked on full channel")
	}
}

func TestServerEventBus_ConcurrentEmit(t *testing.T) {
	bus := NewServerEventBus()

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Emit(ServerEvent{Type: "concurrent"})
		}()
	}

	// Drain events to prevent blocking
	go func() {
		for range ch {
		}
	}()

	wg.Wait()
}
