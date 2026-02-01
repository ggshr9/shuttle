package router

import (
	"net"
	"sync"
)

// GeoIPDB provides IP-to-country lookups.
type GeoIPDB struct {
	mu      sync.RWMutex
	entries map[string][]*net.IPNet // country code → CIDRs
}

// NewGeoIPDB creates a new GeoIP database.
func NewGeoIPDB() *GeoIPDB {
	return &GeoIPDB{
		entries: make(map[string][]*net.IPNet),
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
		db.entries[country] = append(db.entries[country], ipnet)
	}
}

// LookupCountry returns the country code for an IP address.
func (db *GeoIPDB) LookupCountry(ip net.IP) string {
	db.mu.RLock()
	defer db.mu.RUnlock()

	for country, cidrs := range db.entries {
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
