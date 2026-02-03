//go:build windows

package proxy

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"

	"golang.zx2c4.com/wireguard/tun"
)

// globalWinTun stores the WinTun device reference for cleanup.
// This is needed because WinTun device lifecycle is managed separately from *os.File.
var (
	winTunMu        sync.Mutex
	globalWinTunDev tun.Device
)

func (t *TUNServer) createTUN() (*os.File, error) {
	winTunMu.Lock()
	defer winTunMu.Unlock()

	// Create WinTun device
	device, err := tun.CreateTUN(t.config.DeviceName, t.config.MTU)
	if err != nil {
		return nil, fmt.Errorf("create wintun device: %w", err)
	}

	name, err := device.Name()
	if err != nil {
		device.Close()
		return nil, fmt.Errorf("get device name: %w", err)
	}

	// Store device reference for later cleanup
	globalWinTunDev = device

	// WinTun doesn't provide a traditional file descriptor.
	// Instead, we get the file handle from the device.
	f := device.File()
	if f == nil {
		device.Close()
		globalWinTunDev = nil
		return nil, fmt.Errorf("wintun: no file descriptor available")
	}

	t.config.DeviceName = name
	return f, nil
}

func (t *TUNServer) configureTUN() error {
	ip, ipNet, err := net.ParseCIDR(t.config.CIDR)
	if err != nil {
		return fmt.Errorf("parse CIDR %q: %w", t.config.CIDR, err)
	}

	// Calculate local IP (first usable IP in the subnet)
	localIP := make(net.IP, len(ip))
	copy(localIP, ip)
	localIP[len(localIP)-1]++

	// Calculate subnet mask
	mask := net.IP(ipNet.Mask)

	dev := t.config.DeviceName

	// Configure IP address using netsh
	// netsh interface ip set address name="shuttle0" static 198.18.0.1 255.255.0.0
	cmds := [][]string{
		{"netsh", "interface", "ip", "set", "address",
			fmt.Sprintf("name=%s", dev),
			"static", localIP.String(), mask.String()},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			// Ignore "object already exists" errors
			if !strings.Contains(string(out), "already exists") {
				return fmt.Errorf("%s: %s: %w", args[0], string(out), err)
			}
		}
	}

	return nil
}

func (t *TUNServer) setupRoutes() error {
	_, ipNet, err := net.ParseCIDR(t.config.CIDR)
	if err != nil {
		return err
	}

	// Calculate gateway (local IP)
	localIP := make(net.IP, 4)
	copy(localIP, ipNet.IP)
	localIP[3]++

	// Calculate subnet mask
	mask := net.IP(ipNet.Mask)

	// Add route using route command
	// route add 198.18.0.0 mask 255.255.0.0 198.18.0.1
	out, err := exec.Command("route", "add",
		ipNet.IP.String(),
		"mask", mask.String(),
		localIP.String(),
	).CombinedOutput()
	if err != nil {
		// Ignore "route already exists" errors
		if !strings.Contains(string(out), "already exists") {
			return fmt.Errorf("route add: %s: %w", string(out), err)
		}
	}

	t.logger.Info("routes configured", "cidr", ipNet.String(), "dev", t.config.DeviceName)
	return nil
}

func (t *TUNServer) teardownRoutes() {
	_, ipNet, err := net.ParseCIDR(t.config.CIDR)
	if err != nil {
		return
	}

	mask := net.IP(ipNet.Mask)

	// Delete route
	exec.Command("route", "delete",
		ipNet.IP.String(),
		"mask", mask.String(),
	).Run()

	// Close the WinTun device
	winTunMu.Lock()
	if globalWinTunDev != nil {
		globalWinTunDev.Close()
		globalWinTunDev = nil
	}
	winTunMu.Unlock()
}

// AddMeshRoute adds a route for the mesh subnet through the TUN device.
func (t *TUNServer) AddMeshRoute(cidr string) error {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("parse mesh CIDR %q: %w", cidr, err)
	}

	// Calculate gateway from TUN config
	localIP, _, err := net.ParseCIDR(t.config.CIDR)
	if err != nil {
		return err
	}
	gw := make(net.IP, len(localIP))
	copy(gw, localIP)
	gw[len(gw)-1]++

	mask := net.IP(ipNet.Mask)

	out, err := exec.Command("route", "add",
		ipNet.IP.String(),
		"mask", mask.String(),
		gw.String(),
	).CombinedOutput()
	if err != nil {
		if !strings.Contains(string(out), "already exists") {
			return fmt.Errorf("route add mesh: %s: %w", string(out), err)
		}
	}

	t.logger.Info("mesh route added", "cidr", ipNet.String())
	return nil
}
