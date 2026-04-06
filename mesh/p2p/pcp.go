// Package p2p implements PCP (Port Control Protocol) client.
// PCP (RFC 6887) is the successor to NAT-PMP with IPv6 support and more features.
package p2p

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

const (
	pcpPort       = 5351
	pcpVersion    = 2
	pcpHeaderSize = 24
	pcpMapSize    = 36
)

// PCP opcodes
const (
	pcpOpcodeAnnounce = 0
	pcpOpcodeMap      = 1
	pcpOpcodePeer     = 2
)

// PCP result codes
const (
	pcpResultSuccess              = 0
	pcpResultUnsupportedVersion   = 1
	pcpResultNotAuthorized        = 2
	pcpResultMalformedRequest     = 3
	pcpResultUnsupportedOpcode    = 4
	pcpResultUnsupportedOption    = 5
	pcpResultMalformedOption      = 6
	pcpResultNetworkFailure       = 7
	pcpResultNoResources          = 8
	pcpResultUnsupportedProtocol  = 9
	pcpResultUserExceedsQuota     = 10
	pcpResultCannotProvideExt     = 11
	pcpResultAddressMismatch      = 12
	pcpResultExcessiveRemotePeers = 13
)

// Protocol numbers
const (
	protocolTCP = 6
	protocolUDP = 17
)

// PCPClient implements the PCP protocol for NAT traversal.
type PCPClient struct {
	mu         sync.Mutex
	gateway    net.IP
	serverAddr *net.UDPAddr
	externalIP net.IP
	epoch      uint32
	logger     *slog.Logger
	discovered bool
	mappings   map[int]*PCPMapping
}

// PCPMapping represents an active PCP port mapping.
type PCPMapping struct {
	Protocol     int           // TCP (6) or UDP (17)
	InternalPort int           // Internal port
	ExternalPort int           // External port
	ExternalIP   net.IP        // External IP address
	Lifetime     time.Duration // Mapping lifetime
	Created      time.Time     // When mapping was created
	Nonce        [12]byte      // Mapping nonce for refresh/delete
}

// NewPCPClient creates a new PCP client.
func NewPCPClient(gateway net.IP, logger *slog.Logger) *PCPClient {
	if logger == nil {
		logger = slog.Default()
	}

	return &PCPClient{
		gateway:  gateway,
		logger:   logger,
		mappings: make(map[int]*PCPMapping),
	}
}

// Discover attempts to discover a PCP server on the gateway.
func (c *PCPClient) Discover() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.discovered {
		return nil
	}

	gateway := c.gateway
	if gateway == nil {
		var err error
		gateway, err = getDefaultGateway()
		if err != nil {
			return fmt.Errorf("pcp: cannot determine gateway: %w", err)
		}
		c.gateway = gateway
	}

	c.serverAddr = &net.UDPAddr{
		IP:   gateway,
		Port: pcpPort,
	}

	// Send ANNOUNCE to discover PCP server
	conn, err := net.DialUDP("udp4", nil, c.serverAddr)
	if err != nil {
		return fmt.Errorf("pcp: dial failed: %w", err)
	}
	defer conn.Close()

	// Get local address for the request
	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// Build ANNOUNCE request
	req := c.buildAnnounceRequest(localAddr.IP)

	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("pcp: send failed: %w", err)
	}

	// Read response
	resp := make([]byte, 60)
	n, err := conn.Read(resp)
	if err != nil {
		return fmt.Errorf("pcp: no response (gateway may not support PCP): %w", err)
	}

	if err := c.parseAnnounceResponse(resp[:n]); err != nil {
		return fmt.Errorf("pcp: invalid response: %w", err)
	}

	c.discovered = true
	c.logger.Info("PCP server discovered", "gateway", gateway, "external_ip", c.externalIP)

	return nil
}

// buildAnnounceRequest builds a PCP ANNOUNCE request.
func (c *PCPClient) buildAnnounceRequest(clientIP net.IP) []byte {
	buf := make([]byte, pcpHeaderSize)

	// Version (1 byte)
	buf[0] = pcpVersion

	// Opcode (1 byte) - Request bit (7) = 0, Opcode = ANNOUNCE (0)
	buf[1] = pcpOpcodeAnnounce

	// Reserved (2 bytes) = 0
	// buf[2:4] = 0

	// Requested lifetime (4 bytes) = 0 for ANNOUNCE
	// buf[4:8] = 0

	// Client IP address (16 bytes, IPv4-mapped IPv6)
	copy(buf[8:24], ipToIPv6(clientIP))

	return buf
}

// parseAnnounceResponse parses a PCP ANNOUNCE response.
func (c *PCPClient) parseAnnounceResponse(data []byte) error {
	if len(data) < pcpHeaderSize {
		return errors.New("response too short")
	}

	// Version
	if data[0] != pcpVersion {
		return fmt.Errorf("unsupported version: %d", data[0])
	}

	// Check response bit (bit 7 of opcode byte)
	if data[1]&0x80 == 0 {
		return errors.New("not a response")
	}

	// Result code
	resultCode := data[3]
	if resultCode != pcpResultSuccess {
		return fmt.Errorf("result code: %d", resultCode)
	}

	// Lifetime (4 bytes at offset 4)
	// For ANNOUNCE, this is 0

	// Epoch time (4 bytes at offset 8)
	c.epoch = binary.BigEndian.Uint32(data[8:12])

	return nil
}

// AddPortMapping creates a port mapping using PCP MAP opcode.
func (c *PCPClient) AddPortMapping(protocol, internalPort, externalPort int, lifetime time.Duration) (*PCPMapping, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.discovered {
		return nil, errors.New("pcp: not discovered")
	}

	conn, err := net.DialUDP("udp4", nil, c.serverAddr)
	if err != nil {
		return nil, fmt.Errorf("pcp: dial failed: %w", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// Generate nonce for this mapping
	var nonce [12]byte
	for i := range nonce {
		nonce[i] = byte(time.Now().UnixNano() >> (i * 8))
	}

	// Build MAP request
	req := c.buildMapRequest(localAddr.IP, protocol, internalPort, externalPort, int(lifetime.Seconds()), nonce)

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := conn.Write(req); err != nil {
		return nil, fmt.Errorf("pcp: send failed: %w", err)
	}

	// Read response
	resp := make([]byte, pcpHeaderSize+pcpMapSize)
	n, err := conn.Read(resp)
	if err != nil {
		return nil, fmt.Errorf("pcp: no response: %w", err)
	}

	mapping, err := c.parseMapResponse(resp[:n], nonce)
	if err != nil {
		return nil, fmt.Errorf("pcp: invalid response: %w", err)
	}

	mapping.InternalPort = internalPort
	mapping.Protocol = protocol
	mapping.Nonce = nonce
	mapping.Created = time.Now()

	c.mappings[internalPort] = mapping
	c.externalIP = mapping.ExternalIP

	c.logger.Info("PCP mapping created",
		"protocol", protocolName(protocol),
		"internal_port", internalPort,
		"external_port", mapping.ExternalPort,
		"external_ip", mapping.ExternalIP,
		"lifetime", mapping.Lifetime)

	return mapping, nil
}

// buildMapRequest builds a PCP MAP request.
func (c *PCPClient) buildMapRequest(clientIP net.IP, protocol, internalPort, externalPort, lifetime int, nonce [12]byte) []byte {
	buf := make([]byte, pcpHeaderSize+pcpMapSize)

	// Header
	buf[0] = pcpVersion
	buf[1] = pcpOpcodeMap // Request bit = 0, Opcode = MAP

	// Requested lifetime (4 bytes)
	binary.BigEndian.PutUint32(buf[4:8], uint32(lifetime))

	// Client IP address (16 bytes, IPv4-mapped IPv6)
	copy(buf[8:24], ipToIPv6(clientIP))

	// MAP opcode-specific data (36 bytes starting at offset 24)
	offset := pcpHeaderSize

	// Mapping nonce (12 bytes)
	copy(buf[offset:offset+12], nonce[:])
	offset += 12

	// Protocol (1 byte)
	buf[offset] = byte(protocol)
	offset++

	// Reserved (3 bytes)
	offset += 3

	// Internal port (2 bytes)
	binary.BigEndian.PutUint16(buf[offset:offset+2], uint16(internalPort))
	offset += 2

	// Suggested external port (2 bytes)
	binary.BigEndian.PutUint16(buf[offset:offset+2], uint16(externalPort))

	return buf
}

// parseMapResponse parses a PCP MAP response.
func (c *PCPClient) parseMapResponse(data []byte, expectedNonce [12]byte) (*PCPMapping, error) {
	if len(data) < pcpHeaderSize+pcpMapSize {
		return nil, errors.New("response too short")
	}

	// Version
	if data[0] != pcpVersion {
		return nil, fmt.Errorf("unsupported version: %d", data[0])
	}

	// Check response bit
	if data[1]&0x80 == 0 {
		return nil, errors.New("not a response")
	}

	// Check opcode
	if data[1]&0x7F != pcpOpcodeMap {
		return nil, fmt.Errorf("unexpected opcode: %d", data[1]&0x7F)
	}

	// Result code
	resultCode := data[3]
	if resultCode != pcpResultSuccess {
		return nil, fmt.Errorf("error: %s", pcpResultString(resultCode))
	}

	// Lifetime (4 bytes)
	lifetime := binary.BigEndian.Uint32(data[4:8])

	// Epoch time (4 bytes)
	c.epoch = binary.BigEndian.Uint32(data[8:12])

	// Parse MAP response data
	offset := pcpHeaderSize

	// Verify nonce
	var nonce [12]byte
	copy(nonce[:], data[offset:offset+12])
	if nonce != expectedNonce {
		return nil, errors.New("nonce mismatch")
	}
	offset += 12

	// Protocol (1 byte, skip).
	offset += 1

	// Reserved
	offset += 3

	// Internal port (2 bytes, skip).
	offset += 2

	// Assigned external port
	externalPort := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	// Assigned external IP (16 bytes, IPv4-mapped IPv6)
	externalIP := ipv6ToIP(data[offset : offset+16])

	return &PCPMapping{
		ExternalPort: int(externalPort),
		ExternalIP:   externalIP,
		Lifetime:     time.Duration(lifetime) * time.Second,
	}, nil
}

// DeletePortMapping removes a port mapping.
func (c *PCPClient) DeletePortMapping(internalPort int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	mapping, ok := c.mappings[internalPort]
	if !ok {
		return nil // Already deleted
	}

	conn, err := net.DialUDP("udp4", nil, c.serverAddr)
	if err != nil {
		return fmt.Errorf("pcp: dial failed: %w", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// Build MAP request with lifetime=0 to delete
	req := c.buildMapRequest(localAddr.IP, mapping.Protocol, internalPort, 0, 0, mapping.Nonce)

	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("pcp: send failed: %w", err)
	}

	// Read response (optional, deletion is best-effort)
	resp := make([]byte, pcpHeaderSize+pcpMapSize)
	_, _ = conn.Read(resp)

	delete(c.mappings, internalPort)

	c.logger.Info("PCP mapping deleted", "internal_port", internalPort)

	return nil
}

// RefreshMapping refreshes an existing mapping.
func (c *PCPClient) RefreshMapping(internalPort int, lifetime time.Duration) error {
	c.mu.Lock()
	mapping, ok := c.mappings[internalPort]
	c.mu.Unlock()

	if !ok {
		return errors.New("pcp: mapping not found")
	}

	// Re-create the mapping with same parameters
	_, err := c.AddPortMapping(mapping.Protocol, internalPort, mapping.ExternalPort, lifetime)
	return err
}

// ExternalIP returns the external IP address.
func (c *PCPClient) ExternalIP() net.IP {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.externalIP
}

// Gateway returns the gateway IP address.
func (c *PCPClient) Gateway() net.IP {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gateway
}

// GetMappings returns all active mappings.
func (c *PCPClient) GetMappings() []*PCPMapping {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]*PCPMapping, 0, len(c.mappings))
	for _, m := range c.mappings {
		result = append(result, m)
	}
	return result
}

// IsAvailable returns whether PCP is available.
func (c *PCPClient) IsAvailable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.discovered
}

// Close releases all resources and deletes mappings.
func (c *PCPClient) Close() error {
	c.mu.Lock()
	ports := make([]int, 0, len(c.mappings))
	for port := range c.mappings {
		ports = append(ports, port)
	}
	c.mu.Unlock()

	for _, port := range ports {
		_ = c.DeletePortMapping(port)
	}

	return nil
}

// ipToIPv6 converts an IP address to IPv4-mapped IPv6 format.
func ipToIPv6(ip net.IP) []byte {
	result := make([]byte, 16)

	ip4 := ip.To4()
	if ip4 != nil {
		// IPv4-mapped IPv6: ::ffff:a.b.c.d
		result[10] = 0xff
		result[11] = 0xff
		copy(result[12:16], ip4)
	} else if ip6 := ip.To16(); ip6 != nil {
		copy(result, ip6)
	}

	return result
}

// ipv6ToIP converts IPv4-mapped IPv6 back to IPv4 if applicable.
func ipv6ToIP(data []byte) net.IP {
	if len(data) != 16 {
		return nil
	}

	// Check for IPv4-mapped IPv6 (::ffff:a.b.c.d)
	isIPv4Mapped := true
	for i := 0; i < 10; i++ {
		if data[i] != 0 {
			isIPv4Mapped = false
			break
		}
	}
	if isIPv4Mapped && data[10] == 0xff && data[11] == 0xff {
		return net.IPv4(data[12], data[13], data[14], data[15])
	}

	// Return as IPv6
	ip := make(net.IP, 16)
	copy(ip, data)
	return ip
}

// protocolName returns the protocol name.
func protocolName(protocol int) string {
	switch protocol {
	case protocolTCP:
		return "TCP"
	case protocolUDP:
		return "UDP"
	default:
		return fmt.Sprintf("proto-%d", protocol)
	}
}

// pcpResultString returns a human-readable result code string.
func pcpResultString(code byte) string {
	switch code {
	case pcpResultSuccess:
		return "success"
	case pcpResultUnsupportedVersion:
		return "unsupported version"
	case pcpResultNotAuthorized:
		return "not authorized"
	case pcpResultMalformedRequest:
		return "malformed request"
	case pcpResultUnsupportedOpcode:
		return "unsupported opcode"
	case pcpResultUnsupportedOption:
		return "unsupported option"
	case pcpResultMalformedOption:
		return "malformed option"
	case pcpResultNetworkFailure:
		return "network failure"
	case pcpResultNoResources:
		return "no resources"
	case pcpResultUnsupportedProtocol:
		return "unsupported protocol"
	case pcpResultUserExceedsQuota:
		return "user exceeds quota"
	case pcpResultCannotProvideExt:
		return "cannot provide external address"
	case pcpResultAddressMismatch:
		return "address mismatch"
	case pcpResultExcessiveRemotePeers:
		return "excessive remote peers"
	default:
		return fmt.Sprintf("unknown error %d", code)
	}
}
