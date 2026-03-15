package stream

import (
	"sync/atomic"
	"time"
)

// StreamMetrics holds per-stream telemetry counters.
// All mutable fields use atomic types for lock-free, concurrent access.
type StreamMetrics struct {
	StreamID  uint64
	Target    string
	Transport string

	StartTime     time.Time
	FirstByteTime atomic.Pointer[time.Time] // set once on first Read

	BytesSent     atomic.Int64
	BytesReceived atomic.Int64
	Errors        atomic.Int64
	Closed        atomic.Bool
	Priority      atomic.Int32 // QoS priority level (0=Critical … 4=Low)

	// Duration is set once when the stream is closed.
	Duration atomic.Int64 // nanoseconds
}

// SetFirstByte records the first-byte timestamp exactly once (CAS).
func (m *StreamMetrics) SetFirstByte(t time.Time) {
	m.FirstByteTime.CompareAndSwap(nil, &t)
}

// GetFirstByteTime returns the first-byte time or zero value if not yet set.
func (m *StreamMetrics) GetFirstByteTime() time.Time {
	if p := m.FirstByteTime.Load(); p != nil {
		return *p
	}
	return time.Time{}
}

// GetDuration returns the stream duration.
func (m *StreamMetrics) GetDuration() time.Duration {
	return time.Duration(m.Duration.Load())
}

// StreamSummary provides an aggregate view of all tracked streams.
type StreamSummary struct {
	TotalStreams    int64         `json:"total_streams"`
	ActiveStreams   int64         `json:"active_streams"`
	TotalBytesSent int64         `json:"total_bytes_sent"`
	TotalBytesRecv int64         `json:"total_bytes_recv"`
	AvgDuration    time.Duration `json:"avg_duration_ns"`
}
