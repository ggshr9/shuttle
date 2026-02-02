package mesh

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
)

// MeshClient manages a mesh stream on the client side.
type MeshClient struct {
	stream   io.ReadWriteCloser
	mu       sync.Mutex // protects writes
	virtualIP net.IP
	meshNet  *net.IPNet
}

// NewMeshClient opens a mesh stream, performs the handshake, and returns a ready client.
func NewMeshClient(ctx context.Context, openStream func(ctx context.Context) (io.ReadWriteCloser, error)) (*MeshClient, error) {
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

	return &MeshClient{
		stream:    stream,
		virtualIP: ip,
		meshNet:   meshNet,
	}, nil
}

// VirtualIP returns the assigned virtual IP.
func (mc *MeshClient) VirtualIP() net.IP { return mc.virtualIP }

// MeshCIDR returns the mesh network CIDR string (e.g. "10.7.0.0/24").
func (mc *MeshClient) MeshCIDR() string { return mc.meshNet.String() }

// IsMeshDestination reports whether the given IP belongs to the mesh network.
func (mc *MeshClient) IsMeshDestination(ip net.IP) bool {
	return mc.meshNet.Contains(ip)
}

// Send writes a raw IPv4 packet to the mesh stream (thread-safe).
func (mc *MeshClient) Send(pkt []byte) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return WriteFrame(mc.stream, pkt)
}

// Receive reads the next raw IPv4 packet from the mesh stream.
func (mc *MeshClient) Receive() ([]byte, error) {
	return ReadFrame(mc.stream)
}

// Close closes the underlying stream.
func (mc *MeshClient) Close() error {
	return mc.stream.Close()
}
