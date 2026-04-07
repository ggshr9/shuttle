package engine

import "time"

// parseDurationOr parses s as a time.Duration. If s is empty or cannot be
// parsed, def is returned instead.
func parseDurationOr(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return def
}
