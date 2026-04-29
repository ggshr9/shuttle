package router

import (
	"net"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// DomainTrie tests
// ---------------------------------------------------------------------------

func TestReverseDomain(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"example.com", []string{"com", "example"}},
		{"sub.example.com", []string{"com", "example", "sub"}},
		{"+.example.com", []string{"com", "example"}},
		{"Example.COM", []string{"com", "example"}},
		{"example.com.", []string{"com", "example"}},
	}
	for _, tt := range tests {
		got := reverseDomain(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("reverseDomain(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("reverseDomain(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestTrieInsertAndLookupExact(t *testing.T) {
	trie := NewDomainTrie()
	trie.Insert("example.com", "proxy")

	action, found := trie.Lookup("example.com")
	if !found || action != "proxy" {
		t.Errorf("Lookup(example.com) = (%q, %v), want (\"proxy\", true)", action, found)
	}
}

func TestTrieLookupNoMatch(t *testing.T) {
	trie := NewDomainTrie()
	trie.Insert("example.com", "proxy")

	action, found := trie.Lookup("notfound.org")
	if found {
		t.Errorf("Lookup(notfound.org) = (%q, %v), want (\"\", false)", action, found)
	}
}

func TestTrieWildcardMatching(t *testing.T) {
	trie := NewDomainTrie()
	trie.Insert("+.example.com", "direct")

	// Subdomain should match the wildcard.
	action, found := trie.Lookup("sub.example.com")
	if !found || action != "direct" {
		t.Errorf("Lookup(sub.example.com) = (%q, %v), want (\"direct\", true)", action, found)
	}

	// Deeper subdomain should also match.
	action, found = trie.Lookup("deep.sub.example.com")
	if !found || action != "direct" {
		t.Errorf("Lookup(deep.sub.example.com) = (%q, %v), want (\"direct\", true)", action, found)
	}

	// The bare domain also matches because "+." is stripped during Insert,
	// so "+.example.com" stores the same path as "example.com" with isEnd=true.
	action, found = trie.Lookup("example.com")
	if !found || action != "direct" {
		t.Errorf("Lookup(example.com) with wildcard = (%q, %v), want (\"direct\", true)", action, found)
	}
}

func TestTrieWildcardAndExact(t *testing.T) {
	trie := NewDomainTrie()
	trie.Insert("+.example.com", "direct")
	trie.Insert("api.example.com", "proxy")

	// Exact match takes precedence over wildcard for api.example.com.
	action, found := trie.Lookup("api.example.com")
	if !found || action != "proxy" {
		t.Errorf("Lookup(api.example.com) = (%q, %v), want (\"proxy\", true)", action, found)
	}

	// Other subdomains still match wildcard.
	action, found = trie.Lookup("www.example.com")
	if !found || action != "direct" {
		t.Errorf("Lookup(www.example.com) = (%q, %v), want (\"direct\", true)", action, found)
	}
}

func TestTrieDelete(t *testing.T) {
	trie := NewDomainTrie()
	trie.Insert("example.com", "proxy")
	trie.Insert("other.com", "direct")

	if trie.Size() != 2 {
		t.Fatalf("Size() = %d, want 2", trie.Size())
	}

	ok := trie.Delete("example.com")
	if !ok {
		t.Fatal("Delete(example.com) returned false, want true")
	}
	if trie.Size() != 1 {
		t.Errorf("Size() after delete = %d, want 1", trie.Size())
	}

	_, found := trie.Lookup("example.com")
	if found {
		t.Error("Lookup(example.com) found after delete")
	}

	// Deleting a non-existent domain should return false.
	ok = trie.Delete("nonexistent.com")
	if ok {
		t.Error("Delete(nonexistent.com) returned true, want false")
	}

	// Double-delete should return false.
	ok = trie.Delete("example.com")
	if ok {
		t.Error("Double Delete(example.com) returned true, want false")
	}
}

func TestTrieSize(t *testing.T) {
	trie := NewDomainTrie()
	if trie.Size() != 0 {
		t.Fatalf("initial Size() = %d, want 0", trie.Size())
	}

	trie.Insert("a.com", "proxy")
	trie.Insert("b.com", "proxy")
	trie.Insert("c.com", "proxy")
	if trie.Size() != 3 {
		t.Fatalf("Size() = %d, want 3", trie.Size())
	}

	// Re-inserting should not increase size.
	trie.Insert("a.com", "direct")
	if trie.Size() != 3 {
		t.Errorf("Size() after re-insert = %d, want 3", trie.Size())
	}
}

func TestTrieCaseInsensitivity(t *testing.T) {
	trie := NewDomainTrie()
	trie.Insert("Example.COM", "proxy")

	action, found := trie.Lookup("example.com")
	if !found || action != "proxy" {
		t.Errorf("Lookup(example.com) = (%q, %v), want (\"proxy\", true)", action, found)
	}

	action, found = trie.Lookup("EXAMPLE.COM")
	if !found || action != "proxy" {
		t.Errorf("Lookup(EXAMPLE.COM) = (%q, %v), want (\"proxy\", true)", action, found)
	}
}

// ---------------------------------------------------------------------------
// GeoIPDB tests
// ---------------------------------------------------------------------------

func TestGeoIPLoadAndLookupIPv4(t *testing.T) {
	db := NewGeoIPDB()
	db.LoadFromCIDRs("CN", []string{"10.0.0.0/8"})

	country := db.LookupCountry(net.ParseIP("10.1.2.3"))
	if country != "CN" {
		t.Errorf("LookupCountry(10.1.2.3) = %q, want \"CN\"", country)
	}
}

func TestGeoIPUnknownIPv4(t *testing.T) {
	db := NewGeoIPDB()
	db.LoadFromCIDRs("CN", []string{"10.0.0.0/8"})

	country := db.LookupCountry(net.ParseIP("192.168.1.1"))
	if country != "" {
		t.Errorf("LookupCountry(192.168.1.1) = %q, want \"\"", country)
	}
}

func TestGeoIPMultipleCIDRs(t *testing.T) {
	db := NewGeoIPDB()
	db.LoadFromCIDRs("CN", []string{"10.0.0.0/8", "172.16.0.0/12"})
	db.LoadFromCIDRs("US", []string{"192.168.0.0/16"})

	tests := []struct {
		ip   string
		want string
	}{
		{"10.0.0.1", "CN"},
		{"10.255.255.255", "CN"},
		{"172.16.0.1", "CN"},
		{"172.31.255.255", "CN"},
		{"192.168.1.1", "US"},
		{"192.168.255.255", "US"},
		{"8.8.8.8", ""},
	}
	for _, tt := range tests {
		got := db.LookupCountry(net.ParseIP(tt.ip))
		if got != tt.want {
			t.Errorf("LookupCountry(%s) = %q, want %q", tt.ip, got, tt.want)
		}
	}
}

func TestGeoIPBinarySearchEdgeCases(t *testing.T) {
	db := NewGeoIPDB()
	// Two adjacent /24 blocks.
	db.LoadFromCIDRs("JP", []string{"1.0.0.0/24"})
	db.LoadFromCIDRs("KR", []string{"1.0.1.0/24"})

	tests := []struct {
		ip   string
		want string
	}{
		{"1.0.0.0", "JP"},
		{"1.0.0.255", "JP"},
		{"1.0.1.0", "KR"},
		{"1.0.1.255", "KR"},
		{"1.0.2.0", ""},
		{"0.255.255.255", ""},
	}
	for _, tt := range tests {
		got := db.LookupCountry(net.ParseIP(tt.ip))
		if got != tt.want {
			t.Errorf("LookupCountry(%s) = %q, want %q", tt.ip, got, tt.want)
		}
	}
}

func TestGeoIPEmptyDB(t *testing.T) {
	db := NewGeoIPDB()
	country := db.LookupCountry(net.ParseIP("1.2.3.4"))
	if country != "" {
		t.Errorf("LookupCountry on empty DB = %q, want \"\"", country)
	}
}

func TestGeoIPIPv6(t *testing.T) {
	db := NewGeoIPDB()
	db.LoadFromCIDRs("DE", []string{"2001:db8::/32"})

	country := db.LookupCountry(net.ParseIP("2001:db8::1"))
	if country != "DE" {
		t.Errorf("LookupCountry(2001:db8::1) = %q, want \"DE\"", country)
	}

	country = db.LookupCountry(net.ParseIP("2001:db9::1"))
	if country != "" {
		t.Errorf("LookupCountry(2001:db9::1) = %q, want \"\"", country)
	}
}

// ---------------------------------------------------------------------------
// GeoSiteDB tests
// ---------------------------------------------------------------------------

func TestGeoSiteCategories(t *testing.T) {
	db := NewGeoSiteDB()
	db.LoadCategory("cn", []string{"baidu.com"})
	db.LoadCategory("ads", []string{"doubleclick.net"})
	db.LoadCategory("apple", []string{"apple.com"})

	cats := db.Categories()
	if len(cats) != 3 {
		t.Fatalf("expected 3 categories, got %d", len(cats))
	}
	// Should be sorted alphabetically
	if cats[0] != "ads" || cats[1] != "apple" || cats[2] != "cn" {
		t.Fatalf("expected sorted [ads apple cn], got %v", cats)
	}
}

func TestGeoSiteCategoriesEmpty(t *testing.T) {
	db := NewGeoSiteDB()
	cats := db.Categories()
	if len(cats) != 0 {
		t.Fatalf("expected 0 categories, got %d", len(cats))
	}
}

// ---------------------------------------------------------------------------
// Router tests
// ---------------------------------------------------------------------------

func newTestRouter(rules []Rule, defaultAction Action) *Router {
	cfg := &RouterConfig{
		Rules:         rules,
		DefaultAction: defaultAction,
	}
	return NewRouter(cfg, nil, nil, nil)
}

func TestRouterMatchDomain(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect},
		{Type: "domain", Values: []string{"+.blocked.com"}, Action: ActionReject},
	}, ActionProxy)

	tests := []struct {
		domain string
		want   Action
	}{
		{"example.com", ActionDirect},
		{"sub.blocked.com", ActionReject},
		{"unknown.org", ActionProxy},
	}
	for _, tt := range tests {
		got := r.MatchDomain(tt.domain)
		if got != tt.want {
			t.Errorf("MatchDomain(%q) = %q, want %q", tt.domain, got, tt.want)
		}
	}
}

func TestRouterMatchIPCIDR(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "ip-cidr", Values: []string{"10.0.0.0/8"}, Action: ActionDirect},
		{Type: "ip-cidr", Values: []string{"192.168.0.0/16"}, Action: ActionReject},
	}, ActionProxy)

	tests := []struct {
		ip   string
		want Action
	}{
		{"10.1.2.3", ActionDirect},
		{"192.168.1.1", ActionReject},
		{"8.8.8.8", ActionProxy},
	}
	for _, tt := range tests {
		got := r.MatchIP(net.ParseIP(tt.ip))
		if got != tt.want {
			t.Errorf("MatchIP(%s) = %q, want %q", tt.ip, got, tt.want)
		}
	}
}

func TestRouterMatchProcess(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "process", Values: []string{"chrome", "Firefox"}, Action: ActionDirect},
	}, ActionProxy)

	tests := []struct {
		process string
		want    Action
	}{
		{"chrome", ActionDirect},
		{"CHROME", ActionDirect},   // case insensitive
		{"firefox", ActionDirect},  // case insensitive
		{"Firefox", ActionDirect},
		{"unknown", ActionProxy},
	}
	for _, tt := range tests {
		got := r.MatchProcess(tt.process)
		if got != tt.want {
			t.Errorf("MatchProcess(%q) = %q, want %q", tt.process, got, tt.want)
		}
	}
}

func TestRouterMatchProtocol(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "protocol", Values: []string{"bittorrent"}, Action: ActionReject},
	}, ActionProxy)

	got := r.MatchProtocol("bittorrent")
	if got != ActionReject {
		t.Errorf("MatchProtocol(bittorrent) = %q, want %q", got, ActionReject)
	}

	got = r.MatchProtocol("BitTorrent")
	if got != ActionReject {
		t.Errorf("MatchProtocol(BitTorrent) = %q, want %q", got, ActionReject)
	}

	got = r.MatchProtocol("http")
	if got != ActionProxy {
		t.Errorf("MatchProtocol(http) = %q, want %q", got, ActionProxy)
	}
}

func TestRouter_DecisionHookFires(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect},
	}, ActionProxy)

	var (
		hits   []string
		hitsMu sync.Mutex
	)
	r.SetDecisionHook(func(decision, rule string) {
		hitsMu.Lock()
		defer hitsMu.Unlock()
		hits = append(hits, decision+"/"+rule)
	})

	_ = r.MatchDomain("example.com")

	// Hook fires async — wait briefly for goroutine to dispatch.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		hitsMu.Lock()
		n := len(hits)
		hitsMu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	hitsMu.Lock()
	defer hitsMu.Unlock()
	if len(hits) != 1 {
		t.Fatalf("expected 1 hook call, got %d: %v", len(hits), hits)
	}
	if hits[0] != "direct/domain" {
		t.Errorf("expected hook %q, got %q", "direct/domain", hits[0])
	}
}

func TestRouter_DecisionHookCoversAllMatchers(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect},
		{Type: "ip-cidr", Values: []string{"10.0.0.0/8"}, Action: ActionDirect},
		{Type: "process", Values: []string{"chrome"}, Action: ActionReject},
		{Type: "protocol", Values: []string{"bittorrent"}, Action: ActionReject},
	}, ActionProxy)

	var (
		hits   []string
		hitsMu sync.Mutex
	)
	r.SetDecisionHook(func(decision, rule string) {
		hitsMu.Lock()
		defer hitsMu.Unlock()
		hits = append(hits, decision+"/"+rule)
	})

	_ = r.MatchDomain("example.com")
	_ = r.MatchIP(net.ParseIP("10.1.2.3"))
	_ = r.MatchProcess("chrome")
	_ = r.MatchProtocol("bittorrent")
	// default fallthrough
	_ = r.MatchDomain("nomatch.invalid")

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		hitsMu.Lock()
		n := len(hits)
		hitsMu.Unlock()
		if n >= 5 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	hitsMu.Lock()
	defer hitsMu.Unlock()
	if len(hits) < 5 {
		t.Fatalf("expected at least 5 hook calls, got %d: %v", len(hits), hits)
	}
}

func TestRouterMatchPriority(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "protocol", Values: []string{"bittorrent"}, Action: ActionReject},
		{Type: "process", Values: []string{"chrome"}, Action: ActionDirect},
		{Type: "domain", Values: []string{"example.com"}, Action: ActionReject},
		{Type: "ip-cidr", Values: []string{"10.0.0.0/8"}, Action: ActionDirect},
	}, ActionProxy)

	// Protocol takes highest priority.
	got := r.Match("example.com", net.ParseIP("10.1.2.3"), "chrome", "bittorrent", 0, nil)
	if got != ActionReject {
		t.Errorf("Match with protocol = %q, want %q", got, ActionReject)
	}

	// Process is next when protocol does not match a non-default action.
	got = r.Match("example.com", net.ParseIP("10.1.2.3"), "chrome", "", 0, nil)
	if got != ActionDirect {
		t.Errorf("Match with process = %q, want %q", got, ActionDirect)
	}

	// Domain is next.
	got = r.Match("example.com", net.ParseIP("10.1.2.3"), "", "", 0, nil)
	if got != ActionReject {
		t.Errorf("Match with domain = %q, want %q", got, ActionReject)
	}

	// IP is next.
	got = r.Match("", net.ParseIP("10.1.2.3"), "", "", 0, nil)
	if got != ActionDirect {
		t.Errorf("Match with IP = %q, want %q", got, ActionDirect)
	}

	// Default fallback.
	got = r.Match("", nil, "", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("Match default = %q, want %q", got, ActionProxy)
	}
}

func TestRouterDefaultActionFallback(t *testing.T) {
	// Default action should be used when nothing matches.
	r := newTestRouter(nil, ActionDirect)
	got := r.Match("anything.com", net.ParseIP("1.2.3.4"), "someproc", "someproto", 0, nil)
	if got != ActionDirect {
		t.Errorf("Match with all defaults = %q, want %q", got, ActionDirect)
	}
}

func TestRouterDefaultActionEmpty(t *testing.T) {
	// Empty default action should default to "proxy".
	r := newTestRouter(nil, "")
	got := r.Match("", nil, "", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("Match with empty default = %q, want %q", got, ActionProxy)
	}
}

// ---------------------------------------------------------------------------
// DryRun tests
// ---------------------------------------------------------------------------

func TestDryRun_DomainMatch(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect},
		{Type: "domain", Values: []string{"+.blocked.com"}, Action: ActionReject},
	}, ActionProxy)

	// Exact domain match
	result := r.DryRun("example.com")
	if result.Action != "direct" {
		t.Errorf("DryRun(example.com).Action = %q, want \"direct\"", result.Action)
	}
	if result.MatchedBy != "domain_rule" {
		t.Errorf("DryRun(example.com).MatchedBy = %q, want \"domain_rule\"", result.MatchedBy)
	}
	if result.Domain != "example.com" {
		t.Errorf("DryRun(example.com).Domain = %q, want \"example.com\"", result.Domain)
	}

	// Wildcard domain match
	result = r.DryRun("sub.blocked.com")
	if result.Action != "reject" {
		t.Errorf("DryRun(sub.blocked.com).Action = %q, want \"reject\"", result.Action)
	}
	if result.MatchedBy != "domain_rule" {
		t.Errorf("DryRun(sub.blocked.com).MatchedBy = %q, want \"domain_rule\"", result.MatchedBy)
	}
}

func TestDryRun_NoMatch(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect},
	}, ActionProxy)

	result := r.DryRun("unknown.org")
	if result.Action != "proxy" {
		t.Errorf("DryRun(unknown.org).Action = %q, want \"proxy\"", result.Action)
	}
	if result.MatchedBy != "default" {
		t.Errorf("DryRun(unknown.org).MatchedBy = %q, want \"default\"", result.MatchedBy)
	}
}

func TestDryRun_EmptyDomain(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect},
	}, ActionProxy)

	result := r.DryRun("")
	if result.Action != "proxy" {
		t.Errorf("DryRun(\"\").Action = %q, want \"proxy\"", result.Action)
	}
	if result.MatchedBy != "default" {
		t.Errorf("DryRun(\"\").MatchedBy = %q, want \"default\"", result.MatchedBy)
	}
}

func TestDryRun_DefaultDirect(t *testing.T) {
	r := newTestRouter(nil, ActionDirect)

	result := r.DryRun("anything.com")
	if result.Action != "direct" {
		t.Errorf("DryRun(anything.com).Action = %q, want \"direct\"", result.Action)
	}
	if result.MatchedBy != "default" {
		t.Errorf("DryRun(anything.com).MatchedBy = %q, want \"default\"", result.MatchedBy)
	}
}

// ---------------------------------------------------------------------------
// Network-type routing tests
// ---------------------------------------------------------------------------

func TestRouterNetworkTypeRuleDomainMatch(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect, NetworkType: "wifi"},
	}, ActionProxy)

	// Without setting network type, the network rule should not match.
	got := r.Match("example.com", nil, "", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("Match without network type = %q, want %q", got, ActionProxy)
	}

	// Set network type to wifi — now it should match.
	r.SetNetworkType("wifi")
	got = r.Match("example.com", nil, "", "", 0, nil)
	if got != ActionDirect {
		t.Errorf("Match with wifi = %q, want %q", got, ActionDirect)
	}

	// Set network type to cellular — should not match the wifi rule.
	r.SetNetworkType("cellular")
	got = r.Match("example.com", nil, "", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("Match with cellular = %q, want %q", got, ActionProxy)
	}
}

func TestRouterNetworkTypeRuleIPMatch(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "ip-cidr", Values: []string{"10.0.0.0/8"}, Action: ActionDirect, NetworkType: "cellular"},
	}, ActionProxy)

	// Without network type, should fall through to default.
	got := r.Match("", net.ParseIP("10.1.2.3"), "", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("Match IP without network type = %q, want %q", got, ActionProxy)
	}

	// Set to cellular — should match.
	r.SetNetworkType("cellular")
	got = r.Match("", net.ParseIP("10.1.2.3"), "", "", 0, nil)
	if got != ActionDirect {
		t.Errorf("Match IP with cellular = %q, want %q", got, ActionDirect)
	}
}

func TestRouterNetworkTypeRuleProcessMatch(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "process", Values: []string{"chrome"}, Action: ActionReject, NetworkType: "ethernet"},
	}, ActionProxy)

	r.SetNetworkType("ethernet")
	got := r.Match("", nil, "chrome", "", 0, nil)
	if got != ActionReject {
		t.Errorf("Match process with ethernet = %q, want %q", got, ActionReject)
	}

	r.SetNetworkType("wifi")
	got = r.Match("", nil, "chrome", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("Match process with wifi = %q, want %q", got, ActionProxy)
	}
}

func TestRouterNetworkTypeRuleProtocolMatch(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "protocol", Values: []string{"bittorrent"}, Action: ActionReject, NetworkType: "cellular"},
	}, ActionProxy)

	r.SetNetworkType("cellular")
	got := r.Match("", nil, "", "bittorrent", 0, nil)
	if got != ActionReject {
		t.Errorf("Match protocol with cellular = %q, want %q", got, ActionReject)
	}

	r.SetNetworkType("wifi")
	got = r.Match("", nil, "", "bittorrent", 0, nil)
	if got != ActionProxy {
		t.Errorf("Match protocol with wifi = %q, want %q", got, ActionProxy)
	}
}

func TestRouterNetworkTypeGetSet(t *testing.T) {
	r := newTestRouter(nil, ActionProxy)

	if got := r.NetworkType(); got != "" {
		t.Errorf("initial NetworkType() = %q, want empty", got)
	}

	r.SetNetworkType("WiFi")
	if got := r.NetworkType(); got != "wifi" {
		t.Errorf("NetworkType() = %q, want \"wifi\"", got)
	}
}

func TestRouterNetworkTypePriorityOverRegular(t *testing.T) {
	// Network-type rules should take priority over regular rules.
	r := newTestRouter([]Rule{
		{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect},                              // regular
		{Type: "domain", Values: []string{"example.com"}, Action: ActionReject, NetworkType: "cellular"}, // network-type
	}, ActionProxy)

	// When on cellular, the network-type rule should win.
	r.SetNetworkType("cellular")
	got := r.Match("example.com", nil, "", "", 0, nil)
	if got != ActionReject {
		t.Errorf("Match with network-type rule on cellular = %q, want %q", got, ActionReject)
	}

	// When on wifi, the regular rule should apply.
	r.SetNetworkType("wifi")
	got = r.Match("example.com", nil, "", "", 0, nil)
	if got != ActionDirect {
		t.Errorf("Match with regular rule on wifi = %q, want %q", got, ActionDirect)
	}
}

func TestRouterNetworkTypeWildcardDomain(t *testing.T) {
	r := newTestRouter([]Rule{
		{Type: "domain", Values: []string{"+.example.com"}, Action: ActionDirect, NetworkType: "wifi"},
	}, ActionProxy)

	r.SetNetworkType("wifi")

	got := r.Match("sub.example.com", nil, "", "", 0, nil)
	if got != ActionDirect {
		t.Errorf("Match wildcard subdomain with wifi = %q, want %q", got, ActionDirect)
	}

	got = r.Match("example.com", nil, "", "", 0, nil)
	if got != ActionDirect {
		t.Errorf("Match bare domain with wifi = %q, want %q", got, ActionDirect)
	}

	got = r.Match("other.com", nil, "", "", 0, nil)
	if got != ActionProxy {
		t.Errorf("Match non-matching domain with wifi = %q, want %q", got, ActionProxy)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkRouterMatchWithNetworkType(b *testing.B) {
	r := newTestRouter([]Rule{
		{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect, NetworkType: "wifi"},
		{Type: "domain", Values: []string{"+.google.com"}, Action: ActionProxy, NetworkType: "cellular"},
		{Type: "ip-cidr", Values: []string{"10.0.0.0/8"}, Action: ActionDirect, NetworkType: "wifi"},
		{Type: "domain", Values: []string{"fallback.com"}, Action: ActionDirect},
	}, ActionProxy)
	r.SetNetworkType("wifi")

	ip := net.ParseIP("10.1.2.3")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Match("example.com", ip, "", "", 0, nil)
	}
}

// ---------------------------------------------------------------------------
// DNSCache tests
// ---------------------------------------------------------------------------

func newTestCache(maxSize int) *dnsCache {
	return &dnsCache{
		entries: make(map[string]*dnsCacheEntry, maxSize),
		maxSize: maxSize,
	}
}

func TestDNSCachePutAndGet(t *testing.T) {
	c := newTestCache(100)
	ips := []net.IP{net.ParseIP("1.2.3.4")}
	c.put("example.com", ips, 5*time.Minute, false)

	got, ok := c.get("example.com")
	if !ok {
		t.Fatal("get(example.com) returned miss, want hit")
	}
	if len(got) != 1 || !got[0].Equal(ips[0]) {
		t.Errorf("get(example.com) = %v, want %v", got, ips)
	}
}

func TestDNSCacheMiss(t *testing.T) {
	c := newTestCache(100)
	_, ok := c.get("nonexistent.com")
	if ok {
		t.Error("get(nonexistent.com) returned hit on empty cache")
	}
}

func TestDNSCacheExpired(t *testing.T) {
	c := newTestCache(100)
	ips := []net.IP{net.ParseIP("1.2.3.4")}
	// Use a very short TTL so the entry expires immediately.
	c.put("example.com", ips, 1*time.Millisecond, false)

	// Wait for expiration.
	time.Sleep(5 * time.Millisecond)

	_, ok := c.get("example.com")
	if ok {
		t.Error("get(example.com) returned hit for expired entry")
	}
}

func TestDNSCacheEvictionWhenFull(t *testing.T) {
	maxSize := 5
	c := newTestCache(maxSize)

	// Fill the cache to capacity.
	for i := 0; i < maxSize; i++ {
		domain := net.JoinHostPort("d"+string(rune('a'+i))+".com", "")
		// Use a simple naming scheme.
		d := "d" + string(rune('a'+i)) + ".com"
		_ = domain
		c.put(d, []net.IP{net.ParseIP("1.2.3.4")}, 5*time.Minute, false)
	}

	if len(c.entries) != maxSize {
		t.Fatalf("cache size = %d, want %d", len(c.entries), maxSize)
	}

	// Adding one more should trigger eviction. The cache should not exceed maxSize.
	c.put("overflow.com", []net.IP{net.ParseIP("5.6.7.8")}, 5*time.Minute, false)

	if len(c.entries) > maxSize {
		t.Errorf("cache size after eviction = %d, want <= %d", len(c.entries), maxSize)
	}

	// The newly inserted entry should be present.
	_, ok := c.get("overflow.com")
	if !ok {
		t.Error("get(overflow.com) returned miss after insert")
	}
}

func TestDNSCacheEvictsExpiredFirst(t *testing.T) {
	maxSize := 3
	c := newTestCache(maxSize)

	// Insert entries with very short TTL so they expire.
	c.put("expired1.com", []net.IP{net.ParseIP("1.1.1.1")}, 1*time.Millisecond, false)
	c.put("expired2.com", []net.IP{net.ParseIP("2.2.2.2")}, 1*time.Millisecond, false)
	// One long-lived entry.
	c.put("alive.com", []net.IP{net.ParseIP("3.3.3.3")}, 5*time.Minute, false)

	time.Sleep(5 * time.Millisecond)

	// Cache is at capacity. Inserting should evict expired entries first.
	c.put("new.com", []net.IP{net.ParseIP("4.4.4.4")}, 5*time.Minute, false)

	// The alive entry should still be present.
	_, ok := c.get("alive.com")
	if !ok {
		t.Error("get(alive.com) returned miss, expected it to survive eviction")
	}

	// New entry should be present.
	_, ok = c.get("new.com")
	if !ok {
		t.Error("get(new.com) returned miss after insert")
	}
}

func TestDNSCacheOverwrite(t *testing.T) {
	c := newTestCache(100)
	c.put("example.com", []net.IP{net.ParseIP("1.1.1.1")}, 5*time.Minute, false)
	c.put("example.com", []net.IP{net.ParseIP("2.2.2.2")}, 5*time.Minute, false)

	got, ok := c.get("example.com")
	if !ok {
		t.Fatal("get(example.com) returned miss")
	}
	if !got[0].Equal(net.ParseIP("2.2.2.2")) {
		t.Errorf("get(example.com) = %v, want [2.2.2.2]", got)
	}
}
