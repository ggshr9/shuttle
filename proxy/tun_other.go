//go:build !linux && !darwin

package proxy

import (
	"fmt"
	"os"
	"runtime"
)

func (t *TUNServer) createTUN() (*os.File, error) {
	return nil, fmt.Errorf("tun: platform %q is not supported — use SOCKS5 or HTTP proxy instead; "+
		"on Windows consider github.com/WireGuard/wintun for native TUN support", runtime.GOOS)
}

func (t *TUNServer) configureTUN() error {
	return fmt.Errorf("tun: configure not supported on %q", runtime.GOOS)
}

func (t *TUNServer) setupRoutes() error {
	return fmt.Errorf("tun: routes not supported on %q", runtime.GOOS)
}

func (t *TUNServer) teardownRoutes() {}
