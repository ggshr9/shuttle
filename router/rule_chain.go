package router

import (
	"fmt"
	"net"
	"strings"

	"github.com/shuttleX/shuttle/provider"
)

// MatchContext holds per-connection fields for rule chain matching.
type MatchContext struct {
	Domain      string
	IP          net.IP
	Process     string
	Protocol    string
	NetworkType string
}

// Matcher evaluates a single match condition against a MatchContext.
type Matcher interface {
	Match(ctx *MatchContext) bool
}

type logicOp int

const (
	logicAnd logicOp = iota
	logicOr
)

// compiledRule is a pre-compiled rule chain entry.
type compiledRule struct {
	matchers []Matcher
	logic    logicOp
	action   Action
}

// Match evaluates all matchers using the configured logic (AND/OR).
func (r *compiledRule) Match(ctx *MatchContext) bool {
	if len(r.matchers) == 0 {
		return false
	}
	if r.logic == logicOr {
		for _, m := range r.matchers {
			if m.Match(ctx) {
				return true
			}
		}
		return false
	}
	// logicAnd (default)
	for _, m := range r.matchers {
		if !m.Match(ctx) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// RuleChainConfig types (used by RouterConfig to avoid importing config pkg)
// ---------------------------------------------------------------------------

// RuleChainEntry defines a single rule in the ordered rule chain.
type RuleChainEntry struct {
	Match  RuleMatch
	Logic  string // "and" (default) | "or"
	Action string
}

// RuleMatch defines match conditions for a rule chain entry.
type RuleMatch struct {
	Domain        []string
	DomainSuffix  []string
	DomainKeyword []string
	GeoSite       []string
	IPCIDR        []string
	GeoIP         []string
	Process       []string
	Protocol      []string
	NetworkType   []string
	RuleProvider  []string
}

// ---------------------------------------------------------------------------
// Matchers
// ---------------------------------------------------------------------------

// domainExactMatcher matches exact domain names (case-insensitive).
type domainExactMatcher struct {
	domains map[string]struct{} // lower-cased
}

func newDomainExactMatcher(domains []string) *domainExactMatcher {
	m := &domainExactMatcher{domains: make(map[string]struct{}, len(domains))}
	for _, d := range domains {
		m.domains[strings.ToLower(d)] = struct{}{}
	}
	return m
}

func (m *domainExactMatcher) Match(ctx *MatchContext) bool {
	if ctx.Domain == "" {
		return false
	}
	_, ok := m.domains[strings.ToLower(ctx.Domain)]
	return ok
}

// domainSuffixMatcher uses a DomainTrie for suffix matching.
type domainSuffixMatcher struct {
	trie *DomainTrie
}

func newDomainSuffixMatcher(suffixes []string) *domainSuffixMatcher {
	trie := NewDomainTrie()
	for _, s := range suffixes {
		// Insert as wildcard so subdomains match
		lower := strings.ToLower(s)
		trie.Insert("+."+lower, "match")
		// Also match the exact domain itself
		trie.Insert(lower, "match")
	}
	return &domainSuffixMatcher{trie: trie}
}

func (m *domainSuffixMatcher) Match(ctx *MatchContext) bool {
	if ctx.Domain == "" {
		return false
	}
	_, found := m.trie.Lookup(ctx.Domain)
	return found
}

// domainKeywordMatcher matches if the domain contains any keyword (case-insensitive).
type domainKeywordMatcher struct {
	keywords []string // lower-cased
}

func newDomainKeywordMatcher(keywords []string) *domainKeywordMatcher {
	lower := make([]string, len(keywords))
	for i, k := range keywords {
		lower[i] = strings.ToLower(k)
	}
	return &domainKeywordMatcher{keywords: lower}
}

func (m *domainKeywordMatcher) Match(ctx *MatchContext) bool {
	if ctx.Domain == "" {
		return false
	}
	d := strings.ToLower(ctx.Domain)
	for _, k := range m.keywords {
		if strings.Contains(d, k) {
			return true
		}
	}
	return false
}

// ipCIDRMatcher matches IPs against a set of CIDR ranges.
type ipCIDRMatcher struct {
	nets []*net.IPNet
}

func newIPCIDRMatcher(cidrs []string) (*ipCIDRMatcher, error) {
	m := &ipCIDRMatcher{}
	for _, c := range cidrs {
		_, ipnet, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", c, err)
		}
		m.nets = append(m.nets, ipnet)
	}
	return m, nil
}

func (m *ipCIDRMatcher) Match(ctx *MatchContext) bool {
	if ctx.IP == nil {
		return false
	}
	for _, n := range m.nets {
		if n.Contains(ctx.IP) {
			return true
		}
	}
	return false
}

// geoIPMatcher matches IPs by country code via GeoIPDB.
type geoIPMatcher struct {
	countries map[string]struct{} // upper-cased
	db        *GeoIPDB
}

func newGeoIPMatcher(countries []string, db *GeoIPDB) *geoIPMatcher {
	m := &geoIPMatcher{
		countries: make(map[string]struct{}, len(countries)),
		db:        db,
	}
	for _, c := range countries {
		m.countries[strings.ToUpper(c)] = struct{}{}
	}
	return m
}

func (m *geoIPMatcher) Match(ctx *MatchContext) bool {
	if ctx.IP == nil || m.db == nil {
		return false
	}
	country := m.db.LookupCountry(ctx.IP)
	_, ok := m.countries[strings.ToUpper(country)]
	return ok
}

// geoSiteMatcher queries GeoSiteDB at match time so it picks up hot-reloaded data.
type geoSiteMatcher struct {
	categories []string // lower-cased category names
	db         *GeoSiteDB
}

func newGeoSiteMatcher(categories []string, db *GeoSiteDB) *geoSiteMatcher {
	lower := make([]string, len(categories))
	for i, c := range categories {
		lower[i] = strings.ToLower(c)
	}
	return &geoSiteMatcher{categories: lower, db: db}
}

func (m *geoSiteMatcher) Match(ctx *MatchContext) bool {
	if ctx.Domain == "" || m.db == nil {
		return false
	}
	domain := strings.ToLower(ctx.Domain)
	for _, cat := range m.categories {
		for _, d := range m.db.Lookup(cat) {
			d = strings.ToLower(d)
			// Handle "+.example.com" wildcard prefix (match subdomains + exact).
			if strings.HasPrefix(d, "+.") {
				base := d[2:]
				if domain == base || strings.HasSuffix(domain, "."+base) {
					return true
				}
			} else if domain == d || strings.HasSuffix(domain, "."+d) {
				return true
			}
		}
	}
	return false
}

// processMatcher matches process names (case-insensitive).
type processMatcher struct {
	names map[string]struct{} // lower-cased
}

func newProcessMatcher(names []string) *processMatcher {
	m := &processMatcher{names: make(map[string]struct{}, len(names))}
	for _, n := range names {
		m.names[strings.ToLower(n)] = struct{}{}
	}
	return m
}

func (m *processMatcher) Match(ctx *MatchContext) bool {
	if ctx.Process == "" {
		return false
	}
	_, ok := m.names[strings.ToLower(ctx.Process)]
	return ok
}

// protocolMatcher matches protocol names (case-insensitive).
type protocolMatcher struct {
	protocols map[string]struct{} // lower-cased
}

func newProtocolMatcher(protocols []string) *protocolMatcher {
	m := &protocolMatcher{protocols: make(map[string]struct{}, len(protocols))}
	for _, p := range protocols {
		m.protocols[strings.ToLower(p)] = struct{}{}
	}
	return m
}

func (m *protocolMatcher) Match(ctx *MatchContext) bool {
	if ctx.Protocol == "" {
		return false
	}
	_, ok := m.protocols[strings.ToLower(ctx.Protocol)]
	return ok
}

// networkTypeMatcher matches against the current network type.
type networkTypeMatcher struct {
	types map[string]struct{} // lower-cased
}

func newNetworkTypeMatcher(types []string) *networkTypeMatcher {
	m := &networkTypeMatcher{types: make(map[string]struct{}, len(types))}
	for _, t := range types {
		m.types[strings.ToLower(t)] = struct{}{}
	}
	return m
}

func (m *networkTypeMatcher) Match(ctx *MatchContext) bool {
	if ctx.NetworkType == "" {
		return false
	}
	_, ok := m.types[strings.ToLower(ctx.NetworkType)]
	return ok
}

// ruleProviderMatcher matches against one or more RuleProviders.
type ruleProviderMatcher struct {
	providers []*provider.RuleProvider
}

func (m *ruleProviderMatcher) Match(ctx *MatchContext) bool {
	for _, rp := range m.providers {
		if ctx.Domain != "" && rp.MatchDomain(ctx.Domain) {
			return true
		}
		if ctx.IP != nil && rp.MatchIP(ctx.IP.String()) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Compiler
// ---------------------------------------------------------------------------

// CompileRuleChain builds compiled rules from rule chain entries.
// ruleProviders may be nil if no rule providers are configured.
func CompileRuleChain(entries []RuleChainEntry, geoIP *GeoIPDB, geoSite *GeoSiteDB, ruleProviders map[string]*provider.RuleProvider) ([]compiledRule, error) {
	rules := make([]compiledRule, 0, len(entries))
	for i := range entries {
		entry := &entries[i]
		var logic logicOp
		switch strings.ToLower(entry.Logic) {
		case "", "and":
			logic = logicAnd
		case "or":
			logic = logicOr
		default:
			return nil, fmt.Errorf("rule_chain[%d]: invalid logic %q (must be \"and\" or \"or\")", i, entry.Logic)
		}

		action := Action(strings.ToLower(entry.Action))
		if action == "" {
			return nil, fmt.Errorf("rule_chain[%d]: action is required", i)
		}
		switch action {
		case ActionProxy, ActionDirect, ActionReject:
		default:
			return nil, fmt.Errorf("rule_chain[%d]: invalid action %q", i, entry.Action)
		}

		var matchers []Matcher
		m := entry.Match

		if len(m.Domain) > 0 {
			matchers = append(matchers, newDomainExactMatcher(m.Domain))
		}
		if len(m.DomainSuffix) > 0 {
			matchers = append(matchers, newDomainSuffixMatcher(m.DomainSuffix))
		}
		if len(m.DomainKeyword) > 0 {
			matchers = append(matchers, newDomainKeywordMatcher(m.DomainKeyword))
		}
		if len(m.GeoSite) > 0 {
			matchers = append(matchers, newGeoSiteMatcher(m.GeoSite, geoSite))
		}
		if len(m.IPCIDR) > 0 {
			cidrMatcher, err := newIPCIDRMatcher(m.IPCIDR)
			if err != nil {
				return nil, fmt.Errorf("rule_chain[%d]: %w", i, err)
			}
			matchers = append(matchers, cidrMatcher)
		}
		if len(m.GeoIP) > 0 {
			matchers = append(matchers, newGeoIPMatcher(m.GeoIP, geoIP))
		}
		if len(m.Process) > 0 {
			matchers = append(matchers, newProcessMatcher(m.Process))
		}
		if len(m.Protocol) > 0 {
			matchers = append(matchers, newProtocolMatcher(m.Protocol))
		}
		if len(m.NetworkType) > 0 {
			matchers = append(matchers, newNetworkTypeMatcher(m.NetworkType))
		}
		if len(m.RuleProvider) > 0 {
			var rps []*provider.RuleProvider
			for _, name := range m.RuleProvider {
				rp, ok := ruleProviders[name]
				if !ok {
					return nil, fmt.Errorf("rule_chain[%d]: rule provider %q not found", i, name)
				}
				rps = append(rps, rp)
			}
			matchers = append(matchers, &ruleProviderMatcher{providers: rps})
		}

		if len(matchers) == 0 {
			return nil, fmt.Errorf("rule_chain[%d]: at least one match condition is required", i)
		}

		rules = append(rules, compiledRule{
			matchers: matchers,
			logic:    logic,
			action:   action,
		})
	}
	return rules, nil
}
