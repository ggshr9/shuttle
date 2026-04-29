package metrics

import (
	"strings"
	"sync"
	"testing"
)

func TestLabeledCounter_BasicIncrement(t *testing.T) {
	c := newLabeledCounter("shuttle_test_total", []string{"transport", "result"})
	c.Inc("h3", "ok")
	c.Inc("h3", "ok")
	c.Inc("h3", "fail")

	var sb strings.Builder
	c.write(&sb, "Test counter")

	out := sb.String()
	if !strings.Contains(out, `shuttle_test_total{transport="h3",result="ok"} 2`) {
		t.Fatalf("expected ok=2 line, got:\n%s", out)
	}
	if !strings.Contains(out, `shuttle_test_total{transport="h3",result="fail"} 1`) {
		t.Fatalf("expected fail=1 line, got:\n%s", out)
	}
	if !strings.Contains(out, "# TYPE shuttle_test_total counter") {
		t.Fatalf("expected TYPE line, got:\n%s", out)
	}
}

func TestLabeledCounter_LabelArityMismatchPanics(t *testing.T) {
	c := newLabeledCounter("x", []string{"a", "b"})
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on arity mismatch")
		}
	}()
	c.Inc("only-one")
}

func TestLabeledCounter_Concurrent(t *testing.T) {
	c := newLabeledCounter("shuttle_concurrent", []string{"k"})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.Inc("a") }()
	}
	wg.Wait()

	var sb strings.Builder
	c.write(&sb, "concurrent")
	if !strings.Contains(sb.String(), `shuttle_concurrent{k="a"} 100`) {
		t.Fatalf("expected 100, got %s", sb.String())
	}
}

func TestLabeledHistogram_BucketBoundary(t *testing.T) {
	buckets := []float64{0.1, 0.5, 1.0}
	h := newLabeledHistogram("shuttle_dur_seconds", buckets, []string{"transport"})

	// Three observations: 0.05, 0.5, 2.0 — should land in <=0.1, <=0.5, +Inf
	h.Observe(0.05, "h3")
	h.Observe(0.5, "h3")
	h.Observe(2.0, "h3")

	var sb strings.Builder
	h.write(&sb, "Duration")
	out := sb.String()

	for _, want := range []string{
		`shuttle_dur_seconds_bucket{transport="h3",le="0.1"} 1`,
		`shuttle_dur_seconds_bucket{transport="h3",le="0.5"} 2`,
		`shuttle_dur_seconds_bucket{transport="h3",le="1.0"} 2`,
		`shuttle_dur_seconds_bucket{transport="h3",le="+Inf"} 3`,
		`shuttle_dur_seconds_count{transport="h3"} 3`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing line %q in:\n%s", want, out)
		}
	}
}
