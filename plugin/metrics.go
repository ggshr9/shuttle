package plugin

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics collects connection and traffic statistics.
type Metrics struct {
	ActiveConns   atomic.Int64
	TotalConns    atomic.Int64
	BytesSent     atomic.Int64
	BytesReceived atomic.Int64

	// Speed sampling
	mu            sync.RWMutex
	lastSent      int64
	lastRecv      int64
	lastSample    time.Time
	uploadSpeed   int64 // bytes/sec
	downloadSpeed int64 // bytes/sec
}

func NewMetrics() *Metrics {
	return &Metrics{lastSample: time.Now()}
}

func (m *Metrics) Name() string                  { return "metrics" }
func (m *Metrics) Init(ctx context.Context) error { return nil }
func (m *Metrics) Close() error                   { return nil }

func (m *Metrics) OnConnect(conn net.Conn, target string) (net.Conn, error) {
	m.ActiveConns.Add(1)
	m.TotalConns.Add(1)
	return conn, nil
}

func (m *Metrics) OnDisconnect(conn net.Conn) {
	m.ActiveConns.Add(-1)
}

func (m *Metrics) OnData(data []byte, dir Direction) []byte {
	switch dir {
	case Outbound:
		m.BytesSent.Add(int64(len(data)))
	case Inbound:
		m.BytesReceived.Add(int64(len(data)))
	}
	return data
}

// SampleSpeed calculates current upload/download speed since the last sample.
// Call this periodically (e.g. every 1 second).
func (m *Metrics) SampleSpeed() (upload, download int64) {
	now := time.Now()
	sent := m.BytesSent.Load()
	recv := m.BytesReceived.Load()

	m.mu.Lock()
	dt := now.Sub(m.lastSample).Seconds()
	if dt > 0 {
		m.uploadSpeed = int64(float64(sent-m.lastSent) / dt)
		m.downloadSpeed = int64(float64(recv-m.lastRecv) / dt)
	}
	m.lastSent = sent
	m.lastRecv = recv
	m.lastSample = now
	upload = m.uploadSpeed
	download = m.downloadSpeed
	m.mu.Unlock()
	return
}

// Speed returns the most recently sampled speed values.
func (m *Metrics) Speed() (upload, download int64) {
	m.mu.RLock()
	upload = m.uploadSpeed
	download = m.downloadSpeed
	m.mu.RUnlock()
	return
}

// Stats returns a snapshot of current metrics.
func (m *Metrics) Stats() map[string]int64 {
	up, down := m.Speed()
	return map[string]int64{
		"active_conns":   m.ActiveConns.Load(),
		"total_conns":    m.TotalConns.Load(),
		"bytes_sent":     m.BytesSent.Load(),
		"bytes_received": m.BytesReceived.Load(),
		"upload_speed":   up,
		"download_speed": down,
	}
}

var _ ConnPlugin = (*Metrics)(nil)
var _ DataPlugin = (*Metrics)(nil)
