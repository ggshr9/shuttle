package engine

import (
	"math"
	"math/rand"
	"sort"
	"time"
)

// QualityConfig configures quality-based outbound selection.
type QualityConfig struct {
	MaxLatency  time.Duration `json:"max_latency"`   // 0 = no limit
	MaxLossRate float64       `json:"max_loss_rate"` // 0 = no limit
	// Tolerance defines a "good enough" band around the best score:
	// entries within Tolerance of the top score are treated as
	// equivalent and the bucket is shuffled per-dial so concurrent
	// callers don't all stampede the single nominal #1 (thundering
	// herd). When 0, falls back to defaultQualityToleranceMS.
	Tolerance time.Duration `json:"tolerance"`
}

// defaultQualityToleranceMS is the score-distance (in ms-equivalent
// units of qualityScore) within which entries are treated as
// effectively tied. Small enough to still pick the genuinely best
// node, large enough to absorb probe-to-probe noise.
const defaultQualityToleranceMS = 30

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
//
// `cfg.Tolerance` (or defaultQualityToleranceMS when zero) defines a
// score band around the top: entries inside the band are shuffled
// each call so concurrent dialers don't all hit the same nominal #1.
// Entries outside the band keep their score-sorted order.
func rankByQuality(entries []qualityEntry, cfg QualityConfig) []qualityEntry {
	ranked := make([]qualityEntry, len(entries))
	copy(ranked, entries)
	sort.SliceStable(ranked, func(i, j int) bool {
		return qualityScore(ranked[i].latency, ranked[i].loss) <
			qualityScore(ranked[j].latency, ranked[j].loss)
	})
	if len(ranked) <= 1 {
		return ranked
	}

	tolMS := float64(cfg.Tolerance.Milliseconds())
	if tolMS <= 0 {
		tolMS = defaultQualityToleranceMS
	}
	bestScore := qualityScore(ranked[0].latency, ranked[0].loss)
	// Find the index where the score first exceeds best+tolMS.
	bucketEnd := 1
	for bucketEnd < len(ranked) {
		s := qualityScore(ranked[bucketEnd].latency, ranked[bucketEnd].loss)
		if s-bestScore > tolMS {
			break
		}
		bucketEnd++
	}
	if bucketEnd > 1 {
		bucket := ranked[:bucketEnd]
		rand.Shuffle(len(bucket), func(i, j int) {
			bucket[i], bucket[j] = bucket[j], bucket[i]
		})
	}
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
