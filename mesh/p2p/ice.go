package p2p

import (
	"context"
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
	CandidateUPnP                                 // UPnP mapped address (high priority)
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
	case CandidateUPnP:
		return "upnp"
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
	case CandidateUPnP:
		return 120 // Higher priority than STUN, we control the port
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

// AddressFamily represents the IP address family.
type AddressFamily int

const (
	AddressFamilyIPv4 AddressFamily = 4
	AddressFamilyIPv6 AddressFamily = 6
)

// Candidate represents an ICE candidate.
type Candidate struct {
	Type        CandidateType
	Addr        *net.UDPAddr
	Base        *net.UDPAddr // For srflx, the local address behind NAT
	Priority    uint32
	Foundation  string
	RelatedIP   net.IP
	RelatedPort int
	Family      AddressFamily // IPv4 or IPv6
}

// NewCandidate creates a new candidate.
func NewCandidate(typ CandidateType, addr *net.UDPAddr) *Candidate {
	c := &Candidate{
		Type: typ,
		Addr: addr,
	}
	// Determine address family
	if addr != nil && addr.IP != nil {
		if addr.IP.To4() != nil {
			c.Family = AddressFamilyIPv4
		} else {
			c.Family = AddressFamilyIPv6
		}
	}
	c.computePriority()
	c.computeFoundation()
	return c
}

// computePriority calculates the candidate priority per RFC 5245.
func (c *Candidate) computePriority() {
	// priority = (2^24)*(type preference) + (2^8)*(local preference) + (256 - component ID)
	typePreference := uint32(c.Type.Priority()) //nolint:gosec // G115: CandidateType priority is 0-126, fits uint32
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

// gatherHostCandidates gathers local IPv4 network addresses.
func (g *ICEGatherer) gatherHostCandidates(port int) []*Candidate {
	return g.gatherHostCandidatesFamily(port, AddressFamilyIPv4)
}

// gatherHostCandidatesIPv6 gathers local IPv6 network addresses.
func (g *ICEGatherer) gatherHostCandidatesIPv6(port int) []*Candidate {
	return g.gatherHostCandidatesFamily(port, AddressFamilyIPv6)
}

// gatherHostCandidatesFamily gathers local network addresses of the specified family.
func (g *ICEGatherer) gatherHostCandidatesFamily(port int, family AddressFamily) []*Candidate {
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

			// Filter by address family
			if family == AddressFamilyIPv4 {
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
				candidates = append(candidates, candidate)
			} else if family == AddressFamilyIPv6 {
				// Skip if this is actually IPv4
				if ip.To4() != nil {
					continue
				}
				// Skip loopback
				if ip.IsLoopback() {
					continue
				}
				// Skip link-local for IPv6 (fe80::/10)
				// These are not routable across networks
				if ip.IsLinkLocalUnicast() {
					continue
				}
				candidate := NewCandidate(CandidateHost, &net.UDPAddr{
					IP:   ip,
					Port: port,
				})
				candidates = append(candidates, candidate)
			}
		}
	}

	return candidates
}

// gatherHostCandidatesDualStack gathers both IPv4 and IPv6 host candidates.
func (g *ICEGatherer) gatherHostCandidatesDualStack(portV4, portV6 int) []*Candidate {
	candidates := make([]*Candidate, 0)
	candidates = append(candidates, g.gatherHostCandidatesFamily(portV4, AddressFamilyIPv4)...)
	candidates = append(candidates, g.gatherHostCandidatesFamily(portV6, AddressFamilyIPv6)...)
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

// DualStackGatherResult contains gathered candidates for both IPv4 and IPv6.
type DualStackGatherResult struct {
	Candidates  []*Candidate
	LocalConnV4 *net.UDPConn
	LocalConnV6 *net.UDPConn
	NATInfoV4   *NATInfo
	NATInfoV6   *NATInfo
}

// GatherDualStack collects both IPv4 and IPv6 ICE candidates.
func (g *ICEGatherer) GatherDualStack(ctx context.Context) (*DualStackGatherResult, error) {
	result := &DualStackGatherResult{
		Candidates: make([]*Candidate, 0),
	}

	// Create IPv4 UDP socket
	connV4, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("ice: listen ipv4: %w", err)
	}
	result.LocalConnV4 = connV4
	localAddrV4 := connV4.LocalAddr().(*net.UDPAddr)

	// Create IPv6 UDP socket
	connV6, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6zero, Port: 0})
	if err != nil {
		// IPv6 may not be available, continue with IPv4 only
		connV6 = nil
	} else {
		result.LocalConnV6 = connV6
	}

	// Gather IPv4 host candidates
	hostV4 := g.gatherHostCandidatesFamily(localAddrV4.Port, AddressFamilyIPv4)
	result.Candidates = append(result.Candidates, hostV4...)

	// Gather IPv6 host candidates
	if connV6 != nil {
		localAddrV6 := connV6.LocalAddr().(*net.UDPAddr)
		hostV6 := g.gatherHostCandidatesFamily(localAddrV6.Port, AddressFamilyIPv6)
		result.Candidates = append(result.Candidates, hostV6...)
	}

	// Gather server-reflexive candidates (IPv4)
	srflxV4, natInfoV4 := g.gatherServerReflexiveCandidates(connV4)
	result.Candidates = append(result.Candidates, srflxV4...)
	result.NATInfoV4 = natInfoV4

	// Gather server-reflexive candidates (IPv6)
	if connV6 != nil {
		srflxV6, natInfoV6 := g.gatherServerReflexiveCandidatesIPv6(ctx, connV6)
		result.Candidates = append(result.Candidates, srflxV6...)
		result.NATInfoV6 = natInfoV6
	}

	// Sort candidates by priority (highest first)
	sort.Slice(result.Candidates, func(i, j int) bool {
		return result.Candidates[i].Priority > result.Candidates[j].Priority
	})

	return result, nil
}

// gatherServerReflexiveCandidatesIPv6 gathers IPv6 STUN reflexive addresses.
func (g *ICEGatherer) gatherServerReflexiveCandidatesIPv6(ctx context.Context, conn *net.UDPConn) ([]*Candidate, *NATInfo) {
	candidates := make([]*Candidate, 0)

	if len(g.stunServers) == 0 {
		return candidates, nil
	}

	// Use IPv6 STUN servers
	stunClient := NewSTUNClient(g.stunServers, g.timeout)

	result, err := stunClient.QueryParallelIPv6(ctx)
	if err != nil {
		return candidates, nil
	}

	// Add server-reflexive candidate
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	if result.PublicAddr != nil && !result.PublicAddr.IP.Equal(localAddr.IP) {
		candidate := NewCandidate(CandidateServerReflexive, result.PublicAddr)
		candidate.Base = localAddr
		candidate.RelatedIP = localAddr.IP
		candidate.RelatedPort = localAddr.Port
		candidates = append(candidates, candidate)
	}

	// For IPv6, NAT is less common, but we still return info
	natInfo := &NATInfo{
		Type:       NATNone, // IPv6 typically doesn't have NAT
		PublicAddr: result.PublicAddr,
		LocalAddr:  localAddr,
	}

	// Check if addresses differ (indicating NAT66)
	if result.PublicAddr != nil && !result.PublicAddr.IP.Equal(localAddr.IP) {
		natInfo.Type = NATSymmetric // NAT66 behaves like symmetric NAT
	}

	return candidates, natInfo
}

// GatherIPv6 collects IPv6 ICE candidates only.
func (g *ICEGatherer) GatherIPv6(ctx context.Context) (*GatherResult, error) {
	result := &GatherResult{
		Candidates: make([]*Candidate, 0),
	}

	// Create IPv6 UDP socket
	conn, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("ice: listen ipv6: %w", err)
	}
	result.LocalConn = conn

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// Gather IPv6 host candidates
	hostCandidates := g.gatherHostCandidatesIPv6(localAddr.Port)
	result.Candidates = append(result.Candidates, hostCandidates...)

	// Gather server-reflexive candidates via STUN
	srflxCandidates, natInfo := g.gatherServerReflexiveCandidatesIPv6(ctx, conn)
	result.Candidates = append(result.Candidates, srflxCandidates...)
	result.NATInfo = natInfo

	// Sort candidates by priority (highest first)
	sort.Slice(result.Candidates, func(i, j int) bool {
		return result.Candidates[i].Priority > result.Candidates[j].Priority
	})

	return result, nil
}

// IsIPv6 returns true if the candidate is an IPv6 address.
func (c *Candidate) IsIPv6() bool {
	return c.Family == AddressFamilyIPv6
}

// IsIPv4 returns true if the candidate is an IPv4 address.
func (c *Candidate) IsIPv4() bool {
	return c.Family == AddressFamilyIPv4
}
