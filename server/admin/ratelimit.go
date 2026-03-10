package admin

import (
	"net"
	"sync"
	"time"
)

// bucket tracks token state for a single IP.
type bucket struct {
	tokens float64
	last   time.Time
}

// RateLimiter implements an IP-based token bucket rate limiter.
type RateLimiter struct {
	mu    sync.Mutex
	ips   map[string]*bucket
	rate  float64 // tokens per second
	burst int     // max tokens
}

// NewRateLimiter creates a rate limiter with the given refill rate (tokens/sec) and burst capacity.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{
		ips:   make(map[string]*bucket),
		rate:  rate,
		burst: burst,
	}
}

// Allow checks whether a request from the given IP is allowed, consuming one token if so.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.ips[ip]
	if !ok {
		b = &bucket{
			tokens: float64(rl.burst),
			last:   now,
		}
		rl.ips[ip] = b
	}

	// Refill tokens based on elapsed time.
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}
	b.last = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// cleanup removes entries that have been idle for more than 10 minutes.
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute)
	for ip, b := range rl.ips {
		if b.last.Before(cutoff) {
			delete(rl.ips, ip)
		}
	}
}

// extractIP returns just the IP portion of a RemoteAddr (strips port).
func extractIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// Try parsing as bare IP (no port)
		if ip := net.ParseIP(remoteAddr); ip != nil {
			return ip.String()
		}
		return remoteAddr
	}
	return host
}
