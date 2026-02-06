package p2p

import (
	"net"
	"testing"
)

func TestCandidateType(t *testing.T) {
	tests := []struct {
		typ      CandidateType
		str      string
		priority int
	}{
		{CandidateHost, "host", 126},
		{CandidateServerReflexive, "srflx", 100},
		{CandidatePeerReflexive, "prflx", 110},
		{CandidateRelay, "relay", 0},
	}

	for _, tt := range tests {
		if tt.typ.String() != tt.str {
			t.Errorf("CandidateType(%d).String() = %q, want %q", tt.typ, tt.typ.String(), tt.str)
		}
		if tt.typ.Priority() != tt.priority {
			t.Errorf("CandidateType(%d).Priority() = %d, want %d", tt.typ, tt.typ.Priority(), tt.priority)
		}
	}
}

func TestNewCandidate(t *testing.T) {
	addr := &net.UDPAddr{
		IP:   net.IPv4(192, 168, 1, 1),
		Port: 12345,
	}

	c := NewCandidate(CandidateHost, addr)

	if c.Type != CandidateHost {
		t.Errorf("expected CandidateHost, got %v", c.Type)
	}
	if !c.Addr.IP.Equal(addr.IP) {
		t.Errorf("expected IP %v, got %v", addr.IP, c.Addr.IP)
	}
	if c.Addr.Port != 12345 {
		t.Errorf("expected port 12345, got %d", c.Addr.Port)
	}
	if c.Priority == 0 {
		t.Error("expected non-zero priority")
	}
	if c.Foundation == "" {
		t.Error("expected non-empty foundation")
	}
}

func TestCandidatePriority(t *testing.T) {
	// Host candidates should have higher priority than srflx
	host := NewCandidate(CandidateHost, &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 1000})
	srflx := NewCandidate(CandidateServerReflexive, &net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 1000})

	if host.Priority <= srflx.Priority {
		t.Errorf("host priority (%d) should be > srflx priority (%d)", host.Priority, srflx.Priority)
	}
}

func TestComputePairPriority(t *testing.T) {
	localPri := uint32(126 << 24)
	remotePri := uint32(100 << 24)

	// As controlling
	pri1 := ComputePairPriority(true, localPri, remotePri)

	// As controlled
	pri2 := ComputePairPriority(false, localPri, remotePri)

	// Priorities should differ based on role
	if pri1 == pri2 {
		t.Error("priorities should differ based on controlling role")
	}

	// Higher local priority when controlling should give higher pair priority
	if pri1 <= pri2 {
		t.Errorf("controlling with higher local priority should give higher pair priority")
	}
}

func TestNewCandidatePair(t *testing.T) {
	local := NewCandidate(CandidateHost, &net.UDPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 1000})
	remote := NewCandidate(CandidateHost, &net.UDPAddr{IP: net.IPv4(192, 168, 2, 1), Port: 2000})

	pair := NewCandidatePair(local, remote, true)

	if pair.Local != local {
		t.Error("local candidate mismatch")
	}
	if pair.Remote != remote {
		t.Error("remote candidate mismatch")
	}
	if pair.State != CandidatePairFrozen {
		t.Errorf("expected initial state Frozen, got %v", pair.State)
	}
	if pair.Priority == 0 {
		t.Error("expected non-zero pair priority")
	}
}

func TestSortCandidatePairs(t *testing.T) {
	local1 := NewCandidate(CandidateHost, &net.UDPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 1000})
	local2 := NewCandidate(CandidateServerReflexive, &net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 1000})
	remote := NewCandidate(CandidateHost, &net.UDPAddr{IP: net.IPv4(192, 168, 2, 1), Port: 2000})

	pair1 := NewCandidatePair(local1, remote, true)
	pair2 := NewCandidatePair(local2, remote, true)

	pairs := []*CandidatePair{pair2, pair1} // Put lower priority first
	SortCandidatePairs(pairs)

	// After sorting, higher priority should be first
	if pairs[0].Priority < pairs[1].Priority {
		t.Error("pairs not sorted by priority (descending)")
	}
}

func TestICEGathererHostCandidates(t *testing.T) {
	gatherer := NewICEGatherer(nil, 0)
	candidates := gatherer.gatherHostCandidates(12345)

	// We should get at least one host candidate on most systems
	// (unless running in a very restricted environment)
	for _, c := range candidates {
		if c.Type != CandidateHost {
			t.Errorf("expected CandidateHost, got %v", c.Type)
		}
		if c.Addr.Port != 12345 {
			t.Errorf("expected port 12345, got %d", c.Addr.Port)
		}
		if c.Addr.IP.IsLoopback() {
			t.Error("loopback addresses should not be included")
		}
	}
}

func TestAddressFamily(t *testing.T) {
	tests := []struct {
		name   string
		ip     net.IP
		family AddressFamily
	}{
		{"IPv4", net.IPv4(192, 168, 1, 1), AddressFamilyIPv4},
		{"IPv6", net.ParseIP("2001:db8::1"), AddressFamilyIPv6},
		{"IPv4-mapped-IPv6", net.ParseIP("::ffff:192.168.1.1"), AddressFamilyIPv4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := &net.UDPAddr{IP: tt.ip, Port: 1234}
			c := NewCandidate(CandidateHost, addr)
			if c.Family != tt.family {
				t.Errorf("expected family %d, got %d", tt.family, c.Family)
			}
		})
	}
}

func TestCandidateIsIPv4IPv6(t *testing.T) {
	v4 := NewCandidate(CandidateHost, &net.UDPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 1234})
	v6 := NewCandidate(CandidateHost, &net.UDPAddr{IP: net.ParseIP("2001:db8::1"), Port: 1234})

	if !v4.IsIPv4() {
		t.Error("expected IPv4 candidate to return true for IsIPv4()")
	}
	if v4.IsIPv6() {
		t.Error("expected IPv4 candidate to return false for IsIPv6()")
	}

	if !v6.IsIPv6() {
		t.Error("expected IPv6 candidate to return true for IsIPv6()")
	}
	if v6.IsIPv4() {
		t.Error("expected IPv6 candidate to return false for IsIPv4()")
	}
}

func TestICEGathererHostCandidatesIPv6(t *testing.T) {
	gatherer := NewICEGatherer(nil, 0)
	candidates := gatherer.gatherHostCandidatesIPv6(12345)

	// IPv6 may or may not be available, so we just check the candidates we get
	for _, c := range candidates {
		if c.Type != CandidateHost {
			t.Errorf("expected CandidateHost, got %v", c.Type)
		}
		if c.Addr.Port != 12345 {
			t.Errorf("expected port 12345, got %d", c.Addr.Port)
		}
		if c.Addr.IP.IsLoopback() {
			t.Error("loopback addresses should not be included")
		}
		if c.Addr.IP.To4() != nil {
			t.Error("IPv4 address should not be included in IPv6 candidates")
		}
		if c.Addr.IP.IsLinkLocalUnicast() {
			t.Error("link-local addresses should not be included")
		}
		if !c.IsIPv6() {
			t.Error("expected IPv6 family")
		}
	}
}

func TestICEGathererHostCandidatesDualStack(t *testing.T) {
	gatherer := NewICEGatherer(nil, 0)
	candidates := gatherer.gatherHostCandidatesDualStack(12345, 12346)

	hasV4 := false
	hasV6 := false

	for _, c := range candidates {
		if c.Type != CandidateHost {
			t.Errorf("expected CandidateHost, got %v", c.Type)
		}
		if c.IsIPv4() {
			hasV4 = true
			if c.Addr.Port != 12345 {
				t.Errorf("expected IPv4 port 12345, got %d", c.Addr.Port)
			}
		}
		if c.IsIPv6() {
			hasV6 = true
			if c.Addr.Port != 12346 {
				t.Errorf("expected IPv6 port 12346, got %d", c.Addr.Port)
			}
		}
	}

	// We should at least have IPv4 on most systems
	if !hasV4 {
		t.Log("No IPv4 candidates found (unusual but possible in IPv6-only environments)")
	}
	// IPv6 may or may not be available
	if hasV6 {
		t.Log("IPv6 candidates found")
	}
}

func TestICEGathererGatherHostCandidatesFamily(t *testing.T) {
	gatherer := NewICEGatherer(nil, 0)

	// Test IPv4 family
	v4Candidates := gatherer.gatherHostCandidatesFamily(12345, AddressFamilyIPv4)
	for _, c := range v4Candidates {
		if c.Addr.IP.To4() == nil {
			t.Error("expected IPv4 address in IPv4 family")
		}
	}

	// Test IPv6 family
	v6Candidates := gatherer.gatherHostCandidatesFamily(12345, AddressFamilyIPv6)
	for _, c := range v6Candidates {
		if c.Addr.IP.To4() != nil {
			t.Error("expected IPv6 address in IPv6 family")
		}
	}
}
