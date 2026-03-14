package checkperf

import (
	"strings"
	"testing"
)

func TestParseBenchLine(t *testing.T) {
	tests := []struct {
		line    string
		ok      bool
		name    string
		ns      float64
		allocs  int64
	}{
		{
			line:   "BenchmarkBBROnAck-8   	 5000000	       450 ns/op	       3 allocs/op",
			ok:     true,
			name:   "BenchmarkBBROnAck",
			ns:     450,
			allocs: 3,
		},
		{
			line:   "BenchmarkPadding-12   	 1000000	      7200 ns/op	     128 B/op	      12 allocs/op",
			ok:     true,
			name:   "BenchmarkPadding",
			ns:     7200,
			allocs: 12,
		},
		{
			line: "not a benchmark line",
			ok:   false,
		},
		{
			line:   "BenchmarkSimple   	 1000000	       100.5 ns/op",
			ok:     true,
			name:   "BenchmarkSimple",
			ns:     100.5,
			allocs: 0,
		},
	}

	for _, tt := range tests {
		br, ok := parseBenchLine(tt.line)
		if ok != tt.ok {
			t.Errorf("parseBenchLine(%q): ok = %v, want %v", tt.line, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		if br.Name != tt.name {
			t.Errorf("name = %q, want %q", br.Name, tt.name)
		}
		if br.NsPerOp != tt.ns {
			t.Errorf("ns = %f, want %f", br.NsPerOp, tt.ns)
		}
		if br.AllocsPerOp != tt.allocs {
			t.Errorf("allocs = %d, want %d", br.AllocsPerOp, tt.allocs)
		}
	}
}

func TestParseBenchmarksJSON(t *testing.T) {
	input := `{"Action":"output","Test":"BenchmarkBBROnAck","Output":"BenchmarkBBROnAck-8   \t 5000000\t       450 ns/op\t       3 allocs/op\n"}
{"Action":"output","Test":"BenchmarkPadding","Output":"BenchmarkPadding-8   \t 1000000\t      7200 ns/op\t      12 allocs/op\n"}
{"Action":"pass","Test":"BenchmarkBBROnAck"}
`
	results, err := ParseBenchmarks(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Name != "BenchmarkBBROnAck" {
		t.Errorf("results[0].Name = %q", results[0].Name)
	}
}

func TestCheck(t *testing.T) {
	budgets := []Budget{
		{Name: "BenchmarkBBROnAck", MaxNsPerOp: 1000, MaxAllocsPerOp: 5},
		{Name: "BenchmarkPadding", MaxNsPerOp: 5000, MaxAllocsPerOp: 10},
		{Name: "BenchmarkMissing", MaxNsPerOp: 100, MaxAllocsPerOp: 1},
	}
	results := []BenchResult{
		{Name: "BenchmarkBBROnAck", NsPerOp: 450, AllocsPerOp: 3},
		{Name: "BenchmarkPadding", NsPerOp: 7200, AllocsPerOp: 12},
	}

	checks := Check(budgets, results)
	if len(checks) != 3 {
		t.Fatalf("got %d checks, want 3", len(checks))
	}

	// BBR should pass.
	if !checks[0].Pass() {
		t.Error("BBR should pass")
	}

	// Padding should fail (both ns and allocs exceed).
	if checks[1].Pass() {
		t.Error("Padding should fail")
	}
	if checks[1].NsPass {
		t.Error("Padding ns should fail")
	}
	if checks[1].AllocsPass {
		t.Error("Padding allocs should fail")
	}

	// Missing should be missing.
	if !checks[2].Missing {
		t.Error("Missing should be missing")
	}
}

func TestCheckRegexMatch(t *testing.T) {
	budgets := []Budget{
		{Name: "BenchmarkDNSTrieLookup.*", MaxNsPerOp: 2000, MaxAllocsPerOp: 5},
	}
	results := []BenchResult{
		{Name: "BenchmarkDNSTrieLookup/simple", NsPerOp: 1500, AllocsPerOp: 3},
	}
	checks := Check(budgets, results)
	if checks[0].Missing {
		t.Error("should match via regex")
	}
	if !checks[0].Pass() {
		t.Error("should pass")
	}
}

func TestFormatResults(t *testing.T) {
	checks := []CheckResult{
		{
			Budget:     Budget{Name: "BenchmarkA", MaxNsPerOp: 1000, MaxAllocsPerOp: 5},
			Result:     &BenchResult{Name: "BenchmarkA", NsPerOp: 500, AllocsPerOp: 3},
			NsPass:     true,
			AllocsPass: true,
		},
		{
			Budget:  Budget{Name: "BenchmarkB", MaxNsPerOp: 100, MaxAllocsPerOp: 1},
			Missing: true,
		},
	}

	out := FormatResults(checks)
	if !strings.Contains(out, "PASS") {
		t.Error("expected PASS in output")
	}
	if !strings.Contains(out, "SKIP") {
		t.Error("expected SKIP in output")
	}
}
