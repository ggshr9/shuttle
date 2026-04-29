package mesh

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/ggshr9/shuttle/mesh/p2p"
	"github.com/ggshr9/shuttle/mesh/signal"
)

// MeshClient manages a mesh stream on the client side.
type MeshClient struct {
	stream    io.ReadWriteCloser
	mu        sync.Mutex // protects writes
	virtualIP net.IP
	meshNet   *net.IPNet

	// Split tunnel routes — protected by routeMu
	routeMu     sync.RWMutex
	splitRoutes []splitRoute

	// P2P support
	p2pEnabled    bool
	p2pManager    *p2p.Manager
	signalClient  *signal.Client
	fallbackCtrl  *p2p.FallbackController
	logger        *slog.Logger
}

// splitRoute is a parsed subnet-level routing policy.
type splitRoute struct {
	cidr   *net.IPNet
	action string // "mesh", "direct", "proxy"
}

// MeshClientConfig configures the mesh client.
type MeshClientConfig struct {
	P2PEnabled          bool
	STUNServers         []string
	HolePunchTimeout    time.Duration
	DirectRetryInterval time.Duration
	KeepAliveInterval   time.Duration
	FallbackThreshold   float64
	LocalPrivateKey     [32]byte
	LocalPublicKey      [32]byte
	Logger              *slog.Logger

	// Port spoofing for bypassing firewalls
	// Use "dns" for port 53, "https" for port 443, or a custom port number
	SpoofMode string
	SpoofPort int // Custom port when SpoofMode is "custom"

	// UPnP/NAT-PMP configuration
	// By default, port mapping is auto-enabled for best NAT traversal
	EnableUPnP    bool // Deprecated: UPnP is auto-enabled by default
	DisableUPnP   bool // Set to true to disable UPnP/NAT-PMP auto-detection
	PreferredPort int  // Preferred external port (0 = same as local)
}

// NewMeshClient opens a mesh stream, performs the handshake, and returns a ready client.
func NewMeshClient(ctx context.Context, openStream func(ctx context.Context) (io.ReadWriteCloser, error)) (*MeshClient, error) {
	return NewMeshClientWithConfig(ctx, openStream, nil)
}

// NewMeshClientWithConfig opens a mesh stream with custom configuration.
func NewMeshClientWithConfig(ctx context.Context, openStream func(ctx context.Context) (io.ReadWriteCloser, error), cfg *MeshClientConfig) (*MeshClient, error) {
	stream, err := openStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("mesh: open stream: %w", err)
	}

	// Send magic
	if _, err := stream.Write([]byte(MeshMagic)); err != nil {
		stream.Close()
		return nil, fmt.Errorf("mesh: send magic: %w", err)
	}

	// Read handshake response
	var buf [HandshakeSize]byte
	if _, err := io.ReadFull(stream, buf[:]); err != nil {
		stream.Close()
		return nil, fmt.Errorf("mesh: read handshake: %w", err)
	}

	ip, maskIP, _, err := DecodeHandshake(buf[:])
	if err != nil {
		stream.Close()
		return nil, err
	}

	meshNet := &net.IPNet{
		IP:   ip.Mask(net.IPMask(maskIP)),
		Mask: net.IPMask(maskIP),
	}

	mc := &MeshClient{
		stream:    stream,
		virtualIP: ip,
		meshNet:   meshNet,
	}

	// Initialize P2P if enabled
	if cfg != nil && cfg.P2PEnabled {
		mc.p2pEnabled = true
		mc.logger = cfg.Logger
		if mc.logger == nil {
			mc.logger = slog.Default()
		}

		// Create signal client
		mc.signalClient = signal.NewClient(ip, stream, mc.logger)

		// Create fallback controller
		fallbackCfg := &p2p.FallbackConfig{
			HolePunchTimeout:    cfg.HolePunchTimeout,
			DirectRetryInterval: cfg.DirectRetryInterval,
			LossThreshold:       cfg.FallbackThreshold,
		}
		mc.fallbackCtrl = p2p.NewFallbackController(fallbackCfg, mc.logger)

		// Create P2P manager with optional port spoofing
		var spoofCfg *p2p.SpoofConfig
		if cfg.SpoofMode != "" {
			mode := p2p.ParseSpoofMode(cfg.SpoofMode)
			spoofCfg = &p2p.SpoofConfig{
				Mode:       mode,
				CustomPort: cfg.SpoofPort,
			}
			mc.logger.Info("mesh: port spoofing configured", "mode", mode, "port", spoofCfg.GetPort())
		}

		p2pCfg := &p2p.Config{
			LocalVIP:            ip,
			LocalPrivateKey:     cfg.LocalPrivateKey,
			LocalPublicKey:      cfg.LocalPublicKey,
			STUNServers:         cfg.STUNServers,
			HolePunchTimeout:    cfg.HolePunchTimeout,
			DirectRetryInterval: cfg.DirectRetryInterval,
			KeepAliveInterval:   cfg.KeepAliveInterval,
			RelayFunc:           mc.sendViaRelay,
			SpoofConfig:         spoofCfg,
			DisableUPnP:         cfg.DisableUPnP,
			PreferredPort:       cfg.PreferredPort,
		}

		mc.p2pManager, err = p2p.NewManager(p2pCfg, mc.signalClient, mc.logger)
		if err != nil {
			mc.logger.Warn("mesh: failed to create P2P manager", "err", err)
			mc.p2pEnabled = false
		} else {
			if err := mc.p2pManager.Start(); err != nil {
				mc.logger.Warn("mesh: failed to start P2P manager", "err", err)
				mc.p2pEnabled = false
			} else {
				mc.logger.Info("mesh: P2P enabled", "vip", ip)
			}
		}
	}

	return mc, nil
}

// VirtualIP returns the assigned virtual IP.
func (mc *MeshClient) VirtualIP() net.IP { return mc.virtualIP }

// MeshCIDR returns the mesh network CIDR string (e.g. "10.7.0.0/24").
func (mc *MeshClient) MeshCIDR() string { return mc.meshNet.String() }

// SetSplitRoutes configures per-subnet routing policies.
func (mc *MeshClient) SetSplitRoutes(routes []struct{ CIDR, Action string }) {
	parsed := make([]splitRoute, 0, len(routes))
	for _, r := range routes {
		_, cidr, err := net.ParseCIDR(r.CIDR)
		if err != nil {
			if mc.logger != nil {
				mc.logger.Warn("mesh: invalid split route CIDR", "cidr", r.CIDR, "err", err)
			}
			continue
		}
		parsed = append(parsed, splitRoute{cidr: cidr, action: r.Action})
	}
	mc.routeMu.Lock()
	mc.splitRoutes = parsed
	mc.routeMu.Unlock()
}

// RouteMesh returns the routing action for an IP within the mesh network.
// Returns "mesh" if the IP should be sent via mesh, "direct" or "proxy" otherwise.
// If no split route matches, defaults to "mesh" for mesh IPs.
func (mc *MeshClient) RouteMesh(ip net.IP) string {
	// Check split routes first (more specific takes priority)
	mc.routeMu.RLock()
	routes := mc.splitRoutes
	mc.routeMu.RUnlock()
	for _, r := range routes {
		if r.cidr.Contains(ip) {
			return r.action
		}
	}
	// Default: mesh for mesh destinations
	if mc.meshNet.Contains(ip) {
		return "mesh"
	}
	return ""
}

// IsMeshDestination reports whether the given IP should be routed via mesh.
// Respects split route policies: IPs in a "direct" or "proxy" split route are excluded.
func (mc *MeshClient) IsMeshDestination(ip net.IP) bool {
	if !mc.meshNet.Contains(ip) {
		return false
	}
	action := mc.RouteMesh(ip)
	return action == "mesh"
}

// Send writes a raw IPv4 packet to the mesh.
// If P2P is enabled, tries direct connection first, then falls back to relay.
func (mc *MeshClient) Send(pkt []byte) error {
	if len(pkt) < 20 {
		return fmt.Errorf("mesh: packet too short")
	}

	// Extract destination IP from IPv4 header
	dstIP := net.IP(pkt[16:20])

	// Try P2P first if enabled
	if mc.p2pEnabled && mc.p2pManager != nil {
		decision := mc.fallbackCtrl.GetDecision(dstIP)

		if decision == p2p.DecisionDirect {
			start := time.Now()
			err := mc.p2pManager.SendPacket(dstIP, pkt)
			if err == nil {
				mc.fallbackCtrl.RecordSuccess(dstIP, time.Since(start))
				return nil
			}

			// Record failure and get new decision
			newDecision := mc.fallbackCtrl.RecordFailure(dstIP)
			if newDecision == p2p.DecisionDirect {
				// Still try relay this time but don't switch permanently
				return mc.sendViaRelay(pkt)
			}
		}

		// Use relay
		return mc.sendViaRelay(pkt)
	}

	// No P2P, use relay directly
	return mc.sendViaRelay(pkt)
}

// sendViaRelay sends packet through the server relay.
func (mc *MeshClient) sendViaRelay(pkt []byte) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return WriteFrame(mc.stream, pkt)
}

// Receive reads the next raw IPv4 packet from the mesh stream.
func (mc *MeshClient) Receive() ([]byte, error) {
	return ReadFrame(mc.stream)
}

// Close closes the underlying stream and P2P manager.
func (mc *MeshClient) Close() error {
	if mc.p2pManager != nil {
		if err := mc.p2pManager.Stop(); err != nil {
			mc.logger.Debug("p2p manager stop error", "err", err)
		}
	}
	return mc.stream.Close()
}

// SendPacket implements proxy.MeshPacketHandler by delegating to Send.
func (mc *MeshClient) SendPacket(pkt []byte) error { return mc.Send(pkt) }

// ReceivePacket implements proxy.MeshPacketHandler by delegating to Receive.
func (mc *MeshClient) ReceivePacket() ([]byte, error) { return mc.Receive() }

// ConnectPeer initiates a P2P connection to a peer.
func (mc *MeshClient) ConnectPeer(ctx context.Context, peerVIP net.IP) error {
	if !mc.p2pEnabled || mc.p2pManager == nil {
		return fmt.Errorf("mesh: P2P not enabled")
	}
	return mc.p2pManager.Connect(ctx, peerVIP)
}

// GetPeerState returns the connection state for a peer.
func (mc *MeshClient) GetPeerState(peerVIP net.IP) p2p.ConnectionState {
	if !mc.p2pEnabled || mc.p2pManager == nil {
		return p2p.StateDisconnected
	}
	return mc.p2pManager.GetPeerState(peerVIP)
}

// GetP2PStats returns P2P connection statistics.
func (mc *MeshClient) GetP2PStats() *p2p.ManagerStats {
	if !mc.p2pEnabled || mc.p2pManager == nil {
		return &p2p.ManagerStats{}
	}
	return mc.p2pManager.GetStats()
}

// ListPeers returns information about all known peers with quality metrics.
func (mc *MeshClient) ListPeers() []p2p.PeerInfo {
	if !mc.p2pEnabled || mc.p2pManager == nil {
		return nil
	}
	return mc.p2pManager.ListPeers()
}

// IsP2PEnabled returns whether P2P is enabled.
func (mc *MeshClient) IsP2PEnabled() bool {
	return mc.p2pEnabled
}

// GetLocalCandidates returns the local ICE candidates.
func (mc *MeshClient) GetLocalCandidates() []*p2p.Candidate {
	if !mc.p2pEnabled || mc.p2pManager == nil {
		return nil
	}
	return mc.p2pManager.LocalCandidates()
}
