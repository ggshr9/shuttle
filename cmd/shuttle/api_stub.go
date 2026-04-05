//go:build minimal

package main

import (
	"fmt"
	"os"
)

func runAPI(configPath, listen string, autoConnect bool) {
	fmt.Fprintln(os.Stderr, "API server not available: binary built with -tags minimal")
	os.Exit(1)
}
