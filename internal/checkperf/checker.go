// Package checkperf compares Go benchmark results against a performance budget.
package checkperf

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Budget defines a single performance budget entry.
type Budget struct {
	Name          string `yaml:"name"`
	MaxNsPerOp    int64  `yaml:"max_ns_per_op"`
	MaxAllocsPerOp int64  `yaml:"max_allocs_per_op"`
}

// BudgetFile is the top-level .perf-budget.yaml structure.
type BudgetFile struct {
	Budgets []Budget `yaml:"budgets"`
}

// BenchResult holds parsed benchmark results.
type BenchResult struct {
	Name      string
	NsPerOp   float64
	AllocsPerOp int64
}

// CheckResult is the outcome of checking one budget entry.
type CheckResult struct {
	Budget     Budget
	Result     *BenchResult // nil if no matching benchmark found
	NsPass     bool
	AllocsPass bool
	Missing    bool
}

func (r CheckResult) Pass() bool {
	return !r.Missing && r.NsPass && r.AllocsPass
}

// LoadBudgets reads a .perf-budget.yaml file.
func LoadBudgets(path string) (*BudgetFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading budget file: %w", err)
	}
	var bf BudgetFile
	if err := yaml.Unmarshal(data, &bf); err != nil {
		return nil, fmt.Errorf("parsing budget file: %w", err)
	}
	return &bf, nil
}

// testEvent is one line of `go test -json` output.
type testEvent struct {
	Action  string  `json:"Action"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
}

// ParseBenchmarks parses `go test -json` output and extracts benchmark results.
func ParseBenchmarks(r io.Reader) ([]BenchResult, error) {
	var results []BenchResult
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var ev testEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			// Not JSON — try parsing as raw benchmark line.
			if br, ok := parseBenchLine(line); ok {
				results = append(results, br)
			}
			continue
		}

		if ev.Action == "output" && strings.HasPrefix(strings.TrimSpace(ev.Output), "Benchmark") {
			if br, ok := parseBenchLine(strings.TrimSpace(ev.Output)); ok {
				results = append(results, br)
			}
		}
	}
	return results, scanner.Err()
}

// parseBenchLine parses a single benchmark output line like:
// BenchmarkBBROnAck-8    5000000    450 ns/op    3 allocs/op
var benchLineRe = regexp.MustCompile(
	`^(Benchmark\S+)\s+\d+\s+(\d+(?:\.\d+)?)\s+ns/op(?:\s+\d+\s+B/op)?(?:\s+(\d+)\s+allocs/op)?`,
)

func parseBenchLine(line string) (BenchResult, bool) {
	m := benchLineRe.FindStringSubmatch(line)
	if m == nil {
		return BenchResult{}, false
	}
	ns, _ := strconv.ParseFloat(m[2], 64)
	var allocs int64
	if m[3] != "" {
		allocs, _ = strconv.ParseInt(m[3], 10, 64)
	}
	// Strip -N suffix (CPU count).
	name := m[1]
	if idx := strings.LastIndex(name, "-"); idx > 0 {
		if _, err := strconv.Atoi(name[idx+1:]); err == nil {
			name = name[:idx]
		}
	}
	return BenchResult{Name: name, NsPerOp: ns, AllocsPerOp: allocs}, true
}

// Check compares benchmark results against budgets and returns results.
func Check(budgets []Budget, results []BenchResult) []CheckResult {
	var checks []CheckResult
	for _, b := range budgets {
		cr := CheckResult{Budget: b}
		br := findResult(b.Name, results)
		if br == nil {
			cr.Missing = true
			checks = append(checks, cr)
			continue
		}
		cr.Result = br
		cr.NsPass = int64(br.NsPerOp) <= b.MaxNsPerOp
		cr.AllocsPass = br.AllocsPerOp <= b.MaxAllocsPerOp
		checks = append(checks, cr)
	}
	return checks
}

// findResult finds the first benchmark result matching a budget name (supports regex).
func findResult(pattern string, results []BenchResult) *BenchResult {
	// Try exact match first.
	for i := range results {
		if results[i].Name == pattern {
			return &results[i]
		}
	}
	// Try regex match.
	re, err := regexp.Compile("^" + pattern + "$")
	if err != nil {
		return nil
	}
	for i := range results {
		if re.MatchString(results[i].Name) {
			return &results[i]
		}
	}
	return nil
}

// FormatResults produces a human-readable summary.
func FormatResults(checks []CheckResult) string {
	var b strings.Builder
	passed, failed, missing := 0, 0, 0

	for _, c := range checks {
		if c.Missing {
			missing++
			fmt.Fprintf(&b, "SKIP  %-45s  (no benchmark result found)\n", c.Budget.Name)
			continue
		}
		if c.Pass() {
			passed++
			fmt.Fprintf(&b, "PASS  %-45s  %6.0f ns/op  ≤ %-8d  %d allocs ≤ %d\n",
				c.Budget.Name, c.Result.NsPerOp, c.Budget.MaxNsPerOp,
				c.Result.AllocsPerOp, c.Budget.MaxAllocsPerOp)
		} else {
			failed++
			fmt.Fprintf(&b, "FAIL  %-45s  %6.0f ns/op  ≤ %-8d  %d allocs ≤ %d\n",
				c.Budget.Name, c.Result.NsPerOp, c.Budget.MaxNsPerOp,
				c.Result.AllocsPerOp, c.Budget.MaxAllocsPerOp)
		}
	}

	fmt.Fprintf(&b, "---\n")
	total := passed + failed
	if missing > 0 {
		fmt.Fprintf(&b, "%d/%d passed, %d FAILED, %d skipped (no result)\n", passed, total, failed, missing)
	} else {
		fmt.Fprintf(&b, "%d/%d passed, %d FAILED\n", passed, total, failed)
	}
	return b.String()
}
