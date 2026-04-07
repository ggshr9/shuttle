package selector

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/shuttleX/shuttle/transport"
)

// Strategy defines how transport selection works.
type Strategy string

const (
	StrategyAuto      Strategy = "auto"      // Automatically select best transport
	StrategyPriority  Strategy = "priority"  // Use first available in priority order
	StrategyLatency   Strategy = "latency"   // Use lowest latency transport
	StrategyMultipath Strategy = "multipath" // Use all transports simultaneously
)

// Selector manages multiple transports and selects the best one.
type Selector struct {
	transports        []transport.ClientTransport
	active            transport.ClientTransport
	strategy          Strategy
	probes            map[string]*ProbeResult
	migrator          *Migrator
	multipathPool     *MultipathPool
	connPool          *ConnPool
	serverAddr        string
	multipathSchedule string
	warmUpConns       int
	poolMaxIdle       int
	poolIdleTTL       time.Duration
	probeIntervalDur  time.Duration
	mu                sync.RWMutex
	logger            *slog.Logger
}

// ProbeResult stores health check results for a transport.
type ProbeResult struct {
	Transport transport.ClientTransport
	Latency   time.Duration
	Loss      float64
	Available bool
	LastCheck time.Time
}

// Config configures the transport selector.
type Config struct {
	Strategy          Strategy
	ProbeInterval     time.Duration
	MultipathSchedule string        // "weighted" (default), "min-latency", "load-balance"
	ServerAddr        string        // needed by multipath pool to dial persistent connections
	WarmUpConns       int           // pre-dial N connections on startup (0 = disabled)
	PoolMaxIdle       int           // max idle connections per transport (0 = default 4)
	PoolIdleTTL       time.Duration // idle connection TTL (0 = default 60s)
	DrainTimeout      time.Duration // max time to wait for streams to finish before force-closing (0 = default 30s)
}

// New creates a new transport selector.
func New(transports []transport.ClientTransport, cfg *Config, logger *slog.Logger) *Selector {
	if cfg == nil {
		cfg = &Config{
			Strategy:      StrategyAuto,
			ProbeInterval: 30 * time.Second,
		}
	}
	if logger == nil {
		logger = slog.Default()
	}
	s := &Selector{
		transports:        transports,
		strategy:          cfg.Strategy,
		probes:            make(map[string]*ProbeResult),
		serverAddr:        cfg.ServerAddr,
		multipathSchedule: cfg.MultipathSchedule,
		warmUpConns:       cfg.WarmUpConns,
		poolMaxIdle:       cfg.PoolMaxIdle,
		poolIdleTTL:       cfg.PoolIdleTTL,
		probeIntervalDur:  cfg.ProbeInterval,
		logger:            logger,
	}
	s.migrator = NewMigrator(logger, MigratorConfig{DrainTimeout: cfg.DrainTimeout})
	for _, t := range transports {
		s.probes[t.Type()] = &ProbeResult{
			Transport: t,
			Available: true,
		}
	}
	return s
}

// Start begins periodic probing of all transports.
// For multipath strategy, it also creates the MultipathPool with persistent connections.
// If WarmUpConns > 0, it creates a ConnPool and pre-dials connections.
func (s *Selector) Start(ctx context.Context) {
	if s.strategy == StrategyMultipath {
		sched := newScheduler(s.multipathSchedule)
		pool := NewMultipathPool(ctx, s.transports, s.serverAddr, sched, s.logger)
		s.mu.Lock()
		s.multipathPool = pool
		s.mu.Unlock()
	}

	if s.warmUpConns > 0 && len(s.transports) > 0 && s.serverAddr != "" {
		// Use the first transport for the connection pool.
		maxIdle := s.poolMaxIdle
		if maxIdle <= 0 {
			maxIdle = s.warmUpConns
		}
		cp := NewConnPool(s.transports[0], s.serverAddr, maxIdle, s.poolIdleTTL, s.logger)
		s.mu.Lock()
		s.connPool = cp
		s.mu.Unlock()
		cp.WarmUp(ctx, s.warmUpConns)
		go cp.evictLoop(ctx)
	}

	s.migrator.StartDrainLoop()
	go s.probeLoop(ctx)
}

func newScheduler(schedule string) StreamScheduler {
	switch schedule {
	case "min-latency":
		return NewMinLatencyScheduler()
	case "load-balance":
		return NewLoadBalanceScheduler()
	default:
		return NewWeightedLatencyScheduler()
	}
}

func (s *Selector) probeLoop(ctx context.Context) {
	interval := s.probeIntervalDur
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.probeAllParallel(ctx)
		}
	}
}

func (s *Selector) probeAllParallel(ctx context.Context) {
	s.mu.RLock()
	transports := make([]transport.ClientTransport, len(s.transports))
	copy(transports, s.transports)
	s.mu.RUnlock()

	type keyedResult struct {
		typ   string
		probe *ProbeResult
	}
	results := make(chan keyedResult, len(transports))

	for _, t := range transports {
		go func(t transport.ClientTransport) {
			results <- keyedResult{typ: t.Type(), probe: Probe(ctx, t)}
		}(t)
	}

	for range transports {
		r := <-results
		s.mu.Lock()
		s.probes[r.typ] = r.probe
		s.mu.Unlock()
	}

	s.mu.RLock()
	pool := s.multipathPool
	s.mu.RUnlock()
	if pool != nil {
		pool.UpdateMetrics(s.Probes())
	}

	s.maybeSwitch()
}

func (s *Selector) maybeSwitch() {
	s.mu.Lock()
	defer s.mu.Unlock()

	best := s.selectBest()
	if best == nil {
		return
	}
	if s.active != nil && s.active.Type() == best.Type() {
		return
	}
	s.logger.Info("switching transport", "from", s.activeType(), "to", best.Type())
	s.migrator.Migrate()
	s.active = best
}

func (s *Selector) activeType() string {
	if s.active == nil {
		return "none"
	}
	return s.active.Type()
}

func (s *Selector) selectBest() transport.ClientTransport {
	switch s.strategy {
	case StrategyLatency:
		return s.lowestLatency()
	case StrategyPriority:
		return s.firstAvailable()
	default: // auto
		return s.autoSelect()
	}
}

func (s *Selector) lowestLatency() transport.ClientTransport {
	var best *ProbeResult
	for _, p := range s.probes {
		if !p.Available {
			continue
		}
		if best == nil || p.Latency < best.Latency {
			best = p
		}
	}
	if best != nil {
		return best.Transport
	}
	return nil
}

func (s *Selector) firstAvailable() transport.ClientTransport {
	for _, t := range s.transports {
		if p := s.probes[t.Type()]; p != nil && p.Available {
			return t
		}
	}
	return nil
}

func (s *Selector) autoSelect() transport.ClientTransport {
	// Prefer H3 > Reality > CDN, but switch if current is unavailable
	for _, t := range s.transports {
		if p := s.probes[t.Type()]; p != nil && p.Available {
			return t
		}
	}
	return nil
}

// Dial connects using the currently selected transport.
// In multipath mode, it returns a virtual connection that distributes streams.
// If a connection pool is configured, it tries to retrieve an idle connection first.
func (s *Selector) Dial(ctx context.Context, addr string) (transport.Connection, error) {
	s.mu.RLock()
	pool := s.multipathPool
	cp := s.connPool
	active := s.active
	strategy := s.strategy
	s.mu.RUnlock()

	if strategy == StrategyMultipath && pool != nil {
		return pool.VirtualConn(), nil
	}

	// Try the connection pool first for a pre-warmed connection.
	if cp != nil {
		conn, err := cp.Get(ctx)
		if err == nil {
			return conn, nil
		}
		// Pool exhausted or error — fall through to normal dial.
	}

	if active == nil {
		// Try each transport in order
		for _, t := range s.transports {
			conn, err := t.Dial(ctx, addr)
			if err == nil {
				s.mu.Lock()
				if s.active == nil {
					s.active = t
				}
				s.mu.Unlock()
				tc := s.migrator.Track(conn, t.Type())
				return &migratedConn{tc: tc, migrator: s.migrator, sel: s, addr: addr}, nil
			}
			s.logger.Debug("transport dial failed", "type", t.Type(), "err", err)
		}
		return nil, fmt.Errorf("all transports failed")
	}

	conn, err := active.Dial(ctx, addr)
	if err != nil {
		// Active transport failed, try fallback
		s.logger.Warn("active transport failed, trying fallback", "type", active.Type(), "err", err)
		return s.dialFallback(ctx, addr, active)
	}
	tc := s.migrator.Track(conn, active.Type())
	return &migratedConn{tc: tc, migrator: s.migrator, sel: s, addr: addr}, nil
}

func (s *Selector) dialFallback(ctx context.Context, addr string, failed transport.ClientTransport) (transport.Connection, error) {
	s.migrator.Migrate() // drain existing connections
	for _, t := range s.transports {
		if t.Type() == failed.Type() {
			continue
		}
		conn, err := t.Dial(ctx, addr)
		if err == nil {
			s.mu.Lock()
			s.active = t
			s.mu.Unlock()
			tc := s.migrator.Track(conn, t.Type())
			s.logger.Info("fell back to transport", "type", t.Type())
			return &migratedConn{tc: tc, migrator: s.migrator, sel: s, addr: addr}, nil
		}
	}
	return nil, fmt.Errorf("all fallback transports failed")
}

// Probes returns a snapshot of the latest probe results for all transports.
func (s *Selector) Probes() map[string]*ProbeResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*ProbeResult, len(s.probes))
	for k, v := range s.probes {
		cp := *v
		out[k] = &cp
	}
	return out
}

// ActiveTransport returns the type name of the currently active transport.
// In multipath mode it returns "multipath".
func (s *Selector) ActiveTransport() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.strategy == StrategyMultipath && s.multipathPool != nil {
		return "multipath"
	}
	return s.activeType()
}

// ActivePaths returns a snapshot of all multipath paths.
// Returns nil when not in multipath mode.
func (s *Selector) ActivePaths() []PathInfo {
	s.mu.RLock()
	pool := s.multipathPool
	s.mu.RUnlock()
	if pool == nil {
		return nil
	}
	return pool.PathInfos()
}

// SetStrategy changes the transport selection strategy at runtime.
// Switching to/from Multipath involves pool lifecycle management.
func (s *Selector) SetStrategy(ctx context.Context, strategy Strategy) error {
	s.mu.Lock()

	if s.strategy == strategy {
		s.mu.Unlock()
		return nil
	}

	old := s.strategy
	oldPool := s.multipathPool

	switch {
	case old != StrategyMultipath && strategy == StrategyMultipath:
		// Non-multipath → Multipath: create a new MultipathPool.
		sched := newScheduler(s.multipathSchedule)
		pool := NewMultipathPool(ctx, s.transports, s.serverAddr, sched, s.logger)
		s.multipathPool = pool
		s.strategy = strategy
		s.mu.Unlock()
		s.logger.Info("strategy switched", "from", string(old), "to", string(strategy))
		return nil

	case old == StrategyMultipath && strategy != StrategyMultipath:
		// Multipath → non-multipath: swap strategy, close pool in background.
		s.multipathPool = nil
		s.strategy = strategy
		s.mu.Unlock()
		if oldPool != nil {
			go func() {
				if err := oldPool.Close(); err != nil {
					s.logger.Warn("error closing multipath pool during strategy switch", "err", err)
				}
			}()
		}
		s.maybeSwitch()
		s.logger.Info("strategy switched", "from", string(old), "to", string(strategy))
		return nil

	default:
		// Non-multipath → non-multipath: just swap and re-evaluate.
		s.strategy = strategy
		s.mu.Unlock()
		s.maybeSwitch()
		s.logger.Info("strategy switched", "from", string(old), "to", string(strategy))
		return nil
	}
}

// Transports returns a copy of the configured transport list.
func (s *Selector) Transports() []transport.ClientTransport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]transport.ClientTransport, len(s.transports))
	copy(out, s.transports)
	return out
}

// Migrate marks all current connections as draining and triggers a transport
// re-evaluation. This is used by the ProactiveMigrator on network changes.
func (s *Selector) Migrate() {
	s.migrator.Migrate()
	s.probeAllParallel(context.Background())
}

func (s *Selector) Type() string { return "selector" }

// DrainingCount returns the number of connections currently draining.
func (s *Selector) DrainingCount() int {
	if s.migrator == nil {
		return 0
	}
	return s.migrator.DrainingCount()
}

// MigrationStats returns the current state of all tracked connections.
func (s *Selector) MigrationStats() []ConnMigrationStats {
	if s.migrator == nil {
		return nil
	}
	return s.migrator.Stats()
}

func (s *Selector) Close() error {
	s.mu.Lock()
	pool := s.multipathPool
	s.multipathPool = nil
	cp := s.connPool
	s.connPool = nil
	s.mu.Unlock()
	if pool != nil {
		pool.Close()
	}
	if cp != nil {
		cp.Close()
	}
	s.migrator.Close()
	for _, t := range s.transports {
		t.Close()
	}
	return nil
}

// migratedConn wraps a tracked connection so that OpenStream automatically
// goes through the Migrator. If the underlying connection is draining (due to
// a transport switch), OpenStream dials a fresh connection on the selector's
// currently active transport and opens the stream there instead.
type migratedConn struct {
	tc       *migrateConn
	migrator *Migrator
	sel      *Selector
	addr     string
	reconnMu sync.Mutex // serializes dialNewAndOpen to prevent connection leaks
}

func (c *migratedConn) OpenStream(ctx context.Context) (transport.Stream, error) {
	stream, err := c.tc.conn.OpenStream(ctx)
	if err != nil {
		return nil, err
	}
	ts, err := c.migrator.WrapStream(c.tc, stream)
	if err == ErrConnectionDraining {
		// Connection is draining — close the stream we just opened and dial fresh.
		stream.Close()
		return c.dialNewAndOpen(ctx)
	}
	if err != nil {
		stream.Close()
		return nil, err
	}
	return ts, nil
}

func (c *migratedConn) dialNewAndOpen(ctx context.Context) (transport.Stream, error) {
	c.reconnMu.Lock()
	defer c.reconnMu.Unlock()

	// Check if another goroutine already reconnected while we were waiting.
	if c.tc != nil && !c.tc.draining.Load() {
		stream, err := c.tc.conn.OpenStream(ctx)
		if err != nil {
			return nil, err
		}
		ts, err := c.migrator.WrapStream(c.tc, stream)
		if err != nil {
			stream.Close()
			return nil, err
		}
		return ts, nil
	}

	c.sel.mu.RLock()
	active := c.sel.active
	c.sel.mu.RUnlock()

	if active == nil {
		return nil, fmt.Errorf("no active transport for re-dial")
	}

	conn, err := active.Dial(ctx, c.addr)
	if err != nil {
		return nil, fmt.Errorf("re-dial failed: %w", err)
	}

	tc := c.migrator.Track(conn, active.Type())
	// Update our own tracked conn so future calls use the new connection.
	c.tc = tc

	stream, err := conn.OpenStream(ctx)
	if err != nil {
		return nil, err
	}
	ts, err := c.migrator.WrapStream(tc, stream)
	if err != nil {
		stream.Close()
		return nil, err
	}
	return ts, nil
}

func (c *migratedConn) AcceptStream(ctx context.Context) (transport.Stream, error) {
	return c.tc.conn.AcceptStream(ctx)
}

func (c *migratedConn) Close() error {
	return c.tc.conn.Close()
}

func (c *migratedConn) LocalAddr() net.Addr {
	return c.tc.conn.LocalAddr()
}

func (c *migratedConn) RemoteAddr() net.Addr {
	return c.tc.conn.RemoteAddr()
}

var _ transport.Connection = (*migratedConn)(nil)
var _ transport.ClientTransport = (*Selector)(nil)
