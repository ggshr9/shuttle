package geodata

import (
	"bufio"
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// GeoIPEntry represents a country-IP mapping.
type GeoIPEntry struct {
	CountryCode string
	CIDRs       []string
}

// GeoSiteEntry represents a domain category.
type GeoSiteEntry struct {
	Category string
	Domains  []string
}

// DefaultCNRanges contains approximate /8 CIDR ranges for China.
var DefaultCNRanges = []string{
	"1.0.0.0/8", "14.0.0.0/8", "27.0.0.0/8", "36.0.0.0/8", "39.0.0.0/8",
	"42.0.0.0/8", "49.0.0.0/8", "58.0.0.0/8", "59.0.0.0/8", "60.0.0.0/8",
	"61.0.0.0/8", "101.0.0.0/8", "103.0.0.0/8", "106.0.0.0/8", "110.0.0.0/8",
	"111.0.0.0/8", "112.0.0.0/8", "113.0.0.0/8", "114.0.0.0/8", "115.0.0.0/8",
	"116.0.0.0/8", "117.0.0.0/8", "118.0.0.0/8", "119.0.0.0/8", "120.0.0.0/8",
	"121.0.0.0/8", "122.0.0.0/8", "123.0.0.0/8", "124.0.0.0/8", "125.0.0.0/8",
	"175.0.0.0/8", "180.0.0.0/8", "182.0.0.0/8", "183.0.0.0/8", "202.0.0.0/8",
	"203.0.0.0/8", "210.0.0.0/8", "211.0.0.0/8", "218.0.0.0/8", "219.0.0.0/8",
	"220.0.0.0/8", "221.0.0.0/8", "222.0.0.0/8", "223.0.0.0/8",
}

// DefaultCNDomains contains common Chinese domains.
var DefaultCNDomains = []string{
	"baidu.com", "qq.com", "taobao.com", "tmall.com", "jd.com", "weibo.com",
	"bilibili.com", "zhihu.com", "douyin.com", "tiktok.com", "163.com",
	"sohu.com", "sina.com", "alipay.com", "douban.com", "csdn.net",
	"iqiyi.com", "youku.com", "meituan.com", "dianping.com",
}

// LoadDefaults returns the embedded default GeoIP and GeoSite data.
func LoadDefaults() ([]GeoIPEntry, []GeoSiteEntry) {
	geoIP := []GeoIPEntry{
		{CountryCode: "CN", CIDRs: append([]string(nil), DefaultCNRanges...)},
	}
	geoSite := []GeoSiteEntry{
		{Category: "cn", Domains: append([]string(nil), DefaultCNDomains...)},
	}
	return geoIP, geoSite
}

// ---------------------------------------------------------------------------
// GeoIP loading
// ---------------------------------------------------------------------------

// LoadGeoIPFromCSV loads GeoIP data from a MaxMind GeoLite2 Country CSV file.
// Expected columns: network, geoname_id, registered_country_geoname_id, ...
// A separate geoname-to-country mapping is not used here; instead the function
// accepts an optional geonameMap. If nil, geoname_id is used as the country code
// directly (useful when the CSV has already been pre-processed with country codes).
// For the standard MaxMind format you need to pair this with the
// GeoLite2-Country-Locations CSV to build the geoname map externally.
func LoadGeoIPFromCSV(path string) ([]GeoIPEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	// Read header
	header, err := r.Read()
	if err != nil {
		return nil, err
	}

	// Find column indices
	networkIdx := -1
	geonameIdx := -1
	regIdx := -1
	for i, col := range header {
		col = strings.TrimSpace(col)
		switch col {
		case "network":
			networkIdx = i
		case "geoname_id":
			geonameIdx = i
		case "registered_country_geoname_id":
			regIdx = i
		}
	}
	if networkIdx < 0 {
		return nil, &parseError{"network column not found in CSV header"}
	}

	countryMap := make(map[string][]string) // geoname_id -> []CIDR

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip malformed rows
		}

		network := ""
		if networkIdx < len(record) {
			network = strings.TrimSpace(record[networkIdx])
		}
		if network == "" {
			continue
		}

		// Use geoname_id first, fall back to registered_country_geoname_id
		geoID := ""
		if geonameIdx >= 0 && geonameIdx < len(record) {
			geoID = strings.TrimSpace(record[geonameIdx])
		}
		if geoID == "" && regIdx >= 0 && regIdx < len(record) {
			geoID = strings.TrimSpace(record[regIdx])
		}
		if geoID == "" {
			continue
		}

		countryMap[geoID] = append(countryMap[geoID], network)
	}

	entries := make([]GeoIPEntry, 0, len(countryMap))
	for code, cidrs := range countryMap {
		entries = append(entries, GeoIPEntry{CountryCode: code, CIDRs: cidrs})
	}
	return entries, nil
}

// LoadGeoIPFromText loads GeoIP data from a simple text file.
// Each line: "CC CIDR" (e.g., "CN 1.0.0.0/24"). Lines starting with # are comments.
func LoadGeoIPFromText(path string) ([]GeoIPEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return LoadGeoIPFromReader(f)
}

// LoadGeoIPFromReader reads GeoIP data in the simple text format from an io.Reader.
func LoadGeoIPFromReader(r io.Reader) ([]GeoIPEntry, error) {
	countryMap := make(map[string][]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		cc := strings.ToUpper(parts[0])
		cidr := parts[1]
		countryMap[cc] = append(countryMap[cc], cidr)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	entries := make([]GeoIPEntry, 0, len(countryMap))
	for cc, cidrs := range countryMap {
		entries = append(entries, GeoIPEntry{CountryCode: cc, CIDRs: cidrs})
	}
	return entries, nil
}

// ---------------------------------------------------------------------------
// GeoSite loading
// ---------------------------------------------------------------------------

// LoadGeoSiteFromDir loads all GeoSite category files from a directory.
// Each file name becomes the category name. "include:" directives are resolved
// relative to the same directory.
func LoadGeoSiteFromDir(dir string) ([]GeoSiteEntry, error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	// First pass: load all files
	rawCategories := make(map[string][]string)
	for _, de := range dirEntries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		// Skip hidden files and non-data files
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		category := name
		path := filepath.Join(dir, name)
		entry, err := LoadGeoSiteFromFile(path, category)
		if err != nil {
			continue // skip files that fail to parse
		}
		rawCategories[category] = entry.Domains
	}

	// Second pass: resolve "include:" references
	resolved := make(map[string]bool)
	var resolve func(cat string) []string
	resolve = func(cat string) []string {
		if resolved[cat] {
			return rawCategories[cat]
		}
		resolved[cat] = true

		domains := rawCategories[cat]
		var final []string
		for _, d := range domains {
			if strings.HasPrefix(d, "include:") {
				ref := strings.TrimPrefix(d, "include:")
				ref = strings.TrimSpace(ref)
				if included, ok := rawCategories[ref]; ok {
					final = append(final, resolve(ref)...)
					_ = included
				}
			} else {
				final = append(final, d)
			}
		}
		rawCategories[cat] = final
		return final
	}

	var entries []GeoSiteEntry
	for cat := range rawCategories {
		domains := resolve(cat)
		if len(domains) > 0 {
			entries = append(entries, GeoSiteEntry{Category: cat, Domains: domains})
		}
	}
	return entries, nil
}

// LoadGeoSiteFromFile loads a single GeoSite category file.
// Lines starting with # are comments. "include:" lines are kept as-is for later
// resolution by LoadGeoSiteFromDir. Domain type prefixes (full:, domain:, regexp:,
// keyword:) are stripped.
func LoadGeoSiteFromFile(path string, category string) (*GeoSiteEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return LoadGeoSiteFromReader(f, category)
}

// LoadGeoSiteFromReader reads GeoSite data from an io.Reader.
func LoadGeoSiteFromReader(r io.Reader, category string) (*GeoSiteEntry, error) {
	var domains []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip inline comments
		if idx := strings.Index(line, " #"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}

		// Keep include directives for resolution by LoadGeoSiteFromDir
		if strings.HasPrefix(line, "include:") {
			domains = append(domains, line)
			continue
		}

		// Strip @attr annotations (e.g., "@cn")
		if idx := strings.Index(line, "@"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}

		// Strip domain type prefixes
		for _, prefix := range []string{"full:", "domain:", "regexp:", "keyword:"} {
			if strings.HasPrefix(line, prefix) {
				line = strings.TrimPrefix(line, prefix)
				break
			}
		}

		line = strings.TrimSpace(line)
		if line != "" {
			domains = append(domains, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return &GeoSiteEntry{Category: category, Domains: domains}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type parseError struct {
	msg string
}

func (e *parseError) Error() string {
	return "geodata: " + e.msg
}
