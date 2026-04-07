package router

import (
	"net"
	"testing"
)

func TestSrcIPMatcher_CIDR(t *testing.T) {
	m, err := newSrcIPMatcher([]string{"192.168.1.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		ip   string
		want bool
	}{
		{"192.168.1.1", true},
		{"192.168.1.254", true},
		{"192.168.2.1", false},
		{"10.0.0.1", false},
	}
	for _, tt := range tests {
		ctx := &MatchContext{SrcIP: net.ParseIP(tt.ip)}
		if got := m.Match(ctx); got != tt.want {
			t.Errorf("srcIP %s: got %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestSrcIPMatcher_BareIP(t *testing.T) {
	m, err := newSrcIPMatcher([]string{"10.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := &MatchContext{SrcIP: net.ParseIP("10.0.0.1")}
	if !m.Match(ctx) {
		t.Error("exact IP should match")
	}
	ctx = &MatchContext{SrcIP: net.ParseIP("10.0.0.2")}
	if m.Match(ctx) {
		t.Error("different IP should not match")
	}
}

func TestSrcIPMatcher_Mixed(t *testing.T) {
	m, err := newSrcIPMatcher([]string{"10.0.0.0/8", "192.168.1.100"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		ip   string
		want bool
	}{
		{"10.1.2.3", true},
		{"192.168.1.100", true},
		{"192.168.1.101", false},
	}
	for _, tt := range tests {
		ctx := &MatchContext{SrcIP: net.ParseIP(tt.ip)}
		if got := m.Match(ctx); got != tt.want {
			t.Errorf("srcIP %s: got %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestSrcIPMatcher_NilSrcIP(t *testing.T) {
	m, err := newSrcIPMatcher([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Match(&MatchContext{}) {
		t.Error("nil SrcIP should not match")
	}
}

func TestSrcIPMatcher_Invalid(t *testing.T) {
	_, err := newSrcIPMatcher([]string{"not-an-ip"})
	if err == nil {
		t.Error("expected error for invalid spec")
	}
}

func TestSrcIPMatcher_IPv6(t *testing.T) {
	m, err := newSrcIPMatcher([]string{"::1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := &MatchContext{SrcIP: net.ParseIP("::1")}
	if !m.Match(ctx) {
		t.Error("IPv6 loopback should match")
	}
}
