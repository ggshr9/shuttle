package engine

import (
	"math"
	"sort"
	"time"
)

// QualityConfig configures quality-based outbound selection.
type QualityConfig struct {
	MaxLatency  time.Duration `json:"max_latency"`   // 0 = no limit
	MaxLossRate float64       `json:"max_loss_rate"` // 0 = no limit
}

type qualityEntry struct {
	tag     string
	latency time.Duration
	loss    float64
	index   int // original index in outbound slice
}

// qualityScore computes a combined score (lower is better).
// Formula: latency_ms + 5000*sqrt(loss)  (Mathis-inspired loss penalty)
// Examples: 1% loss ≈ +500ms, 5% ≈ +1118ms, 10% ≈ +1581ms
func qualityScore(latency time.Duration, loss float64) float64 {
	ms := float64(latency.Milliseconds())
	if loss <= 0 {
		return ms
	}
	// Mathis-inspired: 5000 * sqrt(loss)
	lossPenalty := 5000 * math.Sqrt(loss)
	return ms + lossPenalty
}

// rankByQuality sorts entries by quality score (best first).
// The input slice is copied so the original is not modified.
func rankByQuality(entries []qualityEntry) []qualityEntry {
	ranked := make([]qualityEntry, len(entries))
	copy(ranked, entries)
	sort.SliceStable(ranked, func(i, j int) bool {
		return qualityScore(ranked[i].latency, ranked[i].loss) <
			qualityScore(ranked[j].latency, ranked[j].loss)
	})
	return ranked
}

// filterByQuality removes entries exceeding thresholds.
// If all entries are filtered out, returns the original list (failover).
func filterByQuality(entries []qualityEntry, cfg QualityConfig) []qualityEntry {
	if len(entries) == 0 {
		return entries
	}

	filtered := entries[:0:0] // nil-length slice with same backing type
	for _, e := range entries {
		if cfg.MaxLatency > 0 && e.latency > cfg.MaxLatency {
			continue
		}
		if cfg.MaxLossRate > 0 && e.loss > cfg.MaxLossRate {
			continue
		}
		filtered = append(filtered, e)
	}

	if len(filtered) == 0 {
		// Graceful fallback: return original list unchanged.
		return entries
	}
	return filtered
}
