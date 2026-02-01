package server

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"
)

// HealthChecker periodically probes nodes and updates their health status.
type HealthChecker struct {
	manager  *NodeManager
	interval time.Duration
	timeout  time.Duration
	mu       sync.Mutex
	cancel   context.CancelFunc
	logger   *slog.Logger
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(nm *NodeManager, interval time.Duration, logger *slog.Logger) *HealthChecker {
	if interval == 0 {
		interval = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HealthChecker{
		manager:  nm,
		interval: interval,
		timeout:  5 * time.Second,
		logger:   logger,
	}
}

// Start begins periodic health checks.
func (hc *HealthChecker) Start(ctx context.Context) {
	ctx, hc.cancel = context.WithCancel(ctx)
	go hc.loop(ctx)
}

// Stop stops health checking.
func (hc *HealthChecker) Stop() {
	if hc.cancel != nil {
		hc.cancel()
	}
}

func (hc *HealthChecker) loop(ctx context.Context) {
	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()

	// Run immediately on start
	hc.checkAll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.checkAll(ctx)
		}
	}
}

func (hc *HealthChecker) checkAll(ctx context.Context) {
	hc.manager.mu.RLock()
	nodes := make([]*Node, len(hc.manager.nodes))
	copy(nodes, hc.manager.nodes)
	hc.manager.mu.RUnlock()

	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(n *Node) {
			defer wg.Done()
			hc.checkNode(ctx, n)
		}(node)
	}
	wg.Wait()

	// Try to migrate to a better node
	hc.manager.Migrate(1.3) // Switch if 30% better
}

func (hc *HealthChecker) checkNode(ctx context.Context, node *Node) {
	probeCtx, cancel := context.WithTimeout(ctx, hc.timeout)
	defer cancel()

	latency, loss := hc.probe(probeCtx, node.Addr)
	hc.manager.UpdateScore(node.Name, latency, loss)

	hc.logger.Debug("health check",
		"node", node.Name,
		"latency", latency,
		"loss", loss,
		"score", node.Score)
}

func (hc *HealthChecker) probe(ctx context.Context, addr string) (time.Duration, float64) {
	const probeCount = 3
	var totalLatency time.Duration
	failures := 0

	for i := 0; i < probeCount; i++ {
		start := time.Now()
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		if err != nil {
			failures++
			continue
		}
		totalLatency += time.Since(start)
		conn.Close()
	}

	if failures == probeCount {
		return hc.timeout, 1.0
	}

	avgLatency := totalLatency / time.Duration(probeCount-failures)
	loss := float64(failures) / float64(probeCount)
	return avgLatency, loss
}
