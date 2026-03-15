package stream

import (
	"context"
	"sync"
)

// Migrator tracks active streams and coordinates migration to a new connection.
type Migrator struct {
	mu      sync.Mutex
	streams map[uint64]*MigrationEntry
}

// MigrationEntry records a single active stream for migration tracking.
type MigrationEntry struct {
	StreamID uint64
	Target   string
	Cancel   context.CancelFunc
}

// NewMigrator creates a Migrator ready for use.
func NewMigrator() *Migrator {
	return &Migrator{streams: make(map[uint64]*MigrationEntry)}
}

// Register adds a stream to the migration tracker.
func (m *Migrator) Register(id uint64, target string, cancel context.CancelFunc) {
	m.mu.Lock()
	m.streams[id] = &MigrationEntry{StreamID: id, Target: target, Cancel: cancel}
	m.mu.Unlock()
}

// Unregister removes a stream from the migration tracker.
func (m *Migrator) Unregister(id uint64) {
	m.mu.Lock()
	delete(m.streams, id)
	m.mu.Unlock()
}

// ActiveCount returns the number of active streams.
func (m *Migrator) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.streams)
}

// CancelAll cancels all active streams (e.g., before reconnect).
func (m *Migrator) CancelAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.streams {
		if e.Cancel != nil {
			e.Cancel()
		}
	}
	m.streams = make(map[uint64]*MigrationEntry)
}
