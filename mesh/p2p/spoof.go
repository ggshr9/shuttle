package p2p

import (
	"fmt"
	"net"
	"runtime"
)

// Common spoof ports that are usually allowed through firewalls
const (
	PortDNS      = 53   // DNS - almost never blocked
	PortHTTPS    = 443  // HTTPS/QUIC - common for UDP
	PortIKE      = 500  // IKE VPN - often allowed
	PortIPSecNAT = 4500 // IPSec NAT-T - often allowed
	PortSTUN     = 3478 // STUN - sometimes allowed
)

// SpoofMode defines the port spoofing strategy
type SpoofMode int

const (
	SpoofNone     SpoofMode = iota // No spoofing, use random port
	SpoofDNS                       // Use port 53 (DNS)
	SpoofHTTPS                     // Use port 443 (HTTPS/QUIC)
	SpoofIKE                       // Use port 500 (IKE)
	SpoofIPSecNAT                  // Use port 4500 (IPSec NAT-T)
	SpoofCustom                    // Use custom port
)

func (m SpoofMode) String() string {
	switch m {
	case SpoofNone:
		return "none"
	case SpoofDNS:
		return "dns"
	case SpoofHTTPS:
		return "https"
	case SpoofIKE:
		return "ike"
	case SpoofIPSecNAT:
		return "ipsec-nat"
	case SpoofCustom:
		return "custom"
	default:
		return "unknown"
	}
}

// ParseSpoofMode parses a string into SpoofMode
func ParseSpoofMode(s string) SpoofMode {
	switch s {
	case "dns", "53":
		return SpoofDNS
	case "https", "443":
		return SpoofHTTPS
	case "ike", "500":
		return SpoofIKE
	case "ipsec-nat", "4500":
		return SpoofIPSecNAT
	case "none", "":
		return SpoofNone
	default:
		return SpoofCustom
	}
}

// SpoofConfig holds port spoofing configuration
type SpoofConfig struct {
	Mode       SpoofMode // Spoofing mode
	CustomPort int       // Custom port when Mode is SpoofCustom
}

// GetPort returns the port to use based on the spoof configuration
func (c *SpoofConfig) GetPort() int {
	switch c.Mode {
	case SpoofDNS:
		return PortDNS
	case SpoofHTTPS:
		return PortHTTPS
	case SpoofIKE:
		return PortIKE
	case SpoofIPSecNAT:
		return PortIPSecNAT
	case SpoofCustom:
		if c.CustomPort > 0 && c.CustomPort <= 65535 {
			return c.CustomPort
		}
		return 0
	default:
		return 0 // Random port
	}
}

// CreateSpoofedConn creates a UDP connection with the spoofed port
func CreateSpoofedConn(cfg *SpoofConfig) (*net.UDPConn, error) {
	port := 0
	if cfg != nil {
		port = cfg.GetPort()
	}

	addr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: port,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		// If binding to privileged port fails, try with SO_REUSEADDR on Unix
		if port > 0 && port < 1024 {
			return nil, fmt.Errorf("spoof: bind to port %d failed (may need root/admin): %w", port, err)
		}
		return nil, fmt.Errorf("spoof: listen: %w", err)
	}

	return conn, nil
}

// CanBindPort checks if we can bind to a specific port
func CanBindPort(port int) bool {
	addr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: port,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// RequiresPrivilege returns whether the spoof mode requires elevated privileges
func (c *SpoofConfig) RequiresPrivilege() bool {
	port := c.GetPort()
	// Ports below 1024 require root/admin on most systems
	return port > 0 && port < 1024
}

// GetRecommendedMode returns a recommended spoof mode based on system capabilities
func GetRecommendedMode() SpoofMode {
	// Try DNS port first (most effective)
	if CanBindPort(PortDNS) {
		return SpoofDNS
	}

	// Try HTTPS port (common for QUIC)
	if CanBindPort(PortHTTPS) {
		return SpoofHTTPS
	}

	// Try IKE port
	if CanBindPort(PortIKE) {
		return SpoofIKE
	}

	// No privileged ports available
	return SpoofNone
}

// IsPrivileged returns whether the current process has elevated privileges
func IsPrivileged() bool {
	switch runtime.GOOS {
	case "windows":
		// On Windows, try to bind to port 53 as a simple check
		return CanBindPort(PortDNS)
	default:
		// On Unix-like systems, check effective UID
		// Note: This is a simplified check
		return CanBindPort(PortDNS)
	}
}

// SpoofInfo contains information about the spoofed connection
type SpoofInfo struct {
	Mode       SpoofMode
	LocalPort  int
	LocalAddr  *net.UDPAddr
	Privileged bool
}

// GetSpoofInfo returns information about a spoofed connection
func GetSpoofInfo(conn *net.UDPConn, mode SpoofMode) *SpoofInfo {
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return &SpoofInfo{
		Mode:       mode,
		LocalPort:  localAddr.Port,
		LocalAddr:  localAddr,
		Privileged: localAddr.Port > 0 && localAddr.Port < 1024,
	}
}

// ValidateSpoofConfig validates the spoof configuration
func ValidateSpoofConfig(cfg *SpoofConfig) error {
	if cfg == nil {
		return nil
	}

	port := cfg.GetPort()
	if port == 0 {
		return nil // Random port, always valid
	}

	if port < 0 || port > 65535 {
		return fmt.Errorf("spoof: invalid port %d", port)
	}

	// Check if we can bind to the port
	if !CanBindPort(port) {
		if port < 1024 {
			return fmt.Errorf("spoof: cannot bind to privileged port %d (need root/admin)", port)
		}
		return fmt.Errorf("spoof: cannot bind to port %d (may be in use)", port)
	}

	return nil
}
