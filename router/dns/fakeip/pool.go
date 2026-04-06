package fakeip

import (
	"fmt"
	"net/netip"
	"sync"
)

// PoolConfig holds the configuration for a fake-ip Pool.
type PoolConfig struct {
	CIDR   netip.Prefix // e.g. "198.18.0.0/15"
	Filter []string     // domain filter patterns (passed to NewFilter)
}

// Pool allocates virtual IPs for domains and supports reverse lookup.
// When the pool is exhausted, allocations wrap around (LRU eviction).
type Pool struct {
	prefix netip.Prefix
	start  netip.Addr // first usable address (network addr + 1)
	size   uint32     // number of usable addresses
	next   uint32     // next allocation index (wraps around)
	filter *Filter

	mu      sync.RWMutex
	forward map[string]netip.Addr // domain → fake IP
	reverse map[netip.Addr]string // fake IP → domain
}

// NewPool creates a Pool from cfg.
// Returns an error if the CIDR is invalid or has no usable addresses.
func NewPool(cfg PoolConfig) (*Pool, error) {
	prefix := cfg.CIDR.Masked()
	if !prefix.IsValid() {
		return nil, fmt.Errorf("fakeip: invalid CIDR %s", cfg.CIDR)
	}

	addr := prefix.Addr()
	if !addr.Is4() {
		return nil, fmt.Errorf("fakeip: only IPv4 CIDRs are supported, got %s", cfg.CIDR)
	}

	bits := prefix.Bits()
	if bits > 30 {
		return nil, fmt.Errorf("fakeip: CIDR %s is too small (need at least /30)", cfg.CIDR)
	}

	// total addresses in the prefix
	total := uint32(1) << (32 - bits)
	// usable = total - 2 (skip network address and broadcast address)
	usable := total - 2
	if usable == 0 {
		return nil, fmt.Errorf("fakeip: CIDR %s has no usable addresses", cfg.CIDR)
	}

	// start = network address + 1
	start := addr.Next()

	return &Pool{
		prefix:  prefix,
		start:   start,
		size:    usable,
		next:    0,
		filter:  NewFilter(cfg.Filter),
		forward: make(map[string]netip.Addr),
		reverse: make(map[netip.Addr]string),
	}, nil
}

// ShouldFakeIP returns true if the domain should be assigned a fake IP.
// Returns false for domains matched by the filter (i.e. they should bypass fake-ip).
func (p *Pool) ShouldFakeIP(domain string) bool {
	return !p.filter.ShouldSkip(domain)
}

// Lookup returns the fake IP assigned to domain, allocating one if needed.
// When the pool is full the next slot wraps around and the evicted mapping is removed.
func (p *Pool) Lookup(domain string) netip.Addr {
	p.mu.Lock()
	defer p.mu.Unlock()

	if ip, ok := p.forward[domain]; ok {
		return ip
	}

	// Allocate the next IP slot.
	ip := p.addrAt(p.next)

	// LRU eviction: if this IP was already mapped to another domain, remove the old entry.
	if old, occupied := p.reverse[ip]; occupied && old != domain {
		delete(p.forward, old)
	}

	p.forward[domain] = ip
	p.reverse[ip] = domain

	p.next = (p.next + 1) % p.size

	return ip
}

// Reverse looks up the domain that was assigned the given fake IP.
func (p *Pool) Reverse(ip netip.Addr) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	domain, ok := p.reverse[ip]
	return domain, ok
}

// IsFakeIP reports whether ip falls within the pool's CIDR range.
func (p *Pool) IsFakeIP(ip netip.Addr) bool {
	return p.prefix.Contains(ip)
}

// addrAt returns the usable address at index idx within the pool.
// idx must be in [0, size).
func (p *Pool) addrAt(idx uint32) netip.Addr {
	addr := p.start
	for i := uint32(0); i < idx; i++ {
		addr = addr.Next()
	}
	return addr
}
