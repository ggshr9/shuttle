package p2p

import (
	"fmt"
	"net"
	"time"

	"github.com/ggshr9/shuttle/mesh/signal"
)

// TriggerICERestart triggers an ICE restart for a specific peer.
func (m *Manager) TriggerICERestart(peerVIP net.IP, reason byte) error {
	if !m.iceRestartEnabled {
		return fmt.Errorf("p2p: ICE restart is disabled")
	}

	key := vipKey(peerVIP)

	m.mu.Lock()
	peer, exists := m.peers[key]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("p2p: peer not found: %s", peerVIP)
	}

	// Check cooldown
	if time.Since(peer.LastICERestart) < m.iceRestartCooldown {
		m.mu.Unlock()
		return fmt.Errorf("p2p: ICE restart cooldown not elapsed")
	}

	// Update restart tracking
	peer.LastICERestart = time.Now()
	peer.RestartCount++
	peer.ICEGeneration++
	generation := peer.ICEGeneration

	// Close existing connection
	if peer.P2PConn != nil {
		peer.P2PConn.Close()
		peer.P2PConn = nil
	}
	peer.State = StateDisconnected
	peer.UseRelay = false
	m.mu.Unlock()

	m.logger.Info("p2p: initiating ICE restart",
		"peer", peerVIP,
		"reason", reason,
		"generation", generation)

	// Create new ICE credentials
	creds := NewICECredentials()
	creds.Generation = generation

	// Send ICE restart signal
	if m.signalClient != nil {
		restartInfo := &signal.ICERestartInfo{
			Reason:           reason,
			Generation:       uint16(generation), //nolint:gosec // G115: generation is a small counter, fits uint16
			UsernameFragment: creds.UsernameFragment,
			Password:         creds.Password,
		}

		if err := m.signalClient.SendICERestart(peerVIP, restartInfo); err != nil {
			m.logger.Debug("p2p: failed to send ICE restart signal",
				"peer", peerVIP,
				"err", err)
		}
	}

	// Re-gather candidates and reconnect
	m.scheduleReconnect(peerVIP)

	return nil
}

// handleICERestart handles incoming ICE restart messages.
func (m *Manager) handleICERestart(msg *signal.Message) {
	restartInfo, err := signal.DecodeICERestartInfo(msg.Payload)
	if err != nil {
		m.logger.Debug("p2p: decode ICE restart info failed", "err", err)
		return
	}

	m.logger.Info("p2p: received ICE restart request",
		"peer", msg.SrcVIP,
		"reason", restartInfo.Reason,
		"generation", restartInfo.Generation)

	m.mu.Lock()
	peer := m.getOrCreatePeer(msg.SrcVIP)

	// Update ICE credentials from remote
	peer.ICEGeneration = int(restartInfo.Generation)

	// Mark connection as disconnected to trigger reconnection
	oldState := peer.State
	if peer.P2PConn != nil {
		peer.P2PConn.Close()
		peer.P2PConn = nil
	}
	peer.State = StateDisconnected
	peer.UseRelay = false
	m.mu.Unlock()

	// If we were connected, initiate reconnection
	if oldState == StateConnected {
		m.scheduleReconnect(msg.SrcVIP)
	}
}

// OnNetworkChange should be called when network conditions change.
// This triggers ICE restart for all connected peers.
func (m *Manager) OnNetworkChange() {
	if !m.iceRestartEnabled {
		return
	}

	m.logger.Info("p2p: network change detected, checking connections")

	m.mu.RLock()
	var connectedPeers []net.IP
	for _, peer := range m.peers {
		if peer.State == StateConnected {
			connectedPeers = append(connectedPeers, peer.VIP)
		}
	}
	m.mu.RUnlock()

	// Trigger ICE restart for all connected peers
	for _, vip := range connectedPeers {
		go func(v net.IP) { _ = m.TriggerICERestart(v, signal.ICERestartReasonNetworkChange) }(vip)
	}
}

// qualityMonitorLoop monitors connection quality and triggers ICE restart if needed.
func (m *Manager) qualityMonitorLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkConnectionQuality()
		}
	}
}

// checkConnectionQuality checks all peer connections and triggers restart if needed.
func (m *Manager) checkConnectionQuality() {
	m.mu.RLock()
	var peersToRestart []net.IP
	for _, peer := range m.peers {
		if peer.State == StateConnected && peer.QualityMonitor != nil {
			metrics := peer.QualityMonitor.GetMetrics()
			if float64(metrics.Score) < m.iceQualityThreshold {
				peersToRestart = append(peersToRestart, peer.VIP)
			}
		}
	}
	m.mu.RUnlock()

	// Trigger restarts outside the lock
	for _, vip := range peersToRestart {
		m.logger.Info("p2p: connection quality degraded, triggering ICE restart",
			"peer", vip)
		go func(v net.IP) { _ = m.TriggerICERestart(v, signal.ICERestartReasonQualityDegraded) }(vip)
	}
}

// RecordRTT records an RTT sample for a peer's quality monitor.
func (m *Manager) RecordRTT(peerVIP net.IP, rtt time.Duration) {
	key := vipKey(peerVIP)

	m.mu.Lock()
	defer m.mu.Unlock()

	if peer, ok := m.peers[key]; ok {
		if peer.QualityMonitor == nil {
			peer.QualityMonitor = NewConnectionQuality()
		}
		peer.QualityMonitor.RecordRTT(rtt)
	}
}

// RecordPacketLoss records packet loss for a peer's quality monitor.
func (m *Manager) RecordPacketLoss(peerVIP net.IP) {
	key := vipKey(peerVIP)

	m.mu.Lock()
	defer m.mu.Unlock()

	if peer, ok := m.peers[key]; ok {
		if peer.QualityMonitor == nil {
			peer.QualityMonitor = NewConnectionQuality()
		}
		peer.QualityMonitor.RecordPacketLost()
	}
}

// GetPeerQuality returns quality metrics for a peer.
func (m *Manager) GetPeerQuality(peerVIP net.IP) *QualityMetrics {
	key := vipKey(peerVIP)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if peer, ok := m.peers[key]; ok && peer.QualityMonitor != nil {
		metrics := peer.QualityMonitor.GetMetrics()
		return &metrics
	}
	return nil
}

// GetICERestartStats returns ICE restart statistics for a peer.
func (m *Manager) GetICERestartStats(peerVIP net.IP) *ICERestartStats {
	key := vipKey(peerVIP)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if peer, ok := m.peers[key]; ok {
		return &ICERestartStats{
			Generation:     peer.ICEGeneration,
			RestartCount:   peer.RestartCount,
			LastRestart:    peer.LastICERestart,
			CooldownActive: time.Since(peer.LastICERestart) < m.iceRestartCooldown,
		}
	}
	return nil
}

// handleTrickleCandidate handles incoming trickle candidate messages.
func (m *Manager) handleTrickleCandidate(msg *signal.Message) {
	info, err := signal.DecodeTrickleCandidate(msg.Payload)
	if err != nil {
		m.logger.Debug("p2p: decode trickle candidate failed", "err", err)
		return
	}

	m.mu.Lock()
	peer := m.getOrCreatePeer(msg.SrcVIP)

	// Convert to internal candidate type
	candidate := &Candidate{
		Type:     CandidateType(info.Candidate.Type),
		Addr:     &net.UDPAddr{IP: info.Candidate.IP, Port: int(info.Candidate.Port)},
		Priority: info.Candidate.Priority,
	}
	if info.Candidate.RelatedIP != nil {
		candidate.RelatedIP = info.Candidate.RelatedIP
		candidate.RelatedPort = int(info.Candidate.RelatedPort)
	}

	// Check for duplicates
	isDuplicate := false
	for _, c := range peer.Candidates {
		if c.Addr.String() == candidate.Addr.String() {
			isDuplicate = true
			break
		}
	}

	if !isDuplicate {
		peer.Candidates = append(peer.Candidates, candidate)
	}
	m.mu.Unlock()

	if !isDuplicate {
		m.logger.Debug("p2p: received trickle candidate",
			"peer", msg.SrcVIP,
			"type", candidate.Type,
			"addr", candidate.Addr,
			"generation", info.Generation)
	}
}

// handleEndOfCandidates handles end-of-candidates signals.
func (m *Manager) handleEndOfCandidates(msg *signal.Message) {
	info, err := signal.DecodeEndOfCandidates(msg.Payload)
	if err != nil {
		m.logger.Debug("p2p: decode end-of-candidates failed", "err", err)
		return
	}

	key := vipKey(msg.SrcVIP)

	m.mu.Lock()
	if peer, exists := m.peers[key]; exists {
		peer.ICEGeneration = int(info.Generation)
	}
	m.mu.Unlock()

	m.logger.Debug("p2p: received end-of-candidates",
		"peer", msg.SrcVIP,
		"generation", info.Generation,
		"total_candidates", info.TotalCandidates)
}

// SendTrickleCandidate sends a trickle candidate to a peer.
func (m *Manager) SendTrickleCandidate(dstVIP net.IP, candidate *Candidate) error {
	if m.signalClient == nil {
		return fmt.Errorf("p2p: no signal client")
	}

	info := &signal.TrickleCandidateInfo{
		Candidate: &signal.CandidateInfo{
			Type:     byte(candidate.Type),
			IP:       candidate.Addr.IP,
			Port:     uint16(candidate.Addr.Port), //nolint:gosec // G115: port range 0-65535, fits uint16
			Priority: candidate.Priority,
		},
		Generation: uint16(m.iceGeneration), //nolint:gosec // G115: generation is a small counter, fits uint16
		MLineIndex: 0,
	}

	if candidate.RelatedIP != nil {
		info.Candidate.RelatedIP = candidate.RelatedIP
		info.Candidate.RelatedPort = uint16(candidate.RelatedPort) //nolint:gosec // G115: port range 0-65535, fits uint16
	}

	return m.signalClient.SendTrickleCandidate(dstVIP, info)
}

// SendEndOfCandidates sends an end-of-candidates signal to a peer.
func (m *Manager) SendEndOfCandidates(dstVIP net.IP) error {
	if m.signalClient == nil {
		return fmt.Errorf("p2p: no signal client")
	}

	info := &signal.EndOfCandidatesInfo{
		Generation:      uint16(m.iceGeneration), //nolint:gosec // G115: generation is a small counter, fits uint16
		TotalCandidates: uint8(len(m.candidates)), //nolint:gosec // G115: candidate count never exceeds 255
	}

	return m.signalClient.SendEndOfCandidates(dstVIP, info)
}

// IsTrickleEnabled returns whether Trickle ICE is enabled.
func (m *Manager) IsTrickleEnabled() bool {
	return m.trickleEnabled
}
