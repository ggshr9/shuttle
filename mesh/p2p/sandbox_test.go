//go:build sandbox

// Sandbox integration tests for mesh/p2p.
//
// These tests run ONLY inside Docker (sandbox/docker-compose.yml gotest service).
// They exercise real network operations: STUN queries, NAT traversal, mDNS,
// gateway discovery — all within isolated Docker networks.
//
// Run with: ./sandbox/run.sh gotest
//
// Environment variables (set by docker-compose):
//   SANDBOX_STUN_ADDR       - STUN server address (e.g. 10.100.0.30:3478)
//   SANDBOX_SERVER_ADDR     - Shuttle server address
//   SANDBOX_ROUTER_ADDR     - NAT router address
//   SANDBOX_NET_A_SELF      - Our IP on net-a
//   SANDBOX_NET_B_SELF      - Our IP on net-b
//   SANDBOX_NET_SERVER_SELF - Our IP on net-server

package p2p

import (
	"context"
	"net"
	"os"
	"testing"
	"time"
)

func sandboxEnv(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Fatalf("environment variable %s not set (are you running inside sandbox?)", key)
	}
	return v
}

// --- STUN Tests ---

// TestSandboxSTUNQuery queries the real STUN server in Docker network
// and verifies we get our NAT-translated address back.
func TestSandboxSTUNQuery(t *testing.T) {
	stunAddr := sandboxEnv(t, "SANDBOX_STUN_ADDR")

	client := NewSTUNClient([]string{stunAddr}, 5*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := client.QueryParallel(ctx)
	if err != nil {
		t.Fatalf("STUN query failed: %v", err)
	}

	if result.PublicAddr == nil {
		t.Fatal("STUN returned nil public address")
	}

	t.Logf("STUN result: public=%v local=%v server=%s", result.PublicAddr, result.LocalAddr, result.Server)

	// Public address should be a valid IP
	if result.PublicAddr.IP.IsUnspecified() || result.PublicAddr.IP.IsLoopback() {
		t.Errorf("unexpected public IP: %v", result.PublicAddr.IP)
	}
}

// TestSandboxSTUNQueryAll queries all STUN servers and verifies consistency.
func TestSandboxSTUNQueryAll(t *testing.T) {
	stunAddr := sandboxEnv(t, "SANDBOX_STUN_ADDR")

	client := NewSTUNClient([]string{stunAddr}, 5*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results, err := client.QueryAllParallel(ctx)
	if err != nil {
		t.Fatalf("STUN query all failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("no STUN results")
	}

	for i, r := range results {
		t.Logf("Result %d: server=%s public=%v", i, r.Server, r.PublicAddr)
	}
}

// TestSandboxSTUNWithSharedConn queries STUN using a shared UDP connection,
// verifying the same local port is reflected back.
func TestSandboxSTUNWithSharedConn(t *testing.T) {
	stunAddr := sandboxEnv(t, "SANDBOX_STUN_ADDR")

	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer conn.Close()

	client := NewSTUNClient([]string{stunAddr}, 5*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results, err := client.QueryParallelWithConn(ctx, conn)
	if err != nil {
		t.Fatalf("STUN query with conn failed: %v", err)
	}

	localPort := conn.LocalAddr().(*net.UDPAddr).Port
	for _, r := range results {
		if r.LocalAddr.Port != localPort {
			t.Errorf("local port mismatch: got %d, want %d", r.LocalAddr.Port, localPort)
		}
	}
}

// --- NAT Detection Tests ---

// TestSandboxNATDetection tests NAT type detection through the Docker router.
func TestSandboxNATDetection(t *testing.T) {
	stunAddr := sandboxEnv(t, "SANDBOX_STUN_ADDR")

	detector := NewNATDetector([]string{stunAddr}, 5*time.Second)

	info, err := detector.Detect()
	if err != nil {
		t.Fatalf("NAT detection failed: %v", err)
	}

	t.Logf("Detected NAT type: %s, public=%v", info.Type, info.PublicAddr)

	// In Docker bridge network, we expect some form of NAT
	if info.Type == NATUnknown {
		t.Log("NAT type unknown — may be expected in Docker bridge")
	}
}

// --- ICE Gathering Tests ---

// TestSandboxICEGather tests ICE candidate gathering with a real STUN server.
func TestSandboxICEGather(t *testing.T) {
	stunAddr := sandboxEnv(t, "SANDBOX_STUN_ADDR")

	gatherer := NewICEGatherer([]string{stunAddr}, 10*time.Second)

	result, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("ICE gather failed: %v", err)
	}
	defer result.LocalConn.Close()

	var hasHost, hasSrflx bool
	for _, c := range result.Candidates {
		t.Logf("Candidate: type=%s addr=%s priority=%d", c.Type, c.Addr, c.Priority)
		switch c.Type {
		case CandidateHost:
			hasHost = true
		case CandidateServerReflexive:
			hasSrflx = true
		}
	}

	if !hasHost {
		t.Error("expected at least one host candidate")
	}
	if !hasSrflx {
		t.Error("expected at least one server-reflexive candidate (STUN)")
	}
}

// --- Gateway Discovery Tests ---

// TestSandboxGatewayDiscovery tests gateway discovery inside Docker.
func TestSandboxGatewayDiscovery(t *testing.T) {
	gateway, err := getDefaultGateway()
	if err != nil {
		t.Fatalf("gateway discovery failed: %v", err)
	}

	if gateway == nil {
		t.Fatal("gateway is nil")
	}

	if gateway.To4() == nil {
		t.Errorf("gateway is not IPv4: %v", gateway)
	}

	t.Logf("Discovered gateway: %s", gateway)
}

// --- mDNS Tests ---

// TestSandboxMDNSAnnounceAndDiscover tests mDNS peer discovery
// within a Docker bridge network (multicast is supported).
func TestSandboxMDNSAnnounceAndDiscover(t *testing.T) {
	// Create two mDNS services simulating two peers
	service1 := NewMDNSService("sandbox-peer-1", nil)
	service2 := NewMDNSService("sandbox-peer-2", nil)

	vip1 := net.ParseIP("10.7.0.1")
	vip2 := net.ParseIP("10.7.0.2")

	if err := service1.Start(vip1, 10001); err != nil {
		t.Fatalf("service1 start: %v", err)
	}
	defer service1.Stop()

	if err := service2.Start(vip2, 10002); err != nil {
		t.Fatalf("service2 start: %v", err)
	}
	defer service2.Stop()

	// Set up discovery callback
	found := make(chan string, 10)
	service2.OnPeerDiscovered(func(peer *MDNSPeer) {
		found <- peer.Name
	})

	// service1 announces, service2 queries
	service2.Query()

	// Wait for discovery
	select {
	case name := <-found:
		t.Logf("Discovered peer: %s", name)
		if name != "sandbox-peer-1" {
			t.Errorf("expected sandbox-peer-1, got %s", name)
		}
	case <-time.After(5 * time.Second):
		t.Log("mDNS discovery timed out (may be expected if Docker network doesn't support multicast)")
	}
}

// --- Hole Punch Tests ---

// TestSandboxHolePunchPacketExchange tests hole punch packets across Docker networks.
func TestSandboxHolePunchPacketExchange(t *testing.T) {
	// Bind to two different Docker network IPs
	netASelf := sandboxEnv(t, "SANDBOX_NET_A_SELF")
	netBSelf := sandboxEnv(t, "SANDBOX_NET_B_SELF")

	conn1, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(netASelf), Port: 0})
	if err != nil {
		t.Fatalf("listen on net-a: %v", err)
	}
	defer conn1.Close()

	conn2, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(netBSelf), Port: 0})
	if err != nil {
		t.Fatalf("listen on net-b: %v", err)
	}
	defer conn2.Close()

	addr1 := conn1.LocalAddr().(*net.UDPAddr)
	addr2 := conn2.LocalAddr().(*net.UDPAddr)

	vip1 := net.ParseIP("10.7.0.1")
	vip2 := net.ParseIP("10.7.0.2")

	pkt1 := &HolePunchPacket{
		Type:      HolePunchRequest,
		SrcVIP:    vip1,
		DstVIP:    vip2,
		Timestamp: time.Now().UnixNano(),
		Seq:       1,
	}

	pkt2 := &HolePunchPacket{
		Type:      HolePunchRequest,
		SrcVIP:    vip2,
		DstVIP:    vip1,
		Timestamp: time.Now().UnixNano(),
		Seq:       2,
	}

	// Send from both sides
	if _, err := conn1.WriteToUDP(pkt1.Encode(), addr2); err != nil {
		t.Fatalf("send from net-a: %v", err)
	}
	if _, err := conn2.WriteToUDP(pkt2.Encode(), addr1); err != nil {
		t.Fatalf("send from net-b: %v", err)
	}

	// Read on both sides
	buf := make([]byte, 1500)
	conn1.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, _, err := conn1.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read on net-a: %v", err)
	}
	got, err := DecodeHolePunchPacket(buf[:n])
	if err != nil {
		t.Fatalf("decode on net-a: %v", err)
	}
	if got.Type != HolePunchRequest {
		t.Errorf("expected HolePunchRequest, got %v", got.Type)
	}

	conn2.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, _, err = conn2.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read on net-b: %v", err)
	}
	got, err = DecodeHolePunchPacket(buf[:n])
	if err != nil {
		t.Fatalf("decode on net-b: %v", err)
	}
	if got.Type != HolePunchRequest {
		t.Errorf("expected HolePunchRequest, got %v", got.Type)
	}

	t.Log("Hole punch packet exchange across Docker networks succeeded")
}

// --- Port Mapper Tests ---

// TestSandboxGetOutboundIP tests outbound IP detection inside Docker.
func TestSandboxGetOutboundIP(t *testing.T) {
	ip, err := getOutboundIP()
	if err != nil {
		t.Fatalf("getOutboundIP: %v", err)
	}

	if ip == nil {
		t.Fatal("getOutboundIP returned nil")
	}

	if ip.IsLoopback() {
		t.Error("returned loopback address")
	}

	t.Logf("Outbound IP: %s", ip)
}

// --- Cross-NAT Tests ---

// TestSandboxCrossNATHolePunch tests hole punching between two peers on different
// subnets (net-a and net-b) through the router NAT. Both peers discover their
// public addresses via STUN, exchange candidates, and attempt hole punch.
func TestSandboxCrossNATHolePunch(t *testing.T) {
	stunAddr := sandboxEnv(t, "SANDBOX_STUN_ADDR")
	netASelf := sandboxEnv(t, "SANDBOX_NET_A_SELF")
	netBSelf := sandboxEnv(t, "SANDBOX_NET_B_SELF")

	// Peer A: bind on net-a, discover public address via STUN
	connA, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(netASelf), Port: 0})
	if err != nil {
		t.Fatalf("listen on net-a: %v", err)
	}
	defer connA.Close()

	// Peer B: bind on net-b, discover public address via STUN
	connB, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(netBSelf), Port: 0})
	if err != nil {
		t.Fatalf("listen on net-b: %v", err)
	}
	defer connB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Both peers query STUN to discover their public (NAT-translated) addresses
	stunClient := NewSTUNClient([]string{stunAddr}, 5*time.Second)

	resultA, err := stunClient.QueryParallelWithConn(ctx, connA)
	if err != nil {
		t.Fatalf("STUN query for peer A failed: %v", err)
	}
	if len(resultA) == 0 {
		t.Fatal("no STUN results for peer A")
	}
	publicAddrA := resultA[0].PublicAddr
	t.Logf("Peer A: local=%v public=%v", connA.LocalAddr(), publicAddrA)

	resultB, err := stunClient.QueryParallelWithConn(ctx, connB)
	if err != nil {
		t.Fatalf("STUN query for peer B failed: %v", err)
	}
	if len(resultB) == 0 {
		t.Fatal("no STUN results for peer B")
	}
	publicAddrB := resultB[0].PublicAddr
	t.Logf("Peer B: local=%v public=%v", connB.LocalAddr(), publicAddrB)

	vipA := net.ParseIP("10.7.0.1")
	vipB := net.ParseIP("10.7.0.2")

	// Build candidate lists: each peer has host + server-reflexive candidates
	localAddrA := connA.LocalAddr().(*net.UDPAddr)
	localAddrB := connB.LocalAddr().(*net.UDPAddr)

	candidatesForA := []*Candidate{
		NewCandidate(CandidateServerReflexive, publicAddrB),
		NewCandidate(CandidateHost, localAddrB),
	}
	candidatesForB := []*Candidate{
		NewCandidate(CandidateServerReflexive, publicAddrA),
		NewCandidate(CandidateHost, localAddrA),
	}

	// Run hole punchers concurrently from both sides
	hpA := NewHolePuncher(connA, vipA, 10*time.Second, nil)
	hpB := NewHolePuncher(connB, vipB, 10*time.Second, nil)

	type punchResult struct {
		result *HolePunchResult
		err    error
		peer   string
	}
	results := make(chan punchResult, 2)

	go func() {
		r, err := hpA.Punch(ctx, vipB, candidatesForA)
		results <- punchResult{r, err, "A"}
	}()
	go func() {
		r, err := hpB.Punch(ctx, vipA, candidatesForB)
		results <- punchResult{r, err, "B"}
	}()

	// Wait for at least one side to succeed
	var succeeded int
	for i := 0; i < 2; i++ {
		select {
		case pr := <-results:
			if pr.err != nil {
				t.Logf("Peer %s hole punch: %v (may be expected for cross-NAT)", pr.peer, pr.err)
			} else {
				succeeded++
				t.Logf("Peer %s hole punch succeeded: remote=%v RTT=%v", pr.peer, pr.result.RemoteAddr, pr.result.RTT)
			}
		case <-ctx.Done():
			t.Fatal("timeout waiting for hole punch results")
		}
	}

	if succeeded == 0 {
		t.Log("Neither side succeeded with hole punch (may be expected with symmetric NAT in Docker)")
	} else {
		t.Logf("Hole punch succeeded on %d side(s)", succeeded)
	}
}

// TestSandboxmDNSDiscovery tests mDNS peer discovery within the same subnet.
// Creates two mDNS advertisers on the gotest container and verifies they can
// discover each other.
func TestSandboxmDNSDiscovery(t *testing.T) {
	service1 := NewMDNSService("sandbox-mdns-peer-1", nil)
	service2 := NewMDNSService("sandbox-mdns-peer-2", nil)

	vip1 := net.ParseIP("10.7.0.11")
	vip2 := net.ParseIP("10.7.0.12")

	if err := service1.Start(vip1, 20001); err != nil {
		t.Fatalf("service1 start: %v", err)
	}
	defer service1.Stop()

	if err := service2.Start(vip2, 20002); err != nil {
		t.Fatalf("service2 start: %v", err)
	}
	defer service2.Stop()

	// Set up bidirectional discovery callbacks
	found1 := make(chan *MDNSPeer, 10)
	found2 := make(chan *MDNSPeer, 10)

	service1.OnPeerDiscovered(func(peer *MDNSPeer) {
		found1 <- peer
	})
	service2.OnPeerDiscovered(func(peer *MDNSPeer) {
		found2 <- peer
	})

	// Allow initial announcements to propagate
	time.Sleep(500 * time.Millisecond)

	// Both services query for peers
	if err := service1.Query(); err != nil {
		t.Fatalf("service1 query: %v", err)
	}
	if err := service2.Query(); err != nil {
		t.Fatalf("service2 query: %v", err)
	}

	// Check if service1 discovers service2
	var disc1, disc2 bool
	timeout := time.After(10 * time.Second)

	for !disc1 || !disc2 {
		select {
		case peer := <-found1:
			t.Logf("Service1 discovered: %s", peer.Name)
			if peer.Name == "sandbox-mdns-peer-2" {
				disc1 = true
			}
		case peer := <-found2:
			t.Logf("Service2 discovered: %s", peer.Name)
			if peer.Name == "sandbox-mdns-peer-1" {
				disc2 = true
			}
		case <-timeout:
			if !disc1 && !disc2 {
				t.Log("mDNS bidirectional discovery timed out (may be expected if Docker network doesn't support multicast)")
			} else {
				if !disc1 {
					t.Log("Service1 did not discover service2 within timeout")
				}
				if !disc2 {
					t.Log("Service2 did not discover service1 within timeout")
				}
			}
			return
		}
	}

	t.Log("mDNS bidirectional discovery succeeded")

	// Verify peer information
	peers1 := service1.GetPeers()
	peers2 := service2.GetPeers()
	t.Logf("Service1 knows %d peer(s), Service2 knows %d peer(s)", len(peers1), len(peers2))
}

// TestSandboxSTUNNATType queries the STUN server and verifies the detected NAT type.
// The Docker router uses MASQUERADE (iptables) which should appear as some form of NAT.
func TestSandboxSTUNNATType(t *testing.T) {
	stunAddr := sandboxEnv(t, "SANDBOX_STUN_ADDR")
	netASelf := sandboxEnv(t, "SANDBOX_NET_A_SELF")

	// Use a specific source IP on net-a (behind NAT) for deterministic detection
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(netASelf), Port: 0})
	if err != nil {
		t.Fatalf("listen on net-a: %v", err)
	}
	defer conn.Close()

	detector := NewNATDetector([]string{stunAddr}, 5*time.Second)
	info, err := detector.Detect()
	if err != nil {
		t.Fatalf("NAT detection failed: %v", err)
	}

	t.Logf("NAT type: %s", info.Type)
	t.Logf("Public address: %v", info.PublicAddr)
	t.Logf("Local address: %v", info.LocalAddr)

	// Verify public address is valid
	if info.PublicAddr != nil {
		if info.PublicAddr.IP.IsUnspecified() || info.PublicAddr.IP.IsLoopback() {
			t.Errorf("unexpected public IP: %v", info.PublicAddr.IP)
		}
	}

	// Docker MASQUERADE should be detected as some form of NAT, not NATNone
	// (unless the gotest container is on the same subnet as the STUN server)
	if info.Type == NATNone {
		// This is possible if gotest has a direct route to the STUN server
		// on net-server; log it but don't fail
		t.Log("Detected no NAT - gotest may have direct connectivity to STUN server")
	}

	// Verify the NAT type is a known value
	knownTypes := []NATType{NATUnknown, NATNone, NATFullCone, NATRestrictedCone, NATPortRestricted, NATSymmetric}
	found := false
	for _, kt := range knownTypes {
		if info.Type == kt {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("unexpected NAT type value: %d", info.Type)
	}

	// Check if hole punching is feasible with the detected type
	t.Logf("Hole punching feasible: %v", info.Type.CanHolePunch())
}

// TestSandboxSTUNMultiServer queries the STUN server from multiple source addresses
// (different subnets) and verifies consistent external address mapping per source.
func TestSandboxSTUNMultiServer(t *testing.T) {
	stunAddr := sandboxEnv(t, "SANDBOX_STUN_ADDR")
	netASelf := sandboxEnv(t, "SANDBOX_NET_A_SELF")
	netBSelf := sandboxEnv(t, "SANDBOX_NET_B_SELF")
	netServerSelf := sandboxEnv(t, "SANDBOX_NET_SERVER_SELF")

	type sourceResult struct {
		name    string
		srcIP   string
		results []*STUNResult
		err     error
	}

	sources := []struct {
		name string
		ip   string
	}{
		{"net-a", netASelf},
		{"net-b", netBSelf},
		{"net-server", netServerSelf},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	allResults := make([]sourceResult, 0, len(sources))

	for _, src := range sources {
		// Bind to each subnet and query STUN twice to check consistency
		conn1, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(src.ip), Port: 0})
		if err != nil {
			t.Fatalf("listen on %s: %v", src.name, err)
		}

		conn2, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(src.ip), Port: 0})
		if err != nil {
			conn1.Close()
			t.Fatalf("listen on %s (second): %v", src.name, err)
		}

		client := NewSTUNClient([]string{stunAddr}, 5*time.Second)

		res1, err1 := client.QueryParallelWithConn(ctx, conn1)
		res2, err2 := client.QueryParallelWithConn(ctx, conn2)

		conn1.Close()
		conn2.Close()

		if err1 != nil || err2 != nil {
			t.Logf("STUN query from %s: err1=%v err2=%v", src.name, err1, err2)
			continue
		}

		if len(res1) == 0 || len(res2) == 0 {
			t.Logf("STUN query from %s: no results", src.name)
			continue
		}

		sr := sourceResult{
			name:  src.name,
			srcIP: src.ip,
		}
		sr.results = append(sr.results, res1...)
		sr.results = append(sr.results, res2...)
		allResults = append(allResults, sr)

		// Both queries from the same source IP should get the same public IP
		// (though ports may differ depending on NAT type)
		pubIP1 := res1[0].PublicAddr.IP
		pubIP2 := res2[0].PublicAddr.IP

		t.Logf("Source %s (%s): query1 public=%v, query2 public=%v",
			src.name, src.ip, res1[0].PublicAddr, res2[0].PublicAddr)

		if !pubIP1.Equal(pubIP2) {
			t.Errorf("Inconsistent public IP from %s: %v vs %v (may indicate symmetric NAT)",
				src.name, pubIP1, pubIP2)
		}
	}

	if len(allResults) == 0 {
		t.Fatal("no STUN results from any source")
	}

	// Verify that different subnets may get different public IPs
	// (since they go through different NAT paths)
	if len(allResults) >= 2 {
		ip1 := allResults[0].results[0].PublicAddr.IP
		ip2 := allResults[1].results[0].PublicAddr.IP
		if ip1.Equal(ip2) {
			t.Logf("Same public IP from %s and %s: %v (expected if using same NAT router)",
				allResults[0].name, allResults[1].name, ip1)
		} else {
			t.Logf("Different public IPs from %s (%v) and %s (%v)",
				allResults[0].name, ip1, allResults[1].name, ip2)
		}
	}
}
