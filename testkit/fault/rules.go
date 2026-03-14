package fault

import (
	"math/rand"
	"sync"
	"time"
)

// Action defines a fault behavior applied to read/write data.
type Action interface {
	// Apply transforms or fails the operation. It receives the data buffer
	// and returns potentially modified data and an error.
	Apply(data []byte) ([]byte, error)
}

// delayAction sleeps for a fixed duration before passing data through.
type delayAction struct {
	d time.Duration
}

func (a *delayAction) Apply(data []byte) ([]byte, error) {
	time.Sleep(a.d)
	return data, nil
}

// errorAction returns a fixed error, discarding the data.
type errorAction struct {
	err error
}

func (a *errorAction) Apply([]byte) ([]byte, error) {
	return nil, a.err
}

// dropAction silently discards data (returns original length to caller).
type dropAction struct{}

func (a *dropAction) Apply(data []byte) ([]byte, error) {
	return nil, nil // sentinel: nil data, nil error means "drop"
}

// corruptAction flips random bits in the data.
type corruptAction struct {
	seed int64
	mu   sync.Mutex
	rng  *rand.Rand
}

func (a *corruptAction) Apply(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rng == nil {
		a.rng = rand.New(rand.NewSource(a.seed))
	}
	out := make([]byte, len(data))
	copy(out, data)
	// Flip 1 to ~12.5% of bits
	idx := a.rng.Intn(len(out))
	bit := byte(1 << uint(a.rng.Intn(8)))
	out[idx] ^= bit
	return out, nil
}

// Rule describes a single fault injection rule with constraints.
type Rule struct {
	action      Action
	probability float64       // 0.0-1.0, default 1.0
	after       time.Duration // activate only after this duration since creation
	maxTimes    int           // max activations; 0 = unlimited
	timesUsed   int
	createdAt   time.Time
}

// matches checks whether this rule should fire, updating internal counters.
// Caller must hold the injector mutex.
func (r *Rule) matches() bool {
	// Check time constraint.
	if r.after > 0 && time.Since(r.createdAt) < r.after {
		return false
	}
	// Check usage limit.
	if r.maxTimes > 0 && r.timesUsed >= r.maxTimes {
		return false
	}
	// Check probability.
	if r.probability < 1.0 && rand.Float64() >= r.probability {
		return false
	}
	r.timesUsed++
	return true
}

// RuleBuilder provides a fluent API for constructing fault rules.
type RuleBuilder struct {
	injector *Injector
	target   string // "read", "write", "dial"
	rule     Rule
}

// Delay adds a delay action to the rule.
func (rb *RuleBuilder) Delay(d time.Duration) *RuleBuilder {
	rb.rule.action = &delayAction{d: d}
	return rb
}

// Error adds an error-returning action to the rule.
func (rb *RuleBuilder) Error(err error) *RuleBuilder {
	rb.rule.action = &errorAction{err: err}
	return rb
}

// Drop adds a silent-discard action to the rule.
func (rb *RuleBuilder) Drop() *RuleBuilder {
	rb.rule.action = &dropAction{}
	return rb
}

// Corrupt adds a bit-flipping action to the rule.
func (rb *RuleBuilder) Corrupt(seed int64) *RuleBuilder {
	rb.rule.action = &corruptAction{seed: seed}
	return rb
}

// WithProbability sets the probability (0.0-1.0) that the rule fires.
func (rb *RuleBuilder) WithProbability(p float64) *RuleBuilder {
	rb.rule.probability = p
	return rb
}

// After sets a delay before the rule becomes active (wall-clock since creation).
func (rb *RuleBuilder) After(d time.Duration) *RuleBuilder {
	rb.rule.after = d
	return rb
}

// Times limits how many times the rule can fire.
func (rb *RuleBuilder) Times(n int) *RuleBuilder {
	rb.rule.maxTimes = n
	return rb
}

// Install finalizes the rule and adds it to the injector.
func (rb *RuleBuilder) Install() {
	rb.rule.createdAt = time.Now()
	rb.injector.mu.Lock()
	defer rb.injector.mu.Unlock()
	switch rb.target {
	case "read":
		rb.injector.readRules = append(rb.injector.readRules, rb.rule)
	case "write":
		rb.injector.writeRules = append(rb.injector.writeRules, rb.rule)
	case "dial":
		rb.injector.dialRules = append(rb.injector.dialRules, rb.rule)
	}
}
