package signal

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"sync"
)

// hubVIPKey converts a net.IP virtual IP to a netip.Addr map key.
// IPv4-mapped IPv6 addresses are normalized to plain IPv4 so that
// net.IPv4(a,b,c,d) and its 16-byte form produce the same key.
func hubVIPKey(ip net.IP) netip.Addr {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}
	}
	return addr.Unmap()
}

// Hub manages signaling message routing on the server side.
// It maintains a mapping of virtual IPs to their mesh streams
// and forwards signaling messages between peers.
type Hub struct {
	mu     sync.RWMutex
	peers  map[netip.Addr]*PeerConn
	logger *slog.Logger
}

// PeerConn represents a connected peer's signaling channel.
type PeerConn struct {
	VIP    net.IP
	Writer io.Writer
	mu     sync.Mutex
}

// NewHub creates a new signaling hub.
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		peers:  make(map[netip.Addr]*PeerConn),
		logger: logger,
	}
}

// Register adds a peer to the hub.
func (h *Hub) Register(vip net.IP, w io.Writer) {
	key := hubVIPKey(vip)

	conn := &PeerConn{
		VIP:    vip,
		Writer: w,
	}

	h.mu.Lock()
	h.peers[key] = conn
	h.mu.Unlock()

	h.logger.Debug("signal: peer registered", "vip", vip)
}

// Unregister removes a peer from the hub.
func (h *Hub) Unregister(vip net.IP) {
	key := hubVIPKey(vip)

	h.mu.Lock()
	delete(h.peers, key)
	h.mu.Unlock()

	h.logger.Debug("signal: peer unregistered", "vip", vip)
}

// Forward routes a signaling message to its destination peer.
func (h *Hub) Forward(msg *Message) error {
	dstKey := hubVIPKey(msg.DstVIP)

	h.mu.RLock()
	peer, ok := h.peers[dstKey]
	h.mu.RUnlock()

	if !ok {
		h.logger.Debug("signal: peer not found", "dst", msg.DstVIP)
		return nil // Silently drop if peer not found
	}

	// Encode and send
	data := msg.Encode()

	peer.mu.Lock()
	_, err := peer.Writer.Write(data)
	peer.mu.Unlock()

	if err != nil {
		h.logger.Debug("signal: forward failed", "dst", msg.DstVIP, "err", err)
		return err
	}

	h.logger.Debug("signal: forwarded",
		"type", msg.Type,
		"src", msg.SrcVIP,
		"dst", msg.DstVIP,
		"len", len(msg.Payload))

	return nil
}

// Broadcast sends a message to all peers except the sender.
func (h *Hub) Broadcast(msg *Message) {
	srcKey := hubVIPKey(msg.SrcVIP)

	data := msg.Encode()

	h.mu.RLock()
	defer h.mu.RUnlock()

	for key, peer := range h.peers {
		if key == srcKey {
			continue
		}

		peer.mu.Lock()
		_, _ = peer.Writer.Write(data)
		peer.mu.Unlock()
	}
}

// GetPeerList returns a list of all registered peer VIPs.
func (h *Hub) GetPeerList() []net.IP {
	h.mu.RLock()
	defer h.mu.RUnlock()

	peers := make([]net.IP, 0, len(h.peers))
	for _, conn := range h.peers {
		peers = append(peers, conn.VIP)
	}
	return peers
}

// HasPeer checks if a peer is registered.
func (h *Hub) HasPeer(vip net.IP) bool {
	key := hubVIPKey(vip)

	h.mu.RLock()
	_, ok := h.peers[key]
	h.mu.RUnlock()

	return ok
}

// PeerCount returns the number of registered peers.
func (h *Hub) PeerCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.peers)
}

// HandleMessage processes an incoming signaling message.
// This is the main entry point for the hub. The senderVIP parameter
// identifies the authenticated peer that sent the message; if the
// decoded SrcVIP does not match, the message is rejected to prevent
// VIP spoofing.
func (h *Hub) HandleMessage(data []byte, senderVIP net.IP) error {
	msg, err := Decode(data)
	if err != nil {
		return err
	}

	// Verify the sender's VIP matches the message's SrcVIP to prevent impersonation.
	if !msg.SrcVIP.Equal(senderVIP) {
		h.logger.Warn("signal: SrcVIP mismatch, dropping message",
			"claimed", msg.SrcVIP, "actual", senderVIP)
		return fmt.Errorf("signal: SrcVIP %v does not match sender %v", msg.SrcVIP, senderVIP)
	}

	switch msg.Type {
	case SignalCandidate, SignalConnect, SignalConnectAck, SignalDisconnect:
		// Forward these messages to destination
		return h.Forward(msg)

	case SignalPing:
		// Respond with pong
		pong := NewPongMessage(msg.DstVIP, msg.SrcVIP)
		return h.Forward(pong)

	default:
		h.logger.Debug("signal: unknown message type", "type", msg.Type)
		return nil
	}
}
