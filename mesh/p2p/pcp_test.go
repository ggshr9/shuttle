package p2p

import (
	"net"
	"testing"
	"time"
)

func TestNewPCPClient(t *testing.T) {
	gateway := net.ParseIP("192.168.1.1")
	client := NewPCPClient(gateway, nil)

	if client == nil {
		t.Fatal("NewPCPClient returned nil")
	}

	if !client.Gateway().Equal(gateway) {
		t.Errorf("Gateway = %v, want %v", client.Gateway(), gateway)
	}

	if client.IsAvailable() {
		t.Error("IsAvailable() should be false before discovery")
	}
}

func TestPCPClientIsAvailable(t *testing.T) {
	client := NewPCPClient(nil, nil)

	if client.IsAvailable() {
		t.Error("New client should not be available")
	}
}

func TestPCPClientExternalIP(t *testing.T) {
	client := NewPCPClient(nil, nil)

	if client.ExternalIP() != nil {
		t.Error("External IP should be nil before discovery")
	}
}

func TestPCPClientGateway(t *testing.T) {
	gateway := net.ParseIP("10.0.0.1")
	client := NewPCPClient(gateway, nil)

	if !client.Gateway().Equal(gateway) {
		t.Errorf("Gateway() = %v, want %v", client.Gateway(), gateway)
	}
}

func TestPCPClientGetMappings(t *testing.T) {
	client := NewPCPClient(nil, nil)

	mappings := client.GetMappings()
	if len(mappings) != 0 {
		t.Errorf("GetMappings() = %d mappings, want 0", len(mappings))
	}
}

func TestPCPMappingStruct(t *testing.T) {
	mapping := &PCPMapping{
		Protocol:     protocolUDP,
		InternalPort: 12345,
		ExternalPort: 54321,
		ExternalIP:   net.ParseIP("203.0.113.1"),
		Lifetime:     time.Hour,
		Created:      time.Now(),
	}

	if mapping.Protocol != protocolUDP {
		t.Errorf("Protocol = %d, want %d", mapping.Protocol, protocolUDP)
	}

	if mapping.InternalPort != 12345 {
		t.Errorf("InternalPort = %d, want 12345", mapping.InternalPort)
	}

	if mapping.ExternalPort != 54321 {
		t.Errorf("ExternalPort = %d, want 54321", mapping.ExternalPort)
	}

	if mapping.Lifetime != time.Hour {
		t.Errorf("Lifetime = %v, want %v", mapping.Lifetime, time.Hour)
	}
}

func TestIPToIPv6(t *testing.T) {
	tests := []struct {
		name     string
		ip       net.IP
		expected []byte
	}{
		{
			name:     "IPv4",
			ip:       net.ParseIP("192.168.1.1"),
			expected: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 192, 168, 1, 1},
		},
		{
			name:     "IPv4 localhost",
			ip:       net.ParseIP("127.0.0.1"),
			expected: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 127, 0, 0, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ipToIPv6(tt.ip)
			if len(result) != 16 {
				t.Fatalf("Length = %d, want 16", len(result))
			}
			for i := range tt.expected {
				if result[i] != tt.expected[i] {
					t.Errorf("Byte %d: got %d, want %d", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestIPv6ToIP(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected net.IP
	}{
		{
			name:     "IPv4-mapped",
			data:     []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 192, 168, 1, 1},
			expected: net.ParseIP("192.168.1.1").To4(),
		},
		{
			name:     "Invalid length",
			data:     []byte{1, 2, 3},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ipv6ToIP(tt.data)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("Got %v, want nil", result)
				}
			} else if !result.Equal(tt.expected) {
				t.Errorf("Got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestProtocolName(t *testing.T) {
	tests := []struct {
		protocol int
		expected string
	}{
		{protocolTCP, "TCP"},
		{protocolUDP, "UDP"},
		{99, "proto-99"},
	}

	for _, tt := range tests {
		result := protocolName(tt.protocol)
		if result != tt.expected {
			t.Errorf("protocolName(%d) = %q, want %q", tt.protocol, result, tt.expected)
		}
	}
}

func TestPCPResultString(t *testing.T) {
	tests := []struct {
		code     byte
		contains string
	}{
		{pcpResultSuccess, "success"},
		{pcpResultUnsupportedVersion, "unsupported version"},
		{pcpResultNotAuthorized, "not authorized"},
		{pcpResultMalformedRequest, "malformed request"},
		{pcpResultUnsupportedOpcode, "unsupported opcode"},
		{pcpResultNetworkFailure, "network failure"},
		{pcpResultNoResources, "no resources"},
		{255, "unknown"},
	}

	for _, tt := range tests {
		result := pcpResultString(tt.code)
		if result == "" {
			t.Errorf("pcpResultString(%d) returned empty string", tt.code)
		}
	}
}

func TestPCPClientDiscoverNoGateway(t *testing.T) {
	// Skip: Discover() sends PCP packets to gateway address over UDP
	t.Skip("skipped: sends network packets to gateway")
}

func TestPCPClientAddMappingNotDiscovered(t *testing.T) {
	client := NewPCPClient(nil, nil)

	_, err := client.AddPortMapping(protocolUDP, 12345, 0, time.Hour)
	if err == nil {
		t.Error("AddPortMapping should fail before discovery")
	}
}

func TestPCPClientDeleteMappingNotFound(t *testing.T) {
	client := NewPCPClient(nil, nil)

	// Should not error for non-existent mapping
	err := client.DeletePortMapping(99999)
	if err != nil {
		t.Errorf("DeletePortMapping should not error for non-existent: %v", err)
	}
}

func TestPCPClientRefreshMappingNotFound(t *testing.T) {
	client := NewPCPClient(nil, nil)

	err := client.RefreshMapping(99999, time.Hour)
	if err == nil {
		t.Error("RefreshMapping should fail for non-existent mapping")
	}
}

func TestPCPClientClose(t *testing.T) {
	client := NewPCPClient(nil, nil)

	// Should not panic or error
	err := client.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestBuildAnnounceRequest(t *testing.T) {
	client := NewPCPClient(nil, nil)
	clientIP := net.ParseIP("192.168.1.100")

	req := client.buildAnnounceRequest(clientIP)

	if len(req) != pcpHeaderSize {
		t.Fatalf("Request length = %d, want %d", len(req), pcpHeaderSize)
	}

	if req[0] != pcpVersion {
		t.Errorf("Version = %d, want %d", req[0], pcpVersion)
	}

	if req[1] != pcpOpcodeAnnounce {
		t.Errorf("Opcode = %d, want %d", req[1], pcpOpcodeAnnounce)
	}
}

func TestBuildMapRequest(t *testing.T) {
	client := NewPCPClient(nil, nil)
	clientIP := net.ParseIP("192.168.1.100")
	var nonce [12]byte
	nonce[0] = 1

	req := client.buildMapRequest(clientIP, protocolUDP, 12345, 54321, 3600, nonce)

	expectedLen := pcpHeaderSize + pcpMapSize
	if len(req) != expectedLen {
		t.Fatalf("Request length = %d, want %d", len(req), expectedLen)
	}

	if req[0] != pcpVersion {
		t.Errorf("Version = %d, want %d", req[0], pcpVersion)
	}

	if req[1] != pcpOpcodeMap {
		t.Errorf("Opcode = %d, want %d", req[1], pcpOpcodeMap)
	}
}

func TestParseAnnounceResponseInvalid(t *testing.T) {
	client := NewPCPClient(nil, nil)

	tests := []struct {
		name string
		data []byte
	}{
		{"too short", []byte{1, 2, 3}},
		{"wrong version", make([]byte, pcpHeaderSize)},
		{"not response", func() []byte {
			d := make([]byte, pcpHeaderSize)
			d[0] = pcpVersion
			return d
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.parseAnnounceResponse(tt.data)
			if err == nil {
				t.Error("Expected error")
			}
		})
	}
}

func TestParseMapResponseInvalid(t *testing.T) {
	client := NewPCPClient(nil, nil)
	var nonce [12]byte

	tests := []struct {
		name string
		data []byte
	}{
		{"too short", []byte{1, 2, 3}},
		{"wrong version", make([]byte, pcpHeaderSize+pcpMapSize)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.parseMapResponse(tt.data, nonce)
			if err == nil {
				t.Error("Expected error")
			}
		})
	}
}

// TestPCPConstants verifies protocol constants are correct
func TestPCPConstants(t *testing.T) {
	if pcpPort != 5351 {
		t.Errorf("pcpPort = %d, want 5351", pcpPort)
	}

	if pcpVersion != 2 {
		t.Errorf("pcpVersion = %d, want 2", pcpVersion)
	}

	if pcpHeaderSize != 24 {
		t.Errorf("pcpHeaderSize = %d, want 24", pcpHeaderSize)
	}

	if pcpMapSize != 36 {
		t.Errorf("pcpMapSize = %d, want 36", pcpMapSize)
	}

	if protocolTCP != 6 {
		t.Errorf("protocolTCP = %d, want 6", protocolTCP)
	}

	if protocolUDP != 17 {
		t.Errorf("protocolUDP = %d, want 17", protocolUDP)
	}
}
