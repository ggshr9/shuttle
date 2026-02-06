package p2p

import (
	"net"
	"testing"
	"time"
)

func TestNewTURNClient(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "turn.example.com:3478",
		Username: "user",
		Password: "pass",
	}

	client, err := NewTURNClient(cfg, nil)
	if err != nil {
		t.Fatalf("NewTURNClient failed: %v", err)
	}

	if client == nil {
		t.Fatal("NewTURNClient returned nil")
	}

	if client.username != "user" {
		t.Errorf("username = %q, want %q", client.username, "user")
	}

	if client.password != "pass" {
		t.Errorf("password = %q, want %q", client.password, "pass")
	}
}

func TestNewTURNClientNilConfig(t *testing.T) {
	_, err := NewTURNClient(nil, nil)
	if err == nil {
		t.Error("Expected error for nil config")
	}
}

func TestNewTURNClientInvalidServer(t *testing.T) {
	cfg := &TURNConfig{
		Server: "invalid:server:address",
	}

	_, err := NewTURNClient(cfg, nil)
	if err == nil {
		t.Error("Expected error for invalid server address")
	}
}

func TestTURNClientIsAllocated(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
	}

	client, _ := NewTURNClient(cfg, nil)

	if client.IsAllocated() {
		t.Error("Should not be allocated before Allocate()")
	}
}

func TestTURNClientRelayAddr(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
	}

	client, _ := NewTURNClient(cfg, nil)

	if client.RelayAddr() != nil {
		t.Error("RelayAddr should be nil before allocation")
	}
}

func TestTURNClientMappedAddr(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
	}

	client, _ := NewTURNClient(cfg, nil)

	if client.MappedAddr() != nil {
		t.Error("MappedAddr should be nil before allocation")
	}
}

func TestTURNClientOnData(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
	}

	client, _ := NewTURNClient(cfg, nil)

	client.OnData(func(from *net.UDPAddr, data []byte) {
		// Callback set
	})

	if client.onData == nil {
		t.Error("OnData callback should be set")
	}
}

func TestTURNClientCloseNotAllocated(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
	}

	client, _ := NewTURNClient(cfg, nil)

	// Should not panic or error
	err := client.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestTURNConfigStruct(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "turn.example.com:3478",
		Username: "testuser",
		Password: "testpass",
		Realm:    "testrealm",
	}

	if cfg.Server != "turn.example.com:3478" {
		t.Errorf("Server = %q, want %q", cfg.Server, "turn.example.com:3478")
	}

	if cfg.Username != "testuser" {
		t.Errorf("Username = %q, want %q", cfg.Username, "testuser")
	}

	if cfg.Realm != "testrealm" {
		t.Errorf("Realm = %q, want %q", cfg.Realm, "testrealm")
	}
}

func TestBuildAllocateRequest(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
	}

	client, _ := NewTURNClient(cfg, nil)

	req := client.buildAllocateRequest(false)

	if len(req) < 20 {
		t.Errorf("Request too short: %d bytes", len(req))
	}

	// Check magic cookie
	cookie := uint32(req[4])<<24 | uint32(req[5])<<16 | uint32(req[6])<<8 | uint32(req[7])
	if cookie != 0x2112A442 {
		t.Errorf("Magic cookie = 0x%08x, want 0x2112A442", cookie)
	}
}

func TestBuildRefreshRequest(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
		Realm:    "realm",
	}

	client, _ := NewTURNClient(cfg, nil)
	client.nonce = "testnonce"

	req := client.buildRefreshRequest(600)

	if len(req) < 20 {
		t.Errorf("Request too short: %d bytes", len(req))
	}
}

func TestBuildSendIndication(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
	}

	client, _ := NewTURNClient(cfg, nil)

	peerAddr := &net.UDPAddr{
		IP:   net.ParseIP("192.168.1.100"),
		Port: 12345,
	}

	data := []byte("test data")
	req := client.buildSendIndication(peerAddr, data)

	if len(req) < 20+8+4+len(data) {
		t.Errorf("Indication too short: %d bytes", len(req))
	}
}

func TestAddXorPeerAddress(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
	}

	client, _ := NewTURNClient(cfg, nil)

	buf := make([]byte, 20)
	addr := &net.UDPAddr{
		IP:   net.ParseIP("192.168.1.1"),
		Port: 3000,
	}

	n := client.addXorPeerAddress(buf, 0, addr)

	if n != 12 { // 4 header + 8 address
		t.Errorf("addXorPeerAddress returned %d, want 12", n)
	}

	// Check attribute type
	attrType := uint16(buf[0])<<8 | uint16(buf[1])
	if attrType != turnAttrXorPeerAddress {
		t.Errorf("Attribute type = 0x%04x, want 0x%04x", attrType, turnAttrXorPeerAddress)
	}
}

func TestCalculateKey(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
		Realm:    "realm",
	}

	client, _ := NewTURNClient(cfg, nil)

	key := client.calculateKey()

	if len(key) != 16 { // MD5 produces 16 bytes
		t.Errorf("Key length = %d, want 16", len(key))
	}
}

func TestParseXorAddress(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
	}

	client, _ := NewTURNClient(cfg, nil)

	// Construct a valid XOR-MAPPED-ADDRESS
	// Magic cookie bytes: 0x21, 0x12, 0xA4, 0x42
	// Port 3000 (0x0BB8) XOR 0x2112 = 0x2AAA
	// IP 192.168.1.1 XOR magic cookie bytes:
	// 192 (0xC0) ^ 0x21 = 0xE1
	// 168 (0xA8) ^ 0x12 = 0xBA
	// 1   (0x01) ^ 0xA4 = 0xA5
	// 1   (0x01) ^ 0x42 = 0x43
	data := []byte{
		0x00, // Reserved
		0x01, // Family (IPv4)
		0x2A, 0xAA, // XOR port (3000 ^ 0x2112)
		0xE1, 0xBA, 0xA5, 0x43, // XOR IP
	}

	msg := make([]byte, 20) // Dummy message header

	addr := client.parseXorAddress(data, msg)

	if addr == nil {
		t.Fatal("parseXorAddress returned nil")
	}

	if addr.Port != 3000 {
		t.Errorf("Port = %d, want 3000", addr.Port)
	}

	expectedIP := net.ParseIP("192.168.1.1").To4()
	if !addr.IP.Equal(expectedIP) {
		t.Errorf("IP = %v, want %v", addr.IP, expectedIP)
	}
}

func TestIsSuccessResponse(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		method   int
		expected bool
	}{
		{
			name:     "success allocate",
			data:     []byte{0x01, 0x03}, // Allocate success
			method:   turnMethodAllocate,
			expected: true,
		},
		{
			name:     "error response",
			data:     []byte{0x01, 0x13}, // Allocate error
			method:   turnMethodAllocate,
			expected: false,
		},
		{
			name:     "too short",
			data:     []byte{0x01},
			method:   turnMethodAllocate,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSuccessResponse(tt.data, tt.method)
			if result != tt.expected {
				t.Errorf("isSuccessResponse() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestTURNConstants(t *testing.T) {
	if turnMethodAllocate != 0x0003 {
		t.Errorf("turnMethodAllocate = 0x%04x, want 0x0003", turnMethodAllocate)
	}

	if turnMethodRefresh != 0x0004 {
		t.Errorf("turnMethodRefresh = 0x%04x, want 0x0004", turnMethodRefresh)
	}

	if turnMethodSend != 0x0006 {
		t.Errorf("turnMethodSend = 0x%04x, want 0x0006", turnMethodSend)
	}

	if turnMethodData != 0x0007 {
		t.Errorf("turnMethodData = 0x%04x, want 0x0007", turnMethodData)
	}

	if turnTransportUDP != 17 {
		t.Errorf("turnTransportUDP = %d, want 17", turnTransportUDP)
	}

	if turnAttrXorRelayedAddr != 0x0016 {
		t.Errorf("turnAttrXorRelayedAddr = 0x%04x, want 0x0016", turnAttrXorRelayedAddr)
	}
}

func TestDefaultTURNServers(t *testing.T) {
	servers := DefaultTURNServers()

	// Currently empty (placeholder)
	t.Logf("Default TURN servers: %v", servers)
}

func TestTURNClientSendNotAllocated(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
	}

	client, _ := NewTURNClient(cfg, nil)

	peerAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 1234}

	err := client.Send(peerAddr, []byte("test"))
	if err == nil {
		t.Error("Send should fail when not allocated")
	}
}

func TestTURNClientCreatePermissionNotAllocated(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
	}

	client, _ := NewTURNClient(cfg, nil)

	peerAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 1234}

	err := client.CreatePermission(nil, peerAddr)
	if err == nil {
		t.Error("CreatePermission should fail when not allocated")
	}
}

func TestParseErrorResponse(t *testing.T) {
	// Build a sample error response with ERROR-CODE attribute
	data := make([]byte, 100)
	// Header
	data[0] = 0x01 // Type high byte
	data[1] = 0x13 // Type low byte (error)
	// Length = 8 (error code attr)
	data[2] = 0x00
	data[3] = 0x08
	// Magic cookie
	data[4] = 0x21
	data[5] = 0x12
	data[6] = 0xA4
	data[7] = 0x42

	// ERROR-CODE attribute at offset 20
	data[20] = 0x00
	data[21] = 0x09 // ERROR-CODE type
	data[22] = 0x00
	data[23] = 0x08 // Length
	// Reserved + class (4) + number (01) = 401
	data[24] = 0x00
	data[25] = 0x00
	data[26] = 0x04 // Class
	data[27] = 0x01 // Number
	// Reason phrase "Unauth"
	data[28] = 'U'
	data[29] = 'n'
	data[30] = 'a'
	data[31] = 'u'

	code, reason := parseErrorResponse(data)

	if code != 401 {
		t.Errorf("Error code = %d, want 401", code)
	}

	if reason != "Unau" {
		t.Errorf("Reason = %q, want %q", reason, "Unau")
	}
}

func TestTURNClientPermissions(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
	}

	client, _ := NewTURNClient(cfg, nil)

	// Manually set allocated state
	client.allocated = true

	// Add a permission
	client.permissions["192.168.1.1"] = time.Now().Add(5 * time.Minute)

	if len(client.permissions) != 1 {
		t.Errorf("permissions length = %d, want 1", len(client.permissions))
	}
}

func TestBuildCreatePermissionRequest(t *testing.T) {
	cfg := &TURNConfig{
		Server:   "127.0.0.1:3478",
		Username: "user",
		Password: "pass",
		Realm:    "realm",
	}

	client, _ := NewTURNClient(cfg, nil)
	client.nonce = "testnonce"

	peerAddr := &net.UDPAddr{
		IP:   net.ParseIP("192.168.1.100"),
		Port: 12345,
	}

	req := client.buildCreatePermissionRequest(peerAddr)

	if len(req) < 20 {
		t.Errorf("Request too short: %d bytes", len(req))
	}

	// Check message type (should be CreatePermission request)
	msgType := uint16(req[0])<<8 | uint16(req[1])
	expected := uint16(turnMethodCreatePerm | turnClassRequest)
	if msgType != expected {
		t.Errorf("Message type = 0x%04x, want 0x%04x", msgType, expected)
	}
}
