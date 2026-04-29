package router

import (
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/ggshr9/shuttle/provider"
)

// Action defines what to do with a connection.
type Action string

const (
	ActionProxy  Action = "proxy"
	ActionDirect Action = "direct"
	ActionReject Action = "reject"
)

// Rule type constants for routing rules.
const (
	RuleTypeDomain       = "domain"
	RuleTypeDomainSuffix = "domain-suffix"
	RuleTypeGeoSite      = "geosite"
	RuleTypeGeoIP        = "geoip"
	RuleTypeIPCIDR       = "ip-cidr"
	RuleTypeProcess      = "process"
	RuleTypeProtocol     = "protocol"
)

// Rule defines a routing rule.
type Rule struct {
	Type        string   // "domain", "domain-suffix", "domain-keyword", "geoip", "geosite", "process", "protocol"
	Values      []string // Match values
	Action      Action
	NetworkType string // optional: "wifi", "cellular", "ethernet" — if set, rule only matches on this network type
}

// Router dispatches connections based on domain, IP, process, and protocol rules.
type Router struct {
	mu           sync.RWMutex
	ruleChain    []compiledRule // ordered rules evaluated before legacy stack
	domainTrie   *DomainTrie
	ipRules      []ipRule
	processMap   map[string]Action
	protocolMap  map[string]Action
	networkRules []networkRule // rules constrained to a specific network type
	geoIP        *GeoIPDB
	geoSite      *GeoSiteDB
	defaultAct   Action
	networkType  string // current network type: "wifi", "cellular", "ethernet", ""
	logger       *slog.Logger

	// decisionHook is invoked (asynchronously) after each Match* decision
	// with the resolved Action and the rule kind that produced it
	// (e.g. "domain", "geoip", "ip-cidr", "process", "protocol", "default").
	// Used by the engine to populate routing-decision metrics.
	decisionHook func(decision, rule string)
}

type ipRule struct {
	cidr   *net.IPNet
	action Action
}

// networkRule is a rule that only applies when the current network type matches.
type networkRule struct {
	ruleType    string // "domain", "ip-cidr", "process", "protocol"
	values      []string
	action      Action
	networkType string // "wifi", "cellular", "ethernet"
}

// RouterConfig configures the router.
type RouterConfig struct {
	RuleChain     []RuleChainEntry // ordered rules evaluated before legacy stack
	Rules         []Rule
	DefaultAction Action
	RuleProviders map[string]*provider.RuleProvider // optional: used by rule chain entries with RuleProvider match
}

// NewRouter creates a new routing engine.
func NewRouter(cfg *RouterConfig, geoIP *GeoIPDB, geoSite *GeoSiteDB, logger *slog.Logger) *Router {
	if cfg.DefaultAction == "" {
		cfg.DefaultAction = ActionProxy
	}
	if logger == nil {
		logger = slog.Default()
	}
	r := &Router{
		domainTrie:  NewDomainTrie(),
		processMap:  make(map[string]Action),
		protocolMap: make(map[string]Action),
		geoIP:       geoIP,
		geoSite:     geoSite,
		defaultAct:  cfg.DefaultAction,
		logger:      logger,
	}

	// Compile the ordered rule chain (evaluated before legacy rules).
	if len(cfg.RuleChain) > 0 {
		compiled, err := CompileRuleChain(cfg.RuleChain, geoIP, geoSite, cfg.RuleProviders)
		if err != nil {
			logger.Error("failed to compile rule chain", "err", err)
		} else {
			r.ruleChain = compiled
		}
	}

	for _, rule := range cfg.Rules {
		r.addRule(rule)
	}

	return r
}

// SetDecisionHook installs a callback that fires after each Match* decision.
// The hook receives the resolved action ("proxy"/"direct"/"reject") and the
// rule kind that produced it ("domain", "geoip", "ip-cidr", "process",
// "protocol", "default"). Pass nil to clear.
//
// The hook is dispatched on a fresh goroutine so it cannot block the match
// path; callers must therefore make their own counters concurrency-safe.
// Order of hook deliveries across concurrent matches is not guaranteed.
func (r *Router) SetDecisionHook(hook func(decision, rule string)) {
	r.mu.Lock()
	r.decisionHook = hook
	r.mu.Unlock()
}

// notifyDecision dispatches a decision to the registered hook (if any) on a
// fresh goroutine. Caller must hold r.mu (read or write) so reading the hook
// pointer is race-free.
func (r *Router) notifyDecision(decision Action, rule string) {
	if r.decisionHook == nil {
		return
	}
	hook := r.decisionHook
	d := string(decision)
	go hook(d, rule)
}

// SetNetworkType updates the current network type used for network-type-aware routing.
// Valid values: "wifi", "cellular", "ethernet", or "" (unset).
func (r *Router) SetNetworkType(nt string) {
	r.mu.Lock()
	r.networkType = strings.ToLower(nt)
	r.mu.Unlock()
}

// NetworkType returns the currently configured network type.
func (r *Router) NetworkType() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.networkType
}

func (r *Router) addRule(rule Rule) {
	// If the rule has a NetworkType constraint, store it separately.
	if rule.NetworkType != "" {
		r.networkRules = append(r.networkRules, networkRule{
			ruleType:    rule.Type,
			values:      rule.Values,
			action:      rule.Action,
			networkType: strings.ToLower(rule.NetworkType),
		})
		return
	}

	switch rule.Type {
	case RuleTypeDomain, RuleTypeDomainSuffix:
		for _, v := range rule.Values {
			r.domainTrie.Insert(v, string(rule.Action))
		}
	case RuleTypeGeoSite:
		if r.geoSite != nil {
			for _, v := range rule.Values {
				domains := r.geoSite.Lookup(v)
				for _, d := range domains {
					r.domainTrie.Insert(d, string(rule.Action))
				}
			}
		}
	case RuleTypeGeoIP:
		// geoip rules are evaluated at lookup time via GeoIPDB
	case RuleTypeIPCIDR:
		for _, v := range rule.Values {
			_, cidr, err := net.ParseCIDR(v)
			if err != nil {
				r.logger.Warn("invalid CIDR rule", "cidr", v, "err", err)
				continue
			}
			r.ipRules = append(r.ipRules, ipRule{cidr: cidr, action: rule.Action})
		}
	case RuleTypeProcess:
		for _, v := range rule.Values {
			r.processMap[strings.ToLower(v)] = rule.Action
		}
	case RuleTypeProtocol:
		for _, v := range rule.Values {
			r.protocolMap[strings.ToLower(v)] = rule.Action
		}
	}
}

// matchNetworkRules checks network-type-constrained rules against the current
// connection parameters. Returns the action and true if a rule matched.
func (r *Router) matchNetworkRules(domain string, ip net.IP, process string, protocol string) (Action, bool) {
	if len(r.networkRules) == 0 || r.networkType == "" {
		return "", false
	}

	for _, nr := range r.networkRules {
		if nr.networkType != r.networkType {
			continue
		}
		switch nr.ruleType {
		case RuleTypeDomain, RuleTypeDomainSuffix:
			if domain != "" {
				for _, v := range nr.values {
					lowerDomain := strings.ToLower(domain)
					lowerVal := strings.ToLower(v)
					if strings.HasPrefix(v, "+.") {
						// Wildcard suffix match
						suffix := strings.ToLower(v[2:])
						if lowerDomain == suffix || strings.HasSuffix(lowerDomain, "."+suffix) {
							return nr.action, true
						}
					} else if lowerDomain == lowerVal {
						return nr.action, true
					}
				}
			}
		case RuleTypeIPCIDR:
			if ip != nil {
				for _, v := range nr.values {
					_, cidr, err := net.ParseCIDR(v)
					if err != nil {
						continue
					}
					if cidr.Contains(ip) {
						return nr.action, true
					}
				}
			}
		case RuleTypeProcess:
			if process != "" {
				for _, v := range nr.values {
					if strings.EqualFold(v, process) {
						return nr.action, true
					}
				}
			}
		case RuleTypeProtocol:
			if protocol != "" {
				for _, v := range nr.values {
					if strings.EqualFold(v, protocol) {
						return nr.action, true
					}
				}
			}
		}
	}
	return "", false
}

// MatchDomain returns the action for a domain name.
func (r *Router) MatchDomain(domain string) Action {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if action, found := r.domainTrie.Lookup(domain); found {
		act := Action(action)
		r.notifyDecision(act, "domain")
		return act
	}
	r.notifyDecision(r.defaultAct, "default")
	return r.defaultAct
}

// MatchIP returns the action for an IP address.
func (r *Router) MatchIP(ip net.IP) Action {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check explicit CIDR rules first
	for _, rule := range r.ipRules {
		if rule.cidr.Contains(ip) {
			r.notifyDecision(rule.action, "ip-cidr")
			return rule.action
		}
	}

	// Check GeoIP
	if r.geoIP != nil {
		country := r.geoIP.LookupCountry(ip)
		if country == "CN" {
			r.notifyDecision(ActionDirect, "geoip")
			return ActionDirect
		}
	}

	r.notifyDecision(r.defaultAct, "default")
	return r.defaultAct
}

// MatchProcess returns the action for a process name.
func (r *Router) MatchProcess(name string) Action {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if action, ok := r.processMap[strings.ToLower(name)]; ok {
		r.notifyDecision(action, "process")
		return action
	}
	r.notifyDecision(r.defaultAct, "default")
	return r.defaultAct
}

// MatchProtocol returns the action for a protocol (e.g., "bittorrent").
func (r *Router) MatchProtocol(proto string) Action {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if action, ok := r.protocolMap[strings.ToLower(proto)]; ok {
		r.notifyDecision(action, "protocol")
		return action
	}
	r.notifyDecision(r.defaultAct, "default")
	return r.defaultAct
}

// DryRunResult describes the routing decision for a given input.
type DryRunResult struct {
	Domain    string `json:"domain"`
	Action    string `json:"action"`               // "proxy", "direct", "reject"
	MatchedBy string `json:"matched_by"`            // "domain_rule", "geosite", "default"
	Rule      string `json:"rule,omitempty"`         // the matched rule pattern
}

// DryRun tests routing for a domain without actually proxying or doing DNS resolution.
// It checks domain rules only and falls back to the default action.
func (r *Router) DryRun(domain string) DryRunResult {
	result := DryRunResult{
		Domain: domain,
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if domain != "" {
		if action, found := r.domainTrie.Lookup(domain); found {
			result.Action = action
			result.MatchedBy = "domain_rule"
			result.Rule = domain
			return result
		}
	}

	result.Action = string(r.defaultAct)
	result.MatchedBy = "default"
	return result
}

// GeoSiteDB returns the GeoSite database used by this router.
func (r *Router) GeoSiteDB() *GeoSiteDB {
	return r.geoSite
}

// Match performs full routing decision for a connection.
// port is the destination port (0 if unknown); srcIP is the client source IP (nil if unknown).
func (r *Router) Match(domain string, ip net.IP, process string, protocol string, port uint16, srcIP net.IP) Action {
	r.mu.RLock()

	// Phase 1: Ordered rule chain (evaluated first, highest priority).
	if len(r.ruleChain) > 0 {
		ctx := &MatchContext{
			Domain:      domain,
			IP:          ip,
			Port:        port,
			SrcIP:       srcIP,
			Process:     process,
			Protocol:    protocol,
			NetworkType: r.networkType,
		}
		for i := range r.ruleChain {
			if r.ruleChain[i].Match(ctx) {
				action := r.ruleChain[i].action
				r.mu.RUnlock()
				return action
			}
		}
	}

	// Phase 2: Network-type-constrained rules.
	if action, ok := r.matchNetworkRules(domain, ip, process, protocol); ok {
		r.mu.RUnlock()
		return action
	}
	r.mu.RUnlock()

	// Phase 3: Legacy category-based matching (protocol > process > domain > IP > default).
	if protocol != "" {
		if action := r.MatchProtocol(protocol); action != r.defaultAct {
			return action
		}
	}
	if process != "" {
		if action := r.MatchProcess(process); action != r.defaultAct {
			return action
		}
	}
	if domain != "" {
		if action := r.MatchDomain(domain); action != r.defaultAct {
			return action
		}
	}
	if ip != nil {
		return r.MatchIP(ip)
	}
	return r.defaultAct
}
