package p2p

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/curve25519"

	"github.com/ggshr9/shuttle/mesh/signal"
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

	// Fallback controller tracks relay vs direct state per peer
	fallback *FallbackController

	// ICE restart configuration
	iceRestartCooldown    time.Duration
	iceQualityThreshold   float64 // Restart if quality drops below this (0-100)
	iceRestartEnabled     bool

	// Trickle ICE configuration
	trickleEnabled bool
	iceGeneration     int // Current ICE credential generation

	// Incoming data handler
	dataHandler func(srcVIP net.IP, data []byte)

	// mdns is the optional mDNS service used for LAN peer discovery.
	// When set, candidates from mDNS peers are only used after the peer's
	// VIP ownership has been confirmed by a successful X25519 handshake.
	mdns *MDNSService

	// activeHPs maps remote-peer VIP -> HolePuncher waiting for packets.
	// Concurrent inbound + outbound (or two simultaneous inbound from
	// different peers) need distinct entries; previously a single
	// activeHP pointer was overwritten silently. Protected by activeHPMu.
	activeHPMu sync.Mutex
	activeHPs  map[[4]byte]*HolePuncher

	// Goroutine lifecycle
	wg sync.WaitGroup

	// stopping flips to true at the very top of Stop, before cancel().
	// Signal-handler goroutines (which may fire from a separate signal
	// client even after cancel) check this before calling wg.Add — Add
	// after Wait would panic with "WaitGroup misuse".
	stopping atomic.Bool

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

// ManagerStats contains P2P manager statistics.
type ManagerStats struct {
	TotalPeers      int
	DirectPeers     int
	RelayPeers      int
	ConnectingPeers int
	FailedPeers     int
}

// ICERestartStats contains ICE restart statistics.
type ICERestartStats struct {
	Generation     int
	RestartCount   int
	LastRestart    time.Time
	CooldownActive bool
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
		activeHPs:           make(map[[4]byte]*HolePuncher),
		stunServers:         cfg.STUNServers,
		holePunchTimeout:    cfg.HolePunchTimeout,
		directRetryInterval: cfg.DirectRetryInterval,
		keepAliveInterval:   cfg.KeepAliveInterval,
		spoofConfig:         cfg.SpoofConfig,
		spoofInfo:           spoofInfo,
		pathCache:           NewPathCache(24 * time.Hour),
		fallback:            NewFallbackController(nil, logger),
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

	// Build the candidate list locally then publish under m.mu so
	// readers (LocalCandidates / GetNATStatus / Connect / handleConnect)
	// never observe a torn assignment or an in-progress append.
	var built []*Candidate
	if err != nil {
		m.logger.Warn("p2p: gather candidates failed", "err", err)
	} else {
		built = result.Candidates
	}
	if m.upnpEnabled && m.portMapper != nil {
		upnpAddr := m.portMapper.GetExternalAddr()
		if upnpAddr != nil {
			upnpCandidate := NewCandidate(CandidateUPnP, upnpAddr)
			upnpCandidate.Base = m.udpConn.LocalAddr().(*net.UDPAddr)
			built = append([]*Candidate{upnpCandidate}, built...)
			m.logger.Info("p2p: added UPnP candidate", "addr", upnpAddr)
		}
	}
	m.mu.Lock()
	m.candidates = built
	m.mu.Unlock()

	m.logger.Info("p2p: gathered candidates", "count", len(built))
	for _, c := range built {
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
	// Set stopping FIRST so any signal handler that fires concurrently
	// with Stop returns early instead of doing wg.Add after wg.Wait.
	m.stopping.Store(true)
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

// candidatesToInfo converts Candidates to CandidateInfo for signaling.
func (m *Manager) candidatesToInfo(candidates []*Candidate) []*signal.CandidateInfo {
	infos := make([]*signal.CandidateInfo, len(candidates))
	for i, c := range candidates {
		infos[i] = &signal.CandidateInfo{
			Type:     byte(c.Type),
			IP:       c.Addr.IP,
			Port:     uint16(c.Addr.Port), //nolint:gosec // G115: port range 0-65535, fits uint16
			Priority: c.Priority,
		}
		if c.RelatedIP != nil {
			infos[i].RelatedIP = c.RelatedIP
			infos[i].RelatedPort = uint16(c.RelatedPort) //nolint:gosec // G115: port range 0-65535, fits uint16
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

// localCandidatesSnapshot returns a snapshot slice of m.candidates safe
// for the caller to range over while concurrent re-gathers / ICE-restart
// reassign m.candidates. The Candidate pointers themselves are treated
// as immutable post-publish.
func (m *Manager) localCandidatesSnapshot() []*Candidate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.candidates) == 0 {
		return nil
	}
	out := make([]*Candidate, len(m.candidates))
	copy(out, m.candidates)
	return out
}

// setActiveHolePuncher registers hp as the active hole puncher for the
// remote peer identified by remoteVIP, so receiveLoop can route
// hole-punch packets sourced from that peer to this puncher. Also marks
// hp as managed so its own Punch loop does not start a competing socket
// reader.
func (m *Manager) setActiveHolePuncher(remoteVIP net.IP, hp *HolePuncher) {
	hp.managed = true
	m.activeHPMu.Lock()
	m.activeHPs[vipKey(remoteVIP)] = hp
	m.activeHPMu.Unlock()
}

// clearActiveHolePuncher removes the registration for remoteVIP.
func (m *Manager) clearActiveHolePuncher(remoteVIP net.IP) {
	m.activeHPMu.Lock()
	delete(m.activeHPs, vipKey(remoteVIP))
	m.activeHPMu.Unlock()
}

// deliverToHolePuncher forwards a hole-punch packet to the puncher that
// is targeting the packet's source VIP (if any). Called from receiveLoop
// under no manager locks.
func (m *Manager) deliverToHolePuncher(data []byte, addr *net.UDPAddr) {
	pkt, err := DecodeHolePunchPacket(data)
	if err != nil {
		return
	}
	m.activeHPMu.Lock()
	hp := m.activeHPs[vipKey(pkt.SrcVIP)]
	m.activeHPMu.Unlock()
	if hp != nil {
		hp.Deliver(data, addr)
	}
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
