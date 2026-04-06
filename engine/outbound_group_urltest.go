package engine

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/outbound/healthcheck"
)

// URLTestConfig configures the url-test group strategy.
type URLTestConfig struct {
	ToleranceMS int // don't switch unless improvement exceeds this (default 50ms)
}

// urlTestState manages periodic health checks and auto-selects the best outbound.
type urlTestState struct {
	checker    *healthcheck.Checker
	selected   atomic.Pointer[adapter.Outbound]
	tolerance  time.Duration
	mu         sync.Mutex
	logger     *slog.Logger
	cancelLoop context.CancelFunc
}

const defaultToleranceMS = 50

// newURLTestState creates a new urlTestState ready to be started.
func newURLTestState(checker *healthcheck.Checker, toleranceMS int, logger *slog.Logger) *urlTestState {
	if toleranceMS <= 0 {
		toleranceMS = defaultToleranceMS
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &urlTestState{
		checker:   checker,
		tolerance: time.Duration(toleranceMS) * time.Millisecond,
		logger:    logger,
	}
}

// Start begins the periodic health-check loop in a background goroutine.
func (s *urlTestState) Start(ctx context.Context, outbounds []adapter.Outbound) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop any existing loop before starting a new one.
	if s.cancelLoop != nil {
		s.cancelLoop()
	}

	loopCtx, cancel := context.WithCancel(ctx)
	s.cancelLoop = cancel
	go s.loop(loopCtx, outbounds)
}

// Stop cancels the background health-check loop.
func (s *urlTestState) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancelLoop != nil {
		s.cancelLoop()
		s.cancelLoop = nil
	}
}

// loop runs checkAll periodically based on the checker's configured interval.
func (s *urlTestState) loop(ctx context.Context, outbounds []adapter.Outbound) {
	// Run an initial check immediately.
	s.checkAll(ctx, outbounds)

	ticker := time.NewTicker(s.checker.Cfg().Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAll(ctx, outbounds)
		}
	}
}

// checkAll checks all member outbounds concurrently, then selects the best one.
func (s *urlTestState) checkAll(ctx context.Context, outbounds []adapter.Outbound) {
	var wg sync.WaitGroup
	wg.Add(len(outbounds))

	for _, ob := range outbounds {
		go func(ob adapter.Outbound) {
			defer wg.Done()
			s.checker.Check(ctx, ob.Tag(), ob.DialContext)
		}(ob)
	}

	wg.Wait()
	s.selectBest(outbounds)
}

// selectBest picks the lowest-latency available node. It only switches away from
// the current selection if the improvement exceeds the configured tolerance.
func (s *urlTestState) selectBest(outbounds []adapter.Outbound) {
	results := s.checker.Results()

	var bestOb adapter.Outbound
	var bestLatency time.Duration

	for _, ob := range outbounds {
		r, ok := results[ob.Tag()]
		if !ok || !r.Available {
			continue
		}
		if bestOb == nil || r.Latency < bestLatency {
			bestOb = ob
			bestLatency = r.Latency
		}
	}

	if bestOb == nil {
		return // no available outbound; keep current selection
	}

	current := s.selected.Load()

	// If we have a current selection that is still available, only switch if
	// the improvement exceeds the tolerance threshold.
	if current != nil {
		currentTag := (*current).Tag()
		if cr, ok := results[currentTag]; ok && cr.Available {
			if cr.Latency-bestLatency <= s.tolerance {
				return // current is good enough
			}
		}
	}

	// Switch to the new best.
	s.selected.Store(&bestOb)
	prevTag := "<none>"
	if current != nil {
		prevTag = (*current).Tag()
	}
	s.logger.Info("url-test selected outbound",
		"group", "url-test",
		"selected", bestOb.Tag(),
		"latency", bestLatency,
		"previous", prevTag,
	)
}

// dialURLTest uses the auto-selected node, falling back to failover if nil or fails.
func (g *OutboundGroup) dialURLTest(ctx context.Context, network, address string) (net.Conn, error) {
	if g.urlTest == nil {
		return g.dialFailover(ctx, network, address)
	}

	sel := g.urlTest.selected.Load()
	if sel != nil {
		conn, err := (*sel).DialContext(ctx, network, address)
		if err == nil {
			return conn, nil
		}
		// Selected node failed at dial time; fall through to failover.
		if g.logger != nil {
			g.logger.Warn("url-test selected node failed, falling back to failover",
				"node", (*sel).Tag(),
				"error", err,
			)
		}
	}

	// Fallback: try all outbounds in order (failover).
	return g.dialFailover(ctx, network, address)
}

// SetURLTest configures this group for url-test strategy with the given checker and tolerance.
func (g *OutboundGroup) SetURLTest(checker *healthcheck.Checker, toleranceMS int) {
	g.urlTest = newURLTestState(checker, toleranceMS, g.logger)
}

// StartURLTest starts the url-test background loop. Must be called after SetURLTest.
func (g *OutboundGroup) StartURLTest(ctx context.Context) {
	if g.urlTest != nil {
		g.urlTest.Start(ctx, g.outbounds)
	}
}

// URLTestSelected returns the tag of the currently auto-selected outbound, or "" if none.
func (g *OutboundGroup) URLTestSelected() string {
	if g.urlTest == nil {
		return ""
	}
	sel := g.urlTest.selected.Load()
	if sel == nil {
		return ""
	}
	return (*sel).Tag()
}
