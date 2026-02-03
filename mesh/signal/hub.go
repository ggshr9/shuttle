package signal

import (
	"io"
	"log/slog"
	"net"
	"sync"
)

// Hub manages signaling message routing on the server side.
// It maintains a mapping of virtual IPs to their mesh streams
// and forwards signaling messages between peers.
type Hub struct {
	mu     sync.RWMutex
	peers  map[[4]byte]*PeerConn
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
		peers:  make(map[[4]byte]*PeerConn),
		logger: logger,
	}
}

// Register adds a peer to the hub.
func (h *Hub) Register(vip net.IP, w io.Writer) {
	var key [4]byte
	copy(key[:], vip.To4())

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
	var key [4]byte
	copy(key[:], vip.To4())

	h.mu.Lock()
	delete(h.peers, key)
	h.mu.Unlock()

	h.logger.Debug("signal: peer unregistered", "vip", vip)
}

// Forward routes a signaling message to its destination peer.
func (h *Hub) Forward(msg *Message) error {
	var dstKey [4]byte
	copy(dstKey[:], msg.DstVIP.To4())

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
	var srcKey [4]byte
	copy(srcKey[:], msg.SrcVIP.To4())

	data := msg.Encode()

	h.mu.RLock()
	defer h.mu.RUnlock()

	for key, peer := range h.peers {
		if key == srcKey {
			continue
		}

		peer.mu.Lock()
		peer.Writer.Write(data)
		peer.mu.Unlock()
	}
}

// GetPeerList returns a list of all registered peer VIPs.
func (h *Hub) GetPeerList() []net.IP {
	h.mu.RLock()
	defer h.mu.RUnlock()

	peers := make([]net.IP, 0, len(h.peers))
	for key := range h.peers {
		ip := make(net.IP, 4)
		copy(ip, key[:])
		peers = append(peers, ip)
	}
	return peers
}

// HasPeer checks if a peer is registered.
func (h *Hub) HasPeer(vip net.IP) bool {
	var key [4]byte
	copy(key[:], vip.To4())

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
// This is the main entry point for the hub.
func (h *Hub) HandleMessage(data []byte) error {
	msg, err := Decode(data)
	if err != nil {
		return err
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
