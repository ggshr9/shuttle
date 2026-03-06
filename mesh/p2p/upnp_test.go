package p2p

import (
	"net"
	"testing"
	"time"
)

func TestNewUPnPClient(t *testing.T) {
	client := NewUPnPClient(nil)
	if client == nil {
		t.Fatal("NewUPnPClient returned nil")
	}

	if client.mappings == nil {
		t.Error("mappings map not initialized")
	}
}

func TestUPnPClientIsAvailable(t *testing.T) {
	client := NewUPnPClient(nil)

	// Should be false before discovery
	if client.IsAvailable() {
		t.Error("IsAvailable should be false before discovery")
	}
}

func TestGetOutboundIP(t *testing.T) {
	// Skip: getOutboundIP() dials 8.8.8.8 which affects local network
	t.Skip("skipped: makes external network connection to 8.8.8.8")
}

func TestPortMapper(t *testing.T) {
	pm := NewPortMapper(nil)
	if pm == nil {
		t.Fatal("NewPortMapper returned nil")
	}

	// Should not be available without discovery
	if pm.IsAvailable() {
		t.Error("IsAvailable should be false without discovery")
	}

	// GetMappedPort should be 0
	if pm.GetMappedPort() != 0 {
		t.Error("GetMappedPort should be 0 without mapping")
	}

	// GetExternalAddr should be nil
	if pm.GetExternalAddr() != nil {
		t.Error("GetExternalAddr should be nil without mapping")
	}
}

func TestPortMappingStruct(t *testing.T) {
	now := time.Now()
	mapping := &PortMapping{
		InternalPort:  12345,
		ExternalPort:  54321,
		Protocol:      "UDP",
		Description:   "Test",
		LeaseDuration: 3600,
		CreatedAt:     now,
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
}

func TestGatewayStruct(t *testing.T) {
	gateway := &Gateway{
		Host:        "192.168.1.1",
		Port:        5000,
		ControlURL:  "http://192.168.1.1:5000/ctl/WANIPConn",
		ServiceType: "urn:schemas-upnp-org:service:WANIPConnection:1",
	}

	if gateway.Host != "192.168.1.1" {
		t.Errorf("Host = %s, want 192.168.1.1", gateway.Host)
	}
	if gateway.Port != 5000 {
		t.Errorf("Port = %d, want 5000", gateway.Port)
	}
}

func TestUPnPDiscoverTimeout(t *testing.T) {
	// Skip: Discover() sends SSDP multicast to 239.255.255.250:1900
	t.Skip("skipped: sends UPnP SSDP multicast on local network")
}

func TestPortMapperMapPortNoGateway(t *testing.T) {
	// Skip: MapPort triggers UPnP/NAT-PMP/PCP discovery on local network
	t.Skip("skipped: triggers network discovery protocols")
}

func TestUPnPClientGetMappings(t *testing.T) {
	client := NewUPnPClient(nil)

	mappings := client.GetMappings()
	if mappings == nil {
		t.Error("GetMappings returned nil")
	}
	if len(mappings) != 0 {
		t.Errorf("GetMappings should be empty, got %d", len(mappings))
	}
}

func TestUPnPClientLocalIP(t *testing.T) {
	client := NewUPnPClient(nil)

	// Before discovery, LocalIP should be nil
	if client.LocalIP() != nil {
		t.Error("LocalIP should be nil before discovery")
	}
}

func TestUPnPClientExternalIP(t *testing.T) {
	client := NewUPnPClient(nil)

	// Before discovery, ExternalIP should be nil
	if client.ExternalIP() != nil {
		t.Error("ExternalIP should be nil before discovery")
	}
}

func TestCandidateUPnPType(t *testing.T) {
	addr := &net.UDPAddr{
		IP:   net.ParseIP("203.0.113.1"),
		Port: 12345,
	}

	candidate := NewCandidate(CandidateUPnP, addr)

	if candidate.Type != CandidateUPnP {
		t.Errorf("Type = %v, want CandidateUPnP", candidate.Type)
	}

	if candidate.Type.String() != "upnp" {
		t.Errorf("Type.String() = %s, want upnp", candidate.Type.String())
	}

	// UPnP should have higher priority than STUN
	stunCandidate := NewCandidate(CandidateServerReflexive, addr)
	if candidate.Priority <= stunCandidate.Priority {
		t.Error("UPnP candidate should have higher priority than STUN")
	}
}
