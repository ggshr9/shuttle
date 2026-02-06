package p2p

import (
	"context"
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"
)

func TestBuildBindingRequest(t *testing.T) {
	txID := make([]byte, 12)
	for i := range txID {
		txID[i] = byte(i)
	}

	req := buildBindingRequest(txID)

	// Check length
	if len(req) != stunHeaderSize {
		t.Errorf("expected length %d, got %d", stunHeaderSize, len(req))
	}

	// Check message type
	msgType := binary.BigEndian.Uint16(req[0:2])
	if msgType != stunBindingRequest {
		t.Errorf("expected message type 0x%04x, got 0x%04x", stunBindingRequest, msgType)
	}

	// Check message length (should be 0 for simple request)
	msgLen := binary.BigEndian.Uint16(req[2:4])
	if msgLen != 0 {
		t.Errorf("expected message length 0, got %d", msgLen)
	}

	// Check magic cookie
	magic := binary.BigEndian.Uint32(req[4:8])
	if magic != stunMagicCookie {
		t.Errorf("expected magic cookie 0x%08x, got 0x%08x", stunMagicCookie, magic)
	}

	// Check transaction ID
	for i := 0; i < 12; i++ {
		if req[8+i] != txID[i] {
			t.Errorf("transaction ID mismatch at byte %d", i)
		}
	}
}

func TestParseBindingResponse(t *testing.T) {
	// Build a mock response with XOR-MAPPED-ADDRESS
	txID := make([]byte, 12)
	for i := range txID {
		txID[i] = byte(i)
	}

	// Build response header
	resp := make([]byte, stunHeaderSize+12) // header + XOR-MAPPED-ADDRESS attribute
	binary.BigEndian.PutUint16(resp[0:2], stunBindingResponse)
	binary.BigEndian.PutUint16(resp[2:4], 12) // attribute length
	binary.BigEndian.PutUint32(resp[4:8], stunMagicCookie)
	copy(resp[8:20], txID)

	// XOR-MAPPED-ADDRESS attribute
	// Type: 0x0020, Length: 8
	binary.BigEndian.PutUint16(resp[20:22], stunAttrXorMappedAddress)
	binary.BigEndian.PutUint16(resp[22:24], 8)
	resp[24] = 0 // reserved
	resp[25] = 1 // IPv4 family

	// Port XORed with magic cookie high bytes
	// Want port 12345, XOR with 0x2112 = 0x3027
	port := uint16(12345) ^ uint16(stunMagicCookie>>16)
	binary.BigEndian.PutUint16(resp[26:28], port)

	// IP XORed with magic cookie
	// Want 1.2.3.4
	ip := []byte{1, 2, 3, 4}
	magicBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(magicBytes, stunMagicCookie)
	for i := 0; i < 4; i++ {
		resp[28+i] = ip[i] ^ magicBytes[i]
	}

	addr, err := parseBindingResponse(resp, txID)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !addr.IP.Equal(net.IPv4(1, 2, 3, 4)) {
		t.Errorf("expected IP 1.2.3.4, got %s", addr.IP)
	}
	if addr.Port != 12345 {
		t.Errorf("expected port 12345, got %d", addr.Port)
	}
}

func TestParseBindingResponseInvalidPacket(t *testing.T) {
	// Too short
	_, err := parseBindingResponse([]byte{0, 1, 2}, nil)
	if err == nil {
		t.Error("expected error for short packet")
	}

	// Wrong message type
	resp := make([]byte, stunHeaderSize)
	binary.BigEndian.PutUint16(resp[0:2], 0x9999)
	binary.BigEndian.PutUint32(resp[4:8], stunMagicCookie)

	_, err = parseBindingResponse(resp, nil)
	if err == nil {
		t.Error("expected error for wrong message type")
	}

	// Wrong magic cookie
	resp2 := make([]byte, stunHeaderSize)
	binary.BigEndian.PutUint16(resp2[0:2], stunBindingResponse)
	binary.BigEndian.PutUint32(resp2[4:8], 0x12345678)

	_, err = parseBindingResponse(resp2, nil)
	if err == nil {
		t.Error("expected error for wrong magic cookie")
	}
}

func TestParseMappedAddress(t *testing.T) {
	// Test IPv4 without XOR
	data := make([]byte, 8)
	data[0] = 0    // reserved
	data[1] = 0x01 // IPv4
	binary.BigEndian.PutUint16(data[2:4], 8080)
	copy(data[4:8], net.IPv4(192, 168, 1, 1).To4())

	addr := parseMappedAddress(data, nil)
	if addr == nil {
		t.Fatal("expected non-nil address")
	}
	if !addr.IP.Equal(net.IPv4(192, 168, 1, 1)) {
		t.Errorf("expected 192.168.1.1, got %s", addr.IP)
	}
	if addr.Port != 8080 {
		t.Errorf("expected port 8080, got %d", addr.Port)
	}
}

func TestDefaultSTUNServers(t *testing.T) {
	servers := DefaultSTUNServers()
	if len(servers) == 0 {
		t.Error("expected non-empty default STUN servers")
	}

	// Check that all servers have valid host:port format
	for _, s := range servers {
		_, port, err := net.SplitHostPort(s)
		if err != nil {
			t.Errorf("invalid STUN server address %q: %v", s, err)
		}
		if port == "" {
			t.Errorf("STUN server %q missing port", s)
		}
	}
}

func TestNewSTUNClient(t *testing.T) {
	servers := []string{"stun.example.com:3478"}
	client := NewSTUNClient(servers, 5*time.Second)

	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if len(client.servers) != 1 {
		t.Errorf("expected 1 server, got %d", len(client.servers))
	}
	if client.timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", client.timeout)
	}
}

func TestNewSTUNClient_DefaultTimeout(t *testing.T) {
	client := NewSTUNClient(nil, 0)

	if client.timeout != 3*time.Second {
		t.Errorf("expected default timeout 3s, got %v", client.timeout)
	}
}

func TestSTUNClient_QueryParallel_NoServers(t *testing.T) {
	client := NewSTUNClient(nil, time.Second)
	ctx := context.Background()

	_, err := client.QueryParallel(ctx)
	if err != ErrSTUNNoResponse {
		t.Errorf("expected ErrSTUNNoResponse, got %v", err)
	}
}

func TestSTUNClient_QueryAllParallel_NoServers(t *testing.T) {
	client := NewSTUNClient(nil, time.Second)
	ctx := context.Background()

	_, err := client.QueryAllParallel(ctx)
	if err != ErrSTUNNoResponse {
		t.Errorf("expected ErrSTUNNoResponse, got %v", err)
	}
}

func TestSTUNClient_QueryParallelWithConn_NoServers(t *testing.T) {
	client := NewSTUNClient(nil, time.Second)
	ctx := context.Background()

	_, err := client.QueryParallelWithConn(ctx, nil)
	if err != ErrSTUNNoResponse {
		t.Errorf("expected ErrSTUNNoResponse, got %v", err)
	}
}

func TestSTUNClient_QueryParallel_ContextCancelled(t *testing.T) {
	// Use invalid servers that won't respond
	servers := []string{"127.0.0.1:1", "127.0.0.1:2"}
	client := NewSTUNClient(servers, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.QueryParallel(ctx)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// mockSTUNServer creates a mock STUN server for testing.
func mockSTUNServer(t *testing.T, responseDelay time.Duration) (string, func()) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		buf := make([]byte, 1024)
		for {
			select {
			case <-done:
				return
			default:
			}

			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}

			// Verify it's a binding request
			if n < stunHeaderSize {
				continue
			}
			msgType := binary.BigEndian.Uint16(buf[0:2])
			if msgType != stunBindingRequest {
				continue
			}

			// Simulate delay
			if responseDelay > 0 {
				time.Sleep(responseDelay)
			}

			// Build response with XOR-MAPPED-ADDRESS
			txID := buf[8:20]
			resp := buildMockSTUNResponse(txID, addr.IP, addr.Port)

			conn.WriteToUDP(resp, addr)
		}
	}()

	cleanup := func() {
		close(done)
		conn.Close()
	}

	return conn.LocalAddr().String(), cleanup
}

// buildMockSTUNResponse builds a STUN Binding Response with XOR-MAPPED-ADDRESS.
func buildMockSTUNResponse(txID []byte, ip net.IP, port int) []byte {
	resp := make([]byte, stunHeaderSize+12)

	// Header
	binary.BigEndian.PutUint16(resp[0:2], stunBindingResponse)
	binary.BigEndian.PutUint16(resp[2:4], 12) // attribute length
	binary.BigEndian.PutUint32(resp[4:8], stunMagicCookie)
	copy(resp[8:20], txID)

	// XOR-MAPPED-ADDRESS attribute
	binary.BigEndian.PutUint16(resp[20:22], stunAttrXorMappedAddress)
	binary.BigEndian.PutUint16(resp[22:24], 8)
	resp[24] = 0    // reserved
	resp[25] = 0x01 // IPv4

	// XOR port
	xorPort := uint16(port) ^ uint16(stunMagicCookie>>16)
	binary.BigEndian.PutUint16(resp[26:28], xorPort)

	// XOR IP
	ip4 := ip.To4()
	if ip4 == nil {
		ip4 = net.IPv4(127, 0, 0, 1).To4()
	}
	magicBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(magicBytes, stunMagicCookie)
	for i := 0; i < 4; i++ {
		resp[28+i] = ip4[i] ^ magicBytes[i]
	}

	return resp
}

func TestSTUNClient_QueryParallel_Success(t *testing.T) {
	// Create two mock servers with different delays
	server1, cleanup1 := mockSTUNServer(t, 50*time.Millisecond)
	defer cleanup1()
	server2, cleanup2 := mockSTUNServer(t, 100*time.Millisecond)
	defer cleanup2()

	client := NewSTUNClient([]string{server1, server2}, 5*time.Second)
	ctx := context.Background()

	result, err := client.QueryParallel(ctx)
	if err != nil {
		t.Fatalf("QueryParallel failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.PublicAddr == nil {
		t.Error("expected non-nil public address")
	}
	if result.LocalAddr == nil {
		t.Error("expected non-nil local address")
	}
	if result.Server == "" {
		t.Error("expected non-empty server")
	}

	t.Logf("Got response from %s: public=%v local=%v", result.Server, result.PublicAddr, result.LocalAddr)
}

func TestSTUNClient_QueryAllParallel_Success(t *testing.T) {
	// Create multiple mock servers
	server1, cleanup1 := mockSTUNServer(t, 10*time.Millisecond)
	defer cleanup1()
	server2, cleanup2 := mockSTUNServer(t, 20*time.Millisecond)
	defer cleanup2()
	server3, cleanup3 := mockSTUNServer(t, 30*time.Millisecond)
	defer cleanup3()

	client := NewSTUNClient([]string{server1, server2, server3}, 5*time.Second)
	ctx := context.Background()

	results, err := client.QueryAllParallel(ctx)
	if err != nil {
		t.Fatalf("QueryAllParallel failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	for i, result := range results {
		t.Logf("Result %d: server=%s public=%v", i, result.Server, result.PublicAddr)
	}
}

func TestSTUNClient_QueryParallelWithConn_Success(t *testing.T) {
	// Create mock servers
	server1, cleanup1 := mockSTUNServer(t, 10*time.Millisecond)
	defer cleanup1()
	server2, cleanup2 := mockSTUNServer(t, 20*time.Millisecond)
	defer cleanup2()

	client := NewSTUNClient([]string{server1, server2}, 5*time.Second)
	ctx := context.Background()

	// Create a shared connection
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer conn.Close()

	results, err := client.QueryParallelWithConn(ctx, conn)
	if err != nil {
		t.Fatalf("QueryParallelWithConn failed: %v", err)
	}

	if len(results) < 1 {
		t.Error("expected at least 1 result")
	}

	// All results should have the same local address
	localAddr := results[0].LocalAddr.String()
	for _, r := range results {
		if r.LocalAddr.String() != localAddr {
			t.Errorf("expected same local address, got %s vs %s", r.LocalAddr, localAddr)
		}
	}
}

func TestSTUNClient_QueryParallel_FastestWins(t *testing.T) {
	// Server1 is slow, Server2 is fast
	server1, cleanup1 := mockSTUNServer(t, 200*time.Millisecond)
	defer cleanup1()
	server2, cleanup2 := mockSTUNServer(t, 10*time.Millisecond)
	defer cleanup2()

	// Put slow server first
	client := NewSTUNClient([]string{server1, server2}, 5*time.Second)
	ctx := context.Background()

	start := time.Now()
	result, err := client.QueryParallel(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("QueryParallel failed: %v", err)
	}

	// Should get result quickly (from fast server), not wait for slow server
	if elapsed > 100*time.Millisecond {
		t.Errorf("expected fast response, took %v", elapsed)
	}

	// Result should be from the fast server
	if result.Server != server2 {
		t.Logf("Got response from %s (expected fast server %s)", result.Server, server2)
	}
}

func TestSTUNClient_QueryParallel_Concurrent(t *testing.T) {
	server, cleanup := mockSTUNServer(t, 10*time.Millisecond)
	defer cleanup()

	client := NewSTUNClient([]string{server}, 5*time.Second)

	// Run multiple parallel queries concurrently
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			_, err := client.QueryParallel(ctx)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent query failed: %v", err)
	}
}
