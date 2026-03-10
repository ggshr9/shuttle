// Package server provides cluster management for multi-instance deployments.
// ClusterManager coordinates peer health checks, state synchronization,
// and load balancing across multiple shuttled instances.
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ClusterConfig configures server clustering.
type ClusterConfig struct {
	Enabled  bool          `yaml:"enabled" json:"enabled"`
	NodeName string        `yaml:"node_name" json:"node_name"` // unique name for this node
	Secret   string        `yaml:"secret" json:"secret"`       // shared secret for inter-node auth
	Peers    []PeerConfig  `yaml:"peers" json:"peers"`         // known peer nodes
	Interval string        `yaml:"interval" json:"interval"`   // health check interval (default "15s")
	MaxConns int64         `yaml:"max_conns" json:"max_conns"` // max conns before load shedding (0 = unlimited)
}

// PeerConfig defines a cluster peer.
type PeerConfig struct {
	Name string `yaml:"name" json:"name"`
	Addr string `yaml:"addr" json:"addr"` // admin API address, e.g. "10.0.0.2:9090"
}

// PeerState represents the runtime state of a cluster peer.
type PeerState struct {
	Name        string        `json:"name"`
	Addr        string        `json:"addr"`
	Status      NodeStatus    `json:"status"`
	ActiveConns int64         `json:"active_conns"`
	TotalConns  int64         `json:"total_conns"`
	BytesSent   int64         `json:"bytes_sent"`
	BytesRecv   int64         `json:"bytes_recv"`
	Latency     time.Duration `json:"latency_ns"`
	LastSeen    time.Time     `json:"last_seen"`
	Version     string        `json:"version"`
}

// ClusterManager manages a cluster of shuttled instances.
type ClusterManager struct {
	mu       sync.RWMutex
	nodeName string
	secret   string
	peers    map[string]*PeerState // keyed by name
	info     *ClusterNodeInfo      // local node info provider
	interval time.Duration
	maxConns int64
	cancel   context.CancelFunc
	client   *http.Client
	logger   *slog.Logger
}

// ClusterNodeInfo provides local node metrics to share with peers.
type ClusterNodeInfo struct {
	ActiveConns *atomic.Int64
	TotalConns  *atomic.Int64
	BytesSent   *atomic.Int64
	BytesRecv   *atomic.Int64
	Version     string
}

// NewClusterManager creates a new cluster manager.
func NewClusterManager(cfg *ClusterConfig, info *ClusterNodeInfo, logger *slog.Logger) *ClusterManager {
	if logger == nil {
		logger = slog.Default()
	}

	interval := 15 * time.Second
	if cfg.Interval != "" {
		if d, err := time.ParseDuration(cfg.Interval); err == nil && d > 0 {
			interval = d
		}
	}

	peers := make(map[string]*PeerState, len(cfg.Peers))
	for _, p := range cfg.Peers {
		peers[p.Name] = &PeerState{
			Name:   p.Name,
			Addr:   p.Addr,
			Status: NodeUnhealthy, // unknown until first check
		}
	}

	return &ClusterManager{
		nodeName: cfg.NodeName,
		secret:   cfg.Secret,
		peers:    peers,
		info:     info,
		interval: interval,
		maxConns: cfg.MaxConns,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger,
	}
}

// Start begins periodic peer health checks.
func (cm *ClusterManager) Start(ctx context.Context) {
	ctx, cm.cancel = context.WithCancel(ctx)
	go cm.loop(ctx)
}

// Stop stops the cluster manager.
func (cm *ClusterManager) Stop() {
	if cm.cancel != nil {
		cm.cancel()
	}
}

func (cm *ClusterManager) loop(ctx context.Context) {
	// Check immediately on start
	cm.checkPeers(ctx)

	ticker := time.NewTicker(cm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cm.checkPeers(ctx)
		}
	}
}

func (cm *ClusterManager) checkPeers(ctx context.Context) {
	cm.mu.RLock()
	names := make([]string, 0, len(cm.peers))
	for name := range cm.peers {
		names = append(names, name)
	}
	cm.mu.RUnlock()

	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			cm.checkPeer(ctx, n)
		}(name)
	}
	wg.Wait()
}

func (cm *ClusterManager) checkPeer(ctx context.Context, name string) {
	cm.mu.RLock()
	peer := cm.peers[name]
	if peer == nil {
		cm.mu.RUnlock()
		return
	}
	addr := peer.Addr
	cm.mu.RUnlock()

	start := time.Now()
	state, err := cm.fetchPeerStatus(ctx, addr)
	latency := time.Since(start)

	cm.mu.Lock()
	defer cm.mu.Unlock()

	p := cm.peers[name]
	if p == nil {
		return
	}

	if err != nil {
		p.Status = NodeUnhealthy
		p.Latency = latency
		cm.logger.Debug("peer check failed", "peer", name, "err", err)
		return
	}

	p.ActiveConns = state.ActiveConns
	p.TotalConns = state.TotalConns
	p.BytesSent = state.BytesSent
	p.BytesRecv = state.BytesRecv
	p.Version = state.Version
	p.Latency = latency
	p.LastSeen = time.Now()

	if latency > 500*time.Millisecond {
		p.Status = NodeDegraded
	} else {
		p.Status = NodeHealthy
	}

	cm.logger.Debug("peer check ok", "peer", name, "conns", state.ActiveConns, "latency", latency)
}

// peerStatusResponse mirrors the /api/status response.
type peerStatusResponse struct {
	Version     string `json:"version"`
	ActiveConns int64  `json:"active_conns"`
	TotalConns  int64  `json:"total_conns"`
	BytesSent   int64  `json:"bytes_sent"`
	BytesRecv   int64  `json:"bytes_recv"`
}

func (cm *ClusterManager) fetchPeerStatus(ctx context.Context, addr string) (*peerStatusResponse, error) {
	url := fmt.Sprintf("http://%s/api/status", addr)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cm.secret)

	resp, err := cm.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}

	var status peerStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &status, nil
}

// Peers returns the current state of all peers.
func (cm *ClusterManager) Peers() []PeerState {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make([]PeerState, 0, len(cm.peers))
	for _, p := range cm.peers {
		result = append(result, *p)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// LeastLoadedPeer returns the healthy peer with fewest active connections,
// or nil if no healthy peers are available.
func (cm *ClusterManager) LeastLoadedPeer() *PeerState {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var best *PeerState
	for _, p := range cm.peers {
		if p.Status == NodeUnhealthy {
			continue
		}
		if best == nil || p.ActiveConns < best.ActiveConns {
			cp := *p
			best = &cp
		}
	}
	return best
}

// ShouldShed returns true if this node's connection count exceeds the
// configured maximum and connections should be redirected to peers.
func (cm *ClusterManager) ShouldShed() bool {
	if cm.maxConns <= 0 || cm.info == nil {
		return false
	}
	return cm.info.ActiveConns.Load() >= cm.maxConns
}

// NodeName returns this node's name.
func (cm *ClusterManager) NodeName() string {
	return cm.nodeName
}

// LocalState returns this node's current state for sharing with peers.
func (cm *ClusterManager) LocalState() PeerState {
	var conns, total, sent, recv int64
	var ver string
	if cm.info != nil {
		conns = cm.info.ActiveConns.Load()
		total = cm.info.TotalConns.Load()
		sent = cm.info.BytesSent.Load()
		recv = cm.info.BytesRecv.Load()
		ver = cm.info.Version
	}
	return PeerState{
		Name:        cm.nodeName,
		Status:      NodeHealthy,
		ActiveConns: conns,
		TotalConns:  total,
		BytesSent:   sent,
		BytesRecv:   recv,
		Version:     ver,
		LastSeen:    time.Now(),
	}
}

// ForwardStream dials the best peer and relays the stream header + data.
// Returns the remote connection or an error if no peer is available.
func (cm *ClusterManager) ForwardStream(target string) (net.Conn, error) {
	peer := cm.LeastLoadedPeer()
	if peer == nil {
		return nil, fmt.Errorf("no healthy peers available")
	}

	// Dial the peer's proxy listen port (same as our listen port).
	// The peer accepts connections on the same transport ports.
	conn, err := net.DialTimeout("tcp", peer.Addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial peer %s: %w", peer.Name, err)
	}

	cm.logger.Info("forwarding to peer", "peer", peer.Name, "target", target)
	return conn, nil
}

// ClusterHandler returns an HTTP handler for cluster API endpoints.
func ClusterHandler(cm *ClusterManager) http.Handler {
	mux := http.NewServeMux()

	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			provided := r.Header.Get("Authorization")
			expected := "Bearer " + cm.secret
			if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}

	// Cluster status — returns all peers + local state
	mux.HandleFunc("GET /api/cluster", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"node":  cm.LocalState(),
			"peers": cm.Peers(),
		})
	}))

	// Cluster health — lightweight endpoint for peer health checks
	mux.HandleFunc("GET /api/cluster/health", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "node": cm.nodeName})
	}))

	return mux
}
