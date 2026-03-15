package selector

import (
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shuttle-proxy/shuttle/transport"
)

// ErrConnectionDraining is returned when attempting to open a stream on a
// connection that has been marked for draining during a transport migration.
var ErrConnectionDraining = errors.New("connection is draining")

// migrateConn wraps a transport.Connection with stream lifecycle tracking.
type migrateConn struct {
	conn          transport.Connection
	transportName string
	activeStreams  atomic.Int32
	draining      atomic.Bool
	created       time.Time
}

// migrateStream wraps a transport.Stream to auto-decrement the parent's
// activeStreams counter on Close.
type migrateStream struct {
	transport.Stream
	parent *migrateConn
	closed atomic.Bool
}

func (s *migrateStream) Close() error {
	if s.closed.CompareAndSwap(false, true) {
		s.parent.activeStreams.Add(-1)
	}
	return s.Stream.Close()
}

// Migrator manages graceful connection migration during transport switches.
// When a transport switch occurs, existing connections are marked as draining.
// New streams are rejected on draining connections, while existing streams are
// allowed to complete. A background drain loop closes draining connections once
// all their active streams have finished.
type Migrator struct {
	mu          sync.Mutex
	connections []*migrateConn
	logger      *slog.Logger
	drainDone   chan struct{}
}

// NewMigrator creates a new Migrator.
func NewMigrator(logger *slog.Logger) *Migrator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Migrator{
		logger:    logger,
		drainDone: make(chan struct{}),
	}
}

// Track registers a new connection for stream tracking and returns the
// tracked wrapper.
func (m *Migrator) Track(conn transport.Connection, transportName string) *migrateConn {
	tc := &migrateConn{
		conn:          conn,
		transportName: transportName,
		created:       time.Now(),
	}
	m.mu.Lock()
	m.connections = append(m.connections, tc)
	m.mu.Unlock()
	m.logger.Info("tracking connection", "transport", transportName)
	return tc
}

// WrapStream wraps a stream opened on a tracked connection. It increments
// activeStreams and returns a trackedStream that decrements on Close.
// Returns ErrConnectionDraining if the connection is draining.
func (m *Migrator) WrapStream(tc *migrateConn, stream transport.Stream) (*migrateStream, error) {
	if tc.draining.Load() {
		return nil, ErrConnectionDraining
	}
	tc.activeStreams.Add(1)
	return &migrateStream{
		Stream: stream,
		parent: tc,
	}, nil
}

// Migrate marks all current connections as draining. Old connections will be
// closed automatically by the drain loop when their activeStreams reach 0.
func (m *Migrator) Migrate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, tc := range m.connections {
		if !tc.draining.Load() {
			tc.draining.Store(true)
			m.logger.Info("draining connection",
				"transport", tc.transportName,
				"active_streams", tc.activeStreams.Load())
		}
	}
}

// StartDrainLoop starts a background goroutine that periodically checks
// draining connections and closes those with 0 active streams.
func (m *Migrator) StartDrainLoop() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-m.drainDone:
				return
			case <-ticker.C:
				m.drainIdle()
			}
		}
	}()
}

// drainIdle closes draining connections that have no active streams and
// removes them from the tracked list.
func (m *Migrator) drainIdle() {
	m.mu.Lock()
	defer m.mu.Unlock()

	remaining := m.connections[:0]
	for _, tc := range m.connections {
		if tc.draining.Load() && tc.activeStreams.Load() == 0 {
			m.logger.Info("closing drained connection", "transport", tc.transportName)
			tc.conn.Close()
			continue
		}
		remaining = append(remaining, tc)
	}
	m.connections = remaining
}

// Close stops the drain loop and closes all tracked connections.
func (m *Migrator) Close() {
	// Signal the drain loop to stop.
	select {
	case <-m.drainDone:
		// already closed
	default:
		close(m.drainDone)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, tc := range m.connections {
		tc.conn.Close()
	}
	m.connections = nil
}

// ConnMigrationStats holds snapshot data about a tracked connection.
type ConnMigrationStats struct {
	Transport    string    `json:"transport"`
	ActiveStreams int32     `json:"active_streams"`
	Draining     bool      `json:"draining"`
	Created      time.Time `json:"created"`
}

// Stats returns the current state of all tracked connections.
func (m *Migrator) Stats() []ConnMigrationStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	stats := make([]ConnMigrationStats, len(m.connections))
	for i, tc := range m.connections {
		stats[i] = ConnMigrationStats{
			Transport:    tc.transportName,
			ActiveStreams: tc.activeStreams.Load(),
			Draining:     tc.draining.Load(),
			Created:      tc.created,
		}
	}
	return stats
}
