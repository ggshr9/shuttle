package limiter

import (
	"context"
	"sync"
)

// GoroutinePool limits the number of concurrent goroutines.
type GoroutinePool struct {
	sem chan struct{}
	wg  sync.WaitGroup
}

// NewGoroutinePool creates a pool with the given max concurrency.
func NewGoroutinePool(maxWorkers int) *GoroutinePool {
	if maxWorkers <= 0 {
		maxWorkers = 1000
	}
	return &GoroutinePool{
		sem: make(chan struct{}, maxWorkers),
	}
}

// Submit runs fn in a goroutine, blocking if the pool is full.
func (p *GoroutinePool) Submit(ctx context.Context, fn func()) error {
	select {
	case p.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	p.wg.Add(1)
	go func() {
		defer func() {
			<-p.sem
			p.wg.Done()
		}()
		fn()
	}()
	return nil
}

// Wait blocks until all submitted goroutines complete.
func (p *GoroutinePool) Wait() {
	p.wg.Wait()
}

// RateLimiter implements a token-bucket rate limiter.
type RateLimiter struct {
	tokens chan struct{}
}

// NewRateLimiter creates a rate limiter with the given capacity.
func NewRateLimiter(rate int) *RateLimiter {
	rl := &RateLimiter{
		tokens: make(chan struct{}, rate),
	}
	// Fill initial tokens
	for i := 0; i < rate; i++ {
		rl.tokens <- struct{}{}
	}
	return rl
}

// Allow consumes a token, returning true if available.
func (rl *RateLimiter) Allow() bool {
	select {
	case <-rl.tokens:
		return true
	default:
		return false
	}
}

// Refill adds a token back.
func (rl *RateLimiter) Refill() {
	select {
	case rl.tokens <- struct{}{}:
	default:
	}
}
