package p2p

import (
	"net"
	"testing"
)

func TestNewNATPMPClient(t *testing.T) {
	client := NewNATPMPClient(nil)
	if client == nil {
		t.Fatal("NewNATPMPClient returned nil")
		return
	}

	if client.mappings == nil {
		t.Error("mappings map not initialized")
	}
}

func TestNATPMPClientIsAvailable(t *testing.T) {
	client := NewNATPMPClient(nil)

	// Should be false before discovery
	if client.IsAvailable() {
		t.Error("IsAvailable should be false before discovery")
	}
}

func TestNATPMPClientExternalIP(t *testing.T) {
	client := NewNATPMPClient(nil)

	// Before discovery, ExternalIP should be nil
	if client.ExternalIP() != nil {
		t.Error("ExternalIP should be nil before discovery")
	}
}

func TestNATPMPClientGateway(t *testing.T) {
	client := NewNATPMPClient(nil)

	// Before discovery, Gateway should be nil
	if client.Gateway() != nil {
		t.Error("Gateway should be nil before discovery")
	}
}

func TestNATPMPClientGetMappings(t *testing.T) {
	client := NewNATPMPClient(nil)

	mappings := client.GetMappings()
	if mappings == nil {
		t.Error("GetMappings returned nil")
	}
	if len(mappings) != 0 {
		t.Errorf("GetMappings should be empty, got %d", len(mappings))
	}
}

func TestNATPMPMappingStruct(t *testing.T) {
	mapping := &NATPMPMapping{
		InternalPort: 12345,
		ExternalPort: 54321,
		Protocol:     "UDP",
		Lifetime:     3600,
	}

	if mapping.InternalPort != 12345 {
		t.Errorf("InternalPort = %d, want 12345", mapping.InternalPort)
	}
	if mapping.ExternalPort != 54321 {
		t.Errorf("ExternalPort = %d, want 54321", mapping.ExternalPort)
	}
	if mapping.Protocol != "UDP" {
		t.Errorf("Protocol = %s, want UDP", mapping.Protocol)
	}
	if mapping.Lifetime != 3600 {
		t.Errorf("Lifetime = %d, want 3600", mapping.Lifetime)
	}
}

func TestGetDefaultGateway(t *testing.T) {
	// Skip: getDefaultGateway() runs system commands and may dial 8.8.8.8 as fallback
	t.Skip("skipped: executes system commands and may make external network connection")
}

func TestNATPMPResultCodeError(t *testing.T) {
	client := NewNATPMPClient(nil)

	tests := []struct {
		code    uint16
		wantErr bool
	}{
		{natpmpSuccess, false},
		{natpmpUnsupportedVersion, true},
		{natpmpNotAuthorized, true},
		{natpmpNetworkFailure, true},
		{natpmpOutOfResources, true},
		{natpmpUnsupportedOpcode, true},
		{99, true}, // unknown code
	}

	for _, tt := range tests {
		err := client.resultCodeError(tt.code)
		if (err != nil) != tt.wantErr {
			t.Errorf("resultCodeError(%d) error = %v, wantErr %v", tt.code, err, tt.wantErr)
		}
	}
}

func TestNATPMPDiscoverNoGateway(t *testing.T) {
	// Skip: Discover() calls getDefaultGateway() which may dial 8.8.8.8,
	// then sends NAT-PMP packets to the gateway
	t.Skip("skipped: makes external network connections")
}

func TestNATPMPDeleteMappingNoGateway(t *testing.T) {
	client := NewNATPMPClient(nil)

	err := client.DeletePortMapping(12345, "UDP")
	if err != ErrNATPMPNotFound {
		t.Errorf("DeletePortMapping without gateway should return ErrNATPMPNotFound, got %v", err)
	}
}

func TestNATPMPAddMappingNoGateway(t *testing.T) {
	client := NewNATPMPClient(nil)

	_, err := client.AddPortMapping(12345, 12345, "UDP", 3600)
	if err != ErrNATPMPNotFound {
		t.Errorf("AddPortMapping without gateway should return ErrNATPMPNotFound, got %v", err)
	}
}

func TestNATPMPGetExternalIPNoGateway(t *testing.T) {
	client := NewNATPMPClient(nil)

	_, err := client.GetExternalIP()
	if err != ErrNATPMPNotFound {
		t.Errorf("GetExternalIP without gateway should return ErrNATPMPNotFound, got %v", err)
	}
}

func TestPortMapperWithNATPMP(t *testing.T) {
	pm := NewPortMapper(nil)
	if pm == nil {
		t.Fatal("NewPortMapper returned nil")
		return
	}

	// Verify NAT-PMP client is initialized
	if pm.natpmp == nil {
		t.Error("natpmp client not initialized")
	}

	// Protocol should be empty before mapping
	if pm.Protocol() != "" {
		t.Errorf("Protocol should be empty before mapping, got %s", pm.Protocol())
	}
}

func TestPortMapperProtocol(t *testing.T) {
	pm := NewPortMapper(nil)

	// Before mapping, protocol should be empty
	if pm.Protocol() != "" {
		t.Errorf("Protocol should be empty, got %s", pm.Protocol())
	}

	// Simulate UPnP mapping
	pm.mu.Lock()
	pm.mappedPort = 12345
	pm.protocol = "upnp"
	pm.mu.Unlock()

	if pm.Protocol() != "upnp" {
		t.Errorf("Protocol should be upnp, got %s", pm.Protocol())
	}

	// Simulate NAT-PMP mapping
	pm.mu.Lock()
	pm.protocol = "nat-pmp"
	pm.mu.Unlock()

	if pm.Protocol() != "nat-pmp" {
		t.Errorf("Protocol should be nat-pmp, got %s", pm.Protocol())
	}

	// Simulate PCP mapping
	pm.mu.Lock()
	pm.protocol = "pcp"
	pm.mu.Unlock()

	if pm.Protocol() != "pcp" {
		t.Errorf("Protocol should be pcp, got %s", pm.Protocol())
	}
}

func TestPortMapperGetExternalAddrNATPMP(t *testing.T) {
	pm := NewPortMapper(nil)

	// Without mapping, should be nil
	if pm.GetExternalAddr() != nil {
		t.Error("GetExternalAddr should be nil without mapping")
	}

	// Simulate NAT-PMP with external IP
	pm.natpmp.mu.Lock()
	pm.natpmp.externalIP = net.ParseIP("203.0.113.1")
	pm.natpmp.mu.Unlock()

	pm.mu.Lock()
	pm.mappedPort = 12345
	pm.protocol = "nat-pmp"
	pm.mu.Unlock()

	addr := pm.GetExternalAddr()
	if addr == nil {
		t.Fatal("GetExternalAddr should not be nil")
	}

	if !addr.IP.Equal(net.ParseIP("203.0.113.1")) {
		t.Errorf("IP = %v, want 203.0.113.1", addr.IP)
	}
	if addr.Port != 12345 {
		t.Errorf("Port = %d, want 12345", addr.Port)
	}
}

func TestPortMapperGetExternalAddrPCP(t *testing.T) {
	pm := NewPortMapper(nil)

	// Simulate PCP with external IP
	pm.pcp.mu.Lock()
	pm.pcp.externalIP = net.ParseIP("203.0.113.2")
	pm.pcp.mu.Unlock()

	pm.mu.Lock()
	pm.mappedPort = 54321
	pm.protocol = "pcp"
	pm.mu.Unlock()

	addr := pm.GetExternalAddr()
	if addr == nil {
		t.Fatal("GetExternalAddr should not be nil")
	}

	if !addr.IP.Equal(net.ParseIP("203.0.113.2")) {
		t.Errorf("IP = %v, want 203.0.113.2", addr.IP)
	}
	if addr.Port != 54321 {
		t.Errorf("Port = %d, want 54321", addr.Port)
	}
}
