package p2p

import (
	"net"
	"time"
)

// vipKey converts a VIP address to the [4]byte map key used in m.peers.
func vipKey(ip net.IP) [4]byte {
	var key [4]byte
	copy(key[:], ip.To4())
	return key
}

// getOrCreatePeer returns the PeerConnection for the given VIP, creating a new
// disconnected entry if one does not exist. The caller must hold m.mu (write lock).
func (m *Manager) getOrCreatePeer(vip net.IP) *PeerConnection {
	key := vipKey(vip)
	peer, exists := m.peers[key]
	if !exists {
		peer = &PeerConnection{
			VIP:   vip,
			State: StateDisconnected,
		}
		m.peers[key] = peer
	}
	return peer
}

// scheduleReconnect spawns a goroutine that waits briefly then attempts to
// reconnect to the given VIP. Used after ICE restarts to allow the signal to
// propagate before reconnection.
func (m *Manager) scheduleReconnect(vip net.IP) {
	go func() {
		time.Sleep(500 * time.Millisecond)
		if err := m.Connect(m.ctx, vip); err != nil {
			m.logger.Debug("p2p: reconnect after ICE restart failed",
				"peer", vip,
				"err", err)
		}
	}()
}
