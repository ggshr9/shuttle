package server

import (
	"context"
	"net"
)

// setLookupIPAddr replaces the package-level resolver hook for the duration
// of a test. Returns a restore function.
func setLookupIPAddr(fn func(ctx context.Context, host string) ([]net.IPAddr, error)) func() {
	prev := lookupIPAddr
	lookupIPAddr = fn
	return func() { lookupIPAddr = prev }
}
