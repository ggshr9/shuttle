package p2p

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

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
}

// NewManager creates a new P2P connection manager.
func NewManager(cfg *Config, signalClient *signal.Client, logger *slog.Logger) (*Manager, error) {
	// Create UDP socket
	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("p2p: listen: %w", err)
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
	if len(m.stunServers) == 0 {
		m.stunServers = DefaultSTUNServers()
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
		m.logger.Info("p2p: gathered candidates", "count", len(m.candidates))
		for _, c := range m.candidates {
			m.logger.Debug("p2p: candidate", "type", c.Type, "addr", c.Addr)
		}
	}

	// Set up signal handlers
	if m.signalClient != nil {
		m.signalClient.OnMessage(signal.SignalCandidate, m.handleCandidates)
		m.signalClient.OnMessage(signal.SignalConnect, m.handleConnect)
		m.signalClient.OnMessage(signal.SignalDisconnect, m.handleDisconnect)
	}

	// Start receive loop
	go m.receiveLoop()

	// Start keep-alive loop
	go m.keepAliveLoop()

	// Start retry loop
	go m.retryLoop()

	return nil
}

// Stop stops the manager.
func (m *Manager) Stop() error {
	m.cancel()

	m.mu.Lock()
	for _, peer := range m.peers {
		if peer.P2PConn != nil {
			peer.P2PConn.Close()
		}
	}
	m.peers = make(map[[4]byte]*PeerConnection)
	m.mu.Unlock()

	return m.udpConn.Close()
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
	sharedSecret := m.deriveSharedSecret(result.RemotePublicKey)
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

	m.logger.Info("p2p: connection established", "peer", dstVIP)
	return nil
}

// markFailed marks a peer connection as failed.
func (m *Manager) markFailed(key [4]byte) {
	m.mu.Lock()
	if peer, ok := m.peers[key]; ok {
		peer.State = StateFailed
		peer.FailCount++
	}
	m.mu.Unlock()
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
	go func() {
		puncher := NewHolePuncher(m.udpConn, m.localVIP, m.holePunchTimeout, m.logger)

		m.mu.RLock()
		candidates := peer.Candidates
		m.mu.RUnlock()

		if len(candidates) == 0 {
			m.logger.Debug("p2p: no candidates for peer", "peer", msg.SrcVIP)
			return
		}

		punchResult, err := puncher.Punch(m.ctx, msg.SrcVIP, candidates)
		if err != nil {
			m.logger.Debug("p2p: hole punch failed", "peer", msg.SrcVIP, "err", err)
			m.markFailed(key)
			return
		}

		// Derive keys (as responder)
		sharedSecret := m.deriveSharedSecret(connectInfo.PublicKey)
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

// deriveSharedSecret derives a shared secret from the remote public key.
// This is a simplified version - in production use proper DH.
func (m *Manager) deriveSharedSecret(remotePub [32]byte) []byte {
	// XOR local private with remote public (simplified)
	// In production, use proper X25519 DH
	secret := make([]byte, 64)
	for i := 0; i < 32; i++ {
		secret[i] = m.localPriv[i] ^ remotePub[i]
		secret[32+i] = m.localPub[i] ^ remotePub[i]
	}
	return secret
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

// LocalCandidates returns the local ICE candidates.
func (m *Manager) LocalCandidates() []*Candidate {
	return m.candidates
}

// SetDataHandler sets the handler for incoming P2P data.
func (m *Manager) SetDataHandler(handler func(srcVIP net.IP, data []byte)) {
	// This would be called when P2P data is received
	// Implementation would store the handler and call it from handleP2PPacket
}
