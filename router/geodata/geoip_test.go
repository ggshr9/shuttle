package geodata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	geoIP, geoSite := LoadDefaults()

	if len(geoIP) != 1 {
		t.Fatalf("expected 1 GeoIP entry, got %d", len(geoIP))
	}
	if geoIP[0].CountryCode != "CN" {
		t.Fatalf("expected country code CN, got %s", geoIP[0].CountryCode)
	}
	if len(geoIP[0].CIDRs) != len(DefaultCNRanges) {
		t.Fatalf("expected %d CIDRs, got %d", len(DefaultCNRanges), len(geoIP[0].CIDRs))
	}

	if len(geoSite) != 1 {
		t.Fatalf("expected 1 GeoSite entry, got %d", len(geoSite))
	}
	if geoSite[0].Category != "cn" {
		t.Fatalf("expected category cn, got %s", geoSite[0].Category)
	}
	if len(geoSite[0].Domains) != len(DefaultCNDomains) {
		t.Fatalf("expected %d domains, got %d", len(DefaultCNDomains), len(geoSite[0].Domains))
	}
}

func TestLoadDefaultsReturnsCopies(t *testing.T) {
	geoIP1, geoSite1 := LoadDefaults()
	geoIP2, geoSite2 := LoadDefaults()

	// Modifying the returned slices should not affect subsequent calls
	geoIP1[0].CIDRs[0] = "modified"
	if geoIP2[0].CIDRs[0] == "modified" {
		t.Fatal("LoadDefaults should return independent copies of CIDRs")
	}

	geoSite1[0].Domains[0] = "modified.com"
	if geoSite2[0].Domains[0] == "modified.com" {
		t.Fatal("LoadDefaults should return independent copies of Domains")
	}
}

func TestDefaultCNRangesAreValidCIDR(t *testing.T) {
	for _, cidr := range DefaultCNRanges {
		if !strings.Contains(cidr, "/") {
			t.Fatalf("expected CIDR notation with /, got %q", cidr)
		}
		parts := strings.Split(cidr, "/")
		if len(parts) != 2 {
			t.Fatalf("invalid CIDR format: %q", cidr)
		}
		if parts[1] != "8" {
			t.Fatalf("expected /8 prefix, got /%s in %q", parts[1], cidr)
		}
	}
}

func TestLoadGeoIPFromReader(t *testing.T) {
	input := `# Comment line
CN 1.0.0.0/24
CN 2.0.0.0/16
US 8.8.8.0/24
US 8.8.4.0/24

# Another comment
DE 5.0.0.0/8
`
	entries, err := LoadGeoIPFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadGeoIPFromReader: %v", err)
	}

	countryMap := make(map[string][]string)
	for _, e := range entries {
		countryMap[e.CountryCode] = e.CIDRs
	}

	if len(countryMap["CN"]) != 2 {
		t.Fatalf("expected 2 CN CIDRs, got %d", len(countryMap["CN"]))
	}
	if len(countryMap["US"]) != 2 {
		t.Fatalf("expected 2 US CIDRs, got %d", len(countryMap["US"]))
	}
	if len(countryMap["DE"]) != 1 {
		t.Fatalf("expected 1 DE CIDR, got %d", len(countryMap["DE"]))
	}
}

func TestLoadGeoIPFromReaderEmpty(t *testing.T) {
	entries, err := LoadGeoIPFromReader(strings.NewReader(""))
	if err != nil {
		t.Fatalf("LoadGeoIPFromReader empty: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for empty input, got %d", len(entries))
	}
}

func TestLoadGeoIPFromReaderOnlyComments(t *testing.T) {
	input := `# Comment 1
# Comment 2
`
	entries, err := LoadGeoIPFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadGeoIPFromReader comments only: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for comments-only input, got %d", len(entries))
	}
}

func TestLoadGeoIPFromReaderSkipsInvalidLines(t *testing.T) {
	input := `CN 1.0.0.0/8
singlefield
CN 2.0.0.0/8
`
	entries, err := LoadGeoIPFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadGeoIPFromReader: %v", err)
	}

	// The single-field line should be skipped
	for _, e := range entries {
		if e.CountryCode == "CN" {
			if len(e.CIDRs) != 2 {
				t.Fatalf("expected 2 CN CIDRs (invalid line skipped), got %d", len(e.CIDRs))
			}
			return
		}
	}
	t.Fatal("expected CN entry")
}

func TestLoadGeoIPFromReaderCountryCodeUpperCase(t *testing.T) {
	input := `cn 1.0.0.0/8
us 8.8.8.0/24
`
	entries, err := LoadGeoIPFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadGeoIPFromReader: %v", err)
	}

	for _, e := range entries {
		if e.CountryCode != strings.ToUpper(e.CountryCode) {
			t.Fatalf("country code should be uppercased, got %q", e.CountryCode)
		}
	}
}

func TestLoadGeoIPFromText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "geoip.txt")

	content := `CN 1.0.0.0/8
US 8.8.8.0/24
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	entries, err := LoadGeoIPFromText(path)
	if err != nil {
		t.Fatalf("LoadGeoIPFromText: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestLoadGeoIPFromTextNonexistent(t *testing.T) {
	_, err := LoadGeoIPFromText("/nonexistent/path/geoip.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadGeoIPFromCSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "geoip.csv")

	content := `network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider
1.0.0.0/24,2077456,,,,
1.0.1.0/24,1814991,,,,
8.8.8.0/24,6252001,,,,
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	entries, err := LoadGeoIPFromCSV(path)
	if err != nil {
		t.Fatalf("LoadGeoIPFromCSV: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("expected at least one entry from CSV")
	}

	// Verify that geoname IDs are used as country codes
	countryMap := make(map[string][]string)
	for _, e := range entries {
		countryMap[e.CountryCode] = e.CIDRs
	}

	if cidrs, ok := countryMap["2077456"]; !ok || len(cidrs) != 1 {
		t.Fatalf("expected 1 CIDR for geoname 2077456, got %v", cidrs)
	}
	if cidrs, ok := countryMap["1814991"]; !ok || len(cidrs) != 1 {
		t.Fatalf("expected 1 CIDR for geoname 1814991, got %v", cidrs)
	}
}

func TestLoadGeoIPFromCSVFallbackToRegistered(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "geoip.csv")

	// geoname_id is empty, should fall back to registered_country_geoname_id
	content := `network,geoname_id,registered_country_geoname_id
1.0.0.0/24,,2077456
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	entries, err := LoadGeoIPFromCSV(path)
	if err != nil {
		t.Fatalf("LoadGeoIPFromCSV: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].CountryCode != "2077456" {
		t.Fatalf("expected fallback country code 2077456, got %s", entries[0].CountryCode)
	}
}

func TestLoadGeoIPFromCSVMissingNetworkColumn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "geoip.csv")

	content := `geoname_id,registered_country_geoname_id
2077456,2077456
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	_, err := LoadGeoIPFromCSV(path)
	if err == nil {
		t.Fatal("expected error when network column is missing")
	}
}

func TestLoadGeoIPFromCSVNonexistent(t *testing.T) {
	_, err := LoadGeoIPFromCSV("/nonexistent/path/geoip.csv")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadGeoIPFromCSVEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "geoip.csv")

	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	_, err := LoadGeoIPFromCSV(path)
	if err == nil {
		t.Fatal("expected error for empty CSV (no header)")
	}
}

func TestLoadGeoSiteFromReader(t *testing.T) {
	input := `# Comment
domain:example.com
full:www.example.com
keyword:test
regexp:.*\.example\.org
plain.example.net
`
	entry, err := LoadGeoSiteFromReader(strings.NewReader(input), "test-category")
	if err != nil {
		t.Fatalf("LoadGeoSiteFromReader: %v", err)
	}

	if entry.Category != "test-category" {
		t.Fatalf("expected category test-category, got %s", entry.Category)
	}

	// All domain type prefixes should be stripped
	for _, d := range entry.Domains {
		for _, prefix := range []string{"domain:", "full:", "keyword:", "regexp:"} {
			if strings.HasPrefix(d, prefix) {
				t.Fatalf("domain %q still has prefix %s", d, prefix)
			}
		}
	}

	if len(entry.Domains) != 5 {
		t.Fatalf("expected 5 domains, got %d: %v", len(entry.Domains), entry.Domains)
	}
}

func TestLoadGeoSiteFromReaderWithInlineComments(t *testing.T) {
	input := `example.com # this is a comment
test.com # another comment
`
	entry, err := LoadGeoSiteFromReader(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("LoadGeoSiteFromReader: %v", err)
	}

	for _, d := range entry.Domains {
		if strings.Contains(d, "#") {
			t.Fatalf("domain %q still contains inline comment", d)
		}
	}
	if len(entry.Domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(entry.Domains))
	}
}

func TestLoadGeoSiteFromReaderWithAttributes(t *testing.T) {
	input := `example.com@cn
test.com@us
`
	entry, err := LoadGeoSiteFromReader(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("LoadGeoSiteFromReader: %v", err)
	}

	for _, d := range entry.Domains {
		if strings.Contains(d, "@") {
			t.Fatalf("domain %q still contains @attr annotation", d)
		}
	}
}

func TestLoadGeoSiteFromReaderKeepsIncludeDirectives(t *testing.T) {
	input := `example.com
include:other-category
test.com
`
	entry, err := LoadGeoSiteFromReader(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("LoadGeoSiteFromReader: %v", err)
	}

	foundInclude := false
	for _, d := range entry.Domains {
		if d == "include:other-category" {
			foundInclude = true
		}
	}
	if !foundInclude {
		t.Fatal("expected include directive to be preserved")
	}
}

func TestLoadGeoSiteFromReaderEmpty(t *testing.T) {
	entry, err := LoadGeoSiteFromReader(strings.NewReader(""), "empty")
	if err != nil {
		t.Fatalf("LoadGeoSiteFromReader: %v", err)
	}
	if len(entry.Domains) != 0 {
		t.Fatalf("expected 0 domains for empty input, got %d", len(entry.Domains))
	}
}

func TestLoadGeoSiteFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-category.txt")

	content := `example.com
test.com
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	entry, err := LoadGeoSiteFromFile(path, "test-category")
	if err != nil {
		t.Fatalf("LoadGeoSiteFromFile: %v", err)
	}
	if len(entry.Domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(entry.Domains))
	}
}

func TestLoadGeoSiteFromFileNonexistent(t *testing.T) {
	_, err := LoadGeoSiteFromFile("/nonexistent/path/test.txt", "test")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadGeoSiteFromDir(t *testing.T) {
	dir := t.TempDir()

	// Create category files
	if err := os.WriteFile(filepath.Join(dir, "cn"), []byte("baidu.com\nqq.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "us"), []byte("google.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := LoadGeoSiteFromDir(dir)
	if err != nil {
		t.Fatalf("LoadGeoSiteFromDir: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	catMap := make(map[string][]string)
	for _, e := range entries {
		catMap[e.Category] = e.Domains
	}

	if len(catMap["cn"]) != 2 {
		t.Fatalf("expected 2 cn domains, got %d", len(catMap["cn"]))
	}
	if len(catMap["us"]) != 1 {
		t.Fatalf("expected 1 us domain, got %d", len(catMap["us"]))
	}
}

func TestLoadGeoSiteFromDirWithInclude(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "base"), []byte("base.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extended"), []byte("include:base\nextended.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := LoadGeoSiteFromDir(dir)
	if err != nil {
		t.Fatalf("LoadGeoSiteFromDir: %v", err)
	}

	catMap := make(map[string][]string)
	for _, e := range entries {
		catMap[e.Category] = e.Domains
	}

	// "extended" should have both base.com and extended.com after include resolution
	extDomains := catMap["extended"]
	if len(extDomains) != 2 {
		t.Fatalf("expected 2 domains in extended (after include resolution), got %d: %v", len(extDomains), extDomains)
	}
}

func TestLoadGeoSiteFromDirSkipsHiddenFiles(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, ".hidden"), []byte("hidden.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_internal"), []byte("internal.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "visible"), []byte("visible.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := LoadGeoSiteFromDir(dir)
	if err != nil {
		t.Fatalf("LoadGeoSiteFromDir: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (hidden and _ files skipped), got %d", len(entries))
	}
	if entries[0].Category != "visible" {
		t.Fatalf("expected category 'visible', got %q", entries[0].Category)
	}
}

func TestLoadGeoSiteFromDirNonexistent(t *testing.T) {
	_, err := LoadGeoSiteFromDir("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestParseError(t *testing.T) {
	pe := &parseError{msg: "test error"}
	expected := "geodata: test error"
	if pe.Error() != expected {
		t.Fatalf("parseError.Error() = %q, want %q", pe.Error(), expected)
	}
}

func TestGeoIPEntryStruct(t *testing.T) {
	entry := GeoIPEntry{
		CountryCode: "US",
		CIDRs:       []string{"8.8.8.0/24", "8.8.4.0/24"},
	}
	if entry.CountryCode != "US" {
		t.Fatalf("CountryCode = %q, want \"US\"", entry.CountryCode)
	}
	if len(entry.CIDRs) != 2 {
		t.Fatalf("expected 2 CIDRs, got %d", len(entry.CIDRs))
	}
}

func TestGeoSiteEntryStruct(t *testing.T) {
	entry := GeoSiteEntry{
		Category: "test",
		Domains:  []string{"example.com", "test.com"},
	}
	if entry.Category != "test" {
		t.Fatalf("Category = %q, want \"test\"", entry.Category)
	}
	if len(entry.Domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(entry.Domains))
	}
}
