// Command checkperf compares Go benchmark results against a performance budget.
//
// Usage:
//
//	go test -bench=. -benchmem -json ./... | checkperf [--budget FILE]
//	checkperf --budget .perf-budget.yaml --input results.json
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/shuttleX/shuttle/internal/checkperf"
)

func main() {
	budgetPath := flag.String("budget", ".perf-budget.yaml", "path to performance budget YAML file")
	inputPath := flag.String("input", "", "path to benchmark results file (default: stdin)")
	flag.Parse()

	bf, err := checkperf.LoadBudgets(*budgetPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}

	input := os.Stdin
	if *inputPath != "" {
		f, err := os.Open(*inputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(2)
		}
		defer f.Close()
		input = f
	}

	results, err := checkperf.ParseBenchmarks(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing benchmarks: %v\n", err)
		os.Exit(2)
	}

	checks := checkperf.Check(bf.Budgets, results)
	fmt.Print(checkperf.FormatResults(checks))

	for _, c := range checks {
		if !c.Missing && !c.Pass() {
			os.Exit(1)
		}
	}
}
