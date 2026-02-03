package p2p

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"
)

// STUN message types
const (
	stunBindingRequest  = 0x0001
	stunBindingResponse = 0x0101
	stunBindingError    = 0x0111

	// STUN attributes
	stunAttrMappedAddress    = 0x0001
	stunAttrXorMappedAddress = 0x0020
	stunAttrErrorCode        = 0x0009
	stunAttrSoftware         = 0x8022

	// Magic cookie (RFC 5389)
	stunMagicCookie = 0x2112A442

	// Header size
	stunHeaderSize = 20
)

var (
	ErrSTUNTimeout       = errors.New("stun: request timeout")
	ErrSTUNNoResponse    = errors.New("stun: no valid response from any server")
	ErrSTUNInvalidPacket = errors.New("stun: invalid packet")
)

// STUNClient performs STUN queries to discover public endpoints.
type STUNClient struct {
	servers []string
	timeout time.Duration
}

// NewSTUNClient creates a STUN client with the given servers.
func NewSTUNClient(servers []string, timeout time.Duration) *STUNClient {
	if timeout == 0 {
		timeout = 3 * time.Second
	}
	return &STUNClient{
		servers: servers,
		timeout: timeout,
	}
}

// STUNResult contains the result of a STUN query.
type STUNResult struct {
	PublicAddr *net.UDPAddr // XOR-MAPPED-ADDRESS (or MAPPED-ADDRESS fallback)
	LocalAddr  *net.UDPAddr // Local address used
	Server     string       // Server that responded
}

// Query sends STUN Binding Request to discover public endpoint.
// Uses the provided UDP connection or creates a new one.
func (c *STUNClient) Query(conn *net.UDPConn) (*STUNResult, error) {
	if conn == nil {
		var err error
		conn, err = net.ListenUDP("udp4", nil)
		if err != nil {
			return nil, fmt.Errorf("stun: listen: %w", err)
		}
		defer conn.Close()
	}

	// Try each server until one responds
	for _, server := range c.servers {
		result, err := c.queryServer(conn, server)
		if err == nil {
			return result, nil
		}
	}

	return nil, ErrSTUNNoResponse
}

// queryServer sends a STUN request to a single server.
func (c *STUNClient) queryServer(conn *net.UDPConn, server string) (*STUNResult, error) {
	addr, err := net.ResolveUDPAddr("udp4", server)
	if err != nil {
		return nil, fmt.Errorf("stun: resolve %s: %w", server, err)
	}

	// Generate transaction ID (12 bytes)
	txID := make([]byte, 12)
	if _, err := rand.Read(txID); err != nil {
		return nil, fmt.Errorf("stun: generate txid: %w", err)
	}

	// Build Binding Request
	req := buildBindingRequest(txID)

	// Set deadline
	conn.SetDeadline(time.Now().Add(c.timeout))
	defer conn.SetDeadline(time.Time{})

	// Send request
	if _, err := conn.WriteToUDP(req, addr); err != nil {
		return nil, fmt.Errorf("stun: send: %w", err)
	}

	// Read response
	buf := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, fmt.Errorf("stun: recv: %w", err)
	}

	// Parse response
	publicAddr, err := parseBindingResponse(buf[:n], txID)
	if err != nil {
		return nil, err
	}

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return &STUNResult{
		PublicAddr: publicAddr,
		LocalAddr:  localAddr,
		Server:     server,
	}, nil
}

// buildBindingRequest creates a STUN Binding Request message.
func buildBindingRequest(txID []byte) []byte {
	msg := make([]byte, stunHeaderSize)

	// Message Type: Binding Request (0x0001)
	binary.BigEndian.PutUint16(msg[0:2], stunBindingRequest)
	// Message Length: 0 (no attributes)
	binary.BigEndian.PutUint16(msg[2:4], 0)
	// Magic Cookie
	binary.BigEndian.PutUint32(msg[4:8], stunMagicCookie)
	// Transaction ID (12 bytes)
	copy(msg[8:20], txID)

	return msg
}

// parseBindingResponse parses a STUN Binding Response message.
func parseBindingResponse(data []byte, expectedTxID []byte) (*net.UDPAddr, error) {
	if len(data) < stunHeaderSize {
		return nil, ErrSTUNInvalidPacket
	}

	// Check message type
	msgType := binary.BigEndian.Uint16(data[0:2])
	if msgType != stunBindingResponse {
		if msgType == stunBindingError {
			return nil, errors.New("stun: server returned error")
		}
		return nil, fmt.Errorf("stun: unexpected message type: 0x%04x", msgType)
	}

	// Check magic cookie
	magic := binary.BigEndian.Uint32(data[4:8])
	if magic != stunMagicCookie {
		return nil, errors.New("stun: invalid magic cookie")
	}

	// Verify transaction ID
	if len(expectedTxID) == 12 {
		for i := 0; i < 12; i++ {
			if data[8+i] != expectedTxID[i] {
				return nil, errors.New("stun: transaction ID mismatch")
			}
		}
	}

	// Parse attributes
	msgLen := binary.BigEndian.Uint16(data[2:4])
	if int(msgLen)+stunHeaderSize > len(data) {
		return nil, ErrSTUNInvalidPacket
	}

	var mappedAddr *net.UDPAddr
	var xorMappedAddr *net.UDPAddr

	offset := stunHeaderSize
	end := stunHeaderSize + int(msgLen)

	for offset+4 <= end {
		attrType := binary.BigEndian.Uint16(data[offset : offset+2])
		attrLen := binary.BigEndian.Uint16(data[offset+2 : offset+4])
		offset += 4

		if offset+int(attrLen) > end {
			break
		}

		attrData := data[offset : offset+int(attrLen)]

		switch attrType {
		case stunAttrMappedAddress:
			if addr := parseMappedAddress(attrData, nil); addr != nil {
				mappedAddr = addr
			}
		case stunAttrXorMappedAddress:
			// XOR key: magic cookie + transaction ID
			xorKey := make([]byte, 16)
			binary.BigEndian.PutUint32(xorKey[0:4], stunMagicCookie)
			copy(xorKey[4:16], data[8:20])
			if addr := parseMappedAddress(attrData, xorKey); addr != nil {
				xorMappedAddr = addr
			}
		}

		// Pad to 4-byte boundary
		offset += int(attrLen)
		if attrLen%4 != 0 {
			offset += 4 - int(attrLen%4)
		}
	}

	// Prefer XOR-MAPPED-ADDRESS over MAPPED-ADDRESS
	if xorMappedAddr != nil {
		return xorMappedAddr, nil
	}
	if mappedAddr != nil {
		return mappedAddr, nil
	}

	return nil, errors.New("stun: no mapped address in response")
}

// parseMappedAddress parses a MAPPED-ADDRESS or XOR-MAPPED-ADDRESS attribute.
func parseMappedAddress(data []byte, xorKey []byte) *net.UDPAddr {
	if len(data) < 4 {
		return nil
	}

	// First byte is reserved/padding
	family := data[1]
	port := binary.BigEndian.Uint16(data[2:4])

	if xorKey != nil {
		// XOR port with first 2 bytes of magic cookie
		port ^= uint16(xorKey[0])<<8 | uint16(xorKey[1])
	}

	var ip net.IP

	switch family {
	case 0x01: // IPv4
		if len(data) < 8 {
			return nil
		}
		ip = make(net.IP, 4)
		copy(ip, data[4:8])
		if xorKey != nil {
			for i := 0; i < 4; i++ {
				ip[i] ^= xorKey[i]
			}
		}
	case 0x02: // IPv6
		if len(data) < 20 {
			return nil
		}
		ip = make(net.IP, 16)
		copy(ip, data[4:20])
		if xorKey != nil {
			for i := 0; i < 16; i++ {
				ip[i] ^= xorKey[i]
			}
		}
	default:
		return nil
	}

	return &net.UDPAddr{
		IP:   ip,
		Port: int(port),
	}
}

// DefaultSTUNServers returns a list of public STUN servers.
func DefaultSTUNServers() []string {
	return []string{
		"stun.l.google.com:19302",
		"stun1.l.google.com:19302",
		"stun.cloudflare.com:3478",
		"stun.stunprotocol.org:3478",
	}
}
