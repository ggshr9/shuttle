package router

import (
	"fmt"
	"net"
	"strings"
)

// escapeJSString escapes a string for safe embedding inside a JavaScript double-quoted string.
func escapeJSString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	return s
}

// PACConfig configures PAC file generation.
type PACConfig struct {
	HTTPProxyAddr  string // e.g., "127.0.0.1:8080"
	SOCKSProxyAddr string // e.g., "127.0.0.1:1080"
	DefaultAction  Action // proxy or direct
}

// GeneratePAC creates a PAC (Proxy Auto-Config) file from routing rules.
// The PAC file uses the domain trie and CIDR rules to route traffic.
func GeneratePAC(r *Router, cfg *PACConfig) string {
	if cfg == nil {
		cfg = &PACConfig{
			HTTPProxyAddr:  "127.0.0.1:8080",
			SOCKSProxyAddr: "127.0.0.1:1080",
			DefaultAction:  ActionProxy,
		}
	}

	proxyStr := fmt.Sprintf("SOCKS5 %s; PROXY %s; DIRECT", cfg.SOCKSProxyAddr, cfg.HTTPProxyAddr)
	directStr := "DIRECT"

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Exact-match domains (RuleTypeDomain): emit as `host === "..."` so
	// they don't get the suffix semantics of dnsDomainIs.
	var exactProxy, exactDirect, exactReject []string
	for d, act := range r.exactDomain {
		switch act {
		case ActionProxy:
			exactProxy = append(exactProxy, d)
		case ActionDirect:
			exactDirect = append(exactDirect, d)
		case ActionReject:
			exactReject = append(exactReject, d)
		}
	}

	// Collect suffix domain rules from trie (RuleTypeDomainSuffix + GeoSite).
	var proxyDomains, directDomains, rejectDomains []string
	collectDomains(r.domainTrie.root, nil, &proxyDomains, &directDomains, &rejectDomains)

	// Collect CIDR rules
	var proxyCIDRs, directCIDRs []string
	for _, rule := range r.ipRules {
		cidr := rule.cidr.String()
		switch rule.action {
		case ActionProxy:
			proxyCIDRs = append(proxyCIDRs, cidr)
		case ActionDirect:
			directCIDRs = append(directCIDRs, cidr)
		}
	}

	var sb strings.Builder
	sb.WriteString("// Shuttle PAC - Auto-generated Proxy Auto-Config\n")
	sb.WriteString("// Do not edit manually; regenerate via Shuttle Settings.\n\n")
	sb.WriteString("function FindProxyForURL(url, host) {\n")

	// Private/localhost → always direct
	sb.WriteString("  // Private networks and localhost\n")
	sb.WriteString("  if (isPlainHostName(host) ||\n")
	sb.WriteString("      shExpMatch(host, \"*.local\") ||\n")
	sb.WriteString("      isInNet(host, \"127.0.0.0\", \"255.0.0.0\") ||\n")
	sb.WriteString("      isInNet(host, \"10.0.0.0\", \"255.0.0.0\") ||\n")
	sb.WriteString("      isInNet(host, \"172.16.0.0\", \"255.240.0.0\") ||\n")
	sb.WriteString("      isInNet(host, \"192.168.0.0\", \"255.255.0.0\")) {\n")
	sb.WriteString("    return \"DIRECT\";\n")
	sb.WriteString("  }\n\n")

	// Exact-match domains first (precise wins over suffix).
	if len(exactReject) > 0 {
		sb.WriteString("  // Exact reject domains\n")
		for _, d := range exactReject {
			sb.WriteString(fmt.Sprintf("  if (host === %q) return \"PROXY 0.0.0.0:1\";\n", escapeJSString(d)))
		}
		sb.WriteString("\n")
	}
	if len(exactDirect) > 0 {
		sb.WriteString("  // Exact direct domains\n")
		for _, d := range exactDirect {
			sb.WriteString(fmt.Sprintf("  if (host === %q) return \"DIRECT\";\n", escapeJSString(d)))
		}
		sb.WriteString("\n")
	}
	if len(exactProxy) > 0 {
		sb.WriteString("  // Exact proxy domains\n")
		for _, d := range exactProxy {
			sb.WriteString(fmt.Sprintf("  if (host === %q) return %q;\n", escapeJSString(d), proxyStr))
		}
		sb.WriteString("\n")
	}

	// Suffix domains (dnsDomainIs).
	if len(rejectDomains) > 0 {
		sb.WriteString("  // Rejected domains (ads/tracking)\n")
		for _, d := range rejectDomains {
			sb.WriteString(fmt.Sprintf("  if (dnsDomainIs(host, %q)) return \"PROXY 0.0.0.0:1\";\n", escapeJSString(d)))
		}
		sb.WriteString("\n")
	}

	if len(directDomains) > 0 {
		sb.WriteString("  // Direct suffix domains\n")
		for _, d := range directDomains {
			sb.WriteString(fmt.Sprintf("  if (dnsDomainIs(host, %q)) return \"DIRECT\";\n", escapeJSString(d)))
		}
		sb.WriteString("\n")
	}

	if len(proxyDomains) > 0 {
		sb.WriteString("  // Proxy suffix domains\n")
		for _, d := range proxyDomains {
			sb.WriteString(fmt.Sprintf("  if (dnsDomainIs(host, %q)) return %q;\n", escapeJSString(d), proxyStr))
		}
		sb.WriteString("\n")
	}

	// CIDR rules
	if len(directCIDRs) > 0 {
		sb.WriteString("  // Direct CIDRs\n")
		for _, cidr := range directCIDRs {
			ip, mask := cidrToNetMask(cidr)
			if ip != "" {
				sb.WriteString(fmt.Sprintf("  if (isInNet(host, %q, %q)) return \"DIRECT\";\n", ip, mask))
			}
		}
		sb.WriteString("\n")
	}

	if len(proxyCIDRs) > 0 {
		sb.WriteString("  // Proxy CIDRs\n")
		for _, cidr := range proxyCIDRs {
			ip, mask := cidrToNetMask(cidr)
			if ip != "" {
				sb.WriteString(fmt.Sprintf("  if (isInNet(host, %q, %q)) return %q;\n", ip, mask, proxyStr))
			}
		}
		sb.WriteString("\n")
	}

	// Default
	sb.WriteString("  // Default action\n")
	if cfg.DefaultAction == ActionDirect {
		sb.WriteString(fmt.Sprintf("  return %q;\n", directStr))
	} else {
		sb.WriteString(fmt.Sprintf("  return %q;\n", proxyStr))
	}

	sb.WriteString("}\n")
	return sb.String()
}

// collectDomains walks the trie to extract domain→action mappings.
func collectDomains(node *trieNode, parts []string, proxy, direct, reject *[]string) {
	if node.isEnd {
		// Reconstruct domain from reversed parts
		domain := reconstructDomain(parts)
		switch Action(node.value) {
		case ActionProxy:
			*proxy = append(*proxy, domain)
		case ActionDirect:
			*direct = append(*direct, domain)
		case ActionReject:
			*reject = append(*reject, domain)
		}
	}
	for label, child := range node.children {
		collectDomains(child, append(parts, label), proxy, direct, reject)
	}
}

// reconstructDomain reverses the trie path back to a domain name.
func reconstructDomain(reversedParts []string) string {
	n := len(reversedParts)
	result := make([]string, n)
	for i, part := range reversedParts {
		result[n-1-i] = part
	}
	return strings.Join(result, ".")
}

// cidrToNetMask converts "10.0.0.0/8" to ("10.0.0.0", "255.0.0.0") for PAC isInNet().
func cidrToNetMask(cidr string) (string, string) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", ""
	}
	ip := ipNet.IP.To4()
	if ip == nil {
		return "", "" // IPv6 not supported in PAC
	}
	mask := net.IP(ipNet.Mask).To4()
	return ip.String(), fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
}
