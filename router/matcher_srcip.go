package router

import (
	"fmt"
	"net"
)

// srcIPMatcher matches the source IP address against a set of CIDRs.
type srcIPMatcher struct {
	nets []*net.IPNet
}

// newSrcIPMatcher parses CIDR strings (e.g. "192.168.1.0/24") and bare IPs
// (e.g. "10.0.0.1", treated as /32 or /128).
func newSrcIPMatcher(specs []string) (*srcIPMatcher, error) {
	m := &srcIPMatcher{}
	for _, spec := range specs {
		// Try CIDR first.
		_, ipnet, err := net.ParseCIDR(spec)
		if err == nil {
			m.nets = append(m.nets, ipnet)
			continue
		}
		// Bare IP — wrap in /32 or /128.
		ip := net.ParseIP(spec)
		if ip == nil {
			return nil, fmt.Errorf("invalid src_ip %q: not a valid CIDR or IP", spec)
		}
		if ip4 := ip.To4(); ip4 != nil {
			m.nets = append(m.nets, &net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)})
		} else {
			m.nets = append(m.nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)})
		}
	}
	if len(m.nets) == 0 {
		return nil, fmt.Errorf("no valid src_ip specs provided")
	}
	return m, nil
}

func (m *srcIPMatcher) Match(ctx *MatchContext) bool {
	if ctx.SrcIP == nil {
		return false
	}
	for _, n := range m.nets {
		if n.Contains(ctx.SrcIP) {
			return true
		}
	}
	return false
}
