package limiter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGoroutinePoolBasic(t *testing.T) {
	pool := NewGoroutinePool(5)

	var count atomic.Int32
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		err := pool.Submit(ctx, func() {
			count.Add(1)
			time.Sleep(10 * time.Millisecond)
		})
		if err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	pool.Wait()

	if count.Load() != 10 {
		t.Fatalf("expected 10 completed tasks, got %d", count.Load())
	}
}

func TestGoroutinePoolMaxConcurrency(t *testing.T) {
	maxWorkers := 3
	pool := NewGoroutinePool(maxWorkers)

	var active atomic.Int32
	var maxActive atomic.Int32

	ctx := context.Background()
	for i := 0; i < 20; i++ {
		pool.Submit(ctx, func() {
			cur := active.Add(1)
			// Track max concurrent
			for {
				old := maxActive.Load()
				if cur <= old || maxActive.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			active.Add(-1)
		})
	}

	pool.Wait()

	if int(maxActive.Load()) > maxWorkers {
		t.Fatalf("max concurrent = %d, exceeds pool size %d", maxActive.Load(), maxWorkers)
	}
}

func TestGoroutinePoolContextCancellation(t *testing.T) {
	pool := NewGoroutinePool(1)

	ctx := context.Background()

	// Fill the pool
	started := make(chan struct{})
	pool.Submit(ctx, func() {
		close(started)
		time.Sleep(100 * time.Millisecond)
	})
	<-started

	// Try to submit with cancelled context — pool is full
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := pool.Submit(cancelCtx, func() {
		t.Error("should not run")
	})
	if err == nil {
		t.Fatal("expected context cancelled error")
	}

	pool.Wait()
}

func TestGoroutinePoolDefaultSize(t *testing.T) {
	pool := NewGoroutinePool(0) // Should default to 1000
	if cap(pool.sem) != 1000 {
		t.Fatalf("default pool size = %d, want 1000", cap(pool.sem))
	}
}

func TestGoroutinePoolNegativeSize(t *testing.T) {
	pool := NewGoroutinePool(-5) // Should default to 1000
	if cap(pool.sem) != 1000 {
		t.Fatalf("negative pool size = %d, want 1000", cap(pool.sem))
	}
}

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(3)

	// Should allow 3 times
	for i := 0; i < 3; i++ {
		if !rl.Allow() {
			t.Fatalf("expected Allow() = true for token %d", i)
		}
	}

	// 4th should fail
	if rl.Allow() {
		t.Fatal("expected Allow() = false when tokens exhausted")
	}
}

func TestRateLimiterRefill(t *testing.T) {
	rl := NewRateLimiter(2)

	// Drain all tokens
	rl.Allow()
	rl.Allow()

	if rl.Allow() {
		t.Fatal("should be empty after draining")
	}

	// Refill one
	rl.Refill()
	if !rl.Allow() {
		t.Fatal("should allow after refill")
	}
}

func TestRateLimiterRefillOverflow(t *testing.T) {
	rl := NewRateLimiter(2)

	// Refill when already full — should not block or panic
	rl.Refill()
	rl.Refill()
	rl.Refill()

	// Should still only have 2 tokens
	count := 0
	for rl.Allow() {
		count++
		if count > 10 {
			t.Fatal("too many tokens")
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 tokens, got %d", count)
	}
}

func TestRateLimiterConcurrent(t *testing.T) {
	rl := NewRateLimiter(100)

	var allowed atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rl.Allow() {
				allowed.Add(1)
			}
		}()
	}

	wg.Wait()

	if allowed.Load() != 100 {
		t.Fatalf("expected 100 allowed, got %d", allowed.Load())
	}
}

// Benchmarks
func BenchmarkGoroutinePoolSubmit(b *testing.B) {
	pool := NewGoroutinePool(100)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Submit(ctx, func() {})
	}
	pool.Wait()
}

func BenchmarkRateLimiterAllow(b *testing.B) {
	rl := NewRateLimiter(b.N + 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow()
	}
}
