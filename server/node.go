package server

import (
	"log/slog"
	"sync"
	"time"
)

// NodeStatus represents the health state of a node.
type NodeStatus int

const (
	NodeHealthy NodeStatus = iota
	NodeDegraded
	NodeUnhealthy
)

// Node represents a proxy server node.
type Node struct {
	Name      string
	Addr      string
	Score     float64
	Status    NodeStatus
	Latency   time.Duration
	Loss      float64
	LastCheck time.Time
}

// NodeManager manages multiple proxy nodes with health checking and failover.
type NodeManager struct {
	mu      sync.RWMutex
	nodes   []*Node
	active  *Node
	checker *HealthChecker
	logger  *slog.Logger
}

// NewNodeManager creates a new node manager.
func NewNodeManager(nodes []*Node, logger *slog.Logger) *NodeManager {
	if logger == nil {
		logger = slog.Default()
	}
	nm := &NodeManager{
		nodes:  nodes,
		logger: logger,
	}
	if len(nodes) > 0 {
		nm.active = nodes[0]
	}
	return nm
}

// Active returns the currently active node.
func (nm *NodeManager) Active() *Node {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.active
}

// BestNode returns the node with the highest score.
func (nm *NodeManager) BestNode() *Node {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	var best *Node
	for _, n := range nm.nodes {
		if n.Status == NodeUnhealthy {
			continue
		}
		if best == nil || n.Score > best.Score {
			best = n
		}
	}
	return best
}

// Migrate switches to a new active node if it's significantly better.
func (nm *NodeManager) Migrate(threshold float64) bool {
	best := nm.BestNode()
	if best == nil {
		return false
	}

	nm.mu.Lock()
	defer nm.mu.Unlock()

	if nm.active == nil || best.Score > nm.active.Score*threshold {
		nm.logger.Info("migrating to better node",
			"from", nm.activeName(),
			"to", best.Name,
			"score", best.Score)
		nm.active = best
		return true
	}
	return false
}

func (nm *NodeManager) activeName() string {
	if nm.active == nil {
		return "none"
	}
	return nm.active.Name
}

// UpdateScore recalculates a node's score based on latency and loss.
func (nm *NodeManager) UpdateScore(name string, latency time.Duration, loss float64) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	for _, n := range nm.nodes {
		if n.Name == name {
			n.Latency = latency
			n.Loss = loss
			n.LastCheck = time.Now()
			n.Score = calculateScore(latency, loss)
			if loss > 0.5 {
				n.Status = NodeUnhealthy
			} else if loss > 0.1 || latency > 500*time.Millisecond {
				n.Status = NodeDegraded
			} else {
				n.Status = NodeHealthy
			}
			return
		}
	}
}

func calculateScore(latency time.Duration, loss float64) float64 {
	// Score = 1000 / (latency_ms * (1 + loss*10))
	latMs := float64(latency) / float64(time.Millisecond)
	if latMs < 1 {
		latMs = 1
	}
	return 1000.0 / (latMs * (1 + loss*10))
}
