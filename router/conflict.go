package router

import (
	"fmt"
	"strings"
)

// Conflict describes a routing rule conflict where the same domain appears
// in multiple rules with different actions.
type Conflict struct {
	Domain  string `json:"domain"`
	Action1 string `json:"action1"`
	Action2 string `json:"action2"`
	Rule1   string `json:"rule1"` // human-readable rule description
	Rule2   string `json:"rule2"`
}

// DetectConflicts scans routing rules for domains that appear in multiple
// rules with different actions. Returns a list of conflicts found.
func DetectConflicts(rules []Rule, geoSite *GeoSiteDB) []Conflict {
	// Map domain → (action, rule description)
	type entry struct {
		action string
		rule   string
	}
	seen := make(map[string]entry)
	var conflicts []Conflict

	for _, rule := range rules {
		var domains []string
		var ruleDesc string

		switch rule.Type {
		case "domain", "domain-suffix":
			domains = rule.Values
			ruleDesc = fmt.Sprintf("%s: %s", rule.Type, strings.Join(rule.Values, ", "))
		case "geosite":
			ruleDesc = "geosite: " + strings.Join(rule.Values, ", ")
			if geoSite != nil {
				for _, cat := range rule.Values {
					domains = append(domains, geoSite.Lookup(cat)...)
				}
			}
		default:
			continue // process, protocol, ip-cidr don't conflict with domain rules
		}

		action := string(rule.Action)
		for _, d := range domains {
			d = normalizeDomain(d)
			if prev, ok := seen[d]; ok {
				if prev.action != action {
					conflicts = append(conflicts, Conflict{
						Domain:  d,
						Action1: prev.action,
						Action2: action,
						Rule1:   prev.rule,
						Rule2:   ruleDesc,
					})
				}
			} else {
				seen[d] = entry{action: action, rule: ruleDesc}
			}
		}
	}

	return conflicts
}

func normalizeDomain(d string) string {
	d = strings.TrimPrefix(d, "+.")
	d = strings.TrimSuffix(d, ".")
	return strings.ToLower(d)
}
