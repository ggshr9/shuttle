// Package perfbudget defines performance budgets for benchmarks and checks
// benchmark results against them. It is used as a CI gate to catch performance
// regressions.
package perfbudget

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Budget defines a performance threshold for a benchmark.
type Budget struct {
	Name             string  `yaml:"name"`
	MaxNsPerOp       int64   `yaml:"max_ns_per_op"`
	MaxAllocsPerOp   int64   `yaml:"max_allocs_per_op"`
	MaxBytesPerOp    int64   `yaml:"max_bytes_per_op"`
	MinThroughputMBps float64 `yaml:"min_throughput_mbps"`
}

// Result represents a parsed benchmark result.
type Result struct {
	Name        string
	NsPerOp     int64
	AllocsPerOp int64
	BytesPerOp  int64
}

// BudgetFile represents the .perf-budget.yaml file.
type BudgetFile struct {
	Budgets []Budget `yaml:"budgets"`
}

// Violation represents a budget that was exceeded.
type Violation struct {
	Budget Budget
	Result Result
	Field  string // "ns/op", "allocs/op", "bytes/op"
	Got    int64
	Limit  int64
}

// benchLine matches standard Go benchmark output lines, e.g.:
//
//	BenchmarkFoo-8   1000   1234 ns/op   56 B/op   3 allocs/op
var benchLine = regexp.MustCompile(
	`^(Benchmark\S+)\s+\d+\s+(.+)$`,
)

var (
	nsPerOpRe     = regexp.MustCompile(`([\d.]+)\s+ns/op`)
	bytesPerOpRe  = regexp.MustCompile(`(\d+)\s+B/op`)
	allocsPerOpRe = regexp.MustCompile(`(\d+)\s+allocs/op`)
)

// ParseBenchOutput parses `go test -bench` output into Results.
func ParseBenchOutput(r io.Reader) ([]Result, error) {
	var results []Result
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		m := benchLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		rest := m[2]

		var res Result
		res.Name = name

		if sub := nsPerOpRe.FindStringSubmatch(rest); sub != nil {
			f, err := strconv.ParseFloat(sub[1], 64)
			if err == nil {
				res.NsPerOp = int64(f)
			}
		}
		if sub := bytesPerOpRe.FindStringSubmatch(rest); sub != nil {
			v, err := strconv.ParseInt(sub[1], 10, 64)
			if err == nil {
				res.BytesPerOp = v
			}
		}
		if sub := allocsPerOpRe.FindStringSubmatch(rest); sub != nil {
			v, err := strconv.ParseInt(sub[1], 10, 64)
			if err == nil {
				res.AllocsPerOp = v
			}
		}

		results = append(results, res)
	}
	return results, scanner.Err()
}

// LoadBudgets loads budgets from a YAML file.
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

// Check compares results against budgets and returns any violations.
func Check(budgets []Budget, results []Result) []Violation {
	var violations []Violation
	for _, b := range budgets {
		re, err := regexp.Compile(b.Name)
		if err != nil {
			// If the budget name is not a valid regex, treat it as a literal.
			re = regexp.MustCompile(regexp.QuoteMeta(b.Name))
		}
		for _, r := range results {
			if !re.MatchString(r.Name) {
				continue
			}
			if b.MaxNsPerOp > 0 && r.NsPerOp > b.MaxNsPerOp {
				violations = append(violations, Violation{
					Budget: b,
					Result: r,
					Field:  "ns/op",
					Got:    r.NsPerOp,
					Limit:  b.MaxNsPerOp,
				})
			}
			if b.MaxAllocsPerOp > 0 && r.AllocsPerOp > b.MaxAllocsPerOp {
				violations = append(violations, Violation{
					Budget: b,
					Result: r,
					Field:  "allocs/op",
					Got:    r.AllocsPerOp,
					Limit:  b.MaxAllocsPerOp,
				})
			}
			if b.MaxBytesPerOp > 0 && r.BytesPerOp > b.MaxBytesPerOp {
				violations = append(violations, Violation{
					Budget: b,
					Result: r,
					Field:  "bytes/op",
					Got:    r.BytesPerOp,
					Limit:  b.MaxBytesPerOp,
				})
			}
		}
	}
	return violations
}

// FormatViolations returns a human-readable summary of violations.
func FormatViolations(violations []Violation) string {
	if len(violations) == 0 {
		return "All performance budgets passed."
	}
	var sb strings.Builder
	sb.WriteString("PERFORMANCE BUDGET VIOLATIONS:\n")
	sb.WriteString(strings.Repeat("-", 80) + "\n")
	sb.WriteString(fmt.Sprintf("%-40s %-12s %12s %12s\n", "Benchmark", "Metric", "Got", "Limit"))
	sb.WriteString(strings.Repeat("-", 80) + "\n")
	for _, v := range violations {
		sb.WriteString(fmt.Sprintf("%-40s %-12s %12d %12d\n",
			v.Result.Name, v.Field, v.Got, v.Limit))
	}
	sb.WriteString(strings.Repeat("-", 80) + "\n")
	sb.WriteString(fmt.Sprintf("%d violation(s) found.\n", len(violations)))
	return sb.String()
}
