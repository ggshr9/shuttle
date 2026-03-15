package engine

import (
	"fmt"
	"sync"
	"time"
)

// CircuitState represents the state of the circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal, allowing requests
	CircuitOpen                         // Blocking requests, waiting for cooldown
	CircuitHalfOpen                     // Allowing one probe request
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// CircuitBreakerConfig configures the circuit breaker.
type CircuitBreakerConfig struct {
	Threshold     int                                  // consecutive failures before opening (default 5)
	BaseCooldown  time.Duration                        // initial cooldown before half-open (default 10s)
	MaxCooldown   time.Duration                        // max cooldown after backoff (default 5min)
	OnStateChange func(CircuitState, time.Duration)    // callback on state transitions
}

// CircuitBreaker implements the circuit breaker pattern for transport connections.
type CircuitBreaker struct {
	mu               sync.Mutex
	state            CircuitState
	consecutiveFails int
	threshold        int
	cooldown         time.Duration // current cooldown (grows with backoff)
	baseCooldown     time.Duration
	maxCooldown      time.Duration
	lastFailTime     time.Time
	resetTimer       *time.Timer
	halfOpenUsed     bool // true if the single half-open probe has been issued
	onStateChange    func(CircuitState, time.Duration)
}

// NewCircuitBreaker creates a circuit breaker with the given config.
// Zero values in config are replaced with sensible defaults.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	threshold := cfg.Threshold
	if threshold <= 0 {
		threshold = 5
	}
	baseCooldown := cfg.BaseCooldown
	if baseCooldown <= 0 {
		baseCooldown = 10 * time.Second
	}
	maxCooldown := cfg.MaxCooldown
	if maxCooldown <= 0 {
		maxCooldown = 5 * time.Minute
	}
	return &CircuitBreaker{
		state:         CircuitClosed,
		threshold:     threshold,
		cooldown:      baseCooldown,
		baseCooldown:  baseCooldown,
		maxCooldown:   maxCooldown,
		onStateChange: cfg.OnStateChange,
	}
}

// Allow reports whether a request is allowed through the circuit breaker.
// Closed: always true. Open: always false. HalfOpen: true once, then false.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		return false
	case CircuitHalfOpen:
		if !cb.halfOpenUsed {
			cb.halfOpenUsed = true
			return true
		}
		return false
	default:
		return false
	}
}

// RecordSuccess records a successful request. Resets the circuit to Closed.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	oldState := cb.state
	cb.state = CircuitClosed
	cb.consecutiveFails = 0
	cb.cooldown = cb.baseCooldown
	cb.halfOpenUsed = false
	if cb.resetTimer != nil {
		cb.resetTimer.Stop()
		cb.resetTimer = nil
	}
	changed := oldState != CircuitClosed
	cb.mu.Unlock()

	if changed && cb.onStateChange != nil {
		cb.onStateChange(CircuitClosed, 0)
	}
}

// RecordFailure records a failed request. Returns true if the circuit just opened.
func (cb *CircuitBreaker) RecordFailure() bool {
	cb.mu.Lock()

	wasHalfOpen := cb.state == CircuitHalfOpen

	cb.consecutiveFails++
	cb.lastFailTime = time.Now()

	if wasHalfOpen {
		// Half-open failure: reopen with doubled cooldown
		cb.cooldown *= 2
		if cb.cooldown > cb.maxCooldown {
			cb.cooldown = cb.maxCooldown
		}
		cb.state = CircuitOpen
		cb.halfOpenUsed = false
		cooldown := cb.cooldown
		cb.startTimerLocked(cooldown)
		cb.mu.Unlock()

		if cb.onStateChange != nil {
			cb.onStateChange(CircuitOpen, cooldown)
		}
		return true
	}

	if cb.state == CircuitClosed && cb.consecutiveFails >= cb.threshold {
		cb.state = CircuitOpen
		cooldown := cb.cooldown
		cb.startTimerLocked(cooldown)
		cb.mu.Unlock()

		if cb.onStateChange != nil {
			cb.onStateChange(CircuitOpen, cooldown)
		}
		return true
	}

	cb.mu.Unlock()
	return false
}

// startTimerLocked starts the cooldown timer that transitions to HalfOpen.
// Must be called with cb.mu held.
func (cb *CircuitBreaker) startTimerLocked(cooldown time.Duration) {
	if cb.resetTimer != nil {
		cb.resetTimer.Stop()
	}
	cb.resetTimer = time.AfterFunc(cooldown, func() {
		cb.mu.Lock()
		if cb.state == CircuitOpen {
			cb.state = CircuitHalfOpen
			cb.halfOpenUsed = false
		}
		cb.mu.Unlock()

		if cb.onStateChange != nil {
			cb.onStateChange(CircuitHalfOpen, 0)
		}
	})
}

// Reset forces the circuit breaker back to Closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	oldState := cb.state
	cb.state = CircuitClosed
	cb.consecutiveFails = 0
	cb.cooldown = cb.baseCooldown
	cb.halfOpenUsed = false
	if cb.resetTimer != nil {
		cb.resetTimer.Stop()
		cb.resetTimer = nil
	}
	cb.mu.Unlock()

	if oldState != CircuitClosed && cb.onStateChange != nil {
		cb.onStateChange(CircuitClosed, 0)
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
