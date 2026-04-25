//go:build tools
// +build tools

// Package tools tracks build-time dependencies that aren't otherwise
// referenced by application code. The build-tag ensures this file is
// never compiled into production binaries — it exists solely so
// `go mod tidy` retains the imported packages in go.mod / go.sum.
//
// gomobile requires golang.org/x/mobile/bind to be present in the
// module's dependency graph; without it `gomobile bind` fails with
// "no Go package in golang.org/x/mobile/bind". Importing it here
// pins it to the module without affecting runtime size.
package tools

import (
	_ "golang.org/x/mobile/bind"
)
