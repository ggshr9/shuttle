package selector

import (
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ggshr9/shuttle/transport"
)

// ErrConnectionDraining is returned when attempting to open a stream on a
// connection that has been marked for draining during a transport migration.
var ErrConnectionDraining = errors.New("connection is draining")

// MigratorConfig holds tunable parameters for a Migrator.
type MigratorConfig struct {
	// DrainInterval is how often the drain loop checks for idle connections.
	// Defaults to 5s when zero.
	DrainInterval time.Duration
	// DrainTimeout is the maximum time a draining connection is kept open
	// before being force-closed regardless of active streams.
	// Defaults to 30s when zero.
	DrainTimeout time.Duration
}

// migrateConn wraps a transport.Connection with stream lifecycle tracking.
type migrateConn struct {
	conn          transport.Connection
	transportName string
	activeStreams  atomic.Int32
	draining      atomic.Bool
	created       time.Time
	drainStarted  time.Time
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
// all their active streams have finished, or once DrainTimeout is exceeded.
type Migrator struct {
	mu            sync.Mutex
	connections   []*migrateConn
	logger        *slog.Logger
	drainDone     chan struct{}
	drainInterval time.Duration
	drainTimeout  time.Duration
}

// defaultDrainInterval is the default polling interval for the drain loop.
const defaultDrainInterval = 5 * time.Second

// defaultDrainTimeout is the default maximum drain period before force-close.
const defaultDrainTimeout = 30 * time.Second

// NewMigrator creates a new Migrator with the given optional config.
// Pass a zero-value MigratorConfig (or nil equivalently via the variadic
// parameter) to use all defaults.
func NewMigrator(logger *slog.Logger, cfgs ...MigratorConfig) *Migrator {
	if logger == nil {
		logger = slog.Default()
	}
	var cfg MigratorConfig
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	if cfg.DrainInterval <= 0 {
		cfg.DrainInterval = defaultDrainInterval
	}
	if cfg.DrainTimeout <= 0 {
		cfg.DrainTimeout = defaultDrainTimeout
	}
	return &Migrator{
		logger:        logger,
		drainDone:     make(chan struct{}),
		drainInterval: cfg.DrainInterval,
		drainTimeout:  cfg.DrainTimeout,
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
// closed automatically by the drain loop when their activeStreams reach 0, or
// when the DrainTimeout is exceeded.
func (m *Migrator) Migrate() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, tc := range m.connections {
		if !tc.draining.Load() {
			tc.draining.Store(true)
			tc.drainStarted = now
			m.logger.Info("draining connection",
				"transport", tc.transportName,
				"active_streams", tc.activeStreams.Load())
		}
	}
}

// StartDrainLoop starts a background goroutine that periodically checks
// draining connections and closes those with 0 active streams or those that
// have exceeded the DrainTimeout.
func (m *Migrator) StartDrainLoop() {
	go func() {
		ticker := time.NewTicker(m.drainInterval)
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

// drainIdle closes draining connections that have no active streams, or that
// have exceeded the DrainTimeout, and removes them from the tracked list.
func (m *Migrator) drainIdle() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	remaining := m.connections[:0]
	for _, tc := range m.connections {
		if tc.draining.Load() {
			idle := tc.activeStreams.Load() == 0
			timedOut := !tc.drainStarted.IsZero() && now.Sub(tc.drainStarted) > m.drainTimeout
			if idle || timedOut {
				if timedOut && !idle {
					m.logger.Warn("force-closing timed-out draining connection",
						"transport", tc.transportName,
						"active_streams", tc.activeStreams.Load(),
						"drain_age", now.Sub(tc.drainStarted).Round(time.Millisecond))
				} else {
					m.logger.Info("closing drained connection", "transport", tc.transportName)
				}
				tc.conn.Close()
				continue
			}
		}
		remaining = append(remaining, tc)
	}
	m.connections = remaining
}

// Close stops the drain loop and closes all tracked connections.
func (m *Migrator) Close() error {
	// Signal the drain loop to stop.
	select {
	case <-m.drainDone:
		// already closed
	default:
		close(m.drainDone)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	var firstErr error
	for _, tc := range m.connections {
		if err := tc.conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	m.connections = nil
	return firstErr
}

// ConnMigrationStats holds snapshot data about a tracked connection.
type ConnMigrationStats struct {
	Transport    string    `json:"transport"`
	ActiveStreams int32     `json:"active_streams"`
	Draining     bool      `json:"draining"`
	Created      time.Time `json:"created"`
	DrainStarted time.Time `json:"drain_started,omitempty"`
}

// DrainingCount returns the number of connections currently draining.
// Cheaper than Stats() when only the count is needed.
func (m *Migrator) DrainingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, tc := range m.connections {
		if tc.draining.Load() {
			count++
		}
	}
	return count
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
			DrainStarted: tc.drainStarted,
		}
	}
	return stats
}
