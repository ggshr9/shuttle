package p2p

import (
	"log/slog"
	"net"
	"sync"
	"time"
)

// FallbackDecision represents the decision on how to send a packet.
type FallbackDecision int

const (
	DecisionDirect FallbackDecision = iota // Use P2P direct connection
	DecisionRelay                          // Use server relay
)

// FallbackConfig configures the fallback decision logic.
type FallbackConfig struct {
	HolePunchTimeout    time.Duration // Timeout for hole punching (default 10s)
	DirectRetryInterval time.Duration // Interval to retry direct connection (default 60s)
	FailureThreshold    int           // Consecutive failures before fallback (default 3)
	LossThreshold       float64       // Packet loss threshold for fallback (default 0.3)
	LatencyThreshold    time.Duration // Latency threshold for relay preference (default 500ms)
}

// DefaultFallbackConfig returns default fallback configuration.
func DefaultFallbackConfig() *FallbackConfig {
	return &FallbackConfig{
		HolePunchTimeout:    10 * time.Second,
		DirectRetryInterval: 60 * time.Second,
		FailureThreshold:    3,
		LossThreshold:       0.3,
		LatencyThreshold:    500 * time.Millisecond,
	}
}

// FallbackController manages fallback decisions for peers.
type FallbackController struct {
	mu      sync.RWMutex
	config  *FallbackConfig
	peers   map[[4]byte]*PeerFallbackState
	logger  *slog.Logger
}

// PeerFallbackState tracks fallback state for a single peer.
type PeerFallbackState struct {
	VIP              net.IP
	UsingRelay       bool
	ConsecutiveFails int
	LastDirectTry    time.Time
	LastSuccess      time.Time
	PacketsSent      uint64
	PacketsLost      uint64
	AvgLatency       time.Duration
	latencySamples   []time.Duration
}

// NewFallbackController creates a new fallback controller.
func NewFallbackController(config *FallbackConfig, logger *slog.Logger) *FallbackController {
	if config == nil {
		config = DefaultFallbackConfig()
	}
	return &FallbackController{
		config: config,
		peers:  make(map[[4]byte]*PeerFallbackState),
		logger: logger,
	}
}

// GetDecision returns the sending decision for a peer.
func (fc *FallbackController) GetDecision(vip net.IP) FallbackDecision {
	var key [4]byte
	copy(key[:], vip.To4())

	fc.mu.RLock()
	state, exists := fc.peers[key]
	fc.mu.RUnlock()

	if !exists {
		return DecisionDirect // Default to direct for new peers
	}

	if state.UsingRelay {
		// Check if we should retry direct
		if time.Since(state.LastDirectTry) > fc.config.DirectRetryInterval {
			return DecisionDirect
		}
		return DecisionRelay
	}

	return DecisionDirect
}

// RecordSuccess records a successful packet send/receive.
func (fc *FallbackController) RecordSuccess(vip net.IP, latency time.Duration) {
	var key [4]byte
	copy(key[:], vip.To4())

	fc.mu.Lock()
	defer fc.mu.Unlock()

	state := fc.getOrCreateState(key, vip)
	state.ConsecutiveFails = 0
	state.LastSuccess = time.Now()
	state.PacketsSent++

	// Update latency
	state.latencySamples = append(state.latencySamples, latency)
	if len(state.latencySamples) > 100 {
		state.latencySamples = state.latencySamples[1:]
	}
	state.AvgLatency = fc.calculateAvgLatency(state.latencySamples)

	// If we were using relay and direct succeeded, switch back
	if state.UsingRelay {
		fc.logger.Info("fallback: switching back to direct",
			"peer", vip,
			"latency", latency)
		state.UsingRelay = false
	}
}

// RecordFailure records a failed packet send.
func (fc *FallbackController) RecordFailure(vip net.IP) FallbackDecision {
	var key [4]byte
	copy(key[:], vip.To4())

	fc.mu.Lock()
	defer fc.mu.Unlock()

	state := fc.getOrCreateState(key, vip)
	state.ConsecutiveFails++
	state.PacketsLost++

	// Check if we should fallback to relay
	if state.ConsecutiveFails >= fc.config.FailureThreshold {
		if !state.UsingRelay {
			fc.logger.Info("fallback: switching to relay",
				"peer", vip,
				"consecutive_fails", state.ConsecutiveFails)
			state.UsingRelay = true
			state.LastDirectTry = time.Now()
		}
		return DecisionRelay
	}

	return DecisionDirect
}

// RecordHolePunchTimeout records a hole punch timeout.
func (fc *FallbackController) RecordHolePunchTimeout(vip net.IP) {
	var key [4]byte
	copy(key[:], vip.To4())

	fc.mu.Lock()
	defer fc.mu.Unlock()

	state := fc.getOrCreateState(key, vip)
	state.UsingRelay = true
	state.LastDirectTry = time.Now()

	fc.logger.Info("fallback: hole punch timeout, using relay", "peer", vip)
}

// ShouldRetryDirect checks if we should retry direct connection.
func (fc *FallbackController) ShouldRetryDirect(vip net.IP) bool {
	var key [4]byte
	copy(key[:], vip.To4())

	fc.mu.RLock()
	state, exists := fc.peers[key]
	fc.mu.RUnlock()

	if !exists {
		return true
	}

	if !state.UsingRelay {
		return false // Already using direct
	}

	return time.Since(state.LastDirectTry) > fc.config.DirectRetryInterval
}

// StartDirectRetry marks the start of a direct connection retry.
func (fc *FallbackController) StartDirectRetry(vip net.IP) {
	var key [4]byte
	copy(key[:], vip.To4())

	fc.mu.Lock()
	state := fc.getOrCreateState(key, vip)
	state.LastDirectTry = time.Now()
	fc.mu.Unlock()
}

// GetPeerStats returns statistics for a peer.
func (fc *FallbackController) GetPeerStats(vip net.IP) *PeerStats {
	var key [4]byte
	copy(key[:], vip.To4())

	fc.mu.RLock()
	defer fc.mu.RUnlock()

	state, exists := fc.peers[key]
	if !exists {
		return nil
	}

	var lossRate float64
	if state.PacketsSent > 0 {
		lossRate = float64(state.PacketsLost) / float64(state.PacketsSent)
	}

	return &PeerStats{
		VIP:              state.VIP,
		UsingRelay:       state.UsingRelay,
		ConsecutiveFails: state.ConsecutiveFails,
		PacketsSent:      state.PacketsSent,
		PacketsLost:      state.PacketsLost,
		LossRate:         lossRate,
		AvgLatency:       state.AvgLatency,
		LastSuccess:      state.LastSuccess,
		LastDirectTry:    state.LastDirectTry,
	}
}

// PeerStats contains statistics for a peer.
type PeerStats struct {
	VIP              net.IP
	UsingRelay       bool
	ConsecutiveFails int
	PacketsSent      uint64
	PacketsLost      uint64
	LossRate         float64
	AvgLatency       time.Duration
	LastSuccess      time.Time
	LastDirectTry    time.Time
}

// GetAllPeers returns all peer VIPs being tracked.
func (fc *FallbackController) GetAllPeers() []net.IP {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	peers := make([]net.IP, 0, len(fc.peers))
	for _, state := range fc.peers {
		peers = append(peers, state.VIP)
	}
	return peers
}

// GetRelayPeers returns VIPs of peers using relay.
func (fc *FallbackController) GetRelayPeers() []net.IP {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	peers := make([]net.IP, 0)
	for _, state := range fc.peers {
		if state.UsingRelay {
			peers = append(peers, state.VIP)
		}
	}
	return peers
}

// GetDirectPeers returns VIPs of peers using direct connection.
func (fc *FallbackController) GetDirectPeers() []net.IP {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	peers := make([]net.IP, 0)
	for _, state := range fc.peers {
		if !state.UsingRelay {
			peers = append(peers, state.VIP)
		}
	}
	return peers
}

// Reset resets the state for a peer.
func (fc *FallbackController) Reset(vip net.IP) {
	var key [4]byte
	copy(key[:], vip.To4())

	fc.mu.Lock()
	delete(fc.peers, key)
	fc.mu.Unlock()
}

// ResetAll resets all peer states.
func (fc *FallbackController) ResetAll() {
	fc.mu.Lock()
	fc.peers = make(map[[4]byte]*PeerFallbackState)
	fc.mu.Unlock()
}

// getOrCreateState gets or creates state for a peer.
func (fc *FallbackController) getOrCreateState(key [4]byte, vip net.IP) *PeerFallbackState {
	state, exists := fc.peers[key]
	if !exists {
		state = &PeerFallbackState{
			VIP:            vip,
			latencySamples: make([]time.Duration, 0, 100),
		}
		fc.peers[key] = state
	}
	return state
}

// calculateAvgLatency calculates average latency from samples.
func (fc *FallbackController) calculateAvgLatency(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}

	var total time.Duration
	for _, s := range samples {
		total += s
	}
	return total / time.Duration(len(samples))
}

// CheckLossThreshold checks if packet loss exceeds threshold.
func (fc *FallbackController) CheckLossThreshold(vip net.IP) bool {
	var key [4]byte
	copy(key[:], vip.To4())

	fc.mu.RLock()
	state, exists := fc.peers[key]
	fc.mu.RUnlock()

	if !exists || state.PacketsSent == 0 {
		return false
	}

	lossRate := float64(state.PacketsLost) / float64(state.PacketsSent)
	return lossRate > fc.config.LossThreshold
}

// CheckLatencyThreshold checks if latency exceeds threshold.
func (fc *FallbackController) CheckLatencyThreshold(vip net.IP) bool {
	var key [4]byte
	copy(key[:], vip.To4())

	fc.mu.RLock()
	state, exists := fc.peers[key]
	fc.mu.RUnlock()

	if !exists {
		return false
	}

	return state.AvgLatency > fc.config.LatencyThreshold
}

// AutoFallback performs automatic fallback decision based on metrics.
func (fc *FallbackController) AutoFallback(vip net.IP) FallbackDecision {
	// Check consecutive failures
	decision := fc.GetDecision(vip)
	if decision == DecisionRelay {
		return DecisionRelay
	}

	// Check loss rate
	if fc.CheckLossThreshold(vip) {
		fc.mu.Lock()
		var key [4]byte
		copy(key[:], vip.To4())
		if state, ok := fc.peers[key]; ok {
			state.UsingRelay = true
			state.LastDirectTry = time.Now()
		}
		fc.mu.Unlock()

		fc.logger.Info("fallback: high loss rate, switching to relay",
			"peer", vip)
		return DecisionRelay
	}

	return DecisionDirect
}
