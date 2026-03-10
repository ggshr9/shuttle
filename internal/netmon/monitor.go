package netmon

import (
	"context"
	"net"
	"sync"
	"time"
)

// Callback is called when a network change is detected.
type Callback func()

// Monitor watches for network interface changes and calls registered callbacks.
type Monitor struct {
	mu        sync.Mutex
	callbacks []Callback
	interval  time.Duration
	cancel    context.CancelFunc
	lastAddrs string // cached address snapshot for change detection
}

// New creates a network monitor with the given poll interval.
func New(interval time.Duration) *Monitor {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Monitor{
		interval: interval,
	}
}

// OnChange registers a callback to be invoked on network change.
func (m *Monitor) OnChange(cb Callback) {
	m.mu.Lock()
	m.callbacks = append(m.callbacks, cb)
	m.mu.Unlock()
}

// Start begins monitoring in a background goroutine.
func (m *Monitor) Start(ctx context.Context) {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	ctx, m.cancel = context.WithCancel(ctx)
	m.lastAddrs = getAddressSnapshot()
	m.mu.Unlock()

	go m.pollLoop(ctx)
}

// Stop stops the monitor.
func (m *Monitor) Stop() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.mu.Unlock()
}

func (m *Monitor) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := getAddressSnapshot()
			m.mu.Lock()
			changed := current != m.lastAddrs
			if changed {
				m.lastAddrs = current
			}
			callbacks := make([]Callback, len(m.callbacks))
			copy(callbacks, m.callbacks)
			m.mu.Unlock()

			if changed {
				for _, cb := range callbacks {
					cb()
				}
			}
		}
	}
}

// getAddressSnapshot returns a string representation of all network interface addresses.
// Changes to this string indicate a network change.
func getAddressSnapshot() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	var s string
	for _, addr := range addrs {
		s += addr.String() + ";"
	}
	return s
}
