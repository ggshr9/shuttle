package engine

import (
	"context"
	"fmt"
	"net"

	"github.com/ggshr9/shuttle/adapter"
)

// MeshOutbound routes connections through the mesh network.
// It dials the destination directly; the TUN device's packet handler
// intercepts packets destined for mesh IPs via MeshHandler.IsMeshDestination().
type MeshOutbound struct {
	tag         string
	meshManager *MeshManager
}

// NewMeshOutbound creates a MeshOutbound that uses the given MeshManager.
func NewMeshOutbound(tag string, mm *MeshManager) *MeshOutbound {
	return &MeshOutbound{tag: tag, meshManager: mm}
}

func (m *MeshOutbound) Tag() string  { return m.tag }
func (m *MeshOutbound) Type() string { return "mesh" }
func (m *MeshOutbound) Close() error { return nil }

// DialContext dials the address directly. Mesh VIPs are routable only when TUN
// is active because MeshManager.Start adds the mesh CIDR route to the TUN device,
// causing the OS to route mesh-destined packets through TUN where MeshHandler
// intercepts them at the packet level. For non-TUN inbounds (SOCKS5/HTTP),
// connections to mesh VIPs will fail because the VIPs are not routable outside
// the TUN path — this is a design limitation of packet-level mesh routing.
func (m *MeshOutbound) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if m.meshManager == nil || m.meshManager.Client() == nil {
		return nil, fmt.Errorf("mesh not connected")
	}
	return (&net.Dialer{}).DialContext(ctx, network, address)
}

// Compile-time interface check.
var _ adapter.Outbound = (*MeshOutbound)(nil)
