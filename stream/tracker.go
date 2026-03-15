package stream

import (
	"sync"
	"time"
)

const defaultRingSize = 1000

// StreamTracker maintains a thread-safe ring buffer of stream metrics.
type StreamTracker struct {
	mu   sync.RWMutex
	ring []*StreamMetrics // ring buffer
	size int              // capacity
	head int              // next write position
	n    int              // total items written (may exceed size)

	// index maps stream ID to its metrics for O(1) lookup.
	index map[uint64]*StreamMetrics
}

// NewStreamTracker creates a tracker with the given ring buffer capacity.
// If size <= 0, defaultRingSize is used.
func NewStreamTracker(size int) *StreamTracker {
	if size <= 0 {
		size = defaultRingSize
	}
	return &StreamTracker{
		ring:  make([]*StreamMetrics, size),
		size:  size,
		index: make(map[uint64]*StreamMetrics),
	}
}

// Track begins tracking a new stream and returns its metrics handle.
func (t *StreamTracker) Track(id uint64, target, transport string) *StreamMetrics {
	m := &StreamMetrics{
		StreamID:  id,
		Target:    target,
		Transport: transport,
		StartTime: time.Now(),
	}

	t.mu.Lock()
	// If we're overwriting an old entry, remove it from the index.
	if old := t.ring[t.head]; old != nil {
		delete(t.index, old.StreamID)
	}
	t.ring[t.head] = m
	t.index[id] = m
	t.head = (t.head + 1) % t.size
	t.n++
	t.mu.Unlock()

	return m
}

// Get returns the metrics for a given stream ID, or nil if not found.
func (t *StreamTracker) Get(id uint64) *StreamMetrics {
	t.mu.RLock()
	m := t.index[id]
	t.mu.RUnlock()
	return m
}

// Active returns metrics for all streams that have not been closed.
func (t *StreamTracker) Active() []*StreamMetrics {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var out []*StreamMetrics
	for _, m := range t.index {
		if m != nil && !m.Closed.Load() {
			out = append(out, m)
		}
	}
	return out
}

// Recent returns up to n most recent stream metrics, newest first.
func (t *StreamTracker) Recent(n int) []*StreamMetrics {
	t.mu.RLock()
	defer t.mu.RUnlock()

	count := t.n
	if count > t.size {
		count = t.size
	}
	if n > count {
		n = count
	}

	out := make([]*StreamMetrics, 0, n)
	pos := (t.head - 1 + t.size) % t.size
	for i := 0; i < n; i++ {
		if m := t.ring[pos]; m != nil {
			out = append(out, m)
		}
		pos = (pos - 1 + t.size) % t.size
	}
	return out
}

// TransportStats holds aggregate stats for a single transport type.
type TransportStats struct {
	Transport    string               `json:"transport"`
	ActiveStreams int64               `json:"active_streams"`
	TotalStreams  int64               `json:"total_streams"`
	BytesSent    int64               `json:"bytes_sent"`
	BytesRecv    int64               `json:"bytes_recv"`
	Priorities   PriorityDistribution `json:"priorities"`
}

// ByTransport returns per-transport aggregate stats.
func (t *StreamTracker) ByTransport() []TransportStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	groups := make(map[string]*TransportStats)
	for _, m := range t.index {
		if m == nil {
			continue
		}
		ts, ok := groups[m.Transport]
		if !ok {
			ts = &TransportStats{Transport: m.Transport}
			groups[m.Transport] = ts
		}
		ts.TotalStreams++
		if !m.Closed.Load() {
			ts.ActiveStreams++
		}
		ts.BytesSent += m.BytesSent.Load()
		ts.BytesRecv += m.BytesReceived.Load()
		addToPriorityDist(&ts.Priorities, m.Priority.Load())
	}

	out := make([]TransportStats, 0, len(groups))
	for _, ts := range groups {
		out = append(out, *ts)
	}
	return out
}

// ByConnID returns all stream metrics for a given connection ID.
func (t *StreamTracker) ByConnID(connID string) []*StreamMetrics {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var out []*StreamMetrics
	for _, m := range t.index {
		if m != nil && m.ConnID == connID {
			out = append(out, m)
		}
	}
	return out
}

// Summary returns an aggregate snapshot of all tracked streams.
func (t *StreamTracker) Summary() StreamSummary {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var s StreamSummary
	var closedCount int64
	var totalDur int64

	for _, m := range t.index {
		if m == nil {
			continue
		}
		s.TotalStreams++
		s.TotalBytesSent += m.BytesSent.Load()
		s.TotalBytesRecv += m.BytesReceived.Load()
		if m.Closed.Load() {
			closedCount++
			totalDur += m.Duration.Load()
		} else {
			s.ActiveStreams++
		}
		addToPriorityDist(&s.Priorities, m.Priority.Load())
	}

	if closedCount > 0 {
		s.AvgDuration = time.Duration(totalDur / closedCount)
	}
	return s
}

// addToPriorityDist increments the appropriate counter in a PriorityDistribution.
func addToPriorityDist(d *PriorityDistribution, priority int32) {
	switch priority {
	case 0:
		d.Critical++
	case 1:
		d.High++
	case 2:
		d.Normal++
	case 3:
		d.Bulk++
	case 4:
		d.Low++
	default:
		d.Normal++ // unknown priority defaults to Normal
	}
}
