package p2p

import (
	"net"
	"testing"
)

func TestParseSpoofMode(t *testing.T) {
	tests := []struct {
		input    string
		expected SpoofMode
	}{
		{"dns", SpoofDNS},
		{"53", SpoofDNS},
		{"https", SpoofHTTPS},
		{"443", SpoofHTTPS},
		{"ike", SpoofIKE},
		{"500", SpoofIKE},
		{"ipsec-nat", SpoofIPSecNAT},
		{"4500", SpoofIPSecNAT},
		{"none", SpoofNone},
		{"", SpoofNone},
		{"custom", SpoofCustom},
		{"12345", SpoofCustom},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseSpoofMode(tt.input)
			if got != tt.expected {
				t.Errorf("ParseSpoofMode(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSpoofConfigGetPort(t *testing.T) {
	tests := []struct {
		name     string
		config   *SpoofConfig
		expected int
	}{
		{"DNS mode", &SpoofConfig{Mode: SpoofDNS}, 53},
		{"HTTPS mode", &SpoofConfig{Mode: SpoofHTTPS}, 443},
		{"IKE mode", &SpoofConfig{Mode: SpoofIKE}, 500},
		{"IPSec NAT mode", &SpoofConfig{Mode: SpoofIPSecNAT}, 4500},
		{"None mode", &SpoofConfig{Mode: SpoofNone}, 0},
		{"Custom port", &SpoofConfig{Mode: SpoofCustom, CustomPort: 8888}, 8888},
		{"Custom invalid port", &SpoofConfig{Mode: SpoofCustom, CustomPort: 0}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetPort()
			if got != tt.expected {
				t.Errorf("GetPort() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestSpoofModeString(t *testing.T) {
	tests := []struct {
		mode     SpoofMode
		expected string
	}{
		{SpoofNone, "none"},
		{SpoofDNS, "dns"},
		{SpoofHTTPS, "https"},
		{SpoofIKE, "ike"},
		{SpoofIPSecNAT, "ipsec-nat"},
		{SpoofCustom, "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRequiresPrivilege(t *testing.T) {
	tests := []struct {
		name     string
		config   *SpoofConfig
		expected bool
	}{
		{"DNS requires privilege", &SpoofConfig{Mode: SpoofDNS}, true},
		{"HTTPS requires privilege", &SpoofConfig{Mode: SpoofHTTPS}, true},
		{"IKE requires privilege", &SpoofConfig{Mode: SpoofIKE}, true},
		{"None doesn't require privilege", &SpoofConfig{Mode: SpoofNone}, false},
		{"High custom port doesn't require privilege", &SpoofConfig{Mode: SpoofCustom, CustomPort: 8888}, false},
		{"Low custom port requires privilege", &SpoofConfig{Mode: SpoofCustom, CustomPort: 80}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.RequiresPrivilege()
			if got != tt.expected {
				t.Errorf("RequiresPrivilege() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCanBindPort(t *testing.T) {
	// Test binding to a random high port (should always succeed)
	if !CanBindPort(0) {
		t.Error("CanBindPort(0) should return true for random port")
	}

	// Test binding to port 53 (likely requires root)
	// Just test that it doesn't panic
	_ = CanBindPort(53)
}

func TestCreateSpoofedConnNone(t *testing.T) {
	// Test with no spoofing (random port)
	conn, err := CreateSpoofedConn(nil)
	if err != nil {
		t.Fatalf("CreateSpoofedConn(nil) failed: %v", err)
	}
	defer conn.Close()

	// Should have a random port > 0
	port := conn.LocalAddr().(*net.UDPAddr).Port
	if port <= 0 {
		t.Errorf("Expected random port > 0, got %d", port)
	}
}

func TestCreateSpoofedConnHighPort(t *testing.T) {
	// Test with a high custom port
	cfg := &SpoofConfig{
		Mode:       SpoofCustom,
		CustomPort: 0, // Use random port
	}

	conn, err := CreateSpoofedConn(cfg)
	if err != nil {
		t.Fatalf("CreateSpoofedConn failed: %v", err)
	}
	defer conn.Close()
}

func TestGetSpoofInfo(t *testing.T) {
	conn, err := CreateSpoofedConn(nil)
	if err != nil {
		t.Fatalf("CreateSpoofedConn failed: %v", err)
	}
	defer conn.Close()

	info := GetSpoofInfo(conn, SpoofNone)
	if info == nil {
		t.Fatal("GetSpoofInfo returned nil")
	}

	if info.Mode != SpoofNone {
		t.Errorf("Mode = %v, want %v", info.Mode, SpoofNone)
	}

	if info.LocalPort <= 0 {
		t.Errorf("LocalPort = %d, want > 0", info.LocalPort)
	}
}
