package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRuleProvider_DomainBehavior(t *testing.T) {
	body := "google.com\nfacebook.com\ntwitter.com\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p, err := NewRuleProvider(RuleProviderConfig{
		Name:     "test-domain",
		URL:      srv.URL,
		Behavior: "domain",
		Interval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewRuleProvider: %v", err)
	}
	allowLoopbackRule(p)

	if err := p.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	tests := []struct {
		domain string
		want   bool
	}{
		{"google.com", true},          // exact match
		{"www.google.com", true},      // suffix match
		{"sub.google.com", true},      // suffix match
		{"facebook.com", true},        // exact match
		{"m.facebook.com", true},      // suffix match
		{"twitter.com", true},         // exact match
		{"example.com", false},        // not in list
		{"notgoogle.com", false},      // doesn't match suffix ".google.com"
		{"GOOGLE.COM", true},          // case-insensitive exact
		{"WWW.GOOGLE.COM", true},      // case-insensitive suffix
	}

	for _, tc := range tests {
		got := p.MatchDomain(tc.domain)
		if got != tc.want {
			t.Errorf("MatchDomain(%q) = %v, want %v", tc.domain, got, tc.want)
		}
	}
}

func TestRuleProvider_IPCIDRBehavior(t *testing.T) {
	body := "10.0.0.0/8\n172.16.0.0/12\n192.168.0.0/16\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p, err := NewRuleProvider(RuleProviderConfig{
		Name:     "test-ipcidr",
		URL:      srv.URL,
		Behavior: "ipcidr",
		Interval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewRuleProvider: %v", err)
	}
	allowLoopbackRule(p)

	if err := p.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	tests := []struct {
		ip   string
		want bool
	}{
		{"10.1.2.3", true},      // in 10.0.0.0/8
		{"10.255.255.255", true}, // in 10.0.0.0/8
		{"172.16.0.1", true},    // in 172.16.0.0/12
		{"172.31.255.255", true}, // in 172.16.0.0/12
		{"192.168.1.1", true},   // in 192.168.0.0/16
		{"192.168.255.255", true},
		{"8.8.8.8", false},      // not in any CIDR
		{"1.1.1.1", false},      // not in any CIDR
		{"11.0.0.1", false},     // not in 10.0.0.0/8
	}

	for _, tc := range tests {
		got := p.MatchIP(tc.ip)
		if got != tc.want {
			t.Errorf("MatchIP(%q) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestRuleProvider_ClassicalBehavior(t *testing.T) {
	body := "DOMAIN-SUFFIX,google.com\nIP-CIDR,8.8.8.0/24\nDOMAIN-KEYWORD,facebook\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p, err := NewRuleProvider(RuleProviderConfig{
		Name:     "test-classical",
		URL:      srv.URL,
		Behavior: "classical",
		Interval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewRuleProvider: %v", err)
	}
	allowLoopbackRule(p)

	if err := p.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Domain matching
	domainTests := []struct {
		domain string
		want   bool
	}{
		{"www.google.com", true},     // DOMAIN-SUFFIX,google.com
		{"google.com", true},         // suffix matches bare domain too
		{"m.facebook.com", true},     // DOMAIN-KEYWORD,facebook
		{"facebook.net", true},       // DOMAIN-KEYWORD,facebook
		{"example.com", false},
		{"twitter.com", false},       // not in classical rules
	}

	for _, tc := range domainTests {
		got := p.MatchDomain(tc.domain)
		if got != tc.want {
			t.Errorf("MatchDomain(%q) = %v, want %v", tc.domain, got, tc.want)
		}
	}

	// IP matching
	ipTests := []struct {
		ip   string
		want bool
	}{
		{"8.8.8.1", true},   // IP-CIDR,8.8.8.0/24
		{"8.8.8.255", true},
		{"8.8.4.4", false},
		{"1.1.1.1", false},
	}

	for _, tc := range ipTests {
		got := p.MatchIP(tc.ip)
		if got != tc.want {
			t.Errorf("MatchIP(%q) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestRuleProvider_ClassicalBehavior_DomainExact(t *testing.T) {
	body := "DOMAIN,example.com\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p, err := NewRuleProvider(RuleProviderConfig{
		Name:     "test-classical-domain",
		URL:      srv.URL,
		Behavior: "classical",
		Interval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewRuleProvider: %v", err)
	}
	allowLoopbackRule(p)
	if err := p.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if !p.MatchDomain("example.com") {
		t.Error("MatchDomain(example.com) should be true for DOMAIN,example.com")
	}
	// DOMAIN rule is exact — subdomain should NOT match
	if p.MatchDomain("sub.example.com") {
		t.Error("MatchDomain(sub.example.com) should be false for exact DOMAIN rule")
	}
}

func TestRuleProvider_ClassicalBehavior_IPv6CIDR(t *testing.T) {
	body := "IP-CIDR6,2001:db8::/32\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p, err := NewRuleProvider(RuleProviderConfig{
		Name:     "test-classical-ipv6",
		URL:      srv.URL,
		Behavior: "classical",
		Interval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewRuleProvider: %v", err)
	}
	allowLoopbackRule(p)
	if err := p.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if !p.MatchIP("2001:db8::1") {
		t.Error("MatchIP(2001:db8::1) should be true for 2001:db8::/32")
	}
	if p.MatchIP("2001:db9::1") {
		t.Error("MatchIP(2001:db9::1) should be false")
	}
}

func TestRuleProvider_InvalidBehavior(t *testing.T) {
	_, err := NewRuleProvider(RuleProviderConfig{
		Name:     "bad",
		URL:      "http://example.com",
		Behavior: "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid behavior")
	}
}

func TestRuleProvider_EmptyBehavior(t *testing.T) {
	_, err := NewRuleProvider(RuleProviderConfig{
		Name: "bad",
		URL:  "http://example.com",
	})
	if err == nil {
		t.Fatal("expected error for empty behavior")
	}
}

func TestRuleProvider_InvalidURL(t *testing.T) {
	_, err := NewRuleProvider(RuleProviderConfig{
		Name:     "bad",
		URL:      "ftp://example.com",
		Behavior: "domain",
	})
	if err == nil {
		t.Fatal("expected error for non-http URL")
	}
}

func TestRuleProvider_EmptyURL(t *testing.T) {
	_, err := NewRuleProvider(RuleProviderConfig{
		Name:     "bad",
		Behavior: "domain",
	})
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestRuleProvider_Metadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("example.com\n"))
	}))
	defer srv.Close()

	p, err := NewRuleProvider(RuleProviderConfig{
		Name:     "meta-test",
		URL:      srv.URL,
		Behavior: "domain",
		Interval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewRuleProvider: %v", err)
	}
	allowLoopbackRule(p)

	if p.Name() != "meta-test" {
		t.Errorf("Name() = %q, want %q", p.Name(), "meta-test")
	}
	if p.Behavior() != "domain" {
		t.Errorf("Behavior() = %q, want %q", p.Behavior(), "domain")
	}

	before := time.Now()
	if err := p.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	after := time.Now()

	if p.Error() != nil {
		t.Errorf("Error() = %v after successful refresh", p.Error())
	}
	if p.UpdatedAt().Before(before) || p.UpdatedAt().After(after) {
		t.Errorf("UpdatedAt() = %v, expected between %v and %v", p.UpdatedAt(), before, after)
	}
}

func TestRuleProvider_NoMatchOnEmptyRules(t *testing.T) {
	p, err := NewRuleProvider(RuleProviderConfig{
		Name:     "empty",
		URL:      "http://127.0.0.1:1", // won't be called
		Behavior: "domain",
		Interval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewRuleProvider: %v", err)
	}

	if p.MatchDomain("google.com") {
		t.Error("MatchDomain should return false when no rules are loaded")
	}
	if p.MatchIP("8.8.8.8") {
		t.Error("MatchIP should return false when no rules are loaded")
	}
}

func TestRuleProvider_CommentsAndBlanks(t *testing.T) {
	body := "# this is a comment\n\ngoogle.com\n  # indented comment\n  \nfacebook.com\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p, err := NewRuleProvider(RuleProviderConfig{
		Name:     "comments",
		URL:      srv.URL,
		Behavior: "domain",
		Interval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewRuleProvider: %v", err)
	}
	allowLoopbackRule(p)
	if err := p.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if !p.MatchDomain("google.com") {
		t.Error("google.com should match after skipping comments and blanks")
	}
	if !p.MatchDomain("facebook.com") {
		t.Error("facebook.com should match after skipping comments and blanks")
	}
}

func TestRuleProvider_StopDoesNotPanic(t *testing.T) {
	p, err := NewRuleProvider(RuleProviderConfig{
		Name:     "stop-test",
		URL:      "http://127.0.0.1:1",
		Behavior: "domain",
		Interval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewRuleProvider: %v", err)
	}
	// Stop without Start should not panic.
	p.Stop()
}

func TestRuleProvider_StartStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("example.com\n"))
	}))
	defer srv.Close()

	p, err := NewRuleProvider(RuleProviderConfig{
		Name:     "start-stop",
		URL:      srv.URL,
		Behavior: "domain",
		Interval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewRuleProvider: %v", err)
	}
	allowLoopbackRule(p)

	ctx := context.Background()
	p.Start(ctx)

	// Give initial fetch a moment.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if p.MatchDomain("example.com") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !p.MatchDomain("example.com") {
		t.Error("expected example.com to match after Start")
	}

	p.Stop()
}
