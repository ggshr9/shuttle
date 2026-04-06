package signal

import (
	"bytes"
	"log/slog"
	"net"
	"sync"
	"testing"
)

func newTestHub() *Hub {
	return NewHub(slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{Level: slog.LevelDebug})))
}

func TestNewHub(t *testing.T) {
	h := newTestHub()
	if h == nil {
		t.Fatal("NewHub returned nil")
		return
	}
	if h.peers == nil {
		t.Fatal("peers map is nil")
	}
	if h.PeerCount() != 0 {
		t.Fatalf("expected 0 peers, got %d", h.PeerCount())
	}
}

func TestNewHubWithCustomLogger(t *testing.T) {
	logger := slog.Default()
	h := NewHub(logger)
	if h.logger != logger {
		t.Fatal("logger not set correctly")
	}
}

func TestHubRegisterAndUnregister(t *testing.T) {
	h := newTestHub()
	vip := net.IPv4(10, 7, 0, 1)
	w := &bytes.Buffer{}

	h.Register(vip, w)

	if h.PeerCount() != 1 {
		t.Fatalf("expected 1 peer, got %d", h.PeerCount())
	}
	if !h.HasPeer(vip) {
		t.Fatal("expected peer to be registered")
	}

	h.Unregister(vip)

	if h.PeerCount() != 0 {
		t.Fatalf("expected 0 peers, got %d", h.PeerCount())
	}
	if h.HasPeer(vip) {
		t.Fatal("expected peer to be unregistered")
	}
}

func TestHubRegisterMultiplePeers(t *testing.T) {
	h := newTestHub()

	peers := []net.IP{
		net.IPv4(10, 7, 0, 1),
		net.IPv4(10, 7, 0, 2),
		net.IPv4(10, 7, 0, 3),
	}

	for _, vip := range peers {
		h.Register(vip, &bytes.Buffer{})
	}

	if h.PeerCount() != 3 {
		t.Fatalf("expected 3 peers, got %d", h.PeerCount())
	}

	for _, vip := range peers {
		if !h.HasPeer(vip) {
			t.Fatalf("expected peer %v to be registered", vip)
		}
	}
}

func TestHubRegisterOverwrite(t *testing.T) {
	h := newTestHub()
	vip := net.IPv4(10, 7, 0, 1)

	w1 := &bytes.Buffer{}
	w2 := &bytes.Buffer{}

	h.Register(vip, w1)
	h.Register(vip, w2)

	// Should still be 1 peer (overwritten)
	if h.PeerCount() != 1 {
		t.Fatalf("expected 1 peer after overwrite, got %d", h.PeerCount())
	}

	// Verify the new writer is used by forwarding a message
	msg := &Message{
		Type:    SignalPing,
		SrcVIP:  net.IPv4(10, 7, 0, 2),
		DstVIP:  vip,
		Payload: nil,
	}
	h.Register(net.IPv4(10, 7, 0, 2), &bytes.Buffer{}) // register sender too
	if err := h.Forward(msg); err != nil {
		t.Fatalf("Forward failed: %v", err)
	}

	if w1.Len() != 0 {
		t.Fatal("old writer should not receive data")
	}
	if w2.Len() == 0 {
		t.Fatal("new writer should receive data")
	}
}

func TestHubUnregisterNonexistent(t *testing.T) {
	h := newTestHub()
	// Should not panic
	h.Unregister(net.IPv4(10, 7, 0, 99))
	if h.PeerCount() != 0 {
		t.Fatal("expected 0 peers")
	}
}

func TestHubHasPeerNotRegistered(t *testing.T) {
	h := newTestHub()
	if h.HasPeer(net.IPv4(10, 7, 0, 1)) {
		t.Fatal("expected HasPeer to return false for unregistered peer")
	}
}

func TestHubGetPeerList(t *testing.T) {
	h := newTestHub()

	vips := []net.IP{
		net.IPv4(10, 7, 0, 1),
		net.IPv4(10, 7, 0, 2),
		net.IPv4(10, 7, 0, 3),
	}

	for _, vip := range vips {
		h.Register(vip, &bytes.Buffer{})
	}

	list := h.GetPeerList()
	if len(list) != 3 {
		t.Fatalf("expected 3 peers in list, got %d", len(list))
	}

	// Each registered VIP should appear in the list
	found := make(map[string]bool)
	for _, ip := range list {
		found[ip.String()] = true
	}
	for _, vip := range vips {
		if !found[vip.To4().String()] {
			t.Fatalf("VIP %v not found in peer list", vip)
		}
	}
}

func TestHubGetPeerListEmpty(t *testing.T) {
	h := newTestHub()
	list := h.GetPeerList()
	if len(list) != 0 {
		t.Fatalf("expected empty peer list, got %d", len(list))
	}
}

func TestHubForwardMessage(t *testing.T) {
	h := newTestHub()

	srcVIP := net.IPv4(10, 7, 0, 1)
	dstVIP := net.IPv4(10, 7, 0, 2)

	srcBuf := &bytes.Buffer{}
	dstBuf := &bytes.Buffer{}

	h.Register(srcVIP, srcBuf)
	h.Register(dstVIP, dstBuf)

	msg := &Message{
		Type:    SignalConnect,
		SrcVIP:  srcVIP,
		DstVIP:  dstVIP,
		Payload: []byte("hello"),
	}

	if err := h.Forward(msg); err != nil {
		t.Fatalf("Forward failed: %v", err)
	}

	// Destination should have received the encoded message
	if dstBuf.Len() == 0 {
		t.Fatal("destination buffer should have received data")
	}

	// Source should not have received anything
	if srcBuf.Len() != 0 {
		t.Fatal("source buffer should not have received data")
	}

	// Decode the forwarded message
	decoded, err := Decode(dstBuf.Bytes())
	if err != nil {
		t.Fatalf("failed to decode forwarded message: %v", err)
	}
	if decoded.Type != SignalConnect {
		t.Fatalf("expected type SignalConnect, got %d", decoded.Type)
	}
	if !decoded.SrcVIP.Equal(srcVIP.To4()) {
		t.Fatalf("SrcVIP mismatch: got %v, want %v", decoded.SrcVIP, srcVIP)
	}
	if !decoded.DstVIP.Equal(dstVIP.To4()) {
		t.Fatalf("DstVIP mismatch: got %v, want %v", decoded.DstVIP, dstVIP)
	}
	if string(decoded.Payload) != "hello" {
		t.Fatalf("payload mismatch: got %q, want %q", decoded.Payload, "hello")
	}
}

func TestHubForwardToUnknownPeer(t *testing.T) {
	h := newTestHub()

	msg := &Message{
		Type:   SignalConnect,
		SrcVIP: net.IPv4(10, 7, 0, 1),
		DstVIP: net.IPv4(10, 7, 0, 99),
	}

	// Should return nil (silently drop)
	err := h.Forward(msg)
	if err != nil {
		t.Fatalf("expected nil error for unknown peer, got: %v", err)
	}
}

type errorWriter struct{}

func (e *errorWriter) Write(p []byte) (int, error) {
	return 0, net.ErrClosed
}

func TestHubForwardWriteError(t *testing.T) {
	h := newTestHub()
	dstVIP := net.IPv4(10, 7, 0, 2)

	h.Register(dstVIP, &errorWriter{})

	msg := &Message{
		Type:   SignalConnect,
		SrcVIP: net.IPv4(10, 7, 0, 1),
		DstVIP: dstVIP,
	}

	err := h.Forward(msg)
	if err == nil {
		t.Fatal("expected error from failed write")
	}
}

func TestHubBroadcast(t *testing.T) {
	h := newTestHub()

	srcVIP := net.IPv4(10, 7, 0, 1)
	dst1VIP := net.IPv4(10, 7, 0, 2)
	dst2VIP := net.IPv4(10, 7, 0, 3)

	srcBuf := &bytes.Buffer{}
	dst1Buf := &bytes.Buffer{}
	dst2Buf := &bytes.Buffer{}

	h.Register(srcVIP, srcBuf)
	h.Register(dst1VIP, dst1Buf)
	h.Register(dst2VIP, dst2Buf)

	msg := &Message{
		Type:    SignalCandidate,
		SrcVIP:  srcVIP,
		DstVIP:  net.IPv4(0, 0, 0, 0),
		Payload: []byte("broadcast data"),
	}

	h.Broadcast(msg)

	// Source should NOT receive the broadcast
	if srcBuf.Len() != 0 {
		t.Fatal("sender should not receive its own broadcast")
	}

	// Both destinations should receive
	if dst1Buf.Len() == 0 {
		t.Fatal("dst1 should have received broadcast")
	}
	if dst2Buf.Len() == 0 {
		t.Fatal("dst2 should have received broadcast")
	}

	// Verify both received the same encoded message
	if !bytes.Equal(dst1Buf.Bytes(), dst2Buf.Bytes()) {
		t.Fatal("both destinations should receive identical data")
	}
}

func TestHubBroadcastSinglePeer(t *testing.T) {
	h := newTestHub()

	vip := net.IPv4(10, 7, 0, 1)
	buf := &bytes.Buffer{}
	h.Register(vip, buf)

	msg := &Message{
		Type:   SignalPing,
		SrcVIP: vip,
		DstVIP: net.IPv4(0, 0, 0, 0),
	}

	// Broadcasting when sender is the only peer - no one should receive
	h.Broadcast(msg)

	if buf.Len() != 0 {
		t.Fatal("sole peer should not receive its own broadcast")
	}
}

func TestHubBroadcastNoPeers(t *testing.T) {
	h := newTestHub()

	msg := &Message{
		Type:   SignalPing,
		SrcVIP: net.IPv4(10, 7, 0, 1),
		DstVIP: net.IPv4(0, 0, 0, 0),
	}

	// Should not panic
	h.Broadcast(msg)
}

func TestHubHandleMessageForward(t *testing.T) {
	h := newTestHub()

	srcVIP := net.IPv4(10, 7, 0, 1)
	dstVIP := net.IPv4(10, 7, 0, 2)

	dstBuf := &bytes.Buffer{}
	h.Register(srcVIP, &bytes.Buffer{})
	h.Register(dstVIP, dstBuf)

	// Build message types that should be forwarded
	forwardTypes := []byte{SignalCandidate, SignalConnect, SignalConnectAck, SignalDisconnect}

	for _, msgType := range forwardTypes {
		dstBuf.Reset()

		msg := &Message{
			Type:   msgType,
			SrcVIP: srcVIP,
			DstVIP: dstVIP,
		}
		data := msg.Encode()

		err := h.HandleMessage(data, srcVIP)
		if err != nil {
			t.Fatalf("HandleMessage failed for type 0x%02x: %v", msgType, err)
		}

		if dstBuf.Len() == 0 {
			t.Fatalf("destination should have received forwarded message of type 0x%02x", msgType)
		}
	}
}

func TestHubHandleMessagePing(t *testing.T) {
	h := newTestHub()

	srcVIP := net.IPv4(10, 7, 0, 1)
	dstVIP := net.IPv4(10, 7, 0, 2)

	srcBuf := &bytes.Buffer{}
	h.Register(srcVIP, srcBuf)
	h.Register(dstVIP, &bytes.Buffer{})

	pingMsg := NewPingMessage(srcVIP, dstVIP)
	data := pingMsg.Encode()

	err := h.HandleMessage(data, srcVIP)
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	// Ping handler should send a pong back to the source
	if srcBuf.Len() == 0 {
		t.Fatal("source should have received pong response")
	}

	// Decode and verify it's a pong
	decoded, err := Decode(srcBuf.Bytes())
	if err != nil {
		t.Fatalf("failed to decode pong: %v", err)
	}
	if decoded.Type != SignalPong {
		t.Fatalf("expected pong (0x%02x), got 0x%02x", SignalPong, decoded.Type)
	}
	// Pong goes from dstVIP to srcVIP
	if !decoded.SrcVIP.Equal(dstVIP.To4()) {
		t.Fatalf("pong SrcVIP: got %v, want %v", decoded.SrcVIP, dstVIP)
	}
	if !decoded.DstVIP.Equal(srcVIP.To4()) {
		t.Fatalf("pong DstVIP: got %v, want %v", decoded.DstVIP, srcVIP)
	}
}

func TestHubHandleMessageSrcMismatch(t *testing.T) {
	h := newTestHub()

	claimedSrc := net.IPv4(10, 7, 0, 1)
	actualSrc := net.IPv4(10, 7, 0, 99)
	dstVIP := net.IPv4(10, 7, 0, 2)

	h.Register(claimedSrc, &bytes.Buffer{})
	h.Register(dstVIP, &bytes.Buffer{})

	msg := &Message{
		Type:   SignalConnect,
		SrcVIP: claimedSrc,
		DstVIP: dstVIP,
	}
	data := msg.Encode()

	err := h.HandleMessage(data, actualSrc)
	if err == nil {
		t.Fatal("expected error for VIP mismatch")
	}
}

func TestHubHandleMessageInvalidData(t *testing.T) {
	h := newTestHub()

	err := h.HandleMessage([]byte{1, 2, 3}, net.IPv4(10, 7, 0, 1))
	if err == nil {
		t.Fatal("expected error for invalid data")
	}
}

func TestHubHandleMessageUnknownType(t *testing.T) {
	h := newTestHub()

	srcVIP := net.IPv4(10, 7, 0, 1)
	h.Register(srcVIP, &bytes.Buffer{})

	msg := &Message{
		Type:   0xFE, // Unknown type (not SignalError 0xFF)
		SrcVIP: srcVIP,
		DstVIP: net.IPv4(10, 7, 0, 2),
	}
	data := msg.Encode()

	// Should return nil for unknown types
	err := h.HandleMessage(data, srcVIP)
	if err != nil {
		t.Fatalf("expected nil error for unknown type, got: %v", err)
	}
}

func TestHubConcurrentRegisterUnregister(t *testing.T) {
	h := newTestHub()
	var wg sync.WaitGroup

	// Concurrently register 100 peers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			vip := net.IPv4(10, 7, byte(i/256), byte(i%256))
			h.Register(vip, &bytes.Buffer{})
		}(i)
	}
	wg.Wait()

	if h.PeerCount() != 100 {
		t.Fatalf("expected 100 peers, got %d", h.PeerCount())
	}

	// Concurrently unregister all peers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			vip := net.IPv4(10, 7, byte(i/256), byte(i%256))
			h.Unregister(vip)
		}(i)
	}
	wg.Wait()

	if h.PeerCount() != 0 {
		t.Fatalf("expected 0 peers after unregister, got %d", h.PeerCount())
	}
}

func TestHubConcurrentForward(t *testing.T) {
	h := newTestHub()

	// Set up peers with thread-safe writers
	type safeBuf struct {
		mu  sync.Mutex
		buf bytes.Buffer
	}

	dst := &safeBuf{}
	dstVIP := net.IPv4(10, 7, 0, 1)

	// Use a thread-safe writer wrapper
	h.Register(dstVIP, &threadSafeWriter{buf: &dst.buf, mu: &dst.mu})

	var wg sync.WaitGroup

	// Concurrently forward messages from multiple senders
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			srcVIP := net.IPv4(10, 7, 1, byte(i))
			msg := &Message{
				Type:    SignalPing,
				SrcVIP:  srcVIP,
				DstVIP:  dstVIP,
				Payload: []byte{byte(i)},
			}
			_ = h.Forward(msg)
		}(i)
	}
	wg.Wait()

	dst.mu.Lock()
	totalBytes := dst.buf.Len()
	dst.mu.Unlock()

	if totalBytes == 0 {
		t.Fatal("destination should have received data from concurrent forwards")
	}
}

func TestHubConcurrentBroadcast(t *testing.T) {
	h := newTestHub()

	// Register several peers
	for i := 0; i < 10; i++ {
		vip := net.IPv4(10, 7, 0, byte(i+1))
		h.Register(vip, &bytes.Buffer{})
	}

	var wg sync.WaitGroup

	// Concurrently broadcast from multiple senders
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			srcVIP := net.IPv4(10, 7, 0, byte(i+1))
			msg := &Message{
				Type:   SignalPing,
				SrcVIP: srcVIP,
				DstVIP: net.IPv4(0, 0, 0, 0),
			}
			h.Broadcast(msg)
		}(i)
	}
	wg.Wait()

	// Just verifying no panic/race
}

func TestHubConcurrentMixedOperations(t *testing.T) {
	h := newTestHub()
	var wg sync.WaitGroup

	// Mix of register, unregister, forward, broadcast, has, count operations
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			vip := net.IPv4(10, 7, 0, byte(i%10+1))

			switch i % 6 {
			case 0:
				h.Register(vip, &bytes.Buffer{})
			case 1:
				h.Unregister(vip)
			case 2:
				h.HasPeer(vip)
			case 3:
				h.PeerCount()
			case 4:
				h.GetPeerList()
			case 5:
				msg := &Message{
					Type:   SignalPing,
					SrcVIP: vip,
					DstVIP: net.IPv4(10, 7, 0, byte((i+1)%10+1)),
				}
				_ = h.Forward(msg)
			}
		}(i)
	}
	wg.Wait()
}

// TestHubIPv6NoCollision verifies that distinct IPv6 VIPs produce distinct keys
// and do not collide to the same map entry as they did with [4]byte keys.
func TestHubIPv6NoCollision(t *testing.T) {
	h := newTestHub()

	vip1 := net.ParseIP("fd00:7::2")
	vip2 := net.ParseIP("fd00:7::3")

	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}

	h.Register(vip1, buf1)
	h.Register(vip2, buf2)

	if h.PeerCount() != 2 {
		t.Fatalf("expected 2 peers for distinct IPv6 VIPs, got %d (key collision!)", h.PeerCount())
	}

	if !h.HasPeer(vip1) {
		t.Fatal("vip1 should be registered")
	}
	if !h.HasPeer(vip2) {
		t.Fatal("vip2 should be registered")
	}

	// Forward a message only to vip2; vip1 must not receive it.
	msg := &Message{
		Type:    SignalConnect,
		SrcVIP:  vip1,
		DstVIP:  vip2,
		Payload: []byte("hello ipv6"),
	}
	if err := h.Forward(msg); err != nil {
		t.Fatalf("Forward failed: %v", err)
	}

	if buf2.Len() == 0 {
		t.Fatal("vip2 should have received the message")
	}
	if buf1.Len() != 0 {
		t.Fatal("vip1 should not have received the message (collision detected!)")
	}
}

// TestHubMixedIPv4IPv6 verifies IPv4 and IPv6 VIPs coexist without collision.
func TestHubMixedIPv4IPv6(t *testing.T) {
	h := newTestHub()

	v4VIP := net.IPv4(10, 7, 0, 2)
	v6VIP := net.ParseIP("fd00:7::2")

	buf4 := &bytes.Buffer{}
	buf6 := &bytes.Buffer{}

	h.Register(v4VIP, buf4)
	h.Register(v6VIP, buf6)

	if h.PeerCount() != 2 {
		t.Fatalf("expected 2 peers for IPv4+IPv6 VIPs, got %d (key collision!)", h.PeerCount())
	}

	// Forward to v6VIP; v4VIP must not receive it.
	msg := &Message{
		Type:    SignalConnect,
		SrcVIP:  v4VIP,
		DstVIP:  v6VIP,
		Payload: []byte("ipv4->ipv6"),
	}
	if err := h.Forward(msg); err != nil {
		t.Fatalf("Forward failed: %v", err)
	}

	if buf6.Len() == 0 {
		t.Fatal("v6VIP should have received the message")
	}
	if buf4.Len() != 0 {
		t.Fatal("v4VIP should not have received the message (collision detected!)")
	}
}

// TestHubIPv6Unregister verifies that IPv6 peers can be unregistered correctly.
func TestHubIPv6Unregister(t *testing.T) {
	h := newTestHub()

	vip := net.ParseIP("fd00:7::5")
	h.Register(vip, &bytes.Buffer{})

	if !h.HasPeer(vip) {
		t.Fatal("IPv6 peer should be registered")
	}

	h.Unregister(vip)

	if h.HasPeer(vip) {
		t.Fatal("IPv6 peer should be unregistered")
	}
	if h.PeerCount() != 0 {
		t.Fatalf("expected 0 peers, got %d", h.PeerCount())
	}
}

// threadSafeWriter wraps a bytes.Buffer with a mutex for concurrent writes.
type threadSafeWriter struct {
	buf *bytes.Buffer
	mu  *sync.Mutex
}

func (w *threadSafeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}
