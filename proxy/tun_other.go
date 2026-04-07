//go:build !linux && !darwin && !windows

package proxy

import (
	"fmt"
	"os"
	"runtime"
)

func (t *TUNServer) createTUN() (*os.File, error) {
	return nil, fmt.Errorf("tun: platform %q is not supported (supported: darwin, linux, windows) — use SOCKS5 or HTTP proxy instead", runtime.GOOS)
}

func (t *TUNServer) configureTUN() error {
	return fmt.Errorf("tun: configure not supported on %q (supported: darwin, linux, windows)", runtime.GOOS)
}

func (t *TUNServer) setupRoutes() error {
	return fmt.Errorf("tun: routes not supported on %q (supported: darwin, linux, windows)", runtime.GOOS)
}

func (t *TUNServer) teardownRoutes() {}

// AddMeshRoute adds a route for the mesh subnet through the TUN device.
func (t *TUNServer) AddMeshRoute(cidr string) error {
	return fmt.Errorf("tun: mesh routes not supported on %q (supported: darwin, linux, windows)", runtime.GOOS)
}
