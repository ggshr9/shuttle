package obfs

import (
	"io"
	"math/rand"
	"sync"
	"time"
)

// ShaperConfig configures traffic shaping.
type ShaperConfig struct {
	Enabled       bool          // Enable traffic shaping
	MinDelay      time.Duration // Minimum inter-packet delay (default 0)
	MaxDelay      time.Duration // Maximum inter-packet delay (default 50ms)
	ChunkMinSize  int           // Minimum chunk size for splitting (default 64)
	ChunkMaxSize  int           // Maximum chunk size for splitting (default 1400)
	PaddingChance float64       // Probability of adding dummy padding (0-1, default 0.1)
}

// DefaultShaperConfig returns a config with minimal overhead.
func DefaultShaperConfig() ShaperConfig {
	return ShaperConfig{
		Enabled:       false,
		MinDelay:      0,
		MaxDelay:      50 * time.Millisecond,
		ChunkMinSize:  64,
		ChunkMaxSize:  1400,
		PaddingChance: 0.1,
	}
}

// Shaper wraps an io.ReadWriter with traffic shaping.
// It randomizes packet sizes and timing to resist traffic analysis.
type Shaper struct {
	inner io.ReadWriter
	cfg   ShaperConfig
	mu    sync.Mutex // protects writes
	rng   *rand.Rand
}

// NewShaper creates a traffic shaper wrapping the given stream.
func NewShaper(inner io.ReadWriter, cfg ShaperConfig) *Shaper {
	return &Shaper{
		inner: inner,
		cfg:   cfg,
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())), //nolint:gosec // shaping randomness does not need crypto rand
	}
}

// Read passes through to the inner reader (shaping is applied on write side).
func (s *Shaper) Read(p []byte) (int, error) {
	return s.inner.Read(p)
}

// Write splits data into random-sized chunks with random delays.
func (s *Shaper) Write(p []byte) (int, error) {
	if !s.cfg.Enabled || len(p) == 0 {
		return s.inner.Write(p)
	}

	total := 0
	remaining := p

	for len(remaining) > 0 {
		// Compute chunk parameters under lock (quick)
		s.mu.Lock()
		chunkSize := s.randomInt(s.cfg.ChunkMinSize, s.cfg.ChunkMaxSize)
		if chunkSize > len(remaining) {
			chunkSize = len(remaining)
		}
		if chunkSize <= 0 {
			chunkSize = len(remaining)
		}
		var delay time.Duration
		if total > 0 && s.cfg.MaxDelay > 0 {
			delay = s.randomDuration(s.cfg.MinDelay, s.cfg.MaxDelay)
		}
		s.mu.Unlock()

		// Sleep and write without holding the mutex
		if delay > 0 {
			time.Sleep(delay)
		}

		chunk := remaining[:chunkSize]
		remaining = remaining[chunkSize:]

		n, err := s.inner.Write(chunk)
		total += n
		if err != nil {
			return total, err
		}
	}

	return total, nil
}

func (s *Shaper) randomInt(min, max int) int {
	if max <= min {
		return min
	}
	return min + s.rng.Intn(max-min)
}

func (s *Shaper) randomDuration(min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	return min + time.Duration(s.rng.Int63n(int64(max-min)))
}
