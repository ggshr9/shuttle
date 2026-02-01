package plugin

import (
	"context"
	"net"
	"sync/atomic"
)

// Metrics collects connection and traffic statistics.
type Metrics struct {
	ActiveConns   atomic.Int64
	TotalConns    atomic.Int64
	BytesSent     atomic.Int64
	BytesReceived atomic.Int64
}

func NewMetrics() *Metrics {
	return &Metrics{}
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

// Stats returns a snapshot of current metrics.
func (m *Metrics) Stats() map[string]int64 {
	return map[string]int64{
		"active_conns":   m.ActiveConns.Load(),
		"total_conns":    m.TotalConns.Load(),
		"bytes_sent":     m.BytesSent.Load(),
		"bytes_received": m.BytesReceived.Load(),
	}
}

var _ ConnPlugin = (*Metrics)(nil)
var _ DataPlugin = (*Metrics)(nil)
