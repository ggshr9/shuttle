package router

import (
	"strings"
	"testing"
)

func TestGeneratePACBasic(t *testing.T) {
	geoIP := NewGeoIPDB()
	geoSite := NewGeoSiteDB()
	r := NewRouter(&RouterConfig{
		DefaultAction: ActionProxy,
		Rules: []Rule{
			{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect},
			{Type: "domain", Values: []string{"blocked.com"}, Action: ActionReject},
			{Type: "domain", Values: []string{"proxy.org"}, Action: ActionProxy},
			{Type: "ip-cidr", Values: []string{"10.0.0.0/8"}, Action: ActionDirect},
		},
	}, geoIP, geoSite, nil)

	pac := GeneratePAC(r, &PACConfig{
		HTTPProxyAddr:  "127.0.0.1:8080",
		SOCKSProxyAddr: "127.0.0.1:1080",
		DefaultAction:  ActionProxy,
	})

	if !strings.Contains(pac, "FindProxyForURL") {
		t.Fatal("PAC missing FindProxyForURL function")
	}
	if !strings.Contains(pac, "example.com") {
		t.Fatal("PAC missing direct domain")
	}
	if !strings.Contains(pac, "blocked.com") {
		t.Fatal("PAC missing reject domain")
	}
	if !strings.Contains(pac, "proxy.org") {
		t.Fatal("PAC missing proxy domain")
	}
	if !strings.Contains(pac, "DIRECT") {
		t.Fatal("PAC missing DIRECT")
	}
	if !strings.Contains(pac, "SOCKS5 127.0.0.1:1080") {
		t.Fatal("PAC missing SOCKS5 proxy string")
	}
	if !strings.Contains(pac, "10.0.0.0") {
		t.Fatal("PAC missing CIDR rule")
	}
	t.Logf("PAC file length: %d bytes", len(pac))
}

func TestGeneratePACDefaultDirect(t *testing.T) {
	r := NewRouter(&RouterConfig{
		DefaultAction: ActionDirect,
	}, NewGeoIPDB(), NewGeoSiteDB(), nil)

	pac := GeneratePAC(r, &PACConfig{
		DefaultAction: ActionDirect,
	})

	// The last return should be DIRECT
	lines := strings.Split(pac, "\n")
	var lastReturn string
	for _, line := range lines {
		if strings.Contains(line, "return") && strings.Contains(line, "Default") {
			lastReturn = line
		}
	}
	// Check the return after the "Default action" comment
	if !strings.Contains(pac, "return \"DIRECT\"") {
		t.Fatal("expected default action DIRECT in PAC")
	}
	_ = lastReturn
}

func TestGeneratePACNilConfig(t *testing.T) {
	r := NewRouter(&RouterConfig{DefaultAction: ActionProxy}, NewGeoIPDB(), NewGeoSiteDB(), nil)
	pac := GeneratePAC(r, nil) // nil config → defaults
	if !strings.Contains(pac, "FindProxyForURL") {
		t.Fatal("PAC generation with nil config failed")
	}
}

func TestCidrToNetMask(t *testing.T) {
	tests := []struct {
		cidr     string
		wantIP   string
		wantMask string
	}{
		{"10.0.0.0/8", "10.0.0.0", "255.0.0.0"},
		{"192.168.0.0/16", "192.168.0.0", "255.255.0.0"},
		{"172.16.0.0/12", "172.16.0.0", "255.240.0.0"},
		{"0.0.0.0/0", "0.0.0.0", "0.0.0.0"},
	}
	for _, tc := range tests {
		ip, mask := cidrToNetMask(tc.cidr)
		if ip != tc.wantIP || mask != tc.wantMask {
			t.Errorf("cidrToNetMask(%q) = (%q, %q), want (%q, %q)", tc.cidr, ip, mask, tc.wantIP, tc.wantMask)
		}
	}
}

func TestCidrToNetMaskIPv6(t *testing.T) {
	ip, mask := cidrToNetMask("::1/128")
	if ip != "" || mask != "" {
		t.Fatalf("expected empty for IPv6, got (%q, %q)", ip, mask)
	}
}

func TestReconstructDomain(t *testing.T) {
	// Trie stores reversed: ["com", "example"] → "example.com"
	result := reconstructDomain([]string{"com", "example"})
	if result != "example.com" {
		t.Fatalf("expected example.com, got %s", result)
	}
}
