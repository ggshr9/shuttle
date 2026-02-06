package p2p

import (
	"context"
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
	ip, err := getOutboundIP()
	if err != nil {
		t.Skipf("getOutboundIP failed (no network): %v", err)
	}

	if ip == nil {
		t.Error("getOutboundIP returned nil IP")
	}

	if ip.IsLoopback() {
		t.Error("getOutboundIP returned loopback address")
	}
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
	client := NewUPnPClient(nil)

	// Discovery with very short timeout should fail or timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := client.Discover(ctx)
	// We expect either timeout or "not found" - both are acceptable
	if err == nil {
		// UPnP device was found (running on real network with UPnP router)
		t.Log("UPnP device found on network")
	}
}

func TestPortMapperMapPortNoGateway(t *testing.T) {
	pm := NewPortMapper(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := pm.MapPort(ctx, 12345, 0)
	if err == nil {
		// Mapping succeeded (real UPnP router on network)
		t.Log("Port mapping succeeded on real network")
		pm.Close()
	}
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
