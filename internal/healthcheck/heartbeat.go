// Package healthcheck provides shared liveness/readiness primitives
// used by server admin and client GUI API health endpoints.
package healthcheck

import (
	"sync/atomic"
	"time"
)

type Heartbeat struct {
	lastNanos atomic.Int64
}

func NewHeartbeat() *Heartbeat {
	return &Heartbeat{}
}

func (h *Heartbeat) Tick() {
	h.lastNanos.Store(time.Now().UnixNano())
}

// IsAlive reports whether Tick was called within the threshold.
// A never-ticked heartbeat is not alive.
func (h *Heartbeat) IsAlive(threshold time.Duration) bool {
	last := h.lastNanos.Load()
	if last == 0 {
		return false
	}
	return time.Since(time.Unix(0, last)) < threshold
}

// Run starts a goroutine that ticks at the given interval until ctx
// is cancelled. Caller is responsible for context lifecycle.
func (h *Heartbeat) Run(stop <-chan struct{}, interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				h.Tick()
			}
		}
	}()
}
