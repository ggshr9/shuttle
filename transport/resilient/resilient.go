// Package resilient provides a connection wrapper that adds automatic
// reconnection with exponential backoff to any transport.Connection.
package resilient

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shuttle-proxy/shuttle/transport"
)

// DialFunc creates a new transport.Connection. It is called during
// reconnection to establish a fresh underlying connection.
type DialFunc func(ctx context.Context) (transport.Connection, error)

// Config holds tunable parameters for the resilient connection wrapper.
type Config struct {
	MaxRetries  int           // Maximum consecutive reconnect attempts (default 5).
	BaseDelay   time.Duration // Initial backoff delay (default 500ms).
	MaxDelay    time.Duration // Upper bound on backoff delay (default 30s).
	OnReconnect func()        // Optional callback invoked after a successful reconnect.
}

func (c Config) withDefaults() Config {
	if c.MaxRetries <= 0 {
		c.MaxRetries = 5
	}
	if c.BaseDelay <= 0 {
		c.BaseDelay = 500 * time.Millisecond
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = 30 * time.Second
	}
	return c
}

// ResilientConn wraps a transport.Connection with automatic reconnection.
// When OpenStream detects a connection-level error, it triggers a reconnect
// with exponential backoff and retries the operation on the new connection.
type ResilientConn struct {
	dial  DialFunc
	inner transport.Connection
	mu    sync.RWMutex

	maxRetries  int
	baseDelay   time.Duration
	maxDelay    time.Duration
	onReconnect func()

	// reconnectMu ensures only one reconnect runs at a time.
	reconnectMu sync.Mutex
	closed      bool

	// sleepFn is an internal hook for testing backoff timing.
	sleepFn func(time.Duration)

	// Keepalive fields.
	stopKeepalive context.CancelFunc
	healthy       atomic.Bool
	tickerFn      func(time.Duration) tickerIface // test hook
}

// Wrap creates a ResilientConn around an existing connection.
// The dial function is used to establish replacement connections on failure.
func Wrap(initial transport.Connection, dial DialFunc, cfg Config) *ResilientConn {
	cfg = cfg.withDefaults()
	return &ResilientConn{
		dial:        dial,
		inner:       initial,
		maxRetries:  cfg.MaxRetries,
		baseDelay:   cfg.BaseDelay,
		maxDelay:    cfg.MaxDelay,
		onReconnect: cfg.OnReconnect,
		sleepFn:     time.Sleep,
	}
}

// isConnectionError returns true if the error indicates the connection is
// broken and a reconnect should be attempted.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
		return true
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	// Check for common network error patterns.
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	// Catch generic "closed" or "reset" errors from various transports.
	msg := err.Error()
	for _, substr := range []string{"closed", "reset", "broken pipe", "connection refused", "shutdown"} {
		if containsLower(msg, substr) {
			return true
		}
	}
	return false
}

func containsLower(s, substr string) bool {
	// Simple substring match (error messages are typically lowercase).
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// getInner returns the current inner connection under read lock.
func (rc *ResilientConn) getInner() transport.Connection {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.inner
}

// OpenStream attempts to open a stream on the underlying connection.
// If the operation fails with a connection-level error, it triggers a
// reconnect and retries on the new connection.
func (rc *ResilientConn) OpenStream(ctx context.Context) (transport.Stream, error) {
	conn := rc.getInner()
	stream, err := conn.OpenStream(ctx)
	if err == nil {
		return stream, nil
	}

	if !isConnectionError(err) {
		return nil, err
	}

	// Connection is broken — attempt reconnect.
	newConn, reconnErr := rc.reconnect(ctx, conn)
	if reconnErr != nil {
		return nil, reconnErr
	}

	return newConn.OpenStream(ctx)
}

// reconnect performs an exponential-backoff reconnect. Only one goroutine
// performs the actual reconnect; concurrent callers wait and share the result.
// staleConn is the connection the caller observed to be broken.
func (rc *ResilientConn) reconnect(ctx context.Context, staleConn transport.Connection) (transport.Connection, error) {
	rc.reconnectMu.Lock()
	defer rc.reconnectMu.Unlock()

	// Check if already closed.
	rc.mu.RLock()
	if rc.closed {
		rc.mu.RUnlock()
		return nil, net.ErrClosed
	}
	rc.mu.RUnlock()

	// Another goroutine may have already reconnected while we waited for
	// the lock. Check if the inner connection has changed.
	current := rc.getInner()
	if current != staleConn {
		return current, nil
	}

	var lastErr error
	for attempt := 0; attempt < rc.maxRetries; attempt++ {
		if attempt > 0 {
			delay := rc.baseDelay << uint(attempt-1)
			if delay > rc.maxDelay {
				delay = rc.maxDelay
			}
			// Add jitter: ±25% to prevent thundering herd on mass reconnect.
			// Cap the jittered value at maxDelay so we never exceed it.
			jitter := time.Duration(float64(delay) * (0.75 + rand.Float64()*0.5))
			if jitter > rc.maxDelay {
				jitter = rc.maxDelay
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				rc.sleepFn(jitter)
			}
		}

		newConn, err := rc.dial(ctx)
		if err != nil {
			lastErr = err
			continue
		}

		// Successfully dialed — swap the connection.
		rc.mu.Lock()
		old := rc.inner
		rc.inner = newConn
		rc.mu.Unlock()

		// Close the old connection (best-effort).
		old.Close()

		if rc.onReconnect != nil {
			rc.onReconnect()
		}

		return newConn, nil
	}

	return nil, lastErr
}

// AcceptStream delegates to the underlying connection.
func (rc *ResilientConn) AcceptStream(ctx context.Context) (transport.Stream, error) {
	return rc.getInner().AcceptStream(ctx)
}

// Close closes the resilient wrapper, stops the keepalive loop if running,
// and closes the underlying connection.
func (rc *ResilientConn) Close() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.closed = true
	if rc.stopKeepalive != nil {
		rc.stopKeepalive()
		rc.stopKeepalive = nil
	}
	return rc.inner.Close()
}

// LocalAddr returns the local address of the current underlying connection.
func (rc *ResilientConn) LocalAddr() net.Addr {
	return rc.getInner().LocalAddr()
}

// RemoteAddr returns the remote address of the current underlying connection.
func (rc *ResilientConn) RemoteAddr() net.Addr {
	return rc.getInner().RemoteAddr()
}
