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

func TestICERestartInfoEncodeDecode(t *testing.T) {
	original := &ICERestartInfo{
		Reason:           ICERestartReasonNetworkChange,
		Generation:       3,
		UsernameFragment: "abcd1234",
		Password:         "password_that_is_long_enough",
	}

	encoded := EncodeICERestartInfo(original)

	decoded, err := DecodeICERestartInfo(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.Reason != original.Reason {
		t.Errorf("Reason: got %d, want %d", decoded.Reason, original.Reason)
	}
	if decoded.Generation != original.Generation {
		t.Errorf("Generation: got %d, want %d", decoded.Generation, original.Generation)
	}
	if decoded.UsernameFragment != original.UsernameFragment {
		t.Errorf("UsernameFragment: got %q, want %q", decoded.UsernameFragment, original.UsernameFragment)
	}
	if decoded.Password != original.Password {
		t.Errorf("Password: got %q, want %q", decoded.Password, original.Password)
	}
}

func TestICERestartInfoEncodeDecodeEmpty(t *testing.T) {
	original := &ICERestartInfo{
		Reason:           ICERestartReasonManual,
		Generation:       0,
		UsernameFragment: "",
		Password:         "",
	}

	encoded := EncodeICERestartInfo(original)

	decoded, err := DecodeICERestartInfo(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.Reason != original.Reason {
		t.Errorf("Reason mismatch")
	}
	if decoded.UsernameFragment != "" {
		t.Errorf("expected empty ufrag, got %q", decoded.UsernameFragment)
	}
	if decoded.Password != "" {
		t.Errorf("expected empty pwd, got %q", decoded.Password)
	}
}

func TestICERestartInfoDecodeTooShort(t *testing.T) {
	// Too short
	_, err := DecodeICERestartInfo([]byte{1, 2, 3})
	if err == nil {
		t.Error("expected error for short data")
	}
}

func TestICERestartInfoDecodeTruncatedUfrag(t *testing.T) {
	// Valid header but ufrag length claims more than available
	data := []byte{
		ICERestartReasonManual, // Reason
		0x00, 0x01,             // Generation
		0xFF, // Ufrag length (255, but no data follows)
	}
	_, err := DecodeICERestartInfo(data)
	if err == nil {
		t.Error("expected error for truncated ufrag")
	}
}

func TestICERestartInfoDecodeTruncatedPwd(t *testing.T) {
	// Valid ufrag but pwd length claims more than available
	data := []byte{
		ICERestartReasonManual, // Reason
		0x00, 0x01,             // Generation
		0x04,             // Ufrag length
		'a', 'b', 'c', 'd', // Ufrag
		0xFF, // Pwd length (255, but no data follows)
	}
	_, err := DecodeICERestartInfo(data)
	if err == nil {
		t.Error("expected error for truncated pwd")
	}
}

func TestNewICERestartMessage(t *testing.T) {
	srcVIP := net.IPv4(10, 7, 0, 2)
	dstVIP := net.IPv4(10, 7, 0, 3)
	info := &ICERestartInfo{
		Reason:           ICERestartReasonQualityDegraded,
		Generation:       5,
		UsernameFragment: "testufrag",
		Password:         "testpassword12345678901234",
	}

	msg := NewICERestartMessage(srcVIP, dstVIP, info)

	if msg.Type != SignalICERestart {
		t.Errorf("expected SignalICERestart (0x%02x), got 0x%02x", SignalICERestart, msg.Type)
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

	// Verify payload can be decoded
	decoded, err := DecodeICERestartInfo(msg.Payload)
	if err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}

	if decoded.Reason != info.Reason {
		t.Errorf("decoded Reason: got %d, want %d", decoded.Reason, info.Reason)
	}
	if decoded.Generation != info.Generation {
		t.Errorf("decoded Generation: got %d, want %d", decoded.Generation, info.Generation)
	}
	if decoded.UsernameFragment != info.UsernameFragment {
		t.Errorf("decoded UsernameFragment: got %q, want %q", decoded.UsernameFragment, info.UsernameFragment)
	}
	if decoded.Password != info.Password {
		t.Errorf("decoded Password: got %q, want %q", decoded.Password, info.Password)
	}
}

func TestICERestartReasonConstants(t *testing.T) {
	// Verify reason constants are unique
	reasons := []byte{
		ICERestartReasonManual,
		ICERestartReasonNetworkChange,
		ICERestartReasonQualityDegraded,
		ICERestartReasonAllPairsFailed,
		ICERestartReasonTimeout,
		ICERestartReasonRemoteRequest,
	}

	seen := make(map[byte]bool)
	for _, r := range reasons {
		if seen[r] {
			t.Errorf("duplicate reason constant: 0x%02x", r)
		}
		seen[r] = true
	}
}

func TestSignalICERestartConstant(t *testing.T) {
	// Verify SignalICERestart has a unique value
	if SignalICERestart == SignalCandidate ||
		SignalICERestart == SignalConnect ||
		SignalICERestart == SignalConnectAck ||
		SignalICERestart == SignalDisconnect ||
		SignalICERestart == SignalPing ||
		SignalICERestart == SignalPong ||
		SignalICERestart == SignalError {
		t.Error("SignalICERestart should have a unique value")
	}

	// Verify it's 0x09
	if SignalICERestart != 0x09 {
		t.Errorf("SignalICERestart = 0x%02x, want 0x09", SignalICERestart)
	}
}

func TestTrickleCandidateEncodeDecode(t *testing.T) {
	original := &TrickleCandidateInfo{
		Candidate: &CandidateInfo{
			Type:     1,
			IP:       net.IPv4(192, 168, 1, 100),
			Port:     12345,
			Priority: 0x7E0000FF,
		},
		Generation: 3,
		MLineIndex: 0,
	}

	encoded := EncodeTrickleCandidate(original)

	decoded, err := DecodeTrickleCandidate(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.Generation != original.Generation {
		t.Errorf("Generation: got %d, want %d", decoded.Generation, original.Generation)
	}
	if decoded.MLineIndex != original.MLineIndex {
		t.Errorf("MLineIndex: got %d, want %d", decoded.MLineIndex, original.MLineIndex)
	}
	if decoded.Candidate.Type != original.Candidate.Type {
		t.Errorf("Candidate.Type: got %d, want %d", decoded.Candidate.Type, original.Candidate.Type)
	}
	if !decoded.Candidate.IP.Equal(original.Candidate.IP) {
		t.Errorf("Candidate.IP: got %v, want %v", decoded.Candidate.IP, original.Candidate.IP)
	}
	if decoded.Candidate.Port != original.Candidate.Port {
		t.Errorf("Candidate.Port: got %d, want %d", decoded.Candidate.Port, original.Candidate.Port)
	}
}

func TestTrickleCandidateDecodeTooShort(t *testing.T) {
	_, err := DecodeTrickleCandidate([]byte{1, 2, 3})
	if err == nil {
		t.Error("expected error for short data")
	}
}

func TestTrickleCandidateDecodeTruncated(t *testing.T) {
	// Valid header but claims more data than present
	data := []byte{
		0x00, 0x01, // Generation
		0x00,       // MLineIndex
		0xFF,       // CandidateLen (255, but no data follows)
	}
	_, err := DecodeTrickleCandidate(data)
	if err == nil {
		t.Error("expected error for truncated data")
	}
}

func TestNewTrickleCandidateMessage(t *testing.T) {
	srcVIP := net.IPv4(10, 7, 0, 2)
	dstVIP := net.IPv4(10, 7, 0, 3)
	info := &TrickleCandidateInfo{
		Candidate: &CandidateInfo{
			Type:     0,
			IP:       net.IPv4(192, 168, 1, 1),
			Port:     10000,
			Priority: 100,
		},
		Generation: 1,
		MLineIndex: 0,
	}

	msg := NewTrickleCandidateMessage(srcVIP, dstVIP, info)

	if msg.Type != SignalTrickleCandidate {
		t.Errorf("expected SignalTrickleCandidate (0x%02x), got 0x%02x", SignalTrickleCandidate, msg.Type)
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

	// Verify payload can be decoded
	decoded, err := DecodeTrickleCandidate(msg.Payload)
	if err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}

	if decoded.Generation != info.Generation {
		t.Errorf("decoded Generation: got %d, want %d", decoded.Generation, info.Generation)
	}
}

func TestEndOfCandidatesEncodeDecode(t *testing.T) {
	original := &EndOfCandidatesInfo{
		Generation:     5,
		TotalCandidates: 12,
	}

	encoded := EncodeEndOfCandidates(original)
	if len(encoded) != 3 {
		t.Errorf("encoded length = %d, want 3", len(encoded))
	}

	decoded, err := DecodeEndOfCandidates(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.Generation != original.Generation {
		t.Errorf("Generation: got %d, want %d", decoded.Generation, original.Generation)
	}
	if decoded.TotalCandidates != original.TotalCandidates {
		t.Errorf("TotalCandidates: got %d, want %d", decoded.TotalCandidates, original.TotalCandidates)
	}
}

func TestEndOfCandidatesDecodeTooShort(t *testing.T) {
	_, err := DecodeEndOfCandidates([]byte{1, 2})
	if err == nil {
		t.Error("expected error for short data")
	}
}

func TestNewEndOfCandidatesMessage(t *testing.T) {
	srcVIP := net.IPv4(10, 7, 0, 2)
	dstVIP := net.IPv4(10, 7, 0, 3)
	info := &EndOfCandidatesInfo{
		Generation:     2,
		TotalCandidates: 5,
	}

	msg := NewEndOfCandidatesMessage(srcVIP, dstVIP, info)

	if msg.Type != SignalEndOfCandidates {
		t.Errorf("expected SignalEndOfCandidates (0x%02x), got 0x%02x", SignalEndOfCandidates, msg.Type)
	}
	if !msg.SrcVIP.Equal(srcVIP) {
		t.Error("SrcVIP mismatch")
	}
	if !msg.DstVIP.Equal(dstVIP) {
		t.Error("DstVIP mismatch")
	}
	if len(msg.Payload) != 3 {
		t.Errorf("payload length = %d, want 3", len(msg.Payload))
	}

	// Verify payload can be decoded
	decoded, err := DecodeEndOfCandidates(msg.Payload)
	if err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}

	if decoded.Generation != info.Generation {
		t.Errorf("decoded Generation: got %d, want %d", decoded.Generation, info.Generation)
	}
	if decoded.TotalCandidates != info.TotalCandidates {
		t.Errorf("decoded TotalCandidates: got %d, want %d", decoded.TotalCandidates, info.TotalCandidates)
	}
}

func TestTrickleSignalConstants(t *testing.T) {
	// Verify trickle constants are unique
	constants := []byte{
		SignalCandidate,
		SignalConnect,
		SignalConnectAck,
		SignalDisconnect,
		SignalPing,
		SignalPong,
		SignalICERestart,
		SignalTrickleCandidate,
		SignalEndOfCandidates,
		SignalWebRTCOffer,
		SignalWebRTCAnswer,
		SignalWebRTCICECandidate,
		SignalError,
	}

	seen := make(map[byte]bool)
	for _, c := range constants {
		if seen[c] {
			t.Errorf("duplicate signal constant: 0x%02x", c)
		}
		seen[c] = true
	}

	// Verify specific values
	if SignalTrickleCandidate != 0x0A {
		t.Errorf("SignalTrickleCandidate = 0x%02x, want 0x0A", SignalTrickleCandidate)
	}
	if SignalEndOfCandidates != 0x0B {
		t.Errorf("SignalEndOfCandidates = 0x%02x, want 0x0B", SignalEndOfCandidates)
	}
}

// ==================== WebRTC Tests ====================

func TestWebRTCSDPEncodeDecode(t *testing.T) {
	original := &WebRTCSDPInfo{
		Type: SDPTypeOffer,
		SDP:  "v=0\r\no=- 123456 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n",
	}

	encoded := EncodeWebRTCSDP(original)
	decoded, err := DecodeWebRTCSDP(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.Type != original.Type {
		t.Errorf("Type: got %d, want %d", decoded.Type, original.Type)
	}
	if decoded.SDP != original.SDP {
		t.Errorf("SDP: got %q, want %q", decoded.SDP, original.SDP)
	}
}

func TestWebRTCSDPDecodeTooShort(t *testing.T) {
	_, err := DecodeWebRTCSDP([]byte{0x01, 0x00})
	if err == nil {
		t.Error("expected error for short data")
	}
}

func TestWebRTCSDPDecodeTruncated(t *testing.T) {
	// Type(1) + Len(2) = 3 bytes, but len says 100 bytes
	data := []byte{0x01, 0x00, 0x64} // type=offer, len=100
	_, err := DecodeWebRTCSDP(data)
	if err == nil {
		t.Error("expected error for truncated data")
	}
}

func TestNewWebRTCOfferMessage(t *testing.T) {
	srcVIP := net.IPv4(10, 7, 0, 2)
	dstVIP := net.IPv4(10, 7, 0, 3)
	sdp := "v=0\r\no=- 123456 2 IN IP4 127.0.0.1\r\n"

	msg := NewWebRTCOfferMessage(srcVIP, dstVIP, sdp)

	if msg.Type != SignalWebRTCOffer {
		t.Errorf("expected SignalWebRTCOffer (0x%02x), got 0x%02x", SignalWebRTCOffer, msg.Type)
	}
	if !msg.SrcVIP.Equal(srcVIP) {
		t.Error("SrcVIP mismatch")
	}
	if !msg.DstVIP.Equal(dstVIP) {
		t.Error("DstVIP mismatch")
	}

	// Verify payload can be decoded
	decoded, err := DecodeWebRTCSDP(msg.Payload)
	if err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if decoded.Type != SDPTypeOffer {
		t.Errorf("SDP type: got %d, want %d", decoded.Type, SDPTypeOffer)
	}
	if decoded.SDP != sdp {
		t.Errorf("SDP: got %q, want %q", decoded.SDP, sdp)
	}
}

func TestNewWebRTCAnswerMessage(t *testing.T) {
	srcVIP := net.IPv4(10, 7, 0, 3)
	dstVIP := net.IPv4(10, 7, 0, 2)
	sdp := "v=0\r\no=- 654321 2 IN IP4 127.0.0.1\r\n"

	msg := NewWebRTCAnswerMessage(srcVIP, dstVIP, sdp)

	if msg.Type != SignalWebRTCAnswer {
		t.Errorf("expected SignalWebRTCAnswer (0x%02x), got 0x%02x", SignalWebRTCAnswer, msg.Type)
	}

	decoded, err := DecodeWebRTCSDP(msg.Payload)
	if err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if decoded.Type != SDPTypeAnswer {
		t.Errorf("SDP type: got %d, want %d", decoded.Type, SDPTypeAnswer)
	}
}

func TestWebRTCICECandidateEncodeDecode(t *testing.T) {
	original := &WebRTCICECandidateInfo{
		Candidate:        "candidate:1 1 UDP 2122252543 192.168.1.1 12345 typ host",
		SDPMid:           "0",
		SDPMLineIndex:    0,
		UsernameFragment: "abcd1234",
	}

	encoded := EncodeWebRTCICECandidate(original)
	decoded, err := DecodeWebRTCICECandidate(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.Candidate != original.Candidate {
		t.Errorf("Candidate: got %q, want %q", decoded.Candidate, original.Candidate)
	}
	if decoded.SDPMid != original.SDPMid {
		t.Errorf("SDPMid: got %q, want %q", decoded.SDPMid, original.SDPMid)
	}
	if decoded.SDPMLineIndex != original.SDPMLineIndex {
		t.Errorf("SDPMLineIndex: got %d, want %d", decoded.SDPMLineIndex, original.SDPMLineIndex)
	}
	if decoded.UsernameFragment != original.UsernameFragment {
		t.Errorf("UsernameFragment: got %q, want %q", decoded.UsernameFragment, original.UsernameFragment)
	}
}

func TestWebRTCICECandidateDecodeTooShort(t *testing.T) {
	_, err := DecodeWebRTCICECandidate([]byte{0, 1, 2, 3, 4})
	if err == nil {
		t.Error("expected error for short data")
	}
}

func TestWebRTCICECandidateDecodeTruncated(t *testing.T) {
	// CandLen=100 but only have a few bytes
	data := []byte{0, 100, 'a', 'b', 'c'}
	_, err := DecodeWebRTCICECandidate(data)
	if err == nil {
		t.Error("expected error for truncated candidate")
	}
}

func TestNewWebRTCICECandidateMessage(t *testing.T) {
	srcVIP := net.IPv4(10, 7, 0, 2)
	dstVIP := net.IPv4(10, 7, 0, 3)
	info := &WebRTCICECandidateInfo{
		Candidate:        "candidate:1 1 UDP 2122252543 192.168.1.1 12345 typ host",
		SDPMid:           "data",
		SDPMLineIndex:    0,
		UsernameFragment: "test",
	}

	msg := NewWebRTCICECandidateMessage(srcVIP, dstVIP, info)

	if msg.Type != SignalWebRTCICECandidate {
		t.Errorf("expected SignalWebRTCICECandidate (0x%02x), got 0x%02x", SignalWebRTCICECandidate, msg.Type)
	}

	decoded, err := DecodeWebRTCICECandidate(msg.Payload)
	if err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if decoded.Candidate != info.Candidate {
		t.Errorf("Candidate mismatch")
	}
}

func TestWebRTCSignalConstants(t *testing.T) {
	if SignalWebRTCOffer != 0x10 {
		t.Errorf("SignalWebRTCOffer = 0x%02x, want 0x10", SignalWebRTCOffer)
	}
	if SignalWebRTCAnswer != 0x11 {
		t.Errorf("SignalWebRTCAnswer = 0x%02x, want 0x11", SignalWebRTCAnswer)
	}
	if SignalWebRTCICECandidate != 0x12 {
		t.Errorf("SignalWebRTCICECandidate = 0x%02x, want 0x12", SignalWebRTCICECandidate)
	}
}

func TestSDPTypeConstants(t *testing.T) {
	if SDPTypeOffer != 0x01 {
		t.Errorf("SDPTypeOffer = 0x%02x, want 0x01", SDPTypeOffer)
	}
	if SDPTypeAnswer != 0x02 {
		t.Errorf("SDPTypeAnswer = 0x%02x, want 0x02", SDPTypeAnswer)
	}
}
