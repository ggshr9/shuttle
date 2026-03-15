package vnet

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/shuttle-proxy/shuttle/testkit/observe"
)

// LinkConfig describes network conditions for one direction of a link.
type LinkConfig struct {
	Latency      time.Duration // one-way delay
	Jitter       time.Duration // random ± variation on latency
	Loss         float64       // write loss probability 0.0–1.0
	Bandwidth    int64         // bytes/sec rate limit, 0 = unlimited
	Seed         int64         // RNG seed for deterministic behavior
	ReorderPct   float64       // probability of reordering 0.0–1.0
	ReorderDelay time.Duration // extra delay for reordered packets (default: 2x Latency)
}

// link applies network conditions between a writer and a reader.
// It reads from src, applies conditions, and writes to dst.
type link struct {
	cfg      LinkConfig
	clock    Clock
	rng      *deterministicRand
	recorder *observe.Recorder

	// token bucket for bandwidth limiting
	mu        sync.Mutex
	tokens    float64
	lastFill  time.Time
	closeCh   chan struct{}
	closeOnce sync.Once

	// dynamic config allows mid-test changes
	dynCfg   *LinkConfig // if non-nil, overrides cfg
	dynMu    sync.RWMutex

	// ownership tracking for UpdateLink
	fromNode *Node
	toNode   *Node
}

// UpdateConfig changes the link conditions dynamically while traffic flows.
func (l *link) UpdateConfig(cfg LinkConfig) {
	l.dynMu.Lock()
	l.dynCfg = &cfg
	l.dynMu.Unlock()
}

// activeCfg returns the current effective config (dynamic override or original).
func (l *link) activeCfg() LinkConfig {
	l.dynMu.RLock()
	defer l.dynMu.RUnlock()
	if l.dynCfg != nil {
		return *l.dynCfg
	}
	return l.cfg
}

func newLink(cfg LinkConfig, clock Clock, rng *deterministicRand, rec *observe.Recorder, from, to *Node) *link {
	return &link{
		cfg:      cfg,
		clock:    clock,
		rng:      rng,
		recorder: rec,
		tokens:   0, // start empty so first write must wait
		lastFill: clock.Now(),
		closeCh:  make(chan struct{}),
		fromNode: from,
		toNode:   to,
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

// deliver applies loss, delay, jitter, bandwidth, and reordering to a chunk of data.
func (l *link) deliver(dst io.Writer, data []byte) {
	cfg := l.activeCfg()

	// Loss: drop with probability
	if cfg.Loss > 0 && l.rng.Float64() < cfg.Loss {
		if l.recorder != nil {
			l.recorder.Record(observe.Event{
				Kind:   "drop",
				From:   "link",
				Detail: "packet loss",
				Size:   len(data),
			})
		}
		return // silently drop
	}

	// Bandwidth: wait for tokens
	if cfg.Bandwidth > 0 {
		l.waitForTokens(len(data))
	}

	// Reorder: randomly delay some packets extra (before normal latency)
	if cfg.ReorderPct > 0 && l.rng.Float64() < cfg.ReorderPct {
		extraDelay := cfg.ReorderDelay
		if extraDelay <= 0 {
			extraDelay = cfg.Latency * 2
		}
		if l.recorder != nil {
			l.recorder.Record(observe.Event{
				Kind:   "reorder",
				From:   "link",
				Detail: fmt.Sprintf("extra delay %v", extraDelay),
				Size:   len(data),
			})
		}
		select {
		case <-l.clock.After(extraDelay):
		case <-l.closeCh:
			return
		}
	}

	// Latency + Jitter: delay delivery
	delay := l.rng.Duration(cfg.Latency, cfg.Jitter)
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

		cfg := l.activeCfg()
		bw := float64(cfg.Bandwidth)
		if bw <= 0 {
			return // bandwidth limit removed dynamically
		}

		l.mu.Lock()
		now := l.clock.Now()
		elapsed := now.Sub(l.lastFill).Seconds()
		l.tokens += elapsed * bw
		l.lastFill = now
		// cap at 1 second burst
		cap := bw
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
		wait := time.Duration(needed / bw * float64(time.Second))
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
