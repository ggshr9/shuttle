package vnet

import (
	"math/rand"
	"sync"
	"time"
)

// deterministicRand provides thread-safe seeded random number generation.
type deterministicRand struct {
	mu  sync.Mutex
	rng *rand.Rand
}

func newRand(seed int64) *deterministicRand {
	return &deterministicRand{rng: rand.New(rand.NewSource(seed))} //nolint:gosec // G404: deterministic PRNG for reproducible test simulations
}

// Float64 returns a random float64 in [0.0, 1.0).
func (r *deterministicRand) Float64() float64 {
	r.mu.Lock()
	v := r.rng.Float64()
	r.mu.Unlock()
	return v
}

// Duration returns base ± random jitter.
func (r *deterministicRand) Duration(base, jitter time.Duration) time.Duration {
	if jitter == 0 {
		return base
	}
	r.mu.Lock()
	// uniform in [-jitter, +jitter]
	offset := time.Duration(r.rng.Int63n(int64(2*jitter+1))) - jitter
	r.mu.Unlock()
	d := base + offset
	if d < 0 {
		d = 0
	}
	return d
}

// childSeed derives a new seed from the parent RNG.
func (r *deterministicRand) childSeed() int64 {
	r.mu.Lock()
	s := r.rng.Int63()
	r.mu.Unlock()
	return s
}
