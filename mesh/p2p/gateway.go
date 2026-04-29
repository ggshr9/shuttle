package p2p

import "net"

// getDefaultGatewayFallback uses a UDP-connection heuristic: open a
// non-blocking UDP "connection" to a public address, take the kernel-
// chosen local address as our outbound IP, and assume the gateway is
// `.1` on the same /24 subnet. Works without privilege and without
// parsing OS-specific routing-table tools, but only correct for the
// common single-NIC, /24-with-router-at-.1 topology — which is why
// each per-OS getDefaultGateway tries the OS-native path first and
// falls back to this only when that fails.
//
// Hoisted out of gateway_unix.go and gateway_windows.go where the
// same function body was duplicated byte-for-byte.
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

	gateway := make(net.IP, 4)
	copy(gateway, ip)
	gateway[3] = 1
	return gateway, nil
}
