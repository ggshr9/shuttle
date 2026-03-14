package p2p

import (
	"net"
	"sync"
	"testing"
	"time"
)

// noSTUNAgent returns an ICEAgentConfig with empty STUN servers
// to prevent any external network access during tests.
func noSTUNAgent() *ICEAgentConfig {
	return &ICEAgentConfig{
		STUNServers:      []string{},
		IsControlling:    true,
		GatherTimeout:    5 * time.Second,
		CheckTimeout:     30 * time.Second,
		RestartCooldown:  10 * time.Second,
		QualityThreshold: 30.0,
	}
}

func TestICEGatheringState_String(t *testing.T) {
	tests := []struct {
		state    ICEGatheringState
		expected string
	}{
		{ICEGatheringNew, "new"},
		{ICEGatheringGathering, "gathering"},
		{ICEGatheringComplete, "complete"},
		{ICEGatheringState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("ICEGatheringState.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestICEConnectionState_String(t *testing.T) {
	tests := []struct {
		state    ICEConnectionState
		expected string
	}{
		{ICEConnectionNew, "new"},
		{ICEConnectionChecking, "checking"},
		{ICEConnectionConnected, "connected"},
		{ICEConnectionCompleted, "completed"},
		{ICEConnectionFailed, "failed"},
		{ICEConnectionDisconnected, "disconnected"},
		{ICEConnectionClosed, "closed"},
		{ICEConnectionState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("ICEConnectionState.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestICERestartReason_String(t *testing.T) {
	tests := []struct {
		reason   ICERestartReason
		expected string
	}{
		{ICERestartReasonManual, "manual"},
		{ICERestartReasonNetworkChange, "network_change"},
		{ICERestartReasonQualityDegraded, "quality_degraded"},
		{ICERestartReasonAllPairsFailed, "all_pairs_failed"},
		{ICERestartReasonTimeout, "timeout"},
		{ICERestartReasonRemoteRequest, "remote_request"},
		{ICERestartReason(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.reason.String(); got != tt.expected {
				t.Errorf("ICERestartReason.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNewICECredentials(t *testing.T) {
	creds := NewICECredentials()

	if creds == nil {
		t.Fatal("NewICECredentials returned nil")
	}

	if len(creds.UsernameFragment) < 4 {
		t.Errorf("UsernameFragment too short: %d chars", len(creds.UsernameFragment))
	}

	if len(creds.Password) < 22 {
		t.Errorf("Password too short: %d chars", len(creds.Password))
	}

	if creds.Generation != 0 {
		t.Errorf("Generation = %d, want 0", creds.Generation)
	}
}

func TestICECredentials_Regenerate(t *testing.T) {
	creds := NewICECredentials()
	oldUfrag := creds.UsernameFragment
	oldPwd := creds.Password
	oldGen := creds.Generation

	creds.Regenerate()

	if creds.UsernameFragment == oldUfrag {
		t.Error("UsernameFragment should change after regeneration")
	}

	if creds.Password == oldPwd {
		t.Error("Password should change after regeneration")
	}

	if creds.Generation != oldGen+1 {
		t.Errorf("Generation = %d, want %d", creds.Generation, oldGen+1)
	}
}

func TestGenerateICEString(t *testing.T) {
	tests := []int{4, 8, 16, 24}

	for _, length := range tests {
		t.Run(string(rune('0'+length)), func(t *testing.T) {
			s := generateICEString(length)
			if len(s) != length {
				t.Errorf("generateICEString(%d) returned string of length %d", length, len(s))
			}
		})
	}

	// Test uniqueness
	s1 := generateICEString(16)
	s2 := generateICEString(16)
	if s1 == s2 {
		t.Error("generateICEString should produce unique strings")
	}
}

func TestDefaultICEAgentConfig(t *testing.T) {
	cfg := DefaultICEAgentConfig()

	if cfg == nil {
		t.Fatal("DefaultICEAgentConfig returned nil")
	}

	// STUNServers may be empty when SHUTTLE_TEST_NO_EXTERNAL is set
	if cfg.STUNServers == nil {
		t.Error("STUNServers should not be nil (may be empty slice)")
	}

	if cfg.GatherTimeout == 0 {
		t.Error("GatherTimeout should not be zero")
	}

	if cfg.CheckTimeout == 0 {
		t.Error("CheckTimeout should not be zero")
	}

	if cfg.RestartCooldown == 0 {
		t.Error("RestartCooldown should not be zero")
	}
}

func TestNewICEAgent(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent()) // Use default config

	if agent == nil {
		t.Fatal("NewICEAgent returned nil")
	}

	if agent.localCredentials == nil {
		t.Error("localCredentials should not be nil")
	}

	if agent.gatheringState != ICEGatheringNew {
		t.Errorf("gatheringState = %v, want %v", agent.gatheringState, ICEGatheringNew)
	}

	if agent.connectionState != ICEConnectionNew {
		t.Errorf("connectionState = %v, want %v", agent.connectionState, ICEConnectionNew)
	}

	if agent.done == nil {
		t.Error("done channel should not be nil")
	}

	agent.Close()
}

func TestNewICEAgentWithConfig(t *testing.T) {
	cfg := &ICEAgentConfig{
		STUNServers:      []string{}, // no external STUN in tests
		IsControlling:    false,
		GatherTimeout:    10 * time.Second,
		CheckTimeout:     60 * time.Second,
		RestartCooldown:  30 * time.Second,
		QualityThreshold: 50.0,
	}

	agent := NewICEAgent(cfg)

	if agent.isControlling != false {
		t.Error("isControlling should be false")
	}

	if agent.restartCooldown != 30*time.Second {
		t.Errorf("restartCooldown = %v, want %v", agent.restartCooldown, 30*time.Second)
	}

	if agent.qualityThreshold != 50.0 {
		t.Errorf("qualityThreshold = %v, want %v", agent.qualityThreshold, 50.0)
	}

	agent.Close()
}

func TestICEAgent_GetLocalCredentials(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	creds := agent.GetLocalCredentials()

	if creds == nil {
		t.Fatal("GetLocalCredentials returned nil")
	}

	if creds.UsernameFragment == "" {
		t.Error("UsernameFragment should not be empty")
	}
}

func TestICEAgent_SetRemoteCredentials(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	remoteCreds := &ICECredentials{
		UsernameFragment: "remote_ufrag",
		Password:         "remote_password_123456",
		Generation:       1,
	}

	agent.SetRemoteCredentials(remoteCreds)

	agent.mu.RLock()
	defer agent.mu.RUnlock()

	if agent.remoteCredentials != remoteCreds {
		t.Error("remoteCredentials not set correctly")
	}
}

func TestICEAgent_GetGatheringState(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	state := agent.GetGatheringState()

	if state != ICEGatheringNew {
		t.Errorf("GetGatheringState = %v, want %v", state, ICEGatheringNew)
	}
}

func TestICEAgent_GetConnectionState(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	state := agent.GetConnectionState()

	if state != ICEConnectionNew {
		t.Errorf("GetConnectionState = %v, want %v", state, ICEConnectionNew)
	}
}

func TestICEAgent_AddRemoteCandidate(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	// First add a local candidate (needed for pairing)
	localCandidate := &Candidate{
		Type: CandidateHost,
		Addr: &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 10000},
	}
	agent.mu.Lock()
	agent.localCandidates = append(agent.localCandidates, localCandidate)
	agent.mu.Unlock()

	// Add remote candidate
	remoteCandidate := &Candidate{
		Type: CandidateServerReflexive,
		Addr: &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 20000},
	}
	agent.AddRemoteCandidate(remoteCandidate)

	candidates := agent.GetRemoteCandidates()
	if len(candidates) != 1 {
		t.Errorf("remoteCandidates length = %d, want 1", len(candidates))
	}

	// Test duplicate detection
	agent.AddRemoteCandidate(remoteCandidate)
	candidates = agent.GetRemoteCandidates()
	if len(candidates) != 1 {
		t.Errorf("Duplicate candidate was added, length = %d, want 1", len(candidates))
	}

	// Check candidate pairs were created
	pairs := agent.GetCandidatePairs()
	if len(pairs) != 1 {
		t.Errorf("candidatePairs length = %d, want 1", len(pairs))
	}
}

func TestICEAgent_RestartCooldown(t *testing.T) {
	cfg := &ICEAgentConfig{
		STUNServers:     []string{},
		RestartCooldown: 100 * time.Millisecond,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	// First restart should succeed (set lastRestart to now - cooldown to simulate past restart)
	agent.mu.Lock()
	agent.lastRestart = time.Now().Add(-200 * time.Millisecond)
	agent.mu.Unlock()

	err := agent.Restart(ICERestartReasonManual)
	if err != nil {
		t.Errorf("First restart should succeed: %v", err)
	}

	// Immediate second restart should fail (cooldown)
	err = agent.Restart(ICERestartReasonManual)
	if err == nil {
		t.Error("Second restart should fail due to cooldown")
	}

	// Wait for cooldown
	time.Sleep(150 * time.Millisecond)

	// Third restart should succeed
	err = agent.Restart(ICERestartReasonManual)
	if err != nil {
		t.Errorf("Third restart should succeed: %v", err)
	}
}

func TestICEAgent_RestartCount(t *testing.T) {
	cfg := &ICEAgentConfig{
		STUNServers:     []string{},
		RestartCooldown: 1 * time.Millisecond,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	if agent.GetRestartCount() != 0 {
		t.Errorf("Initial restart count = %d, want 0", agent.GetRestartCount())
	}

	agent.Restart(ICERestartReasonManual)
	time.Sleep(5 * time.Millisecond)

	if agent.GetRestartCount() != 1 {
		t.Errorf("After first restart, count = %d, want 1", agent.GetRestartCount())
	}

	agent.Restart(ICERestartReasonNetworkChange)
	time.Sleep(5 * time.Millisecond)

	if agent.GetRestartCount() != 2 {
		t.Errorf("After second restart, count = %d, want 2", agent.GetRestartCount())
	}
}

func TestICEAgent_NeedsRestart(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	// In new state, no restart needed
	needs, _ := agent.NeedsRestart()
	if needs {
		t.Error("Should not need restart in new state")
	}

	// Set to failed state
	agent.mu.Lock()
	agent.connectionState = ICEConnectionFailed
	agent.mu.Unlock()

	needs, reason := agent.NeedsRestart()
	if !needs {
		t.Error("Should need restart in failed state")
	}
	if reason != ICERestartReasonAllPairsFailed {
		t.Errorf("Reason = %v, want %v", reason, ICERestartReasonAllPairsFailed)
	}

	// Set to disconnected state
	agent.mu.Lock()
	agent.connectionState = ICEConnectionDisconnected
	agent.mu.Unlock()

	needs, reason = agent.NeedsRestart()
	if !needs {
		t.Error("Should need restart in disconnected state")
	}
	if reason != ICERestartReasonTimeout {
		t.Errorf("Reason = %v, want %v", reason, ICERestartReasonTimeout)
	}
}

func TestICEAgent_Callbacks(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	agent.OnStateChange(func(state ICEConnectionState) {
		_ = state // Use parameter
	})

	agent.OnGatheringComplete(func(candidates []*Candidate) {
		_ = candidates // Use parameter
	})

	agent.OnSelectedPair(func(pair *CandidatePair) {
		_ = pair // Use parameter
	})

	agent.OnRestartNeeded(func(reason ICERestartReason) {
		_ = reason // Use parameter
	})

	// Verify callbacks are set
	agent.mu.RLock()
	if agent.onStateChange == nil {
		t.Error("onStateChange callback not set")
	}
	if agent.onGatheringComplete == nil {
		t.Error("onGatheringComplete callback not set")
	}
	if agent.onSelectedPair == nil {
		t.Error("onSelectedPair callback not set")
	}
	if agent.onRestartNeeded == nil {
		t.Error("onRestartNeeded callback not set")
	}
	agent.mu.RUnlock()
}

func TestICEAgent_RecordRTT(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	// Should not panic
	agent.RecordRTT(100 * time.Millisecond)
	agent.RecordRTT(50 * time.Millisecond)
}

func TestICEAgent_RecordPacketLoss(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	// Should not panic
	agent.RecordPacketLoss()
	agent.RecordPacketLoss()
}

func TestICEAgent_GetSelectedPair(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	// Initially nil
	pair := agent.GetSelectedPair()
	if pair != nil {
		t.Error("GetSelectedPair should return nil initially")
	}

	// Set a pair
	testPair := &CandidatePair{
		Local: &Candidate{
			Addr: &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 10000},
		},
		Remote: &Candidate{
			Addr: &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 20000},
		},
	}

	agent.mu.Lock()
	agent.selectedPair = testPair
	agent.mu.Unlock()

	pair = agent.GetSelectedPair()
	if pair != testPair {
		t.Error("GetSelectedPair returned wrong pair")
	}
}

func TestICEAgent_GetConnection(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	// Initially nil
	conn := agent.GetConnection()
	if conn != nil {
		t.Error("GetConnection should return nil initially")
	}
}

func TestICEAgent_GetLocalCandidates(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	// Initially empty
	candidates := agent.GetLocalCandidates()
	if len(candidates) != 0 {
		t.Errorf("GetLocalCandidates = %d, want 0", len(candidates))
	}

	// Add some candidates
	agent.mu.Lock()
	agent.localCandidates = []*Candidate{
		{Addr: &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 10000}},
		{Addr: &net.UDPAddr{IP: net.ParseIP("192.168.1.2"), Port: 10001}},
	}
	agent.mu.Unlock()

	candidates = agent.GetLocalCandidates()
	if len(candidates) != 2 {
		t.Errorf("GetLocalCandidates = %d, want 2", len(candidates))
	}
}

func TestICEAgent_GetQualityMetrics(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	metrics := agent.GetQualityMetrics()
	// Should return empty metrics (not panic)
	_ = metrics
}

func TestICEAgent_Close(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())

	// First close should succeed
	err := agent.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify state
	state := agent.GetConnectionState()
	if state != ICEConnectionClosed {
		t.Errorf("connectionState = %v, want %v", state, ICEConnectionClosed)
	}

	// Second close should be idempotent
	err = agent.Close()
	if err != nil {
		t.Errorf("Second close failed: %v", err)
	}
}

func TestICEAgent_CloseWithConnection(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())

	// Create a mock connection
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("Failed to create UDP conn: %v", err)
	}

	agent.mu.Lock()
	agent.conn = conn
	agent.mu.Unlock()

	// Close should also close the connection
	err = agent.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Connection should be closed (write should fail)
	_, writeErr := conn.Write([]byte("test"))
	if writeErr == nil {
		t.Error("Connection should be closed")
	}
}

func TestICEAgent_HandleNetworkChange(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	restartReason := ICERestartReason(-1)
	agent.OnRestartNeeded(func(reason ICERestartReason) {
		restartReason = reason
	})

	// In new state, network change should be ignored
	agent.HandleNetworkChange()
	time.Sleep(10 * time.Millisecond)
	if restartReason != ICERestartReason(-1) {
		t.Error("Network change in new state should not trigger restart")
	}

	// In connected state, should trigger restart
	agent.mu.Lock()
	agent.connectionState = ICEConnectionConnected
	agent.selectedPair = &CandidatePair{}
	agent.mu.Unlock()

	agent.HandleNetworkChange()
	time.Sleep(10 * time.Millisecond)

	if restartReason != ICERestartReasonNetworkChange {
		t.Errorf("Restart reason = %v, want %v", restartReason, ICERestartReasonNetworkChange)
	}

	// State should be disconnected
	state := agent.GetConnectionState()
	if state != ICEConnectionDisconnected {
		t.Errorf("Connection state = %v, want %v", state, ICEConnectionDisconnected)
	}
}

func TestICEAgent_PendingRestart(t *testing.T) {
	cfg := &ICEAgentConfig{
		STUNServers:     []string{},
		RestartCooldown: 1 * time.Millisecond,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	// Simulate pending restart
	agent.mu.Lock()
	agent.pendingRestart = true
	agent.mu.Unlock()

	err := agent.Restart(ICERestartReasonManual)
	if err == nil {
		t.Error("Restart should fail when pending restart is true")
	}
}

func TestICEAgent_CredentialsRegeneration(t *testing.T) {
	cfg := &ICEAgentConfig{
		STUNServers:     []string{},
		RestartCooldown: 1 * time.Millisecond,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	oldUfrag := agent.localCredentials.UsernameFragment
	oldGen := agent.localCredentials.Generation

	agent.Restart(ICERestartReasonManual)
	time.Sleep(5 * time.Millisecond)

	// Credentials should be regenerated
	creds := agent.GetLocalCredentials()
	if creds.UsernameFragment == oldUfrag {
		t.Error("UsernameFragment should change after restart")
	}
	if creds.Generation != oldGen+1 {
		t.Errorf("Generation = %d, want %d", creds.Generation, oldGen+1)
	}
}

func TestICEAgent_StartConnectivityChecksInvalidState(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	// Set invalid state
	agent.mu.Lock()
	agent.connectionState = ICEConnectionConnected
	agent.mu.Unlock()

	err := agent.StartConnectivityChecks()
	if err == nil {
		t.Error("StartConnectivityChecks should fail in connected state")
	}
}

func TestICEAgent_StartConnectivityChecksNoConnection(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	// conn is nil
	err := agent.StartConnectivityChecks()
	if err == nil {
		t.Error("StartConnectivityChecks should fail without connection")
	}
}

func TestICEAgent_StateChangeCallback(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	var mu sync.Mutex
	stateChanges := make([]ICEConnectionState, 0)
	agent.OnStateChange(func(state ICEConnectionState) {
		mu.Lock()
		stateChanges = append(stateChanges, state)
		mu.Unlock()
	})

	// Trigger state change
	agent.mu.Lock()
	agent.setConnectionStateLocked(ICEConnectionChecking)
	agent.mu.Unlock()

	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(stateChanges) != 1 {
		t.Errorf("State changes = %d, want 1", len(stateChanges))
	}

	if stateChanges[0] != ICEConnectionChecking {
		t.Errorf("State = %v, want %v", stateChanges[0], ICEConnectionChecking)
	}
}

func TestICEAgent_CandidatePairsSorted(t *testing.T) {
	agent := NewICEAgent(noSTUNAgent())
	defer agent.Close()

	// Add local candidates with different priorities
	agent.mu.Lock()
	agent.localCandidates = []*Candidate{
		{Type: CandidateHost, Addr: &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 10000}, Priority: 126},
		{Type: CandidateServerReflexive, Addr: &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 10001}, Priority: 100},
	}
	agent.mu.Unlock()

	// Add remote candidates
	agent.AddRemoteCandidate(&Candidate{
		Type:     CandidateHost,
		Addr:     &net.UDPAddr{IP: net.ParseIP("192.168.2.1"), Port: 20000},
		Priority: 126,
	})

	// Pairs should be sorted by priority
	pairs := agent.GetCandidatePairs()
	if len(pairs) != 2 {
		t.Fatalf("Expected 2 pairs, got %d", len(pairs))
	}

	// Higher priority pair should be first
	if pairs[0].Priority < pairs[1].Priority {
		t.Error("Pairs should be sorted by priority (highest first)")
	}
}

// Test ICE Restart integration with manager
func TestICERestartStats(t *testing.T) {
	stats := &ICERestartStats{
		Generation:     3,
		RestartCount:   5,
		LastRestart:    time.Now().Add(-30 * time.Second),
		CooldownActive: false,
	}

	if stats.Generation != 3 {
		t.Errorf("Generation = %d, want 3", stats.Generation)
	}

	if stats.RestartCount != 5 {
		t.Errorf("RestartCount = %d, want 5", stats.RestartCount)
	}

	if stats.CooldownActive {
		t.Error("CooldownActive should be false")
	}
}

func TestICECredentialsMultipleRegenerate(t *testing.T) {
	creds := NewICECredentials()

	// Track all generations
	generations := make(map[string]bool)
	generations[creds.UsernameFragment] = true

	// Regenerate multiple times
	for i := 0; i < 10; i++ {
		creds.Regenerate()
		if generations[creds.UsernameFragment] {
			t.Error("UsernameFragment should be unique across regenerations")
		}
		generations[creds.UsernameFragment] = true

		if creds.Generation != i+1 {
			t.Errorf("Generation = %d, want %d", creds.Generation, i+1)
		}
	}
}

func TestICERestartReasonAllValues(t *testing.T) {
	reasons := []ICERestartReason{
		ICERestartReasonManual,
		ICERestartReasonNetworkChange,
		ICERestartReasonQualityDegraded,
		ICERestartReasonAllPairsFailed,
		ICERestartReasonTimeout,
		ICERestartReasonRemoteRequest,
	}

	// Verify all reasons have unique string representations
	seen := make(map[string]bool)
	for _, r := range reasons {
		s := r.String()
		if s == "unknown" {
			t.Errorf("Reason %d has unknown string", r)
		}
		if seen[s] {
			t.Errorf("Duplicate reason string: %s", s)
		}
		seen[s] = true
	}
}

func TestICEGatheringStateTransitions(t *testing.T) {
	// Test valid state transitions
	validTransitions := []struct {
		from ICEGatheringState
		to   ICEGatheringState
	}{
		{ICEGatheringNew, ICEGatheringGathering},
		{ICEGatheringGathering, ICEGatheringComplete},
		{ICEGatheringComplete, ICEGatheringNew}, // On restart
	}

	for _, tt := range validTransitions {
		t.Run(tt.from.String()+"->"+tt.to.String(), func(t *testing.T) {
			// Just verify the strings are valid
			if tt.from.String() == "unknown" || tt.to.String() == "unknown" {
				t.Error("Invalid state")
			}
		})
	}
}

func TestICEConnectionStateTransitions(t *testing.T) {
	// Test valid state transitions
	validTransitions := []struct {
		from ICEConnectionState
		to   ICEConnectionState
	}{
		{ICEConnectionNew, ICEConnectionChecking},
		{ICEConnectionChecking, ICEConnectionConnected},
		{ICEConnectionConnected, ICEConnectionCompleted},
		{ICEConnectionConnected, ICEConnectionDisconnected},
		{ICEConnectionChecking, ICEConnectionFailed},
		{ICEConnectionFailed, ICEConnectionNew}, // On restart
		{ICEConnectionDisconnected, ICEConnectionNew}, // On restart
	}

	for _, tt := range validTransitions {
		t.Run(tt.from.String()+"->"+tt.to.String(), func(t *testing.T) {
			// Just verify the strings are valid
			if tt.from.String() == "unknown" || tt.to.String() == "unknown" {
				t.Error("Invalid state")
			}
		})
	}
}

func TestICEAgentMultipleRestarts(t *testing.T) {
	cfg := &ICEAgentConfig{
		STUNServers:     []string{},
		RestartCooldown: 1 * time.Millisecond,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	// Perform multiple restarts
	for i := 0; i < 5; i++ {
		err := agent.Restart(ICERestartReasonManual)
		if err != nil {
			t.Errorf("Restart %d failed: %v", i, err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Verify restart count
	count := agent.GetRestartCount()
	if count != 5 {
		t.Errorf("RestartCount = %d, want 5", count)
	}

	// Verify credential generation incremented
	creds := agent.GetLocalCredentials()
	if creds.Generation != 5 {
		t.Errorf("Generation = %d, want 5", creds.Generation)
	}
}

func TestICEAgentQualityBasedRestart(t *testing.T) {
	cfg := &ICEAgentConfig{
		STUNServers:      []string{},
		QualityThreshold: 50.0,
	}
	agent := NewICEAgent(cfg)
	defer agent.Close()

	// Set up connection with poor quality
	agent.mu.Lock()
	agent.connectionState = ICEConnectionConnected
	agent.selectedPair = &CandidatePair{}
	// Simulate poor quality by recording high RTT
	// The quality score is reduced by RTT over 50ms
	// Each 50ms over 50ms costs 10 points
	// So 500ms RTT = (500-50)/50 * 10 = 90 point penalty = score of 10
	for i := 0; i < 10; i++ {
		agent.qualityMonitor.RecordRTT(500 * time.Millisecond)
	}
	agent.mu.Unlock()

	// Check if restart is needed
	needs, reason := agent.NeedsRestart()
	if !needs {
		t.Error("Should need restart due to poor quality")
	}
	if reason != ICERestartReasonQualityDegraded {
		t.Errorf("Reason = %v, want %v", reason, ICERestartReasonQualityDegraded)
	}
}
