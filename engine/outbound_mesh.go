package engine

import (
	"context"
	"fmt"
	"net"

	"github.com/shuttleX/shuttle/adapter"
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

// DialContext dials the address directly. Mesh-destined packets are
// intercepted by the TUN device's MeshHandler at the packet level.
func (m *MeshOutbound) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if m.meshManager == nil || m.meshManager.Client() == nil {
		return nil, fmt.Errorf("mesh not connected")
	}
	return (&net.Dialer{}).DialContext(ctx, network, address)
}

// Compile-time interface check.
var _ adapter.Outbound = (*MeshOutbound)(nil)
