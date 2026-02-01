package selector

import (
	"context"
	"log/slog"
	"sync"

	"github.com/shuttle-proxy/shuttle/transport"
)

// Migrator handles seamless migration of active streams between transports.
type Migrator struct {
	selector    *Selector
	activeConns []transport.Connection
	mu          sync.Mutex
	logger      *slog.Logger
}

// NewMigrator creates a new connection migrator.
func NewMigrator(sel *Selector, logger *slog.Logger) *Migrator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Migrator{
		selector: sel,
		logger:   logger,
	}
}

// Migrate moves new traffic to a new transport while letting existing streams
// complete on the old connection.
func (m *Migrator) Migrate(ctx context.Context, newTransport transport.ClientTransport, addr string) (transport.Connection, error) {
	newConn, err := newTransport.Dial(ctx, addr)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.activeConns = append(m.activeConns, newConn)
	m.mu.Unlock()

	m.logger.Info("migrated to new connection",
		"transport", newTransport.Type(),
		"remote", newConn.RemoteAddr())

	return newConn, nil
}

// Cleanup closes idle old connections.
func (m *Migrator) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.activeConns) <= 1 {
		return
	}

	// Keep only the most recent connection, close older idle ones
	for i := 0; i < len(m.activeConns)-1; i++ {
		m.activeConns[i].Close()
	}
	m.activeConns = m.activeConns[len(m.activeConns)-1:]
}
