// Package tuic implements the TUIC v5 protocol over QUIC with
// UUID+Token authentication and per-stream CONNECT headers.
package tuic

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"github.com/quic-go/quic-go"
)

// Protocol constants.
const (
	Version byte = 0x05

	CmdConnect    byte = 0x01
	CmdUDPAssoc   byte = 0x02
	CmdDissociate byte = 0x03
	CmdHeartbeat  byte = 0x04

	AddrIPv4   byte = 0x01
	AddrDomain byte = 0x03
	AddrIPv6   byte = 0x04

	UUIDLen  = 16
	TokenLen = 32
)

// AuthRequest holds the TUIC authentication datagram: UUID(16) + Token(32).
type AuthRequest struct {
	UUID  [UUIDLen]byte
	Token [TokenLen]byte
}

// EncodeAuth serialises the auth request into a 48-byte slice.
func (a *AuthRequest) Encode() []byte {
	buf := make([]byte, UUIDLen+TokenLen)
	copy(buf[:UUIDLen], a.UUID[:])
	copy(buf[UUIDLen:], a.Token[:])
	return buf
}

// DecodeAuth reads an AuthRequest from a 48-byte datagram.
func DecodeAuth(data []byte) (*AuthRequest, error) {
	if len(data) < UUIDLen+TokenLen {
		return nil, fmt.Errorf("tuic: auth datagram too short (%d bytes)", len(data))
	}
	a := &AuthRequest{}
	copy(a.UUID[:], data[:UUIDLen])
	copy(a.Token[:], data[UUIDLen:UUIDLen+TokenLen])
	return a, nil
}

// ComputeToken computes HMAC-SHA256(uuid, password) producing a 32-byte token.
func ComputeToken(uuid [UUIDLen]byte, password []byte) [TokenLen]byte {
	mac := hmac.New(sha256.New, password)
	mac.Write(uuid[:])
	var token [TokenLen]byte
	copy(token[:], mac.Sum(nil))
	return token
}

// ParseUUID parses a UUID string (with or without hyphens) into 16 bytes.
func ParseUUID(s string) ([UUIDLen]byte, error) {
	// Strip hyphens
	clean := make([]byte, 0, 32)
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			clean = append(clean, s[i])
		}
	}
	if len(clean) != 32 {
		return [UUIDLen]byte{}, fmt.Errorf("tuic: invalid UUID length %d (expected 32 hex chars)", len(clean))
	}

	var uuid [UUIDLen]byte
	for i := 0; i < 16; i++ {
		hi, ok1 := hexVal(clean[i*2])
		lo, ok2 := hexVal(clean[i*2+1])
		if !ok1 || !ok2 {
			return [UUIDLen]byte{}, fmt.Errorf("tuic: invalid hex in UUID at position %d", i*2)
		}
		uuid[i] = hi<<4 | lo
	}
	return uuid, nil
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	default:
		return 0, false
	}
}

// ConnectHeader is the per-stream TUIC v5 CONNECT header.
//
//	[1B version=0x05][1B cmd][1B addr-type][variable addr][2B port]
type ConnectHeader struct {
	Version byte
	Command byte
	Address string // host:port format for encoding; raw addr for decode
	Port    uint16
	// Decoded address fields
	AddrType byte
	AddrRaw  []byte // IP bytes or domain bytes (without length prefix)
}

// EncodeConnectHeader writes a TUIC v5 CONNECT header to w.
// address should be in "host:port" format.
func EncodeConnectHeader(w io.Writer, cmd byte, address string) error {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("tuic: invalid address %q: %w", address, err)
	}

	port, err := parsePort(portStr)
	if err != nil {
		return err
	}

	// Determine address type and encode
	var addrType byte
	var addrBytes []byte

	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			addrType = AddrIPv4
			addrBytes = ip4
		} else {
			addrType = AddrIPv6
			addrBytes = ip.To16()
		}
	} else {
		// Domain name
		addrType = AddrDomain
		if len(host) > 255 {
			return fmt.Errorf("tuic: domain name too long (%d bytes)", len(host))
		}
		addrBytes = append([]byte{byte(len(host))}, []byte(host)...)
	}

	// Header: version(1) + cmd(1) + addrType(1) + addr(variable) + port(2)
	buf := make([]byte, 3+len(addrBytes)+2)
	buf[0] = Version
	buf[1] = cmd
	buf[2] = addrType
	copy(buf[3:3+len(addrBytes)], addrBytes)
	binary.BigEndian.PutUint16(buf[3+len(addrBytes):], port)

	_, err = w.Write(buf)
	return err
}

// DecodeConnectHeader reads a TUIC v5 CONNECT header from r.
// Returns the command and the address in "host:port" format.
func DecodeConnectHeader(r io.Reader) (cmd byte, address string, err error) {
	// Read version + cmd + addrType (3 bytes)
	hdr := [3]byte{}
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return 0, "", fmt.Errorf("tuic: read header: %w", err)
	}

	if hdr[0] != Version {
		return 0, "", fmt.Errorf("tuic: unsupported version 0x%02x", hdr[0])
	}

	cmd = hdr[1]
	addrType := hdr[2]

	var host string
	switch addrType {
	case AddrIPv4:
		buf := [4]byte{}
		if _, err = io.ReadFull(r, buf[:]); err != nil {
			return 0, "", fmt.Errorf("tuic: read IPv4: %w", err)
		}
		host = net.IP(buf[:]).String()

	case AddrIPv6:
		buf := [16]byte{}
		if _, err = io.ReadFull(r, buf[:]); err != nil {
			return 0, "", fmt.Errorf("tuic: read IPv6: %w", err)
		}
		host = net.IP(buf[:]).String()

	case AddrDomain:
		lenBuf := [1]byte{}
		if _, err = io.ReadFull(r, lenBuf[:]); err != nil {
			return 0, "", fmt.Errorf("tuic: read domain length: %w", err)
		}
		domainBuf := make([]byte, lenBuf[0])
		if _, err = io.ReadFull(r, domainBuf); err != nil {
			return 0, "", fmt.Errorf("tuic: read domain: %w", err)
		}
		host = string(domainBuf)

	default:
		return 0, "", fmt.Errorf("tuic: unknown address type 0x%02x", addrType)
	}

	// Read port (2 bytes big-endian)
	portBuf := [2]byte{}
	if _, err = io.ReadFull(r, portBuf[:]); err != nil {
		return 0, "", fmt.Errorf("tuic: read port: %w", err)
	}
	port := binary.BigEndian.Uint16(portBuf[:])

	address = net.JoinHostPort(host, fmt.Sprintf("%d", port))
	return cmd, address, nil
}

func parsePort(s string) (uint16, error) {
	var port uint16
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("tuic: invalid port %q", s)
		}
		port = port*10 + uint16(c-'0')
	}
	return port, nil
}

// streamConn wraps a *quic.Stream as a net.Conn.
type streamConn struct {
	*quic.Stream
	local, remote net.Addr
}

func (c *streamConn) LocalAddr() net.Addr  { return c.local }
func (c *streamConn) RemoteAddr() net.Addr { return c.remote }
