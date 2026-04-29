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
