package vnet

import (
	"sort"
	"sync"
	"time"
)

// Clock abstracts time operations for testability.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
	Sleep(d time.Duration)
}

// RealClock uses the standard time package.
type RealClock struct{}

func (RealClock) Now() time.Time                        { return time.Now() }
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (RealClock) Sleep(d time.Duration)                  { time.Sleep(d) }

// ---------------------------------------------------------------------------
// VirtualClock — deterministic, controllable clock for fast testing
// ---------------------------------------------------------------------------

// VirtualClock is a controllable clock for deterministic testing.
// Time does not advance automatically; the test must call Advance or AdvanceTo.
// All waiters registered via After/Sleep are triggered when time advances past
// their deadline.
type VirtualClock struct {
	mu      sync.Mutex
	cond    *sync.Cond
	now     time.Time
	waiters []virtualWaiter
}

type virtualWaiter struct {
	deadline time.Time
	ch       chan time.Time
}

// NewVirtualClock creates a VirtualClock starting at the given time.
// If zero, defaults to 2024-01-01T00:00:00Z.
func NewVirtualClock(start time.Time) *VirtualClock {
	if start.IsZero() {
		start = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	vc := &VirtualClock{now: start}
	vc.cond = sync.NewCond(&vc.mu)
	return vc
}

// Now returns the current virtual time.
func (vc *VirtualClock) Now() time.Time {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	return vc.now
}

// After returns a channel that receives the virtual time when it advances
// past now+d. The channel has buffer size 1 so senders never block.
func (vc *VirtualClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	if d <= 0 {
		vc.mu.Lock()
		ch <- vc.now
		vc.mu.Unlock()
		return ch
	}
	vc.mu.Lock()
	deadline := vc.now.Add(d)
	vc.waiters = append(vc.waiters, virtualWaiter{deadline: deadline, ch: ch})
	vc.cond.Broadcast()
	vc.mu.Unlock()
	return ch
}

// Sleep blocks until the virtual time advances past now+d.
// Must be called from a goroutine other than the one calling Advance.
func (vc *VirtualClock) Sleep(d time.Duration) {
	if d <= 0 {
		return
	}
	<-vc.After(d)
}

// Advance moves the virtual clock forward by d and fires all waiters
// whose deadlines fall within the new time range. Waiters are fired
// in chronological order.
func (vc *VirtualClock) Advance(d time.Duration) {
	vc.mu.Lock()
	target := vc.now.Add(d)
	vc.mu.Unlock()
	vc.AdvanceTo(target)
}

// AdvanceTo moves the virtual clock to the target time and fires all
// waiters whose deadlines are ≤ target. Panics if target is before Now().
func (vc *VirtualClock) AdvanceTo(target time.Time) {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if target.Before(vc.now) {
		panic("VirtualClock.AdvanceTo: cannot go backwards")
	}

	// Sort waiters by deadline so we fire in order.
	sort.Slice(vc.waiters, func(i, j int) bool {
		return vc.waiters[i].deadline.Before(vc.waiters[j].deadline)
	})

	// Fire all waiters whose deadline ≤ target.
	remaining := vc.waiters[:0]
	for _, w := range vc.waiters {
		if !w.deadline.After(target) {
			// Advance now to the waiter's deadline before firing,
			// so that any code triggered sees the correct Now().
			if w.deadline.After(vc.now) {
				vc.now = w.deadline
			}
			w.ch <- vc.now
		} else {
			remaining = append(remaining, w)
		}
	}
	vc.waiters = remaining
	vc.now = target
	vc.cond.Broadcast()
}

// WaiterCount returns the number of pending waiters. Useful for tests
// to synchronize: spin until WaiterCount reaches the expected value
// before calling Advance.
func (vc *VirtualClock) WaiterCount() int {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	return len(vc.waiters)
}

// BlockUntilWaiters blocks until at least n waiters are registered.
// This avoids busy-spin polling in tests.
func (vc *VirtualClock) BlockUntilWaiters(n int) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	for len(vc.waiters) < n {
		vc.cond.Wait()
	}
}

