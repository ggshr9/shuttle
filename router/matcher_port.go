package router

import (
	"fmt"
	"strconv"
	"strings"
)

// portRange represents a single port or a contiguous port range.
type portRange struct {
	min uint16
	max uint16
}

// portMatcher matches the destination port against a set of port ranges.
type portMatcher struct {
	ranges []portRange
}

// newPortMatcher parses port specs (e.g. "80", "8080-8090") and returns a Matcher.
func newPortMatcher(specs []string) (*portMatcher, error) {
	m := &portMatcher{}
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		if idx := strings.IndexByte(spec, '-'); idx >= 0 {
			lo, err := parsePort(spec[:idx])
			if err != nil {
				return nil, fmt.Errorf("invalid port range %q: %w", spec, err)
			}
			hi, err := parsePort(spec[idx+1:])
			if err != nil {
				return nil, fmt.Errorf("invalid port range %q: %w", spec, err)
			}
			if lo > hi {
				return nil, fmt.Errorf("invalid port range %q: min > max", spec)
			}
			m.ranges = append(m.ranges, portRange{min: lo, max: hi})
		} else {
			p, err := parsePort(spec)
			if err != nil {
				return nil, fmt.Errorf("invalid port %q: %w", spec, err)
			}
			m.ranges = append(m.ranges, portRange{min: p, max: p})
		}
	}
	if len(m.ranges) == 0 {
		return nil, fmt.Errorf("no valid port specs provided")
	}
	return m, nil
}

func parsePort(s string) (uint16, error) {
	s = strings.TrimSpace(s)
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, fmt.Errorf("port must be 1-65535")
	}
	return uint16(n), nil
}

func (m *portMatcher) Match(ctx *MatchContext) bool {
	if ctx.Port == 0 {
		return false
	}
	for _, r := range m.ranges {
		if ctx.Port >= r.min && ctx.Port <= r.max {
			return true
		}
	}
	return false
}
