// Package p2p implements mDNS (Multicast DNS) for local peer discovery.
// mDNS allows Shuttle clients on the same LAN to discover each other
// without requiring a central server.
package p2p

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	// mDNS multicast addresses
	mdnsIPv4Addr = "224.0.0.251:5353"
	mdnsIPv6Addr = "[ff02::fb]:5353"

	// Service name for Shuttle P2P discovery
	shuttleServiceName = "_shuttle._udp.local."

	// mDNS packet constants
	mdnsMaxPacketSize = 9000
	mdnsTTL           = 120 // seconds
)

// mDNS flags and types
const (
	mdnsQueryFlag    = 0x0000
	mdnsResponseFlag = 0x8400

	mdnsTypeA     = 1   // IPv4 address
	mdnsTypeAAAA  = 28  // IPv6 address
	mdnsTypePTR   = 12  // Pointer (service discovery)
	mdnsTypeSRV   = 33  // Service location
	mdnsTypeTXT   = 16  // Text record
	mdnsClassIN   = 1   // Internet class
)

// mdnsClassFlush is CLASS with cache-flush bit set
const mdnsClassFlush uint16 = 0x8001

// MDNSPeer represents a discovered peer on the local network.
type MDNSPeer struct {
	Name      string        // Peer instance name
	VIP       net.IP        // Virtual IP (mesh VIP)
	Addresses []net.IP      // Local IP addresses
	Port      int           // P2P UDP port
	LastSeen  time.Time     // Last time this peer was seen
	Metadata  map[string]string // Additional metadata from TXT records
}

// MDNSService handles mDNS service announcement and discovery.
type MDNSService struct {
	mu sync.RWMutex

	instanceName string            // Our instance name
	vip          net.IP            // Our mesh VIP
	port         int               // Our P2P port
	metadata     map[string]string // Our metadata

	conn4   *net.UDPConn // IPv4 multicast connection
	conn6   *net.UDPConn // IPv6 multicast connection
	logger  *slog.Logger

	peers   map[string]*MDNSPeer // Discovered peers by name
	running bool
	done    chan struct{}

	// Callbacks
	onPeerDiscovered func(*MDNSPeer)
	onPeerLost       func(*MDNSPeer)
}

// NewMDNSService creates a new mDNS service.
func NewMDNSService(instanceName string, logger *slog.Logger) *MDNSService {
	if logger == nil {
		logger = slog.Default()
	}

	// Generate a unique instance name if not provided
	if instanceName == "" {
		instanceName = fmt.Sprintf("shuttle-%d", time.Now().UnixNano()%100000)
	}

	return &MDNSService{
		instanceName: instanceName,
		metadata:     make(map[string]string),
		peers:        make(map[string]*MDNSPeer),
		logger:       logger,
	}
}

// Start begins mDNS service announcement and discovery.
func (s *MDNSService) Start(vip net.IP, port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	s.vip = vip
	s.port = port
	s.done = make(chan struct{})

	// Create IPv4 multicast socket
	addr4, err := net.ResolveUDPAddr("udp4", mdnsIPv4Addr)
	if err != nil {
		return fmt.Errorf("mdns: resolve IPv4 addr: %w", err)
	}

	s.conn4, err = net.ListenMulticastUDP("udp4", nil, addr4)
	if err != nil {
		s.logger.Warn("mdns: IPv4 multicast not available", "err", err)
	}

	// Try IPv6 (optional)
	addr6, err := net.ResolveUDPAddr("udp6", mdnsIPv6Addr)
	if err == nil {
		s.conn6, err = net.ListenMulticastUDP("udp6", nil, addr6)
		if err != nil {
			s.logger.Debug("mdns: IPv6 multicast not available", "err", err)
		}
	}

	if s.conn4 == nil && s.conn6 == nil {
		return fmt.Errorf("mdns: no multicast connection available")
	}

	s.running = true

	// Start receiver goroutines
	if s.conn4 != nil {
		go s.receiveLoop(s.conn4)
	}
	if s.conn6 != nil {
		go s.receiveLoop(s.conn6)
	}

	// Start announcement goroutine
	go s.announceLoop()

	// Start peer expiry goroutine
	go s.expiryLoop()

	s.logger.Info("mDNS service started",
		"instance", s.instanceName,
		"vip", vip,
		"port", port)

	return nil
}

// Stop stops the mDNS service.
func (s *MDNSService) Stop() error {
	s.mu.Lock()

	if !s.running {
		s.mu.Unlock()
		return nil
	}

	close(s.done)
	s.running = false

	// Get connection references before unlocking
	conn4 := s.conn4
	conn6 := s.conn6
	s.mu.Unlock()

	// Send goodbye (TTL=0) - outside of lock since it acquires RLock
	s.sendGoodbye()

	// Close connections
	if conn4 != nil {
		conn4.Close()
	}
	if conn6 != nil {
		conn6.Close()
	}

	s.logger.Info("mDNS service stopped")
	return nil
}

// SetMetadata sets metadata to be advertised.
func (s *MDNSService) SetMetadata(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metadata[key] = value
}

// OnPeerDiscovered sets a callback for when a new peer is discovered.
func (s *MDNSService) OnPeerDiscovered(cb func(*MDNSPeer)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPeerDiscovered = cb
}

// OnPeerLost sets a callback for when a peer is no longer available.
func (s *MDNSService) OnPeerLost(cb func(*MDNSPeer)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPeerLost = cb
}

// GetPeers returns all discovered peers.
func (s *MDNSService) GetPeers() []*MDNSPeer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*MDNSPeer, 0, len(s.peers))
	for _, p := range s.peers {
		result = append(result, p)
	}
	return result
}

// GetPeer returns a specific peer by name.
func (s *MDNSService) GetPeer(name string) *MDNSPeer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.peers[name]
}

// Query sends a query to discover peers.
func (s *MDNSService) Query() error {
	s.mu.RLock()
	running := s.running
	s.mu.RUnlock()

	if !running {
		return fmt.Errorf("mdns: service not running")
	}

	packet := s.buildQuery()
	return s.sendPacket(packet)
}

// receiveLoop receives and processes mDNS packets.
func (s *MDNSService) receiveLoop(conn *net.UDPConn) {
	buf := make([]byte, mdnsMaxPacketSize)

	for {
		select {
		case <-s.done:
			return
		default:
		}

		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-s.done:
				return
			default:
				s.logger.Debug("mdns: read error", "err", err)
				continue
			}
		}

		s.processPacket(buf[:n], from)
	}
}

// announceLoop periodically announces our presence.
func (s *MDNSService) announceLoop() {
	// Initial announcement
	s.announce()

	ticker := time.NewTicker(time.Duration(mdnsTTL/2) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.announce()
		}
	}
}

// expiryLoop removes stale peers.
func (s *MDNSService) expiryLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.expirePeers()
		}
	}
}

// announce sends our service announcement.
func (s *MDNSService) announce() {
	s.mu.RLock()
	if !s.running {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()

	packet := s.buildResponse()
	if err := s.sendPacket(packet); err != nil {
		s.logger.Debug("mdns: announce failed", "err", err)
	}
}

// sendGoodbye sends a goodbye packet (TTL=0).
func (s *MDNSService) sendGoodbye() {
	// Build response with TTL=0
	packet := s.buildResponseWithTTL(0)
	_ = s.sendPacket(packet)
}

// sendPacket sends a packet to the mDNS multicast address.
func (s *MDNSService) sendPacket(packet []byte) error {
	var lastErr error

	if s.conn4 != nil {
		addr, _ := net.ResolveUDPAddr("udp4", mdnsIPv4Addr)
		if _, err := s.conn4.WriteToUDP(packet, addr); err != nil {
			lastErr = err
		}
	}

	if s.conn6 != nil {
		addr, _ := net.ResolveUDPAddr("udp6", mdnsIPv6Addr)
		if _, err := s.conn6.WriteToUDP(packet, addr); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// processPacket processes an incoming mDNS packet.
func (s *MDNSService) processPacket(data []byte, from *net.UDPAddr) {
	if len(data) < 12 {
		return
	}

	// Parse header
	flags := uint16(data[2])<<8 | uint16(data[3])
	qdCount := int(data[4])<<8 | int(data[5])
	anCount := int(data[6])<<8 | int(data[7])

	// Skip queries
	if flags&0x8000 == 0 && qdCount > 0 {
		// This is a query, we might want to respond
		s.handleQuery(data)
		return
	}

	// Process answers
	if anCount == 0 {
		return
	}

	// Parse resource records
	offset := 12

	// Skip questions
	for i := 0; i < qdCount && offset < len(data); i++ {
		_, newOffset := parseDNSName(data, offset)
		offset = newOffset + 4 // Skip QTYPE and QCLASS
	}

	// Parse answers
	for i := 0; i < anCount && offset < len(data); i++ {
		name, newOffset := parseDNSName(data, offset)
		if newOffset+10 > len(data) {
			break
		}
		offset = newOffset

		rtype := uint16(data[offset])<<8 | uint16(data[offset+1])
		// rclass := uint16(data[offset+2])<<8 | uint16(data[offset+3])
		ttl := uint32(data[offset+4])<<24 | uint32(data[offset+5])<<16 |
			uint32(data[offset+6])<<8 | uint32(data[offset+7])
		rdlen := int(data[offset+8])<<8 | int(data[offset+9])
		offset += 10

		if offset+rdlen > len(data) {
			break
		}

		rdata := data[offset : offset+rdlen]
		offset += rdlen

		s.processRecord(name, rtype, ttl, rdata, data, from)
	}
}

// handleQuery handles an mDNS query and responds if needed.
func (s *MDNSService) handleQuery(data []byte) {
	if len(data) < 12 {
		return
	}

	qdCount := int(data[4])<<8 | int(data[5])
	offset := 12

	for i := 0; i < qdCount && offset < len(data); i++ {
		name, newOffset := parseDNSName(data, offset)
		if newOffset+4 > len(data) {
			break
		}
		offset = newOffset + 4

		// Check if query is for our service
		if strings.Contains(name, "_shuttle._udp") {
			s.announce() // Respond to query
			return
		}
	}
}

// processRecord processes a resource record.
func (s *MDNSService) processRecord(name string, rtype uint16, ttl uint32, rdata []byte, packet []byte, from *net.UDPAddr) {
	// Only process Shuttle service records
	if !strings.Contains(name, "_shuttle") && !strings.HasSuffix(name, ".local.") {
		return
	}

	switch rtype {
	case mdnsTypePTR:
		// Service pointer - parse instance name
		instanceName, _ := parseDNSName(packet, len(packet)-len(rdata))
		if instanceName != "" && instanceName != s.instanceName {
			s.updatePeer(instanceName, from, ttl)
		}

	case mdnsTypeSRV:
		// Service location record
		if len(rdata) >= 6 {
			port := int(rdata[4])<<8 | int(rdata[5])
			s.updatePeerPort(name, port, ttl)
		}

	case mdnsTypeTXT:
		// Text record with metadata
		s.updatePeerMetadata(name, rdata, ttl)

	case mdnsTypeA:
		// IPv4 address
		if len(rdata) == 4 {
			ip := net.IPv4(rdata[0], rdata[1], rdata[2], rdata[3])
			s.updatePeerAddress(name, ip, ttl)
		}

	case mdnsTypeAAAA:
		// IPv6 address
		if len(rdata) == 16 {
			ip := make(net.IP, 16)
			copy(ip, rdata)
			s.updatePeerAddress(name, ip, ttl)
		}
	}
}

// updatePeer updates or creates a peer record.
func (s *MDNSService) updatePeer(name string, from *net.UDPAddr, ttl uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ignore ourselves
	if name == s.instanceName || strings.HasPrefix(name, s.instanceName) {
		return
	}

	// Extract just the instance name from full PTR record
	instanceName := name
	if idx := strings.Index(name, "._shuttle"); idx > 0 {
		instanceName = name[:idx]
	}

	peer, exists := s.peers[instanceName]
	if !exists {
		peer = &MDNSPeer{
			Name:      instanceName,
			Addresses: []net.IP{},
			Metadata:  make(map[string]string),
		}
		s.peers[instanceName] = peer

		s.logger.Debug("mDNS: new peer discovered", "name", instanceName)

		if s.onPeerDiscovered != nil {
			go s.onPeerDiscovered(peer)
		}
	}

	peer.LastSeen = time.Now()
	if from != nil && from.IP != nil {
		s.addPeerAddress(peer, from.IP)
	}
}

// updatePeerPort updates a peer's port.
func (s *MDNSService) updatePeerPort(name string, port int, ttl uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	instanceName := extractInstanceName(name)
	if peer, ok := s.peers[instanceName]; ok {
		peer.Port = port
		peer.LastSeen = time.Now()
	}
}

// updatePeerMetadata updates a peer's metadata from TXT record.
func (s *MDNSService) updatePeerMetadata(name string, rdata []byte, ttl uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	instanceName := extractInstanceName(name)
	peer, ok := s.peers[instanceName]
	if !ok {
		return
	}

	// Parse TXT record strings
	offset := 0
	for offset < len(rdata) {
		length := int(rdata[offset])
		offset++
		if offset+length > len(rdata) {
			break
		}
		txt := string(rdata[offset : offset+length])
		offset += length

		if idx := strings.Index(txt, "="); idx > 0 {
			key := txt[:idx]
			value := txt[idx+1:]
			peer.Metadata[key] = value

			// Handle special keys
			if key == "vip" {
				peer.VIP = net.ParseIP(value)
			}
		}
	}
	peer.LastSeen = time.Now()
}

// updatePeerAddress adds an address to a peer.
func (s *MDNSService) updatePeerAddress(name string, ip net.IP, ttl uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	instanceName := extractInstanceName(name)
	peer, ok := s.peers[instanceName]
	if !ok {
		return
	}

	s.addPeerAddress(peer, ip)
	peer.LastSeen = time.Now()
}

// addPeerAddress adds an IP to peer's address list if not already present.
func (s *MDNSService) addPeerAddress(peer *MDNSPeer, ip net.IP) {
	for _, existing := range peer.Addresses {
		if existing.Equal(ip) {
			return
		}
	}
	peer.Addresses = append(peer.Addresses, ip)
}

// expirePeers removes peers that haven't been seen recently.
func (s *MDNSService) expirePeers() {
	s.mu.Lock()
	defer s.mu.Unlock()

	threshold := time.Now().Add(-time.Duration(mdnsTTL*2) * time.Second)

	for name, peer := range s.peers {
		if peer.LastSeen.Before(threshold) {
			delete(s.peers, name)
			s.logger.Debug("mDNS: peer expired", "name", name)

			if s.onPeerLost != nil {
				go s.onPeerLost(peer)
			}
		}
	}
}

// buildQuery builds an mDNS query packet.
func (s *MDNSService) buildQuery() []byte {
	buf := make([]byte, 512)
	offset := 0

	// Transaction ID (0 for mDNS)
	offset += 2

	// Flags (query)
	buf[offset] = 0x00
	buf[offset+1] = 0x00
	offset += 2

	// QDCOUNT = 1
	buf[offset+1] = 1
	offset += 2

	// ANCOUNT, NSCOUNT, ARCOUNT = 0
	offset += 6

	// Question: _shuttle._udp.local PTR
	offset += writeDNSName(buf[offset:], shuttleServiceName)
	buf[offset] = 0x00
	buf[offset+1] = mdnsTypePTR // QTYPE
	buf[offset+2] = 0x00
	buf[offset+3] = mdnsClassIN // QCLASS
	offset += 4

	return buf[:offset]
}

// buildResponse builds an mDNS response packet.
func (s *MDNSService) buildResponse() []byte {
	return s.buildResponseWithTTL(mdnsTTL)
}

// buildResponseWithTTL builds an mDNS response with specified TTL.
func (s *MDNSService) buildResponseWithTTL(ttl int) []byte {
	s.mu.RLock()
	instanceName := s.instanceName
	vip := s.vip
	port := s.port
	metadata := make(map[string]string)
	for k, v := range s.metadata {
		metadata[k] = v
	}
	s.mu.RUnlock()

	buf := make([]byte, 1500)
	offset := 0

	// Transaction ID (0 for mDNS)
	offset += 2

	// Flags (response, authoritative)
	buf[offset] = 0x84
	buf[offset+1] = 0x00
	offset += 2

	// QDCOUNT = 0
	offset += 2

	// ANCOUNT = 4 (PTR, SRV, TXT, A)
	buf[offset+1] = 4
	offset += 2

	// NSCOUNT, ARCOUNT = 0
	offset += 4

	fullName := instanceName + "." + shuttleServiceName

	// PTR record
	offset += writeDNSName(buf[offset:], shuttleServiceName)
	buf[offset] = 0x00
	buf[offset+1] = mdnsTypePTR
	buf[offset+2] = 0x00
	buf[offset+3] = mdnsClassIN
	offset += 4
	offset += writeTTL(buf[offset:], uint32(ttl)) //nolint:gosec // G115: TTL is a small positive duration in seconds
	ptrData := []byte{}
	ptrData = append(ptrData, writeDNSNameBytes(fullName)...)
	buf[offset] = byte(len(ptrData) >> 8)
	buf[offset+1] = byte(len(ptrData))
	offset += 2
	copy(buf[offset:], ptrData)
	offset += len(ptrData)

	// SRV record
	offset += writeDNSName(buf[offset:], fullName)
	buf[offset] = 0x00
	buf[offset+1] = mdnsTypeSRV
	buf[offset+2] = 0x80 // Cache flush + IN class
	buf[offset+3] = 0x01
	offset += 4
	offset += writeTTL(buf[offset:], uint32(ttl)) //nolint:gosec // G115: TTL is a small positive duration in seconds
	// SRV RDATA: priority(2) + weight(2) + port(2) + target
	hostName := instanceName + ".local."
	srvRdataLen := 6 + len(writeDNSNameBytes(hostName))
	buf[offset] = byte(srvRdataLen >> 8)
	buf[offset+1] = byte(srvRdataLen)
	offset += 2
	buf[offset] = 0
	buf[offset+1] = 0 // Priority
	buf[offset+2] = 0
	buf[offset+3] = 0 // Weight
	buf[offset+4] = byte(port >> 8)
	buf[offset+5] = byte(port)
	offset += 6
	offset += writeDNSName(buf[offset:], hostName)

	// TXT record with metadata
	offset += writeDNSName(buf[offset:], fullName)
	buf[offset] = 0x00
	buf[offset+1] = mdnsTypeTXT
	buf[offset+2] = 0x80 // Cache flush + IN class
	buf[offset+3] = 0x01
	offset += 4
	offset += writeTTL(buf[offset:], uint32(ttl)) //nolint:gosec // G115: TTL is a small positive duration in seconds

	// Build TXT RDATA
	txtData := []byte{}
	if vip != nil {
		txtEntry := "vip=" + vip.String()
		txtData = append(txtData, byte(len(txtEntry)))
		txtData = append(txtData, txtEntry...)
	}
	for k, v := range metadata {
		txtEntry := k + "=" + v
		txtData = append(txtData, byte(len(txtEntry)))
		txtData = append(txtData, txtEntry...)
	}
	if len(txtData) == 0 {
		txtData = []byte{0} // Empty TXT record
	}
	buf[offset] = byte(len(txtData) >> 8)
	buf[offset+1] = byte(len(txtData))
	offset += 2
	copy(buf[offset:], txtData)
	offset += len(txtData)

	// A record for our host
	localIPs := getLocalIPs()
	for _, ip := range localIPs {
		if ip4 := ip.To4(); ip4 != nil {
			offset += writeDNSName(buf[offset:], hostName)
			buf[offset] = 0x00
			buf[offset+1] = mdnsTypeA
			buf[offset+2] = 0x80 // Cache flush + IN class
			buf[offset+3] = 0x01
			offset += 4
			offset += writeTTL(buf[offset:], uint32(ttl))
			buf[offset] = 0x00
			buf[offset+1] = 0x04 // RDLENGTH = 4
			offset += 2
			copy(buf[offset:], ip4)
			offset += 4
			break // Only one A record
		}
	}

	return buf[:offset]
}

// Helper functions

// parseDNSName parses a DNS name from a packet.
func parseDNSName(data []byte, offset int) (string, int) {
	var parts []string
	visited := make(map[int]bool)

	for offset < len(data) {
		if visited[offset] {
			break // Prevent infinite loops
		}
		visited[offset] = true

		length := int(data[offset])
		if length == 0 {
			offset++
			break
		}

		// Check for compression pointer
		if length&0xC0 == 0xC0 {
			if offset+1 >= len(data) {
				break
			}
			pointer := (int(data[offset]&0x3F) << 8) | int(data[offset+1])
			name, _ := parseDNSName(data, pointer)
			parts = append(parts, name)
			offset += 2
			break
		}

		offset++
		if offset+length > len(data) {
			break
		}
		parts = append(parts, string(data[offset:offset+length]))
		offset += length
	}

	return strings.Join(parts, "."), offset
}

// writeDNSName writes a DNS name to a buffer.
func writeDNSName(buf []byte, name string) int {
	offset := 0
	parts := strings.Split(strings.TrimSuffix(name, "."), ".")
	for _, part := range parts {
		if len(part) > 0 {
			buf[offset] = byte(len(part))
			offset++
			copy(buf[offset:], part)
			offset += len(part)
		}
	}
	buf[offset] = 0 // Terminator
	return offset + 1
}

// writeDNSNameBytes returns a DNS name as bytes.
func writeDNSNameBytes(name string) []byte {
	buf := make([]byte, len(name)+2)
	n := writeDNSName(buf, name)
	return buf[:n]
}

// writeTTL writes a TTL value to a buffer.
func writeTTL(buf []byte, ttl uint32) int {
	buf[0] = byte(ttl >> 24)
	buf[1] = byte(ttl >> 16)
	buf[2] = byte(ttl >> 8)
	buf[3] = byte(ttl)
	return 4
}

// extractInstanceName extracts instance name from a full DNS name.
func extractInstanceName(name string) string {
	if idx := strings.Index(name, "._shuttle"); idx > 0 {
		return name[:idx]
	}
	if idx := strings.Index(name, ".local"); idx > 0 {
		return name[:idx]
	}
	return name
}

// getLocalIPs returns all local IP addresses.
func getLocalIPs() []net.IP {
	var ips []net.IP

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if !ipnet.IP.IsLoopback() && !ipnet.IP.IsLinkLocalUnicast() {
				ips = append(ips, ipnet.IP)
			}
		}
	}

	return ips
}

// LookupPeers discovers peers using mDNS with a timeout.
func LookupPeers(ctx context.Context, timeout time.Duration) ([]*MDNSPeer, error) {
	service := NewMDNSService("", nil)

	found := make(map[string]*MDNSPeer)
	var mu sync.Mutex

	service.OnPeerDiscovered(func(peer *MDNSPeer) {
		mu.Lock()
		found[peer.Name] = peer
		mu.Unlock()
	})

	// Start with a dummy VIP and port
	if err := service.Start(net.ParseIP("10.7.0.1"), 0); err != nil {
		return nil, err
	}
	defer func() { _ = service.Stop() }()

	// Send initial query
	_ = service.Query()

	// Wait for responses
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(timeout):
	}

	mu.Lock()
	defer mu.Unlock()

	result := make([]*MDNSPeer, 0, len(found))
	for _, peer := range found {
		result = append(result, peer)
	}
	return result, nil
}
