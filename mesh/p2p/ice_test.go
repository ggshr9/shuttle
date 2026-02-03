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
