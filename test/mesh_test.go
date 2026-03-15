//go:build sandbox

package test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/shuttle-proxy/shuttle/mesh"
)

// mockStream simulates a bidirectional stream using two pipes.
type mockStream struct {
	r io.ReadCloser
	w io.WriteCloser
}

func (m *mockStream) Read(p []byte) (int, error)  { return m.r.Read(p) }
func (m *mockStream) Write(p []byte) (int, error)  { return m.w.Write(p) }
func (m *mockStream) Close() error {
	m.w.Close()
	return m.r.Close()
}

// newStreamPair creates two connected mock streams (like a pipe pair).
func newStreamPair() (*mockStream, *mockStream) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()
	return &mockStream{r: r1, w: w2}, &mockStream{r: r2, w: w1}
}

// TestMeshRelay tests that two clients can exchange packets through the server's PeerTable.
func TestMeshRelay(t *testing.T) {
	logger := slog.Default()
	allocator, err := mesh.NewIPAllocator("10.7.0.0/24")
	if err != nil {
		t.Fatalf("NewIPAllocator: %v", err)
	}
	peerTable := mesh.NewPeerTable(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Simulate two client connections to the server
	clientStream1, serverStream1 := newStreamPair()
	clientStream2, serverStream2 := newStreamPair()

	// Server-side: handle both mesh streams
	var wg sync.WaitGroup

	serverHandle := func(stream io.ReadWriteCloser) {
		defer wg.Done()

		// Read magic
		magic := make([]byte, len(mesh.MeshMagic))
		if _, err := io.ReadFull(stream, magic); err != nil {
			t.Errorf("read magic: %v", err)
			return
		}
		if string(magic) != mesh.MeshMagic {
			t.Errorf("unexpected magic: %q", magic)
			return
		}

		// Allocate IP
		ip, err := allocator.Allocate()
		if err != nil {
			t.Errorf("allocate: %v", err)
			return
		}
		defer allocator.Release(ip)

		// Send handshake
		hs := mesh.EncodeHandshake(ip, allocator.Mask(), allocator.Gateway())
		if _, err := stream.Write(hs); err != nil {
			t.Errorf("write handshake: %v", err)
			return
		}

		// Register peer
		fw := &syncFrameWriter{stream: stream}
		peerTable.Register(ip, fw)
		defer peerTable.Unregister(ip)

		// Forward loop
		for {
			pkt, err := mesh.ReadFrame(stream)
			if err != nil {
				return
			}
			peerTable.Forward(pkt)
		}
	}

	wg.Add(2)
	go serverHandle(serverStream1)
	go serverHandle(serverStream2)

	// Client 1
	mc1, err := mesh.NewMeshClient(ctx, func(ctx context.Context) (io.ReadWriteCloser, error) {
		return clientStream1, nil
	})
	if err != nil {
		t.Fatalf("client1 handshake: %v", err)
	}
	defer mc1.Close()

	// Client 2
	mc2, err := mesh.NewMeshClient(ctx, func(ctx context.Context) (io.ReadWriteCloser, error) {
		return clientStream2, nil
	})
	if err != nil {
		t.Fatalf("client2 handshake: %v", err)
	}
	defer mc2.Close()

	t.Logf("client1 IP: %v, client2 IP: %v", mc1.VirtualIP(), mc2.VirtualIP())

	// Build a fake IPv4 packet from client1 to client2
	pkt := buildFakeIPv4Packet(mc1.VirtualIP(), mc2.VirtualIP(), []byte("hello from c1"))

	// Send from client1
	if err := mc1.Send(pkt); err != nil {
		t.Fatalf("client1 send: %v", err)
	}

	// Receive on client2
	got, err := mc2.Receive()
	if err != nil {
		t.Fatalf("client2 receive: %v", err)
	}

	if !bytes.Equal(got, pkt) {
		t.Fatalf("packet mismatch: got %d bytes, want %d bytes", len(got), len(pkt))
	}

	// Now send from client2 to client1
	pkt2 := buildFakeIPv4Packet(mc2.VirtualIP(), mc1.VirtualIP(), []byte("hello from c2"))
	if err := mc2.Send(pkt2); err != nil {
		t.Fatalf("client2 send: %v", err)
	}

	got2, err := mc1.Receive()
	if err != nil {
		t.Fatalf("client1 receive: %v", err)
	}

	if !bytes.Equal(got2, pkt2) {
		t.Fatalf("packet2 mismatch")
	}
}

// buildFakeIPv4Packet creates a minimal fake IPv4 packet with the given src/dst and payload.
func buildFakeIPv4Packet(src, dst net.IP, payload []byte) []byte {
	headerLen := 20
	totalLen := headerLen + len(payload)
	pkt := make([]byte, totalLen)
	pkt[0] = 0x45 // version=4, IHL=5
	pkt[2] = byte(totalLen >> 8)
	pkt[3] = byte(totalLen)
	pkt[8] = 64  // TTL
	pkt[9] = 17  // UDP
	copy(pkt[12:16], src.To4())
	copy(pkt[16:20], dst.To4())
	copy(pkt[20:], payload)
	return pkt
}

type syncFrameWriter struct {
	mu     sync.Mutex
	stream io.WriteCloser
}

func (sw *syncFrameWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if err := mesh.WriteFrame(sw.stream, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (sw *syncFrameWriter) Close() error { return sw.stream.Close() }

// connectClient creates a stream pair, starts handlePeer in a goroutine, and returns a connected MeshClient.
func connectClient(t *testing.T, ctx context.Context, ms *meshServer) *mesh.MeshClient {
	t.Helper()
	clientStream, serverStream := newStreamPair()
	errCh := make(chan error, 1)
	go func() {
		_, err := ms.handlePeer(serverStream)
		errCh <- err
	}()
	mc, err := mesh.NewMeshClient(ctx, func(ctx context.Context) (io.ReadWriteCloser, error) {
		return clientStream, nil
	})
	if err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("server handlePeer: %v", err)
	}
	return mc
}

// meshServer is a reusable test helper that runs the server-side mesh logic.
type meshServer struct {
	allocator *mesh.IPAllocator
	peerTable *mesh.PeerTable
}

func newMeshServer(cidr string) *meshServer {
	alloc, _ := mesh.NewIPAllocator(cidr)
	return &meshServer{
		allocator: alloc,
		peerTable: mesh.NewPeerTable(slog.Default()),
	}
}

func (ms *meshServer) handlePeer(stream io.ReadWriteCloser) (net.IP, error) {
	magic := make([]byte, len(mesh.MeshMagic))
	if _, err := io.ReadFull(stream, magic); err != nil {
		return nil, err
	}
	ip, err := ms.allocator.Allocate()
	if err != nil {
		return nil, err
	}
	hs := mesh.EncodeHandshake(ip, ms.allocator.Mask(), ms.allocator.Gateway())
	if _, err := stream.Write(hs); err != nil {
		ms.allocator.Release(ip)
		return nil, err
	}
	fw := &syncFrameWriter{stream: stream}
	ms.peerTable.Register(ip, fw)

	go func() {
		defer ms.peerTable.Unregister(ip)
		defer ms.allocator.Release(ip)
		for {
			pkt, err := mesh.ReadFrame(stream)
			if err != nil {
				return
			}
			ms.peerTable.Forward(pkt)
		}
	}()

	return ip, nil
}

// TestMeshRelayMultipleClients tests 4 clients relaying through server.
func TestMeshRelayMultipleClients(t *testing.T) {
	ms := newMeshServer("10.7.0.0/24")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const numClients = 4
	clients := make([]*mesh.MeshClient, numClients)
	for i := 0; i < numClients; i++ {
		clients[i] = connectClient(t, ctx, ms)
		t.Logf("client %d: %v", i, clients[i].VirtualIP())
	}
	defer func() {
		for _, mc := range clients {
			mc.Close()
		}
	}()

	// Each client sends to the next one
	for i := 0; i < numClients; i++ {
		src := clients[i]
		dst := clients[(i+1)%numClients]
		payload := []byte("from-" + src.VirtualIP().String())
		pkt := buildFakeIPv4Packet(src.VirtualIP(), dst.VirtualIP(), payload)

		if err := src.Send(pkt); err != nil {
			t.Fatalf("client %d send: %v", i, err)
		}

		got, err := dst.Receive()
		if err != nil {
			t.Fatalf("client %d receive: %v", (i+1)%numClients, err)
		}
		if !bytes.Equal(got, pkt) {
			t.Fatalf("relay %d->%d: packet mismatch", i, (i+1)%numClients)
		}
	}
}

// TestMeshClientSendAfterClose verifies Send returns error after Close.
func TestMeshClientSendAfterClose(t *testing.T) {
	ms := newMeshServer("10.7.0.0/24")
	ctx := context.Background()

	mc := connectClient(t, ctx, ms)
	mc.Close()

	pkt := buildFakeIPv4Packet(mc.VirtualIP(), net.IPv4(10, 7, 0, 99), []byte("test"))
	err := mc.Send(pkt)
	if err == nil {
		t.Fatal("expected error sending after close")
	}
}

// TestMeshRelayStress sends many packets concurrently between two clients.
func TestMeshRelayStress(t *testing.T) {
	ms := newMeshServer("10.7.0.0/24")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mc1 := connectClient(t, ctx, ms)
	mc2 := connectClient(t, ctx, ms)
	defer mc1.Close()
	defer mc2.Close()

	const numPackets = 100
	var wg sync.WaitGroup

	// c1 -> c2
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numPackets; i++ {
			pkt := buildFakeIPv4Packet(mc1.VirtualIP(), mc2.VirtualIP(), []byte{byte(i)})
			if err := mc1.Send(pkt); err != nil {
				t.Errorf("c1 send %d: %v", i, err)
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numPackets; i++ {
			_, err := mc2.Receive()
			if err != nil {
				t.Errorf("c2 receive %d: %v", i, err)
				return
			}
		}
	}()

	wg.Wait()
}

// TestMeshRelayToNonexistentPeer verifies packets to unknown IPs are dropped.
func TestMeshRelayToNonexistentPeer(t *testing.T) {
	ms := newMeshServer("10.7.0.0/24")
	ctx := context.Background()

	mc1 := connectClient(t, ctx, ms)
	defer mc1.Close()

	// Send to an IP that has no peer
	pkt := buildFakeIPv4Packet(mc1.VirtualIP(), net.IPv4(10, 7, 0, 99), []byte("void"))
	// Should not error — packet is just dropped by server
	if err := mc1.Send(pkt); err != nil {
		t.Fatalf("send: %v", err)
	}
}

// TestMeshClientVirtualIPAndCIDR verifies accessor methods.
func TestMeshClientVirtualIPAndCIDR(t *testing.T) {
	ms := newMeshServer("10.7.0.0/24")
	ctx := context.Background()

	mc := connectClient(t, ctx, ms)
	defer mc.Close()

	if mc.VirtualIP() == nil {
		t.Fatal("VirtualIP should not be nil")
	}
	cidr := mc.MeshCIDR()
	if cidr == "" {
		t.Fatal("MeshCIDR should not be empty")
	}
	// Should be parseable
	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("MeshCIDR not parseable: %v", err)
	}
	t.Logf("VirtualIP=%v, MeshCIDR=%s", mc.VirtualIP(), cidr)
}

// TestMeshClientOpenStreamError verifies error when openStream fails.
func TestMeshClientOpenStreamError(t *testing.T) {
	ctx := context.Background()
	_, err := mesh.NewMeshClient(ctx, func(ctx context.Context) (io.ReadWriteCloser, error) {
		return nil, io.ErrClosedPipe
	})
	if err == nil {
		t.Fatal("expected error when openStream fails")
	}
}
