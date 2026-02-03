package p2p

import (
	"net"
	"testing"
)

func TestHolePunchPacketEncodeDecode(t *testing.T) {
	original := &HolePunchPacket{
		Type:      HolePunchRequest,
		SrcVIP:    net.IPv4(10, 7, 0, 2),
		DstVIP:    net.IPv4(10, 7, 0, 3),
		Timestamp: 1234567890,
		Seq:       42,
	}

	encoded := original.Encode()
	if len(encoded) != HolePunchPacketSize {
		t.Errorf("expected encoded size %d, got %d", HolePunchPacketSize, len(encoded))
	}

	// Check magic
	if encoded[0] != 'H' || encoded[1] != 'O' || encoded[2] != 'L' || encoded[3] != 'E' {
		t.Error("magic bytes incorrect")
	}

	decoded, err := DecodeHolePunchPacket(encoded)
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
	if decoded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp: got %d, want %d", decoded.Timestamp, original.Timestamp)
	}
	if decoded.Seq != original.Seq {
		t.Errorf("Seq: got %d, want %d", decoded.Seq, original.Seq)
	}
}

func TestDecodeHolePunchPacketInvalid(t *testing.T) {
	// Too short
	_, err := DecodeHolePunchPacket([]byte{1, 2, 3})
	if err == nil {
		t.Error("expected error for short packet")
	}

	// Wrong magic
	wrongMagic := make([]byte, HolePunchPacketSize)
	copy(wrongMagic[0:4], []byte("NOPE"))
	_, err = DecodeHolePunchPacket(wrongMagic)
	if err == nil {
		t.Error("expected error for wrong magic")
	}
}

func TestIsHolePunchPacket(t *testing.T) {
	tests := []struct {
		data     []byte
		expected bool
	}{
		{[]byte{'H', 'O', 'L', 'E', 1, 2, 3}, true},
		{[]byte{'H', 'O', 'L', 'E'}, true},
		{[]byte{'N', 'O', 'P', 'E'}, false},
		{[]byte{'H', 'O', 'L'}, false},
		{[]byte{}, false},
		{nil, false},
	}

	for i, tt := range tests {
		result := IsHolePunchPacket(tt.data)
		if result != tt.expected {
			t.Errorf("test %d: IsHolePunchPacket(%v) = %v, want %v", i, tt.data, result, tt.expected)
		}
	}
}

func TestHolePunchPacketTypes(t *testing.T) {
	types := []byte{HolePunchRequest, HolePunchResponse, HolePunchAck}

	for _, typ := range types {
		pkt := &HolePunchPacket{
			Type:   typ,
			SrcVIP: net.IPv4(10, 0, 0, 1),
			DstVIP: net.IPv4(10, 0, 0, 2),
		}

		encoded := pkt.Encode()
		decoded, err := DecodeHolePunchPacket(encoded)
		if err != nil {
			t.Errorf("type %d: decode failed: %v", typ, err)
		}
		if decoded.Type != typ {
			t.Errorf("type mismatch: got %d, want %d", decoded.Type, typ)
		}
	}
}

func TestCandidatesToAddrs(t *testing.T) {
	candidates := []*Candidate{
		NewCandidate(CandidateHost, &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 1000}),
		NewCandidate(CandidateHost, &net.UDPAddr{IP: net.IPv4(10, 0, 0, 2), Port: 2000}),
	}

	addrs := CandidatesToAddrs(candidates)
	if len(addrs) != 2 {
		t.Errorf("expected 2 addresses, got %d", len(addrs))
	}

	for i, addr := range addrs {
		if !addr.IP.Equal(candidates[i].Addr.IP) {
			t.Errorf("addr %d IP mismatch", i)
		}
		if addr.Port != candidates[i].Addr.Port {
			t.Errorf("addr %d port mismatch", i)
		}
	}
}

func TestAddrsToCandidates(t *testing.T) {
	addrs := []*net.UDPAddr{
		{IP: net.IPv4(10, 0, 0, 1), Port: 1000},
		{IP: net.IPv4(10, 0, 0, 2), Port: 2000},
	}

	candidates := AddrsToCandidates(addrs)
	if len(candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(candidates))
	}

	for i, c := range candidates {
		if c.Type != CandidateHost {
			t.Errorf("candidate %d: expected CandidateHost, got %v", i, c.Type)
		}
		if !c.Addr.IP.Equal(addrs[i].IP) {
			t.Errorf("candidate %d IP mismatch", i)
		}
	}
}
