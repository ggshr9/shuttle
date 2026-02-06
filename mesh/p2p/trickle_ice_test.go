package p2p

import (
	"net"
	"sync"
	"testing"
	"time"
)

func TestTrickleState_String(t *testing.T) {
	tests := []struct {
		state    TrickleState
		expected string
	}{
		{TrickleStateNew, "new"},
		{TrickleStateGathering, "gathering"},
		{TrickleStateComplete, "complete"},
		{TrickleState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("TrickleState.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNewTrickleICEGatherer(t *testing.T) {
	gatherer := NewTrickleICEGatherer(nil) // Use defaults

	if gatherer == nil {
		t.Fatal("NewTrickleICEGatherer returned nil")
	}

	if gatherer.state != TrickleStateNew {
		t.Errorf("state = %v, want %v", gatherer.state, TrickleStateNew)
	}

	if len(gatherer.stunServers) == 0 {
		t.Error("stunServers should not be empty")
	}

	gatherer.Close()
}

func TestNewTrickleICEGathererWithConfig(t *testing.T) {
	cfg := &TrickleGathererConfig{
		STUNServers: []string{"stun.example.com:3478"},
		Timeout:     10 * time.Second,
	}

	gatherer := NewTrickleICEGatherer(cfg)

	if len(gatherer.stunServers) != 1 {
		t.Errorf("stunServers length = %d, want 1", len(gatherer.stunServers))
	}

	if gatherer.timeout != 10*time.Second {
		t.Errorf("timeout = %v, want %v", gatherer.timeout, 10*time.Second)
	}

	gatherer.Close()
}

func TestTrickleICEGatherer_GetState(t *testing.T) {
	gatherer := NewTrickleICEGatherer(nil)
	defer gatherer.Close()

	state := gatherer.GetState()
	if state != TrickleStateNew {
		t.Errorf("GetState = %v, want %v", state, TrickleStateNew)
	}
}

func TestTrickleICEGatherer_GetCandidates(t *testing.T) {
	gatherer := NewTrickleICEGatherer(nil)
	defer gatherer.Close()

	candidates := gatherer.GetCandidates()
	if len(candidates) != 0 {
		t.Errorf("GetCandidates length = %d, want 0", len(candidates))
	}
}

func TestTrickleICEGatherer_Callbacks(t *testing.T) {
	gatherer := NewTrickleICEGatherer(nil)
	defer gatherer.Close()

	candidateCalled := false
	completeCalled := false

	gatherer.OnCandidate(func(c *Candidate) {
		candidateCalled = true
	})

	gatherer.OnGatheringComplete(func() {
		completeCalled = true
	})

	// Verify callbacks are set
	gatherer.mu.RLock()
	if gatherer.onCandidate == nil {
		t.Error("onCandidate callback not set")
	}
	if gatherer.onGatheringComplete == nil {
		t.Error("onGatheringComplete callback not set")
	}
	gatherer.mu.RUnlock()

	// Suppress unused variable warnings
	_ = candidateCalled
	_ = completeCalled
}

func TestTrickleICEGatherer_DoubleGather(t *testing.T) {
	gatherer := NewTrickleICEGatherer(nil)
	defer gatherer.Close()

	// Start gathering
	err := gatherer.Gather()
	if err != nil {
		t.Fatalf("First Gather failed: %v", err)
	}

	// Second gather should fail
	err = gatherer.Gather()
	if err == nil {
		t.Error("Second Gather should fail")
	}
}

func TestTrickleICEGatherer_Stop(t *testing.T) {
	gatherer := NewTrickleICEGatherer(nil)

	gatherer.Stop()

	// Should not panic
	gatherer.Stop()

	gatherer.Close()
}

func TestTrickleICEGatherer_Close(t *testing.T) {
	gatherer := NewTrickleICEGatherer(nil)

	err := gatherer.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Should be idempotent
	err = gatherer.Close()
	if err != nil {
		t.Errorf("Second Close failed: %v", err)
	}
}

func TestTrickleICEGatherer_GatherHostCandidates(t *testing.T) {
	gatherer := NewTrickleICEGatherer(nil)
	defer gatherer.Close()

	var mu sync.Mutex
	var candidates []*Candidate

	gatherer.OnCandidate(func(c *Candidate) {
		mu.Lock()
		candidates = append(candidates, c)
		mu.Unlock()
	})

	completeChan := make(chan struct{})
	gatherer.OnGatheringComplete(func() {
		close(completeChan)
	})

	err := gatherer.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	// Wait for gathering to complete with timeout
	select {
	case <-completeChan:
		// Good
	case <-time.After(10 * time.Second):
		t.Fatal("Gathering timed out")
	}

	mu.Lock()
	candidateCount := len(candidates)
	mu.Unlock()

	// Should have at least one host candidate
	if candidateCount == 0 {
		t.Error("Expected at least one candidate")
	}

	t.Logf("Gathered %d candidates via trickle", candidateCount)
}

func TestTrickleICEGatherer_GetLocalConn(t *testing.T) {
	gatherer := NewTrickleICEGatherer(nil)
	defer gatherer.Close()

	// Before gathering, should be nil
	conn := gatherer.GetLocalConn()
	if conn != nil {
		t.Error("GetLocalConn should be nil before gathering")
	}

	completeChan := make(chan struct{})
	gatherer.OnGatheringComplete(func() {
		close(completeChan)
	})

	err := gatherer.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	<-completeChan

	// After gathering, should have connection
	conn = gatherer.GetLocalConn()
	if conn == nil {
		t.Error("GetLocalConn should not be nil after gathering")
	}
}

func TestTrickleICEGatherer_GatherWithConnection(t *testing.T) {
	// Create a connection first
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("Failed to create UDP conn: %v", err)
	}
	defer conn.Close()

	gatherer := NewTrickleICEGatherer(nil)
	defer gatherer.Close()

	var mu sync.Mutex
	var candidates []*Candidate

	gatherer.OnCandidate(func(c *Candidate) {
		mu.Lock()
		candidates = append(candidates, c)
		mu.Unlock()
	})

	completeChan := make(chan struct{})
	gatherer.OnGatheringComplete(func() {
		close(completeChan)
	})

	err = gatherer.GatherWithConnection(conn)
	if err != nil {
		t.Fatalf("GatherWithConnection failed: %v", err)
	}

	<-completeChan

	// Verify the connection is the one we provided
	if gatherer.GetLocalConn() != conn {
		t.Error("GetLocalConn should return the provided connection")
	}
}

func TestBuildSTUNBindingRequest(t *testing.T) {
	req := buildSTUNBindingRequest()

	if len(req) != 20 {
		t.Errorf("Request length = %d, want 20", len(req))
	}

	// Check message type (Binding Request: 0x0001)
	if req[0] != 0x00 || req[1] != 0x01 {
		t.Errorf("Message type = 0x%02x%02x, want 0x0001", req[0], req[1])
	}

	// Check message length (0)
	if req[2] != 0x00 || req[3] != 0x00 {
		t.Errorf("Message length = 0x%02x%02x, want 0x0000", req[2], req[3])
	}

	// Check magic cookie
	if req[4] != 0x21 || req[5] != 0x12 || req[6] != 0xA4 || req[7] != 0x42 {
		t.Error("Invalid magic cookie")
	}
}

func TestParseSTUNBindingResponse_TooShort(t *testing.T) {
	_, err := parseSTUNBindingResponse([]byte{1, 2, 3})
	if err == nil {
		t.Error("Expected error for short response")
	}
}

func TestParseSTUNBindingResponse_WrongType(t *testing.T) {
	// Build a response with wrong type
	resp := make([]byte, 20)
	resp[0] = 0x00
	resp[1] = 0x02 // Wrong type

	_, err := parseSTUNBindingResponse(resp)
	if err == nil {
		t.Error("Expected error for wrong message type")
	}
}

func TestParseSTUNBindingResponse_InvalidCookie(t *testing.T) {
	resp := make([]byte, 20)
	resp[0] = 0x01
	resp[1] = 0x01 // Correct type
	// Wrong magic cookie
	resp[4] = 0x00
	resp[5] = 0x00
	resp[6] = 0x00
	resp[7] = 0x00

	_, err := parseSTUNBindingResponse(resp)
	if err == nil {
		t.Error("Expected error for invalid magic cookie")
	}
}

func TestParseSTUNBindingResponse_ValidXorMappedAddress(t *testing.T) {
	// Build a valid response with XOR-MAPPED-ADDRESS
	// Header: Type (2) + Length (2) + Magic Cookie (4) + Transaction ID (12) = 20 bytes
	// XOR-MAPPED-ADDRESS: Type (2) + Length (2) + Reserved (1) + Family (1) + XOR-Port (2) + XOR-IP (4) = 12 bytes
	resp := make([]byte, 32)

	// Message Type: Binding Success Response (0x0101)
	resp[0] = 0x01
	resp[1] = 0x01

	// Message Length: 12 bytes
	resp[2] = 0x00
	resp[3] = 0x0C

	// Magic Cookie
	resp[4] = 0x21
	resp[5] = 0x12
	resp[6] = 0xA4
	resp[7] = 0x42

	// Transaction ID (12 bytes, any values)
	for i := 8; i < 20; i++ {
		resp[i] = byte(i)
	}

	// XOR-MAPPED-ADDRESS attribute
	resp[20] = 0x00
	resp[21] = 0x20 // Type: XOR-MAPPED-ADDRESS
	resp[22] = 0x00
	resp[23] = 0x08 // Length: 8 bytes
	resp[24] = 0x00 // Reserved
	resp[25] = 0x01 // Family: IPv4
	// XOR-Port: 3000 (0x0BB8) ^ 0x2112 = 0x2AAA
	resp[26] = 0x2A
	resp[27] = 0xAA
	// XOR-IP: 192.168.1.1 ^ magic cookie
	// 192 (0xC0) ^ 0x21 = 0xE1
	// 168 (0xA8) ^ 0x12 = 0xBA
	// 1   (0x01) ^ 0xA4 = 0xA5
	// 1   (0x01) ^ 0x42 = 0x43
	resp[28] = 0xE1
	resp[29] = 0xBA
	resp[30] = 0xA5
	resp[31] = 0x43

	addr, err := parseSTUNBindingResponse(resp)
	if err != nil {
		t.Fatalf("parseSTUNBindingResponse failed: %v", err)
	}

	if addr.Port != 3000 {
		t.Errorf("Port = %d, want 3000", addr.Port)
	}

	expectedIP := net.ParseIP("192.168.1.1").To4()
	if !addr.IP.Equal(expectedIP) {
		t.Errorf("IP = %v, want %v", addr.IP, expectedIP)
	}
}

func TestIsEndOfCandidates(t *testing.T) {
	// nil candidate indicates end-of-candidates
	if !IsEndOfCandidates(nil) {
		t.Error("nil should be end-of-candidates")
	}

	// Non-nil candidate is not end-of-candidates
	candidate := &Candidate{
		Addr: &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 10000},
	}
	if IsEndOfCandidates(candidate) {
		t.Error("Non-nil candidate should not be end-of-candidates")
	}
}

// Test ICEAgent Trickle mode
func TestICEAgent_TrickleEnabled(t *testing.T) {
	cfg := &ICEAgentConfig{
		TrickleEnabled: true,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	if !agent.IsTrickleEnabled() {
		t.Error("IsTrickleEnabled should be true")
	}

	if agent.trickleGatherer == nil {
		t.Error("trickleGatherer should be initialized")
	}
}

func TestICEAgent_TrickleDisabled(t *testing.T) {
	cfg := &ICEAgentConfig{
		TrickleEnabled: false,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	if agent.IsTrickleEnabled() {
		t.Error("IsTrickleEnabled should be false")
	}
}

func TestICEAgent_OnLocalCandidate(t *testing.T) {
	cfg := &ICEAgentConfig{
		TrickleEnabled: true,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	called := false
	agent.OnLocalCandidate(func(c *Candidate) {
		called = true
	})

	agent.mu.RLock()
	if agent.onLocalCandidate == nil {
		t.Error("onLocalCandidate callback not set")
	}
	agent.mu.RUnlock()

	_ = called
}

func TestICEAgent_OnEndOfCandidates(t *testing.T) {
	cfg := &ICEAgentConfig{
		TrickleEnabled: true,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	called := false
	agent.OnEndOfCandidates(func() {
		called = true
	})

	agent.mu.RLock()
	if agent.onEndOfCandidates == nil {
		t.Error("onEndOfCandidates callback not set")
	}
	agent.mu.RUnlock()

	_ = called
}

func TestICEAgent_GatherCandidatesTrickle_NotEnabled(t *testing.T) {
	cfg := &ICEAgentConfig{
		TrickleEnabled: false,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	err := agent.GatherCandidatesTrickle()
	if err == nil {
		t.Error("GatherCandidatesTrickle should fail when trickle not enabled")
	}
}

func TestICEAgent_StartConnectivityChecksTrickle_NotEnabled(t *testing.T) {
	cfg := &ICEAgentConfig{
		TrickleEnabled: false,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	err := agent.StartConnectivityChecksTrickle()
	if err == nil {
		t.Error("StartConnectivityChecksTrickle should fail when trickle not enabled")
	}
}

func TestICEAgent_AddRemoteCandidateTrickle(t *testing.T) {
	cfg := &ICEAgentConfig{
		TrickleEnabled: true,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	// Add a local candidate first
	agent.mu.Lock()
	agent.localCandidates = append(agent.localCandidates, &Candidate{
		Type: CandidateHost,
		Addr: &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 10000},
	})
	agent.mu.Unlock()

	// Add remote candidate via trickle
	remoteCandidate := &Candidate{
		Type: CandidateServerReflexive,
		Addr: &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 20000},
	}
	agent.AddRemoteCandidateTrickle(remoteCandidate)

	candidates := agent.GetRemoteCandidates()
	if len(candidates) != 1 {
		t.Errorf("remoteCandidates length = %d, want 1", len(candidates))
	}

	// Should have created candidate pairs
	pairs := agent.GetCandidatePairs()
	if len(pairs) != 1 {
		t.Errorf("candidatePairs length = %d, want 1", len(pairs))
	}
}

func TestICEAgent_AddRemoteCandidateTrickle_Duplicate(t *testing.T) {
	cfg := &ICEAgentConfig{
		TrickleEnabled: true,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	candidate := &Candidate{
		Type: CandidateHost,
		Addr: &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 10000},
	}

	agent.AddRemoteCandidateTrickle(candidate)
	agent.AddRemoteCandidateTrickle(candidate) // Duplicate

	candidates := agent.GetRemoteCandidates()
	if len(candidates) != 1 {
		t.Errorf("Duplicate should be ignored, got %d candidates", len(candidates))
	}
}

func TestICEAgent_SetRemoteGatheringDone(t *testing.T) {
	cfg := &ICEAgentConfig{
		TrickleEnabled: true,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	agent.mu.RLock()
	before := agent.remoteGatheringDone
	agent.mu.RUnlock()

	if before {
		t.Error("remoteGatheringDone should be false initially")
	}

	agent.SetRemoteGatheringDone()

	agent.mu.RLock()
	after := agent.remoteGatheringDone
	agent.mu.RUnlock()

	if !after {
		t.Error("remoteGatheringDone should be true after SetRemoteGatheringDone")
	}
}
