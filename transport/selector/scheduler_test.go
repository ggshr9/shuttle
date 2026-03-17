package selector

import (
	"sync/atomic"
	"testing"
	"time"
)

func makePaths(n int, available bool) []*PathMetrics {
	paths := make([]*PathMetrics, n)
	for i := range paths {
		paths[i] = &PathMetrics{
			Available: available,
			Latency:   time.Duration(i+1) * time.Millisecond,
		}
		if available {
			paths[i].Conn = &fakeConn{} // non-nil so eligible
		}
	}
	return paths
}

func TestFilterEligibleBasic(t *testing.T) {
	paths := makePaths(3, true)
	paths[1].Available = false // make one ineligible

	eligible := filterEligible(paths)
	if len(eligible) != 2 {
		t.Fatalf("expected 2 eligible, got %d", len(eligible))
	}
}

func TestFilterEligibleExcludesHighFailures(t *testing.T) {
	paths := makePaths(3, true)
	atomic.StoreInt64(&paths[0].Failures, 3) // ≥ 3 failures

	eligible := filterEligible(paths)
	if len(eligible) != 2 {
		t.Fatalf("expected 2 eligible (excluding high failures), got %d", len(eligible))
	}
}

func TestFilterEligibleExcludesNilConn(t *testing.T) {
	paths := makePaths(3, true)
	paths[2].Conn = nil

	eligible := filterEligible(paths)
	if len(eligible) != 2 {
		t.Fatalf("expected 2 eligible (excluding nil conn), got %d", len(eligible))
	}
}

func TestFilterEligibleNoneAvailable(t *testing.T) {
	paths := makePaths(3, false)
	eligible := filterEligible(paths)
	if len(eligible) != 0 {
		t.Fatalf("expected 0 eligible, got %d", len(eligible))
	}
}

func TestMinLatencyScheduler(t *testing.T) {
	paths := makePaths(3, true)
	paths[0].Latency = 10 * time.Millisecond
	paths[1].Latency = 5 * time.Millisecond
	paths[2].Latency = 15 * time.Millisecond

	s := NewMinLatencyScheduler()
	picked := s.Pick(paths)
	if picked != paths[1] {
		t.Fatalf("expected path with 5ms latency, got %v", picked.Latency)
	}
}

func TestMinLatencySchedulerEmpty(t *testing.T) {
	s := NewMinLatencyScheduler()
	if s.Pick(nil) != nil {
		t.Fatal("expected nil for empty paths")
	}
}

func TestLoadBalanceScheduler(t *testing.T) {
	paths := makePaths(3, true)
	atomic.StoreInt64(&paths[0].ActiveStreams, 5)
	atomic.StoreInt64(&paths[1].ActiveStreams, 2)
	atomic.StoreInt64(&paths[2].ActiveStreams, 8)

	s := NewLoadBalanceScheduler()
	picked := s.Pick(paths)
	if picked != paths[1] {
		t.Fatalf("expected path with fewest streams (2), got %d", atomic.LoadInt64(&picked.ActiveStreams))
	}
}

func TestLoadBalanceSchedulerEmpty(t *testing.T) {
	s := NewLoadBalanceScheduler()
	if s.Pick(nil) != nil {
		t.Fatal("expected nil for empty paths")
	}
}

func TestWeightedLatencyScheduler(t *testing.T) {
	paths := makePaths(2, true)
	paths[0].Latency = 1 * time.Millisecond  // low latency → high weight
	paths[1].Latency = 100 * time.Millisecond // high latency → low weight

	s := NewWeightedLatencyScheduler()

	// Run many picks — lower latency path should be picked more often
	counts := map[int]int{0: 0, 1: 0}
	for i := 0; i < 1000; i++ {
		picked := s.Pick(paths)
		if picked == paths[0] {
			counts[0]++
		} else {
			counts[1]++
		}
	}

	// Path 0 (1ms) should get ~99x more picks than path 1 (100ms)
	if counts[0] < counts[1] {
		t.Fatalf("expected low-latency path to be picked more often: path0=%d, path1=%d", counts[0], counts[1])
	}
}

func TestWeightedLatencySchedulerSingle(t *testing.T) {
	paths := makePaths(1, true)
	s := NewWeightedLatencyScheduler()
	picked := s.Pick(paths)
	if picked != paths[0] {
		t.Fatal("expected the only path to be picked")
	}
}
