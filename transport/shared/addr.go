package shared

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
)

const (
	AddrTypeIPv4   = 0x01
	AddrTypeDomain = 0x03
	AddrTypeIPv6   = 0x04
	CmdConnect     = 0x01
	CmdUDPAssociate = 0x03
)

// EncodeAddr writes a SOCKS5-style address to w.
// Format: [atype(1)][addr][port(2 big-endian)]
// Domain: [0x03][len(1)][domain_bytes][port(2)]
// IPv4:   [0x01][4 bytes][port(2)]
// IPv6:   [0x04][16 bytes][port(2)]
func EncodeAddr(w io.Writer, network, address string) error {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("shared/addr: split host:port: %w", err)
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return fmt.Errorf("shared/addr: parse port: %w", err)
	}

	portBytes := [2]byte{}
	binary.BigEndian.PutUint16(portBytes[:], uint16(port))

	ip := net.ParseIP(host)
	if ip == nil {
		// Domain name
		domain := []byte(host)
		if len(domain) > 255 {
			return fmt.Errorf("shared/addr: domain name too long (%d bytes)", len(domain))
		}
		buf := make([]byte, 2+len(domain))
		buf[0] = AddrTypeDomain
		buf[1] = byte(len(domain))
		copy(buf[2:], domain)
		if _, err := w.Write(buf); err != nil {
			return err
		}
	} else if v4 := ip.To4(); v4 != nil {
		buf := make([]byte, 5)
		buf[0] = AddrTypeIPv4
		copy(buf[1:], v4)
		if _, err := w.Write(buf); err != nil {
			return err
		}
	} else {
		v6 := ip.To16()
		buf := make([]byte, 17)
		buf[0] = AddrTypeIPv6
		copy(buf[1:], v6)
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}

	_, err = w.Write(portBytes[:])
	return err
}

// DecodeAddr reads a SOCKS5-style address from r.
// Returns network ("tcp" or "udp") and address ("host:port").
// IPv6 addresses are wrapped in brackets: [::1]:8080.
func DecodeAddr(r io.Reader) (network string, address string, err error) {
	atype := [1]byte{}
	if _, err = io.ReadFull(r, atype[:]); err != nil {
		return "", "", fmt.Errorf("shared/addr: read atype: %w", err)
	}

	var host string
	switch atype[0] {
	case AddrTypeIPv4:
		buf := make([]byte, 4)
		if _, err = io.ReadFull(r, buf); err != nil {
			return "", "", fmt.Errorf("shared/addr: read ipv4: %w", err)
		}
		host = net.IP(buf).String()

	case AddrTypeDomain:
		lenBuf := [1]byte{}
		if _, err = io.ReadFull(r, lenBuf[:]); err != nil {
			return "", "", fmt.Errorf("shared/addr: read domain length: %w", err)
		}
		domainBuf := make([]byte, lenBuf[0])
		if _, err = io.ReadFull(r, domainBuf); err != nil {
			return "", "", fmt.Errorf("shared/addr: read domain: %w", err)
		}
		host = string(domainBuf)

	case AddrTypeIPv6:
		buf := make([]byte, 16)
		if _, err = io.ReadFull(r, buf); err != nil {
			return "", "", fmt.Errorf("shared/addr: read ipv6: %w", err)
		}
		host = "[" + net.IP(buf).String() + "]"

	default:
		return "", "", fmt.Errorf("shared/addr: unknown address type 0x%02x", atype[0])
	}

	portBuf := [2]byte{}
	if _, err = io.ReadFull(r, portBuf[:]); err != nil {
		return "", "", fmt.Errorf("shared/addr: read port: %w", err)
	}
	port := binary.BigEndian.Uint16(portBuf[:])

	address = host + ":" + strconv.FormatUint(uint64(port), 10)
	return "tcp", address, nil
}
