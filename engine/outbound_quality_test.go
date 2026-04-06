package engine

import (
	"math"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"

)

func TestQualityScore(t *testing.T) {
	// Lower latency should produce a lower score.
	lowLatency := qualityScore(10*time.Millisecond, 0)
	highLatency := qualityScore(100*time.Millisecond, 0)
	if lowLatency >= highLatency {
		t.Errorf("expected lowLatency score (%v) < highLatency score (%v)", lowLatency, highLatency)
	}

	// Lower loss should produce a lower score.
	lowLoss := qualityScore(10*time.Millisecond, 0.01)
	highLoss := qualityScore(10*time.Millisecond, 0.5)
	if lowLoss >= highLoss {
		t.Errorf("expected lowLoss score (%v) < highLoss score (%v)", lowLoss, highLoss)
	}

	// Verify formula: latency_ms + 5000*sqrt(loss)
	score := qualityScore(50*time.Millisecond, 0.1)
	expected := 50.0 + 5000*math.Sqrt(0.1) // ≈ 1630.28
	if score != expected {
		t.Errorf("qualityScore(50ms, 0.1) = %v, want %v", score, expected)
	}

	// Zero loss returns latency only (no NaN/negative from sqrt).
	zeroLoss := qualityScore(100*time.Millisecond, 0)
	if zeroLoss != 100.0 {
		t.Errorf("qualityScore(100ms, 0) = %v, want 100.0", zeroLoss)
	}
}

func TestQualityRank(t *testing.T) {
	entries := []qualityEntry{
		{tag: "slow", latency: 200 * time.Millisecond, loss: 0.0, index: 0},
		{tag: "fast", latency: 10 * time.Millisecond, loss: 0.0, index: 1},
		{tag: "medium", latency: 80 * time.Millisecond, loss: 0.0, index: 2},
	}

	ranked := rankByQuality(entries)

	if len(ranked) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(ranked))
	}
	if ranked[0].tag != "fast" {
		t.Errorf("ranked[0].tag = %q, want %q", ranked[0].tag, "fast")
	}
	if ranked[1].tag != "medium" {
		t.Errorf("ranked[1].tag = %q, want %q", ranked[1].tag, "medium")
	}
	if ranked[2].tag != "slow" {
		t.Errorf("ranked[2].tag = %q, want %q", ranked[2].tag, "slow")
	}

	// Verify the original slice is not mutated.
	if entries[0].tag != "slow" {
		t.Errorf("original slice mutated: entries[0].tag = %q, want %q", entries[0].tag, "slow")
	}
}

func TestQualityRank_LossBreaksTie(t *testing.T) {
	// Two entries with same latency; lower loss should rank first.
	entries := []qualityEntry{
		{tag: "high-loss", latency: 50 * time.Millisecond, loss: 0.3, index: 0},
		{tag: "low-loss", latency: 50 * time.Millisecond, loss: 0.05, index: 1},
	}

	ranked := rankByQuality(entries)

	if ranked[0].tag != "low-loss" {
		t.Errorf("ranked[0].tag = %q, want %q", ranked[0].tag, "low-loss")
	}
}

func TestQualityFilter(t *testing.T) {
	entries := []qualityEntry{
		{tag: "ok", latency: 50 * time.Millisecond, loss: 0.01, index: 0},
		{tag: "slow", latency: 500 * time.Millisecond, loss: 0.01, index: 1},
		{tag: "lossy", latency: 50 * time.Millisecond, loss: 0.5, index: 2},
	}

	cfg := QualityConfig{
		MaxLatency:  200 * time.Millisecond,
		MaxLossRate: 0.1,
	}

	filtered := filterByQuality(entries, cfg)

	if len(filtered) != 1 {
		t.Fatalf("expected 1 entry after filtering, got %d", len(filtered))
	}
	if filtered[0].tag != "ok" {
		t.Errorf("filtered[0].tag = %q, want %q", filtered[0].tag, "ok")
	}
}

func TestQualityFilter_AllFiltered(t *testing.T) {
	// All entries exceed thresholds — should return original list as fallback.
	entries := []qualityEntry{
		{tag: "a", latency: 300 * time.Millisecond, loss: 0.5, index: 0},
		{tag: "b", latency: 400 * time.Millisecond, loss: 0.6, index: 1},
	}

	cfg := QualityConfig{
		MaxLatency:  100 * time.Millisecond,
		MaxLossRate: 0.1,
	}

	result := filterByQuality(entries, cfg)

	// Must return original list (graceful fallback), not empty.
	if len(result) != len(entries) {
		t.Fatalf("expected %d entries (fallback), got %d", len(entries), len(result))
	}
	if result[0].tag != "a" || result[1].tag != "b" {
		t.Errorf("fallback entries = %v, want original order", result)
	}
}

func TestQualityFilter_NoLimits(t *testing.T) {
	// Zero config means no filtering — all entries pass through.
	entries := []qualityEntry{
		{tag: "a", latency: 999 * time.Millisecond, loss: 0.99, index: 0},
		{tag: "b", latency: 500 * time.Millisecond, loss: 0.5, index: 1},
	}

	cfg := QualityConfig{} // MaxLatency=0, MaxLossRate=0

	result := filterByQuality(entries, cfg)

	if len(result) != len(entries) {
		t.Fatalf("expected %d entries (no filtering), got %d", len(entries), len(result))
	}
}

func TestQualityScore_LossyNodePenalized(t *testing.T) {
	scoreLossy := qualityScore(200*time.Millisecond, 0.01)
	scoreClean := qualityScore(250*time.Millisecond, 0.0)
	assert.Greater(t, scoreLossy, scoreClean,
		"1%% loss at 200ms should score worse than 0%% loss at 250ms")
}
