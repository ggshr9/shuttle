package p2p

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/shuttleX/shuttle/mesh/signal"
)

// SendPacket sends a packet to a peer.
// Uses P2P connection if available, falls back to relay.
func (m *Manager) SendPacket(dstVIP net.IP, data []byte) error {
	key := vipKey(dstVIP)

	m.mu.RLock()
	peer, exists := m.peers[key]
	m.mu.RUnlock()

	// Try P2P first
	if exists && peer.State == StateConnected && peer.P2PConn != nil && !peer.UseRelay {
		err := peer.P2PConn.Send(data)
		if err == nil {
			return nil
		}

		m.logger.Debug("p2p: send failed, falling back to relay",
			"peer", dstVIP,
			"err", err)

		// Mark for relay
		m.mu.Lock()
		peer.FailCount++
		if peer.FailCount >= 3 {
			peer.UseRelay = true
		}
		m.mu.Unlock()
	}

	// Fall back to relay
	if m.relayFunc != nil {
		return m.relayFunc(data)
	}

	return fmt.Errorf("p2p: no route to %s", dstVIP)
}

// Connect initiates a P2P connection to a peer.
func (m *Manager) Connect(ctx context.Context, dstVIP net.IP) error {
	key := vipKey(dstVIP)

	m.mu.Lock()
	peer := m.getOrCreatePeer(dstVIP)

	if peer.State == StateConnected || peer.State == StateConnecting {
		m.mu.Unlock()
		return nil
	}

	peer.State = StateConnecting
	peer.LastAttempt = time.Now()
	m.mu.Unlock()

	m.logger.Info("p2p: initiating connection", "peer", dstVIP)

	// Build local candidate list, augmenting with a verified mDNS host
	// candidate when one is available (fast LAN path on reconnect).
	candidates := m.candidates
	m.mu.RLock()
	mdnsSvc := m.mdns
	m.mu.RUnlock()
	if mdnsSvc != nil {
		if mdnsPeer := mdnsSvc.GetPeerByVIP(dstVIP); mdnsPeer != nil && mdnsPeer.Verified && mdnsPeer.Port > 0 {
			for _, addr := range mdnsPeer.Addresses {
				hostCandidate := NewCandidate(CandidateHost, &net.UDPAddr{IP: addr, Port: mdnsPeer.Port})
				candidates = append(candidates, hostCandidate)
			}
			m.logger.Debug("p2p: added verified mDNS host candidates",
				"peer", dstVIP,
				"count", len(mdnsPeer.Addresses))
		}
	}

	// Convert candidates to signal format
	candidateInfos := m.candidatesToInfo(candidates)

	// Perform signaling handshake
	result, err := m.signalClient.Handshake(ctx, dstVIP, m.localPub, candidateInfos, m.holePunchTimeout)
	if err != nil {
		m.logger.Debug("p2p: signaling handshake failed", "peer", dstVIP, "err", err)
		m.markFailed(key)
		return err
	}

	// Convert remote candidates
	remoteCandidates := m.infoCandidates(result.RemoteCandidates)

	m.mu.Lock()
	peer.Candidates = remoteCandidates
	m.mu.Unlock()

	// Perform hole punching — register the puncher so receiveLoop forwards
	// hole-punch packets to it instead of discarding them.
	puncher := NewHolePuncher(m.udpConn, m.localVIP, m.holePunchTimeout, m.logger)
	m.setActiveHolePuncher(puncher)
	punchResult, err := puncher.Punch(ctx, dstVIP, remoteCandidates)
	m.clearActiveHolePuncher()
	if err != nil {
		m.logger.Debug("p2p: hole punch failed", "peer", dstVIP, "err", err)
		m.markFailed(key)
		return err
	}

	m.logger.Info("p2p: hole punch succeeded",
		"peer", dstVIP,
		"addr", punchResult.RemoteAddr,
		"rtt", punchResult.RTT)

	// Derive keys
	sharedSecret, err := m.deriveSharedSecret(result.RemotePublicKey)
	if err != nil {
		m.markFailed(key)
		return fmt.Errorf("p2p: derive shared secret: %w", err)
	}

	// X25519 handshake succeeded — the remote peer owns this VIP.
	// Mark it verified in mDNS so future reconnects can use the LAN shortcut.
	m.mu.RLock()
	mdnsSvcMark := m.mdns
	m.mu.RUnlock()
	if mdnsSvcMark != nil {
		mdnsSvcMark.MarkVerified(dstVIP)
	}

	sendKey, recvKey, err := DeriveP2PKeys(sharedSecret, true)
	if err != nil {
		m.markFailed(key)
		return fmt.Errorf("p2p: derive keys: %w", err)
	}

	// Create P2P connection
	p2pConn, err := NewP2PConn(m.udpConn, punchResult.RemoteAddr, dstVIP, m.localVIP, sendKey, recvKey)
	if err != nil {
		m.markFailed(key)
		return err
	}

	m.mu.Lock()
	peer.P2PConn = p2pConn
	peer.State = StateConnected
	peer.FailCount = 0
	peer.UseRelay = false
	m.mu.Unlock()

	// Reset FallbackController so the peer is no longer treated as relay-only
	// (important after an ICE restart or reconnect clears past failures).
	m.fallback.ResetPeer(dstVIP)

	// Record successful path for faster reconnection
	method := m.detectConnectionMethod(punchResult)
	m.pathCache.RecordSuccess(dstVIP, punchResult.RemoteAddr, method, punchResult.RTT)

	m.logger.Info("p2p: connection established", "peer", dstVIP, "method", method)
	return nil
}

// detectConnectionMethod determines how the connection was established
func (m *Manager) detectConnectionMethod(result *HolePunchResult) ConnectionMethod {
	if result == nil || result.RemoteAddr == nil {
		return MethodUnknown
	}

	// Check if we used UPnP/NAT-PMP
	if m.upnpEnabled && m.portMapper != nil {
		protocol := m.portMapper.Protocol()
		if protocol == "upnp" {
			return MethodUPnP
		}
		if protocol == "nat-pmp" {
			return MethodNATPMP
		}
	}

	// Check if it's a direct LAN connection
	if isPrivateIP(result.RemoteAddr.IP) && isPrivateIP(m.udpConn.LocalAddr().(*net.UDPAddr).IP) {
		localNet := getNetworkPrefix(m.udpConn.LocalAddr().(*net.UDPAddr).IP)
		remoteNet := getNetworkPrefix(result.RemoteAddr.IP)
		if localNet == remoteNet {
			return MethodDirect
		}
	}

	return MethodSTUN
}

// handleCandidates handles incoming candidate messages.
func (m *Manager) handleCandidates(msg *signal.Message) {
	candidates, err := signal.DecodeCandidates(msg.Payload)
	if err != nil {
		m.logger.Debug("p2p: decode candidates failed", "err", err)
		return
	}

	m.mu.Lock()
	peer := m.getOrCreatePeer(msg.SrcVIP)
	peer.Candidates = m.infoCandidates(candidates)
	m.mu.Unlock()

	m.logger.Debug("p2p: received candidates",
		"peer", msg.SrcVIP,
		"count", len(candidates))
}

// handleConnect handles incoming connection requests.
func (m *Manager) handleConnect(msg *signal.Message) {
	connectInfo, err := signal.DecodeConnectInfo(msg.Payload)
	if err != nil {
		m.logger.Debug("p2p: decode connect info failed", "err", err)
		return
	}

	key := vipKey(msg.SrcVIP)

	m.mu.Lock()
	peer := m.getOrCreatePeer(msg.SrcVIP)
	m.mu.Unlock()

	m.logger.Info("p2p: received connection request", "peer", msg.SrcVIP)

	// Respond with our candidates and public key
	candidateInfos := m.candidatesToInfo(m.candidates)
	if err := m.signalClient.RespondToHandshake(m.ctx, msg.SrcVIP, m.localPub, candidateInfos); err != nil {
		m.logger.Debug("p2p: respond to handshake failed", "err", err)
		return
	}

	// Start hole punching in background
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		puncher := NewHolePuncher(m.udpConn, m.localVIP, m.holePunchTimeout, m.logger)

		m.mu.RLock()
		candidates := peer.Candidates
		m.mu.RUnlock()

		if len(candidates) == 0 {
			m.logger.Debug("p2p: no candidates for peer", "peer", msg.SrcVIP)
			return
		}

		punchCtx, punchCancel := context.WithTimeout(m.ctx, m.holePunchTimeout)
		defer punchCancel()
		// Register puncher so receiveLoop forwards hole-punch packets to it.
		m.setActiveHolePuncher(puncher)
		punchResult, err := puncher.Punch(punchCtx, msg.SrcVIP, candidates)
		m.clearActiveHolePuncher()
		if err != nil {
			m.logger.Debug("p2p: hole punch failed", "peer", msg.SrcVIP, "err", err)
			m.markFailed(key)
			return
		}

		// Derive keys (as responder)
		sharedSecret, err := m.deriveSharedSecret(connectInfo.PublicKey)
		if err != nil {
			m.logger.Debug("p2p: derive shared secret failed", "peer", msg.SrcVIP, "err", err)
			m.markFailed(key)
			return
		}

		// X25519 handshake succeeded — mark this VIP as verified in mDNS.
		m.mu.RLock()
		mdnsSvc := m.mdns
		m.mu.RUnlock()
		if mdnsSvc != nil {
			mdnsSvc.MarkVerified(msg.SrcVIP)
		}

		sendKey, recvKey, err := DeriveP2PKeys(sharedSecret, false)
		if err != nil {
			m.logger.Debug("p2p: derive keys failed", "peer", msg.SrcVIP, "err", err)
			m.markFailed(key)
			return
		}

		// Create P2P connection
		p2pConn, err := NewP2PConn(m.udpConn, punchResult.RemoteAddr, msg.SrcVIP, m.localVIP, sendKey, recvKey)
		if err != nil {
			m.markFailed(key)
			return
		}

		m.mu.Lock()
		peer.P2PConn = p2pConn
		peer.State = StateConnected
		peer.FailCount = 0
		peer.UseRelay = false
		m.mu.Unlock()

		// Reset FallbackController so accumulated relay state is cleared.
		m.fallback.ResetPeer(msg.SrcVIP)

		m.logger.Info("p2p: connection established (responder)", "peer", msg.SrcVIP)
	}()
}

// handleDisconnect handles disconnect notifications.
func (m *Manager) handleDisconnect(msg *signal.Message) {
	key := vipKey(msg.SrcVIP)

	m.mu.Lock()
	if peer, ok := m.peers[key]; ok {
		if peer.P2PConn != nil {
			peer.P2PConn.Close()
		}
		peer.State = StateDisconnected
		peer.P2PConn = nil
	}
	m.mu.Unlock()

	m.logger.Info("p2p: peer disconnected", "peer", msg.SrcVIP)
}

// receiveLoop handles incoming UDP packets.
func (m *Manager) receiveLoop() {
	buf := make([]byte, 1500)

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		_ = m.udpConn.SetReadDeadline(time.Now().Add(time.Second))
		n, addr, err := m.udpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			m.logger.Debug("p2p: receive error", "err", err)
			continue
		}

		data := buf[:n]

		// Handle hole punch packets — dispatch to the active HolePuncher (if
		// any) via channel so the HolePuncher's receiveLoop sees every packet.
		// Previously this was a bare continue which discarded the packet,
		// creating a race between Manager and HolePuncher on the shared socket.
		if IsHolePunchPacket(data) {
			m.deliverToHolePuncher(data, addr)
			continue
		}

		// Handle P2P data packets
		if IsP2PPacket(data) {
			m.handleP2PPacket(data, addr)
			continue
		}
	}
}

// handleP2PPacket handles an incoming P2P packet.
func (m *Manager) handleP2PPacket(data []byte, from *net.UDPAddr) {
	// Find the peer by address
	m.mu.RLock()
	var peer *PeerConnection
	for _, p := range m.peers {
		if p.P2PConn != nil && p.P2PConn.RemoteAddr().String() == from.String() {
			peer = p
			break
		}
	}
	m.mu.RUnlock()

	if peer == nil || peer.P2PConn == nil {
		m.logger.Debug("p2p: packet from unknown peer", "from", from)
		return
	}

	plaintext, typ, err := peer.P2PConn.Decrypt(data)
	if err != nil {
		m.logger.Debug("p2p: decrypt failed", "err", err)
		return
	}

	switch typ {
	case P2PData:
		m.mu.RLock()
		handler := m.dataHandler
		m.mu.RUnlock()
		if handler != nil {
			handler(peer.VIP, plaintext)
		} else {
			m.logger.Debug("p2p: received data but no handler set", "len", len(plaintext), "peer", peer.VIP)
		}
	case P2PKeepAlive:
		m.logger.Debug("p2p: received keepalive", "peer", peer.VIP)
	case P2PClose:
		m.logger.Info("p2p: peer closed connection", "peer", peer.VIP)
		m.mu.Lock()
		peer.State = StateDisconnected
		peer.P2PConn = nil
		m.mu.Unlock()
	}
}

// keepAliveLoop sends keep-alive packets to connected peers.
func (m *Manager) keepAliveLoop() {
	ticker := time.NewTicker(m.keepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.mu.RLock()
			for _, peer := range m.peers {
				if peer.State == StateConnected && peer.P2PConn != nil {
					_ = peer.P2PConn.SendKeepAlive()
				}
			}
			m.mu.RUnlock()
		}
	}
}

// retryLoop periodically retries failed connections.
func (m *Manager) retryLoop() {
	ticker := time.NewTicker(m.directRetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.mu.RLock()
			toRetry := make([]net.IP, 0)
			for _, peer := range m.peers {
				if peer.UseRelay && time.Since(peer.LastAttempt) > m.directRetryInterval {
					toRetry = append(toRetry, peer.VIP)
				}
			}
			m.mu.RUnlock()

			for _, vip := range toRetry {
				go func(v net.IP) {
					if err := m.Connect(m.ctx, v); err != nil {
						m.logger.Debug("background reconnect failed", "vip", v, "err", err)
					}
				}(vip)
			}
		}
	}
}
