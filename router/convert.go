package router

import "github.com/shuttleX/shuttle/config"

// ConfigRuleToRouterRule converts a config.RouteRule to a router.Rule.
// This is the single authoritative conversion used by engine setup and GUI API handlers.
func ConfigRuleToRouterRule(rule config.RouteRule) Rule { //nolint:gocritic // hugeParam: exported API, callers pass by value
	r := Rule{
		Action:      Action(rule.Action),
		NetworkType: rule.NetworkType,
	}
	switch {
	case rule.Domains != "":
		r.Type = RuleTypeDomain
		r.Values = []string{rule.Domains}
	case rule.GeoSite != "":
		r.Type = RuleTypeGeoSite
		r.Values = []string{rule.GeoSite}
	case rule.GeoIP != "":
		r.Type = RuleTypeGeoIP
		r.Values = []string{rule.GeoIP}
	case len(rule.Process) > 0:
		r.Type = RuleTypeProcess
		r.Values = rule.Process
	case rule.Protocol != "":
		r.Type = RuleTypeProtocol
		r.Values = []string{rule.Protocol}
	case len(rule.IPCIDR) > 0:
		r.Type = RuleTypeIPCIDR
		r.Values = rule.IPCIDR
	}
	return r
}
