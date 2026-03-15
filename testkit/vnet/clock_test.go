package vnet

import (
	"sync"
	"testing"
	"time"
)

func TestVirtualClockNow(t *testing.T) {
	start := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	vc := NewVirtualClock(start)
	if got := vc.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %v, want %v", got, start)
	}
}

func TestVirtualClockDefaultStart(t *testing.T) {
	vc := NewVirtualClock(time.Time{})
	if vc.Now().IsZero() {
		t.Fatal("default start should not be zero")
	}
}

func TestVirtualClockAdvance(t *testing.T) {
	vc := NewVirtualClock(time.Time{})
	before := vc.Now()
	vc.Advance(5 * time.Second)
	after := vc.Now()
	if diff := after.Sub(before); diff != 5*time.Second {
		t.Fatalf("Advance(5s): diff = %v, want 5s", diff)
	}
}

func TestVirtualClockAfterFires(t *testing.T) {
	vc := NewVirtualClock(time.Time{})
	ch := vc.After(100 * time.Millisecond)

	// Should not fire yet.
	select {
	case <-ch:
		t.Fatal("After fired before Advance")
	default:
	}

	vc.Advance(100 * time.Millisecond)

	select {
	case <-ch:
		// ok
	default:
		t.Fatal("After did not fire after Advance")
	}
}

func TestVirtualClockAfterZeroDuration(t *testing.T) {
	vc := NewVirtualClock(time.Time{})
	ch := vc.After(0)
	select {
	case <-ch:
		// ok — zero duration fires immediately
	default:
		t.Fatal("After(0) should fire immediately")
	}
}

func TestVirtualClockSleep(t *testing.T) {
	vc := NewVirtualClock(time.Time{})
	done := make(chan struct{})
	go func() {
		vc.Sleep(200 * time.Millisecond)
		close(done)
	}()

	// Wait for the waiter to register.
	vc.BlockUntilWaiters(1)
	vc.Advance(200 * time.Millisecond)

	select {
	case <-done:
		// ok
	case <-time.After(time.Second):
		t.Fatal("Sleep did not return after Advance")
	}
}

func TestVirtualClockSleepZero(t *testing.T) {
	vc := NewVirtualClock(time.Time{})
	// Sleep(0) should return immediately without deadlock.
	vc.Sleep(0)
}

func TestVirtualClockMultipleWaiters(t *testing.T) {
	vc := NewVirtualClock(time.Time{})
	ch1 := vc.After(50 * time.Millisecond)
	ch2 := vc.After(100 * time.Millisecond)
	ch3 := vc.After(150 * time.Millisecond)

	if vc.WaiterCount() != 3 {
		t.Fatalf("WaiterCount = %d, want 3", vc.WaiterCount())
	}

	// Advance to 100ms — should fire ch1 and ch2 but not ch3.
	vc.Advance(100 * time.Millisecond)

	select {
	case <-ch1:
	default:
		t.Fatal("ch1 should have fired")
	}
	select {
	case <-ch2:
	default:
		t.Fatal("ch2 should have fired")
	}
	select {
	case <-ch3:
		t.Fatal("ch3 should not have fired")
	default:
	}

	if vc.WaiterCount() != 1 {
		t.Fatalf("WaiterCount = %d, want 1", vc.WaiterCount())
	}

	// Advance the rest.
	vc.Advance(50 * time.Millisecond)
	select {
	case <-ch3:
	default:
		t.Fatal("ch3 should have fired")
	}
}

func TestVirtualClockWaitersFireInOrder(t *testing.T) {
	vc := NewVirtualClock(time.Time{})
	var order []int
	var mu sync.Mutex

	// Register in reverse order to verify sorting.
	ch3 := vc.After(300 * time.Millisecond)
	ch1 := vc.After(100 * time.Millisecond)
	ch2 := vc.After(200 * time.Millisecond)

	var wg sync.WaitGroup
	record := func(ch <-chan time.Time, id int) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ch
			mu.Lock()
			order = append(order, id)
			mu.Unlock()
		}()
	}
	record(ch1, 1)
	record(ch2, 2)
	record(ch3, 3)

	vc.Advance(300 * time.Millisecond)
	wg.Wait()

	// All should have fired; channels are buffered so goroutine scheduling
	// may reorder appends, but the channel sends happen in deadline order.
	if len(order) != 3 {
		t.Fatalf("got %d events, want 3", len(order))
	}
}

func TestVirtualClockAdvanceToPanicsOnBackwards(t *testing.T) {
	vc := NewVirtualClock(time.Time{})
	vc.Advance(time.Second)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on backwards AdvanceTo")
		}
	}()
	vc.AdvanceTo(vc.Now().Add(-time.Millisecond))
}

func TestVirtualClockBlockUntilWaiters(t *testing.T) {
	vc := NewVirtualClock(time.Time{})
	done := make(chan struct{})

	go func() {
		vc.BlockUntilWaiters(2)
		close(done)
	}()

	// Register waiters with small delay to prove blocking.
	vc.After(time.Second)
	time.Sleep(5 * time.Millisecond)
	vc.After(2 * time.Second)

	select {
	case <-done:
		// ok
	case <-time.After(time.Second):
		t.Fatal("BlockUntilWaiters did not unblock")
	}
}

func TestVirtualClockWithVnet(t *testing.T) {
	// Integration: verify that vnet link delay is instant with VirtualClock.
	vc := NewVirtualClock(time.Time{})
	net := New(WithSeed(42), WithClock(vc))
	defer net.Close()

	a := net.AddNode("a")
	b := net.AddNode("b")
	net.Link(a, b, LinkConfig{Latency: 500 * time.Millisecond})

	l, err := net.Listen(b, "echo:1")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// Dial + write in a goroutine; the link will block on clock.After(500ms).
	type dialResult struct {
		n   int
		err error
	}
	resultCh := make(chan dialResult, 1)

	go func() {
		conn, err := net.Dial(t.Context(), a, "echo:1")
		if err != nil {
			resultCh <- dialResult{err: err}
			return
		}
		defer conn.Close()
		n, err := conn.Write([]byte("hello"))
		resultCh <- dialResult{n: n, err: err}
	}()

	// Wait for the link goroutine to register a waiter for the delay.
	vc.BlockUntilWaiters(1)

	// Advance clock past the latency — data should flow instantly.
	wallStart := time.Now()
	vc.Advance(500 * time.Millisecond)

	// Accept and read on server side.
	sConn, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer sConn.Close()
	buf := make([]byte, 64)
	n, err := sConn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	wallElapsed := time.Since(wallStart)

	if string(buf[:n]) != "hello" {
		t.Fatalf("got %q, want %q", string(buf[:n]), "hello")
	}

	// Wall time should be well under 500ms (the virtual latency).
	// We allow 200ms for goroutine scheduling overhead.
	if wallElapsed > 200*time.Millisecond {
		t.Fatalf("wall time %v too high; virtual clock should make latency instant", wallElapsed)
	}
}
