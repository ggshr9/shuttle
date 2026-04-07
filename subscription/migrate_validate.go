package subscription

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// MigrationReport describes what will and won't survive migration from a Clash config to Shuttle.
type MigrationReport struct {
	Supported   []string `json:"supported"`
	Unsupported []string `json:"unsupported"`
	Warnings    []string `json:"warnings"`
	ServerCount int      `json:"server_count"`
	GroupCount  int      `json:"group_count"`
	RuleCount   int      `json:"rule_count"`
}

// ValidateClashMigration parses a Clash YAML config and returns a structured
// report of features that will be supported, unsupported, or require attention
// when migrating to Shuttle.
func ValidateClashMigration(data []byte) MigrationReport {
	var clash struct {
		Proxies     []map[string]any `yaml:"proxies"`
		ProxyGroups []map[string]any `yaml:"proxy-groups"`
		Rules       []string         `yaml:"rules"`
		DNS         map[string]any   `yaml:"dns"`
	}
	report := MigrationReport{}

	if err := yaml.Unmarshal(data, &clash); err != nil {
		report.Unsupported = append(report.Unsupported, "Invalid YAML: "+err.Error())
		return report
	}

	report.ServerCount = len(clash.Proxies)
	report.GroupCount = len(clash.ProxyGroups)
	report.RuleCount = len(clash.Rules)

	// Check proxy types
	supportedTypes := map[string]bool{
		"ss": true, "trojan": true, "vmess": true, "vless": true,
		"hysteria2": true, "hysteria": true, "wireguard": true, "tuic": true,
	}
	unsupportedProxyTypes := map[string]bool{}
	for _, p := range clash.Proxies {
		typ, _ := p["type"].(string)
		if typ != "" && !supportedTypes[typ] {
			unsupportedProxyTypes[typ] = true
		}
	}
	for typ := range unsupportedProxyTypes {
		report.Unsupported = append(report.Unsupported, "Proxy type not supported: "+typ)
	}

	// Check group strategies
	supportedStrategies := map[string]bool{
		"select": true, "url-test": true, "fallback": true, "load-balance": true,
	}
	for _, g := range clash.ProxyGroups {
		typ, _ := g["type"].(string)
		if typ != "" && !supportedStrategies[typ] {
			report.Warnings = append(report.Warnings, "Group strategy may differ: "+typ)
		}
	}

	// Check DNS features
	if dns := clash.DNS; dns != nil {
		if _, ok := dns["nameserver-policy"]; ok {
			report.Supported = append(report.Supported, "DNS nameserver-policy → domain_policy")
		}
		if _, ok := dns["hosts"]; ok {
			report.Supported = append(report.Supported, "DNS hosts table")
		}
		if _, ok := dns["fake-ip-filter"]; ok {
			report.Supported = append(report.Supported, "Fake-IP with domain filter")
		}
	}

	// Check rule types
	unsupportedRuleTypes := map[string]bool{}
	for _, rule := range clash.Rules {
		parts := splitRule(rule)
		if len(parts) < 2 {
			continue
		}
		ruleType := parts[0]
		switch ruleType {
		case "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "GEOIP", "GEOSITE",
			"IP-CIDR", "IP-CIDR6", "PROCESS-NAME", "MATCH":
			// supported natively
		case "SRC-IP-CIDR":
			report.Supported = append(report.Supported, "SRC-IP-CIDR rules → src_ip matcher")
		case "DST-PORT", "SRC-PORT":
			report.Supported = append(report.Supported, ruleType+" rules → port matcher")
		case "IN-TYPE", "NETWORK":
			report.Supported = append(report.Supported, ruleType+" → network_type matcher")
		case "SUB-RULE", "NOT", "AND", "OR":
			unsupportedRuleTypes[ruleType] = true
		case "RULE-SET":
			report.Supported = append(report.Supported, "RULE-SET → rule providers")
		default:
			unsupportedRuleTypes[ruleType] = true
		}
	}
	for rt := range unsupportedRuleTypes {
		report.Unsupported = append(report.Unsupported, "Rule type not supported: "+rt)
	}

	// Always-supported features
	report.Supported = append(report.Supported,
		"Proxy servers import",
		"Proxy groups (select, url-test, fallback, load-balance)",
		"Domain/IP/GeoIP/GeoSite/Process routing rules",
		"DNS split resolution (domestic/remote)",
	)

	return report
}

// splitRule splits a Clash rule string like "DOMAIN,example.com,PROXY" into its parts.
func splitRule(rule string) []string {
	var parts []string
	for _, p := range strings.SplitN(rule, ",", 3) {
		parts = append(parts, strings.TrimSpace(p))
	}
	return parts
}
