package p2p

import (
	"fmt"
	"net"
	"sort"
	"time"
)

// CandidateType represents the type of ICE candidate.
type CandidateType int

const (
	CandidateHost            CandidateType = iota // Local network address
	CandidateServerReflexive                      // Address from STUN (NAT public address)
	CandidatePeerReflexive                        // Discovered during connectivity checks
	CandidateRelay                                // TURN relay address
)

func (c CandidateType) String() string {
	switch c {
	case CandidateHost:
		return "host"
	case CandidateServerReflexive:
		return "srflx"
	case CandidatePeerReflexive:
		return "prflx"
	case CandidateRelay:
		return "relay"
	default:
		return "unknown"
	}
}

// Priority returns the base priority for this candidate type.
// Lower number = higher priority.
func (c CandidateType) Priority() int {
	switch c {
	case CandidateHost:
		return 126
	case CandidateServerReflexive:
		return 100
	case CandidatePeerReflexive:
		return 110
	case CandidateRelay:
		return 0
	default:
		return 0
	}
}

// Candidate represents an ICE candidate.
type Candidate struct {
	Type       CandidateType
	Addr       *net.UDPAddr
	Base       *net.UDPAddr // For srflx, the local address behind NAT
	Priority   uint32
	Foundation string
	RelatedIP  net.IP
	RelatedPort int
}

// NewCandidate creates a new candidate.
func NewCandidate(typ CandidateType, addr *net.UDPAddr) *Candidate {
	c := &Candidate{
		Type: typ,
		Addr: addr,
	}
	c.computePriority()
	c.computeFoundation()
	return c
}

// computePriority calculates the candidate priority per RFC 5245.
func (c *Candidate) computePriority() {
	// priority = (2^24)*(type preference) + (2^8)*(local preference) + (256 - component ID)
	typePreference := uint32(c.Type.Priority())
	localPreference := uint32(65535) // Single component for simplicity

	// Prefer non-link-local addresses
	if c.Addr != nil && !c.Addr.IP.IsLinkLocalUnicast() {
		localPreference = 65535
	} else {
		localPreference = 32768
	}

	componentID := uint32(1) // UDP component

	c.Priority = (typePreference << 24) + (localPreference << 8) + (256 - componentID)
}

// computeFoundation computes a foundation string that identifies related candidates.
func (c *Candidate) computeFoundation() {
	// Foundation is based on type and base IP
	ip := c.Addr.IP.String()
	if c.Base != nil {
		ip = c.Base.IP.String()
	}
	c.Foundation = fmt.Sprintf("%s-%s", c.Type, ip)
}

// String returns a string representation of the candidate.
func (c *Candidate) String() string {
	return fmt.Sprintf("%s %s (priority=%d)", c.Type, c.Addr, c.Priority)
}

// ICEGatherer collects ICE candidates.
type ICEGatherer struct {
	stunServers []string
	timeout     time.Duration
}

// NewICEGatherer creates a new ICE gatherer.
func NewICEGatherer(stunServers []string, timeout time.Duration) *ICEGatherer {
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &ICEGatherer{
		stunServers: stunServers,
		timeout:     timeout,
	}
}

// GatherResult contains the gathered candidates.
type GatherResult struct {
	Candidates  []*Candidate
	LocalConn   *net.UDPConn
	NATInfo     *NATInfo
}

// Gather collects ICE candidates.
func (g *ICEGatherer) Gather() (*GatherResult, error) {
	result := &GatherResult{
		Candidates: make([]*Candidate, 0),
	}

	// Create UDP socket for P2P communication
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("ice: listen: %w", err)
	}
	result.LocalConn = conn

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// Gather host candidates
	hostCandidates := g.gatherHostCandidates(localAddr.Port)
	result.Candidates = append(result.Candidates, hostCandidates...)

	// Gather server-reflexive candidates via STUN
	srflxCandidates, natInfo := g.gatherServerReflexiveCandidates(conn)
	result.Candidates = append(result.Candidates, srflxCandidates...)
	result.NATInfo = natInfo

	// Sort candidates by priority (highest first)
	sort.Slice(result.Candidates, func(i, j int) bool {
		return result.Candidates[i].Priority > result.Candidates[j].Priority
	})

	return result, nil
}

// gatherHostCandidates gathers local network addresses.
func (g *ICEGatherer) gatherHostCandidates(port int) []*Candidate {
	candidates := make([]*Candidate, 0)

	interfaces, err := net.Interfaces()
	if err != nil {
		return candidates
	}

	for _, iface := range interfaces {
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

			// Skip loopback and link-local for host candidates
			if ip4.IsLoopback() {
				continue
			}

			candidate := NewCandidate(CandidateHost, &net.UDPAddr{
				IP:   ip4,
				Port: port,
			})
			candidates = append(candidates, candidate)
		}
	}

	return candidates
}

// gatherServerReflexiveCandidates gathers STUN reflexive addresses.
func (g *ICEGatherer) gatherServerReflexiveCandidates(conn *net.UDPConn) ([]*Candidate, *NATInfo) {
	candidates := make([]*Candidate, 0)

	if len(g.stunServers) == 0 {
		return candidates, nil
	}

	// Perform NAT detection which also gets our public address
	detector := NewNATDetector(g.stunServers, g.timeout)
	natInfo, err := detector.Detect()
	if err != nil {
		return candidates, nil
	}

	// Add server-reflexive candidate if different from local
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	if natInfo.PublicAddr != nil && !natInfo.PublicAddr.IP.Equal(localAddr.IP) {
		candidate := NewCandidate(CandidateServerReflexive, natInfo.PublicAddr)
		candidate.Base = localAddr
		candidate.RelatedIP = localAddr.IP
		candidate.RelatedPort = localAddr.Port
		candidates = append(candidates, candidate)
	}

	return candidates, natInfo
}

// GatherWithConnection gathers candidates using an existing UDP connection.
func (g *ICEGatherer) GatherWithConnection(conn *net.UDPConn) (*GatherResult, error) {
	result := &GatherResult{
		Candidates: make([]*Candidate, 0),
		LocalConn:  conn,
	}

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// Gather host candidates
	hostCandidates := g.gatherHostCandidates(localAddr.Port)
	result.Candidates = append(result.Candidates, hostCandidates...)

	// Gather server-reflexive candidates
	srflxCandidates, natInfo := g.gatherServerReflexiveCandidates(conn)
	result.Candidates = append(result.Candidates, srflxCandidates...)
	result.NATInfo = natInfo

	// Sort by priority
	sort.Slice(result.Candidates, func(i, j int) bool {
		return result.Candidates[i].Priority > result.Candidates[j].Priority
	})

	return result, nil
}

// CandidatePair represents a pair of local and remote candidates for connectivity checking.
type CandidatePair struct {
	Local    *Candidate
	Remote   *Candidate
	Priority uint64
	State    CandidatePairState
}

// CandidatePairState represents the state of a candidate pair.
type CandidatePairState int

const (
	CandidatePairFrozen CandidatePairState = iota
	CandidatePairWaiting
	CandidatePairInProgress
	CandidatePairSucceeded
	CandidatePairFailed
)

// ComputePairPriority computes the priority of a candidate pair per RFC 5245.
func ComputePairPriority(controlling bool, localPriority, remotePriority uint32) uint64 {
	var g, d uint32
	if controlling {
		g = localPriority
		d = remotePriority
	} else {
		g = remotePriority
		d = localPriority
	}

	// priority = 2^32*MIN(G,D) + 2*MAX(G,D) + (G>D?1:0)
	min := g
	max := d
	if d < g {
		min = d
		max = g
	}

	var tiebreaker uint64
	if g > d {
		tiebreaker = 1
	}

	return uint64(min)<<32 + uint64(max)*2 + tiebreaker
}

// NewCandidatePair creates a candidate pair.
func NewCandidatePair(local, remote *Candidate, controlling bool) *CandidatePair {
	return &CandidatePair{
		Local:    local,
		Remote:   remote,
		Priority: ComputePairPriority(controlling, local.Priority, remote.Priority),
		State:    CandidatePairFrozen,
	}
}

// SortCandidatePairs sorts pairs by priority (highest first).
func SortCandidatePairs(pairs []*CandidatePair) {
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Priority > pairs[j].Priority
	})
}
