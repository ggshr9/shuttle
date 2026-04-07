package router

import (
	"net"
	"testing"
)

// ---------------------------------------------------------------------------
// Individual matcher tests
// ---------------------------------------------------------------------------

func TestMatcher_DomainExact(t *testing.T) {
	m := newDomainExactMatcher([]string{"example.com", "Foo.Bar"})

	tests := []struct {
		domain string
		want   bool
	}{
		{"example.com", true},
		{"EXAMPLE.COM", true},
		{"foo.bar", true},
		{"Foo.Bar", true},
		{"other.com", false},
		{"sub.example.com", false},
		{"", false},
	}
	for _, tt := range tests {
		ctx := &MatchContext{Domain: tt.domain}
		if got := m.Match(ctx); got != tt.want {
			t.Errorf("domainExact.Match(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestMatcher_DomainSuffix(t *testing.T) {
	m := newDomainSuffixMatcher([]string{"example.com"})

	tests := []struct {
		domain string
		want   bool
	}{
		{"example.com", true},        // exact match
		{"sub.example.com", true},    // subdomain
		{"deep.sub.example.com", true},
		{"notexample.com", false},    // not a suffix match
		{"example.org", false},
		{"", false},
	}
	for _, tt := range tests {
		ctx := &MatchContext{Domain: tt.domain}
		if got := m.Match(ctx); got != tt.want {
			t.Errorf("domainSuffix.Match(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestMatcher_DomainKeyword(t *testing.T) {
	m := newDomainKeywordMatcher([]string{"google", "AD"})

	tests := []struct {
		domain string
		want   bool
	}{
		{"www.google.com", true},
		{"google.com", true},
		{"ads.example.com", true},   // contains "ad" case-insensitive
		{"loading.com", true},        // contains "ad" in "loading"
		{"example.com", false},
		{"", false},
	}
	for _, tt := range tests {
		ctx := &MatchContext{Domain: tt.domain}
		if got := m.Match(ctx); got != tt.want {
			t.Errorf("domainKeyword.Match(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestMatcher_IPCIDR(t *testing.T) {
	m, err := newIPCIDRMatcher([]string{"10.0.0.0/8", "192.168.1.0/24"})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		ip   string
		want bool
	}{
		{"10.1.2.3", true},
		{"10.255.255.255", true},
		{"192.168.1.100", true},
		{"192.168.2.1", false},
		{"8.8.8.8", false},
	}
	for _, tt := range tests {
		ctx := &MatchContext{IP: net.ParseIP(tt.ip)}
		if got := m.Match(ctx); got != tt.want {
			t.Errorf("ipCIDR.Match(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}

	// nil IP should not match
	ctx := &MatchContext{}
	if m.Match(ctx) {
		t.Error("ipCIDR.Match(nil) = true, want false")
	}
}

func TestMatcher_IPCIDR_InvalidCIDR(t *testing.T) {
	_, err := newIPCIDRMatcher([]string{"not-a-cidr"})
	if err == nil {
		t.Error("expected error for invalid CIDR, got nil")
	}
}

func TestMatcher_GeoIP(t *testing.T) {
	db := NewGeoIPDB()
	db.LoadFromCIDRs("CN", []string{"10.0.0.0/8"})
	db.LoadFromCIDRs("US", []string{"192.168.0.0/16"})

	m := newGeoIPMatcher([]string{"cn"}, db)

	tests := []struct {
		ip   string
		want bool
	}{
		{"10.1.2.3", true},   // CN
		{"192.168.1.1", false}, // US, not CN
		{"8.8.8.8", false},    // unknown
	}
	for _, tt := range tests {
		ctx := &MatchContext{IP: net.ParseIP(tt.ip)}
		if got := m.Match(ctx); got != tt.want {
			t.Errorf("geoIP.Match(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}

	// nil IP
	if m.Match(&MatchContext{}) {
		t.Error("geoIP.Match(nil IP) = true, want false")
	}

	// nil DB
	m2 := newGeoIPMatcher([]string{"CN"}, nil)
	if m2.Match(&MatchContext{IP: net.ParseIP("10.1.2.3")}) {
		t.Error("geoIP.Match with nil DB = true, want false")
	}
}

func TestMatcher_GeoSite(t *testing.T) {
	db := NewGeoSiteDB()
	db.LoadCategory("cn", []string{"baidu.com", "+.taobao.com"})

	m := newGeoSiteMatcher([]string{"cn"}, db)

	tests := []struct {
		domain string
		want   bool
	}{
		{"baidu.com", true},
		{"www.taobao.com", true},
		{"taobao.com", true},
		{"google.com", false},
		{"", false},
	}
	for _, tt := range tests {
		ctx := &MatchContext{Domain: tt.domain}
		if got := m.Match(ctx); got != tt.want {
			t.Errorf("geoSite.Match(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}

	// nil GeoSiteDB should produce an empty matcher
	m2 := newGeoSiteMatcher([]string{"cn"}, nil)
	if m2.Match(&MatchContext{Domain: "baidu.com"}) {
		t.Error("geoSite.Match with nil DB = true, want false")
	}
}

func TestMatcher_Process(t *testing.T) {
	m := newProcessMatcher([]string{"Chrome", "firefox"})

	tests := []struct {
		process string
		want    bool
	}{
		{"chrome", true},
		{"CHROME", true},
		{"Chrome", true},
		{"firefox", true},
		{"Firefox", true},
		{"safari", false},
		{"", false},
	}
	for _, tt := range tests {
		ctx := &MatchContext{Process: tt.process}
		if got := m.Match(ctx); got != tt.want {
			t.Errorf("process.Match(%q) = %v, want %v", tt.process, got, tt.want)
		}
	}
}

func TestMatcher_Protocol(t *testing.T) {
	m := newProtocolMatcher([]string{"BitTorrent", "quic"})

	tests := []struct {
		protocol string
		want     bool
	}{
		{"bittorrent", true},
		{"BitTorrent", true},
		{"BITTORRENT", true},
		{"quic", true},
		{"QUIC", true},
		{"http", false},
		{"", false},
	}
	for _, tt := range tests {
		ctx := &MatchContext{Protocol: tt.protocol}
		if got := m.Match(ctx); got != tt.want {
			t.Errorf("protocol.Match(%q) = %v, want %v", tt.protocol, got, tt.want)
		}
	}
}

func TestMatcher_NetworkType(t *testing.T) {
	m := newNetworkTypeMatcher([]string{"wifi", "Ethernet"})

	tests := []struct {
		nt   string
		want bool
	}{
		{"wifi", true},
		{"WiFi", true},
		{"ethernet", true},
		{"Ethernet", true},
		{"cellular", false},
		{"", false},
	}
	for _, tt := range tests {
		ctx := &MatchContext{NetworkType: tt.nt}
		if got := m.Match(ctx); got != tt.want {
			t.Errorf("networkType.Match(%q) = %v, want %v", tt.nt, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// compiledRule logic tests
// ---------------------------------------------------------------------------

func TestCompiledRule_ANDLogic(t *testing.T) {
	rule := &compiledRule{
		matchers: []Matcher{
			newDomainExactMatcher([]string{"example.com"}),
			newProcessMatcher([]string{"chrome"}),
		},
		logic:  logicAnd,
		action: ActionReject,
	}

	// Both match
	ctx := &MatchContext{Domain: "example.com", Process: "chrome"}
	if !rule.Match(ctx) {
		t.Error("AND rule with both matching = false, want true")
	}

	// Only domain matches
	ctx = &MatchContext{Domain: "example.com", Process: "firefox"}
	if rule.Match(ctx) {
		t.Error("AND rule with only domain matching = true, want false")
	}

	// Only process matches
	ctx = &MatchContext{Domain: "other.com", Process: "chrome"}
	if rule.Match(ctx) {
		t.Error("AND rule with only process matching = true, want false")
	}

	// Neither matches
	ctx = &MatchContext{Domain: "other.com", Process: "firefox"}
	if rule.Match(ctx) {
		t.Error("AND rule with neither matching = true, want false")
	}
}

func TestCompiledRule_ORLogic(t *testing.T) {
	rule := &compiledRule{
		matchers: []Matcher{
			newDomainExactMatcher([]string{"example.com"}),
			newProcessMatcher([]string{"chrome"}),
		},
		logic:  logicOr,
		action: ActionDirect,
	}

	// Both match
	ctx := &MatchContext{Domain: "example.com", Process: "chrome"}
	if !rule.Match(ctx) {
		t.Error("OR rule with both matching = false, want true")
	}

	// Only domain matches
	ctx = &MatchContext{Domain: "example.com", Process: "firefox"}
	if !rule.Match(ctx) {
		t.Error("OR rule with domain matching = false, want true")
	}

	// Only process matches
	ctx = &MatchContext{Domain: "other.com", Process: "chrome"}
	if !rule.Match(ctx) {
		t.Error("OR rule with process matching = false, want true")
	}

	// Neither matches
	ctx = &MatchContext{Domain: "other.com", Process: "firefox"}
	if rule.Match(ctx) {
		t.Error("OR rule with neither matching = true, want false")
	}
}

func TestCompiledRule_EmptyMatchers(t *testing.T) {
	rule := &compiledRule{
		matchers: nil,
		logic:    logicAnd,
		action:   ActionProxy,
	}
	if rule.Match(&MatchContext{Domain: "anything.com"}) {
		t.Error("rule with no matchers = true, want false")
	}
}

// ---------------------------------------------------------------------------
// CompileRuleChain tests
// ---------------------------------------------------------------------------

func TestCompileRuleChain_ValidEntries(t *testing.T) {
	entries := []RuleChainEntry{
		{
			Match:  RuleMatch{Domain: []string{"example.com"}},
			Action: "direct",
		},
		{
			Match:  RuleMatch{Process: []string{"chrome"}, Protocol: []string{"quic"}},
			Logic:  "or",
			Action: "reject",
		},
	}

	rules, err := CompileRuleChain(entries, nil, nil, nil)
	if err != nil {
		t.Fatalf("CompileRuleChain() error = %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}
	if rules[0].logic != logicAnd {
		t.Error("rule[0].logic = or, want and (default)")
	}
	if rules[1].logic != logicOr {
		t.Error("rule[1].logic = and, want or")
	}
}

func TestCompileRuleChain_InvalidAction(t *testing.T) {
	entries := []RuleChainEntry{
		{
			Match:  RuleMatch{Domain: []string{"example.com"}},
			Action: "invalid",
		},
	}
	_, err := CompileRuleChain(entries, nil, nil, nil)
	if err == nil {
		t.Error("expected error for invalid action, got nil")
	}
}

func TestCompileRuleChain_MissingAction(t *testing.T) {
	entries := []RuleChainEntry{
		{
			Match: RuleMatch{Domain: []string{"example.com"}},
		},
	}
	_, err := CompileRuleChain(entries, nil, nil, nil)
	if err == nil {
		t.Error("expected error for missing action, got nil")
	}
}

func TestCompileRuleChain_InvalidLogic(t *testing.T) {
	entries := []RuleChainEntry{
		{
			Match:  RuleMatch{Domain: []string{"example.com"}},
			Logic:  "xor",
			Action: "proxy",
		},
	}
	_, err := CompileRuleChain(entries, nil, nil, nil)
	if err == nil {
		t.Error("expected error for invalid logic, got nil")
	}
}

func TestCompileRuleChain_NoMatchConditions(t *testing.T) {
	entries := []RuleChainEntry{
		{
			Match:  RuleMatch{},
			Action: "proxy",
		},
	}
	_, err := CompileRuleChain(entries, nil, nil, nil)
	if err == nil {
		t.Error("expected error for empty match conditions, got nil")
	}
}

func TestCompileRuleChain_InvalidCIDR(t *testing.T) {
	entries := []RuleChainEntry{
		{
			Match:  RuleMatch{IPCIDR: []string{"not-valid"}},
			Action: "proxy",
		},
	}
	_, err := CompileRuleChain(entries, nil, nil, nil)
	if err == nil {
		t.Error("expected error for invalid CIDR, got nil")
	}
}

// ---------------------------------------------------------------------------
// Router integration: rule chain before legacy
// ---------------------------------------------------------------------------

func TestRuleChain_BeforeLegacy(t *testing.T) {
	cfg := &RouterConfig{
		RuleChain: []RuleChainEntry{
			{
				Match:  RuleMatch{Domain: []string{"example.com"}},
				Action: "reject",
			},
		},
		Rules: []Rule{
			{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect},
		},
		DefaultAction: ActionProxy,
	}
	r := NewRouter(cfg, nil, nil, nil)

	// Rule chain should win over the legacy domain rule.
	got := r.Match("example.com", nil, "", "", 0, nil)
	if got != ActionReject {
		t.Errorf("Match(example.com) = %q, want %q (rule chain should override legacy)", got, ActionReject)
	}
}

func TestRuleChain_FallsThroughToLegacy(t *testing.T) {
	cfg := &RouterConfig{
		RuleChain: []RuleChainEntry{
			{
				Match:  RuleMatch{Domain: []string{"blocked.com"}},
				Action: "reject",
			},
		},
		Rules: []Rule{
			{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect},
		},
		DefaultAction: ActionProxy,
	}
	r := NewRouter(cfg, nil, nil, nil)

	// "blocked.com" matches rule chain
	got := r.Match("blocked.com", nil, "", "", 0, nil)
	if got != ActionReject {
		t.Errorf("Match(blocked.com) = %q, want %q", got, ActionReject)
	}

	// "example.com" falls through to legacy
	got = r.Match("example.com", nil, "", "", 0, nil)
	if got != ActionDirect {
		t.Errorf("Match(example.com) = %q, want %q (should fall through to legacy)", got, ActionDirect)
	}

	// "other.com" falls through to default
	got = r.Match("other.com", nil, "", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("Match(other.com) = %q, want %q (should fall through to default)", got, ActionProxy)
	}
}

func TestRuleChain_Empty(t *testing.T) {
	// No rule chain — legacy behavior should be unchanged.
	cfg := &RouterConfig{
		Rules: []Rule{
			{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect},
			{Type: "process", Values: []string{"chrome"}, Action: ActionReject},
		},
		DefaultAction: ActionProxy,
	}
	r := NewRouter(cfg, nil, nil, nil)

	got := r.Match("example.com", nil, "", "", 0, nil)
	if got != ActionDirect {
		t.Errorf("Match(example.com) = %q, want %q", got, ActionDirect)
	}

	got = r.Match("", nil, "chrome", "", 0, nil)
	if got != ActionReject {
		t.Errorf("Match(chrome) = %q, want %q", got, ActionReject)
	}

	got = r.Match("other.com", nil, "", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("Match(other.com) = %q, want %q", got, ActionProxy)
	}
}

func TestRuleChain_ANDWithMultipleConditions(t *testing.T) {
	cfg := &RouterConfig{
		RuleChain: []RuleChainEntry{
			{
				Match: RuleMatch{
					DomainSuffix: []string{"example.com"},
					Process:      []string{"chrome"},
				},
				Logic:  "and",
				Action: "reject",
			},
		},
		DefaultAction: ActionProxy,
	}
	r := NewRouter(cfg, nil, nil, nil)

	// Both conditions match
	got := r.Match("sub.example.com", nil, "chrome", "", 0, nil)
	if got != ActionReject {
		t.Errorf("AND both match: got %q, want %q", got, ActionReject)
	}

	// Only domain matches
	got = r.Match("sub.example.com", nil, "firefox", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("AND only domain: got %q, want %q", got, ActionProxy)
	}

	// Only process matches
	got = r.Match("other.com", nil, "chrome", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("AND only process: got %q, want %q", got, ActionProxy)
	}
}

func TestRuleChain_ORWithMultipleConditions(t *testing.T) {
	cfg := &RouterConfig{
		RuleChain: []RuleChainEntry{
			{
				Match: RuleMatch{
					Domain:  []string{"blocked.com"},
					Process: []string{"torrent"},
				},
				Logic:  "or",
				Action: "reject",
			},
		},
		DefaultAction: ActionProxy,
	}
	r := NewRouter(cfg, nil, nil, nil)

	// Domain matches
	got := r.Match("blocked.com", nil, "", "", 0, nil)
	if got != ActionReject {
		t.Errorf("OR domain match: got %q, want %q", got, ActionReject)
	}

	// Process matches
	got = r.Match("", nil, "torrent", "", 0, nil)
	if got != ActionReject {
		t.Errorf("OR process match: got %q, want %q", got, ActionReject)
	}

	// Neither matches
	got = r.Match("other.com", nil, "chrome", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("OR neither match: got %q, want %q", got, ActionProxy)
	}
}

func TestRuleChain_WithNetworkType(t *testing.T) {
	cfg := &RouterConfig{
		RuleChain: []RuleChainEntry{
			{
				Match: RuleMatch{
					DomainKeyword: []string{"video"},
					NetworkType:   []string{"cellular"},
				},
				Logic:  "and",
				Action: "reject",
			},
		},
		DefaultAction: ActionProxy,
	}
	r := NewRouter(cfg, nil, nil, nil)

	// Without network type set, networkType matcher won't match
	got := r.Match("video.example.com", nil, "", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("no network type: got %q, want %q", got, ActionProxy)
	}

	// Set to cellular — both conditions match
	r.SetNetworkType("cellular")
	got = r.Match("video.example.com", nil, "", "", 0, nil)
	if got != ActionReject {
		t.Errorf("cellular + video: got %q, want %q", got, ActionReject)
	}

	// Set to wifi — network type doesn't match
	r.SetNetworkType("wifi")
	got = r.Match("video.example.com", nil, "", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("wifi + video: got %q, want %q", got, ActionProxy)
	}
}

func TestRuleChain_WithIPCIDR(t *testing.T) {
	cfg := &RouterConfig{
		RuleChain: []RuleChainEntry{
			{
				Match:  RuleMatch{IPCIDR: []string{"10.0.0.0/8"}},
				Action: "direct",
			},
		},
		DefaultAction: ActionProxy,
	}
	r := NewRouter(cfg, nil, nil, nil)

	got := r.Match("", net.ParseIP("10.1.2.3"), "", "", 0, nil)
	if got != ActionDirect {
		t.Errorf("IP in CIDR: got %q, want %q", got, ActionDirect)
	}

	got = r.Match("", net.ParseIP("192.168.1.1"), "", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("IP not in CIDR: got %q, want %q", got, ActionProxy)
	}
}

func TestRuleChain_OrderMatters(t *testing.T) {
	cfg := &RouterConfig{
		RuleChain: []RuleChainEntry{
			{
				Match:  RuleMatch{Domain: []string{"example.com"}},
				Action: "reject",
			},
			{
				Match:  RuleMatch{Domain: []string{"example.com"}},
				Action: "direct",
			},
		},
		DefaultAction: ActionProxy,
	}
	r := NewRouter(cfg, nil, nil, nil)

	// First rule should win
	got := r.Match("example.com", nil, "", "", 0, nil)
	if got != ActionReject {
		t.Errorf("first rule should win: got %q, want %q", got, ActionReject)
	}
}

func TestRuleChain_Negate(t *testing.T) {
	cfg := &RouterConfig{
		RuleChain: []RuleChainEntry{
			{
				// Reject everything EXCEPT example.com
				Match:  RuleMatch{Domain: []string{"example.com"}},
				Negate: true,
				Action: "reject",
			},
		},
		DefaultAction: ActionProxy,
	}
	r := NewRouter(cfg, nil, nil, nil)

	// example.com matches the domain matcher, but negate inverts it => no match => fall through to default
	got := r.Match("example.com", nil, "", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("negated match should NOT fire: got %q, want %q", got, ActionProxy)
	}

	// other.com does NOT match the domain matcher, negate inverts => match => reject
	got = r.Match("other.com", nil, "", "", 0, nil)
	if got != ActionReject {
		t.Errorf("negated non-match should fire: got %q, want %q", got, ActionReject)
	}
}

func TestRuleChain_NegateWithAND(t *testing.T) {
	cfg := &RouterConfig{
		RuleChain: []RuleChainEntry{
			{
				// Negate: match everything that is NOT (domain=example.com AND process=chrome)
				Match: RuleMatch{
					Domain:  []string{"example.com"},
					Process: []string{"chrome"},
				},
				Logic:  "and",
				Negate: true,
				Action: "reject",
			},
		},
		DefaultAction: ActionProxy,
	}
	r := NewRouter(cfg, nil, nil, nil)

	// Both match => AND=true => negate=false => falls through
	got := r.Match("example.com", nil, "chrome", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("both match + negate: got %q, want %q", got, ActionProxy)
	}

	// Only domain matches => AND=false => negate=true => reject
	got = r.Match("example.com", nil, "firefox", "", 0, nil)
	if got != ActionReject {
		t.Errorf("partial match + negate: got %q, want %q", got, ActionReject)
	}
}

func TestRuleChain_PortRouting(t *testing.T) {
	cfg := &RouterConfig{
		RuleChain: []RuleChainEntry{
			{
				Match:  RuleMatch{Port: []string{"80", "443", "8080-8090"}},
				Action: "direct",
			},
		},
		DefaultAction: ActionProxy,
	}
	r := NewRouter(cfg, nil, nil, nil)

	got := r.Match("example.com", nil, "", "", 80, nil)
	if got != ActionDirect {
		t.Errorf("port 80: got %q, want %q", got, ActionDirect)
	}

	got = r.Match("example.com", nil, "", "", 8085, nil)
	if got != ActionDirect {
		t.Errorf("port 8085: got %q, want %q", got, ActionDirect)
	}

	got = r.Match("example.com", nil, "", "", 22, nil)
	if got != ActionProxy {
		t.Errorf("port 22: got %q, want %q", got, ActionProxy)
	}

	// No port info
	got = r.Match("example.com", nil, "", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("port 0: got %q, want %q", got, ActionProxy)
	}
}

func TestRuleChain_SrcIPRouting(t *testing.T) {
	cfg := &RouterConfig{
		RuleChain: []RuleChainEntry{
			{
				Match:  RuleMatch{SrcIP: []string{"192.168.1.0/24"}},
				Action: "direct",
			},
		},
		DefaultAction: ActionProxy,
	}
	r := NewRouter(cfg, nil, nil, nil)

	got := r.Match("example.com", nil, "", "", 0, net.ParseIP("192.168.1.50"))
	if got != ActionDirect {
		t.Errorf("srcIP in range: got %q, want %q", got, ActionDirect)
	}

	got = r.Match("example.com", nil, "", "", 0, net.ParseIP("10.0.0.1"))
	if got != ActionProxy {
		t.Errorf("srcIP not in range: got %q, want %q", got, ActionProxy)
	}

	got = r.Match("example.com", nil, "", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("nil srcIP: got %q, want %q", got, ActionProxy)
	}
}

// ---------------------------------------------------------------------------
// Benchmark
// ---------------------------------------------------------------------------

func BenchmarkRuleChainMatch(b *testing.B) {
	cfg := &RouterConfig{
		RuleChain: []RuleChainEntry{
			{Match: RuleMatch{DomainSuffix: []string{"google.com", "youtube.com"}}, Action: "proxy"},
			{Match: RuleMatch{IPCIDR: []string{"10.0.0.0/8", "172.16.0.0/12"}}, Action: "direct"},
			{Match: RuleMatch{Process: []string{"chrome", "firefox"}, Protocol: []string{"quic"}}, Logic: "and", Action: "proxy"},
			{Match: RuleMatch{DomainKeyword: []string{"ads", "tracker"}}, Action: "reject"},
		},
		DefaultAction: ActionProxy,
	}
	r := NewRouter(cfg, nil, nil, nil)
	ip := net.ParseIP("10.1.2.3")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Match("www.google.com", ip, "chrome", "quic", 0, nil)
	}
}
