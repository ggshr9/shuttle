package server

import (
	"sync"
	"time"
)

// ReputationConfig configures the IP reputation system.
type ReputationConfig struct {
	Enabled      bool
	MaxFailures  int             // failures before ban (default 5)
	WindowSize   time.Duration   // time window for failures (default 5 min)
	BanDurations []time.Duration // exponential ban: [1m, 5m, 30m, 24h]
}

// DefaultReputationConfig returns sensible defaults.
func DefaultReputationConfig() ReputationConfig {
	return ReputationConfig{
		Enabled:     true,
		MaxFailures: 5,
		WindowSize:  5 * time.Minute,
		BanDurations: []time.Duration{
			1 * time.Minute,
			5 * time.Minute,
			30 * time.Minute,
			24 * time.Hour,
		},
	}
}

type ipRecord struct {
	failures []time.Time // timestamps of recent failures
	banUntil time.Time   // when the current ban expires
	banCount int         // number of times banned (for escalation)
}

// Reputation tracks IP-level reputation for rate limiting auth failures.
type Reputation struct {
	mu     sync.Mutex
	ips    map[string]*ipRecord
	config ReputationConfig
}

// NewReputation creates a new reputation tracker.
func NewReputation(cfg ReputationConfig) *Reputation {
	if cfg.MaxFailures <= 0 {
		cfg.MaxFailures = 5
	}
	if cfg.WindowSize == 0 {
		cfg.WindowSize = 5 * time.Minute
	}
	if len(cfg.BanDurations) == 0 {
		cfg.BanDurations = DefaultReputationConfig().BanDurations
	}
	return &Reputation{
		ips:    make(map[string]*ipRecord),
		config: cfg,
	}
}

// IsBanned returns true if the IP is currently banned.
func (r *Reputation) IsBanned(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.ips[ip]
	if !ok {
		return false
	}
	return time.Now().Before(rec.banUntil)
}

// RecordFailure records an auth failure for an IP.
// Returns true if the IP is now banned.
func (r *Reputation) RecordFailure(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	rec, ok := r.ips[ip]
	if !ok {
		rec = &ipRecord{}
		r.ips[ip] = rec
	}

	// If currently banned, don't add more failures
	if now.Before(rec.banUntil) {
		return true
	}

	// Prune old failures outside the window
	cutoff := now.Add(-r.config.WindowSize)
	fresh := rec.failures[:0]
	for _, t := range rec.failures {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}
	rec.failures = append(fresh, now)

	// Check if threshold exceeded
	if len(rec.failures) >= r.config.MaxFailures {
		// Ban with escalating duration
		idx := rec.banCount
		if idx >= len(r.config.BanDurations) {
			idx = len(r.config.BanDurations) - 1
		}
		rec.banUntil = now.Add(r.config.BanDurations[idx])
		rec.banCount++
		rec.failures = nil // Reset failures
		return true
	}
	return false
}

// RecordSuccess records a successful auth, resetting failure count.
func (r *Reputation) RecordSuccess(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.ips, ip)
}

// BannedIPs returns a list of currently banned IPs with their ban expiry.
func (r *Reputation) BannedIPs() map[string]time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	result := make(map[string]time.Time)
	for ip, rec := range r.ips {
		if now.Before(rec.banUntil) {
			result[ip] = rec.banUntil
		}
	}
	return result
}

// Cleanup removes expired records. Call periodically.
func (r *Reputation) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for ip, rec := range r.ips {
		if now.After(rec.banUntil) && len(rec.failures) == 0 {
			delete(r.ips, ip)
		}
	}
}
