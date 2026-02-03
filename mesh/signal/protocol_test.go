package signal

import (
	"net"
	"testing"
)

func TestMessageEncodeDecode(t *testing.T) {
	original := &Message{
		Type:    SignalCandidate,
		SrcVIP:  net.IPv4(10, 7, 0, 2),
		DstVIP:  net.IPv4(10, 7, 0, 3),
		Payload: []byte("test payload"),
	}

	encoded := original.Encode()

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.Type != original.Type {
		t.Errorf("Type: got %d, want %d", decoded.Type, original.Type)
	}
	if !decoded.SrcVIP.Equal(original.SrcVIP) {
		t.Errorf("SrcVIP: got %v, want %v", decoded.SrcVIP, original.SrcVIP)
	}
	if !decoded.DstVIP.Equal(original.DstVIP) {
		t.Errorf("DstVIP: got %v, want %v", decoded.DstVIP, original.DstVIP)
	}
	if string(decoded.Payload) != string(original.Payload) {
		t.Errorf("Payload: got %q, want %q", decoded.Payload, original.Payload)
	}
}

func TestMessageEncodeDecodeEmptyPayload(t *testing.T) {
	original := &Message{
		Type:   SignalPing,
		SrcVIP: net.IPv4(10, 7, 0, 2),
		DstVIP: net.IPv4(10, 7, 0, 3),
	}

	encoded := original.Encode()
	if len(encoded) != HeaderSize {
		t.Errorf("expected length %d for empty payload, got %d", HeaderSize, len(encoded))
	}

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.Type != original.Type {
		t.Errorf("Type mismatch")
	}
	if len(decoded.Payload) != 0 {
		t.Errorf("expected empty payload, got %d bytes", len(decoded.Payload))
	}
}

func TestDecodeInvalidMessage(t *testing.T) {
	// Too short
	_, err := Decode([]byte{1, 2, 3})
	if err == nil {
		t.Error("expected error for short message")
	}

	// Payload length exceeds data
	msg := make([]byte, HeaderSize)
	msg[9] = 0xFF // high byte of length
	msg[10] = 0xFF // low byte of length (65535)
	_, err = Decode(msg)
	if err == nil {
		t.Error("expected error for oversized payload length")
	}
}

func TestCandidateInfoEncodeDecode(t *testing.T) {
	original := &CandidateInfo{
		Type:        1,
		IP:          net.IPv4(192, 168, 1, 1),
		Port:        12345,
		Priority:    0x7E0000FF,
		RelatedIP:   net.IPv4(10, 0, 0, 1),
		RelatedPort: 54321,
	}

	encoded := EncodeCandidate(original)

	decoded, err := DecodeCandidate(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.Type != original.Type {
		t.Errorf("Type: got %d, want %d", decoded.Type, original.Type)
	}
	if !decoded.IP.Equal(original.IP) {
		t.Errorf("IP: got %v, want %v", decoded.IP, original.IP)
	}
	if decoded.Port != original.Port {
		t.Errorf("Port: got %d, want %d", decoded.Port, original.Port)
	}
	if decoded.Priority != original.Priority {
		t.Errorf("Priority: got %d, want %d", decoded.Priority, original.Priority)
	}
	if !decoded.RelatedIP.Equal(original.RelatedIP) {
		t.Errorf("RelatedIP: got %v, want %v", decoded.RelatedIP, original.RelatedIP)
	}
	if decoded.RelatedPort != original.RelatedPort {
		t.Errorf("RelatedPort: got %d, want %d", decoded.RelatedPort, original.RelatedPort)
	}
}

func TestCandidateInfoWithoutRelated(t *testing.T) {
	original := &CandidateInfo{
		Type:     0,
		IP:       net.IPv4(192, 168, 1, 1),
		Port:     12345,
		Priority: 100,
	}

	encoded := EncodeCandidate(original)
	if len(encoded) != CandidateInfoSize {
		t.Errorf("expected length %d without related, got %d", CandidateInfoSize, len(encoded))
	}

	decoded, err := DecodeCandidate(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.RelatedIP != nil {
		t.Errorf("expected nil RelatedIP, got %v", decoded.RelatedIP)
	}
}

func TestEncodDecodeCandidates(t *testing.T) {
	candidates := []*CandidateInfo{
		{Type: 0, IP: net.IPv4(10, 0, 0, 1), Port: 1000, Priority: 100},
		{Type: 1, IP: net.IPv4(1, 2, 3, 4), Port: 2000, Priority: 80},
	}

	encoded := EncodeCandidates(candidates)

	decoded, err := DecodeCandidates(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(decoded) != len(candidates) {
		t.Errorf("expected %d candidates, got %d", len(candidates), len(decoded))
	}

	for i, c := range decoded {
		if c.Type != candidates[i].Type {
			t.Errorf("candidate %d Type mismatch", i)
		}
		if !c.IP.Equal(candidates[i].IP) {
			t.Errorf("candidate %d IP mismatch", i)
		}
		if c.Port != candidates[i].Port {
			t.Errorf("candidate %d Port mismatch", i)
		}
	}
}

func TestConnectInfoEncodeDecode(t *testing.T) {
	var pubKey [32]byte
	for i := range pubKey {
		pubKey[i] = byte(i)
	}

	original := &ConnectInfo{PublicKey: pubKey}

	encoded := EncodeConnectInfo(original)
	if len(encoded) != 32 {
		t.Errorf("expected length 32, got %d", len(encoded))
	}

	decoded, err := DecodeConnectInfo(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.PublicKey != original.PublicKey {
		t.Error("public key mismatch")
	}
}

func TestNewCandidateMessage(t *testing.T) {
	srcVIP := net.IPv4(10, 7, 0, 2)
	dstVIP := net.IPv4(10, 7, 0, 3)
	candidates := []*CandidateInfo{
		{Type: 0, IP: net.IPv4(10, 0, 0, 1), Port: 1000, Priority: 100},
	}

	msg := NewCandidateMessage(srcVIP, dstVIP, candidates)

	if msg.Type != SignalCandidate {
		t.Errorf("expected SignalCandidate, got %d", msg.Type)
	}
	if !msg.SrcVIP.Equal(srcVIP) {
		t.Error("SrcVIP mismatch")
	}
	if !msg.DstVIP.Equal(dstVIP) {
		t.Error("DstVIP mismatch")
	}
	if len(msg.Payload) == 0 {
		t.Error("expected non-empty payload")
	}
}

func TestNewConnectMessage(t *testing.T) {
	srcVIP := net.IPv4(10, 7, 0, 2)
	dstVIP := net.IPv4(10, 7, 0, 3)
	var pubKey [32]byte

	msg := NewConnectMessage(srcVIP, dstVIP, pubKey)

	if msg.Type != SignalConnect {
		t.Errorf("expected SignalConnect, got %d", msg.Type)
	}
	if len(msg.Payload) != 32 {
		t.Errorf("expected 32 byte payload, got %d", len(msg.Payload))
	}
}

func TestNewDisconnectMessage(t *testing.T) {
	msg := NewDisconnectMessage(net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 2))
	if msg.Type != SignalDisconnect {
		t.Errorf("expected SignalDisconnect, got %d", msg.Type)
	}
}

func TestNewPingPongMessages(t *testing.T) {
	ping := NewPingMessage(net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 2))
	if ping.Type != SignalPing {
		t.Errorf("expected SignalPing, got %d", ping.Type)
	}

	pong := NewPongMessage(net.IPv4(10, 0, 0, 2), net.IPv4(10, 0, 0, 1))
	if pong.Type != SignalPong {
		t.Errorf("expected SignalPong, got %d", pong.Type)
	}
}

func TestNewErrorMessage(t *testing.T) {
	msg := NewErrorMessage(net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 2), "test error")
	if msg.Type != SignalError {
		t.Errorf("expected SignalError, got %d", msg.Type)
	}
	if string(msg.Payload) != "test error" {
		t.Errorf("payload mismatch: got %q", msg.Payload)
	}
}
