package test

import (
	"net"
	"testing"

	"github.com/shuttle-proxy/shuttle/router"
)

func TestDomainTrie(t *testing.T) {
	trie := router.NewDomainTrie()

	trie.Insert("google.com", "proxy")
	trie.Insert("baidu.com", "direct")
	trie.Insert("+.cn", "direct") // All .cn domains

	tests := []struct {
		domain string
		want   string
		found  bool
	}{
		{"google.com", "proxy", true},
		{"www.google.com", "proxy", true}, // suffix match: google.com matches subdomains
		{"baidu.com", "direct", true},
		{"example.cn", "direct", true},    // wildcard .cn
		{"sub.example.cn", "direct", true}, // wildcard .cn
		{"example.com", "", false},
	}

	for _, tt := range tests {
		action, found := trie.Lookup(tt.domain)
		if found != tt.found {
			t.Errorf("Lookup(%q): found=%v, want %v", tt.domain, found, tt.found)
		}
		if found && action != tt.want {
			t.Errorf("Lookup(%q): action=%q, want %q", tt.domain, action, tt.want)
		}
	}
}

func TestRouter(t *testing.T) {
	rt := router.NewRouter(&router.RouterConfig{
		Rules: []router.Rule{
			{Type: "domain", Values: []string{"google.com"}, Action: router.ActionProxy},
			{Type: "domain", Values: []string{"baidu.com"}, Action: router.ActionDirect},
			{Type: "process", Values: []string{"WeChat"}, Action: router.ActionDirect},
			{Type: "protocol", Values: []string{"bittorrent"}, Action: router.ActionDirect},
		},
		DefaultAction: router.ActionProxy,
	}, nil, nil, nil)

	if rt.MatchDomain("google.com") != router.ActionProxy {
		t.Error("google.com should be proxy")
	}
	if rt.MatchDomain("baidu.com") != router.ActionDirect {
		t.Error("baidu.com should be direct")
	}
	if rt.MatchDomain("unknown.com") != router.ActionProxy {
		t.Error("unknown should be proxy (default)")
	}
	if rt.MatchProcess("WeChat") != router.ActionDirect {
		t.Error("WeChat should be direct")
	}
	if rt.MatchProtocol("bittorrent") != router.ActionDirect {
		t.Error("bittorrent should be direct")
	}
}

func TestRouterIPMatch(t *testing.T) {
	rt := router.NewRouter(&router.RouterConfig{
		Rules: []router.Rule{
			{Type: "ip-cidr", Values: []string{"192.168.0.0/16"}, Action: router.ActionDirect},
			{Type: "ip-cidr", Values: []string{"10.0.0.0/8"}, Action: router.ActionDirect},
		},
		DefaultAction: router.ActionProxy,
	}, nil, nil, nil)

	if rt.MatchIP(net.ParseIP("192.168.1.1")) != router.ActionDirect {
		t.Error("192.168.1.1 should be direct")
	}
	if rt.MatchIP(net.ParseIP("8.8.8.8")) != router.ActionProxy {
		t.Error("8.8.8.8 should be proxy")
	}
}
