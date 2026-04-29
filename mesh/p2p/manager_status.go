package p2p

import (
	"net"
	"time"
)

// PeerInfo contains information about a peer including quality metrics.
type PeerInfo struct {
	VirtualIP string          `json:"virtual_ip"`
	State     string          `json:"state"`
	Method    string          `json:"method,omitempty"`
	Quality   *QualityMetrics `json:"quality,omitempty"`
}

// PortMappingInfo contains port mapping status information.
type PortMappingInfo struct {
	Enabled      bool
	Protocol     string // "upnp" or "nat-pmp"
	ExternalIP   net.IP
	ExternalPort int
	LocalPort    int
}

// NATStatus contains comprehensive NAT traversal status information.
type NATStatus struct {
	// Local endpoint
	LocalAddr *net.UDPAddr

	// External endpoint (best known)
	ExternalAddr *net.UDPAddr

	// Port mapping status
	PortMappingEnabled  bool
	PortMappingProtocol string // "upnp", "nat-pmp", or ""

	// Candidates gathered
	CandidateCount     int
	HasHostCandidate   bool
	HasSTUNCandidate   bool
	HasUPnPCandidate   bool

	// Connection stats
	TotalPeers   int
	DirectPeers  int
	RelayPeers   int

	// Path cache stats
	CachedPaths int
}

// GetPeerState returns the state of a peer connection.
func (m *Manager) GetPeerState(vip net.IP) ConnectionState {
	key := vipKey(vip)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if peer, ok := m.peers[key]; ok {
		return peer.State
	}
	return StateDisconnected
}

// GetStats returns statistics about P2P connections.
func (m *Manager) GetStats() *ManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &ManagerStats{
		TotalPeers: len(m.peers),
	}

	for _, peer := range m.peers {
		switch peer.State {
		case StateConnected:
			if peer.UseRelay {
				stats.RelayPeers++
			} else {
				stats.DirectPeers++
			}
		case StateConnecting:
			stats.ConnectingPeers++
		case StateFailed:
			stats.FailedPeers++
		}
	}

	return stats
}

// ListPeers returns information about all known peers.
func (m *Manager) ListPeers() []PeerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	peers := make([]PeerInfo, 0, len(m.peers))
	for _, peer := range m.peers {
		method := "direct"
		if peer.UseRelay {
			method = "relay"
		} else if peer.P2PConn != nil {
			method = "p2p"
		}
		info := PeerInfo{
			VirtualIP: peer.VIP.String(),
			State:     peer.State.String(),
			Method:    method,
		}
		if peer.QualityMonitor != nil {
			metrics := peer.QualityMonitor.GetMetrics()
			info.Quality = &metrics
		}
		peers = append(peers, info)
	}
	return peers
}

// LocalCandidates returns a snapshot of local ICE candidates. Safe to
// range over even if a concurrent ICE restart re-gathers.
func (m *Manager) LocalCandidates() []*Candidate {
	return m.localCandidatesSnapshot()
}

// SetDataHandler sets the handler for incoming P2P data.
func (m *Manager) SetDataHandler(handler func(srcVIP net.IP, data []byte)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dataHandler = handler
}

// GetSpoofInfo returns information about port spoofing configuration.
// Returns nil if spoofing is not enabled.
func (m *Manager) GetSpoofInfo() *SpoofInfo {
	return m.spoofInfo
}

// IsSpoofEnabled returns whether port spoofing is enabled.
func (m *Manager) IsSpoofEnabled() bool {
	return m.spoofInfo != nil && m.spoofConfig != nil && m.spoofConfig.Mode != SpoofNone
}

// GetPortMappingInfo returns port mapping information (UPnP or NAT-PMP).
// Returns nil if port mapping is not active.
func (m *Manager) GetPortMappingInfo() *PortMappingInfo {
	if !m.upnpEnabled || m.portMapper == nil {
		return nil
	}

	extAddr := m.portMapper.GetExternalAddr()
	if extAddr == nil {
		return nil
	}

	return &PortMappingInfo{
		Enabled:      true,
		Protocol:     m.portMapper.Protocol(),
		ExternalIP:   extAddr.IP,
		ExternalPort: extAddr.Port,
		LocalPort:    m.udpConn.LocalAddr().(*net.UDPAddr).Port,
	}
}
// IsUPnPEnabled returns whether UPnP port mapping is active.
func (m *Manager) IsUPnPEnabled() bool {
	return m.upnpEnabled
}

// GetExternalEndpoint returns the best known external endpoint.
// Prioritizes UPnP > STUN > local address.
func (m *Manager) GetExternalEndpoint() *net.UDPAddr {
	// Try UPnP first
	if m.upnpEnabled && m.portMapper != nil {
		if addr := m.portMapper.GetExternalAddr(); addr != nil {
			return addr
		}
	}

	// Look for server-reflexive or UPnP candidate
	for _, c := range m.localCandidatesSnapshot() {
		if c.Type == CandidateUPnP || c.Type == CandidateServerReflexive {
			return c.Addr
		}
	}

	// Fall back to local address
	return m.udpConn.LocalAddr().(*net.UDPAddr)
}

// GetNATStatus returns comprehensive NAT traversal status.
func (m *Manager) GetNATStatus() *NATStatus {
	status := &NATStatus{
		LocalAddr:    m.udpConn.LocalAddr().(*net.UDPAddr),
		ExternalAddr: m.GetExternalEndpoint(),
	}

	// Port mapping info
	if m.upnpEnabled && m.portMapper != nil {
		status.PortMappingEnabled = true
		status.PortMappingProtocol = m.portMapper.Protocol()
	}

	// Candidates
	cands := m.localCandidatesSnapshot()
	status.CandidateCount = len(cands)
	for _, c := range cands {
		switch c.Type {
		case CandidateHost:
			status.HasHostCandidate = true
		case CandidateServerReflexive:
			status.HasSTUNCandidate = true
		case CandidateUPnP:
			status.HasUPnPCandidate = true
		}
	}

	// Connection stats
	stats := m.GetStats()
	status.TotalPeers = stats.TotalPeers
	status.DirectPeers = stats.DirectPeers
	status.RelayPeers = stats.RelayPeers

	// Path cache
	if m.pathCache != nil {
		cacheStats := m.pathCache.Stats()
		status.CachedPaths = cacheStats.TotalEntries
	}

	return status
}

// GetPathCacheStats returns path cache statistics.
func (m *Manager) GetPathCacheStats() PathCacheStats {
	if m.pathCache == nil {
		return PathCacheStats{}
	}
	return m.pathCache.Stats()
}

// GetCachedPath returns the cached path info for a peer.
func (m *Manager) GetCachedPath(peerVIP net.IP) *PathEntry {
	if m.pathCache == nil {
		return nil
	}
	return m.pathCache.Get(peerVIP)
}

// ClearPathCache clears all cached paths.
func (m *Manager) ClearPathCache() {
	if m.pathCache != nil {
		m.pathCache.Clear()
	}
}

// upnpRefreshLoop periodically refreshes UPnP port mappings.
func (m *Manager) upnpRefreshLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if m.portMapper != nil {
				if err := m.portMapper.Refresh(); err != nil {
					m.logger.Warn("p2p: failed to refresh UPnP mapping", "err", err)
				}
			}
		}
	}
}

// SetMDNS attaches an MDNSService for LAN peer discovery. mDNS-advertised
// candidates are gated behind Verified=true (set by MarkVerified after the
// X25519 handshake) to prevent VIP spoofing attacks on the local network.
func (m *Manager) SetMDNS(svc *MDNSService) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mdns = svc
}

// pathCacheCleanupLoop periodically cleans up expired path cache entries.
func (m *Manager) pathCacheCleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if m.pathCache != nil {
				m.pathCache.Cleanup()
			}
		}
	}
}
