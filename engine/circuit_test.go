package engine

import (
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_StartsClosedAllowsRequests(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{})
	if cb.State() != CircuitClosed {
		t.Fatalf("expected CircuitClosed, got %s", cb.State())
	}
	for i := 0; i < 10; i++ {
		if !cb.Allow() {
			t.Fatalf("expected Allow()=true on attempt %d", i)
		}
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{Threshold: 3})
	// First two failures: still closed
	for i := 0; i < 2; i++ {
		opened := cb.RecordFailure()
		if opened {
			t.Fatalf("circuit should not open after %d failures", i+1)
		}
		if cb.State() != CircuitClosed {
			t.Fatalf("expected CircuitClosed after %d failures", i+1)
		}
	}
	// Third failure: should open
	opened := cb.RecordFailure()
	if !opened {
		t.Fatal("expected circuit to open after threshold failures")
	}
	if cb.State() != CircuitOpen {
		t.Fatalf("expected CircuitOpen, got %s", cb.State())
	}
	if cb.Allow() {
		t.Fatal("expected Allow()=false when circuit is open")
	}
}

func TestCircuitBreaker_HalfOpenAfterCooldown(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Threshold:    2,
		BaseCooldown: 50 * time.Millisecond,
	})
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected CircuitOpen, got %s", cb.State())
	}
	// Wait for cooldown
	time.Sleep(100 * time.Millisecond)
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected CircuitHalfOpen after cooldown, got %s", cb.State())
	}
	// Should allow exactly one request
	if !cb.Allow() {
		t.Fatal("expected Allow()=true in half-open state")
	}
	if cb.Allow() {
		t.Fatal("expected Allow()=false after probe in half-open state")
	}
}

func TestCircuitBreaker_HalfOpenSuccess_ClosesCircuit(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Threshold:    2,
		BaseCooldown: 50 * time.Millisecond,
	})
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(100 * time.Millisecond)
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected CircuitHalfOpen, got %s", cb.State())
	}
	cb.Allow() // consume the probe
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected CircuitClosed after half-open success, got %s", cb.State())
	}
	// Should allow requests again
	if !cb.Allow() {
		t.Fatal("expected Allow()=true after circuit closed")
	}
}

func TestCircuitBreaker_HalfOpenFailure_ReopensWithBackoff(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Threshold:    2,
		BaseCooldown: 50 * time.Millisecond,
		MaxCooldown:  1 * time.Second,
	})
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(100 * time.Millisecond)
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected CircuitHalfOpen, got %s", cb.State())
	}
	cb.Allow() // consume the probe
	opened := cb.RecordFailure()
	if !opened {
		t.Fatal("expected circuit to reopen on half-open failure")
	}
	if cb.State() != CircuitOpen {
		t.Fatalf("expected CircuitOpen, got %s", cb.State())
	}
	// Cooldown should have doubled: 100ms now
	// Wait original cooldown (50ms) — should still be open
	time.Sleep(70 * time.Millisecond)
	if cb.State() != CircuitOpen {
		t.Fatalf("expected circuit still open after original cooldown, got %s", cb.State())
	}
	// Wait the rest of the doubled cooldown
	time.Sleep(80 * time.Millisecond)
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected CircuitHalfOpen after doubled cooldown, got %s", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{Threshold: 2})
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected CircuitOpen, got %s", cb.State())
	}
	cb.Reset()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected CircuitClosed after Reset, got %s", cb.State())
	}
	if !cb.Allow() {
		t.Fatal("expected Allow()=true after Reset")
	}
}

func TestCircuitBreaker_SuccessResetsCounter(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{Threshold: 3})
	cb.RecordFailure()
	cb.RecordFailure()
	// Two failures, then a success should reset counter
	cb.RecordSuccess()
	// Now need 3 more failures to open
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected CircuitClosed after 2 failures post-reset, got %s", cb.State())
	}
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected CircuitOpen after 3 consecutive failures, got %s", cb.State())
	}
}

func TestCircuitBreaker_CooldownCapped(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Threshold:    1,
		BaseCooldown: 50 * time.Millisecond,
		MaxCooldown:  100 * time.Millisecond,
	})

	// First trip: cooldown = 50ms
	cb.RecordFailure()
	time.Sleep(80 * time.Millisecond)
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected HalfOpen, got %s", cb.State())
	}

	// Fail in half-open: cooldown doubles to 100ms
	cb.Allow()
	cb.RecordFailure()
	time.Sleep(130 * time.Millisecond)
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected HalfOpen after 100ms cooldown, got %s", cb.State())
	}

	// Fail again: cooldown would be 200ms but capped at 100ms
	cb.Allow()
	cb.RecordFailure()
	// Should transition at 100ms, not 200ms
	time.Sleep(130 * time.Millisecond)
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected HalfOpen (cooldown should be capped at maxCooldown), got %s", cb.State())
	}
}

func TestCircuitBreaker_OnStateChangeCallback(t *testing.T) {
	var mu sync.Mutex
	var transitions []CircuitState

	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Threshold:    2,
		BaseCooldown: 50 * time.Millisecond,
		OnStateChange: func(state CircuitState, _ time.Duration) {
			mu.Lock()
			transitions = append(transitions, state)
			mu.Unlock()
		},
	})

	// Trip the breaker
	cb.RecordFailure()
	cb.RecordFailure()

	mu.Lock()
	if len(transitions) != 1 || transitions[0] != CircuitOpen {
		t.Fatalf("expected [CircuitOpen], got %v", transitions)
	}
	mu.Unlock()

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(transitions) != 2 || transitions[1] != CircuitHalfOpen {
		t.Fatalf("expected [CircuitOpen, CircuitHalfOpen], got %v", transitions)
	}
	mu.Unlock()

	// Success in half-open → closed
	cb.Allow()
	cb.RecordSuccess()

	mu.Lock()
	if len(transitions) != 3 || transitions[2] != CircuitClosed {
		t.Fatalf("expected [CircuitOpen, CircuitHalfOpen, CircuitClosed], got %v", transitions)
	}
	mu.Unlock()
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Threshold:    10,
		BaseCooldown: 50 * time.Millisecond,
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.Allow()
			cb.RecordFailure()
			cb.RecordSuccess()
			cb.State()
		}()
	}
	wg.Wait()

	// Should not panic and should be in a valid state
	state := cb.State()
	if state != CircuitClosed && state != CircuitOpen && state != CircuitHalfOpen {
		t.Fatalf("unexpected state: %s", state)
	}
}
