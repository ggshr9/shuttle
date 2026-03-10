package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClusterManager(t *testing.T) {
	cfg := &ClusterConfig{
		Enabled:  true,
		NodeName: "node-1",
		Secret:   "test-secret",
		Peers: []PeerConfig{
			{Name: "node-2", Addr: "10.0.0.2:9090"},
			{Name: "node-3", Addr: "10.0.0.3:9090"},
		},
		Interval: "10s",
		MaxConns: 500,
	}
	info := &ClusterNodeInfo{
		ActiveConns: &atomic.Int64{},
		TotalConns:  &atomic.Int64{},
		BytesSent:   &atomic.Int64{},
		BytesRecv:   &atomic.Int64{},
		Version:     "0.1.0",
	}

	cm := NewClusterManager(cfg, info, nil)

	if cm.NodeName() != "node-1" {
		t.Errorf("NodeName() = %q, want %q", cm.NodeName(), "node-1")
	}

	peers := cm.Peers()
	if len(peers) != 2 {
		t.Fatalf("Peers() len = %d, want 2", len(peers))
	}
	// Peers are sorted by name
	if peers[0].Name != "node-2" || peers[1].Name != "node-3" {
		t.Errorf("peers = %v, want node-2, node-3", peers)
	}
	// Initially unhealthy
	for _, p := range peers {
		if p.Status != NodeUnhealthy {
			t.Errorf("peer %s status = %d, want NodeUnhealthy", p.Name, p.Status)
		}
	}
}

func TestClusterManagerLocalState(t *testing.T) {
	info := &ClusterNodeInfo{
		ActiveConns: &atomic.Int64{},
		TotalConns:  &atomic.Int64{},
		BytesSent:   &atomic.Int64{},
		BytesRecv:   &atomic.Int64{},
		Version:     "0.1.0",
	}
	info.ActiveConns.Store(42)
	info.TotalConns.Store(100)
	info.BytesSent.Store(1024)
	info.BytesRecv.Store(2048)

	cm := NewClusterManager(&ClusterConfig{
		NodeName: "test-node",
		Secret:   "s",
	}, info, nil)

	state := cm.LocalState()
	if state.Name != "test-node" {
		t.Errorf("Name = %q, want %q", state.Name, "test-node")
	}
	if state.ActiveConns != 42 {
		t.Errorf("ActiveConns = %d, want 42", state.ActiveConns)
	}
	if state.TotalConns != 100 {
		t.Errorf("TotalConns = %d, want 100", state.TotalConns)
	}
	if state.BytesSent != 1024 {
		t.Errorf("BytesSent = %d, want 1024", state.BytesSent)
	}
	if state.BytesRecv != 2048 {
		t.Errorf("BytesRecv = %d, want 2048", state.BytesRecv)
	}
	if state.Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", state.Version, "0.1.0")
	}
	if state.Status != NodeHealthy {
		t.Errorf("Status = %d, want NodeHealthy", state.Status)
	}
}

func TestClusterManagerShouldShed(t *testing.T) {
	info := &ClusterNodeInfo{
		ActiveConns: &atomic.Int64{},
		TotalConns:  &atomic.Int64{},
		BytesSent:   &atomic.Int64{},
		BytesRecv:   &atomic.Int64{},
	}

	// No max conns — never shed
	cm := NewClusterManager(&ClusterConfig{
		NodeName: "n",
		Secret:   "s",
		MaxConns: 0,
	}, info, nil)
	if cm.ShouldShed() {
		t.Error("ShouldShed() = true with MaxConns=0")
	}

	// Below threshold
	cm2 := NewClusterManager(&ClusterConfig{
		NodeName: "n",
		Secret:   "s",
		MaxConns: 100,
	}, info, nil)
	info.ActiveConns.Store(50)
	if cm2.ShouldShed() {
		t.Error("ShouldShed() = true with 50/100 conns")
	}

	// At threshold
	info.ActiveConns.Store(100)
	if !cm2.ShouldShed() {
		t.Error("ShouldShed() = false with 100/100 conns")
	}

	// Above threshold
	info.ActiveConns.Store(150)
	if !cm2.ShouldShed() {
		t.Error("ShouldShed() = false with 150/100 conns")
	}
}

func TestClusterManagerLeastLoadedPeer(t *testing.T) {
	cm := NewClusterManager(&ClusterConfig{
		NodeName: "n",
		Secret:   "s",
		Peers: []PeerConfig{
			{Name: "a", Addr: "1.1.1.1:9090"},
			{Name: "b", Addr: "2.2.2.2:9090"},
		},
	}, nil, nil)

	// All unhealthy — no result
	if p := cm.LeastLoadedPeer(); p != nil {
		t.Errorf("LeastLoadedPeer() = %v, want nil (all unhealthy)", p)
	}

	// Make one healthy
	cm.mu.Lock()
	cm.peers["a"].Status = NodeHealthy
	cm.peers["a"].ActiveConns = 10
	cm.mu.Unlock()

	p := cm.LeastLoadedPeer()
	if p == nil || p.Name != "a" {
		t.Errorf("LeastLoadedPeer() = %v, want a", p)
	}

	// Make both healthy, b has fewer conns
	cm.mu.Lock()
	cm.peers["b"].Status = NodeHealthy
	cm.peers["b"].ActiveConns = 5
	cm.mu.Unlock()

	p = cm.LeastLoadedPeer()
	if p == nil || p.Name != "b" {
		t.Errorf("LeastLoadedPeer() = %v, want b (fewer conns)", p)
	}

	// Degraded peers are still eligible
	cm.mu.Lock()
	cm.peers["b"].Status = NodeDegraded
	cm.peers["b"].ActiveConns = 1
	cm.mu.Unlock()

	p = cm.LeastLoadedPeer()
	if p == nil || p.Name != "b" {
		t.Errorf("LeastLoadedPeer() = %v, want b (degraded but fewer conns)", p)
	}
}

func TestClusterManagerCheckPeer(t *testing.T) {
	// Create a mock peer server
	mockPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/status" {
			http.NotFound(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version":      "0.1.0",
			"active_conns": 25,
			"total_conns":  100,
			"bytes_sent":   5000,
			"bytes_recv":   6000,
		})
	}))
	defer mockPeer.Close()

	// Extract host:port from mock server URL
	peerAddr := mockPeer.Listener.Addr().String()

	cm := NewClusterManager(&ClusterConfig{
		NodeName: "local",
		Secret:   "test-secret",
		Peers:    []PeerConfig{{Name: "mock-peer", Addr: peerAddr}},
	}, nil, nil)

	ctx := context.Background()
	cm.checkPeer(ctx, "mock-peer")

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	peer := cm.peers["mock-peer"]
	if peer.Status != NodeHealthy {
		t.Errorf("peer status = %d, want NodeHealthy", peer.Status)
	}
	if peer.ActiveConns != 25 {
		t.Errorf("ActiveConns = %d, want 25", peer.ActiveConns)
	}
	if peer.TotalConns != 100 {
		t.Errorf("TotalConns = %d, want 100", peer.TotalConns)
	}
	if peer.Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", peer.Version, "0.1.0")
	}
	if peer.LastSeen.IsZero() {
		t.Error("LastSeen is zero, expected non-zero")
	}
}

func TestClusterManagerCheckPeerUnhealthy(t *testing.T) {
	// Create a peer that returns errors
	mockPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockPeer.Close()

	peerAddr := mockPeer.Listener.Addr().String()
	cm := NewClusterManager(&ClusterConfig{
		NodeName: "local",
		Secret:   "test-secret",
		Peers:    []PeerConfig{{Name: "bad-peer", Addr: peerAddr}},
	}, nil, nil)

	cm.checkPeer(context.Background(), "bad-peer")

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	peer := cm.peers["bad-peer"]
	if peer.Status != NodeUnhealthy {
		t.Errorf("peer status = %d, want NodeUnhealthy", peer.Status)
	}
}

func TestClusterManagerStartStop(t *testing.T) {
	cm := NewClusterManager(&ClusterConfig{
		NodeName: "n",
		Secret:   "s",
		Interval: "100ms",
	}, nil, nil)

	ctx := context.Background()
	cm.Start(ctx)

	// Let it run a couple ticks
	time.Sleep(250 * time.Millisecond)

	cm.Stop()
	// Should not panic on double stop
	cm.Stop()
}

func TestClusterHandlerHealth(t *testing.T) {
	cm := NewClusterManager(&ClusterConfig{
		NodeName: "test",
		Secret:   "s",
	}, nil, nil)

	handler := ClusterHandler(cm)
	req := httptest.NewRequest("GET", "/api/cluster/health", nil)
	req.Header.Set("Authorization", "Bearer s")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["node"] != "test" {
		t.Errorf("node = %q, want %q", resp["node"], "test")
	}
}

func TestClusterHandlerStatus(t *testing.T) {
	info := &ClusterNodeInfo{
		ActiveConns: &atomic.Int64{},
		TotalConns:  &atomic.Int64{},
		BytesSent:   &atomic.Int64{},
		BytesRecv:   &atomic.Int64{},
		Version:     "0.1.0",
	}
	info.ActiveConns.Store(10)

	cm := NewClusterManager(&ClusterConfig{
		NodeName: "test",
		Secret:   "my-secret",
		Peers:    []PeerConfig{{Name: "peer1", Addr: "1.1.1.1:9090"}},
	}, info, nil)

	handler := ClusterHandler(cm)

	// Without auth — should fail
	req := httptest.NewRequest("GET", "/api/cluster", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}

	// With auth
	req = httptest.NewRequest("GET", "/api/cluster", nil)
	req.Header.Set("Authorization", "Bearer my-secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp struct {
		Node  PeerState   `json:"node"`
		Peers []PeerState `json:"peers"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Node.Name != "test" {
		t.Errorf("node.name = %q, want %q", resp.Node.Name, "test")
	}
	if resp.Node.ActiveConns != 10 {
		t.Errorf("node.active_conns = %d, want 10", resp.Node.ActiveConns)
	}
	if len(resp.Peers) != 1 {
		t.Fatalf("peers len = %d, want 1", len(resp.Peers))
	}
	if resp.Peers[0].Name != "peer1" {
		t.Errorf("peers[0].name = %q, want %q", resp.Peers[0].Name, "peer1")
	}
}

func TestClusterManagerDefaultInterval(t *testing.T) {
	cm := NewClusterManager(&ClusterConfig{
		NodeName: "n",
		Secret:   "s",
	}, nil, nil)

	if cm.interval != 15*time.Second {
		t.Errorf("interval = %v, want 15s", cm.interval)
	}
}

func TestClusterManagerForwardStreamNoPeers(t *testing.T) {
	cm := NewClusterManager(&ClusterConfig{
		NodeName: "n",
		Secret:   "s",
	}, nil, nil)

	_, err := cm.ForwardStream("example.com:443")
	if err == nil {
		t.Error("ForwardStream() should fail with no peers")
	}
}
