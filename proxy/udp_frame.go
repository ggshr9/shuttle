package proxy

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

const (
	// UDPStreamPrefix is the protocol marker prepended to the stream header
	// to signal UDP relay mode (e.g., "UDP:host:port\n").
	UDPStreamPrefix = "UDP:"

	// maxUDPPayload is the maximum UDP datagram payload size.
	maxUDPPayload = 65507
	// udpFrameHeaderMin is the minimum frame header size: 2 (length) + 1 (atyp) + 4 (ipv4) + 2 (port).
	udpFrameHeaderMin = 9
)

// WriteUDPFrame writes a length-prefixed UDP frame to w.
// Frame format: [2-byte big-endian length][1-byte ATYP][variable ADDR][2-byte PORT][payload]
// The length field covers everything after itself (ATYP + ADDR + PORT + payload).
func WriteUDPFrame(w io.Writer, addr string, payload []byte) error {
	if len(payload) > maxUDPPayload {
		return fmt.Errorf("udp frame: payload too large (%d > %d)", len(payload), maxUDPPayload)
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("udp frame: invalid addr %q: %w", addr, err)
	}
	port, err := net.LookupPort("udp", portStr)
	if err != nil {
		return fmt.Errorf("udp frame: invalid port %q: %w", portStr, err)
	}

	var addrBytes []byte
	var atyp byte

	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			atyp = atypIPv4
			addrBytes = ip4
		} else {
			atyp = atypIPv6
			addrBytes = ip.To16()
		}
	} else {
		// Domain name
		if len(host) > 255 {
			return fmt.Errorf("udp frame: domain too long (%d)", len(host))
		}
		atyp = atypDomain
		addrBytes = append([]byte{byte(len(host))}, []byte(host)...)
	}

	// Total frame body length: 1 (atyp) + len(addrBytes) + 2 (port) + len(payload)
	bodyLen := 1 + len(addrBytes) + 2 + len(payload)
	frame := make([]byte, 2+bodyLen)
	binary.BigEndian.PutUint16(frame[0:2], uint16(bodyLen))
	frame[2] = atyp
	copy(frame[3:3+len(addrBytes)], addrBytes)
	binary.BigEndian.PutUint16(frame[3+len(addrBytes):5+len(addrBytes)], uint16(port))
	copy(frame[5+len(addrBytes):], payload)

	_, err = w.Write(frame)
	return err
}

// ReadUDPFrame reads a length-prefixed UDP frame from r.
// Returns the target address as "host:port" and the payload.
func ReadUDPFrame(r io.Reader) (addr string, payload []byte, err error) {
	// Read length prefix
	var lenBuf [2]byte
	if _, err = io.ReadFull(r, lenBuf[:]); err != nil {
		return "", nil, fmt.Errorf("udp frame: read length: %w", err)
	}
	bodyLen := int(binary.BigEndian.Uint16(lenBuf[:]))
	if bodyLen < 1+4+2 { // minimum: atyp(1) + ipv4(4) + port(2)
		return "", nil, fmt.Errorf("udp frame: body too short (%d)", bodyLen)
	}
	if bodyLen > maxUDPPayload+1+256+2 { // generous upper bound
		return "", nil, fmt.Errorf("udp frame: body too large (%d)", bodyLen)
	}

	body := make([]byte, bodyLen)
	if _, err = io.ReadFull(r, body); err != nil {
		return "", nil, fmt.Errorf("udp frame: read body: %w", err)
	}

	atyp := body[0]
	var host string
	var addrEnd int

	switch atyp {
	case atypIPv4:
		if len(body) < 1+4+2 {
			return "", nil, fmt.Errorf("udp frame: body too short for IPv4")
		}
		host = net.IP(body[1:5]).String()
		addrEnd = 5
	case atypDomain:
		if len(body) < 2 {
			return "", nil, fmt.Errorf("udp frame: body too short for domain length")
		}
		dLen := int(body[1])
		if len(body) < 2+dLen+2 {
			return "", nil, fmt.Errorf("udp frame: body too short for domain")
		}
		host = string(body[2 : 2+dLen])
		addrEnd = 2 + dLen
	case atypIPv6:
		if len(body) < 1+16+2 {
			return "", nil, fmt.Errorf("udp frame: body too short for IPv6")
		}
		host = net.IP(body[1:17]).String()
		addrEnd = 17
	default:
		return "", nil, fmt.Errorf("udp frame: unsupported address type: %d", atyp)
	}

	if len(body) < addrEnd+2 {
		return "", nil, fmt.Errorf("udp frame: body too short for port")
	}
	port := binary.BigEndian.Uint16(body[addrEnd : addrEnd+2])
	payload = body[addrEnd+2:]

	return net.JoinHostPort(host, fmt.Sprintf("%d", port)), payload, nil
}
