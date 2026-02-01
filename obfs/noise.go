package obfs

import (
	"context"
	"crypto/rand"
	"io"
	"math/big"
	"time"
)

// NoiseGenerator sends random data during idle periods to prevent
// "silence when idle" traffic patterns from being detected.
type NoiseGenerator struct {
	writer      io.Writer
	minInterval time.Duration
	maxInterval time.Duration
	minSize     int
	maxSize     int
}

// NewNoiseGenerator creates a new idle noise generator.
func NewNoiseGenerator(w io.Writer) *NoiseGenerator {
	return &NoiseGenerator{
		writer:      w,
		minInterval: 1 * time.Second,
		maxInterval: 5 * time.Second,
		minSize:     64,
		maxSize:     512,
	}
}

// Run generates random noise until the context is cancelled.
func (ng *NoiseGenerator) Run(ctx context.Context) {
	for {
		interval := ng.randomDuration(ng.minInterval, ng.maxInterval)
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			size := ng.randomInt(ng.minSize, ng.maxSize)
			noise := make([]byte, size)
			rand.Read(noise)
			ng.writer.Write(noise)
		}
	}
}

func (ng *NoiseGenerator) randomDuration(min, max time.Duration) time.Duration {
	diff := max - min
	if diff <= 0 {
		return min
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(diff)))
	return min + time.Duration(n.Int64())
}

func (ng *NoiseGenerator) randomInt(min, max int) int {
	diff := max - min
	if diff <= 0 {
		return min
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(diff)))
	return min + int(n.Int64())
}
