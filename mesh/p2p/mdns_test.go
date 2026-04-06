package p2p

import (
	"net"
	"testing"
	"time"
)

func TestNewMDNSService(t *testing.T) {
	service := NewMDNSService("test-instance", nil)

	if service == nil {
		t.Fatal("NewMDNSService returned nil")
		return
	}

	if service.instanceName != "test-instance" {
		t.Errorf("instanceName = %q, want %q", service.instanceName, "test-instance")
	}
}

func TestNewMDNSServiceAutoName(t *testing.T) {
	service := NewMDNSService("", nil)

	if service == nil {
		t.Fatal("NewMDNSService returned nil")
		return
	}

	if service.instanceName == "" {
		t.Error("instanceName should be auto-generated")
	}

	if len(service.instanceName) < 8 {
		t.Errorf("instanceName too short: %q", service.instanceName)
	}
}

func TestMDNSServiceSetMetadata(t *testing.T) {
	service := NewMDNSService("test", nil)

	service.SetMetadata("key1", "value1")
	service.SetMetadata("key2", "value2")

	if len(service.metadata) != 2 {
		t.Errorf("metadata length = %d, want 2", len(service.metadata))
	}

	if service.metadata["key1"] != "value1" {
		t.Errorf("metadata[key1] = %q, want %q", service.metadata["key1"], "value1")
	}
}

func TestMDNSServiceGetPeers(t *testing.T) {
	service := NewMDNSService("test", nil)

	// Add some test peers
	service.peers["peer1"] = &MDNSPeer{Name: "peer1"}
	service.peers["peer2"] = &MDNSPeer{Name: "peer2"}

	peers := service.GetPeers()
	if len(peers) != 2 {
		t.Errorf("GetPeers() returned %d peers, want 2", len(peers))
	}
}

func TestMDNSServiceGetPeer(t *testing.T) {
	service := NewMDNSService("test", nil)

	service.peers["peer1"] = &MDNSPeer{Name: "peer1", Port: 12345}

	peer := service.GetPeer("peer1")
	if peer == nil {
		t.Fatal("GetPeer returned nil")
		return
	}
	if peer.Port != 12345 {
		t.Errorf("peer.Port = %d, want 12345", peer.Port)
	}

	// Non-existent peer
	peer = service.GetPeer("nonexistent")
	if peer != nil {
		t.Error("GetPeer should return nil for non-existent peer")
	}
}

func TestMDNSPeerStruct(t *testing.T) {
	peer := &MDNSPeer{
		Name:      "test-peer",
		VIP:       net.ParseIP("10.7.0.2"),
		Addresses: []net.IP{net.ParseIP("192.168.1.100")},
		Port:      12345,
		LastSeen:  time.Now(),
		Metadata:  map[string]string{"key": "value"},
	}

	if peer.Name != "test-peer" {
		t.Errorf("Name = %q, want %q", peer.Name, "test-peer")
	}

	if !peer.VIP.Equal(net.ParseIP("10.7.0.2")) {
		t.Errorf("VIP = %v, want 10.7.0.2", peer.VIP)
	}

	if peer.Port != 12345 {
		t.Errorf("Port = %d, want 12345", peer.Port)
	}
}

func TestParseDNSName(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		offset   int
		expected string
	}{
		{
			name:     "simple name",
			data:     []byte{4, 't', 'e', 's', 't', 5, 'l', 'o', 'c', 'a', 'l', 0},
			offset:   0,
			expected: "test.local",
		},
		{
			name:     "single label",
			data:     []byte{3, 'f', 'o', 'o', 0},
			offset:   0,
			expected: "foo",
		},
		{
			name:     "empty",
			data:     []byte{0},
			offset:   0,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := parseDNSName(tt.data, tt.offset)
			if result != tt.expected {
				t.Errorf("parseDNSName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestWriteDNSName(t *testing.T) {
	buf := make([]byte, 100)

	n := writeDNSName(buf, "test.local.")
	expected := []byte{4, 't', 'e', 's', 't', 5, 'l', 'o', 'c', 'a', 'l', 0}

	if n != len(expected) {
		t.Errorf("writeDNSName length = %d, want %d", n, len(expected))
	}

	for i := 0; i < n; i++ {
		if buf[i] != expected[i] {
			t.Errorf("byte %d: got %d, want %d", i, buf[i], expected[i])
		}
	}
}

func TestWriteTTL(t *testing.T) {
	buf := make([]byte, 4)
	n := writeTTL(buf, 120)

	if n != 4 {
		t.Errorf("writeTTL length = %d, want 4", n)
	}

	expected := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	if expected != 120 {
		t.Errorf("TTL = %d, want 120", expected)
	}
}

func TestExtractInstanceName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test._shuttle._udp.local.", "test"},
		{"my-instance._shuttle._udp.local.", "my-instance"},
		{"hostname.local.", "hostname"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		result := extractInstanceName(tt.input)
		if result != tt.expected {
			t.Errorf("extractInstanceName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestGetLocalIPs(t *testing.T) {
	// getLocalIPs only reads network interface info, does not make connections.
	ips := getLocalIPs()

	t.Logf("Found %d local IPs", len(ips))

	for _, ip := range ips {
		if ip.IsLoopback() {
			t.Errorf("Should not include loopback: %v", ip)
		}
		if ip.IsLinkLocalUnicast() {
			t.Errorf("Should not include link-local: %v", ip)
		}
	}
}

func TestMDNSServiceCallbacks(t *testing.T) {
	service := NewMDNSService("test", nil)

	discovered := make(chan *MDNSPeer, 1)
	lost := make(chan *MDNSPeer, 1)

	service.OnPeerDiscovered(func(peer *MDNSPeer) {
		discovered <- peer
	})
	service.OnPeerLost(func(peer *MDNSPeer) {
		lost <- peer
	})

	// Simulate peer discovery
	service.updatePeer("other-peer._shuttle._udp.local.", nil, 120)

	select {
	case peer := <-discovered:
		if peer.Name != "other-peer" {
			t.Errorf("peer.Name = %q, want %q", peer.Name, "other-peer")
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for peer discovery callback")
	}
}

func TestMDNSServiceBuildQuery(t *testing.T) {
	service := NewMDNSService("test", nil)

	query := service.buildQuery()

	if len(query) < 12 {
		t.Errorf("Query too short: %d bytes", len(query))
	}

	// Check flags (query = 0x0000)
	flags := uint16(query[2])<<8 | uint16(query[3])
	if flags != 0 {
		t.Errorf("Flags = 0x%04x, want 0x0000", flags)
	}

	// Check QDCOUNT = 1
	qdcount := uint16(query[4])<<8 | uint16(query[5])
	if qdcount != 1 {
		t.Errorf("QDCOUNT = %d, want 1", qdcount)
	}
}

func TestMDNSServiceBuildResponse(t *testing.T) {
	service := NewMDNSService("test-instance", nil)
	service.vip = net.ParseIP("10.7.0.1")
	service.port = 12345

	response := service.buildResponse()

	if len(response) < 12 {
		t.Errorf("Response too short: %d bytes", len(response))
	}

	// Check flags (response = 0x8400)
	flags := uint16(response[2])<<8 | uint16(response[3])
	if flags != 0x8400 {
		t.Errorf("Flags = 0x%04x, want 0x8400", flags)
	}

	// Check ANCOUNT > 0
	ancount := uint16(response[6])<<8 | uint16(response[7])
	if ancount == 0 {
		t.Error("ANCOUNT should be > 0")
	}
}

func TestMDNSServiceStopWithoutStart(t *testing.T) {
	service := NewMDNSService("test", nil)

	// Should not panic or error
	err := service.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestMDNSServiceQueryWithoutStart(t *testing.T) {
	service := NewMDNSService("test", nil)

	err := service.Query()
	if err == nil {
		t.Error("Query should fail when service not running")
	}
}

func TestMDNSConstants(t *testing.T) {
	if mdnsIPv4Addr != "224.0.0.251:5353" {
		t.Errorf("mdnsIPv4Addr = %q, want 224.0.0.251:5353", mdnsIPv4Addr)
	}

	if mdnsTTL != 120 {
		t.Errorf("mdnsTTL = %d, want 120", mdnsTTL)
	}

	if shuttleServiceName != "_shuttle._udp.local." {
		t.Errorf("shuttleServiceName = %q, want _shuttle._udp.local.", shuttleServiceName)
	}

	if mdnsTypeA != 1 {
		t.Errorf("mdnsTypeA = %d, want 1", mdnsTypeA)
	}

	if mdnsTypePTR != 12 {
		t.Errorf("mdnsTypePTR = %d, want 12", mdnsTypePTR)
	}

	if mdnsTypeSRV != 33 {
		t.Errorf("mdnsTypeSRV = %d, want 33", mdnsTypeSRV)
	}

	if mdnsTypeTXT != 16 {
		t.Errorf("mdnsTypeTXT = %d, want 16", mdnsTypeTXT)
	}
}

func TestMDNSServiceExpirePeers(t *testing.T) {
	service := NewMDNSService("test", nil)

	// Add a peer with old timestamp
	service.peers["old-peer"] = &MDNSPeer{
		Name:     "old-peer",
		LastSeen: time.Now().Add(-5 * time.Minute), // Old peer
	}
	service.peers["new-peer"] = &MDNSPeer{
		Name:     "new-peer",
		LastSeen: time.Now(), // Fresh peer
	}

	service.expirePeers()

	if _, ok := service.peers["old-peer"]; ok {
		t.Error("Old peer should have been expired")
	}

	if _, ok := service.peers["new-peer"]; !ok {
		t.Error("New peer should not have been expired")
	}
}

func TestLookupPeersTimeout(t *testing.T) {
	// Skip: LookupPeers joins mDNS multicast group 224.0.0.251:5353
	// and sends multicast queries on the local network
	t.Skip("skipped: sends mDNS multicast on local network")
}

func TestAddPeerAddress(t *testing.T) {
	service := NewMDNSService("test", nil)
	peer := &MDNSPeer{
		Name:      "test-peer",
		Addresses: []net.IP{},
	}

	ip1 := net.ParseIP("192.168.1.1")
	ip2 := net.ParseIP("192.168.1.2")

	// Add first IP
	service.addPeerAddress(peer, ip1)
	if len(peer.Addresses) != 1 {
		t.Errorf("Addresses length = %d, want 1", len(peer.Addresses))
	}

	// Add same IP again (should not duplicate)
	service.addPeerAddress(peer, ip1)
	if len(peer.Addresses) != 1 {
		t.Errorf("Addresses length = %d, want 1 (no duplicate)", len(peer.Addresses))
	}

	// Add different IP
	service.addPeerAddress(peer, ip2)
	if len(peer.Addresses) != 2 {
		t.Errorf("Addresses length = %d, want 2", len(peer.Addresses))
	}
}

// TestMDNSPeerVerifiedFlag verifies that the Verified field is false by default
// and that MarkVerified flips it to true for the matching VIP.
func TestMDNSPeerVerifiedFlag(t *testing.T) {
	service := NewMDNSService("test", nil)

	vip1 := net.ParseIP("10.7.0.2")
	vip2 := net.ParseIP("10.7.0.3")

	service.peers["peer-a"] = &MDNSPeer{
		Name:     "peer-a",
		VIP:      vip1,
		Verified: false,
	}
	service.peers["peer-b"] = &MDNSPeer{
		Name:     "peer-b",
		VIP:      vip2,
		Verified: false,
	}

	// Initially both peers are unverified.
	peerA := service.GetPeerByVIP(vip1)
	if peerA == nil {
		t.Fatal("GetPeerByVIP returned nil for peer-a")
	}
	if peerA.Verified {
		t.Error("peer-a should be unverified before handshake")
	}

	// Verify peer-a by VIP.
	service.MarkVerified(vip1)

	peerA = service.GetPeerByVIP(vip1)
	if peerA == nil {
		t.Fatal("GetPeerByVIP returned nil for peer-a after MarkVerified")
	}
	if !peerA.Verified {
		t.Error("peer-a should be verified after MarkVerified")
	}

	// peer-b must remain unverified.
	peerB := service.GetPeerByVIP(vip2)
	if peerB == nil {
		t.Fatal("GetPeerByVIP returned nil for peer-b")
	}
	if peerB.Verified {
		t.Error("peer-b should still be unverified")
	}
}

// TestMDNSGetPeerByVIP tests looking up a peer by VIP.
func TestMDNSGetPeerByVIP(t *testing.T) {
	service := NewMDNSService("test", nil)

	vip := net.ParseIP("10.7.0.5")
	service.peers["my-peer"] = &MDNSPeer{
		Name: "my-peer",
		VIP:  vip,
		Port: 54321,
	}

	// Found case.
	peer := service.GetPeerByVIP(vip)
	if peer == nil {
		t.Fatal("GetPeerByVIP returned nil for existing peer")
	}
	if peer.Port != 54321 {
		t.Errorf("peer.Port = %d, want 54321", peer.Port)
	}

	// Not found case.
	peer = service.GetPeerByVIP(net.ParseIP("10.7.0.99"))
	if peer != nil {
		t.Error("GetPeerByVIP should return nil for unknown VIP")
	}
}

// TestMDNSMarkVerifiedUnknownVIP verifies that MarkVerified is a no-op for
// an unknown VIP (no panic, no spurious side effects).
func TestMDNSMarkVerifiedUnknownVIP(t *testing.T) {
	service := NewMDNSService("test", nil)
	// Should not panic.
	service.MarkVerified(net.ParseIP("10.7.0.99"))
}

// TestMDNSVerifiedFlagNotSetOnNewDiscovery ensures that freshly discovered
// peers always start unverified, regardless of any previous state.
func TestMDNSVerifiedFlagNotSetOnNewDiscovery(t *testing.T) {
	service := NewMDNSService("test-host", nil)

	// Simulate discovery of a new peer via mDNS PTR record.
	service.updatePeer("evil-peer._shuttle._udp.local.", nil, 120)

	peer := service.GetPeer("evil-peer")
	if peer == nil {
		t.Fatal("updatePeer did not create peer entry")
	}
	if peer.Verified {
		t.Error("newly discovered mDNS peer must not be pre-verified")
	}
}
