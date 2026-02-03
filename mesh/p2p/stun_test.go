package p2p

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestBuildBindingRequest(t *testing.T) {
	txID := make([]byte, 12)
	for i := range txID {
		txID[i] = byte(i)
	}

	req := buildBindingRequest(txID)

	// Check length
	if len(req) != stunHeaderSize {
		t.Errorf("expected length %d, got %d", stunHeaderSize, len(req))
	}

	// Check message type
	msgType := binary.BigEndian.Uint16(req[0:2])
	if msgType != stunBindingRequest {
		t.Errorf("expected message type 0x%04x, got 0x%04x", stunBindingRequest, msgType)
	}

	// Check message length (should be 0 for simple request)
	msgLen := binary.BigEndian.Uint16(req[2:4])
	if msgLen != 0 {
		t.Errorf("expected message length 0, got %d", msgLen)
	}

	// Check magic cookie
	magic := binary.BigEndian.Uint32(req[4:8])
	if magic != stunMagicCookie {
		t.Errorf("expected magic cookie 0x%08x, got 0x%08x", stunMagicCookie, magic)
	}

	// Check transaction ID
	for i := 0; i < 12; i++ {
		if req[8+i] != txID[i] {
			t.Errorf("transaction ID mismatch at byte %d", i)
		}
	}
}

func TestParseBindingResponse(t *testing.T) {
	// Build a mock response with XOR-MAPPED-ADDRESS
	txID := make([]byte, 12)
	for i := range txID {
		txID[i] = byte(i)
	}

	// Build response header
	resp := make([]byte, stunHeaderSize+12) // header + XOR-MAPPED-ADDRESS attribute
	binary.BigEndian.PutUint16(resp[0:2], stunBindingResponse)
	binary.BigEndian.PutUint16(resp[2:4], 12) // attribute length
	binary.BigEndian.PutUint32(resp[4:8], stunMagicCookie)
	copy(resp[8:20], txID)

	// XOR-MAPPED-ADDRESS attribute
	// Type: 0x0020, Length: 8
	binary.BigEndian.PutUint16(resp[20:22], stunAttrXorMappedAddress)
	binary.BigEndian.PutUint16(resp[22:24], 8)
	resp[24] = 0 // reserved
	resp[25] = 1 // IPv4 family

	// Port XORed with magic cookie high bytes
	// Want port 12345, XOR with 0x2112 = 0x3027
	port := uint16(12345) ^ uint16(stunMagicCookie>>16)
	binary.BigEndian.PutUint16(resp[26:28], port)

	// IP XORed with magic cookie
	// Want 1.2.3.4
	ip := []byte{1, 2, 3, 4}
	magicBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(magicBytes, stunMagicCookie)
	for i := 0; i < 4; i++ {
		resp[28+i] = ip[i] ^ magicBytes[i]
	}

	addr, err := parseBindingResponse(resp, txID)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !addr.IP.Equal(net.IPv4(1, 2, 3, 4)) {
		t.Errorf("expected IP 1.2.3.4, got %s", addr.IP)
	}
	if addr.Port != 12345 {
		t.Errorf("expected port 12345, got %d", addr.Port)
	}
}

func TestParseBindingResponseInvalidPacket(t *testing.T) {
	// Too short
	_, err := parseBindingResponse([]byte{0, 1, 2}, nil)
	if err == nil {
		t.Error("expected error for short packet")
	}

	// Wrong message type
	resp := make([]byte, stunHeaderSize)
	binary.BigEndian.PutUint16(resp[0:2], 0x9999)
	binary.BigEndian.PutUint32(resp[4:8], stunMagicCookie)

	_, err = parseBindingResponse(resp, nil)
	if err == nil {
		t.Error("expected error for wrong message type")
	}

	// Wrong magic cookie
	resp2 := make([]byte, stunHeaderSize)
	binary.BigEndian.PutUint16(resp2[0:2], stunBindingResponse)
	binary.BigEndian.PutUint32(resp2[4:8], 0x12345678)

	_, err = parseBindingResponse(resp2, nil)
	if err == nil {
		t.Error("expected error for wrong magic cookie")
	}
}

func TestParseMappedAddress(t *testing.T) {
	// Test IPv4 without XOR
	data := make([]byte, 8)
	data[0] = 0    // reserved
	data[1] = 0x01 // IPv4
	binary.BigEndian.PutUint16(data[2:4], 8080)
	copy(data[4:8], net.IPv4(192, 168, 1, 1).To4())

	addr := parseMappedAddress(data, nil)
	if addr == nil {
		t.Fatal("expected non-nil address")
	}
	if !addr.IP.Equal(net.IPv4(192, 168, 1, 1)) {
		t.Errorf("expected 192.168.1.1, got %s", addr.IP)
	}
	if addr.Port != 8080 {
		t.Errorf("expected port 8080, got %d", addr.Port)
	}
}

func TestDefaultSTUNServers(t *testing.T) {
	servers := DefaultSTUNServers()
	if len(servers) == 0 {
		t.Error("expected non-empty default STUN servers")
	}

	// Check that all servers have valid host:port format
	for _, s := range servers {
		_, port, err := net.SplitHostPort(s)
		if err != nil {
			t.Errorf("invalid STUN server address %q: %v", s, err)
		}
		if port == "" {
			t.Errorf("STUN server %q missing port", s)
		}
	}
}
