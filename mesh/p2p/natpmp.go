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

// NAT-PMP protocol constants (RFC 6886)
const (
	natpmpPort    = 5351
	natpmpVersion = 0

	// Opcodes
	opExternalAddr = 0
	opMapUDP       = 1
	opMapTCP       = 2

	// Response codes
	natpmpSuccess              = 0
	natpmpUnsupportedVersion   = 1
	natpmpNotAuthorized        = 2
	natpmpNetworkFailure       = 3
	natpmpOutOfResources       = 4
	natpmpUnsupportedOpcode    = 5
)

// NAT-PMP error codes
var (
	ErrNATPMPNotFound       = errors.New("natpmp: gateway not found")
	ErrNATPMPTimeout        = errors.New("natpmp: request timeout")
	ErrNATPMPNotSupported   = errors.New("natpmp: not supported by gateway")
	ErrNATPMPMappingFailed  = errors.New("natpmp: port mapping failed")
)

// NATPMPClient handles NAT-PMP protocol operations
type NATPMPClient struct {
	mu sync.Mutex

	gateway    net.IP
	externalIP net.IP
	epoch      uint32
	logger     *slog.Logger
	discovered bool
	mappings   map[int]*NATPMPMapping
}

// NATPMPMapping represents an active NAT-PMP port mapping
type NATPMPMapping struct {
	InternalPort  int
	ExternalPort  int
	Protocol      string
	Lifetime      uint32
	CreatedAt     time.Time
}

// NewNATPMPClient creates a new NAT-PMP client
func NewNATPMPClient(logger *slog.Logger) *NATPMPClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &NATPMPClient{
		logger:   logger,
		mappings: make(map[int]*NATPMPMapping),
	}
}

// Discover finds the NAT-PMP gateway (default gateway)
func (c *NATPMPClient) Discover() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.discovered && c.gateway != nil {
		return nil
	}

	// Find default gateway
	gateway, err := getDefaultGateway()
	if err != nil {
		return fmt.Errorf("natpmp: find gateway: %w", err)
	}

	c.gateway = gateway

	// Try to get external IP to verify NAT-PMP support
	extIP, err := c.getExternalIPLocked()
	if err != nil {
		return fmt.Errorf("natpmp: %w", err)
	}

	c.externalIP = extIP
	c.discovered = true

	c.logger.Info("natpmp: discovered gateway",
		"gateway", gateway,
		"external_ip", extIP)

	return nil
}

// getExternalIPLocked retrieves external IP (must hold lock)
func (c *NATPMPClient) getExternalIPLocked() (net.IP, error) {
	if c.gateway == nil {
		return nil, ErrNATPMPNotFound
	}

	// Build request: version (1 byte) + opcode (1 byte)
	req := []byte{natpmpVersion, opExternalAddr}

	resp, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	if len(resp) < 12 {
		return nil, errors.New("response too short")
	}

	// Parse response:
	// - version (1 byte)
	// - opcode (1 byte) = 128 + request opcode
	// - result code (2 bytes)
	// - epoch (4 bytes)
	// - external IP (4 bytes)

	if resp[0] != natpmpVersion {
		return nil, fmt.Errorf("unexpected version: %d", resp[0])
	}

	if resp[1] != 128+opExternalAddr {
		return nil, fmt.Errorf("unexpected opcode: %d", resp[1])
	}

	resultCode := binary.BigEndian.Uint16(resp[2:4])
	if resultCode != natpmpSuccess {
		return nil, c.resultCodeError(resultCode)
	}

	c.epoch = binary.BigEndian.Uint32(resp[4:8])

	ip := net.IP(resp[8:12])
	return ip, nil
}

// GetExternalIP retrieves the external IP address
func (c *NATPMPClient) GetExternalIP() (net.IP, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getExternalIPLocked()
}

// AddPortMapping creates a port mapping
func (c *NATPMPClient) AddPortMapping(internalPort, externalPort int, protocol string, lifetime uint32) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.gateway == nil {
		return 0, ErrNATPMPNotFound
	}

	if lifetime == 0 {
		lifetime = 3600 // 1 hour default
	}

	var opcode byte
	switch protocol {
	case "UDP", "udp":
		opcode = opMapUDP
	case "TCP", "tcp":
		opcode = opMapTCP
	default:
		opcode = opMapUDP
	}

	// Build request:
	// - version (1 byte)
	// - opcode (1 byte)
	// - reserved (2 bytes)
	// - internal port (2 bytes)
	// - suggested external port (2 bytes)
	// - requested lifetime (4 bytes)
	req := make([]byte, 12)
	req[0] = natpmpVersion
	req[1] = opcode
	// req[2:4] reserved = 0
	binary.BigEndian.PutUint16(req[4:6], uint16(internalPort))
	binary.BigEndian.PutUint16(req[6:8], uint16(externalPort))
	binary.BigEndian.PutUint32(req[8:12], lifetime)

	resp, err := c.sendRequest(req)
	if err != nil {
		return 0, err
	}

	if len(resp) < 16 {
		return 0, errors.New("response too short")
	}

	// Parse response:
	// - version (1 byte)
	// - opcode (1 byte) = 128 + request opcode
	// - result code (2 bytes)
	// - epoch (4 bytes)
	// - internal port (2 bytes)
	// - mapped external port (2 bytes)
	// - lifetime (4 bytes)

	if resp[0] != natpmpVersion {
		return 0, fmt.Errorf("unexpected version: %d", resp[0])
	}

	resultCode := binary.BigEndian.Uint16(resp[2:4])
	if resultCode != natpmpSuccess {
		return 0, c.resultCodeError(resultCode)
	}

	c.epoch = binary.BigEndian.Uint32(resp[4:8])
	mappedInternal := binary.BigEndian.Uint16(resp[8:10])
	mappedExternal := binary.BigEndian.Uint16(resp[10:12])
	mappedLifetime := binary.BigEndian.Uint32(resp[12:16])

	if int(mappedInternal) != internalPort {
		c.logger.Warn("natpmp: internal port mismatch",
			"requested", internalPort,
			"mapped", mappedInternal)
	}

	c.mappings[int(mappedExternal)] = &NATPMPMapping{
		InternalPort:  int(mappedInternal),
		ExternalPort:  int(mappedExternal),
		Protocol:      protocol,
		Lifetime:      mappedLifetime,
		CreatedAt:     time.Now(),
	}

	c.logger.Info("natpmp: port mapping added",
		"internal", mappedInternal,
		"external", mappedExternal,
		"lifetime", mappedLifetime)

	return int(mappedExternal), nil
}

// DeletePortMapping removes a port mapping (set lifetime to 0)
func (c *NATPMPClient) DeletePortMapping(internalPort int, protocol string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.gateway == nil {
		return ErrNATPMPNotFound
	}

	var opcode byte
	switch protocol {
	case "UDP", "udp":
		opcode = opMapUDP
	case "TCP", "tcp":
		opcode = opMapTCP
	default:
		opcode = opMapUDP
	}

	// Request with lifetime=0 deletes the mapping
	req := make([]byte, 12)
	req[0] = natpmpVersion
	req[1] = opcode
	binary.BigEndian.PutUint16(req[4:6], uint16(internalPort))
	// external port = 0, lifetime = 0

	resp, err := c.sendRequest(req)
	if err != nil {
		return err
	}

	if len(resp) < 16 {
		return errors.New("response too short")
	}

	resultCode := binary.BigEndian.Uint16(resp[2:4])
	if resultCode != natpmpSuccess {
		return c.resultCodeError(resultCode)
	}

	// Remove from our tracking
	for port, m := range c.mappings {
		if m.InternalPort == internalPort {
			delete(c.mappings, port)
			break
		}
	}

	c.logger.Info("natpmp: port mapping deleted", "internal", internalPort)
	return nil
}

// sendRequest sends a NAT-PMP request and waits for response
func (c *NATPMPClient) sendRequest(req []byte) ([]byte, error) {
	addr := &net.UDPAddr{
		IP:   c.gateway,
		Port: natpmpPort,
	}

	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	// NAT-PMP retry schedule: 250ms, 500ms, 1s, 2s, 4s, 8s, 16s, 32s, 64s
	// We'll use a simplified version with 3 retries
	timeouts := []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, 1 * time.Second}

	var lastErr error
	for _, timeout := range timeouts {
		if _, err := conn.Write(req); err != nil {
			lastErr = err
			continue
		}

		_ = conn.SetReadDeadline(time.Now().Add(timeout))

		resp := make([]byte, 16)
		n, err := conn.Read(resp)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				lastErr = ErrNATPMPTimeout
				continue
			}
			lastErr = err
			continue
		}

		return resp[:n], nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrNATPMPTimeout
}

// resultCodeError converts a NAT-PMP result code to an error
func (c *NATPMPClient) resultCodeError(code uint16) error {
	switch code {
	case natpmpSuccess:
		return nil
	case natpmpUnsupportedVersion:
		return errors.New("unsupported NAT-PMP version")
	case natpmpNotAuthorized:
		return errors.New("not authorized")
	case natpmpNetworkFailure:
		return errors.New("network failure")
	case natpmpOutOfResources:
		return errors.New("out of resources")
	case natpmpUnsupportedOpcode:
		return ErrNATPMPNotSupported
	default:
		return fmt.Errorf("unknown error code: %d", code)
	}
}

// ExternalIP returns the cached external IP
func (c *NATPMPClient) ExternalIP() net.IP {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.externalIP
}

// Gateway returns the gateway IP
func (c *NATPMPClient) Gateway() net.IP {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gateway
}

// IsAvailable returns whether NAT-PMP is available
func (c *NATPMPClient) IsAvailable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.discovered && c.gateway != nil
}

// GetMappings returns all active mappings
func (c *NATPMPClient) GetMappings() map[int]*NATPMPMapping {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[int]*NATPMPMapping)
	for k, v := range c.mappings {
		result[k] = v
	}
	return result
}

// Close removes all port mappings
func (c *NATPMPClient) Close() error {
	c.mu.Lock()
	mappings := make([]*NATPMPMapping, 0, len(c.mappings))
	for _, m := range c.mappings {
		mappings = append(mappings, m)
	}
	c.mu.Unlock()

	for _, m := range mappings {
		_ = c.DeletePortMapping(m.InternalPort, m.Protocol)
	}

	return nil
}

// RefreshMappings renews mappings that are about to expire
func (c *NATPMPClient) RefreshMappings() error {
	c.mu.Lock()
	toRefresh := make([]*NATPMPMapping, 0)
	for _, m := range c.mappings {
		// Refresh if less than 5 minutes remaining
		elapsed := time.Since(m.CreatedAt)
		remaining := time.Duration(m.Lifetime)*time.Second - elapsed
		if remaining < 5*time.Minute {
			toRefresh = append(toRefresh, m)
		}
	}
	c.mu.Unlock()

	for _, m := range toRefresh {
		_, err := c.AddPortMapping(m.InternalPort, m.ExternalPort, m.Protocol, m.Lifetime)
		if err != nil {
			c.logger.Warn("natpmp: refresh failed",
				"internal", m.InternalPort,
				"err", err)
		}
	}

	return nil
}

// getDefaultGateway is implemented in gateway_unix.go and gateway_windows.go
