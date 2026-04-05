package proxy

import "net"

// MeshPacketHandler abstracts mesh packet send/receive for TUN integration.
// This interface decouples the TUN device from the concrete mesh.MeshClient type,
// enabling testing with mock implementations and avoiding import cycles.
type MeshPacketHandler interface {
	IsMeshDestination(ip net.IP) bool
	SendPacket(pkt []byte) error
	ReceivePacket() ([]byte, error)
	MeshCIDR() string
	Close() error
}
