package vnet

import (
	"io"
	"sync"
	"time"
)

// LinkConfig describes network conditions for one direction of a link.
type LinkConfig struct {
	Latency   time.Duration // one-way delay
	Jitter    time.Duration // random ± variation on latency
	Loss      float64       // write loss probability 0.0–1.0
	Bandwidth int64         // bytes/sec rate limit, 0 = unlimited
	Seed      int64         // RNG seed for deterministic behavior
}

// link applies network conditions between a writer and a reader.
// It reads from src, applies conditions, and writes to dst.
type link struct {
	cfg   LinkConfig
	clock Clock
	rng   *deterministicRand

	// token bucket for bandwidth limiting
	mu        sync.Mutex
	tokens    float64
	lastFill  time.Time
	closeCh   chan struct{}
	closeOnce sync.Once
}

func newLink(cfg LinkConfig, clock Clock, rng *deterministicRand) *link {
	return &link{
		cfg:      cfg,
		clock:    clock,
		rng:      rng,
		tokens:   0, // start empty so first write must wait
		lastFill: clock.Now(),
		closeCh:  make(chan struct{}),
	}
}

// run copies data from src to dst applying link conditions.
// It runs until src returns EOF/error or the link is closed.
// When done, it closes dst so the reader gets EOF.
func (l *link) run(dst io.WriteCloser, src io.Reader) {
	defer dst.Close()
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-l.closeCh:
			return
		default:
		}

		n, err := src.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			l.deliver(dst, data)
		}
		if err != nil {
			return
		}
	}
}

// deliver applies loss, delay, jitter, and bandwidth to a chunk of data.
func (l *link) deliver(dst io.Writer, data []byte) {
	// Loss: drop with probability
	if l.cfg.Loss > 0 && l.rng.Float64() < l.cfg.Loss {
		return // silently drop
	}

	// Bandwidth: wait for tokens
	if l.cfg.Bandwidth > 0 {
		l.waitForTokens(len(data))
	}

	// Latency + Jitter: delay delivery
	delay := l.rng.Duration(l.cfg.Latency, l.cfg.Jitter)
	if delay > 0 {
		select {
		case <-l.clock.After(delay):
		case <-l.closeCh:
			return
		}
	}

	// Write (ignore errors — the conn will surface them)
	_, _ = dst.Write(data)
}

// waitForTokens blocks until enough bandwidth tokens are available.
func (l *link) waitForTokens(n int) {
	needed := float64(n)
	for needed > 0 {
		select {
		case <-l.closeCh:
			return
		default:
		}

		l.mu.Lock()
		now := l.clock.Now()
		elapsed := now.Sub(l.lastFill).Seconds()
		l.tokens += elapsed * float64(l.cfg.Bandwidth)
		l.lastFill = now
		// cap at 1 second burst
		cap := float64(l.cfg.Bandwidth)
		if cap < 1 {
			cap = 1
		}
		if l.tokens > cap {
			l.tokens = cap
		}

		if l.tokens >= needed {
			l.tokens -= needed
			l.mu.Unlock()
			return
		}
		// consume what we can, wait for more
		needed -= l.tokens
		l.tokens = 0
		l.mu.Unlock()

		// sleep until enough tokens accrue
		wait := time.Duration(needed / float64(l.cfg.Bandwidth) * float64(time.Second))
		if wait < time.Millisecond {
			wait = time.Millisecond
		}
		select {
		case <-l.clock.After(wait):
		case <-l.closeCh:
			return
		}
	}
}

func (l *link) close() {
	l.closeOnce.Do(func() { close(l.closeCh) })
}
