package p2p

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
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
// This method queries servers sequentially. Use QueryParallel for concurrent queries.
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

// QueryParallel sends STUN Binding Requests to all servers in parallel.
// Returns the first successful response, cancelling remaining queries.
// This provides faster results and better reliability through redundancy.
func (c *STUNClient) QueryParallel(ctx context.Context) (*STUNResult, error) {
	if len(c.servers) == 0 {
		return nil, ErrSTUNNoResponse
	}

	// Create a context with timeout if not already set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	resultCh := make(chan *STUNResult, len(c.servers))
	errCh := make(chan error, len(c.servers))

	// Query all servers in parallel
	var wg sync.WaitGroup
	for _, server := range c.servers {
		wg.Add(1)
		go func(srv string) {
			defer wg.Done()

			// Each goroutine gets its own connection to avoid interference
			conn, err := net.ListenUDP("udp4", nil)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			defer conn.Close()

			result, err := c.queryServerWithContext(ctx, conn, srv)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}

			select {
			case resultCh <- result:
			default:
			}
		}(server)
	}

	// Close channels when all goroutines complete
	go func() {
		wg.Wait()
		close(resultCh)
		close(errCh)
	}()

	// Wait for first successful result or all failures
	select {
	case result := <-resultCh:
		if result != nil {
			return result, nil
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return nil, ErrSTUNNoResponse
}

// QueryAllParallel queries all servers in parallel and returns all successful results.
// This is useful for NAT detection where we need responses from multiple servers.
func (c *STUNClient) QueryAllParallel(ctx context.Context) ([]*STUNResult, error) {
	if len(c.servers) == 0 {
		return nil, ErrSTUNNoResponse
	}

	// Create a context with timeout if not already set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	results := make([]*STUNResult, 0, len(c.servers))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, server := range c.servers {
		wg.Add(1)
		go func(srv string) {
			defer wg.Done()

			// Each goroutine gets its own connection
			conn, err := net.ListenUDP("udp4", nil)
			if err != nil {
				return
			}
			defer conn.Close()

			result, err := c.queryServerWithContext(ctx, conn, srv)
			if err != nil {
				return
			}

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(server)
	}

	wg.Wait()

	if len(results) == 0 {
		return nil, ErrSTUNNoResponse
	}

	return results, nil
}

// QueryParallelWithConn sends STUN requests to all servers using the same connection.
// This is useful for NAT type detection where we need consistent local port.
func (c *STUNClient) QueryParallelWithConn(ctx context.Context, conn *net.UDPConn) ([]*STUNResult, error) {
	if len(c.servers) == 0 {
		return nil, ErrSTUNNoResponse
	}

	if conn == nil {
		var err error
		conn, err = net.ListenUDP("udp4", nil)
		if err != nil {
			return nil, fmt.Errorf("stun: listen: %w", err)
		}
		defer conn.Close()
	}

	// Create a context with timeout if not already set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	// For shared connection, we need to coordinate reads
	// Send all requests first, then collect responses
	type pendingQuery struct {
		txID   []byte
		server string
		addr   *net.UDPAddr
	}

	pending := make([]pendingQuery, 0, len(c.servers))

	// Send all requests
	for _, server := range c.servers {
		addr, err := net.ResolveUDPAddr("udp4", server)
		if err != nil {
			continue
		}

		txID := make([]byte, 12)
		if _, err := rand.Read(txID); err != nil {
			continue
		}

		req := buildBindingRequest(txID)
		if _, err := conn.WriteToUDP(req, addr); err != nil {
			continue
		}

		pending = append(pending, pendingQuery{
			txID:   txID,
			server: server,
			addr:   addr,
		})
	}

	if len(pending) == 0 {
		return nil, ErrSTUNNoResponse
	}

	// Collect responses
	results := make([]*STUNResult, 0, len(pending))
	buf := make([]byte, 1024)
	localAddr := conn.LocalAddr().(*net.UDPAddr)

	deadline, _ := ctx.Deadline()
	_ = conn.SetReadDeadline(deadline)
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	responded := make(map[string]bool)

	for len(responded) < len(pending) {
		select {
		case <-ctx.Done():
			if len(results) > 0 {
				return results, nil
			}
			return nil, ctx.Err()
		default:
		}

		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}

		// Find matching pending query
		for _, p := range pending {
			if responded[p.server] {
				continue
			}

			if from.IP.Equal(p.addr.IP) && from.Port == p.addr.Port {
				publicAddr, err := parseBindingResponse(buf[:n], p.txID)
				if err == nil {
					results = append(results, &STUNResult{
						PublicAddr: publicAddr,
						LocalAddr:  localAddr,
						Server:     p.server,
					})
					responded[p.server] = true
				}
				break
			}
		}
	}

	if len(results) == 0 {
		return nil, ErrSTUNNoResponse
	}

	return results, nil
}

// queryServerWithContext queries a single server with context support.
func (c *STUNClient) queryServerWithContext(ctx context.Context, conn *net.UDPConn, server string) (*STUNResult, error) {
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

	// Set deadline from context or use default timeout
	deadline := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)
	defer conn.SetDeadline(time.Time{})

	// Send request
	if _, err := conn.WriteToUDP(req, addr); err != nil {
		return nil, fmt.Errorf("stun: send: %w", err)
	}

	// Read response with context check
	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return nil, fmt.Errorf("stun: recv: %w", err)
		}

		// Parse response
		publicAddr, err := parseBindingResponse(buf[:n], txID)
		if err != nil {
			// Maybe a response for a different transaction, keep reading
			continue
		}

		localAddr := conn.LocalAddr().(*net.UDPAddr)

		return &STUNResult{
			PublicAddr: publicAddr,
			LocalAddr:  localAddr,
			Server:     server,
		}, nil
	}
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
// When SHUTTLE_TEST_NO_EXTERNAL is set, returns an empty list to prevent
// external network access during tests.
func DefaultSTUNServers() []string {
	if os.Getenv("SHUTTLE_TEST_NO_EXTERNAL") != "" {
		return []string{}
	}
	return []string{
		"stun.l.google.com:19302",
		"stun1.l.google.com:19302",
		"stun.cloudflare.com:3478",
		"stun.stunprotocol.org:3478",
	}
}

// DefaultSTUNServersIPv6 returns a list of public STUN servers that support IPv6.
// When SHUTTLE_TEST_NO_EXTERNAL is set, returns an empty list to prevent
// external network access during tests.
func DefaultSTUNServersIPv6() []string {
	if os.Getenv("SHUTTLE_TEST_NO_EXTERNAL") != "" {
		return []string{}
	}
	return []string{
		"stun.l.google.com:19302",
		"stun1.l.google.com:19302",
		"stun.cloudflare.com:3478",
	}
}

// QueryIPv6 sends STUN Binding Request to discover IPv6 public endpoint.
func (c *STUNClient) QueryIPv6(conn *net.UDPConn) (*STUNResult, error) {
	if conn == nil {
		var err error
		conn, err = net.ListenUDP("udp6", nil)
		if err != nil {
			return nil, fmt.Errorf("stun: listen ipv6: %w", err)
		}
		defer conn.Close()
	}

	// Try each server until one responds
	for _, server := range c.servers {
		result, err := c.queryServerIPv6(conn, server)
		if err == nil {
			return result, nil
		}
	}

	return nil, ErrSTUNNoResponse
}

// queryServerIPv6 sends a STUN request to a single server over IPv6.
func (c *STUNClient) queryServerIPv6(conn *net.UDPConn, server string) (*STUNResult, error) {
	addr, err := net.ResolveUDPAddr("udp6", server)
	if err != nil {
		return nil, fmt.Errorf("stun: resolve ipv6 %s: %w", server, err)
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
		return nil, fmt.Errorf("stun: send ipv6: %w", err)
	}

	// Read response
	buf := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, fmt.Errorf("stun: recv ipv6: %w", err)
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

// QueryParallelIPv6 sends STUN Binding Requests to all servers in parallel over IPv6.
func (c *STUNClient) QueryParallelIPv6(ctx context.Context) (*STUNResult, error) {
	if len(c.servers) == 0 {
		return nil, ErrSTUNNoResponse
	}

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	resultCh := make(chan *STUNResult, len(c.servers))
	errCh := make(chan error, len(c.servers))

	var wg sync.WaitGroup
	for _, server := range c.servers {
		wg.Add(1)
		go func(srv string) {
			defer wg.Done()

			conn, err := net.ListenUDP("udp6", nil)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			defer conn.Close()

			result, err := c.queryServerIPv6WithContext(ctx, conn, srv)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}

			select {
			case resultCh <- result:
			default:
			}
		}(server)
	}

	go func() {
		wg.Wait()
		close(resultCh)
		close(errCh)
	}()

	select {
	case result := <-resultCh:
		if result != nil {
			return result, nil
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return nil, ErrSTUNNoResponse
}

// queryServerIPv6WithContext queries a single server over IPv6 with context support.
func (c *STUNClient) queryServerIPv6WithContext(ctx context.Context, conn *net.UDPConn, server string) (*STUNResult, error) {
	addr, err := net.ResolveUDPAddr("udp6", server)
	if err != nil {
		return nil, fmt.Errorf("stun: resolve ipv6 %s: %w", server, err)
	}

	txID := make([]byte, 12)
	if _, err := rand.Read(txID); err != nil {
		return nil, fmt.Errorf("stun: generate txid: %w", err)
	}

	req := buildBindingRequest(txID)

	deadline := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)
	defer conn.SetDeadline(time.Time{})

	if _, err := conn.WriteToUDP(req, addr); err != nil {
		return nil, fmt.Errorf("stun: send ipv6: %w", err)
	}

	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return nil, fmt.Errorf("stun: recv ipv6: %w", err)
		}

		publicAddr, err := parseBindingResponse(buf[:n], txID)
		if err != nil {
			continue
		}

		localAddr := conn.LocalAddr().(*net.UDPAddr)

		return &STUNResult{
			PublicAddr: publicAddr,
			LocalAddr:  localAddr,
			Server:     server,
		}, nil
	}
}

// QueryDualStack queries both IPv4 and IPv6 in parallel.
// Returns results for both address families if available.
type DualStackSTUNResult struct {
	IPv4 *STUNResult
	IPv6 *STUNResult
}

// QueryDualStack queries STUN servers for both IPv4 and IPv6 addresses in parallel.
func (c *STUNClient) QueryDualStack(ctx context.Context) (*DualStackSTUNResult, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	result := &DualStackSTUNResult{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Query IPv4
	wg.Add(1)
	go func() {
		defer wg.Done()
		r, err := c.QueryParallel(ctx)
		if err == nil {
			mu.Lock()
			result.IPv4 = r
			mu.Unlock()
		}
	}()

	// Query IPv6
	wg.Add(1)
	go func() {
		defer wg.Done()
		r, err := c.QueryParallelIPv6(ctx)
		if err == nil {
			mu.Lock()
			result.IPv6 = r
			mu.Unlock()
		}
	}()

	wg.Wait()

	if result.IPv4 == nil && result.IPv6 == nil {
		return nil, ErrSTUNNoResponse
	}

	return result, nil
}
