package perfbudget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleBenchOutput = `goos: linux
goarch: amd64
pkg: github.com/shuttle-proxy/shuttle/test
cpu: AMD EPYC 7B13
BenchmarkBBROnAck-8              5000000               234 ns/op          48 B/op          2 allocs/op
BenchmarkBrutalOnAck-8          10000000               112 ns/op           0 B/op          0 allocs/op
BenchmarkEncryptDecrypt-8         500000              3456 ns/op         512 B/op          4 allocs/op
BenchmarkReplayFilter-8         20000000                89 ns/op           0 B/op          1 allocs/op
BenchmarkDomainTrieLookup-8      3000000               456 ns/op          32 B/op          1 allocs/op
BenchmarkRouterMatch-8           2000000               678 ns/op          64 B/op          2 allocs/op
BenchmarkBufferPool-8           50000000                34 ns/op           0 B/op          0 allocs/op
BenchmarkPadding-8               1000000              1234 ns/op         128 B/op          3 allocs/op
PASS
ok      github.com/shuttle-proxy/shuttle/test   12.345s
`

func TestParseBenchOutput(t *testing.T) {
	results, err := ParseBenchOutput(strings.NewReader(sampleBenchOutput))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 8 {
		t.Fatalf("expected 8 results, got %d", len(results))
	}

	// Spot-check a few entries.
	r := results[0]
	if r.Name != "BenchmarkBBROnAck-8" {
		t.Errorf("expected name BenchmarkBBROnAck-8, got %s", r.Name)
	}
	if r.NsPerOp != 234 {
		t.Errorf("expected 234 ns/op, got %d", r.NsPerOp)
	}
	if r.BytesPerOp != 48 {
		t.Errorf("expected 48 B/op, got %d", r.BytesPerOp)
	}
	if r.AllocsPerOp != 2 {
		t.Errorf("expected 2 allocs/op, got %d", r.AllocsPerOp)
	}

	// Check BufferPool (zero allocs).
	r = results[6]
	if r.Name != "BenchmarkBufferPool-8" {
		t.Errorf("expected BenchmarkBufferPool-8, got %s", r.Name)
	}
	if r.NsPerOp != 34 {
		t.Errorf("expected 34 ns/op, got %d", r.NsPerOp)
	}
	if r.AllocsPerOp != 0 {
		t.Errorf("expected 0 allocs/op, got %d", r.AllocsPerOp)
	}
}

func TestParseBenchOutputEmpty(t *testing.T) {
	results, err := ParseBenchOutput(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestParseBenchOutputMalformed(t *testing.T) {
	garbage := `this is not benchmark output
=== RUN TestSomething
--- PASS: TestSomething (0.00s)
random garbage line 12345
PASS
ok      some/pkg   0.001s
`
	results, err := ParseBenchOutput(strings.NewReader(garbage))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results from garbage input, got %d", len(results))
	}
}

func TestCheckNoBudgets(t *testing.T) {
	results := []Result{
		{Name: "BenchmarkFoo-8", NsPerOp: 100},
	}
	violations := Check(nil, results)
	if len(violations) != 0 {
		t.Fatalf("expected no violations with no budgets, got %d", len(violations))
	}
}

func TestCheckAllPass(t *testing.T) {
	budgets := []Budget{
		{Name: "BenchmarkFoo", MaxNsPerOp: 1000, MaxAllocsPerOp: 5},
	}
	results := []Result{
		{Name: "BenchmarkFoo-8", NsPerOp: 500, AllocsPerOp: 2},
	}
	violations := Check(budgets, results)
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(violations))
	}
}

func TestCheckNsPerOpExceeded(t *testing.T) {
	budgets := []Budget{
		{Name: "BenchmarkFoo", MaxNsPerOp: 100},
	}
	results := []Result{
		{Name: "BenchmarkFoo-8", NsPerOp: 200},
	}
	violations := Check(budgets, results)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	v := violations[0]
	if v.Field != "ns/op" {
		t.Errorf("expected field ns/op, got %s", v.Field)
	}
	if v.Got != 200 || v.Limit != 100 {
		t.Errorf("expected got=200 limit=100, got got=%d limit=%d", v.Got, v.Limit)
	}
}

func TestCheckAllocsExceeded(t *testing.T) {
	budgets := []Budget{
		{Name: "BenchmarkFoo", MaxAllocsPerOp: 2},
	}
	results := []Result{
		{Name: "BenchmarkFoo-8", AllocsPerOp: 5},
	}
	violations := Check(budgets, results)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Field != "allocs/op" {
		t.Errorf("expected field allocs/op, got %s", violations[0].Field)
	}
}

func TestCheckBytesExceeded(t *testing.T) {
	budgets := []Budget{
		{Name: "BenchmarkFoo", MaxBytesPerOp: 100},
	}
	results := []Result{
		{Name: "BenchmarkFoo-8", BytesPerOp: 256},
	}
	violations := Check(budgets, results)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Field != "bytes/op" {
		t.Errorf("expected field bytes/op, got %s", violations[0].Field)
	}
}

func TestCheckMultipleViolations(t *testing.T) {
	budgets := []Budget{
		{Name: "BenchmarkA", MaxNsPerOp: 100},
		{Name: "BenchmarkB", MaxAllocsPerOp: 1, MaxBytesPerOp: 50},
	}
	results := []Result{
		{Name: "BenchmarkA-8", NsPerOp: 200},
		{Name: "BenchmarkB-8", AllocsPerOp: 5, BytesPerOp: 100},
	}
	violations := Check(budgets, results)
	if len(violations) != 3 {
		t.Fatalf("expected 3 violations, got %d", len(violations))
	}
}

func TestCheckRegexMatch(t *testing.T) {
	budgets := []Budget{
		{Name: "BenchmarkDNSTrie.*", MaxNsPerOp: 1000},
	}
	results := []Result{
		{Name: "BenchmarkDNSTrieLookup/hit-8", NsPerOp: 500},
		{Name: "BenchmarkDNSTrieLookup/miss-8", NsPerOp: 1500},
		{Name: "BenchmarkOther-8", NsPerOp: 9999},
	}
	violations := Check(budgets, results)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Result.Name != "BenchmarkDNSTrieLookup/miss-8" {
		t.Errorf("expected violation for miss, got %s", violations[0].Result.Name)
	}
}

func TestLoadBudgets(t *testing.T) {
	content := `budgets:
  - name: "BenchmarkFoo"
    max_ns_per_op: 1000
    max_allocs_per_op: 5
  - name: "BenchmarkBar"
    max_bytes_per_op: 256
`
	dir := t.TempDir()
	path := filepath.Join(dir, ".perf-budget.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	bf, err := LoadBudgets(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bf.Budgets) != 2 {
		t.Fatalf("expected 2 budgets, got %d", len(bf.Budgets))
	}
	if bf.Budgets[0].Name != "BenchmarkFoo" {
		t.Errorf("expected BenchmarkFoo, got %s", bf.Budgets[0].Name)
	}
	if bf.Budgets[0].MaxNsPerOp != 1000 {
		t.Errorf("expected 1000, got %d", bf.Budgets[0].MaxNsPerOp)
	}
	if bf.Budgets[0].MaxAllocsPerOp != 5 {
		t.Errorf("expected 5, got %d", bf.Budgets[0].MaxAllocsPerOp)
	}
	if bf.Budgets[1].MaxBytesPerOp != 256 {
		t.Errorf("expected 256, got %d", bf.Budgets[1].MaxBytesPerOp)
	}
}

func TestLoadBudgetsMissing(t *testing.T) {
	_, err := LoadBudgets("/nonexistent/path/.perf-budget.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
