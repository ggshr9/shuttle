package signal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

// Signal message types
const (
	SignalCandidate   byte = 0x01 // ICE candidate exchange
	SignalConnect     byte = 0x04 // Connection request
	SignalConnectAck  byte = 0x05 // Connection acknowledgment
	SignalDisconnect  byte = 0x06 // Peer disconnection notification
	SignalPing        byte = 0x07 // Keep-alive ping
	SignalPong        byte = 0x08 // Keep-alive pong
	SignalError       byte = 0xFF // Error message
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
