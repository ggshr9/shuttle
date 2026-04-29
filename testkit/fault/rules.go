package fault

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/ggshr9/shuttle/testkit/vnet"
)

// Action defines a fault behavior applied to read/write data.
type Action interface {
	// Apply transforms or fails the operation. It receives the data buffer
	// and returns potentially modified data and an error.
	Apply(data []byte) ([]byte, error)
	// Name returns a human-readable name for observability logging.
	Name() string
}

// delayAction sleeps for a fixed duration using the injector's clock.
type delayAction struct {
	d     time.Duration
	clock vnet.Clock
}

func (a *delayAction) Apply(data []byte) ([]byte, error) {
	a.clock.Sleep(a.d)
	return data, nil
}
func (a *delayAction) Name() string { return fmt.Sprintf("delay(%v)", a.d) }

// errorAction returns a fixed error, discarding the data.
type errorAction struct {
	err error
}

func (a *errorAction) Apply([]byte) ([]byte, error) {
	return nil, a.err
}
func (a *errorAction) Name() string { return fmt.Sprintf("error(%v)", a.err) }

// dropAction silently discards data (returns original length to caller).
type dropAction struct{}

func (a *dropAction) Apply(data []byte) ([]byte, error) {
	return nil, nil // sentinel: nil data, nil error means "drop"
}
func (a *dropAction) Name() string { return "drop" }

// corruptAction flips random bits in the data.
type corruptAction struct {
	rng *rand.Rand
}

func (a *corruptAction) Apply(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}
	out := make([]byte, len(data))
	copy(out, data)
	// Flip 1 bit at a random position
	idx := a.rng.Intn(len(out))
	bit := byte(1 << uint(a.rng.Intn(8)))
	out[idx] ^= bit
	return out, nil
}
func (a *corruptAction) Name() string { return "corrupt" }

// Rule describes a single fault injection rule with constraints.
type Rule struct {
	action      Action
	probability float64       // 0.0-1.0, default 1.0
	after       time.Duration // activate only after this duration since creation
	maxTimes    int           // max activations; 0 = unlimited
	timesUsed   int
	createdAt   time.Time
	clock       vnet.Clock // clock used for time checks
	rng         *rand.Rand // RNG used for probability checks
}

// matches checks whether this rule should fire, updating internal counters.
// Caller must hold the injector mutex.
func (r *Rule) matches() bool {
	// Check time constraint using the injector's clock.
	if r.after > 0 {
		elapsed := r.clock.Now().Sub(r.createdAt)
		if elapsed < r.after {
			return false
		}
	}
	// Check usage limit.
	if r.maxTimes > 0 && r.timesUsed >= r.maxTimes {
		return false
	}
	// Check probability.
	if r.probability < 1.0 && r.rng.Float64() >= r.probability {
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
	rb.rule.action = &delayAction{d: d, clock: rb.injector.clock}
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
// Uses the injector's RNG for deterministic corruption.
func (rb *RuleBuilder) Corrupt(seed int64) *RuleBuilder {
	rb.rule.action = &corruptAction{rng: rand.New(rand.NewSource(seed))}
	return rb
}

// WithProbability sets the probability (0.0-1.0) that the rule fires.
func (rb *RuleBuilder) WithProbability(p float64) *RuleBuilder {
	rb.rule.probability = p
	return rb
}

// After sets a delay before the rule becomes active (relative to creation time,
// measured by the injector's Clock).
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
	rb.rule.createdAt = rb.injector.clock.Now()
	rb.rule.clock = rb.injector.clock
	rb.rule.rng = rb.injector.rng
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
