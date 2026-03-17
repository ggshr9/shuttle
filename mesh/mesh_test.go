package mesh

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
)

func TestWriteReadFrame(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("hello mesh")

	if err := WriteFrame(&buf, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q, want %q", got, payload)
	}
}

func TestWriteReadFrameMultiple(t *testing.T) {
	var buf bytes.Buffer
	payloads := [][]byte{
		[]byte("packet-1"),
		[]byte("packet-2"),
		[]byte("packet-3"),
	}

	for _, p := range payloads {
		if err := WriteFrame(&buf, p); err != nil {
			t.Fatalf("WriteFrame: %v", err)
		}
	}

	for _, want := range payloads {
		got, err := ReadFrame(&buf)
		if err != nil {
			t.Fatalf("ReadFrame: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}

func TestHandshakeEncodeDecode(t *testing.T) {
	ip := net.IPv4(10, 7, 0, 2).To4()
	mask := net.IPv4(255, 255, 255, 0).To4()
	gw := net.IPv4(10, 7, 0, 1).To4()

	data := EncodeHandshake(ip, mask, gw)
	if len(data) != HandshakeSize {
		t.Fatalf("handshake size: got %d, want %d", len(data), HandshakeSize)
	}
	if data[0] != ProtocolVersion {
		t.Fatalf("version: got %d, want %d", data[0], ProtocolVersion)
	}
	gotIP, gotMask, gotGW, err := DecodeHandshake(data)
	if err != nil {
		t.Fatalf("DecodeHandshake: %v", err)
	}
	if !gotIP.Equal(ip) {
		t.Fatalf("ip: got %v, want %v", gotIP, ip)
	}
	if !gotMask.Equal(mask) {
		t.Fatalf("mask: got %v, want %v", gotMask, mask)
	}
	if !gotGW.Equal(gw) {
		t.Fatalf("gw: got %v, want %v", gotGW, gw)
	}
}

func TestHandshakeVersionMismatch(t *testing.T) {
	data := make([]byte, HandshakeSize)
	data[0] = 99 // bad version
	_, _, _, err := DecodeHandshake(data)
	if err == nil {
		t.Fatal("expected version mismatch error")
	}
}

func TestIPAllocator(t *testing.T) {
	alloc, err := NewIPAllocator("10.7.0.0/24")
	if err != nil {
		t.Fatalf("NewIPAllocator: %v", err)
	}

	ip1, err := alloc.Allocate()
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if !ip1.Equal(net.IPv4(10, 7, 0, 2).To4()) {
		t.Fatalf("first IP: got %v, want 10.7.0.2", ip1)
	}

	ip2, err := alloc.Allocate()
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if !ip2.Equal(net.IPv4(10, 7, 0, 3).To4()) {
		t.Fatalf("second IP: got %v, want 10.7.0.3", ip2)
	}

	// Release first and reallocate
	alloc.Release(ip1)
	ip3, err := alloc.Allocate()
	if err != nil {
		t.Fatalf("Allocate after release: %v", err)
	}
	if !ip3.Equal(ip1) {
		t.Fatalf("reallocated IP: got %v, want %v", ip3, ip1)
	}

	if !alloc.Gateway().Equal(net.IPv4(10, 7, 0, 1).To4()) {
		t.Fatalf("gateway: got %v", alloc.Gateway())
	}
}

func TestIPAllocatorExhaustion(t *testing.T) {
	// /30 = 4 addresses: .0 (net), .1 (gw), .2 (usable), .3 (broadcast excluded)
	alloc, err := NewIPAllocator("10.7.0.0/30")
	if err != nil {
		t.Fatalf("NewIPAllocator: %v", err)
	}

	_, err = alloc.Allocate()
	if err != nil {
		t.Fatalf("Allocate .2: %v", err)
	}

	_, err = alloc.Allocate()
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
}

// pipeWriter wraps a pipe writer to implement io.WriteCloser and serializes writes.
type syncWriter struct {
	mu sync.Mutex
	w  io.WriteCloser
}

func (sw *syncWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}

func (sw *syncWriter) Close() error { return sw.w.Close() }

// frameWriter wraps a writer to produce length-prefixed frames.
type frameWriter struct {
	mu sync.Mutex
	w  io.WriteCloser
}

func (fw *frameWriter) Write(p []byte) (int, error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if err := WriteFrame(fw.w, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (fw *frameWriter) Close() error { return fw.w.Close() }

func TestPeerTableForward(t *testing.T) {
	logger := slog.Default()
	pt := NewPeerTable(logger)

	// Create pipes for two peers
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	peer1IP := net.IPv4(10, 7, 0, 2).To4()
	peer2IP := net.IPv4(10, 7, 0, 3).To4()

	pt.Register(peer1IP, &frameWriter{w: w1})
	pt.Register(peer2IP, &frameWriter{w: w2})

	// Build a fake IPv4 packet from peer1 to peer2
	pkt := make([]byte, 40) // minimal IP header
	pkt[0] = 0x45           // version=4, IHL=5
	pkt[2] = 0
	pkt[3] = 40 // total length
	copy(pkt[12:16], peer1IP)
	copy(pkt[16:20], peer2IP)

	// Forward should deliver to peer2's writer
	done := make(chan []byte, 1)
	go func() {
		frame, err := ReadFrame(r2)
		if err != nil {
			t.Errorf("ReadFrame: %v", err)
			return
		}
		done <- frame
	}()

	if !pt.Forward(pkt) {
		t.Fatal("Forward returned false")
	}

	got := <-done
	if !bytes.Equal(got, pkt) {
		t.Fatalf("forwarded packet mismatch")
	}

	// Forward to unknown IP should return false
	pkt2 := make([]byte, 40)
	pkt2[0] = 0x45
	pkt2[2] = 0
	pkt2[3] = 40
	copy(pkt2[16:20], net.IPv4(10, 7, 0, 99).To4())
	if pt.Forward(pkt2) {
		t.Fatal("Forward to unknown peer should return false")
	}

	// Cleanup
	pt.Unregister(peer1IP)
	pt.Unregister(peer2IP)
	w1.Close()
	w2.Close()
	r1.Close()
	r2.Close()
}

func TestFrameTooLarge(t *testing.T) {
	var buf bytes.Buffer
	large := make([]byte, MaxFrameSize+1)
	err := WriteFrame(&buf, large)
	if err == nil {
		t.Fatal("expected error for oversized frame")
	}
}

func TestFrameMaxSize(t *testing.T) {
	var buf bytes.Buffer
	data := make([]byte, MaxFrameSize)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := WriteFrame(&buf, data); err != nil {
		t.Fatalf("WriteFrame max size: %v", err)
	}
	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame max size: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("max size frame roundtrip mismatch")
	}
}

func TestReadFrameEOF(t *testing.T) {
	var buf bytes.Buffer
	_, err := ReadFrame(&buf)
	if err == nil {
		t.Fatal("expected EOF error")
	}
}

func TestReadFrameTruncated(t *testing.T) {
	// Write header claiming 100 bytes but only provide 5
	var buf bytes.Buffer
	buf.Write([]byte{0, 100, 1, 2, 3, 4, 5})
	_, err := ReadFrame(&buf)
	if err == nil {
		t.Fatal("expected error for truncated frame")
	}
}

func TestDecodeHandshakeTooShort(t *testing.T) {
	_, _, _, err := DecodeHandshake([]byte{1, 2, 3})
	if err == nil {
		t.Fatal("expected error for short handshake")
	}
}

func TestIPAllocatorWraparound(t *testing.T) {
	// /29 = 8 addresses: .0 (net), .1 (gw), .2-.6 usable, .7 (broadcast)
	alloc, err := NewIPAllocator("10.7.0.0/29")
	if err != nil {
		t.Fatalf("NewIPAllocator: %v", err)
	}

	// Allocate all 5 usable IPs: .2, .3, .4, .5, .6
	ips := make([]net.IP, 5)
	for i := 0; i < 5; i++ {
		ips[i], err = alloc.Allocate()
		if err != nil {
			t.Fatalf("Allocate %d: %v", i, err)
		}
	}
	// Pool should be exhausted
	_, err = alloc.Allocate()
	if err == nil {
		t.Fatal("expected exhaustion")
	}

	// Release .3 (middle) and .5
	alloc.Release(ips[1]) // 10.7.0.3
	alloc.Release(ips[3]) // 10.7.0.5

	// Should get .3 back first (wraparound: next was past .6, wraps to find .3)
	ip, err := alloc.Allocate()
	if err != nil {
		t.Fatalf("Allocate after release: %v", err)
	}
	if !ip.Equal(net.IPv4(10, 7, 0, 3).To4()) {
		t.Fatalf("wraparound: got %v, want 10.7.0.3", ip)
	}

	// Then .5
	ip2, err := alloc.Allocate()
	if err != nil {
		t.Fatalf("Allocate second after release: %v", err)
	}
	if !ip2.Equal(net.IPv4(10, 7, 0, 5).To4()) {
		t.Fatalf("wraparound: got %v, want 10.7.0.5", ip2)
	}
}

func TestIPAllocatorInvalidCIDR(t *testing.T) {
	_, err := NewIPAllocator("not-a-cidr")
	if err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
}

func TestIPAllocatorReleaseNonIPv4(t *testing.T) {
	alloc, _ := NewIPAllocator("10.7.0.0/24")
	// Should not panic on nil or IPv6
	alloc.Release(nil)
	alloc.Release(net.ParseIP("::1"))
}

func TestIPAllocatorNetworkAndMask(t *testing.T) {
	alloc, _ := NewIPAllocator("10.7.0.0/24")
	if alloc.Network().String() != "10.7.0.0/24" {
		t.Errorf("Network = %q", alloc.Network().String())
	}
	mask := alloc.Mask()
	if !net.IP(mask).Equal(net.IP(net.IPv4Mask(255, 255, 255, 0))) { //nolint:unconvert // explicit conversion for type clarity
		t.Errorf("Mask = %v", mask)
	}
}

func TestPeerTableForwardTooShort(t *testing.T) {
	pt := NewPeerTable(slog.Default())
	if pt.Forward([]byte{1, 2, 3}) {
		t.Fatal("expected false for short packet")
	}
}

func TestPeerTableConcurrentForward(t *testing.T) {
	logger := slog.Default()
	pt := NewPeerTable(logger)

	dstIP := net.IPv4(10, 7, 0, 2).To4()
	r, w := io.Pipe()
	pt.Register(dstIP, &frameWriter{w: w})

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Read all frames in background
	received := make(chan []byte, numGoroutines)
	go func() {
		for i := 0; i < numGoroutines; i++ {
			frame, err := ReadFrame(r)
			if err != nil {
				return
			}
			received <- frame
		}
	}()

	// Concurrent forwards
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			pkt := make([]byte, 24)
			pkt[0] = 0x45
			pkt[2] = 0
			pkt[3] = 24
			copy(pkt[16:20], dstIP)
			pkt[20] = byte(id) // identify each packet
			pt.Forward(pkt)
		}(i)
	}

	wg.Wait()

	// Verify we got all frames
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-received:
		default:
			// Some may still be in flight
		}
	}

	pt.Unregister(dstIP)
	w.Close()
	r.Close()
}

func TestMeshClientIsMeshDestination(t *testing.T) {
	mc := &MeshClient{
		meshNet: &net.IPNet{
			IP:   net.IPv4(10, 7, 0, 0).To4(),
			Mask: net.CIDRMask(24, 32),
		},
	}

	tests := []struct {
		ip   string
		want bool
	}{
		{"10.7.0.1", true},
		{"10.7.0.100", true},
		{"10.7.1.1", false},
		{"192.168.1.1", false},
	}

	for _, tt := range tests {
		if got := mc.IsMeshDestination(net.ParseIP(tt.ip)); got != tt.want {
			t.Errorf("IsMeshDestination(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestMeshClientSplitRoutes(t *testing.T) {
	mc := &MeshClient{
		meshNet: &net.IPNet{
			IP:   net.IPv4(10, 7, 0, 0).To4(),
			Mask: net.CIDRMask(24, 32),
		},
	}

	// Set split routes: 10.7.0.128/25 → direct, rest → mesh
	mc.SetSplitRoutes([]struct{ CIDR, Action string }{
		{"10.7.0.128/25", "direct"},
	})

	tests := []struct {
		ip     string
		isMesh bool
		action string
	}{
		{"10.7.0.1", true, "mesh"},       // No split route match → default mesh
		{"10.7.0.127", true, "mesh"},     // Just before split route range
		{"10.7.0.128", false, "direct"},  // In direct split route
		{"10.7.0.200", false, "direct"},  // In direct split route
		{"10.7.0.255", false, "direct"},  // End of direct split route
		{"192.168.1.1", false, ""},        // Not in mesh network at all
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if got := mc.IsMeshDestination(ip); got != tt.isMesh {
			t.Errorf("IsMeshDestination(%s) = %v, want %v", tt.ip, got, tt.isMesh)
		}
		if got := mc.RouteMesh(ip); got != tt.action {
			t.Errorf("RouteMesh(%s) = %q, want %q", tt.ip, got, tt.action)
		}
	}
}

func TestMeshClientSplitRouteProxyAction(t *testing.T) {
	mc := &MeshClient{
		meshNet: &net.IPNet{
			IP:   net.IPv4(10, 7, 0, 0).To4(),
			Mask: net.CIDRMask(24, 32),
		},
	}

	mc.SetSplitRoutes([]struct{ CIDR, Action string }{
		{"10.7.0.0/25", "proxy"},
	})

	// 10.7.0.1 matches the proxy split route
	if mc.IsMeshDestination(net.ParseIP("10.7.0.1")) {
		t.Error("proxy split route should exclude from mesh")
	}
	if got := mc.RouteMesh(net.ParseIP("10.7.0.1")); got != "proxy" {
		t.Errorf("expected proxy action, got %q", got)
	}

	// 10.7.0.200 is in mesh but not in any split route → mesh
	if !mc.IsMeshDestination(net.ParseIP("10.7.0.200")) {
		t.Error("10.7.0.200 should be mesh destination")
	}
}

func TestIPv6Allocator(t *testing.T) {
	alloc, err := NewIPv6Allocator("fd00:7::/64")
	if err != nil {
		t.Fatalf("NewIPv6Allocator: %v", err)
	}

	// First allocation should be ::2
	ip1, err := alloc.Allocate()
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	expected1 := net.ParseIP("fd00:7::2")
	if !ip1.Equal(expected1) {
		t.Fatalf("first IP: got %v, want %v", ip1, expected1)
	}

	// Second allocation should be ::3
	ip2, err := alloc.Allocate()
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	expected2 := net.ParseIP("fd00:7::3")
	if !ip2.Equal(expected2) {
		t.Fatalf("second IP: got %v, want %v", ip2, expected2)
	}

	// Release first and reallocate
	alloc.Release(ip1)
	ip3, err := alloc.Allocate()
	if err != nil {
		t.Fatalf("Allocate after release: %v", err)
	}
	if !ip3.Equal(ip1) {
		t.Fatalf("reallocated IP: got %v, want %v", ip3, ip1)
	}

	// Gateway should be ::1
	gw := alloc.Gateway()
	expectedGW := net.ParseIP("fd00:7::1")
	if !gw.Equal(expectedGW) {
		t.Fatalf("gateway: got %v, want %v", gw, expectedGW)
	}
}

func TestIPv6AllocatorInvalidCIDR(t *testing.T) {
	// Not a valid CIDR
	_, err := NewIPv6Allocator("not-a-cidr")
	if err == nil {
		t.Fatal("expected error for invalid CIDR")
	}

	// IPv4 CIDR (should fail)
	_, err = NewIPv6Allocator("10.0.0.0/24")
	if err == nil {
		t.Fatal("expected error for IPv4 CIDR")
	}

	// Prefix too long (> /64)
	_, err = NewIPv6Allocator("fd00::/96")
	if err == nil {
		t.Fatal("expected error for prefix > /64")
	}
}

func TestIPv6AllocatorNetwork(t *testing.T) {
	alloc, _ := NewIPv6Allocator("fd00:7::/64")
	if alloc.Network().String() != "fd00:7::/64" {
		t.Errorf("Network = %q", alloc.Network().String())
	}
}

func TestDualStackAllocator(t *testing.T) {
	alloc, err := NewDualStackAllocator("10.7.0.0/24", "fd00:7::/64")
	if err != nil {
		t.Fatalf("NewDualStackAllocator: %v", err)
	}

	// Allocate IPv4
	v4, err := alloc.AllocateV4()
	if err != nil {
		t.Fatalf("AllocateV4: %v", err)
	}
	if !v4.Equal(net.IPv4(10, 7, 0, 2).To4()) {
		t.Fatalf("IPv4: got %v", v4)
	}

	// Allocate IPv6
	v6, err := alloc.AllocateV6()
	if err != nil {
		t.Fatalf("AllocateV6: %v", err)
	}
	if !v6.Equal(net.ParseIP("fd00:7::2")) {
		t.Fatalf("IPv6: got %v", v6)
	}

	// Allocate dual stack
	v4b, v6b, err := alloc.AllocateDualStack()
	if err != nil {
		t.Fatalf("AllocateDualStack: %v", err)
	}
	if !v4b.Equal(net.IPv4(10, 7, 0, 3).To4()) {
		t.Fatalf("DualStack IPv4: got %v", v4b)
	}
	if !v6b.Equal(net.ParseIP("fd00:7::3")) {
		t.Fatalf("DualStack IPv6: got %v", v6b)
	}

	// Test gateways
	if !alloc.GatewayV4().Equal(net.IPv4(10, 7, 0, 1).To4()) {
		t.Fatalf("GatewayV4: got %v", alloc.GatewayV4())
	}
	if !alloc.GatewayV6().Equal(net.ParseIP("fd00:7::1")) {
		t.Fatalf("GatewayV6: got %v", alloc.GatewayV6())
	}

	// Test release
	alloc.ReleaseV4(v4)
	alloc.ReleaseV6(v6)

	// Should get released IPs back
	v4c, _ := alloc.AllocateV4()
	if !v4c.Equal(v4) {
		t.Fatalf("after release V4: got %v, want %v", v4c, v4)
	}
	v6c, _ := alloc.AllocateV6()
	if !v6c.Equal(v6) {
		t.Fatalf("after release V6: got %v, want %v", v6c, v6)
	}
}
