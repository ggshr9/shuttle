//go:build windows

package p2p

import (
	"bufio"
	"bytes"
	"net"
	"os/exec"
	"strings"
)

// getDefaultGateway finds the default gateway IP on Windows
func getDefaultGateway() (net.IP, error) {
	// Use route print to get default gateway
	cmd := exec.Command("route", "print", "0.0.0.0")
	output, err := cmd.Output()
	if err != nil {
		return getDefaultGatewayFallback()
	}

	// Parse output looking for default route
	// Format: Network Destination    Netmask          Gateway       Interface  Metric
	//         0.0.0.0          0.0.0.0      192.168.1.1    192.168.1.100    25
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "0.0.0.0") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				ip := net.ParseIP(parts[2])
				if ip != nil && ip.To4() != nil && !ip.Equal(net.IPv4zero) {
					return ip.To4(), nil
				}
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
