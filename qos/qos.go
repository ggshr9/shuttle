// Package qos implements Quality of Service traffic classification and DSCP marking.
package qos

import (
	"strings"
	"sync"

	"github.com/shuttleX/shuttle/config"
)

// Priority levels for traffic classification.
type Priority int

const (
	PriorityCritical Priority = 0 // Interactive/low-latency (SSH, gaming, VoIP)
	PriorityHigh     Priority = 1 // Real-time streaming (video calls, media)
	PriorityNormal   Priority = 2 // Default web traffic
	PriorityBulk     Priority = 3 // Background downloads, updates
	PriorityLow      Priority = 4 // Bulk transfers, torrents
)

// DSCP (Differentiated Services Code Point) values for IP header marking.
// These are shifted left by 2 bits when placed in the TOS byte.
const (
	DSCPExpedited   = 46 // EF - Expedited Forwarding (critical)
	DSCPAF41        = 34 // AF41 - Assured Forwarding class 4 (high)
	DSCPAF21        = 18 // AF21 - Assured Forwarding class 2 (normal)
	DSCPAF11        = 10 // AF11 - Assured Forwarding class 1 (bulk)
	DSCPBestEffort  = 0  // BE - Best Effort (low/default)
)

// PriorityToDSCP maps priority levels to DSCP values.
var PriorityToDSCP = map[Priority]uint8{
	PriorityCritical: DSCPExpedited,
	PriorityHigh:     DSCPAF41,
	PriorityNormal:   DSCPAF21,
	PriorityBulk:     DSCPAF11,
	PriorityLow:      DSCPBestEffort,
}

// DSCPToTOS converts a DSCP value to the TOS byte value.
func DSCPToTOS(dscp uint8) uint8 {
	return dscp << 2
}

// Classifier classifies traffic into priority levels based on rules.
type Classifier struct {
	mu       sync.RWMutex
	enabled  bool
	rules    []rule
	portMap  map[uint16]Priority // fast lookup for port-based rules
	defaults map[uint16]Priority // well-known port defaults
}

type rule struct {
	ports    map[uint16]bool
	protocol string
	domains  []string
	process  map[string]bool
	priority Priority
}

// NewClassifier creates a new QoS classifier from config.
func NewClassifier(cfg *config.QoSConfig) *Classifier {
	c := &Classifier{
		enabled:  cfg != nil && cfg.Enabled,
		portMap:  make(map[uint16]Priority),
		defaults: defaultPortPriorities(),
	}

	if cfg == nil || !cfg.Enabled {
		return c
	}

	for _, r := range cfg.Rules {
		c.addRule(r)
	}

	return c
}

// defaultPortPriorities returns well-known port to priority mappings.
func defaultPortPriorities() map[uint16]Priority {
	return map[uint16]Priority{
		// Critical: Interactive/low-latency
		22:   PriorityCritical, // SSH
		23:   PriorityCritical, // Telnet
		3389: PriorityCritical, // RDP

		// High: Real-time
		5060: PriorityHigh, // SIP
		5061: PriorityHigh, // SIP TLS

		// Bulk: Background
		123:  PriorityBulk, // NTP

		// Low: Bulk transfers
		6881: PriorityLow, // BitTorrent
		6882: PriorityLow,
		6883: PriorityLow,
		6884: PriorityLow,
		6885: PriorityLow,
		6886: PriorityLow,
		6887: PriorityLow,
		6888: PriorityLow,
		6889: PriorityLow,
	}
}

func parsePriority(s string) Priority {
	switch strings.ToLower(s) {
	case "critical":
		return PriorityCritical
	case "high":
		return PriorityHigh
	case "normal":
		return PriorityNormal
	case "bulk":
		return PriorityBulk
	case "low":
		return PriorityLow
	default:
		return PriorityNormal
	}
}

func (c *Classifier) addRule(cfg config.QoSRule) {
	r := rule{
		protocol: strings.ToLower(cfg.Protocol),
		domains:  cfg.Domains,
		priority: parsePriority(cfg.Priority),
	}

	// Build port set
	if len(cfg.Ports) > 0 {
		r.ports = make(map[uint16]bool)
		for _, p := range cfg.Ports {
			r.ports[p] = true
			// Also add to fast port lookup
			c.portMap[p] = r.priority
		}
	}

	// Build process set
	if len(cfg.Process) > 0 {
		r.process = make(map[string]bool)
		for _, p := range cfg.Process {
			r.process[strings.ToLower(p)] = true
		}
	}

	c.rules = append(c.rules, r)
}

// Enabled returns whether QoS is enabled.
func (c *Classifier) Enabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.enabled
}

// ClassifyPort returns the priority for a destination port using fast lookup.
func (c *Classifier) ClassifyPort(port uint16) Priority {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.enabled {
		return PriorityNormal
	}

	// Check explicit rules first
	if p, ok := c.portMap[port]; ok {
		return p
	}

	// Check defaults
	if p, ok := c.defaults[port]; ok {
		return p
	}

	return PriorityNormal
}

// Classify performs full classification based on all available info.
func (c *Classifier) Classify(port uint16, protocol, domain, process string) Priority {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.enabled {
		return PriorityNormal
	}

	protocol = strings.ToLower(protocol)
	process = strings.ToLower(process)
	domain = strings.ToLower(domain)

	// Check rules in order (first match wins)
	for _, r := range c.rules {
		if c.ruleMatches(r, port, protocol, domain, process) {
			return r.priority
		}
	}

	// Fall back to port-based defaults
	if p, ok := c.portMap[port]; ok {
		return p
	}
	if p, ok := c.defaults[port]; ok {
		return p
	}

	return PriorityNormal
}

func (c *Classifier) ruleMatches(r rule, port uint16, protocol, domain, process string) bool {
	// Protocol must match if specified
	if r.protocol != "" && r.protocol != protocol {
		return false
	}

	// At least one condition must match
	matched := false

	// Port match
	if r.ports != nil && r.ports[port] {
		matched = true
	}

	// Process match
	if r.process != nil && r.process[process] {
		matched = true
	}

	// Domain match (suffix matching)
	if len(r.domains) > 0 && domain != "" {
		for _, d := range r.domains {
			if strings.HasSuffix(domain, d) || domain == d {
				matched = true
				break
			}
		}
	}

	return matched
}

// GetDSCP returns the DSCP value for the given priority.
func (c *Classifier) GetDSCP(priority Priority) uint8 {
	if dscp, ok := PriorityToDSCP[priority]; ok {
		return dscp
	}
	return DSCPBestEffort
}

// GetTOS returns the TOS byte value for the given priority.
func (c *Classifier) GetTOS(priority Priority) uint8 {
	return DSCPToTOS(c.GetDSCP(priority))
}

// Update replaces the classifier configuration.
func (c *Classifier) Update(cfg *config.QoSConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.enabled = cfg != nil && cfg.Enabled
	c.rules = nil
	c.portMap = make(map[uint16]Priority)
	c.defaults = defaultPortPriorities()

	if cfg == nil || !cfg.Enabled {
		return
	}

	for _, r := range cfg.Rules {
		c.addRule(r)
	}
}
