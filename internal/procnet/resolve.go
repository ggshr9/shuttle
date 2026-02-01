package procnet

import (
	"sync"
	"time"
)

// Resolver maps local TCP ports to process names with a cached PID→name table.
type Resolver struct {
	mu       sync.Mutex
	pidNames map[uint32]string
	lastLoad time.Time
	ttl      time.Duration
}

// NewResolver creates a Resolver with a 5-second cache TTL.
func NewResolver() *Resolver {
	return &Resolver{
		pidNames: make(map[uint32]string),
		ttl:      5 * time.Second,
	}
}

// Resolve returns the process name owning the given local TCP port.
// Returns "" if unknown.
func (r *Resolver) Resolve(localPort uint16) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if time.Since(r.lastLoad) > r.ttl {
		r.refresh()
	}

	pid := PortToPID(localPort)
	if pid == 0 {
		return ""
	}
	return r.pidNames[pid]
}

func (r *Resolver) refresh() {
	procs, err := ListNetworkProcesses()
	if err != nil {
		r.lastLoad = time.Now()
		return
	}

	names := make(map[uint32]string, len(procs))
	for _, p := range procs {
		names[p.PID] = p.Name
	}
	r.pidNames = names
	r.lastLoad = time.Now()
}
