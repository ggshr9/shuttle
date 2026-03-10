package router

import (
	"encoding/binary"
	"net"
	"sort"
	"sync"
)

// geoEntry represents a single IPv4 CIDR with its country code.
type geoEntry struct {
	start   uint32 // Network start address
	mask    uint32 // Bitmask derived from prefix length
	country string
}

// GeoIPDB provides IP-to-country lookups.
type GeoIPDB struct {
	mu sync.RWMutex

	// Sorted by start address for binary search (IPv4 only).
	ipv4Entries []geoEntry
	ipv4Sorted  bool

	// IPv6 entries kept as net.IPNet for simplicity (less common).
	ipv6Entries map[string][]*net.IPNet // country code → CIDRs
}

// NewGeoIPDB creates a new GeoIP database.
func NewGeoIPDB() *GeoIPDB {
	return &GeoIPDB{
		ipv6Entries: make(map[string][]*net.IPNet),
	}
}

// LoadFromCIDRs loads CIDR entries for a country.
func (db *GeoIPDB) LoadFromCIDRs(country string, cidrs []string) {
	db.mu.Lock()
	defer db.mu.Unlock()

	for _, cidr := range cidrs {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		// IPv4 → fast path with binary search
		if ip4 := ipnet.IP.To4(); ip4 != nil {
			start := binary.BigEndian.Uint32(ip4)
			ones, _ := ipnet.Mask.Size()
			mask := uint32(0)
			if ones > 0 {
				mask = ^uint32(0) << (32 - ones)
			}
			db.ipv4Entries = append(db.ipv4Entries, geoEntry{
				start:   start & mask,
				mask:    mask,
				country: country,
			})
			db.ipv4Sorted = false
		} else {
			// IPv6 — linear lookup
			db.ipv6Entries[country] = append(db.ipv6Entries[country], ipnet)
		}
	}
}

// ensureSorted sorts IPv4 entries by start address if needed.
func (db *GeoIPDB) ensureSorted() {
	if db.ipv4Sorted {
		return
	}
	sort.Slice(db.ipv4Entries, func(i, j int) bool {
		return db.ipv4Entries[i].start < db.ipv4Entries[j].start
	})
	db.ipv4Sorted = true
}

// LookupCountry returns the country code for an IP address.
func (db *GeoIPDB) LookupCountry(ip net.IP) string {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if ip4 := ip.To4(); ip4 != nil {
		return db.lookupIPv4(binary.BigEndian.Uint32(ip4))
	}
	return db.lookupIPv6(ip)
}

// lookupIPv4 uses binary search on sorted entries.
func (db *GeoIPDB) lookupIPv4(addr uint32) string {
	db.ensureSorted()
	entries := db.ipv4Entries
	n := len(entries)
	if n == 0 {
		return ""
	}

	// Find rightmost entry whose start <= addr
	idx := sort.Search(n, func(i int) bool {
		return entries[i].start > addr
	}) - 1

	// Check entries going backwards (in case of overlapping CIDRs, most specific wins)
	for i := idx; i >= 0; i-- {
		e := entries[i]
		if addr&e.mask == e.start {
			return e.country
		}
		// If the entry's start is far below, no earlier entry can match
		if e.start < addr&e.mask {
			break
		}
	}
	return ""
}

// lookupIPv6 uses linear scan (IPv6 usage is rare in this context).
func (db *GeoIPDB) lookupIPv6(ip net.IP) string {
	for country, cidrs := range db.ipv6Entries {
		for _, cidr := range cidrs {
			if cidr.Contains(ip) {
				return country
			}
		}
	}
	return ""
}

// GeoSiteDB provides domain category lookups.
type GeoSiteDB struct {
	mu         sync.RWMutex
	categories map[string][]string // category → domains
}

// NewGeoSiteDB creates a new GeoSite database.
func NewGeoSiteDB() *GeoSiteDB {
	return &GeoSiteDB{
		categories: make(map[string][]string),
	}
}

// LoadCategory loads domains for a category.
func (db *GeoSiteDB) LoadCategory(category string, domains []string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.categories[category] = domains
}

// Lookup returns all domains in a category.
func (db *GeoSiteDB) Lookup(category string) []string {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.categories[category]
}

// Categories returns a sorted list of all loaded category names.
func (db *GeoSiteDB) Categories() []string {
	db.mu.RLock()
	defer db.mu.RUnlock()
	cats := make([]string, 0, len(db.categories))
	for k := range db.categories {
		cats = append(cats, k)
	}
	sort.Strings(cats)
	return cats
}
