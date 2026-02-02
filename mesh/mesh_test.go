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
