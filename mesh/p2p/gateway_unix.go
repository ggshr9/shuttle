//go:build darwin || linux

package p2p

import (
	"bufio"
	"bytes"
	"net"
	"os/exec"
	"runtime"
	"strings"
)

// getDefaultGateway finds the default gateway IP by reading the routing table
func getDefaultGateway() (net.IP, error) {
	switch runtime.GOOS {
	case "darwin":
		return getDefaultGatewayDarwin()
	case "linux":
		return getDefaultGatewayLinux()
	default:
		return getDefaultGatewayFallback()
	}
}

// getDefaultGatewayDarwin reads the routing table on macOS
func getDefaultGatewayDarwin() (net.IP, error) {
	// netstat -nr | grep default
	cmd := exec.Command("route", "-n", "get", "default")
	output, err := cmd.Output()
	if err != nil {
		return getDefaultGatewayFallback()
	}

	// Parse output like:
	//    route to: default
	//    gateway: 192.168.1.1
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "gateway:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				ip := net.ParseIP(parts[1])
				if ip != nil && ip.To4() != nil {
					return ip.To4(), nil
				}
			}
		}
	}

	return getDefaultGatewayFallback()
}

// getDefaultGatewayLinux reads the routing table on Linux
func getDefaultGatewayLinux() (net.IP, error) {
	// Try /proc/net/route first (most reliable)
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return getDefaultGatewayFallback()
	}

	// Parse output like:
	// default via 192.168.1.1 dev eth0
	line := strings.TrimSpace(string(output))
	parts := strings.Fields(line)
	for i, part := range parts {
		if part == "via" && i+1 < len(parts) {
			ip := net.ParseIP(parts[i+1])
			if ip != nil && ip.To4() != nil {
				return ip.To4(), nil
			}
		}
	}

	return getDefaultGatewayFallback()
}

// getDefaultGatewayFallback uses UDP connection heuristic
func getDefaultGatewayFallback() (net.IP, error) {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	ip := localAddr.IP.To4()
	if ip == nil {
		return nil, ErrNATPMPNotFound
	}

	// Assume gateway is .1 on the same subnet
	gateway := make(net.IP, 4)
	copy(gateway, ip)
	gateway[3] = 1

	return gateway, nil
}
