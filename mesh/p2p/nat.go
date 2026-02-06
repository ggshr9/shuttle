package p2p

import (
	"context"
	"fmt"
	"net"
	"time"
)

// NATType represents the detected NAT type.
type NATType int

const (
	NATUnknown NATType = iota
	NATNone            // No NAT (public IP)
	NATFullCone        // Full cone NAT (easiest to traverse)
	NATRestrictedCone  // Restricted cone NAT
	NATPortRestricted  // Port-restricted cone NAT
	NATSymmetric       // Symmetric NAT (hardest to traverse)
)

func (n NATType) String() string {
	switch n {
	case NATNone:
		return "No NAT (Public IP)"
	case NATFullCone:
		return "Full Cone NAT"
	case NATRestrictedCone:
		return "Restricted Cone NAT"
	case NATPortRestricted:
		return "Port-Restricted Cone NAT"
	case NATSymmetric:
		return "Symmetric NAT"
	default:
		return "Unknown"
	}
}

// CanHolePunch returns whether this NAT type supports UDP hole punching.
func (n NATType) CanHolePunch() bool {
	switch n {
	case NATNone, NATFullCone, NATRestrictedCone, NATPortRestricted:
		return true
	case NATSymmetric:
		// Symmetric NAT can sometimes work with port prediction
		return true
	default:
		return false
	}
}

// NATDetector detects the NAT type using STUN.
type NATDetector struct {
	stunClient *STUNClient
	timeout    time.Duration
}

// NewNATDetector creates a NAT detector.
func NewNATDetector(stunServers []string, timeout time.Duration) *NATDetector {
	return &NATDetector{
		stunClient: NewSTUNClient(stunServers, timeout),
		timeout:    timeout,
	}
}

// NATInfo contains detailed NAT detection results.
type NATInfo struct {
	Type          NATType
	PublicAddr    *net.UDPAddr
	LocalAddr     *net.UDPAddr
	PortPreserved bool // Whether the NAT preserves source port
	HairpinSupport bool // Whether the NAT supports hairpin
}

// Detect performs NAT detection.
func (d *NATDetector) Detect() (*NATInfo, error) {
	info := &NATInfo{
		Type: NATUnknown,
	}

	// Create a UDP connection for all tests
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, fmt.Errorf("nat: listen: %w", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	info.LocalAddr = localAddr

	// First STUN query - get public address
	result1, err := d.stunClient.Query(conn)
	if err != nil {
		return nil, fmt.Errorf("nat: first stun query: %w", err)
	}
	info.PublicAddr = result1.PublicAddr

	// Check if we have a public IP (no NAT)
	if result1.PublicAddr.IP.Equal(localAddr.IP) {
		info.Type = NATNone
		info.PortPreserved = true
		return info, nil
	}

	// Check port preservation
	info.PortPreserved = result1.PublicAddr.Port == localAddr.Port

	// Second STUN query to a different server to check for symmetric NAT
	if len(d.stunClient.servers) > 1 {
		// Create second query to different server
		secondClient := NewSTUNClient(d.stunClient.servers[1:], d.timeout)
		result2, err := secondClient.Query(conn)
		if err == nil {
			// If we get different public addresses from different servers,
			// it's symmetric NAT
			if !result1.PublicAddr.IP.Equal(result2.PublicAddr.IP) ||
				result1.PublicAddr.Port != result2.PublicAddr.Port {
				info.Type = NATSymmetric
				return info, nil
			}
		}
	}

	// At this point, we know it's some form of cone NAT
	// Full detection requires more sophisticated tests with CHANGE-REQUEST
	// which many STUN servers don't support. We'll use a simplified approach:
	// - If port is preserved: likely Full Cone or Restricted Cone
	// - If port is not preserved: likely Port-Restricted Cone

	if info.PortPreserved {
		// Conservative assumption: Port-Restricted Cone
		// Full Cone would require testing if external hosts can reach us
		info.Type = NATPortRestricted
	} else {
		// Port not preserved - Port-Restricted Cone
		info.Type = NATPortRestricted
	}

	return info, nil
}

// DetectWithMultipleSockets performs NAT detection using multiple sockets
// to better detect symmetric NAT.
func (d *NATDetector) DetectWithMultipleSockets() (*NATInfo, error) {
	info := &NATInfo{
		Type: NATUnknown,
	}

	// First socket
	conn1, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, fmt.Errorf("nat: listen 1: %w", err)
	}
	defer conn1.Close()

	result1, err := d.stunClient.Query(conn1)
	if err != nil {
		return nil, fmt.Errorf("nat: stun query 1: %w", err)
	}

	info.PublicAddr = result1.PublicAddr
	info.LocalAddr = result1.LocalAddr

	// Check for public IP
	if result1.PublicAddr.IP.Equal(result1.LocalAddr.IP) {
		info.Type = NATNone
		info.PortPreserved = true
		return info, nil
	}

	info.PortPreserved = result1.PublicAddr.Port == result1.LocalAddr.Port

	// Second socket to same STUN server
	conn2, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, fmt.Errorf("nat: listen 2: %w", err)
	}
	defer conn2.Close()

	result2, err := d.stunClient.Query(conn2)
	if err != nil {
		return nil, fmt.Errorf("nat: stun query 2: %w", err)
	}

	// Check if NAT assigns different public ports for different local ports
	// to the same destination
	portDelta1 := result1.PublicAddr.Port - result1.LocalAddr.Port
	portDelta2 := result2.PublicAddr.Port - result2.LocalAddr.Port

	if portDelta1 != portDelta2 {
		// NAT uses different port mapping strategies - likely symmetric
		info.Type = NATSymmetric
	} else if !info.PortPreserved {
		info.Type = NATPortRestricted
	} else {
		// Conservative: assume Port-Restricted until proven otherwise
		info.Type = NATPortRestricted
	}

	return info, nil
}

// QuickDetect performs a quick NAT detection (just gets public address).
func (d *NATDetector) QuickDetect() (*net.UDPAddr, error) {
	result, err := d.stunClient.Query(nil)
	if err != nil {
		return nil, err
	}
	return result.PublicAddr, nil
}

// GetPublicEndpoint returns the public endpoint for a given UDP connection.
func GetPublicEndpoint(conn *net.UDPConn, stunServers []string) (*net.UDPAddr, error) {
	client := NewSTUNClient(stunServers, 3*time.Second)
	result, err := client.Query(conn)
	if err != nil {
		return nil, err
	}
	return result.PublicAddr, nil
}

// DetectParallel performs NAT detection using parallel STUN queries.
// This is faster and more reliable than sequential detection.
func (d *NATDetector) DetectParallel(ctx context.Context) (*NATInfo, error) {
	info := &NATInfo{
		Type: NATUnknown,
	}

	// Create a UDP connection for all tests
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, fmt.Errorf("nat: listen: %w", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	info.LocalAddr = localAddr

	// Query all servers in parallel using the same connection
	results, err := d.stunClient.QueryParallelWithConn(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("nat: parallel stun query: %w", err)
	}

	if len(results) == 0 {
		return nil, ErrSTUNNoResponse
	}

	// Use first result as primary
	info.PublicAddr = results[0].PublicAddr

	// Check if we have a public IP (no NAT)
	if results[0].PublicAddr.IP.Equal(localAddr.IP) {
		info.Type = NATNone
		info.PortPreserved = true
		return info, nil
	}

	// Check port preservation
	info.PortPreserved = results[0].PublicAddr.Port == localAddr.Port

	// Check for symmetric NAT by comparing results from different servers
	if len(results) > 1 {
		for i := 1; i < len(results); i++ {
			// If we get different public addresses from different servers,
			// it's symmetric NAT
			if !results[0].PublicAddr.IP.Equal(results[i].PublicAddr.IP) ||
				results[0].PublicAddr.Port != results[i].PublicAddr.Port {
				info.Type = NATSymmetric
				return info, nil
			}
		}
	}

	// At this point, we know it's some form of cone NAT
	if info.PortPreserved {
		info.Type = NATPortRestricted
	} else {
		info.Type = NATPortRestricted
	}

	return info, nil
}

// QuickDetectParallel performs fast parallel NAT detection.
func (d *NATDetector) QuickDetectParallel(ctx context.Context) (*net.UDPAddr, error) {
	result, err := d.stunClient.QueryParallel(ctx)
	if err != nil {
		return nil, err
	}
	return result.PublicAddr, nil
}

// GetPublicEndpointParallel returns the public endpoint using parallel STUN queries.
func GetPublicEndpointParallel(ctx context.Context, conn *net.UDPConn, stunServers []string) (*net.UDPAddr, error) {
	client := NewSTUNClient(stunServers, 3*time.Second)

	if conn != nil {
		// Use the provided connection
		results, err := client.QueryParallelWithConn(ctx, conn)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			return results[0].PublicAddr, nil
		}
		return nil, ErrSTUNNoResponse
	}

	// Create new connections for parallel query
	result, err := client.QueryParallel(ctx)
	if err != nil {
		return nil, err
	}
	return result.PublicAddr, nil
}
