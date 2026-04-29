package selector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ggshr9/shuttle/transport"
)

// PathInfo is a read-only snapshot of a path's status for external reporting.
type PathInfo struct {
	Transport     string `json:"transport"`
	Latency       int64  `json:"latency_ms"`
	ActiveStreams int64  `json:"active_streams"`
	TotalStreams  int64  `json:"total_streams"`
	Available     bool   `json:"available"`
	Failures      int64  `json:"failures"`
	BytesSent     int64  `json:"bytes_sent"`
	BytesReceived int64  `json:"bytes_received"`
}

// PathMetrics tracks per-path quality metrics and holds the persistent connection.
type PathMetrics struct {
	Transport     transport.ClientTransport
	Conn          transport.Connection // guarded by mu
	Latency       time.Duration
	ActiveStreams int64 // atomic
	TotalStreams  int64 // atomic
	Failures      int64 // atomic — consecutive failures
	BytesSent     int64 // atomic — total bytes sent via this path
	BytesReceived int64 // atomic — total bytes received via this path
	available     atomic.Bool // replaces Available bool
	mu            sync.Mutex
}

// IsAvailable returns true if this path is currently available.
func (pm *PathMetrics) IsAvailable() bool { return pm.available.Load() }

// SetAvailable sets the availability state of this path.
func (pm *PathMetrics) SetAvailable(v bool) { pm.available.Store(v) }

// GetConn returns the current connection under the path mutex.
func (pm *PathMetrics) GetConn() transport.Connection {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.Conn
}

// MultipathPool manages persistent connections across all available transports.
type MultipathPool struct {
	paths                []*PathMetrics
	serverAddr           string
	scheduler            StreamScheduler
	pathFailureThreshold int64
	healthCheckInterval  time.Duration
	mu                   sync.RWMutex
	ctx                  context.Context
	cancel               context.CancelFunc
	logger               *slog.Logger
}

// NewMultipathPool creates a pool that dials all transports and starts health monitoring.
// pathFailureThreshold is the number of consecutive failures before a path is reconnected (0 = default 3).
// healthCheckInterval is how often to check paths (0 = default 10s).
func NewMultipathPool(
	ctx context.Context,
	transports []transport.ClientTransport,
	serverAddr string,
	scheduler StreamScheduler,
	pathFailureThreshold int,
	healthCheckInterval time.Duration,
	logger *slog.Logger,
) *MultipathPool {
	if logger == nil {
		logger = slog.Default()
	}
	threshold := int64(pathFailureThreshold)
	if threshold <= 0 {
		threshold = 3
	}
	if healthCheckInterval <= 0 {
		healthCheckInterval = 10 * time.Second
	}
	poolCtx, cancel := context.WithCancel(ctx)
	pool := &MultipathPool{
		serverAddr:           serverAddr,
		scheduler:            scheduler,
		pathFailureThreshold: threshold,
		healthCheckInterval:  healthCheckInterval,
		ctx:                  poolCtx,
		cancel:               cancel,
		logger:               logger,
	}

	for _, t := range transports {
		pm := &PathMetrics{
			Transport: t,
		}
		// available defaults to false via atomic.Bool zero value.
		// Attempt initial dial; failures are non-blocking — healthLoop will retry.
		conn, err := t.Dial(poolCtx, serverAddr)
		if err != nil {
			logger.Warn("multipath: initial dial failed, will retry", "transport", t.Type(), "err", err)
			atomic.StoreInt64(&pm.Failures, 1)
		} else {
			pm.Conn = conn
			pm.SetAvailable(true)
			logger.Info("multipath: path connected", "transport", t.Type())
		}
		pool.paths = append(pool.paths, pm)
	}

	// Propagate the failure threshold to the scheduler so filterEligible uses it.
	pool.scheduler.SetFailureThreshold(pool.pathFailureThreshold)
	go pool.healthLoop()
	return pool
}

// pickPath uses the scheduler to select the best available path.
func (p *MultipathPool) pickPath() *PathMetrics {
	p.mu.RLock()
	paths := p.paths
	p.mu.RUnlock()
	return p.scheduler.Pick(paths)
}

// VirtualConn returns a lightweight multipathConn wrapper that delegates
// OpenStream to the pool's scheduler.
func (p *MultipathPool) VirtualConn() transport.Connection {
	return &multipathConn{pool: p}
}

// PathInfos returns a snapshot of all path statuses.
func (p *MultipathPool) PathInfos() []PathInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]PathInfo, len(p.paths))
	for i, pm := range p.paths {
		out[i] = PathInfo{
			Transport:     pm.Transport.Type(),
			Latency:       pm.Latency.Milliseconds(),
			ActiveStreams: atomic.LoadInt64(&pm.ActiveStreams),
			TotalStreams:  atomic.LoadInt64(&pm.TotalStreams),
			Available:     pm.IsAvailable(),
			Failures:      atomic.LoadInt64(&pm.Failures),
			BytesSent:     atomic.LoadInt64(&pm.BytesSent),
			BytesReceived: atomic.LoadInt64(&pm.BytesReceived),
		}
	}
	return out
}

// UpdateMetrics applies probe results (keyed by transport type) to path latencies.
func (p *MultipathPool) UpdateMetrics(probes map[string]*ProbeResult) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, pm := range p.paths {
		if pr, ok := probes[pm.Transport.Type()]; ok {
			pm.mu.Lock()
			pm.Latency = pr.Latency
			pm.mu.Unlock()
			pm.SetAvailable(pr.Available)
		}
	}
}

// Close shuts down all persistent connections and stops the health loop.
func (p *MultipathPool) Close() error {
	p.cancel()
	p.mu.Lock()
	defer p.mu.Unlock()
	var firstErr error
	for _, pm := range p.paths {
		pm.mu.Lock()
		if pm.Conn != nil {
			if err := pm.Conn.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			pm.Conn = nil
		}
		pm.mu.Unlock()
		pm.SetAvailable(false)
	}
	return firstErr
}

// healthLoop periodically checks paths and reconnects failed ones.
func (p *MultipathPool) healthLoop() {
	ticker := time.NewTicker(p.healthCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.checkPaths()
		}
	}
}

func (p *MultipathPool) checkPaths() {
	p.mu.RLock()
	paths := p.paths
	p.mu.RUnlock()

	for _, pm := range paths {
		failures := atomic.LoadInt64(&pm.Failures)
		if failures >= p.pathFailureThreshold {
			// Close stale connection and try to reconnect.
			pm.mu.Lock()
			if pm.Conn != nil {
				pm.Conn.Close()
				pm.Conn = nil
			}
			pm.mu.Unlock()

			conn, err := pm.Transport.Dial(p.ctx, p.serverAddr)
			if err != nil {
				p.logger.Debug("multipath: reconnect failed", "transport", pm.Transport.Type(), "err", err)
				continue
			}
			pm.mu.Lock()
			pm.Conn = conn
			pm.mu.Unlock()
			pm.SetAvailable(true)
			atomic.StoreInt64(&pm.Failures, 0)
			p.logger.Info("multipath: path reconnected", "transport", pm.Transport.Type())
		}
	}
}

// --- multipathConn implements transport.Connection ---

type multipathConn struct {
	pool *MultipathPool
}

func (mc *multipathConn) OpenStream(ctx context.Context) (transport.Stream, error) {
	path := mc.pool.pickPath()
	if path == nil {
		return nil, errors.New("multipath: no available path")
	}

	path.mu.Lock()
	conn := path.Conn
	path.mu.Unlock()

	if conn == nil {
		atomic.AddInt64(&path.Failures, 1)
		return mc.openStreamFallback(ctx, path)
	}

	stream, err := conn.OpenStream(ctx)
	if err != nil {
		// Selected path failed; try fallback.
		atomic.AddInt64(&path.Failures, 1)
		return mc.openStreamFallback(ctx, path)
	}

	atomic.AddInt64(&path.ActiveStreams, 1)
	atomic.AddInt64(&path.TotalStreams, 1)
	atomic.StoreInt64(&path.Failures, 0)
	return &trackedStream{Stream: stream, path: path}, nil
}

func (mc *multipathConn) openStreamFallback(ctx context.Context, failed *PathMetrics) (transport.Stream, error) {
	mc.pool.mu.RLock()
	paths := mc.pool.paths
	mc.pool.mu.RUnlock()

	for _, pm := range paths {
		if pm == failed {
			continue
		}
		if !pm.IsAvailable() || atomic.LoadInt64(&pm.Failures) >= mc.pool.pathFailureThreshold {
			continue
		}

		conn := pm.GetConn()
		if conn == nil {
			continue
		}
		stream, err := conn.OpenStream(ctx)
		if err != nil {
			atomic.AddInt64(&pm.Failures, 1)
			continue
		}
		atomic.AddInt64(&pm.ActiveStreams, 1)
		atomic.AddInt64(&pm.TotalStreams, 1)
		atomic.StoreInt64(&pm.Failures, 0)
		return &trackedStream{Stream: stream, path: pm}, nil
	}
	return nil, fmt.Errorf("multipath: all paths failed to open stream")
}

func (mc *multipathConn) AcceptStream(ctx context.Context) (transport.Stream, error) {
	return nil, errors.New("multipath: AcceptStream not supported on client")
}

// Close is a no-op — pool lifecycle is managed by Selector.
func (mc *multipathConn) Close() error { return nil }

func (mc *multipathConn) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (mc *multipathConn) RemoteAddr() net.Addr { return &net.TCPAddr{} }

// --- trackedStream wraps a real stream and tracks ActiveStreams ---

type trackedStream struct {
	transport.Stream
	path   *PathMetrics
	closed atomic.Bool
}

func (ts *trackedStream) Read(b []byte) (int, error) {
	n, err := ts.Stream.Read(b)
	if n > 0 {
		atomic.AddInt64(&ts.path.BytesReceived, int64(n))
	}
	if err != nil {
		ts.decrement()
	}
	return n, err
}

func (ts *trackedStream) Write(b []byte) (int, error) {
	n, err := ts.Stream.Write(b)
	if n > 0 {
		atomic.AddInt64(&ts.path.BytesSent, int64(n))
	}
	return n, err
}

func (ts *trackedStream) Close() error {
	ts.decrement()
	return ts.Stream.Close()
}

func (ts *trackedStream) decrement() {
	if ts.closed.CompareAndSwap(false, true) {
		atomic.AddInt64(&ts.path.ActiveStreams, -1)
	}
}

var (
	_ transport.Connection = (*multipathConn)(nil)
	_ transport.Stream     = (*trackedStream)(nil)
)
