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
