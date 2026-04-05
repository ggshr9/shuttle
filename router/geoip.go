package router

import (
	"encoding/binary"
	"net"
	"sort"
	"sync/atomic"
)

// geoEntry represents a single IPv4 CIDR with its country code.
type geoEntry struct {
	start   uint32 // Network start address
	mask    uint32 // Bitmask derived from prefix length
	country string
}

// geoIPSnapshot is an immutable point-in-time view of GeoIP data.
type geoIPSnapshot struct {
	ipv4Entries []geoEntry
	ipv4Sorted  bool
	ipv6Entries map[string][]*net.IPNet // country code → CIDRs
}

// GeoIPDB provides IP-to-country lookups using lock-free atomic reads.
type GeoIPDB struct {
	data atomic.Pointer[geoIPSnapshot]
}

// NewGeoIPDB creates a new GeoIP database.
func NewGeoIPDB() *GeoIPDB {
	db := &GeoIPDB{}
	db.data.Store(&geoIPSnapshot{
		ipv6Entries: make(map[string][]*net.IPNet),
	})
	return db
}

// LoadFromCIDRs loads CIDR entries for a country.
// It performs a copy-on-write update of the snapshot.
func (db *GeoIPDB) LoadFromCIDRs(country string, cidrs []string) {
	for {
		old := db.data.Load()

		// Copy IPv4 entries
		newIPv4 := make([]geoEntry, len(old.ipv4Entries))
		copy(newIPv4, old.ipv4Entries)

		// Deep copy IPv6 entries
		newIPv6 := make(map[string][]*net.IPNet, len(old.ipv6Entries))
		for k, v := range old.ipv6Entries {
			dst := make([]*net.IPNet, len(v))
			copy(dst, v)
			newIPv6[k] = dst
		}

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
				newIPv4 = append(newIPv4, geoEntry{
					start:   start & mask,
					mask:    mask,
					country: country,
				})
			} else {
				// IPv6 — linear lookup
				newIPv6[country] = append(newIPv6[country], ipnet)
			}
		}

		snap := &geoIPSnapshot{
			ipv4Entries: newIPv4,
			ipv4Sorted:  false,
			ipv6Entries: newIPv6,
		}

		if db.data.CompareAndSwap(old, snap) {
			return
		}
	}
}

// Reload atomically replaces all GeoIP data with the provided entries.
// Each entry contains a CountryCode and a slice of CIDR strings.
func (db *GeoIPDB) Reload(entries []GeoIPEntry) {
	snap := &geoIPSnapshot{
		ipv6Entries: make(map[string][]*net.IPNet),
	}
	for _, entry := range entries {
		for _, cidr := range entry.CIDRs {
			_, ipnet, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				start := binary.BigEndian.Uint32(ip4)
				ones, _ := ipnet.Mask.Size()
				mask := uint32(0)
				if ones > 0 {
					mask = ^uint32(0) << (32 - ones)
				}
				snap.ipv4Entries = append(snap.ipv4Entries, geoEntry{
					start:   start & mask,
					mask:    mask,
					country: entry.CountryCode,
				})
			} else {
				snap.ipv6Entries[entry.CountryCode] = append(snap.ipv6Entries[entry.CountryCode], ipnet)
			}
		}
	}
	sort.Slice(snap.ipv4Entries, func(i, j int) bool {
		return snap.ipv4Entries[i].start < snap.ipv4Entries[j].start
	})
	snap.ipv4Sorted = true
	db.data.Store(snap)
}

// GeoIPEntry represents a country code and its associated CIDRs for Reload().
// This avoids importing router/geodata from router.
type GeoIPEntry struct {
	CountryCode string
	CIDRs       []string
}

// LookupCountry returns the country code for an IP address.
func (db *GeoIPDB) LookupCountry(ip net.IP) string {
	snap := db.data.Load()

	if ip4 := ip.To4(); ip4 != nil {
		return lookupIPv4(snap, binary.BigEndian.Uint32(ip4))
	}
	return lookupIPv6(snap, ip)
}

// lookupIPv4 uses binary search on sorted entries.
func lookupIPv4(snap *geoIPSnapshot, addr uint32) string {
	if !snap.ipv4Sorted {
		// Snapshot was not pre-sorted (came from LoadFromCIDRs).
		// Sort a copy to avoid mutating the shared snapshot.
		sorted := make([]geoEntry, len(snap.ipv4Entries))
		copy(sorted, snap.ipv4Entries)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].start < sorted[j].start
		})
		// Create a new sorted snapshot and try to swap it in.
		// Even if the CAS fails (another goroutine did the same), this lookup
		// still works correctly with the local sorted copy.
		snap = &geoIPSnapshot{
			ipv4Entries: sorted,
			ipv4Sorted:  true,
			ipv6Entries: snap.ipv6Entries,
		}
	}

	entries := snap.ipv4Entries
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
func lookupIPv6(snap *geoIPSnapshot, ip net.IP) string {
	for country, cidrs := range snap.ipv6Entries {
		for _, cidr := range cidrs {
			if cidr.Contains(ip) {
				return country
			}
		}
	}
	return ""
}

// geoSiteSnapshot is an immutable point-in-time view of GeoSite data.
type geoSiteSnapshot struct {
	categories map[string][]string // category → domains
}

// GeoSiteDB provides domain category lookups using lock-free atomic reads.
type GeoSiteDB struct {
	data atomic.Pointer[geoSiteSnapshot]
}

// NewGeoSiteDB creates a new GeoSite database.
func NewGeoSiteDB() *GeoSiteDB {
	db := &GeoSiteDB{}
	db.data.Store(&geoSiteSnapshot{
		categories: make(map[string][]string),
	})
	return db
}

// LoadCategory loads domains for a category.
func (db *GeoSiteDB) LoadCategory(category string, domains []string) {
	for {
		old := db.data.Load()

		newCats := make(map[string][]string, len(old.categories)+1)
		for k, v := range old.categories {
			newCats[k] = v
		}
		newCats[category] = domains

		snap := &geoSiteSnapshot{categories: newCats}
		if db.data.CompareAndSwap(old, snap) {
			return
		}
	}
}

// ReloadSites atomically replaces all GeoSite data with the provided entries.
func (db *GeoSiteDB) ReloadSites(entries []GeoSiteEntry) {
	cats := make(map[string][]string, len(entries))
	for _, entry := range entries {
		cats[entry.Category] = entry.Domains
	}
	db.data.Store(&geoSiteSnapshot{categories: cats})
}

// GeoSiteEntry represents a category and its domains for ReloadSites().
// This avoids importing router/geodata from router.
type GeoSiteEntry struct {
	Category string
	Domains  []string
}

// Lookup returns all domains in a category.
func (db *GeoSiteDB) Lookup(category string) []string {
	snap := db.data.Load()
	return snap.categories[category]
}

// Categories returns a sorted list of all loaded category names.
func (db *GeoSiteDB) Categories() []string {
	snap := db.data.Load()
	cats := make([]string, 0, len(snap.categories))
	for k := range snap.categories {
		cats = append(cats, k)
	}
	sort.Strings(cats)
	return cats
}
