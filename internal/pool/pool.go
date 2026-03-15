package pool

import (
	"sync"
	"sync/atomic"
)

// PoolStats tracks allocation statistics for a buffer pool tier.
type PoolStats struct {
	Gets   int64 `json:"gets"`
	Puts   int64 `json:"puts"`
	Hits   int64 `json:"hits"`   // reused from pool (gets - misses)
	Misses int64 `json:"misses"` // allocated new
	InUse  int64 `json:"in_use"` // gets - puts
}

// statsCounters holds per-tier atomic counters for lock-free stats.
type statsCounters struct {
	gets   atomic.Int64
	puts   atomic.Int64
	misses atomic.Int64
}

func (c *statsCounters) snapshot() *PoolStats {
	gets := c.gets.Load()
	puts := c.puts.Load()
	misses := c.misses.Load()
	hits := gets - misses
	if hits < 0 {
		hits = 0
	}
	return &PoolStats{
		Gets:   gets,
		Puts:   puts,
		Hits:   hits,
		Misses: misses,
		InUse:  gets - puts,
	}
}

func (c *statsCounters) reset() {
	c.gets.Store(0)
	c.puts.Store(0)
	c.misses.Store(0)
}

var (
	smallStats    statsCounters
	mediumStats   statsCounters
	medLargeStats statsCounters
	largeStats    statsCounters
)

// BufferPool manages reusable byte buffers to reduce GC pressure.
var (
	// Small buffers (1KB) for headers/control data
	SmallPool = &sync.Pool{
		New: func() any {
			smallStats.misses.Add(1)
			b := make([]byte, 1024)
			return &b
		},
	}

	// Medium buffers (16KB) for typical data transfer
	MediumPool = &sync.Pool{
		New: func() any {
			mediumStats.misses.Add(1)
			b := make([]byte, 16*1024)
			return &b
		},
	}

	// MedLargePool provides 32KB buffers for relay/copy operations.
	MedLargePool = &sync.Pool{
		New: func() any {
			medLargeStats.misses.Add(1)
			b := make([]byte, 32*1024)
			return &b
		},
	}

	// Large buffers (64KB) for high-throughput transfer
	LargePool = &sync.Pool{
		New: func() any {
			largeStats.misses.Add(1)
			b := make([]byte, 64*1024)
			return &b
		},
	}
)

// Get returns a buffer of at least the given size from the appropriate pool.
func Get(size int) []byte {
	switch {
	case size <= 1024:
		smallStats.gets.Add(1)
		bp := SmallPool.Get().(*[]byte)
		return (*bp)[:size]
	case size <= 16*1024:
		mediumStats.gets.Add(1)
		bp := MediumPool.Get().(*[]byte)
		return (*bp)[:size]
	case size <= 32*1024:
		medLargeStats.gets.Add(1)
		bp := MedLargePool.Get().(*[]byte)
		return (*bp)[:size]
	default:
		largeStats.gets.Add(1)
		bp := LargePool.Get().(*[]byte)
		return (*bp)[:min(size, 64*1024)]
	}
}

// GetMedLarge returns a 32KB buffer from the MedLarge pool.
func GetMedLarge() []byte {
	medLargeStats.gets.Add(1)
	bp := MedLargePool.Get().(*[]byte)
	return (*bp)[:32*1024]
}

// PutMedLarge zeroes and returns a buffer to the MedLarge pool.
func PutMedLarge(buf []byte) {
	c := cap(buf)
	b := buf[:c]
	clear(b)
	medLargeStats.puts.Add(1)
	MedLargePool.Put(&b)
}

// Put zeroes a buffer and returns it to the appropriate pool.
// Zeroing prevents sensitive data (keys, plaintext) from leaking to
// subsequent pool users.
func Put(buf []byte) {
	c := cap(buf)
	b := buf[:c]
	clear(b)
	switch {
	case c <= 1024:
		smallStats.puts.Add(1)
		SmallPool.Put(&b)
	case c <= 16*1024:
		mediumStats.puts.Add(1)
		MediumPool.Put(&b)
	case c <= 32*1024:
		medLargeStats.puts.Add(1)
		MedLargePool.Put(&b)
	default:
		largeStats.puts.Add(1)
		LargePool.Put(&b)
	}
}

// Stats returns allocation statistics per pool tier.
func Stats() map[string]*PoolStats {
	return map[string]*PoolStats{
		"small":     smallStats.snapshot(),
		"medium":    mediumStats.snapshot(),
		"med_large": medLargeStats.snapshot(),
		"large":     largeStats.snapshot(),
	}
}

// ResetStats clears all pool statistics counters.
func ResetStats() {
	smallStats.reset()
	mediumStats.reset()
	medLargeStats.reset()
	largeStats.reset()
}
