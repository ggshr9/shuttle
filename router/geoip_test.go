package router

import (
	"net"
	"sync"
	"testing"
)

func TestGeoIPDB_HotReload(t *testing.T) {
	db := NewGeoIPDB()

	// Load initial data: 10.0.0.0/8 → CN
	db.LoadFromCIDRs("CN", []string{"10.0.0.0/8"})
	if got := db.LookupCountry(net.ParseIP("10.1.2.3")); got != "CN" {
		t.Fatalf("before reload: expected CN, got %q", got)
	}

	// Reload with different data: 10.0.0.0/8 → US
	db.Reload([]GeoIPEntry{
		{CountryCode: "US", CIDRs: []string{"10.0.0.0/8"}},
	})

	if got := db.LookupCountry(net.ParseIP("10.1.2.3")); got != "US" {
		t.Fatalf("after reload: expected US, got %q", got)
	}

	// Old CN data should be gone
	if got := db.LookupCountry(net.ParseIP("10.1.2.3")); got == "CN" {
		t.Fatal("after reload: CN data should have been replaced")
	}
}

func TestGeoIPDB_ConcurrentReadDuringReload(t *testing.T) {
	db := NewGeoIPDB()
	db.LoadFromCIDRs("CN", []string{"10.0.0.0/8"})

	var wg sync.WaitGroup

	// 50 goroutines continuously reading
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				got := db.LookupCountry(net.ParseIP("10.1.2.3"))
				// Must be either CN or US, never empty (after initial load)
				if got != "CN" && got != "US" {
					t.Errorf("concurrent read got unexpected %q", got)
					return
				}
			}
		}()
	}

	// Reload multiple times concurrently with reads
	for i := 0; i < 10; i++ {
		db.Reload([]GeoIPEntry{
			{CountryCode: "US", CIDRs: []string{"10.0.0.0/8"}},
		})
		db.Reload([]GeoIPEntry{
			{CountryCode: "CN", CIDRs: []string{"10.0.0.0/8"}},
		})
	}

	wg.Wait()
}

func TestGeoIPDB_ReloadIPv6(t *testing.T) {
	db := NewGeoIPDB()
	db.LoadFromCIDRs("DE", []string{"2001:db8::/32"})

	if got := db.LookupCountry(net.ParseIP("2001:db8::1")); got != "DE" {
		t.Fatalf("before reload: expected DE, got %q", got)
	}

	db.Reload([]GeoIPEntry{
		{CountryCode: "FR", CIDRs: []string{"2001:db8::/32"}},
	})

	if got := db.LookupCountry(net.ParseIP("2001:db8::1")); got != "FR" {
		t.Fatalf("after reload: expected FR, got %q", got)
	}
}

func TestGeoSiteDB_HotReload(t *testing.T) {
	db := NewGeoSiteDB()

	db.LoadCategory("cn", []string{"baidu.com", "qq.com"})
	if domains := db.Lookup("cn"); len(domains) != 2 {
		t.Fatalf("before reload: expected 2 domains, got %d", len(domains))
	}

	// Reload with different data
	db.ReloadSites([]GeoSiteEntry{
		{Category: "cn", Domains: []string{"weibo.com"}},
		{Category: "ads", Domains: []string{"doubleclick.net"}},
	})

	cnDomains := db.Lookup("cn")
	if len(cnDomains) != 1 || cnDomains[0] != "weibo.com" {
		t.Fatalf("after reload cn: expected [weibo.com], got %v", cnDomains)
	}
	adsDomains := db.Lookup("ads")
	if len(adsDomains) != 1 || adsDomains[0] != "doubleclick.net" {
		t.Fatalf("after reload ads: expected [doubleclick.net], got %v", adsDomains)
	}

	// Old categories should reflect new state
	cats := db.Categories()
	if len(cats) != 2 {
		t.Fatalf("after reload: expected 2 categories, got %d: %v", len(cats), cats)
	}
}

func TestGeoSiteDB_ConcurrentReadDuringReload(t *testing.T) {
	db := NewGeoSiteDB()
	db.LoadCategory("cn", []string{"baidu.com"})

	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				domains := db.Lookup("cn")
				if len(domains) == 0 {
					t.Errorf("concurrent read got empty domains")
					return
				}
			}
		}()
	}

	for i := 0; i < 10; i++ {
		db.ReloadSites([]GeoSiteEntry{
			{Category: "cn", Domains: []string{"weibo.com"}},
		})
		db.ReloadSites([]GeoSiteEntry{
			{Category: "cn", Domains: []string{"baidu.com"}},
		})
	}

	wg.Wait()
}
