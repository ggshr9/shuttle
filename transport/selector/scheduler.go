package selector

import (
	"math/rand"
	"sync/atomic"
)

// StreamScheduler selects the best path for the next stream.
type StreamScheduler interface {
	Pick(paths []*PathMetrics) *PathMetrics
}

// NewWeightedLatencyScheduler returns a scheduler that distributes streams
// proportional to inverse latency. Lower latency paths receive more streams.
func NewWeightedLatencyScheduler() StreamScheduler {
	return &weightedLatencyScheduler{}
}

// NewMinLatencyScheduler returns a scheduler that always picks the path
// with the lowest latency.
func NewMinLatencyScheduler() StreamScheduler {
	return &minLatencyScheduler{}
}

// NewLoadBalanceScheduler returns a scheduler that picks the path with the
// fewest active streams.
func NewLoadBalanceScheduler() StreamScheduler {
	return &loadBalanceScheduler{}
}

// weightedLatencyScheduler distributes streams proportional to inverse latency.
// Lower latency paths receive more streams.
type weightedLatencyScheduler struct{}

func (s *weightedLatencyScheduler) Pick(paths []*PathMetrics) *PathMetrics {
	eligible := filterEligible(paths)
	if len(eligible) == 0 {
		return nil
	}
	if len(eligible) == 1 {
		return eligible[0]
	}

	// Weight = 1 / latency (in microseconds). Minimum latency 1µs to avoid div-by-zero.
	weights := make([]float64, len(eligible))
	var total float64
	for i, p := range eligible {
		lat := float64(p.Latency.Microseconds())
		if lat < 1 {
			lat = 1
		}
		w := 1.0 / lat
		weights[i] = w
		total += w
	}

	r := rand.Float64() * total //nolint:gosec // G404: used for load balancing, not security
	var cumulative float64
	for i, w := range weights {
		cumulative += w
		if r <= cumulative {
			return eligible[i]
		}
	}
	return eligible[len(eligible)-1]
}

// minLatencyScheduler always picks the path with the lowest latency.
type minLatencyScheduler struct{}

func (s *minLatencyScheduler) Pick(paths []*PathMetrics) *PathMetrics {
	eligible := filterEligible(paths)
	if len(eligible) == 0 {
		return nil
	}
	best := eligible[0]
	for _, p := range eligible[1:] {
		if p.Latency < best.Latency {
			best = p
		}
	}
	return best
}

// loadBalanceScheduler picks the path with the fewest active streams.
type loadBalanceScheduler struct{}

func (s *loadBalanceScheduler) Pick(paths []*PathMetrics) *PathMetrics {
	eligible := filterEligible(paths)
	if len(eligible) == 0 {
		return nil
	}
	best := eligible[0]
	bestActive := atomic.LoadInt64(&best.ActiveStreams)
	for _, p := range eligible[1:] {
		active := atomic.LoadInt64(&p.ActiveStreams)
		if active < bestActive {
			best = p
			bestActive = active
		}
	}
	return best
}

// filterEligible returns paths that are available, have a connection, and
// have fewer than 3 consecutive failures.
func filterEligible(paths []*PathMetrics) []*PathMetrics {
	var out []*PathMetrics
	for _, p := range paths {
		if p.IsAvailable() && p.GetConn() != nil && atomic.LoadInt64(&p.Failures) < 3 {
			out = append(out, p)
		}
	}
	return out
}
