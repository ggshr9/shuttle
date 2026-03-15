package selector

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/shuttle-proxy/shuttle/transport"
)

var errPoolClosed = errors.New("connection pool closed")

// ConnPool maintains a pool of idle transport connections to eliminate
// cold-start latency by pre-dialing connections on engine startup.
type ConnPool struct {
	mu        sync.Mutex
	idle      []*idleConn
	maxIdle   int
	idleTTL   time.Duration
	transport transport.ClientTransport
	addr      string
	logger    *slog.Logger
	closed    bool
}

type idleConn struct {
	conn      transport.Connection
	idleSince time.Time
}

// NewConnPool creates a connection pool for the given transport and server address.
func NewConnPool(t transport.ClientTransport, addr string, maxIdle int, logger *slog.Logger) *ConnPool {
	if maxIdle <= 0 {
		maxIdle = 4
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ConnPool{
		maxIdle:   maxIdle,
		idleTTL:   60 * time.Second,
		transport: t,
		addr:      addr,
		logger:    logger,
	}
}

// Get returns an idle connection from the pool, or dials a new one.
func (p *ConnPool) Get(ctx context.Context) (transport.Connection, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errPoolClosed
	}

	now := time.Now()
	for len(p.idle) > 0 {
		// Pop from end (LIFO — most recently used connection)
		ic := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]

		if now.Sub(ic.idleSince) > p.idleTTL {
			// Expired — close and try next
			p.mu.Unlock()
			ic.conn.Close()
			p.mu.Lock()
			if p.closed {
				p.mu.Unlock()
				return nil, errPoolClosed
			}
			continue
		}

		p.mu.Unlock()
		return ic.conn, nil
	}
	p.mu.Unlock()

	// No idle connection available — dial a new one.
	return p.transport.Dial(ctx, p.addr)
}

// Put returns a connection to the pool. If the pool is full or closed,
// the connection is closed instead.
func (p *ConnPool) Put(conn transport.Connection) {
	p.mu.Lock()
	if p.closed || len(p.idle) >= p.maxIdle {
		p.mu.Unlock()
		conn.Close()
		return
	}
	p.idle = append(p.idle, &idleConn{
		conn:      conn,
		idleSince: time.Now(),
	})
	p.mu.Unlock()
}

// WarmUp asynchronously dials count connections and adds them to the pool.
func (p *ConnPool) WarmUp(ctx context.Context, count int) {
	for i := 0; i < count; i++ {
		go func() {
			conn, err := p.transport.Dial(ctx, p.addr)
			if err != nil {
				p.logger.Debug("pool warm-up dial failed", "err", err)
				return
			}
			p.Put(conn)
		}()
	}
}

// Close closes all idle connections and marks the pool as closed.
func (p *ConnPool) Close() {
	p.mu.Lock()
	p.closed = true
	idle := p.idle
	p.idle = nil
	p.mu.Unlock()

	for _, ic := range idle {
		ic.conn.Close()
	}
}

// evictLoop periodically removes connections that have been idle beyond the TTL.
// It runs until the context is cancelled.
func (p *ConnPool) evictLoop(ctx context.Context) {
	ticker := time.NewTicker(p.idleTTL / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.evictExpired()
		}
	}
}

func (p *ConnPool) evictExpired() {
	now := time.Now()
	var toClose []transport.Connection

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	kept := p.idle[:0]
	for _, ic := range p.idle {
		if now.Sub(ic.idleSince) > p.idleTTL {
			toClose = append(toClose, ic.conn)
		} else {
			kept = append(kept, ic)
		}
	}
	p.idle = kept
	p.mu.Unlock()

	for _, c := range toClose {
		c.Close()
	}
}
