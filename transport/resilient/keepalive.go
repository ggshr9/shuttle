package resilient

import (
	"context"
	"sync"
	"time"

	"github.com/shuttle-proxy/shuttle/transport"
)

// Ping/pong protocol markers exchanged over a control stream.
var (
	pingMarker = [4]byte{'P', 'I', 'N', 'G'}
	pongMarker = [4]byte{'P', 'O', 'N', 'G'}
)

// KeepaliveConfig controls the heartbeat probing behaviour.
type KeepaliveConfig struct {
	Interval    time.Duration // How often to ping (default 15s).
	Timeout     time.Duration // How long to wait for pong (default 5s).
	MaxFailures int           // Consecutive failures before triggering reconnect (default 2).
}

func (c KeepaliveConfig) withDefaults() KeepaliveConfig {
	if c.Interval <= 0 {
		c.Interval = 15 * time.Second
	}
	if c.Timeout <= 0 {
		c.Timeout = 5 * time.Second
	}
	if c.MaxFailures <= 0 {
		c.MaxFailures = 2
	}
	return c
}

// tickerIface abstracts time.Ticker for testing.
type tickerIface interface {
	C() <-chan time.Time
	Stop()
}

type realTicker struct{ t *time.Ticker }

func (r *realTicker) C() <-chan time.Time { return r.t.C }
func (r *realTicker) Stop()              { r.t.Stop() }

// newTicker creates a ticker. It is overridden in tests.
func (rc *ResilientConn) newTicker(d time.Duration) tickerIface {
	if rc.tickerFn != nil {
		return rc.tickerFn(d)
	}
	return &realTicker{t: time.NewTicker(d)}
}

// WithKeepalive starts a background heartbeat loop that proactively detects
// stale connections and closes the inner connection to trigger a reconnect
// on the next OpenStream call. It returns rc for chaining.
func (rc *ResilientConn) WithKeepalive(cfg KeepaliveConfig) *ResilientConn {
	cfg = cfg.withDefaults()

	rc.mu.Lock()
	if rc.stopKeepalive != nil {
		// Already running; stop the previous loop.
		rc.stopKeepalive()
	}
	ctx, cancel := context.WithCancel(context.Background())
	rc.stopKeepalive = cancel
	rc.healthy.Store(true)
	rc.mu.Unlock()

	go rc.keepaliveLoop(ctx, cfg)
	return rc
}

// IsHealthy reports whether the most recent heartbeat succeeded.
func (rc *ResilientConn) IsHealthy() bool {
	return rc.healthy.Load()
}

// keepaliveLoop runs the periodic ping check until ctx is cancelled.
func (rc *ResilientConn) keepaliveLoop(ctx context.Context, cfg KeepaliveConfig) {
	failures := 0
	ticker := rc.newTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
		}

		if rc.isClosed() {
			return
		}

		if rc.ping(ctx, cfg.Timeout) {
			failures = 0
			rc.healthy.Store(true)
		} else {
			failures++
			if failures >= cfg.MaxFailures {
				rc.healthy.Store(false)
				// Close the inner connection to force reconnect on next use.
				rc.closeInner()
				failures = 0
			}
		}
	}
}

// ping opens a stream, writes the ping marker, and reads a pong response
// within the given timeout. Returns true on success.
func (rc *ResilientConn) ping(parent context.Context, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	conn := rc.getInner()
	stream, err := conn.OpenStream(ctx)
	if err != nil {
		return false
	}
	defer stream.Close()

	if _, err := stream.Write(pingMarker[:]); err != nil {
		return false
	}

	var resp [4]byte
	if _, err := readFull(ctx, stream, resp[:]); err != nil {
		return false
	}

	return resp == pongMarker
}

// readFull reads exactly len(buf) bytes from r, respecting ctx cancellation.
func readFull(ctx context.Context, r transport.Stream, buf []byte) (int, error) {
	type result struct {
		n   int
		err error
	}
	ch := make(chan result, 1)
	go func() {
		n, err := readAllBytes(r, buf)
		ch <- result{n, err}
	}()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case res := <-ch:
		return res.n, res.err
	}
}

// readAllBytes reads exactly len(buf) bytes from r.
func readAllBytes(r transport.Stream, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// closeInner closes the current inner connection (best-effort) to trigger
// reconnect on the next OpenStream.
func (rc *ResilientConn) closeInner() {
	rc.mu.RLock()
	inner := rc.inner
	rc.mu.RUnlock()
	inner.Close()
}

// isClosed reports whether the ResilientConn has been closed.
func (rc *ResilientConn) isClosed() bool {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.closed
}

// ---------- StaleDetector ----------

// StaleDetector monitors a transport.Connection and fires a callback when
// no successful stream operation has occurred within the maxIdle duration.
// It is a standalone component not tied to ResilientConn.
type StaleDetector struct {
	conn    transport.Connection
	maxIdle time.Duration
	onStale func()

	mu          sync.Mutex
	lastSuccess time.Time
	stopped     bool
	cancel      context.CancelFunc

	// nowFn and tickerFn are test hooks.
	nowFn    func() time.Time
	tickerFn func(time.Duration) tickerIface
}

// NewStaleDetector creates and starts a StaleDetector. It checks the idle
// duration at intervals of maxIdle/2 (minimum 1s) and fires onStale when
// the connection has been idle for longer than maxIdle.
func NewStaleDetector(conn transport.Connection, maxIdle time.Duration, onStale func()) *StaleDetector {
	sd := &StaleDetector{
		conn:        conn,
		maxIdle:     maxIdle,
		onStale:     onStale,
		lastSuccess: time.Now(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	sd.cancel = cancel
	go sd.loop(ctx)
	return sd
}

// RecordSuccess records a successful stream operation timestamp.
func (sd *StaleDetector) RecordSuccess() {
	sd.mu.Lock()
	sd.lastSuccess = sd.now()
	sd.mu.Unlock()
}

// LastSuccess returns the timestamp of the last successful operation.
func (sd *StaleDetector) LastSuccess() time.Time {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	return sd.lastSuccess
}

// Stop halts the stale detection loop.
func (sd *StaleDetector) Stop() {
	sd.mu.Lock()
	sd.stopped = true
	if sd.cancel != nil {
		sd.cancel()
	}
	sd.mu.Unlock()
}

func (sd *StaleDetector) now() time.Time {
	if sd.nowFn != nil {
		return sd.nowFn()
	}
	return time.Now()
}

func (sd *StaleDetector) loop(ctx context.Context) {
	checkInterval := sd.maxIdle / 2
	if checkInterval < time.Second {
		checkInterval = time.Second
	}

	var ticker tickerIface
	if sd.tickerFn != nil {
		ticker = sd.tickerFn(checkInterval)
	} else {
		ticker = &realTicker{t: time.NewTicker(checkInterval)}
	}
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
		}

		sd.mu.Lock()
		if sd.stopped {
			sd.mu.Unlock()
			return
		}
		idle := sd.now().Sub(sd.lastSuccess)
		sd.mu.Unlock()

		if idle >= sd.maxIdle {
			sd.onStale()
		}
	}
}
