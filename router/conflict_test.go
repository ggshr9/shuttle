package router

import (
	"testing"
)

func TestDetectConflictsNone(t *testing.T) {
	rules := []Rule{
		{Type: "domain", Values: []string{"google.com"}, Action: ActionProxy},
		{Type: "domain", Values: []string{"baidu.com"}, Action: ActionDirect},
	}
	conflicts := DetectConflicts(rules, nil)
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(conflicts))
	}
}

func TestDetectConflictsSameDomainDifferentAction(t *testing.T) {
	rules := []Rule{
		{Type: "domain", Values: []string{"example.com"}, Action: ActionProxy},
		{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect},
	}
	conflicts := DetectConflicts(rules, nil)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Domain != "example.com" {
		t.Fatalf("expected conflict on example.com, got %s", conflicts[0].Domain)
	}
	if conflicts[0].Action1 != "proxy" || conflicts[0].Action2 != "direct" {
		t.Fatalf("expected proxy vs direct, got %s vs %s", conflicts[0].Action1, conflicts[0].Action2)
	}
}

func TestDetectConflictsSameActionNoConflict(t *testing.T) {
	rules := []Rule{
		{Type: "domain", Values: []string{"example.com"}, Action: ActionProxy},
		{Type: "domain", Values: []string{"example.com"}, Action: ActionProxy},
	}
	conflicts := DetectConflicts(rules, nil)
	if len(conflicts) != 0 {
		t.Fatalf("same action should not conflict, got %d", len(conflicts))
	}
}

func TestDetectConflictsGeoSiteVsDomain(t *testing.T) {
	geoSite := NewGeoSiteDB()
	geoSite.LoadCategory("cn", []string{"baidu.com", "taobao.com"})

	rules := []Rule{
		{Type: "geosite", Values: []string{"cn"}, Action: ActionDirect},
		{Type: "domain", Values: []string{"baidu.com"}, Action: ActionProxy},
	}
	conflicts := DetectConflicts(rules, geoSite)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict (geosite vs domain), got %d", len(conflicts))
	}
	if conflicts[0].Domain != "baidu.com" {
		t.Fatalf("expected conflict on baidu.com, got %s", conflicts[0].Domain)
	}
}

func TestDetectConflictsMultipleDomains(t *testing.T) {
	rules := []Rule{
		{Type: "domain", Values: []string{"a.com", "b.com", "c.com"}, Action: ActionProxy},
		{Type: "domain", Values: []string{"b.com", "c.com"}, Action: ActionDirect},
	}
	conflicts := DetectConflicts(rules, nil)
	if len(conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d", len(conflicts))
	}
}

func TestDetectConflictsIgnoresNonDomainRules(t *testing.T) {
	rules := []Rule{
		{Type: "process", Values: []string{"chrome"}, Action: ActionProxy},
		{Type: "ip-cidr", Values: []string{"10.0.0.0/8"}, Action: ActionDirect},
		{Type: "protocol", Values: []string{"bittorrent"}, Action: ActionReject},
	}
	conflicts := DetectConflicts(rules, nil)
	if len(conflicts) != 0 {
		t.Fatalf("non-domain rules should not conflict, got %d", len(conflicts))
	}
}

func TestDetectConflictsCaseInsensitive(t *testing.T) {
	rules := []Rule{
		{Type: "domain", Values: []string{"Example.COM"}, Action: ActionProxy},
		{Type: "domain", Values: []string{"example.com"}, Action: ActionDirect},
	}
	conflicts := DetectConflicts(rules, nil)
	if len(conflicts) != 1 {
		t.Fatalf("expected case-insensitive conflict detection, got %d", len(conflicts))
	}
}

func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"example.com", "example.com"},
		{"+.example.com", "example.com"},
		{"example.com.", "example.com"},
		{"Example.COM", "example.com"},
	}
	for _, tc := range tests {
		got := normalizeDomain(tc.input)
		if got != tc.want {
			t.Errorf("normalizeDomain(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
