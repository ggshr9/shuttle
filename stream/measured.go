package stream

import (
	"io"
	"sync"
	"time"

	"github.com/shuttleX/shuttle/transport"
)

// MeasuredStream wraps a transport.Stream, recording read/write metrics.
type MeasuredStream struct {
	inner   transport.Stream
	metrics *StreamMetrics

	firstByteRecorded bool // avoids CAS on every Read after first byte
	closeOnce         sync.Once
}

var _ transport.Stream = (*MeasuredStream)(nil)

// NewMeasuredStream wraps s with the given metrics handle.
func NewMeasuredStream(s transport.Stream, m *StreamMetrics) *MeasuredStream {
	return &MeasuredStream{
		inner:   s,
		metrics: m,
	}
}

// Read reads from the underlying stream and updates BytesReceived.
// The first successful read records FirstByteTime.
func (ms *MeasuredStream) Read(p []byte) (int, error) {
	n, err := ms.inner.Read(p)
	if n > 0 {
		ms.metrics.BytesReceived.Add(int64(n))
		if !ms.firstByteRecorded {
			ms.metrics.SetFirstByte(time.Now())
			ms.firstByteRecorded = true
		}
	}
	if err != nil && err != io.EOF {
		ms.metrics.Errors.Add(1)
	}
	return n, err
}

// Write writes to the underlying stream and updates BytesSent.
func (ms *MeasuredStream) Write(p []byte) (int, error) {
	n, err := ms.inner.Write(p)
	if n > 0 {
		ms.metrics.BytesSent.Add(int64(n))
	}
	if err != nil {
		ms.metrics.Errors.Add(1)
	}
	return n, err
}

// Close closes the underlying stream and finalises metrics.
func (ms *MeasuredStream) Close() error {
	var closeErr error
	ms.closeOnce.Do(func() {
		closeErr = ms.inner.Close()
		ms.metrics.Duration.Store(int64(time.Since(ms.metrics.StartTime)))
		ms.metrics.Closed.Store(true)
	})
	return closeErr
}

// StreamID delegates to the underlying stream.
func (ms *MeasuredStream) StreamID() uint64 {
	return ms.inner.StreamID()
}

// Metrics returns the metrics handle for this stream.
func (ms *MeasuredStream) Metrics() *StreamMetrics {
	return ms.metrics
}

// SetPriority records the QoS priority level for this stream.
func (ms *MeasuredStream) SetPriority(p int) {
	ms.metrics.Priority.Store(int32(p))
}
