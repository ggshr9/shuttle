package p2p

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/curve25519"

	"github.com/shuttle-proxy/shuttle/mesh/signal"
)

// ConnectionState represents the state of a peer connection.
type ConnectionState int

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
	StateFailed
)

func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// PeerConnection represents a connection to a single peer.
type PeerConnection struct {
	VIP           net.IP
	State         ConnectionState
	P2PConn       *P2PConn
	Candidates    []*Candidate
	LastAttempt   time.Time
	FailCount     int
	UseRelay      bool // Whether to use relay instead of direct

	// ICE restart tracking
	ICEGeneration   int       // Current ICE credential generation
	LastICERestart  time.Time // When the last ICE restart occurred
	RestartCount    int       // Number of ICE restarts for this peer
	QualityMonitor  *ConnectionQuality // Monitor connection quality
}

// Manager manages P2P connections to peers.
type Manager struct {
	mu sync.RWMutex

	localVIP    net.IP
	localPriv   [32]byte
	localPub    [32]byte
	udpConn     *net.UDPConn
	signalClient *signal.Client
	relayFunc   func([]byte) error // Fallback relay function
	logger      *slog.Logger

	peers       map[[4]byte]*PeerConnection
	candidates  []*Candidate // Local candidates

	stunServers       []string
	holePunchTimeout  time.Duration
	directRetryInterval time.Duration
	keepAliveInterval time.Duration

	// Port spoofing
	spoofConfig *SpoofConfig
	spoofInfo   *SpoofInfo

	// UPnP/NAT-PMP port mapping
	portMapper  *PortMapper
	upnpEnabled bool
	upnpPort    int // The externally mapped port

	// Connection optimization
	pathCache *PathCache // Remember successful paths

	// ICE restart configuration
	iceRestartCooldown    time.Duration
	iceQualityThreshold   float64 // Restart if quality drops below this (0-100)
	iceRestartEnabled     bool

	// Trickle ICE configuration
	trickleEnabled    bool
	trickleGatherer   *TrickleICEGatherer
	iceGeneration     int // Current ICE credential generation

	// Goroutine lifecycle
	wg sync.WaitGroup

	ctx    context.Context
	cancel context.CancelFunc
}

// Config holds configuration for the P2P manager.
type Config struct {
	LocalVIP            net.IP
	LocalPrivateKey     [32]byte
	LocalPublicKey      [32]byte
	STUNServers         []string
	HolePunchTimeout    time.Duration
	DirectRetryInterval time.Duration
	KeepAliveInterval   time.Duration
	RelayFunc           func([]byte) error

	// Port spoofing configuration
	// Use SpoofDNS (port 53) to bypass firewalls that only allow DNS traffic
	SpoofConfig *SpoofConfig

	// UPnP/NAT-PMP configuration
	// By default, the manager will automatically try UPnP/NAT-PMP for best NAT traversal
	EnableUPnP     bool // Explicitly enable UPnP (auto-enabled by default)
	DisableUPnP    bool // Explicitly disable UPnP/NAT-PMP auto-detection
	PreferredPort  int  // Preferred external port (0 = same as local)
	SamePortPunch  bool // Try to use same port on both sides for hole punching

	// ICE restart configuration
	EnableICERestart    bool          // Enable ICE restart support (default: true)
	ICERestartCooldown  time.Duration // Minimum time between restarts (default: 10s)
	ICEQualityThreshold float64       // Quality score below which to trigger restart (default: 30.0)

	// Trickle ICE configuration (RFC 8838)
	EnableTrickleICE bool // Enable Trickle ICE for faster connection establishment
}

// NewManager creates a new P2P connection manager.
func NewManager(cfg *Config, signalClient *signal.Client, logger *slog.Logger) (*Manager, error) {
	// Create UDP socket with optional port spoofing
	var udpConn *net.UDPConn
	var err error
	var spoofInfo *SpoofInfo

	if cfg.SpoofConfig != nil && cfg.SpoofConfig.Mode != SpoofNone {
		// Validate spoof config
		if err := ValidateSpoofConfig(cfg.SpoofConfig); err != nil {
			logger.Warn("p2p: spoof config validation failed, falling back to random port",
				"mode", cfg.SpoofConfig.Mode,
				"err", err)
			cfg.SpoofConfig = nil
		}
	}

	if cfg.SpoofConfig != nil && cfg.SpoofConfig.Mode != SpoofNone {
		udpConn, err = CreateSpoofedConn(cfg.SpoofConfig)
		if err != nil {
			logger.Warn("p2p: failed to create spoofed connection, falling back to random port",
				"mode", cfg.SpoofConfig.Mode,
				"err", err)
			// Fall back to random port
			udpConn, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
			if err != nil {
				return nil, fmt.Errorf("p2p: listen: %w", err)
			}
		} else {
			spoofInfo = GetSpoofInfo(udpConn, cfg.SpoofConfig.Mode)
			logger.Info("p2p: using spoofed port",
				"mode", cfg.SpoofConfig.Mode,
				"port", spoofInfo.LocalPort)
		}
	} else {
		udpConn, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
		if err != nil {
			return nil, fmt.Errorf("p2p: listen: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		localVIP:            cfg.LocalVIP,
		localPriv:           cfg.LocalPrivateKey,
		localPub:            cfg.LocalPublicKey,
		udpConn:             udpConn,
		signalClient:        signalClient,
		relayFunc:           cfg.RelayFunc,
		logger:              logger,
		peers:               make(map[[4]byte]*PeerConnection),
		stunServers:         cfg.STUNServers,
		holePunchTimeout:    cfg.HolePunchTimeout,
		directRetryInterval: cfg.DirectRetryInterval,
		keepAliveInterval:   cfg.KeepAliveInterval,
		spoofConfig:         cfg.SpoofConfig,
		spoofInfo:           spoofInfo,
		pathCache:           NewPathCache(24 * time.Hour),
		iceRestartEnabled:   cfg.EnableICERestart,
		iceRestartCooldown:  cfg.ICERestartCooldown,
		iceQualityThreshold: cfg.ICEQualityThreshold,
		trickleEnabled:      cfg.EnableTrickleICE,
		ctx:                 ctx,
		cancel:              cancel,
	}

	// Set defaults
	if m.holePunchTimeout == 0 {
		m.holePunchTimeout = 10 * time.Second
	}
	if m.directRetryInterval == 0 {
		m.directRetryInterval = 60 * time.Second
	}
	if m.keepAliveInterval == 0 {
		m.keepAliveInterval = 30 * time.Second
	}
	// Only fill defaults when stunServers is nil (not explicitly set).
	// An explicit empty slice []string{} means "no STUN servers".
	if m.stunServers == nil {
		m.stunServers = DefaultSTUNServers()
	}
	if m.iceRestartCooldown == 0 {
		m.iceRestartCooldown = 10 * time.Second
	}
	if m.iceQualityThreshold == 0 {
		m.iceQualityThreshold = 30.0
	}

	// Auto-try UPnP/NAT-PMP unless explicitly disabled
	// This provides the best out-of-box experience for NAT traversal
	if !cfg.DisableUPnP {
		m.portMapper = NewPortMapper(logger)
		localPort := udpConn.LocalAddr().(*net.UDPAddr).Port
		preferredPort := cfg.PreferredPort
		if preferredPort == 0 {
			preferredPort = localPort
		}

		mappedPort, err := m.portMapper.MapPort(ctx, localPort, preferredPort)
		if err != nil {
			// This is expected on networks without UPnP/NAT-PMP support
			// Fall back to STUN-based NAT traversal silently
			logger.Debug("p2p: port mapping unavailable (UPnP/NAT-PMP not supported)", "err", err)
		} else {
			m.upnpEnabled = true
			m.upnpPort = mappedPort
			protocol := m.portMapper.Protocol()
			logger.Info("p2p: port mapping created",
				"protocol", protocol,
				"local_port", localPort,
				"external_port", mappedPort,
				"external_ip", m.portMapper.GetExternalAddr().IP)
		}
	}

	return m, nil
}

// Start starts the manager's background routines.
func (m *Manager) Start() error {
	// Gather local candidates
	gatherer := NewICEGatherer(m.stunServers, 5*time.Second)
	result, err := gatherer.GatherWithConnection(m.udpConn)
	if err != nil {
		m.logger.Warn("p2p: gather candidates failed", "err", err)
	} else {
		m.candidates = result.Candidates
	}

	// Add UPnP candidate if available (highest priority for direct connection)
	if m.upnpEnabled && m.portMapper != nil {
		upnpAddr := m.portMapper.GetExternalAddr()
		if upnpAddr != nil {
			upnpCandidate := NewCandidate(CandidateUPnP, upnpAddr)
			upnpCandidate.Base = m.udpConn.LocalAddr().(*net.UDPAddr)
			// Insert at the beginning (highest priority)
			m.candidates = append([]*Candidate{upnpCandidate}, m.candidates...)
			m.logger.Info("p2p: added UPnP candidate", "addr", upnpAddr)
		}
	}

	m.logger.Info("p2p: gathered candidates", "count", len(m.candidates))
	for _, c := range m.candidates {
		m.logger.Debug("p2p: candidate", "type", c.Type, "addr", c.Addr)
	}

	// Set up signal handlers
	if m.signalClient != nil {
		m.signalClient.OnMessage(signal.SignalCandidate, m.handleCandidates)
		m.signalClient.OnMessage(signal.SignalConnect, m.handleConnect)
		m.signalClient.OnMessage(signal.SignalDisconnect, m.handleDisconnect)
		m.signalClient.OnMessage(signal.SignalICERestart, m.handleICERestart)
		m.signalClient.OnMessage(signal.SignalTrickleCandidate, m.handleTrickleCandidate)
		m.signalClient.OnMessage(signal.SignalEndOfCandidates, m.handleEndOfCandidates)
	}

	// Start receive loop
	m.wg.Add(1)
	go func() { defer m.wg.Done(); m.receiveLoop() }()

	// Start keep-alive loop
	m.wg.Add(1)
	go func() { defer m.wg.Done(); m.keepAliveLoop() }()

	// Start retry loop
	m.wg.Add(1)
	go func() { defer m.wg.Done(); m.retryLoop() }()

	// Start UPnP refresh loop if enabled
	if m.upnpEnabled && m.portMapper != nil {
		m.wg.Add(1)
		go func() { defer m.wg.Done(); m.upnpRefreshLoop() }()
	}

	// Start path cache cleanup loop
	m.wg.Add(1)
	go func() { defer m.wg.Done(); m.pathCacheCleanupLoop() }()

	// Start connection quality monitoring loop
	if m.iceRestartEnabled {
		m.wg.Add(1)
		go func() { defer m.wg.Done(); m.qualityMonitorLoop() }()
	}

	return nil
}

// Stop stops the manager and waits for all goroutines to exit.
func (m *Manager) Stop() error {
	m.cancel()

	// Wait for all background goroutines to exit before closing resources.
	m.wg.Wait()

	m.mu.Lock()
	for _, peer := range m.peers {
		if peer.P2PConn != nil {
			peer.P2PConn.Close()
		}
	}
	m.peers = make(map[[4]byte]*PeerConnection)
	m.mu.Unlock()

	// Clean up UPnP port mapping
	if m.portMapper != nil {
		if err := m.portMapper.Close(); err != nil {
			m.logger.Warn("p2p: failed to remove UPnP mapping", "err", err)
		}
	}

	return m.udpConn.Close()
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
		go m.TriggerICERestart(vip, signal.ICERestartReasonQualityDegraded)
	}
}

// handleICERestart handles incoming ICE restart messages.
func (m *Manager) handleICERestart(msg *signal.Message) {
	restartInfo, err := signal.DecodeICERestartInfo(msg.Payload)
	if err != nil {
		m.logger.Debug("p2p: decode ICE restart info failed", "err", err)
		return
	}

	var key [4]byte
	copy(key[:], msg.SrcVIP.To4())

	m.logger.Info("p2p: received ICE restart request",
		"peer", msg.SrcVIP,
		"reason", restartInfo.Reason,
		"generation", restartInfo.Generation)

	m.mu.Lock()
	peer, exists := m.peers[key]
	if !exists {
		peer = &PeerConnection{
			VIP:   msg.SrcVIP,
			State: StateDisconnected,
		}
		m.peers[key] = peer
	}

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
		go func() {
			// Wait a bit to allow the remote side to be ready
			time.Sleep(500 * time.Millisecond)
			if err := m.Connect(m.ctx, msg.SrcVIP); err != nil {
				m.logger.Debug("p2p: reconnect after ICE restart failed",
					"peer", msg.SrcVIP,
					"err", err)
			}
		}()
	}
}

// TriggerICERestart triggers an ICE restart for a specific peer.
func (m *Manager) TriggerICERestart(peerVIP net.IP, reason byte) error {
	if !m.iceRestartEnabled {
		return fmt.Errorf("p2p: ICE restart is disabled")
	}

	var key [4]byte
	copy(key[:], peerVIP.To4())

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
			Generation:       uint16(generation),
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
	go func() {
		// Wait a bit to allow the signal to be delivered
		time.Sleep(500 * time.Millisecond)
		if err := m.Connect(m.ctx, peerVIP); err != nil {
			m.logger.Debug("p2p: reconnect after ICE restart failed",
				"peer", peerVIP,
				"err", err)
		}
	}()

	return nil
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
		go m.TriggerICERestart(vip, signal.ICERestartReasonNetworkChange)
	}
}

// RecordRTT records an RTT sample for a peer's quality monitor.
func (m *Manager) RecordRTT(peerVIP net.IP, rtt time.Duration) {
	var key [4]byte
	copy(key[:], peerVIP.To4())

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
	var key [4]byte
	copy(key[:], peerVIP.To4())

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
	var key [4]byte
	copy(key[:], peerVIP.To4())

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
	var key [4]byte
	copy(key[:], peerVIP.To4())

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

// ICERestartStats contains ICE restart statistics.
type ICERestartStats struct {
	Generation     int
	RestartCount   int
	LastRestart    time.Time
	CooldownActive bool
}

// SendPacket sends a packet to a peer.
// Uses P2P connection if available, falls back to relay.
func (m *Manager) SendPacket(dstVIP net.IP, data []byte) error {
	var key [4]byte
	copy(key[:], dstVIP.To4())

	m.mu.RLock()
	peer, exists := m.peers[key]
	m.mu.RUnlock()

	// Try P2P first
	if exists && peer.State == StateConnected && peer.P2PConn != nil && !peer.UseRelay {
		err := peer.P2PConn.Send(data)
		if err == nil {
			return nil
		}

		m.logger.Debug("p2p: send failed, falling back to relay",
			"peer", dstVIP,
			"err", err)

		// Mark for relay
		m.mu.Lock()
		peer.FailCount++
		if peer.FailCount >= 3 {
			peer.UseRelay = true
		}
		m.mu.Unlock()
	}

	// Fall back to relay
	if m.relayFunc != nil {
		return m.relayFunc(data)
	}

	return fmt.Errorf("p2p: no route to %s", dstVIP)
}

// Connect initiates a P2P connection to a peer.
func (m *Manager) Connect(ctx context.Context, dstVIP net.IP) error {
	var key [4]byte
	copy(key[:], dstVIP.To4())

	m.mu.Lock()
	peer, exists := m.peers[key]
	if !exists {
		peer = &PeerConnection{
			VIP:   dstVIP,
			State: StateDisconnected,
		}
		m.peers[key] = peer
	}

	if peer.State == StateConnected || peer.State == StateConnecting {
		m.mu.Unlock()
		return nil
	}

	peer.State = StateConnecting
	peer.LastAttempt = time.Now()
	m.mu.Unlock()

	m.logger.Info("p2p: initiating connection", "peer", dstVIP)

	// Convert candidates to signal format
	candidateInfos := m.candidatesToInfo(m.candidates)

	// Perform signaling handshake
	result, err := m.signalClient.Handshake(ctx, dstVIP, m.localPub, candidateInfos, m.holePunchTimeout)
	if err != nil {
		m.logger.Debug("p2p: signaling handshake failed", "peer", dstVIP, "err", err)
		m.markFailed(key)
		return err
	}

	// Convert remote candidates
	remoteCandidates := m.infoCandidates(result.RemoteCandidates)

	m.mu.Lock()
	peer.Candidates = remoteCandidates
	m.mu.Unlock()

	// Perform hole punching
	puncher := NewHolePuncher(m.udpConn, m.localVIP, m.holePunchTimeout, m.logger)
	punchResult, err := puncher.Punch(ctx, dstVIP, remoteCandidates)
	if err != nil {
		m.logger.Debug("p2p: hole punch failed", "peer", dstVIP, "err", err)
		m.markFailed(key)
		return err
	}

	m.logger.Info("p2p: hole punch succeeded",
		"peer", dstVIP,
		"addr", punchResult.RemoteAddr,
		"rtt", punchResult.RTT)

	// Derive keys
	sharedSecret, err := m.deriveSharedSecret(result.RemotePublicKey)
	if err != nil {
		m.markFailed(key)
		return fmt.Errorf("p2p: derive shared secret: %w", err)
	}
	sendKey, recvKey := DeriveP2PKeys(sharedSecret, true)

	// Create P2P connection
	p2pConn, err := NewP2PConn(m.udpConn, punchResult.RemoteAddr, dstVIP, m.localVIP, sendKey, recvKey)
	if err != nil {
		m.markFailed(key)
		return err
	}

	m.mu.Lock()
	peer.P2PConn = p2pConn
	peer.State = StateConnected
	peer.FailCount = 0
	peer.UseRelay = false
	m.mu.Unlock()

	// Record successful path for faster reconnection
	method := m.detectConnectionMethod(punchResult)
	m.pathCache.RecordSuccess(dstVIP, punchResult.RemoteAddr, method, punchResult.RTT)

	m.logger.Info("p2p: connection established", "peer", dstVIP, "method", method)
	return nil
}

// detectConnectionMethod determines how the connection was established
func (m *Manager) detectConnectionMethod(result *HolePunchResult) ConnectionMethod {
	if result == nil || result.RemoteAddr == nil {
		return MethodUnknown
	}

	// Check if we used UPnP/NAT-PMP
	if m.upnpEnabled && m.portMapper != nil {
		protocol := m.portMapper.Protocol()
		if protocol == "upnp" {
			return MethodUPnP
		}
		if protocol == "nat-pmp" {
			return MethodNATPMP
		}
	}

	// Check if it's a direct LAN connection
	if isPrivateIP(result.RemoteAddr.IP) && isPrivateIP(m.udpConn.LocalAddr().(*net.UDPAddr).IP) {
		localNet := getNetworkPrefix(m.udpConn.LocalAddr().(*net.UDPAddr).IP)
		remoteNet := getNetworkPrefix(result.RemoteAddr.IP)
		if localNet == remoteNet {
			return MethodDirect
		}
	}

	return MethodSTUN
}

// isPrivateIP checks if an IP is in a private range
func isPrivateIP(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	return ip[0] == 10 ||
		(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) ||
		(ip[0] == 192 && ip[1] == 168)
}

// getNetworkPrefix returns a string representing the /24 network
func getNetworkPrefix(ip net.IP) string {
	ip = ip.To4()
	if ip == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d", ip[0], ip[1], ip[2])
}

// markFailed marks a peer connection as failed.
func (m *Manager) markFailed(key [4]byte) {
	m.mu.Lock()
	var peerVIP net.IP
	if peer, ok := m.peers[key]; ok {
		peer.State = StateFailed
		peer.FailCount++
		peerVIP = peer.VIP
	}
	m.mu.Unlock()

	// Record failure in path cache
	if peerVIP != nil {
		m.pathCache.RecordFailure(peerVIP)
	}
}

// handleCandidates handles incoming candidate messages.
func (m *Manager) handleCandidates(msg *signal.Message) {
	candidates, err := signal.DecodeCandidates(msg.Payload)
	if err != nil {
		m.logger.Debug("p2p: decode candidates failed", "err", err)
		return
	}

	var key [4]byte
	copy(key[:], msg.SrcVIP.To4())

	m.mu.Lock()
	peer, exists := m.peers[key]
	if !exists {
		peer = &PeerConnection{
			VIP:   msg.SrcVIP,
			State: StateDisconnected,
		}
		m.peers[key] = peer
	}
	peer.Candidates = m.infoCandidates(candidates)
	m.mu.Unlock()

	m.logger.Debug("p2p: received candidates",
		"peer", msg.SrcVIP,
		"count", len(candidates))
}

// handleConnect handles incoming connection requests.
func (m *Manager) handleConnect(msg *signal.Message) {
	connectInfo, err := signal.DecodeConnectInfo(msg.Payload)
	if err != nil {
		m.logger.Debug("p2p: decode connect info failed", "err", err)
		return
	}

	var key [4]byte
	copy(key[:], msg.SrcVIP.To4())

	m.mu.Lock()
	peer, exists := m.peers[key]
	if !exists {
		peer = &PeerConnection{
			VIP:   msg.SrcVIP,
			State: StateDisconnected,
		}
		m.peers[key] = peer
	}
	m.mu.Unlock()

	m.logger.Info("p2p: received connection request", "peer", msg.SrcVIP)

	// Respond with our candidates and public key
	candidateInfos := m.candidatesToInfo(m.candidates)
	if err := m.signalClient.RespondToHandshake(m.ctx, msg.SrcVIP, m.localPub, candidateInfos); err != nil {
		m.logger.Debug("p2p: respond to handshake failed", "err", err)
		return
	}

	// Start hole punching in background
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		puncher := NewHolePuncher(m.udpConn, m.localVIP, m.holePunchTimeout, m.logger)

		m.mu.RLock()
		candidates := peer.Candidates
		m.mu.RUnlock()

		if len(candidates) == 0 {
			m.logger.Debug("p2p: no candidates for peer", "peer", msg.SrcVIP)
			return
		}

		punchCtx, punchCancel := context.WithTimeout(m.ctx, m.holePunchTimeout)
		defer punchCancel()
		punchResult, err := puncher.Punch(punchCtx, msg.SrcVIP, candidates)
		if err != nil {
			m.logger.Debug("p2p: hole punch failed", "peer", msg.SrcVIP, "err", err)
			m.markFailed(key)
			return
		}

		// Derive keys (as responder)
		sharedSecret, err := m.deriveSharedSecret(connectInfo.PublicKey)
		if err != nil {
			m.logger.Debug("p2p: derive shared secret failed", "peer", msg.SrcVIP, "err", err)
			m.markFailed(key)
			return
		}
		sendKey, recvKey := DeriveP2PKeys(sharedSecret, false)

		// Create P2P connection
		p2pConn, err := NewP2PConn(m.udpConn, punchResult.RemoteAddr, msg.SrcVIP, m.localVIP, sendKey, recvKey)
		if err != nil {
			m.markFailed(key)
			return
		}

		m.mu.Lock()
		peer.P2PConn = p2pConn
		peer.State = StateConnected
		peer.FailCount = 0
		peer.UseRelay = false
		m.mu.Unlock()

		m.logger.Info("p2p: connection established (responder)", "peer", msg.SrcVIP)
	}()
}

// handleDisconnect handles disconnect notifications.
func (m *Manager) handleDisconnect(msg *signal.Message) {
	var key [4]byte
	copy(key[:], msg.SrcVIP.To4())

	m.mu.Lock()
	if peer, ok := m.peers[key]; ok {
		if peer.P2PConn != nil {
			peer.P2PConn.Close()
		}
		peer.State = StateDisconnected
		peer.P2PConn = nil
	}
	m.mu.Unlock()

	m.logger.Info("p2p: peer disconnected", "peer", msg.SrcVIP)
}

// handleTrickleCandidate handles incoming trickle candidate messages.
func (m *Manager) handleTrickleCandidate(msg *signal.Message) {
	info, err := signal.DecodeTrickleCandidate(msg.Payload)
	if err != nil {
		m.logger.Debug("p2p: decode trickle candidate failed", "err", err)
		return
	}

	var key [4]byte
	copy(key[:], msg.SrcVIP.To4())

	m.mu.Lock()
	peer, exists := m.peers[key]
	if !exists {
		peer = &PeerConnection{
			VIP:   msg.SrcVIP,
			State: StateDisconnected,
		}
		m.peers[key] = peer
	}

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

	var key [4]byte
	copy(key[:], msg.SrcVIP.To4())

	m.mu.Lock()
	peer, exists := m.peers[key]
	if exists {
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
			Port:     uint16(candidate.Addr.Port),
			Priority: candidate.Priority,
		},
		Generation: uint16(m.iceGeneration),
		MLineIndex: 0,
	}

	if candidate.RelatedIP != nil {
		info.Candidate.RelatedIP = candidate.RelatedIP
		info.Candidate.RelatedPort = uint16(candidate.RelatedPort)
	}

	return m.signalClient.SendTrickleCandidate(dstVIP, info)
}

// SendEndOfCandidates sends an end-of-candidates signal to a peer.
func (m *Manager) SendEndOfCandidates(dstVIP net.IP) error {
	if m.signalClient == nil {
		return fmt.Errorf("p2p: no signal client")
	}

	info := &signal.EndOfCandidatesInfo{
		Generation:     uint16(m.iceGeneration),
		TotalCandidates: uint8(len(m.candidates)),
	}

	return m.signalClient.SendEndOfCandidates(dstVIP, info)
}

// IsTrickleEnabled returns whether Trickle ICE is enabled.
func (m *Manager) IsTrickleEnabled() bool {
	return m.trickleEnabled
}

// receiveLoop handles incoming UDP packets.
func (m *Manager) receiveLoop() {
	buf := make([]byte, 1500)

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		m.udpConn.SetReadDeadline(time.Now().Add(time.Second))
		n, addr, err := m.udpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			m.logger.Debug("p2p: receive error", "err", err)
			continue
		}

		data := buf[:n]

		// Handle hole punch packets
		if IsHolePunchPacket(data) {
			continue // Handled by HolePuncher
		}

		// Handle P2P data packets
		if IsP2PPacket(data) {
			m.handleP2PPacket(data, addr)
			continue
		}
	}
}

// handleP2PPacket handles an incoming P2P packet.
func (m *Manager) handleP2PPacket(data []byte, from *net.UDPAddr) {
	// Find the peer by address
	m.mu.RLock()
	var peer *PeerConnection
	for _, p := range m.peers {
		if p.P2PConn != nil && p.P2PConn.RemoteAddr().String() == from.String() {
			peer = p
			break
		}
	}
	m.mu.RUnlock()

	if peer == nil || peer.P2PConn == nil {
		m.logger.Debug("p2p: packet from unknown peer", "from", from)
		return
	}

	plaintext, typ, err := peer.P2PConn.Decrypt(data)
	if err != nil {
		m.logger.Debug("p2p: decrypt failed", "err", err)
		return
	}

	switch typ {
	case P2PData:
		// TODO: Forward to TUN device or mesh handler
		m.logger.Debug("p2p: received data", "len", len(plaintext), "peer", peer.VIP)
	case P2PKeepAlive:
		m.logger.Debug("p2p: received keepalive", "peer", peer.VIP)
	case P2PClose:
		m.logger.Info("p2p: peer closed connection", "peer", peer.VIP)
		m.mu.Lock()
		peer.State = StateDisconnected
		peer.P2PConn = nil
		m.mu.Unlock()
	}
}

// keepAliveLoop sends keep-alive packets to connected peers.
func (m *Manager) keepAliveLoop() {
	ticker := time.NewTicker(m.keepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.mu.RLock()
			for _, peer := range m.peers {
				if peer.State == StateConnected && peer.P2PConn != nil {
					peer.P2PConn.SendKeepAlive()
				}
			}
			m.mu.RUnlock()
		}
	}
}

// retryLoop periodically retries failed connections.
func (m *Manager) retryLoop() {
	ticker := time.NewTicker(m.directRetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.mu.RLock()
			toRetry := make([]net.IP, 0)
			for _, peer := range m.peers {
				if peer.UseRelay && time.Since(peer.LastAttempt) > m.directRetryInterval {
					toRetry = append(toRetry, peer.VIP)
				}
			}
			m.mu.RUnlock()

			for _, vip := range toRetry {
				go m.Connect(m.ctx, vip)
			}
		}
	}
}

// candidatesToInfo converts Candidates to CandidateInfo for signaling.
func (m *Manager) candidatesToInfo(candidates []*Candidate) []*signal.CandidateInfo {
	infos := make([]*signal.CandidateInfo, len(candidates))
	for i, c := range candidates {
		infos[i] = &signal.CandidateInfo{
			Type:     byte(c.Type),
			IP:       c.Addr.IP,
			Port:     uint16(c.Addr.Port),
			Priority: c.Priority,
		}
		if c.RelatedIP != nil {
			infos[i].RelatedIP = c.RelatedIP
			infos[i].RelatedPort = uint16(c.RelatedPort)
		}
	}
	return infos
}

// infoCandidates converts CandidateInfo to Candidates.
func (m *Manager) infoCandidates(infos []*signal.CandidateInfo) []*Candidate {
	candidates := make([]*Candidate, len(infos))
	for i, info := range infos {
		candidates[i] = &Candidate{
			Type:     CandidateType(info.Type),
			Addr:     &net.UDPAddr{IP: info.IP, Port: int(info.Port)},
			Priority: info.Priority,
		}
		if info.RelatedIP != nil {
			candidates[i].RelatedIP = info.RelatedIP
			candidates[i].RelatedPort = int(info.RelatedPort)
		}
	}
	return candidates
}

// deriveSharedSecret derives a shared secret from the remote public key using X25519 ECDH.
func (m *Manager) deriveSharedSecret(remotePub [32]byte) ([]byte, error) {
	shared, err := curve25519.X25519(m.localPriv[:], remotePub[:])
	if err != nil {
		return nil, fmt.Errorf("x25519: %w", err)
	}
	return shared, nil
}

// GetPeerState returns the state of a peer connection.
func (m *Manager) GetPeerState(vip net.IP) ConnectionState {
	var key [4]byte
	copy(key[:], vip.To4())

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

// ManagerStats contains P2P manager statistics.
type ManagerStats struct {
	TotalPeers      int
	DirectPeers     int
	RelayPeers      int
	ConnectingPeers int
	FailedPeers     int
}

// PeerInfo contains information about a peer including quality metrics.
type PeerInfo struct {
	VirtualIP string          `json:"virtual_ip"`
	State     string          `json:"state"`
	Method    string          `json:"method,omitempty"`
	Quality   *QualityMetrics `json:"quality,omitempty"`
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

// LocalCandidates returns the local ICE candidates.
func (m *Manager) LocalCandidates() []*Candidate {
	return m.candidates
}

// SetDataHandler sets the handler for incoming P2P data.
func (m *Manager) SetDataHandler(handler func(srcVIP net.IP, data []byte)) {
	// This would be called when P2P data is received
	// Implementation would store the handler and call it from handleP2PPacket
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

// PortMappingInfo contains port mapping status information.
type PortMappingInfo struct {
	Enabled      bool
	Protocol     string // "upnp" or "nat-pmp"
	ExternalIP   net.IP
	ExternalPort int
	LocalPort    int
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

// GetUPnPInfo returns UPnP port mapping information.
// Deprecated: Use GetPortMappingInfo instead.
func (m *Manager) GetUPnPInfo() *PortMappingInfo {
	return m.GetPortMappingInfo()
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
	for _, c := range m.candidates {
		if c.Type == CandidateUPnP || c.Type == CandidateServerReflexive {
			return c.Addr
		}
	}

	// Fall back to local address
	return m.udpConn.LocalAddr().(*net.UDPAddr)
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
	status.CandidateCount = len(m.candidates)
	for _, c := range m.candidates {
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
