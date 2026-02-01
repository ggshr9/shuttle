package obfs

import (
	"math/rand"
	"time"
)

// TimingObfuscator adds random jitter to packet timing
// to eliminate timing-based traffic fingerprinting.
type TimingObfuscator struct {
	maxJitter time.Duration
	rng       *rand.Rand
}

// NewTimingObfuscator creates a new timing obfuscator.
// maxJitter is the maximum random delay added to each packet (default 2ms).
func NewTimingObfuscator(maxJitter time.Duration) *TimingObfuscator {
	if maxJitter <= 0 {
		maxJitter = 2 * time.Millisecond
	}
	return &TimingObfuscator{
		maxJitter: maxJitter,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Delay returns a random delay to apply before sending a packet.
func (t *TimingObfuscator) Delay() time.Duration {
	return time.Duration(t.rng.Int63n(int64(t.maxJitter)))
}

// Wait applies a random delay.
func (t *TimingObfuscator) Wait() {
	time.Sleep(t.Delay())
}
