package mesh

import (
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
)

// IPAllocator allocates virtual IPs from a CIDR range.
// .0 is reserved (network), .1 is the gateway, .255 is broadcast.
type IPAllocator struct {
	mu      sync.Mutex
	network *net.IPNet
	gateway net.IP
	next    uint32
	max     uint32
	used    map[uint32]bool
}

// NewIPAllocator creates an allocator from a CIDR string (e.g. "10.7.0.0/24").
func NewIPAllocator(cidr string) (*IPAllocator, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("mesh: parse CIDR %q: %w", cidr, err)
	}

	base := binary.BigEndian.Uint32(ipNet.IP.To4())
	ones, bits := ipNet.Mask.Size()
	size := uint32(1) << uint(bits-ones)

	gateway := make(net.IP, 4)
	binary.BigEndian.PutUint32(gateway, base+1)

	return &IPAllocator{
		network: ipNet,
		gateway: gateway,
		next:    2, // start from .2 (skip .0 network and .1 gateway)
		max:     size - 1, // exclude broadcast
		used:    make(map[uint32]bool),
	}, nil
}

// Allocate returns the next available virtual IP.
func (a *IPAllocator) Allocate() (net.IP, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	base := binary.BigEndian.Uint32(a.network.IP.To4())
	const start = uint32(2) // first allocatable offset

	// Search forward from a.next, then wrap around from start.
	for offset := a.next; offset < a.max; offset++ {
		if !a.used[offset] {
			a.used[offset] = true
			a.next = offset + 1
			ip := make(net.IP, 4)
			binary.BigEndian.PutUint32(ip, base+offset)
			return ip, nil
		}
	}
	for offset := start; offset < a.next; offset++ {
		if !a.used[offset] {
			a.used[offset] = true
			a.next = offset + 1
			ip := make(net.IP, 4)
			binary.BigEndian.PutUint32(ip, base+offset)
			return ip, nil
		}
	}
	return nil, fmt.Errorf("mesh: IP pool exhausted")
}

// Release returns an IP to the pool.
func (a *IPAllocator) Release(ip net.IP) {
	a.mu.Lock()
	defer a.mu.Unlock()

	ip4 := ip.To4()
	if ip4 == nil {
		return
	}
	base := binary.BigEndian.Uint32(a.network.IP.To4())
	offset := binary.BigEndian.Uint32(ip4) - base
	delete(a.used, offset)
	if offset < a.next {
		a.next = offset
	}
}

// Gateway returns the gateway IP for this mesh network.
func (a *IPAllocator) Gateway() net.IP { return a.gateway }

// Network returns the mesh IPNet.
func (a *IPAllocator) Network() *net.IPNet { return a.network }

// Mask returns the network mask as an IP (for handshake encoding).
func (a *IPAllocator) Mask() net.IP {
	return net.IP(a.network.Mask)
}

// PeerTable maps virtual IPs to their mesh streams and handles forwarding.
type PeerTable struct {
	mu     sync.RWMutex
	peers  map[[4]byte]io.WriteCloser
	logger *slog.Logger
}

// NewPeerTable creates a new peer table.
func NewPeerTable(logger *slog.Logger) *PeerTable {
	return &PeerTable{
		peers:  make(map[[4]byte]io.WriteCloser),
		logger: logger,
	}
}

// Register adds a peer with its virtual IP.
func (pt *PeerTable) Register(ip net.IP, w io.WriteCloser) {
	var key [4]byte
	copy(key[:], ip.To4())
	pt.mu.Lock()
	pt.peers[key] = w
	pt.mu.Unlock()
	pt.logger.Info("mesh peer registered", "ip", ip)
}

// Unregister removes a peer.
func (pt *PeerTable) Unregister(ip net.IP) {
	var key [4]byte
	copy(key[:], ip.To4())
	pt.mu.Lock()
	delete(pt.peers, key)
	pt.mu.Unlock()
	pt.logger.Info("mesh peer unregistered", "ip", ip)
}

// Forward sends a packet to the peer owning dstIP. Returns false if no such peer.
func (pt *PeerTable) Forward(pkt []byte) bool {
	if len(pkt) < 20 {
		return false
	}
	var dstIP [4]byte
	copy(dstIP[:], pkt[16:20])

	pt.mu.RLock()
	w, ok := pt.peers[dstIP]
	if !ok {
		pt.mu.RUnlock()
		return false
	}
	// Write under RLock to prevent Unregister from closing the writer
	// while we're writing. The writer itself has its own mutex for
	// serializing concurrent Forward calls.
	_, err := w.Write(pkt)
	pt.mu.RUnlock()

	if err != nil {
		pt.logger.Debug("mesh forward error", "dst", net.IP(dstIP[:]), "err", err)
		return false
	}
	return true
}
