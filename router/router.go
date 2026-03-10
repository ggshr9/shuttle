package router

import (
	"log/slog"
	"net"
	"strings"
	"sync"
)

// Action defines what to do with a connection.
type Action string

const (
	ActionProxy  Action = "proxy"
	ActionDirect Action = "direct"
	ActionReject Action = "reject"
)

// Rule defines a routing rule.
type Rule struct {
	Type   string   // "domain", "domain-suffix", "domain-keyword", "geoip", "geosite", "process", "protocol"
	Values []string // Match values
	Action Action
}

// Router dispatches connections based on domain, IP, process, and protocol rules.
type Router struct {
	mu          sync.RWMutex
	domainTrie  *DomainTrie
	ipRules     []ipRule
	processMap  map[string]Action
	protocolMap map[string]Action
	geoIP       *GeoIPDB
	geoSite     *GeoSiteDB
	defaultAct  Action
	logger      *slog.Logger
}

type ipRule struct {
	cidr   *net.IPNet
	action Action
}

// RouterConfig configures the router.
type RouterConfig struct {
	Rules         []Rule
	DefaultAction Action
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

	for _, rule := range cfg.Rules {
		r.addRule(rule)
	}

	return r
}

func (r *Router) addRule(rule Rule) {
	switch rule.Type {
	case "domain", "domain-suffix":
		for _, v := range rule.Values {
			r.domainTrie.Insert(v, string(rule.Action))
		}
	case "geosite":
		if r.geoSite != nil {
			for _, v := range rule.Values {
				domains := r.geoSite.Lookup(v)
				for _, d := range domains {
					r.domainTrie.Insert(d, string(rule.Action))
				}
			}
		}
	case "geoip":
		// geoip rules are evaluated at lookup time via GeoIPDB
	case "ip-cidr":
		for _, v := range rule.Values {
			_, cidr, err := net.ParseCIDR(v)
			if err != nil {
				r.logger.Warn("invalid CIDR rule", "cidr", v, "err", err)
				continue
			}
			r.ipRules = append(r.ipRules, ipRule{cidr: cidr, action: rule.Action})
		}
	case "process":
		for _, v := range rule.Values {
			r.processMap[strings.ToLower(v)] = rule.Action
		}
	case "protocol":
		for _, v := range rule.Values {
			r.protocolMap[strings.ToLower(v)] = rule.Action
		}
	}
}

// MatchDomain returns the action for a domain name.
func (r *Router) MatchDomain(domain string) Action {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if action, found := r.domainTrie.Lookup(domain); found {
		return Action(action)
	}
	return r.defaultAct
}

// MatchIP returns the action for an IP address.
func (r *Router) MatchIP(ip net.IP) Action {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check explicit CIDR rules first
	for _, rule := range r.ipRules {
		if rule.cidr.Contains(ip) {
			return rule.action
		}
	}

	// Check GeoIP
	if r.geoIP != nil {
		country := r.geoIP.LookupCountry(ip)
		if country == "CN" {
			return ActionDirect
		}
	}

	return r.defaultAct
}

// MatchProcess returns the action for a process name.
func (r *Router) MatchProcess(name string) Action {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if action, ok := r.processMap[strings.ToLower(name)]; ok {
		return action
	}
	return r.defaultAct
}

// MatchProtocol returns the action for a protocol (e.g., "bittorrent").
func (r *Router) MatchProtocol(proto string) Action {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if action, ok := r.protocolMap[strings.ToLower(proto)]; ok {
		return action
	}
	return r.defaultAct
}

// GeoSiteDB returns the GeoSite database used by this router.
func (r *Router) GeoSiteDB() *GeoSiteDB {
	return r.geoSite
}

// Match performs full routing decision for a connection.
func (r *Router) Match(domain string, ip net.IP, process string, protocol string) Action {
	// Priority: protocol > process > domain > IP > default
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
