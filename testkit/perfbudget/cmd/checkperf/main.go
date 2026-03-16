// Command checkperf reads benchmark output from stdin and checks it against
// a performance budget file. It exits 0 if all budgets pass, 1 if any
// violations are found.
//
// Usage:
//
//	go test -bench=. ./test/ | checkperf .perf-budget.yaml
package main

import (
	"fmt"
	"os"

	"github.com/shuttleX/shuttle/testkit/perfbudget"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <budget-file.yaml>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  Reads benchmark output from stdin.\n")
		fmt.Fprintf(os.Stderr, "  Example: go test -bench=. ./test/ | %s .perf-budget.yaml\n", os.Args[0])
		os.Exit(2)
	}

	budgetPath := os.Args[1]
	bf, err := perfbudget.LoadBudgets(budgetPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading budgets: %v\n", err)
		os.Exit(2)
	}

	results, err := perfbudget.ParseBenchOutput(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing benchmark output: %v\n", err)
		os.Exit(2)
	}

	if len(results) == 0 {
		fmt.Fprintf(os.Stderr, "Warning: no benchmark results found in input.\n")
		os.Exit(0)
	}

	fmt.Fprintf(os.Stdout, "Parsed %d benchmark result(s).\n", len(results))
	fmt.Fprintf(os.Stdout, "Checking against %d budget(s)...\n\n", len(bf.Budgets))

	violations := perfbudget.Check(bf.Budgets, results)
	fmt.Print(perfbudget.FormatViolations(violations))

	if len(violations) > 0 {
		os.Exit(1)
	}
}
