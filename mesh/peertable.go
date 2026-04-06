package mesh

import (
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
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
	size := uint32(1) << uint(bits-ones) //nolint:gosec // G115: bits-ones is 0-32 for IPv4, fits uint

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

// IPv6Allocator allocates virtual IPv6 addresses from a prefix.
// Designed for Unique Local Address (ULA) ranges like fd00::/8.
type IPv6Allocator struct {
	mu      sync.Mutex
	network *net.IPNet
	prefix  []byte // 8 or 16 bytes depending on prefix length
	next    uint64
	max     uint64
	used    map[uint64]bool
}

// NewIPv6Allocator creates an allocator from an IPv6 CIDR string.
// Recommended: use a /64 ULA prefix like "fd00:1234:5678:abcd::/64"
func NewIPv6Allocator(cidr string) (*IPv6Allocator, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("mesh: parse IPv6 CIDR %q: %w", cidr, err)
	}

	// Ensure it's IPv6
	if ipNet.IP.To4() != nil {
		return nil, fmt.Errorf("mesh: %q is not an IPv6 CIDR", cidr)
	}

	ones, _ := ipNet.Mask.Size()
	if ones > 64 {
		return nil, fmt.Errorf("mesh: IPv6 prefix must be /64 or shorter, got /%d", ones)
	}

	// Copy the prefix bytes
	prefix := make([]byte, 16)
	copy(prefix, ipNet.IP.To16())

	// For /64 prefix, we have 64 bits for host part
	// We'll use the lower 64 bits for allocation
	hostBits := 128 - ones
	var maxVal uint64
	if hostBits >= 64 {
		maxVal = ^uint64(0) // max uint64
	} else {
		maxVal = uint64(1)<<hostBits - 1
	}

	return &IPv6Allocator{
		network: ipNet,
		prefix:  prefix,
		next:    2, // skip ::0 (network) and ::1 (gateway)
		max:     maxVal,
		used:    make(map[uint64]bool),
	}, nil
}

// Allocate returns the next available IPv6 address.
func (a *IPv6Allocator) Allocate() (net.IP, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	const start = uint64(2) // first allocatable offset

	// Search forward from a.next, then wrap around
	for offset := a.next; offset < a.max && offset < a.next+10000; offset++ {
		if !a.used[offset] {
			a.used[offset] = true
			a.next = offset + 1
			return a.makeIP(offset), nil
		}
	}
	for offset := start; offset < a.next; offset++ {
		if !a.used[offset] {
			a.used[offset] = true
			a.next = offset + 1
			return a.makeIP(offset), nil
		}
	}
	return nil, fmt.Errorf("mesh: IPv6 pool exhausted")
}

// makeIP creates an IPv6 address from the prefix and offset.
func (a *IPv6Allocator) makeIP(offset uint64) net.IP {
	ip := make(net.IP, 16)
	copy(ip, a.prefix)

	// Set offset in lower 64 bits (overwrite, don't OR, to avoid prefix residue)
	ip[8] = byte(offset >> 56)
	ip[9] = byte(offset >> 48)
	ip[10] = byte(offset >> 40)
	ip[11] = byte(offset >> 32)
	ip[12] = byte(offset >> 24)
	ip[13] = byte(offset >> 16)
	ip[14] = byte(offset >> 8)
	ip[15] = byte(offset)

	return ip
}

// Release returns an IPv6 address to the pool.
func (a *IPv6Allocator) Release(ip net.IP) {
	a.mu.Lock()
	defer a.mu.Unlock()

	ip16 := ip.To16()
	if ip16 == nil {
		return
	}

	// Extract offset from lower 64 bits
	offset := uint64(ip16[8])<<56 | uint64(ip16[9])<<48 |
		uint64(ip16[10])<<40 | uint64(ip16[11])<<32 |
		uint64(ip16[12])<<24 | uint64(ip16[13])<<16 |
		uint64(ip16[14])<<8 | uint64(ip16[15])

	delete(a.used, offset)
	if offset < a.next {
		a.next = offset
	}
}

// Gateway returns the gateway IPv6 address (::1 in the prefix).
func (a *IPv6Allocator) Gateway() net.IP {
	return a.makeIP(1)
}

// Network returns the mesh IPv6 network.
func (a *IPv6Allocator) Network() *net.IPNet { return a.network }

// DualStackAllocator manages both IPv4 and IPv6 address allocation.
type DualStackAllocator struct {
	v4 *IPAllocator
	v6 *IPv6Allocator
}

// NewDualStackAllocator creates an allocator for both IPv4 and IPv6.
// v4CIDR example: "10.7.0.0/24"
// v6CIDR example: "fd00:7::/64"
func NewDualStackAllocator(v4CIDR, v6CIDR string) (*DualStackAllocator, error) {
	v4, err := NewIPAllocator(v4CIDR)
	if err != nil {
		return nil, err
	}

	v6, err := NewIPv6Allocator(v6CIDR)
	if err != nil {
		return nil, err
	}

	return &DualStackAllocator{v4: v4, v6: v6}, nil
}

// AllocateV4 returns the next available IPv4 address.
func (d *DualStackAllocator) AllocateV4() (net.IP, error) {
	return d.v4.Allocate()
}

// AllocateV6 returns the next available IPv6 address.
func (d *DualStackAllocator) AllocateV6() (net.IP, error) {
	return d.v6.Allocate()
}

// AllocateDualStack returns both IPv4 and IPv6 addresses.
func (d *DualStackAllocator) AllocateDualStack() (v4, v6 net.IP, err error) {
	v4, err = d.v4.Allocate()
	if err != nil {
		return nil, nil, err
	}
	v6, err = d.v6.Allocate()
	if err != nil {
		d.v4.Release(v4) // rollback v4 allocation
		return nil, nil, err
	}
	return v4, v6, nil
}

// ReleaseV4 returns an IPv4 address to the pool.
func (d *DualStackAllocator) ReleaseV4(ip net.IP) {
	d.v4.Release(ip)
}

// ReleaseV6 returns an IPv6 address to the pool.
func (d *DualStackAllocator) ReleaseV6(ip net.IP) {
	d.v6.Release(ip)
}

// GatewayV4 returns the IPv4 gateway.
func (d *DualStackAllocator) GatewayV4() net.IP { return d.v4.Gateway() }

// GatewayV6 returns the IPv6 gateway.
func (d *DualStackAllocator) GatewayV6() net.IP { return d.v6.Gateway() }

// NetworkV4 returns the IPv4 network.
func (d *DualStackAllocator) NetworkV4() *net.IPNet { return d.v4.Network() }

// NetworkV6 returns the IPv6 network.
func (d *DualStackAllocator) NetworkV6() *net.IPNet { return d.v6.Network() }

// vipKey converts a net.IP virtual IP to a netip.Addr map key.
// IPv4-mapped IPv6 addresses are normalized to plain IPv4 so that
// net.IPv4(a,b,c,d) and its 16-byte form produce the same key.
func vipKey(ip net.IP) netip.Addr {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}
	}
	return addr.Unmap()
}

// PeerTable maps virtual IPs to their mesh streams and handles forwarding.
type PeerTable struct {
	mu     sync.RWMutex
	peers  map[netip.Addr]io.WriteCloser
	logger *slog.Logger
}

// NewPeerTable creates a new peer table.
func NewPeerTable(logger *slog.Logger) *PeerTable {
	return &PeerTable{
		peers:  make(map[netip.Addr]io.WriteCloser),
		logger: logger,
	}
}

// Register adds a peer with its virtual IP.
func (pt *PeerTable) Register(ip net.IP, w io.WriteCloser) {
	key := vipKey(ip)
	pt.mu.Lock()
	pt.peers[key] = w
	pt.mu.Unlock()
	pt.logger.Info("mesh peer registered", "ip", ip)
}

// Unregister removes a peer.
func (pt *PeerTable) Unregister(ip net.IP) {
	key := vipKey(ip)
	pt.mu.Lock()
	delete(pt.peers, key)
	pt.mu.Unlock()
	pt.logger.Info("mesh peer unregistered", "ip", ip)
}

// Forward sends a packet to the peer owning dstIP. Returns false if no such peer.
// Supports both IPv4 (version 4) and IPv6 (version 6) packets.
func (pt *PeerTable) Forward(pkt []byte) bool {
	if len(pkt) < 1 {
		return false
	}

	var dstKey netip.Addr
	version := pkt[0] >> 4
	switch version {
	case 4:
		// IPv4: minimum header 20 bytes, dst at bytes 16-20
		if len(pkt) < 20 {
			return false
		}
		dstKey = netip.AddrFrom4([4]byte(pkt[16:20]))
	case 6:
		// IPv6: fixed 40-byte header, dst at bytes 24-40
		if len(pkt) < 40 {
			return false
		}
		dstKey = netip.AddrFrom16([16]byte(pkt[24:40]))
	default:
		return false
	}

	pt.mu.RLock()
	w, ok := pt.peers[dstKey]
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
		pt.logger.Debug("mesh forward error", "dst", dstKey, "err", err)
		return false
	}
	return true
}
