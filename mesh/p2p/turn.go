// Package p2p implements TURN (Traversal Using Relays around NAT) client.
// TURN (RFC 5766/8656) provides relay functionality when direct P2P fails.
package p2p

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// magicCookieBytes for XOR operations
var magicCookieBytes = []byte{0x21, 0x12, 0xA4, 0x42}

// TURN message types
const (
	turnMethodAllocate      = 0x0003
	turnMethodRefresh       = 0x0004
	turnMethodSend          = 0x0006
	turnMethodData          = 0x0007
	turnMethodCreatePerm    = 0x0008
	turnMethodChannelBind   = 0x0009
)

// TURN message classes
const (
	turnClassRequest    = 0x0000
	turnClassIndication = 0x0010
	turnClassSuccess    = 0x0100
	turnClassError      = 0x0110
)

// TURN attributes
const (
	turnAttrMappedAddress     = 0x0001
	turnAttrUsername          = 0x0006
	turnAttrMessageIntegrity  = 0x0008
	turnAttrErrorCode         = 0x0009
	turnAttrChannelNumber     = 0x000C
	turnAttrLifetime          = 0x000D
	turnAttrXorPeerAddress    = 0x0012
	turnAttrData              = 0x0013
	turnAttrRealm             = 0x0014
	turnAttrNonce             = 0x0015
	turnAttrXorRelayedAddr    = 0x0016
	turnAttrRequestedTransport = 0x0019
	turnAttrXorMappedAddress  = 0x0020
	turnAttrSoftware          = 0x8022
	turnAttrFingerprint       = 0x8028
)

// Transport protocols
const (
	turnTransportUDP = 17
	turnTransportTCP = 6
)

// TURN error codes
const (
	turnErrBadRequest        = 400
	turnErrUnauthorized      = 401
	turnErrForbidden         = 403
	turnErrMobilityForbidden = 405
	turnErrUnknownAttr       = 420
	turnErrStaleNonce        = 438
	turnErrServerError       = 500
	turnErrInsuffCapacity    = 508
)

// TURNClient implements TURN protocol for relay-based NAT traversal.
type TURNClient struct {
	mu sync.Mutex

	serverAddr  *net.UDPAddr
	username    string
	password    string
	realm       string
	nonce       string

	conn        *net.UDPConn
	relayAddr   *net.UDPAddr    // Allocated relay address
	mappedAddr  *net.UDPAddr    // Our address as seen by server
	lifetime    time.Duration   // Allocation lifetime
	allocated   bool
	permissions map[string]time.Time // Peer permissions
	channels    map[string]uint16    // Channel bindings

	logger      *slog.Logger
	done        chan struct{}

	// Callback for received data
	onData func(from *net.UDPAddr, data []byte)
}

// TURNConfig holds TURN server configuration.
type TURNConfig struct {
	Server   string // TURN server address (host:port)
	Username string // Authentication username
	Password string // Authentication password
	Realm    string // Optional realm (auto-discovered if empty)
}

// NewTURNClient creates a new TURN client.
func NewTURNClient(cfg *TURNConfig, logger *slog.Logger) (*TURNClient, error) {
	if cfg == nil {
		return nil, errors.New("turn: config required")
	}

	addr, err := net.ResolveUDPAddr("udp", cfg.Server)
	if err != nil {
		return nil, fmt.Errorf("turn: resolve server: %w", err)
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &TURNClient{
		serverAddr:  addr,
		username:    cfg.Username,
		password:    cfg.Password,
		realm:       cfg.Realm,
		permissions: make(map[string]time.Time),
		channels:    make(map[string]uint16),
		logger:      logger,
	}, nil
}

// Allocate requests a relay allocation from the TURN server.
func (c *TURNClient) Allocate(ctx context.Context) error {
	c.mu.Lock()
	if c.allocated {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	// Create UDP connection to TURN server
	conn, err := net.DialUDP("udp", nil, c.serverAddr)
	if err != nil {
		return fmt.Errorf("turn: dial: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.done = make(chan struct{})
	c.mu.Unlock()

	// First request without auth (to get nonce/realm)
	req := c.buildAllocateRequest(false)
	if err := c.sendRequest(ctx, req); err != nil {
		conn.Close()
		return fmt.Errorf("turn: send request: %w", err)
	}

	// Read response
	resp, err := c.readResponse(ctx)
	if err != nil {
		conn.Close()
		return fmt.Errorf("turn: read response: %w", err)
	}

	// Check if we got 401 Unauthorized (expected, need to authenticate)
	if isErrorResponse(resp, turnErrUnauthorized) {
		// Parse realm and nonce
		c.parseAuthChallenge(resp)

		// Retry with authentication
		req = c.buildAllocateRequest(true)
		if err := c.sendRequest(ctx, req); err != nil {
			conn.Close()
			return fmt.Errorf("turn: send auth request: %w", err)
		}

		resp, err = c.readResponse(ctx)
		if err != nil {
			conn.Close()
			return fmt.Errorf("turn: read auth response: %w", err)
		}
	}

	// Check for success
	if !isSuccessResponse(resp, turnMethodAllocate) {
		conn.Close()
		code, msg := parseErrorResponse(resp)
		return fmt.Errorf("turn: allocate failed: %d %s", code, msg)
	}

	// Parse allocation response
	if err := c.parseAllocateResponse(resp); err != nil {
		conn.Close()
		return fmt.Errorf("turn: parse response: %w", err)
	}

	c.mu.Lock()
	c.allocated = true
	c.mu.Unlock()

	// Start receiver and refresh goroutines
	go c.receiveLoop()
	go c.refreshLoop()

	c.logger.Info("TURN allocation successful",
		"relay_addr", c.relayAddr,
		"mapped_addr", c.mappedAddr,
		"lifetime", c.lifetime)

	return nil
}

// RelayAddr returns the allocated relay address.
func (c *TURNClient) RelayAddr() *net.UDPAddr {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.relayAddr
}

// MappedAddr returns our address as seen by the TURN server.
func (c *TURNClient) MappedAddr() *net.UDPAddr {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mappedAddr
}

// IsAllocated returns whether we have an active allocation.
func (c *TURNClient) IsAllocated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.allocated
}

// CreatePermission creates a permission for a peer to send data to us.
func (c *TURNClient) CreatePermission(ctx context.Context, peerAddr *net.UDPAddr) error {
	c.mu.Lock()
	if !c.allocated {
		c.mu.Unlock()
		return errors.New("turn: not allocated")
	}
	c.mu.Unlock()

	req := c.buildCreatePermissionRequest(peerAddr)
	if err := c.sendRequest(ctx, req); err != nil {
		return fmt.Errorf("turn: send permission request: %w", err)
	}

	resp, err := c.readResponse(ctx)
	if err != nil {
		return fmt.Errorf("turn: read permission response: %w", err)
	}

	if !isSuccessResponse(resp, turnMethodCreatePerm) {
		code, msg := parseErrorResponse(resp)
		return fmt.Errorf("turn: permission failed: %d %s", code, msg)
	}

	c.mu.Lock()
	c.permissions[peerAddr.IP.String()] = time.Now().Add(5 * time.Minute)
	c.mu.Unlock()

	c.logger.Debug("TURN permission created", "peer", peerAddr.IP)
	return nil
}

// Send sends data to a peer through the relay.
func (c *TURNClient) Send(peerAddr *net.UDPAddr, data []byte) error {
	c.mu.Lock()
	if !c.allocated {
		c.mu.Unlock()
		return errors.New("turn: not allocated")
	}
	conn := c.conn
	c.mu.Unlock()

	// Build Send indication
	req := c.buildSendIndication(peerAddr, data)

	_, err := conn.Write(req)
	return err
}

// OnData sets a callback for received data.
func (c *TURNClient) OnData(cb func(from *net.UDPAddr, data []byte)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onData = cb
}

// Close releases the allocation and closes the connection.
func (c *TURNClient) Close() error {
	c.mu.Lock()
	if !c.allocated {
		c.mu.Unlock()
		return nil
	}

	close(c.done)
	c.allocated = false
	conn := c.conn
	c.mu.Unlock()

	// Send refresh with lifetime=0 to release allocation
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := c.buildRefreshRequest(0)
	_ = c.sendRequest(ctx, req)

	if conn != nil {
		conn.Close()
	}

	c.logger.Info("TURN allocation released")
	return nil
}

// buildAllocateRequest builds an Allocate request.
func (c *TURNClient) buildAllocateRequest(withAuth bool) []byte {
	buf := make([]byte, 512)
	offset := 0

	// STUN header
	msgType := uint16(turnMethodAllocate | turnClassRequest)
	binary.BigEndian.PutUint16(buf[offset:], msgType)
	offset += 2
	offset += 2 // Length placeholder
	binary.BigEndian.PutUint32(buf[offset:], 0x2112A442) // Magic cookie
	offset += 4
	// Transaction ID (12 bytes)
	for i := 0; i < 12; i++ {
		buf[offset+i] = byte(time.Now().UnixNano() >> (i * 4))
	}
	offset += 12

	// Requested transport attribute (UDP)
	binary.BigEndian.PutUint16(buf[offset:], turnAttrRequestedTransport)
	binary.BigEndian.PutUint16(buf[offset+2:], 4)
	buf[offset+4] = turnTransportUDP
	offset += 8

	if withAuth {
		offset = c.addAuthAttributes(buf, offset)
	}

	// Update length
	binary.BigEndian.PutUint16(buf[2:], uint16(offset-20))

	if withAuth {
		// Add message integrity
		offset = c.addMessageIntegrity(buf, offset)
	}

	return buf[:offset]
}

// buildRefreshRequest builds a Refresh request.
func (c *TURNClient) buildRefreshRequest(lifetime int) []byte {
	buf := make([]byte, 512)
	offset := 0

	// STUN header
	msgType := uint16(turnMethodRefresh | turnClassRequest)
	binary.BigEndian.PutUint16(buf[offset:], msgType)
	offset += 2
	offset += 2 // Length placeholder
	binary.BigEndian.PutUint32(buf[offset:], 0x2112A442)
	offset += 4
	for i := 0; i < 12; i++ {
		buf[offset+i] = byte(time.Now().UnixNano() >> (i * 4))
	}
	offset += 12

	// Lifetime attribute
	binary.BigEndian.PutUint16(buf[offset:], turnAttrLifetime)
	binary.BigEndian.PutUint16(buf[offset+2:], 4)
	binary.BigEndian.PutUint32(buf[offset+4:], uint32(lifetime))
	offset += 8

	offset = c.addAuthAttributes(buf, offset)
	binary.BigEndian.PutUint16(buf[2:], uint16(offset-20))
	offset = c.addMessageIntegrity(buf, offset)

	return buf[:offset]
}

// buildCreatePermissionRequest builds a CreatePermission request.
func (c *TURNClient) buildCreatePermissionRequest(peerAddr *net.UDPAddr) []byte {
	buf := make([]byte, 512)
	offset := 0

	// STUN header
	msgType := uint16(turnMethodCreatePerm | turnClassRequest)
	binary.BigEndian.PutUint16(buf[offset:], msgType)
	offset += 2
	offset += 2 // Length placeholder
	binary.BigEndian.PutUint32(buf[offset:], 0x2112A442)
	offset += 4
	for i := 0; i < 12; i++ {
		buf[offset+i] = byte(time.Now().UnixNano() >> (i * 4))
	}
	offset += 12

	// XOR-PEER-ADDRESS attribute
	offset = c.addXorPeerAddress(buf, offset, peerAddr)

	offset = c.addAuthAttributes(buf, offset)
	binary.BigEndian.PutUint16(buf[2:], uint16(offset-20))
	offset = c.addMessageIntegrity(buf, offset)

	return buf[:offset]
}

// buildSendIndication builds a Send indication.
func (c *TURNClient) buildSendIndication(peerAddr *net.UDPAddr, data []byte) []byte {
	buf := make([]byte, 512+len(data))
	offset := 0

	// STUN header
	msgType := uint16(turnMethodSend | turnClassIndication)
	binary.BigEndian.PutUint16(buf[offset:], msgType)
	offset += 2
	offset += 2 // Length placeholder
	binary.BigEndian.PutUint32(buf[offset:], 0x2112A442)
	offset += 4
	for i := 0; i < 12; i++ {
		buf[offset+i] = byte(time.Now().UnixNano() >> (i * 4))
	}
	offset += 12

	// XOR-PEER-ADDRESS
	offset = c.addXorPeerAddress(buf, offset, peerAddr)

	// DATA attribute
	dataLen := len(data)
	binary.BigEndian.PutUint16(buf[offset:], turnAttrData)
	binary.BigEndian.PutUint16(buf[offset+2:], uint16(dataLen))
	offset += 4
	copy(buf[offset:], data)
	offset += dataLen
	// Padding
	for offset%4 != 0 {
		offset++
	}

	// Update length
	binary.BigEndian.PutUint16(buf[2:], uint16(offset-20))

	return buf[:offset]
}

// addXorPeerAddress adds XOR-PEER-ADDRESS attribute.
func (c *TURNClient) addXorPeerAddress(buf []byte, offset int, addr *net.UDPAddr) int {
	binary.BigEndian.PutUint16(buf[offset:], turnAttrXorPeerAddress)
	binary.BigEndian.PutUint16(buf[offset+2:], 8)
	offset += 4

	buf[offset] = 0 // Reserved
	buf[offset+1] = 1 // Family (IPv4)
	// XOR port with magic cookie (first 2 bytes)
	xorPort := uint16(addr.Port) ^ 0x2112
	binary.BigEndian.PutUint16(buf[offset+2:], xorPort)
	// XOR address with magic cookie
	ip4 := addr.IP.To4()
	if ip4 != nil {
		for i := 0; i < 4; i++ {
			buf[offset+4+i] = ip4[i] ^ magicCookieBytes[i]
		}
	}
	offset += 8

	return offset
}

// addAuthAttributes adds username, realm, nonce attributes.
func (c *TURNClient) addAuthAttributes(buf []byte, offset int) int {
	c.mu.Lock()
	username := c.username
	realm := c.realm
	nonce := c.nonce
	c.mu.Unlock()

	// Username
	usernameLen := len(username)
	binary.BigEndian.PutUint16(buf[offset:], turnAttrUsername)
	binary.BigEndian.PutUint16(buf[offset+2:], uint16(usernameLen))
	offset += 4
	copy(buf[offset:], username)
	offset += usernameLen
	for offset%4 != 0 {
		offset++
	}

	// Realm
	realmLen := len(realm)
	binary.BigEndian.PutUint16(buf[offset:], turnAttrRealm)
	binary.BigEndian.PutUint16(buf[offset+2:], uint16(realmLen))
	offset += 4
	copy(buf[offset:], realm)
	offset += realmLen
	for offset%4 != 0 {
		offset++
	}

	// Nonce
	nonceLen := len(nonce)
	binary.BigEndian.PutUint16(buf[offset:], turnAttrNonce)
	binary.BigEndian.PutUint16(buf[offset+2:], uint16(nonceLen))
	offset += 4
	copy(buf[offset:], nonce)
	offset += nonceLen
	for offset%4 != 0 {
		offset++
	}

	return offset
}

// addMessageIntegrity adds MESSAGE-INTEGRITY attribute.
func (c *TURNClient) addMessageIntegrity(buf []byte, offset int) int {
	// Calculate key: MD5(username:realm:password)
	c.mu.Lock()
	key := c.calculateKey()
	c.mu.Unlock()

	// Update message length to include MESSAGE-INTEGRITY
	binary.BigEndian.PutUint16(buf[2:], uint16(offset-20+24)) // 24 = 4 + 20 (HMAC-SHA1)

	// Calculate HMAC-SHA1
	h := hmac.New(sha1.New, key)
	h.Write(buf[:offset])
	mac := h.Sum(nil)

	// Add attribute
	binary.BigEndian.PutUint16(buf[offset:], turnAttrMessageIntegrity)
	binary.BigEndian.PutUint16(buf[offset+2:], 20)
	offset += 4
	copy(buf[offset:], mac)
	offset += 20

	return offset
}

// calculateKey calculates the long-term credential key.
func (c *TURNClient) calculateKey() []byte {
	// key = MD5(username:realm:password)
	h := md5.New()
	h.Write([]byte(c.username + ":" + c.realm + ":" + c.password))
	return h.Sum(nil)
}

// sendRequest sends a request packet.
func (c *TURNClient) sendRequest(ctx context.Context, data []byte) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return errors.New("turn: not connected")
	}

	_, err := conn.Write(data)
	return err
}

// readResponse reads a response packet.
func (c *TURNClient) readResponse(ctx context.Context) ([]byte, error) {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return nil, errors.New("turn: not connected")
	}

	buf := make([]byte, 2048)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}

	return buf[:n], nil
}

// parseAuthChallenge parses realm and nonce from 401 response.
func (c *TURNClient) parseAuthChallenge(data []byte) {
	// Skip header
	offset := 20

	for offset+4 <= len(data) {
		attrType := binary.BigEndian.Uint16(data[offset:])
		attrLen := int(binary.BigEndian.Uint16(data[offset+2:]))
		offset += 4

		if offset+attrLen > len(data) {
			break
		}

		switch attrType {
		case turnAttrRealm:
			c.mu.Lock()
			c.realm = string(data[offset : offset+attrLen])
			c.mu.Unlock()
		case turnAttrNonce:
			c.mu.Lock()
			c.nonce = string(data[offset : offset+attrLen])
			c.mu.Unlock()
		}

		offset += attrLen
		for offset%4 != 0 {
			offset++
		}
	}
}

// parseAllocateResponse parses a successful Allocate response.
func (c *TURNClient) parseAllocateResponse(data []byte) error {
	offset := 20

	for offset+4 <= len(data) {
		attrType := binary.BigEndian.Uint16(data[offset:])
		attrLen := int(binary.BigEndian.Uint16(data[offset+2:]))
		offset += 4

		if offset+attrLen > len(data) {
			break
		}

		switch attrType {
		case turnAttrXorRelayedAddr:
			addr := c.parseXorAddress(data[offset:offset+attrLen], data)
			c.mu.Lock()
			c.relayAddr = addr
			c.mu.Unlock()

		case turnAttrXorMappedAddress:
			addr := c.parseXorAddress(data[offset:offset+attrLen], data)
			c.mu.Lock()
			c.mappedAddr = addr
			c.mu.Unlock()

		case turnAttrLifetime:
			lifetime := binary.BigEndian.Uint32(data[offset:])
			c.mu.Lock()
			c.lifetime = time.Duration(lifetime) * time.Second
			c.mu.Unlock()
		}

		offset += attrLen
		for offset%4 != 0 {
			offset++
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.relayAddr == nil {
		return errors.New("turn: no relay address in response")
	}

	return nil
}

// parseXorAddress parses an XOR-*-ADDRESS attribute.
func (c *TURNClient) parseXorAddress(data []byte, msg []byte) *net.UDPAddr {
	if len(data) < 8 {
		return nil
	}

	// Skip reserved byte
	family := data[1]
	xorPort := binary.BigEndian.Uint16(data[2:4])
	port := int(xorPort ^ 0x2112)

	var ip net.IP
	if family == 1 { // IPv4
		ip = make(net.IP, 4)
		for i := 0; i < 4; i++ {
			ip[i] = data[4+i] ^ magicCookieBytes[i]
		}
	} else if family == 2 { // IPv6
		ip = make(net.IP, 16)
		// XOR with magic cookie + transaction ID
		for i := 0; i < 16; i++ {
			if i < 4 {
				ip[i] = data[4+i] ^ magicCookieBytes[i]
			} else {
				ip[i] = data[4+i] ^ msg[4+i] // XOR with transaction ID
			}
		}
	}

	return &net.UDPAddr{IP: ip, Port: port}
}

// receiveLoop receives data from the TURN server.
func (c *TURNClient) receiveLoop() {
	c.mu.Lock()
	conn := c.conn
	done := c.done
	c.mu.Unlock()

	buf := make([]byte, 4096)

	for {
		select {
		case <-done:
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-done:
				return
			default:
				c.logger.Debug("TURN receive error", "err", err)
				continue
			}
		}

		c.handleMessage(buf[:n])
	}
}

// handleMessage handles an incoming TURN message.
func (c *TURNClient) handleMessage(data []byte) {
	if len(data) < 20 {
		return
	}

	msgType := binary.BigEndian.Uint16(data[0:2])

	// Check if it's a Data indication
	if msgType == (turnMethodData | turnClassIndication) {
		c.handleDataIndication(data)
	}
}

// handleDataIndication handles incoming Data indication.
func (c *TURNClient) handleDataIndication(data []byte) {
	offset := 20
	var peerAddr *net.UDPAddr
	var payload []byte

	for offset+4 <= len(data) {
		attrType := binary.BigEndian.Uint16(data[offset:])
		attrLen := int(binary.BigEndian.Uint16(data[offset+2:]))
		offset += 4

		if offset+attrLen > len(data) {
			break
		}

		switch attrType {
		case turnAttrXorPeerAddress:
			peerAddr = c.parseXorAddress(data[offset:offset+attrLen], data)
		case turnAttrData:
			payload = make([]byte, attrLen)
			copy(payload, data[offset:offset+attrLen])
		}

		offset += attrLen
		for offset%4 != 0 {
			offset++
		}
	}

	if peerAddr != nil && payload != nil {
		c.mu.Lock()
		cb := c.onData
		c.mu.Unlock()

		if cb != nil {
			cb(peerAddr, payload)
		}
	}
}

// refreshLoop periodically refreshes the allocation.
func (c *TURNClient) refreshLoop() {
	c.mu.Lock()
	lifetime := c.lifetime
	done := c.done
	c.mu.Unlock()

	// Refresh at 80% of lifetime
	interval := time.Duration(float64(lifetime) * 0.8)
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := c.refresh(ctx); err != nil {
				c.logger.Warn("TURN refresh failed", "err", err)
			}
			cancel()
		}
	}
}

// refresh sends a Refresh request.
func (c *TURNClient) refresh(ctx context.Context) error {
	req := c.buildRefreshRequest(600) // Request 10 minutes

	if err := c.sendRequest(ctx, req); err != nil {
		return err
	}

	resp, err := c.readResponse(ctx)
	if err != nil {
		return err
	}

	if !isSuccessResponse(resp, turnMethodRefresh) {
		code, msg := parseErrorResponse(resp)
		return fmt.Errorf("refresh failed: %d %s", code, msg)
	}

	c.logger.Debug("TURN allocation refreshed")
	return nil
}

// Helper functions

func isSuccessResponse(data []byte, method int) bool {
	if len(data) < 2 {
		return false
	}
	msgType := binary.BigEndian.Uint16(data[0:2])
	return msgType == uint16(method|turnClassSuccess)
}

func isErrorResponse(data []byte, code int) bool {
	if len(data) < 20 {
		return false
	}

	msgType := binary.BigEndian.Uint16(data[0:2])
	if msgType&0x0110 != turnClassError {
		return false
	}

	// Parse error code
	errorCode, _ := parseErrorResponse(data)
	return errorCode == code
}

func parseErrorResponse(data []byte) (int, string) {
	offset := 20

	for offset+4 <= len(data) {
		attrType := binary.BigEndian.Uint16(data[offset:])
		attrLen := int(binary.BigEndian.Uint16(data[offset+2:]))
		offset += 4

		if offset+attrLen > len(data) {
			break
		}

		if attrType == turnAttrErrorCode {
			if attrLen >= 4 {
				errorClass := int(data[offset+2] & 0x07)
				errorNumber := int(data[offset+3])
				errorCode := errorClass*100 + errorNumber
				reason := ""
				if attrLen > 4 {
					reason = string(data[offset+4 : offset+attrLen])
				}
				return errorCode, reason
			}
		}

		offset += attrLen
		for offset%4 != 0 {
			offset++
		}
	}

	return 0, ""
}

// DefaultTURNServers returns a list of public TURN servers.
// Note: Public TURN servers are rare and usually require authentication.
func DefaultTURNServers() []string {
	return []string{
		// Note: These are placeholder addresses. In production, you need your own TURN server
		// or use a commercial service like Twilio, Xirsys, etc.
	}
}
