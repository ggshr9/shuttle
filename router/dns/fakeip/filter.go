package fakeip

import (
	"path/filepath"
	"strings"
)

// Filter determines which domains should bypass fake-ip and get real DNS resolution.
type Filter struct {
	exact    map[string]bool // exact domain match (lowercased)
	suffixes []string        // "+.lan" → ".lan" (lowercased)
	patterns []string        // glob patterns like "stun.*", "time.*.com" (lowercased)
}

// NewFilter creates a Filter from the given pattern list.
//
// Pattern formats:
//   - "example.com"   → exact match (case-insensitive)
//   - "+.lan"         → suffix match: anything ending in ".lan"
//   - "stun.*"        → glob pattern using filepath.Match
func NewFilter(patterns []string) *Filter {
	f := &Filter{
		exact: make(map[string]bool),
	}

	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}

		if strings.HasPrefix(p, "+.") {
			// Suffix pattern: "+.lan" → skip any domain ending in ".lan"
			suffix := p[1:] // keep the leading dot, e.g. ".lan"
			f.suffixes = append(f.suffixes, suffix)
		} else if strings.ContainsAny(p, "*?[") {
			// Glob pattern
			f.patterns = append(f.patterns, p)
		} else {
			// Exact match
			f.exact[p] = true
		}
	}

	return f
}

// ShouldSkip returns true if the domain matches any pattern in the filter,
// meaning it should bypass fake-ip and use real DNS resolution.
// All matching is case-insensitive.
func (f *Filter) ShouldSkip(domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return false
	}

	// Exact match
	if f.exact[domain] {
		return true
	}

	// Suffix match
	for _, suffix := range f.suffixes {
		if strings.HasSuffix(domain, suffix) {
			return true
		}
	}

	// Glob pattern match
	for _, pattern := range f.patterns {
		matched, err := filepath.Match(pattern, domain)
		if err == nil && matched {
			return true
		}
	}

	return false
}
