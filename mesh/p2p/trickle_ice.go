// Package p2p implements Trickle ICE per RFC 8838.
// Trickle ICE allows ICE agents to send and receive candidates incrementally,
// reducing the time to establish connectivity.
package p2p

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// TrickleState represents the state of trickle ICE gathering.
type TrickleState int

const (
	TrickleStateNew TrickleState = iota
	TrickleStateGathering
	TrickleStateComplete
)

func (s TrickleState) String() string {
	switch s {
	case TrickleStateNew:
		return "new"
	case TrickleStateGathering:
		return "gathering"
	case TrickleStateComplete:
		return "complete"
	default:
		return "unknown"
	}
}

// TrickleCandidateCallback is called when a new candidate is discovered.
type TrickleCandidateCallback func(candidate *Candidate)

// TrickleGatheringCompleteCallback is called when gathering is complete.
type TrickleGatheringCompleteCallback func()

// TrickleICEGatherer gathers ICE candidates incrementally.
type TrickleICEGatherer struct {
	mu sync.RWMutex

	stunServers []string
	timeout     time.Duration
	logger      *slog.Logger

	state       TrickleState
	candidates  []*Candidate
	localConn   *net.UDPConn
	natInfo     *NATInfo

	// Callbacks
	onCandidate         TrickleCandidateCallback
	onGatheringComplete TrickleGatheringCompleteCallback

	// Cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// TrickleGathererConfig holds configuration for trickle gatherer.
type TrickleGathererConfig struct {
	STUNServers []string
	Timeout     time.Duration
	Logger      *slog.Logger
}

// NewTrickleICEGatherer creates a new trickle ICE gatherer.
func NewTrickleICEGatherer(cfg *TrickleGathererConfig) *TrickleICEGatherer {
	if cfg == nil {
		cfg = &TrickleGathererConfig{}
	}

	// Only fill defaults when STUNServers is nil (not explicitly set).
	// An explicit empty slice []string{} means "no STUN servers".
	if cfg.STUNServers == nil {
		cfg.STUNServers = DefaultSTUNServers()
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TrickleICEGatherer{
		stunServers: cfg.STUNServers,
		timeout:     cfg.Timeout,
		logger:      cfg.Logger,
		state:       TrickleStateNew,
		candidates:  make([]*Candidate, 0),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// OnCandidate sets the callback for new candidate discovery.
func (g *TrickleICEGatherer) OnCandidate(cb TrickleCandidateCallback) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onCandidate = cb
}

// OnGatheringComplete sets the callback for gathering completion.
func (g *TrickleICEGatherer) OnGatheringComplete(cb TrickleGatheringCompleteCallback) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onGatheringComplete = cb
}

// GetState returns the current gathering state.
func (g *TrickleICEGatherer) GetState() TrickleState {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.state
}

// GetCandidates returns all gathered candidates so far.
func (g *TrickleICEGatherer) GetCandidates() []*Candidate {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]*Candidate, len(g.candidates))
	copy(result, g.candidates)
	return result
}

// GetLocalConn returns the local UDP connection.
func (g *TrickleICEGatherer) GetLocalConn() *net.UDPConn {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.localConn
}

// GetNATInfo returns NAT information if available.
func (g *TrickleICEGatherer) GetNATInfo() *NATInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.natInfo
}

// Gather starts gathering candidates asynchronously.
// Returns immediately; candidates are delivered via OnCandidate callback.
func (g *TrickleICEGatherer) Gather() error {
	g.mu.Lock()
	if g.state != TrickleStateNew {
		g.mu.Unlock()
		return fmt.Errorf("trickle: already gathering or complete")
	}
	g.state = TrickleStateGathering
	g.mu.Unlock()

	g.logger.Debug("trickle: starting candidate gathering")

	// Create UDP socket
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		g.mu.Lock()
		g.state = TrickleStateNew
		g.mu.Unlock()
		return fmt.Errorf("trickle: listen: %w", err)
	}

	g.mu.Lock()
	g.localConn = conn
	g.mu.Unlock()

	// Start gathering in background
	go g.gatherAsync(conn)

	return nil
}

// GatherWithConnection starts gathering using an existing connection.
func (g *TrickleICEGatherer) GatherWithConnection(conn *net.UDPConn) error {
	g.mu.Lock()
	if g.state != TrickleStateNew {
		g.mu.Unlock()
		return fmt.Errorf("trickle: already gathering or complete")
	}
	g.state = TrickleStateGathering
	g.localConn = conn
	g.mu.Unlock()

	g.logger.Debug("trickle: starting candidate gathering with existing connection")

	go g.gatherAsync(conn)

	return nil
}

// gatherAsync gathers candidates asynchronously.
func (g *TrickleICEGatherer) gatherAsync(conn *net.UDPConn) {
	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// Phase 1: Gather host candidates (fast, local operation)
	g.gatherHostCandidates(localAddr.Port)

	// Check for cancellation
	select {
	case <-g.ctx.Done():
		g.completeGathering()
		return
	default:
	}

	// Phase 2: Gather server-reflexive candidates (requires network)
	g.gatherServerReflexiveCandidates(conn)

	// Phase 3: Optionally gather relay candidates (TURN)
	// This would be added here if TURN is configured

	// Mark gathering as complete
	g.completeGathering()
}

// gatherHostCandidates gathers local network addresses.
func (g *TrickleICEGatherer) gatherHostCandidates(port int) {
	interfaces, err := net.Interfaces()
	if err != nil {
		g.logger.Debug("trickle: failed to get interfaces", "err", err)
		return
	}

	for _, iface := range interfaces {
		// Check for cancellation
		select {
		case <-g.ctx.Done():
			return
		default:
		}

		// Skip down interfaces
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Skip loopback
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil {
				continue
			}

			// Only IPv4 for now
			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}

			// Skip loopback
			if ip4.IsLoopback() {
				continue
			}

			candidate := NewCandidate(CandidateHost, &net.UDPAddr{
				IP:   ip4,
				Port: port,
			})

			g.addCandidate(candidate)
		}
	}
}

// gatherServerReflexiveCandidates gathers STUN reflexive addresses.
func (g *TrickleICEGatherer) gatherServerReflexiveCandidates(conn *net.UDPConn) {
	if len(g.stunServers) == 0 {
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(g.ctx, g.timeout)
	defer cancel()

	// Try each STUN server
	for _, server := range g.stunServers {
		select {
		case <-ctx.Done():
			return
		default:
		}

		publicAddr, err := g.querySTUNServer(conn, server)
		if err != nil {
			g.logger.Debug("trickle: STUN query failed",
				"server", server,
				"err", err)
			continue
		}

		if publicAddr != nil {
			localAddr := conn.LocalAddr().(*net.UDPAddr)

			// Only add if different from local address
			if !publicAddr.IP.Equal(localAddr.IP) {
				candidate := NewCandidate(CandidateServerReflexive, publicAddr)
				candidate.Base = localAddr
				candidate.RelatedIP = localAddr.IP
				candidate.RelatedPort = localAddr.Port

				g.addCandidate(candidate)

				// Store NAT info from first successful STUN response
				g.mu.Lock()
				if g.natInfo == nil {
					g.natInfo = &NATInfo{
						PublicAddr: publicAddr,
					}
				}
				g.mu.Unlock()
			}

			// One successful STUN response is usually enough
			break
		}
	}
}

// querySTUNServer queries a single STUN server.
func (g *TrickleICEGatherer) querySTUNServer(conn *net.UDPConn, server string) (*net.UDPAddr, error) {
	// Resolve server address
	serverAddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, err
	}

	// Build STUN binding request
	req := buildSTUNBindingRequest()

	// Set deadline for this request
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.WriteToUDP(req, serverAddr)
	if err != nil {
		return nil, err
	}

	// Read response
	buf := make([]byte, 1500)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, err
	}

	// Parse STUN response
	return parseSTUNBindingResponse(buf[:n])
}

// addCandidate adds a candidate and notifies callback.
func (g *TrickleICEGatherer) addCandidate(candidate *Candidate) {
	g.mu.Lock()
	// Check for duplicates
	for _, c := range g.candidates {
		if c.Addr.String() == candidate.Addr.String() && c.Type == candidate.Type {
			g.mu.Unlock()
			return
		}
	}
	g.candidates = append(g.candidates, candidate)
	cb := g.onCandidate
	g.mu.Unlock()

	g.logger.Debug("trickle: discovered candidate",
		"type", candidate.Type,
		"addr", candidate.Addr)

	// Notify callback outside lock
	if cb != nil {
		cb(candidate)
	}
}

// completeGathering marks gathering as complete.
func (g *TrickleICEGatherer) completeGathering() {
	g.mu.Lock()
	if g.state == TrickleStateComplete {
		g.mu.Unlock()
		return
	}
	g.state = TrickleStateComplete
	cb := g.onGatheringComplete
	candidateCount := len(g.candidates)
	g.mu.Unlock()

	g.logger.Debug("trickle: gathering complete",
		"candidates", candidateCount)

	if cb != nil {
		cb()
	}
}

// Stop stops the gathering process.
func (g *TrickleICEGatherer) Stop() {
	g.cancel()
}

// Close stops gathering and cleans up resources.
func (g *TrickleICEGatherer) Close() error {
	g.Stop()

	g.mu.Lock()
	defer g.mu.Unlock()

	// Note: We don't close localConn here as it may be shared
	g.state = TrickleStateComplete

	return nil
}

// buildSTUNBindingRequest builds a STUN Binding Request.
func buildSTUNBindingRequest() []byte {
	// STUN header: Type (2) + Length (2) + Magic Cookie (4) + Transaction ID (12) = 20 bytes
	buf := make([]byte, 20)

	// Message Type: Binding Request (0x0001)
	buf[0] = 0x00
	buf[1] = 0x01

	// Message Length: 0 (no attributes)
	buf[2] = 0x00
	buf[3] = 0x00

	// Magic Cookie: 0x2112A442
	buf[4] = 0x21
	buf[5] = 0x12
	buf[6] = 0xA4
	buf[7] = 0x42

	// Transaction ID: random 12 bytes
	for i := 8; i < 20; i++ {
		buf[i] = byte(time.Now().UnixNano() >> (i * 8))
	}

	return buf
}

// parseSTUNBindingResponse parses a STUN Binding Response.
func parseSTUNBindingResponse(data []byte) (*net.UDPAddr, error) {
	if len(data) < 20 {
		return nil, fmt.Errorf("stun: response too short")
	}

	// Check message type (Binding Success Response: 0x0101)
	msgType := uint16(data[0])<<8 | uint16(data[1])
	if msgType != 0x0101 {
		return nil, fmt.Errorf("stun: not a binding response: 0x%04x", msgType)
	}

	// Check magic cookie
	cookie := uint32(data[4])<<24 | uint32(data[5])<<16 | uint32(data[6])<<8 | uint32(data[7])
	if cookie != 0x2112A442 {
		return nil, fmt.Errorf("stun: invalid magic cookie")
	}

	// Parse attributes
	msgLen := int(uint16(data[2])<<8 | uint16(data[3]))
	if len(data) < 20+msgLen {
		return nil, fmt.Errorf("stun: truncated message")
	}

	offset := 20
	for offset < 20+msgLen {
		if offset+4 > len(data) {
			break
		}

		attrType := uint16(data[offset])<<8 | uint16(data[offset+1])
		attrLen := int(uint16(data[offset+2])<<8 | uint16(data[offset+3]))
		offset += 4

		if offset+attrLen > len(data) {
			break
		}

		attrData := data[offset : offset+attrLen]

		// XOR-MAPPED-ADDRESS (0x0020) or MAPPED-ADDRESS (0x0001)
		if attrType == 0x0020 && attrLen >= 8 {
			// XOR-MAPPED-ADDRESS
			family := attrData[1]
			if family == 0x01 { // IPv4
				xorPort := uint16(attrData[2])<<8 | uint16(attrData[3])
				port := xorPort ^ 0x2112 // XOR with magic cookie upper 16 bits

				xorIP := make([]byte, 4)
				copy(xorIP, attrData[4:8])
				ip := make(net.IP, 4)
				ip[0] = xorIP[0] ^ 0x21
				ip[1] = xorIP[1] ^ 0x12
				ip[2] = xorIP[2] ^ 0xA4
				ip[3] = xorIP[3] ^ 0x42

				return &net.UDPAddr{IP: ip, Port: int(port)}, nil
			}
		} else if attrType == 0x0001 && attrLen >= 8 {
			// MAPPED-ADDRESS (fallback for old servers)
			family := attrData[1]
			if family == 0x01 { // IPv4
				port := uint16(attrData[2])<<8 | uint16(attrData[3])
				ip := net.IP(attrData[4:8])
				return &net.UDPAddr{IP: ip, Port: int(port)}, nil
			}
		}

		// Advance to next attribute (with padding to 4-byte boundary)
		offset += attrLen
		if attrLen%4 != 0 {
			offset += 4 - (attrLen % 4)
		}
	}

	return nil, fmt.Errorf("stun: no mapped address in response")
}

// EndOfCandidates is a sentinel value indicating no more candidates will arrive.
// In Trickle ICE, this is signaled after gathering is complete.
type EndOfCandidates struct{}

// IsEndOfCandidates checks if a candidate represents end-of-candidates.
func IsEndOfCandidates(c *Candidate) bool {
	return c == nil
}
