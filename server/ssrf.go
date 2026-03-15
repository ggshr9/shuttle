package server

import "net"

// blockedCIDRs contains the CIDR ranges that should be blocked from proxying.
var blockedCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // private
		"172.16.0.0/12",  // private
		"192.168.0.0/16", // private
		"169.254.0.0/16", // link-local
		"0.0.0.0/8",      // unspecified
		"::1/128",        // loopback v6
		"fe80::/10",      // link-local v6
		"fc00::/7",       // unique local v6
		"::/128",         // unspecified v6
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("invalid blocked CIDR: " + cidr)
		}
		nets = append(nets, n)
	}
	return nets
}()

// IsBlockedTarget checks whether the target address points to an internal or
// private network. It resolves the hostname to IP addresses first to prevent
// DNS rebinding attacks, then checks each resolved IP against blocked ranges.
func IsBlockedTarget(target string) bool {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		// target might not have a port; try as-is
		host = target
	}

	// Resolve hostname to IPs (also handles literal IPs)
	ips, err := net.LookupHost(host)
	if err != nil {
		// If we can't resolve, check if it's a literal IP
		if ip := net.ParseIP(host); ip != nil {
			return isBlockedIP(ip)
		}
		// Can't resolve — block by default to be safe
		return true
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if isBlockedIP(ip) {
			return true
		}
	}
	return false
}

func isBlockedIP(ip net.IP) bool {
	for _, n := range blockedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
