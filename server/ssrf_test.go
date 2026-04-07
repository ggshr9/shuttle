package server

import (
	"net"
	"testing"
)

func TestIsBlockedTarget_PrivateRanges(t *testing.T) {
	blocked := []string{
		"10.0.0.1:80", "172.16.0.1:443", "192.168.1.1:22",
		"127.0.0.1:8080", "169.254.169.254:80",
		"[::1]:80", "[fe80::1]:443",
	}
	for _, addr := range blocked {
		if !IsBlockedTarget(addr) {
			t.Errorf("IsBlockedTarget(%q) should be true", addr)
		}
	}
}

func TestIsBlockedTarget_PublicRanges(t *testing.T) {
	allowed := []string{"8.8.8.8:53", "1.1.1.1:443", "93.184.216.34:80"}
	for _, addr := range allowed {
		if IsBlockedTarget(addr) {
			t.Errorf("IsBlockedTarget(%q) should be false", addr)
		}
	}
}

func TestIsBlockedIP_Direct(t *testing.T) {
	if !IsBlockedIP(net.ParseIP("10.0.0.1")) {
		t.Error("10.0.0.1 should be blocked")
	}
	if !IsBlockedIP(net.ParseIP("::1")) {
		t.Error("::1 should be blocked")
	}
	if IsBlockedIP(net.ParseIP("8.8.8.8")) {
		t.Error("8.8.8.8 should not be blocked")
	}
}
