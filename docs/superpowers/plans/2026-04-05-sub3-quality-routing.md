# Sub-3: Congestion-Aware Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `quality` strategy to OutboundGroup that automatically selects the best outbound based on real-time latency and packet loss metrics.

**Architecture:** OutboundGroup gains a `GroupQuality` strategy that leverages Selector probe data (latency + loss, collected every 30s) to rank member outbounds. Members exceeding configured thresholds are deprioritized. Falls back to failover when all members degrade.

**Tech Stack:** Go 1.24+, existing Selector probe infrastructure, OutboundGroup

---

## File Structure

- Create: `engine/outbound_quality.go` — quality scoring + ranking logic
- Create: `engine/outbound_quality_test.go` — tests
- Modify: `engine/outbound_group.go` — add GroupQuality strategy, wire quality selector
- Modify: `engine/engine.go` — expose probe data accessor for outbound quality
- Modify: `engine/engine_inbound.go` — pass probe accessor when building groups

---

## Task 1: Quality Scorer

**Files:**
- Create: `engine/outbound_quality.go`
- Create: `engine/outbound_quality_test.go`

- [ ] **Step 1: Write failing test**

```go
// engine/outbound_quality_test.go
package engine

import (
	"testing"
	"time"
)

func TestQualityScore(t *testing.T) {
	// Low latency, zero loss = best score
	s1 := qualityScore(50*time.Millisecond, 0.0)
	// High latency, zero loss
	s2 := qualityScore(200*time.Millisecond, 0.0)
	// Low latency, high loss
	s3 := qualityScore(50*time.Millisecond, 0.05)

	if s1 >= s2 {
		t.Errorf("50ms should score better than 200ms: %f >= %f", s1, s2)
	}
	if s1 >= s3 {
		t.Errorf("0%% loss should score better than 5%% loss: %f >= %f", s1, s3)
	}
}

func TestQualityRank(t *testing.T) {
	entries := []qualityEntry{
		{tag: "slow", latency: 300 * time.Millisecond, loss: 0.0},
		{tag: "fast", latency: 30 * time.Millisecond, loss: 0.0},
		{tag: "lossy", latency: 50 * time.Millisecond, loss: 0.10},
	}
	ranked := rankByQuality(entries)
	if ranked[0].tag != "fast" {
		t.Errorf("expected 'fast' first, got %q", ranked[0].tag)
	}
}

func TestQualityFilter(t *testing.T) {
	entries := []qualityEntry{
		{tag: "ok", latency: 100 * time.Millisecond, loss: 0.01},
		{tag: "bad-latency", latency: 500 * time.Millisecond, loss: 0.0},
		{tag: "bad-loss", latency: 50 * time.Millisecond, loss: 0.10},
	}
	cfg := QualityConfig{MaxLatency: 200 * time.Millisecond, MaxLossRate: 0.05}
	filtered := filterByQuality(entries, cfg)
	if len(filtered) != 1 || filtered[0].tag != "ok" {
		t.Errorf("expected only 'ok', got %v", filtered)
	}
}
```

- [ ] **Step 2: Implement quality scoring**

```go
// engine/outbound_quality.go
package engine

import (
	"sort"
	"time"
)

// QualityConfig configures quality-based outbound selection.
type QualityConfig struct {
	MaxLatency    time.Duration `json:"max_latency"`     // e.g., 200ms; 0 = no limit
	MaxLossRate   float64       `json:"max_loss_rate"`   // e.g., 0.02 (2%); 0 = no limit
}

type qualityEntry struct {
	tag     string
	latency time.Duration
	loss    float64
	index   int // original index in outbound slice
}

// qualityScore computes a combined score (lower is better).
// Formula: latency_ms + loss_rate * 1000
func qualityScore(latency time.Duration, loss float64) float64 {
	return float64(latency.Milliseconds()) + loss*1000
}

// rankByQuality sorts entries by quality score (best first).
func rankByQuality(entries []qualityEntry) []qualityEntry {
	sort.Slice(entries, func(i, j int) bool {
		return qualityScore(entries[i].latency, entries[i].loss) <
			qualityScore(entries[j].latency, entries[j].loss)
	})
	return entries
}

// filterByQuality removes entries exceeding quality thresholds.
// If all entries are filtered out, returns the original list (failover mode).
func filterByQuality(entries []qualityEntry, cfg QualityConfig) []qualityEntry {
	if cfg.MaxLatency == 0 && cfg.MaxLossRate == 0 {
		return entries
	}
	var passed []qualityEntry
	for _, e := range entries {
		if cfg.MaxLatency > 0 && e.latency > cfg.MaxLatency {
			continue
		}
		if cfg.MaxLossRate > 0 && e.loss > cfg.MaxLossRate {
			continue
		}
		passed = append(passed, e)
	}
	if len(passed) == 0 {
		return entries // fallback: try all rather than none
	}
	return passed
}
```

- [ ] **Step 3: Run tests, commit**

Run: `./scripts/test.sh --run TestQuality --pkg ./engine/`
Commit: `feat(engine): add quality scoring and ranking for outbound selection`

---

## Task 2: Wire GroupQuality into OutboundGroup

**Files:**
- Modify: `engine/outbound_group.go` — add `GroupQuality` constant, quality config, probe accessor
- Modify: `engine/outbound_quality.go` — add `qualitySelect` method
- Test: `engine/outbound_group_test.go` — add quality strategy tests

- [ ] **Step 1: Add GroupQuality strategy constant**

In `engine/outbound_group.go`, add:
```go
const GroupQuality GroupStrategy = "quality"
```

- [ ] **Step 2: Extend OutboundGroup with quality config and probe accessor**

```go
type OutboundGroup struct {
	tag         string
	strategy    GroupStrategy
	outbounds   []adapter.Outbound
	counter     atomic.Uint64
	qualityCfg  QualityConfig
	probeGetter func() map[string]ProbeSnapshot // returns tag → {latency, loss}
}

// ProbeSnapshot is a point-in-time quality reading for an outbound.
type ProbeSnapshot struct {
	Latency   time.Duration
	Loss      float64
	Available bool
}
```

- [ ] **Step 3: Implement qualitySelect in DialContext**

When strategy is `GroupQuality`:
1. Call `probeGetter()` to get current probe data
2. Build qualityEntry list for each member outbound
3. Filter by QualityConfig thresholds
4. Rank remaining by quality score
5. Try ranked outbounds in order (failover among qualified)

- [ ] **Step 4: Write tests**

```go
func TestOutboundGroup_Quality(t *testing.T) {
	// Set up probeGetter returning different latencies
	// Verify lowest latency outbound is tried first
}

func TestOutboundGroup_Quality_Failover(t *testing.T) {
	// Best quality outbound fails, verify fallback to second-best
}
```

- [ ] **Step 5: Run tests, commit**

Run: `./scripts/test.sh --pkg ./engine/`
Commit: `feat(engine): add quality strategy to OutboundGroup for congestion-aware routing`

---

## Task 3: Expose Probe Data for OutboundGroup

**Files:**
- Modify: `engine/engine.go` — add method to get probe snapshots keyed by outbound tag
- Modify: `engine/engine_inbound.go` — pass probe accessor when building groups

- [ ] **Step 1: Add ProbeSnapshots method to Engine**

```go
// ProbeSnapshots returns current quality metrics for all proxy outbounds.
func (e *Engine) ProbeSnapshots() map[string]ProbeSnapshot {
	sel := e.selector()
	if sel == nil {
		return nil
	}
	probes := sel.Probes()
	result := make(map[string]ProbeSnapshot)
	for name, pr := range probes {
		result[name] = ProbeSnapshot{
			Latency:   pr.Latency,
			Loss:      pr.Loss,
			Available: pr.Available,
		}
	}
	return result
}
```

- [ ] **Step 2: Pass probe accessor in startInbounds**

When building groups in `engine_inbound.go`, pass `e.ProbeSnapshots` as the probe getter.

- [ ] **Step 3: Parse QualityConfig from group options**

Update `parseOutboundGroupConfig` to include quality config fields.

- [ ] **Step 4: Run all tests, commit**

Run: `./scripts/test.sh`
Commit: `feat(engine): wire quality probe data into OutboundGroup`

---

## Config Example

```yaml
outbounds:
  - tag: "us"
    type: "proxy"
    options: {server: "us.example.com:443"}
  - tag: "jp"
    type: "proxy"
    options: {server: "jp.example.com:443"}
  - tag: "smart"
    type: "group"
    options:
      strategy: "quality"
      outbounds: ["us", "jp"]
      max_latency: "200ms"
      max_loss_rate: 0.02

routing:
  default: "smart"
```
