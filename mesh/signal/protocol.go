package signal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

// Signal message types
const (
	SignalCandidate        byte = 0x01 // ICE candidate exchange (batch)
	SignalConnect          byte = 0x04 // Connection request
	SignalConnectAck       byte = 0x05 // Connection acknowledgment
	SignalDisconnect       byte = 0x06 // Peer disconnection notification
	SignalPing             byte = 0x07 // Keep-alive ping
	SignalPong             byte = 0x08 // Keep-alive pong
	SignalICERestart       byte = 0x09 // ICE restart request/notification
	SignalTrickleCandidate byte = 0x0A // Trickle ICE: single candidate
	SignalEndOfCandidates  byte = 0x0B // Trickle ICE: end of candidates

	// WebRTC DataChannel signaling
	SignalWebRTCOffer        byte = 0x10 // WebRTC SDP offer
	SignalWebRTCAnswer       byte = 0x11 // WebRTC SDP answer
	SignalWebRTCICECandidate byte = 0x12 // WebRTC ICE candidate

	SignalError byte = 0xFF // Error message
)

// SignalMagic identifies signaling messages within mesh stream.
const SignalMagic = "SIG\n"

// MaxSignalPayload is the maximum payload size for signaling messages.
const MaxSignalPayload = 4096

// Message represents a signaling message.
// Format: [1B Type][4B SrcVIP][4B DstVIP][2B Len][Payload]
type Message struct {
	Type    byte
	SrcVIP  net.IP // Source virtual IP (4 bytes)
	DstVIP  net.IP // Destination virtual IP (4 bytes)
	Payload []byte
}

// HeaderSize is the fixed header size: Type(1) + SrcVIP(4) + DstVIP(4) + Len(2) = 11 bytes
const HeaderSize = 11

// Encode encodes the message into bytes.
func (m *Message) Encode() []byte {
	payloadLen := len(m.Payload)
	buf := make([]byte, HeaderSize+payloadLen)

	buf[0] = m.Type
	copy(buf[1:5], m.SrcVIP.To4())
	copy(buf[5:9], m.DstVIP.To4())
	binary.BigEndian.PutUint16(buf[9:11], uint16(payloadLen))

	if payloadLen > 0 {
		copy(buf[HeaderSize:], m.Payload)
	}

	return buf
}

// Decode decodes bytes into a message.
func Decode(data []byte) (*Message, error) {
	if len(data) < HeaderSize {
		return nil, errors.New("signal: message too short")
	}

	m := &Message{
		Type:   data[0],
		SrcVIP: net.IP(make([]byte, 4)),
		DstVIP: net.IP(make([]byte, 4)),
	}

	copy(m.SrcVIP, data[1:5])
	copy(m.DstVIP, data[5:9])

	payloadLen := binary.BigEndian.Uint16(data[9:11])
	if payloadLen > MaxSignalPayload {
		return nil, fmt.Errorf("signal: payload too large: %d", payloadLen)
	}

	expectedLen := HeaderSize + int(payloadLen)
	if len(data) < expectedLen {
		return nil, fmt.Errorf("signal: message truncated: have %d, want %d", len(data), expectedLen)
	}

	if payloadLen > 0 {
		m.Payload = make([]byte, payloadLen)
		copy(m.Payload, data[HeaderSize:expectedLen])
	}

	return m, nil
}

// CandidateInfo represents ICE candidate information for signaling.
type CandidateInfo struct {
	Type        byte   // Candidate type (host=0, srflx=1, prflx=2, relay=3)
	IP          net.IP // Candidate IP address
	Port        uint16 // Candidate port
	Priority    uint32 // Candidate priority
	Foundation  string // Candidate foundation (optional, for debugging)
	RelatedIP   net.IP // Related address IP (for srflx/prflx)
	RelatedPort uint16 // Related address port
}

// CandidateInfoSize is the minimum size of encoded candidate info.
// Type(1) + IP(4) + Port(2) + Priority(4) = 11 bytes minimum
const CandidateInfoSize = 11

// EncodeCandidate encodes candidate information.
func EncodeCandidate(c *CandidateInfo) []byte {
	// Base size: Type(1) + IP(4) + Port(2) + Priority(4) = 11
	// Optional: RelatedIP(4) + RelatedPort(2) = 6
	buf := make([]byte, 17)

	buf[0] = c.Type
	copy(buf[1:5], c.IP.To4())
	binary.BigEndian.PutUint16(buf[5:7], c.Port)
	binary.BigEndian.PutUint32(buf[7:11], c.Priority)

	// Include related address if present
	if c.RelatedIP != nil {
		copy(buf[11:15], c.RelatedIP.To4())
		binary.BigEndian.PutUint16(buf[15:17], c.RelatedPort)
		return buf
	}

	return buf[:11]
}

// DecodeCandidate decodes candidate information.
func DecodeCandidate(data []byte) (*CandidateInfo, error) {
	if len(data) < CandidateInfoSize {
		return nil, errors.New("signal: candidate too short")
	}

	c := &CandidateInfo{
		Type:     data[0],
		IP:       net.IP(make([]byte, 4)),
		Port:     binary.BigEndian.Uint16(data[5:7]),
		Priority: binary.BigEndian.Uint32(data[7:11]),
	}
	copy(c.IP, data[1:5])

	// Check for related address
	if len(data) >= 17 {
		c.RelatedIP = net.IP(make([]byte, 4))
		copy(c.RelatedIP, data[11:15])
		c.RelatedPort = binary.BigEndian.Uint16(data[15:17])
	}

	return c, nil
}

// EncodeCandidates encodes multiple candidates into a single payload.
func EncodeCandidates(candidates []*CandidateInfo) []byte {
	if len(candidates) == 0 {
		return nil
	}

	// First byte is count
	buf := make([]byte, 1)
	buf[0] = byte(len(candidates))

	for _, c := range candidates {
		encoded := EncodeCandidate(c)
		// Length prefix each candidate
		lenBuf := make([]byte, 1)
		lenBuf[0] = byte(len(encoded))
		buf = append(buf, lenBuf...)
		buf = append(buf, encoded...)
	}

	return buf
}

// DecodeCandidates decodes multiple candidates from a payload.
func DecodeCandidates(data []byte) ([]*CandidateInfo, error) {
	if len(data) < 1 {
		return nil, errors.New("signal: empty candidates")
	}

	count := int(data[0])
	candidates := make([]*CandidateInfo, 0, count)

	offset := 1
	for i := 0; i < count; i++ {
		if offset >= len(data) {
			break
		}

		candidateLen := int(data[offset])
		offset++

		if offset+candidateLen > len(data) {
			break
		}

		c, err := DecodeCandidate(data[offset : offset+candidateLen])
		if err != nil {
			continue
		}
		candidates = append(candidates, c)
		offset += candidateLen
	}

	return candidates, nil
}

// ConnectInfo represents connection request information.
type ConnectInfo struct {
	PublicKey [32]byte // Initiator's public key for Noise IK handshake
}

// EncodeConnectInfo encodes connection info.
func EncodeConnectInfo(c *ConnectInfo) []byte {
	buf := make([]byte, 32)
	copy(buf, c.PublicKey[:])
	return buf
}

// DecodeConnectInfo decodes connection info.
func DecodeConnectInfo(data []byte) (*ConnectInfo, error) {
	if len(data) < 32 {
		return nil, errors.New("signal: connect info too short")
	}
	c := &ConnectInfo{}
	copy(c.PublicKey[:], data[:32])
	return c, nil
}

// ICE Restart reason codes
const (
	ICERestartReasonManual         byte = 0x00 // Explicit restart request
	ICERestartReasonNetworkChange  byte = 0x01 // Network interface changed
	ICERestartReasonQualityDegraded byte = 0x02 // Connection quality dropped
	ICERestartReasonAllPairsFailed byte = 0x03 // All candidate pairs failed
	ICERestartReasonTimeout        byte = 0x04 // Connection timeout
	ICERestartReasonRemoteRequest  byte = 0x05 // Remote peer requested restart
)

// ICERestartInfo represents ICE restart information for signaling.
// This includes new ICE credentials and the reason for restart.
type ICERestartInfo struct {
	Reason           byte   // Why restart was triggered
	Generation       uint16 // Credential generation number
	UsernameFragment string // New ice-ufrag (8+ chars)
	Password         string // New ice-pwd (24+ chars)
}

// ICERestartInfoMinSize is the minimum size of encoded ICE restart info.
// Reason(1) + Generation(2) + UfragLen(1) + PwdLen(1) = 5 bytes minimum
const ICERestartInfoMinSize = 5

// EncodeICERestartInfo encodes ICE restart information.
func EncodeICERestartInfo(info *ICERestartInfo) []byte {
	ufragBytes := []byte(info.UsernameFragment)
	pwdBytes := []byte(info.Password)

	// Reason(1) + Generation(2) + UfragLen(1) + Ufrag + PwdLen(1) + Pwd
	buf := make([]byte, 5+len(ufragBytes)+len(pwdBytes))

	buf[0] = info.Reason
	binary.BigEndian.PutUint16(buf[1:3], info.Generation)
	buf[3] = byte(len(ufragBytes))
	copy(buf[4:4+len(ufragBytes)], ufragBytes)
	buf[4+len(ufragBytes)] = byte(len(pwdBytes))
	copy(buf[5+len(ufragBytes):], pwdBytes)

	return buf
}

// DecodeICERestartInfo decodes ICE restart information.
func DecodeICERestartInfo(data []byte) (*ICERestartInfo, error) {
	if len(data) < ICERestartInfoMinSize {
		return nil, errors.New("signal: ice restart info too short")
	}

	info := &ICERestartInfo{
		Reason:     data[0],
		Generation: binary.BigEndian.Uint16(data[1:3]),
	}

	ufragLen := int(data[3])
	if len(data) < 4+ufragLen+1 {
		return nil, errors.New("signal: ice restart info truncated (ufrag)")
	}
	info.UsernameFragment = string(data[4 : 4+ufragLen])

	pwdLen := int(data[4+ufragLen])
	if len(data) < 5+ufragLen+pwdLen {
		return nil, errors.New("signal: ice restart info truncated (pwd)")
	}
	info.Password = string(data[5+ufragLen : 5+ufragLen+pwdLen])

	return info, nil
}

// NewCandidateMessage creates a candidate exchange message.
func NewCandidateMessage(srcVIP, dstVIP net.IP, candidates []*CandidateInfo) *Message {
	return &Message{
		Type:    SignalCandidate,
		SrcVIP:  srcVIP,
		DstVIP:  dstVIP,
		Payload: EncodeCandidates(candidates),
	}
}

// NewConnectMessage creates a connection request message.
func NewConnectMessage(srcVIP, dstVIP net.IP, publicKey [32]byte) *Message {
	info := &ConnectInfo{PublicKey: publicKey}
	return &Message{
		Type:    SignalConnect,
		SrcVIP:  srcVIP,
		DstVIP:  dstVIP,
		Payload: EncodeConnectInfo(info),
	}
}

// NewConnectAckMessage creates a connection acknowledgment message.
func NewConnectAckMessage(srcVIP, dstVIP net.IP, publicKey [32]byte) *Message {
	info := &ConnectInfo{PublicKey: publicKey}
	return &Message{
		Type:    SignalConnectAck,
		SrcVIP:  srcVIP,
		DstVIP:  dstVIP,
		Payload: EncodeConnectInfo(info),
	}
}

// NewDisconnectMessage creates a disconnect notification.
func NewDisconnectMessage(srcVIP, dstVIP net.IP) *Message {
	return &Message{
		Type:   SignalDisconnect,
		SrcVIP: srcVIP,
		DstVIP: dstVIP,
	}
}

// NewPingMessage creates a ping message.
func NewPingMessage(srcVIP, dstVIP net.IP) *Message {
	return &Message{
		Type:   SignalPing,
		SrcVIP: srcVIP,
		DstVIP: dstVIP,
	}
}

// NewPongMessage creates a pong message.
func NewPongMessage(srcVIP, dstVIP net.IP) *Message {
	return &Message{
		Type:   SignalPong,
		SrcVIP: srcVIP,
		DstVIP: dstVIP,
	}
}

// NewErrorMessage creates an error message.
func NewErrorMessage(srcVIP, dstVIP net.IP, errMsg string) *Message {
	return &Message{
		Type:    SignalError,
		SrcVIP:  srcVIP,
		DstVIP:  dstVIP,
		Payload: []byte(errMsg),
	}
}

// NewICERestartMessage creates an ICE restart message.
// This is sent when a peer needs to restart ICE due to network changes,
// quality degradation, or other reasons per RFC 8445.
func NewICERestartMessage(srcVIP, dstVIP net.IP, info *ICERestartInfo) *Message {
	return &Message{
		Type:    SignalICERestart,
		SrcVIP:  srcVIP,
		DstVIP:  dstVIP,
		Payload: EncodeICERestartInfo(info),
	}
}

// TrickleCandidateInfo represents a single trickled candidate with metadata.
type TrickleCandidateInfo struct {
	Candidate  *CandidateInfo
	Generation uint16 // ICE credential generation this candidate belongs to
	MLineIndex uint8  // Media line index (0 for single stream)
}

// EncodeTrickleCandidate encodes a trickle candidate.
func EncodeTrickleCandidate(info *TrickleCandidateInfo) []byte {
	candidateBytes := EncodeCandidate(info.Candidate)

	// Generation(2) + MLineIndex(1) + CandidateLen(1) + Candidate
	buf := make([]byte, 4+len(candidateBytes))

	binary.BigEndian.PutUint16(buf[0:2], info.Generation)
	buf[2] = info.MLineIndex
	buf[3] = byte(len(candidateBytes))
	copy(buf[4:], candidateBytes)

	return buf
}

// DecodeTrickleCandidate decodes a trickle candidate.
func DecodeTrickleCandidate(data []byte) (*TrickleCandidateInfo, error) {
	if len(data) < 4 {
		return nil, errors.New("signal: trickle candidate too short")
	}

	info := &TrickleCandidateInfo{
		Generation: binary.BigEndian.Uint16(data[0:2]),
		MLineIndex: data[2],
	}

	candidateLen := int(data[3])
	if len(data) < 4+candidateLen {
		return nil, errors.New("signal: trickle candidate truncated")
	}

	candidate, err := DecodeCandidate(data[4 : 4+candidateLen])
	if err != nil {
		return nil, err
	}
	info.Candidate = candidate

	return info, nil
}

// NewTrickleCandidateMessage creates a trickle candidate message.
// This is sent when a new candidate is discovered during gathering.
func NewTrickleCandidateMessage(srcVIP, dstVIP net.IP, info *TrickleCandidateInfo) *Message {
	return &Message{
		Type:    SignalTrickleCandidate,
		SrcVIP:  srcVIP,
		DstVIP:  dstVIP,
		Payload: EncodeTrickleCandidate(info),
	}
}

// EndOfCandidatesInfo contains metadata for end-of-candidates signal.
type EndOfCandidatesInfo struct {
	Generation     uint16 // ICE credential generation
	TotalCandidates uint8  // Total number of candidates gathered
}

// EncodeEndOfCandidates encodes end-of-candidates info.
func EncodeEndOfCandidates(info *EndOfCandidatesInfo) []byte {
	buf := make([]byte, 3)
	binary.BigEndian.PutUint16(buf[0:2], info.Generation)
	buf[2] = info.TotalCandidates
	return buf
}

// DecodeEndOfCandidates decodes end-of-candidates info.
func DecodeEndOfCandidates(data []byte) (*EndOfCandidatesInfo, error) {
	if len(data) < 3 {
		return nil, errors.New("signal: end-of-candidates too short")
	}
	return &EndOfCandidatesInfo{
		Generation:     binary.BigEndian.Uint16(data[0:2]),
		TotalCandidates: data[2],
	}, nil
}

// NewEndOfCandidatesMessage creates an end-of-candidates message.
// This signals that gathering is complete and no more candidates will be sent.
func NewEndOfCandidatesMessage(srcVIP, dstVIP net.IP, info *EndOfCandidatesInfo) *Message {
	return &Message{
		Type:    SignalEndOfCandidates,
		SrcVIP:  srcVIP,
		DstVIP:  dstVIP,
		Payload: EncodeEndOfCandidates(info),
	}
}

// ==================== WebRTC Signaling ====================

// SDPType represents the SDP type for WebRTC offers/answers.
type SDPType byte

const (
	SDPTypeOffer  SDPType = 0x01
	SDPTypeAnswer SDPType = 0x02
)

// WebRTCSDPInfo contains SDP information for WebRTC signaling.
type WebRTCSDPInfo struct {
	Type SDPType // Offer or Answer
	SDP  string  // SDP content
}

// MaxWebRTCSDPSize is the maximum SDP size we support.
// Typical SDP is 1-3KB, we allow up to 8KB for complex scenarios.
const MaxWebRTCSDPSize = 8192

// EncodeWebRTCSDP encodes WebRTC SDP information.
func EncodeWebRTCSDP(info *WebRTCSDPInfo) []byte {
	sdpBytes := []byte(info.SDP)
	if len(sdpBytes) > MaxWebRTCSDPSize {
		sdpBytes = sdpBytes[:MaxWebRTCSDPSize]
	}

	// Type(1) + SDPLen(2) + SDP
	buf := make([]byte, 3+len(sdpBytes))
	buf[0] = byte(info.Type)
	binary.BigEndian.PutUint16(buf[1:3], uint16(len(sdpBytes)))
	copy(buf[3:], sdpBytes)

	return buf
}

// DecodeWebRTCSDP decodes WebRTC SDP information.
func DecodeWebRTCSDP(data []byte) (*WebRTCSDPInfo, error) {
	if len(data) < 3 {
		return nil, errors.New("signal: webrtc sdp too short")
	}

	info := &WebRTCSDPInfo{
		Type: SDPType(data[0]),
	}

	sdpLen := binary.BigEndian.Uint16(data[1:3])
	if int(sdpLen) > len(data)-3 {
		return nil, errors.New("signal: webrtc sdp truncated")
	}

	info.SDP = string(data[3 : 3+sdpLen])
	return info, nil
}

// NewWebRTCOfferMessage creates a WebRTC SDP offer message.
func NewWebRTCOfferMessage(srcVIP, dstVIP net.IP, sdp string) *Message {
	info := &WebRTCSDPInfo{
		Type: SDPTypeOffer,
		SDP:  sdp,
	}
	return &Message{
		Type:    SignalWebRTCOffer,
		SrcVIP:  srcVIP,
		DstVIP:  dstVIP,
		Payload: EncodeWebRTCSDP(info),
	}
}

// NewWebRTCAnswerMessage creates a WebRTC SDP answer message.
func NewWebRTCAnswerMessage(srcVIP, dstVIP net.IP, sdp string) *Message {
	info := &WebRTCSDPInfo{
		Type: SDPTypeAnswer,
		SDP:  sdp,
	}
	return &Message{
		Type:    SignalWebRTCAnswer,
		SrcVIP:  srcVIP,
		DstVIP:  dstVIP,
		Payload: EncodeWebRTCSDP(info),
	}
}

// WebRTCICECandidateInfo contains a WebRTC ICE candidate for signaling.
// This uses the standard ICE candidate string format.
type WebRTCICECandidateInfo struct {
	Candidate        string // ICE candidate string (e.g., "candidate:...")
	SDPMid           string // Media stream identification (usually "0" or "data")
	SDPMLineIndex    uint16 // Index of the media line
	UsernameFragment string // Ice-ufrag for this candidate
}

// EncodeWebRTCICECandidate encodes a WebRTC ICE candidate.
func EncodeWebRTCICECandidate(info *WebRTCICECandidateInfo) []byte {
	candBytes := []byte(info.Candidate)
	midBytes := []byte(info.SDPMid)
	ufragBytes := []byte(info.UsernameFragment)

	// CandLen(2) + Cand + MidLen(1) + Mid + MLineIndex(2) + UfragLen(1) + Ufrag
	buf := make([]byte, 6+len(candBytes)+len(midBytes)+len(ufragBytes))

	offset := 0
	binary.BigEndian.PutUint16(buf[offset:offset+2], uint16(len(candBytes)))
	offset += 2
	copy(buf[offset:], candBytes)
	offset += len(candBytes)

	buf[offset] = byte(len(midBytes))
	offset++
	copy(buf[offset:], midBytes)
	offset += len(midBytes)

	binary.BigEndian.PutUint16(buf[offset:offset+2], info.SDPMLineIndex)
	offset += 2

	buf[offset] = byte(len(ufragBytes))
	offset++
	copy(buf[offset:], ufragBytes)

	return buf
}

// DecodeWebRTCICECandidate decodes a WebRTC ICE candidate.
func DecodeWebRTCICECandidate(data []byte) (*WebRTCICECandidateInfo, error) {
	if len(data) < 6 {
		return nil, errors.New("signal: webrtc ice candidate too short")
	}

	info := &WebRTCICECandidateInfo{}
	offset := 0

	candLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if offset+candLen > len(data) {
		return nil, errors.New("signal: webrtc ice candidate truncated")
	}
	info.Candidate = string(data[offset : offset+candLen])
	offset += candLen

	if offset >= len(data) {
		return nil, errors.New("signal: webrtc ice candidate missing mid")
	}
	midLen := int(data[offset])
	offset++
	if offset+midLen > len(data) {
		return nil, errors.New("signal: webrtc ice candidate mid truncated")
	}
	info.SDPMid = string(data[offset : offset+midLen])
	offset += midLen

	if offset+2 > len(data) {
		return nil, errors.New("signal: webrtc ice candidate missing mline index")
	}
	info.SDPMLineIndex = binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	if offset >= len(data) {
		return nil, errors.New("signal: webrtc ice candidate missing ufrag")
	}
	ufragLen := int(data[offset])
	offset++
	if offset+ufragLen > len(data) {
		return nil, errors.New("signal: webrtc ice candidate ufrag truncated")
	}
	info.UsernameFragment = string(data[offset : offset+ufragLen])

	return info, nil
}

// NewWebRTCICECandidateMessage creates a WebRTC ICE candidate message.
func NewWebRTCICECandidateMessage(srcVIP, dstVIP net.IP, info *WebRTCICECandidateInfo) *Message {
	return &Message{
		Type:    SignalWebRTCICECandidate,
		SrcVIP:  srcVIP,
		DstVIP:  dstVIP,
		Payload: EncodeWebRTCICECandidate(info),
	}
}
