package p2p

import (
	"fmt"
	"net"
	"testing"
	"time"
)

// TestP2PConnectionLocal tests P2P connection between two local endpoints
func TestP2PConnectionLocal(t *testing.T) {
	// Create two UDP sockets simulating two peers, bound to localhost
	conn1, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("Failed to create conn1: %v", err)
	}
	defer conn1.Close()

	conn2, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("Failed to create conn2: %v", err)
	}
	defer conn2.Close()

	// Get addresses
	addr1 := conn1.LocalAddr().(*net.UDPAddr)
	addr2 := conn2.LocalAddr().(*net.UDPAddr)

	// Test bidirectional communication
	testData := []byte("hello from peer 1")

	// Send from conn1 to conn2
	_, err = conn1.WriteToUDP(testData, addr2)
	if err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	// Receive on conn2
	buf := make([]byte, 1500)
	conn2.SetReadDeadline(time.Now().Add(time.Second))
	n, from, err := conn2.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("Failed to receive: %v", err)
	}

	if string(buf[:n]) != string(testData) {
		t.Errorf("Data mismatch: got %q, want %q", buf[:n], testData)
	}

	// Verify source address matches conn1
	if from.Port != addr1.Port {
		t.Errorf("Source port mismatch: got %d, want %d", from.Port, addr1.Port)
	}
}

// TestHolePunchPacketExchange tests hole punch packet exchange
func TestHolePunchPacketExchange(t *testing.T) {
	// Create two UDP sockets
	conn1, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("Failed to create conn1: %v", err)
	}
	defer conn1.Close()

	conn2, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("Failed to create conn2: %v", err)
	}
	defer conn2.Close()

	addr1 := conn1.LocalAddr().(*net.UDPAddr)
	addr2 := conn2.LocalAddr().(*net.UDPAddr)

	// Use localhost addresses for reliable local communication
	addr1Local := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: addr1.Port}
	addr2Local := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: addr2.Port}

	vip1 := net.ParseIP("10.7.0.1")
	vip2 := net.ParseIP("10.7.0.2")

	// Create hole punch packets
	syn1 := &HolePunchPacket{
		Type:      HolePunchRequest,
		SrcVIP:    vip1,
		DstVIP:    vip2,
		Timestamp: time.Now().UnixNano(),
		Seq:       1,
	}

	syn2 := &HolePunchPacket{
		Type:      HolePunchRequest,
		SrcVIP:    vip2,
		DstVIP:    vip1,
		Timestamp: time.Now().UnixNano(),
		Seq:       2,
	}

	// Use channels for synchronization
	ready := make(chan struct{})
	errChan := make(chan error, 2)

	// Peer 1: wait for signal, then read
	go func() {
		<-ready // Wait for both sends to complete
		buf := make([]byte, 1500)
		conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _, err := conn1.ReadFromUDP(buf)
		if err != nil {
			errChan <- fmt.Errorf("peer 1 read: %w", err)
			return
		}

		pkt, err := DecodeHolePunchPacket(buf[:n])
		if err != nil {
			errChan <- fmt.Errorf("peer 1 decode: %w", err)
			return
		}

		if pkt.Type != HolePunchRequest {
			errChan <- fmt.Errorf("peer 1: expected Request, got %v", pkt.Type)
			return
		}
		errChan <- nil
	}()

	// Peer 2: wait for signal, then read
	go func() {
		<-ready // Wait for both sends to complete
		buf := make([]byte, 1500)
		conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _, err := conn2.ReadFromUDP(buf)
		if err != nil {
			errChan <- fmt.Errorf("peer 2 read: %w", err)
			return
		}

		pkt, err := DecodeHolePunchPacket(buf[:n])
		if err != nil {
			errChan <- fmt.Errorf("peer 2 decode: %w", err)
			return
		}

		if pkt.Type != HolePunchRequest {
			errChan <- fmt.Errorf("peer 2: expected Request, got %v", pkt.Type)
			return
		}
		errChan <- nil
	}()

	// Send packets first, then signal readers
	if _, err := conn1.WriteToUDP(syn1.Encode(), addr2Local); err != nil {
		t.Fatalf("conn1 send failed: %v", err)
	}
	if _, err := conn2.WriteToUDP(syn2.Encode(), addr1Local); err != nil {
		t.Fatalf("conn2 send failed: %v", err)
	}

	close(ready) // Signal readers to start

	// Collect results
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			t.Error(err)
		}
	}
}

// TestICEGathererIntegration tests full ICE candidate gathering
func TestICEGathererIntegration(t *testing.T) {
	gatherer := NewICEGatherer([]string{}, 5*time.Second) // no external STUN in tests

	result, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("Gather error: %v", err)
	}
	defer result.LocalConn.Close()

	// Should at least have host candidates
	hasHost := false
	for _, c := range result.Candidates {
		t.Logf("Candidate: type=%s addr=%s", c.Type, c.Addr)
		if c.Type == CandidateHost {
			hasHost = true
		}
	}

	if !hasHost && len(result.Candidates) > 0 {
		t.Error("Expected at least one host candidate")
	}

	t.Logf("Gathered %d candidates", len(result.Candidates))
}

// TestPortMapperIntegration tests port mapper with both protocols
func TestPortMapperIntegration(t *testing.T) {
	// Skip: MapPort triggers UPnP SSDP multicast, NAT-PMP, and PCP discovery
	t.Skip("skipped: triggers network discovery protocols")
}

// TestPathCacheIntegration tests path cache with realistic workflow
func TestPathCacheIntegration(t *testing.T) {
	cache := NewPathCache(time.Hour)

	peers := []net.IP{
		net.ParseIP("10.7.0.2"),
		net.ParseIP("10.7.0.3"),
		net.ParseIP("10.7.0.4"),
	}

	// Simulate connections with different methods
	methods := []ConnectionMethod{MethodUPnP, MethodSTUN, MethodDirect}

	for i, peer := range peers {
		addr := &net.UDPAddr{
			IP:   net.ParseIP("203.0.113.1"),
			Port: 10000 + i,
		}
		rtt := time.Duration(20+i*10) * time.Millisecond

		cache.RecordSuccess(peer, addr, methods[i], rtt)
	}

	// Verify cache stats
	stats := cache.Stats()
	if stats.TotalEntries != 3 {
		t.Errorf("TotalEntries = %d, want 3", stats.TotalEntries)
	}

	// Verify best methods
	for i, peer := range peers {
		best := cache.GetBestMethod(peer)
		if best != methods[i] {
			t.Errorf("GetBestMethod(%s) = %v, want %v", peer, best, methods[i])
		}
	}

	// Simulate failure
	cache.RecordFailure(peers[0])
	cache.RecordFailure(peers[0])
	cache.RecordFailure(peers[0])
	cache.RecordFailure(peers[0])

	// After multiple failures, should not recommend
	// (need >3 total and <50% success rate)
	best := cache.GetBestMethod(peers[0])
	if best != MethodUnknown {
		t.Errorf("Expected MethodUnknown after failures, got %v", best)
	}
}

// TestConnectionQualityIntegration tests quality monitoring
func TestConnectionQualityIntegration(t *testing.T) {
	q := NewConnectionQuality()

	// Simulate a good connection
	for i := 0; i < 50; i++ {
		rtt := time.Duration(30+i%10) * time.Millisecond
		q.RecordRTT(rtt)
		q.RecordPacketSent()
		q.RecordPacketReceived()
	}

	metrics := q.GetMetrics()

	if !metrics.IsGood() {
		t.Error("Expected good quality for low-latency connection")
	}

	if metrics.LossRate != 0 {
		t.Errorf("Expected 0 loss rate, got %f", metrics.LossRate)
	}

	if metrics.Score < 70 {
		t.Errorf("Expected score >= 70, got %d", metrics.Score)
	}

	t.Logf("Quality metrics: RTT=%v, Jitter=%v, Score=%d",
		metrics.AvgRTT, metrics.Jitter, metrics.Score)
}

// TestConnectionQualityDegraded tests quality with packet loss
func TestConnectionQualityDegraded(t *testing.T) {
	q := NewConnectionQuality()

	// Simulate degraded connection with 20% packet loss
	for i := 0; i < 100; i++ {
		rtt := time.Duration(100+i%50) * time.Millisecond
		q.RecordRTT(rtt)
		q.RecordPacketSent()

		if i%5 != 0 { // 80% received
			q.RecordPacketReceived()
		} else {
			q.RecordPacketLost()
		}
	}

	metrics := q.GetMetrics()

	if metrics.IsGood() {
		t.Error("Expected bad quality for high-latency connection with loss")
	}

	expectedLoss := 0.2
	if metrics.LossRate < expectedLoss-0.05 || metrics.LossRate > expectedLoss+0.05 {
		t.Errorf("Expected ~20%% loss rate, got %f", metrics.LossRate)
	}

	t.Logf("Degraded metrics: RTT=%v, Loss=%f%%, Score=%d",
		metrics.AvgRTT, metrics.LossRate*100, metrics.Score)
}

// TestCandidatePrioritization tests that candidates are properly prioritized
func TestCandidatePrioritization(t *testing.T) {
	candidates := []*Candidate{
		NewCandidate(CandidateHost, &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 12345}),
		NewCandidate(CandidateServerReflexive, &net.UDPAddr{IP: net.ParseIP("203.0.113.1"), Port: 54321}),
		NewCandidate(CandidateUPnP, &net.UDPAddr{IP: net.ParseIP("203.0.113.1"), Port: 12345}),
	}

	// Host should have highest priority (126) for local connectivity,
	// followed by UPnP (120), then ServerReflexive (100)
	var maxPriority uint32
	var maxType CandidateType

	for _, c := range candidates {
		if c.Priority > maxPriority {
			maxPriority = c.Priority
			maxType = c.Type
		}
	}

	if maxType != CandidateHost {
		t.Errorf("Expected Host to have highest priority, got %v", maxType)
	}

	// Verify priority ordering: Host > UPnP > ServerReflexive
	if candidates[0].Priority <= candidates[2].Priority {
		t.Errorf("Expected Host priority > UPnP priority, got %d <= %d",
			candidates[0].Priority, candidates[2].Priority)
	}
	if candidates[2].Priority <= candidates[1].Priority {
		t.Errorf("Expected UPnP priority > ServerReflexive priority, got %d <= %d",
			candidates[2].Priority, candidates[1].Priority)
	}

	// Create pairs and verify ordering
	localCandidates := candidates
	remoteCandidates := []*Candidate{
		NewCandidate(CandidateServerReflexive, &net.UDPAddr{IP: net.ParseIP("198.51.100.1"), Port: 33333}),
	}

	pairs := make([]*CandidatePair, 0)
	for _, local := range localCandidates {
		for _, remote := range remoteCandidates {
			pairs = append(pairs, NewCandidatePair(local, remote, true))
		}
	}

	SortCandidatePairs(pairs)

	// First pair should include Host candidate (highest priority)
	if pairs[0].Local.Type != CandidateHost {
		t.Errorf("Expected first pair to have Host local candidate, got %v", pairs[0].Local.Type)
	}
}

// TestNATTypeDetection tests NAT type detection logic
func TestNATTypeDetection(t *testing.T) {
	tests := []struct {
		name     string
		natType  NATType
		expected string
	}{
		{"Full Cone", NATFullCone, "Full Cone NAT"},
		{"Restricted Cone", NATRestrictedCone, "Restricted Cone NAT"},
		{"Port Restricted", NATPortRestricted, "Port-Restricted Cone NAT"},
		{"Symmetric", NATSymmetric, "Symmetric NAT"},
		{"Unknown", NATUnknown, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.natType.String() != tt.expected {
				t.Errorf("String() = %q, want %q", tt.natType.String(), tt.expected)
			}
		})
	}
}

// TestSpoofConfigValidation tests spoof config validation
func TestSpoofConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *SpoofConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "none mode",
			config:  &SpoofConfig{Mode: SpoofNone},
			wantErr: false,
		},
		{
			name:    "dns mode",
			config:  &SpoofConfig{Mode: SpoofDNS},
			wantErr: false, // May fail if can't bind to port 53, but validation accepts it
		},
		{
			name:    "custom port high",
			config:  &SpoofConfig{Mode: SpoofCustom, CustomPort: 8080},
			wantErr: false,
		},
		{
			name:    "custom port zero means random",
			config:  &SpoofConfig{Mode: SpoofCustom, CustomPort: 0},
			wantErr: false, // CustomPort=0 means random port, which is valid
		},
		{
			name:    "custom port valid range",
			config:  &SpoofConfig{Mode: SpoofCustom, CustomPort: 12345},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSpoofConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSpoofConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestGatewayDiscovery tests platform-specific gateway discovery
func TestGatewayDiscovery(t *testing.T) {
	// Skip: runs system commands (route/ip) and may dial 8.8.8.8 as fallback
	t.Skip("skipped: executes system commands and may make external network connection")
}

// BenchmarkHolePunchPacketEncode benchmarks packet encoding
func BenchmarkHolePunchPacketEncode(b *testing.B) {
	pkt := &HolePunchPacket{
		Type:      HolePunchRequest,
		SrcVIP:    net.ParseIP("10.7.0.1"),
		DstVIP:    net.ParseIP("10.7.0.2"),
		Timestamp: time.Now().UnixNano(),
		Seq:       12345,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pkt.Encode()
	}
}

// BenchmarkHolePunchPacketDecode benchmarks packet decoding
func BenchmarkHolePunchPacketDecode(b *testing.B) {
	pkt := &HolePunchPacket{
		Type:      HolePunchRequest,
		SrcVIP:    net.ParseIP("10.7.0.1"),
		DstVIP:    net.ParseIP("10.7.0.2"),
		Timestamp: time.Now().UnixNano(),
		Seq:       12345,
	}
	data := pkt.Encode()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DecodeHolePunchPacket(data)
	}
}

// BenchmarkCandidatePairSort benchmarks candidate pair sorting
func BenchmarkCandidatePairSort(b *testing.B) {
	localCandidates := []*Candidate{
		NewCandidate(CandidateHost, &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 12345}),
		NewCandidate(CandidateServerReflexive, &net.UDPAddr{IP: net.ParseIP("203.0.113.1"), Port: 54321}),
		NewCandidate(CandidateUPnP, &net.UDPAddr{IP: net.ParseIP("203.0.113.1"), Port: 12345}),
	}

	remoteCandidates := []*Candidate{
		NewCandidate(CandidateHost, &net.UDPAddr{IP: net.ParseIP("192.168.1.200"), Port: 12345}),
		NewCandidate(CandidateServerReflexive, &net.UDPAddr{IP: net.ParseIP("198.51.100.1"), Port: 33333}),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pairs := make([]*CandidatePair, 0, len(localCandidates)*len(remoteCandidates))
		for _, local := range localCandidates {
			for _, remote := range remoteCandidates {
				pairs = append(pairs, NewCandidatePair(local, remote, true))
			}
		}
		SortCandidatePairs(pairs)
	}
}

// BenchmarkPathCacheLookup benchmarks path cache lookup
func BenchmarkPathCacheLookup(b *testing.B) {
	cache := NewPathCache(time.Hour)

	// Pre-populate cache
	for i := 0; i < 100; i++ {
		peer := net.IPv4(10, 7, 0, byte(i+1))
		addr := &net.UDPAddr{IP: net.ParseIP("203.0.113.1"), Port: 10000 + i}
		cache.RecordSuccess(peer, addr, MethodSTUN, 50*time.Millisecond)
	}

	peer := net.ParseIP("10.7.0.50")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Get(peer)
	}
}
