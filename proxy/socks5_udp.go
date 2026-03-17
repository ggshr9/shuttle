package proxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	cmdUDPAssociate = 0x03

	// udpBufSize is the maximum size for a single UDP datagram read from the client.
	udpBufSize = 65535
	// udpIdleTimeout is how long to wait for a response on a UDP stream before cleanup.
	udpIdleTimeout = 5 * time.Second
	// udpAssociateReadTimeout is the max time to wait for the first datagram.
	udpAssociateReadTimeout = 120 * time.Second
)

// udpStreamEntry tracks a transport stream opened for a specific UDP target.
type udpStreamEntry struct {
	stream io.ReadWriteCloser
	mu     sync.Mutex
}

// handleUDPAssociate implements SOCKS5 UDP ASSOCIATE (CMD 0x03).
// It binds a local UDP socket, replies to the client with the bound address,
// then relays datagrams between the client and remote targets via transport streams.
func (s *SOCKS5Server) handleUDPAssociate(ctx context.Context, conn net.Conn) {
	// Bind a local UDP socket on a random port.
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		_ = s.sendReply(conn, repGeneralFailure, nil)
		s.logger.Debug("socks5 udp: resolve addr failed", "err", err)
		return
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		_ = s.sendReply(conn, repGeneralFailure, nil)
		s.logger.Debug("socks5 udp: listen failed", "err", err)
		return
	}
	defer udpConn.Close()

	// Reply with the bound UDP address.
	boundAddr := udpConn.LocalAddr().(*net.UDPAddr)
	_ = s.sendUDPReply(conn, repSuccess, boundAddr)
	s.logger.Debug("socks5 udp associate bound", "addr", boundAddr)

	// Monitor the TCP control connection — when it closes, we clean up everything.
	tcpClosed := make(chan struct{})
	go func() {
		defer close(tcpClosed)
		buf := make([]byte, 1)
		for {
			_, err := conn.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Track open streams per target address for reuse.
	var streamsMu sync.Mutex
	streams := make(map[string]*udpStreamEntry)
	defer func() {
		streamsMu.Lock()
		for _, se := range streams {
			se.stream.Close()
		}
		streamsMu.Unlock()
	}()

	// clientAddr tracks the UDP client address (first datagram sender).
	var clientAddr *net.UDPAddr

	buf := make([]byte, udpBufSize)
	for {
		select {
		case <-tcpClosed:
			return
		case <-ctx.Done():
			return
		default:
		}

		// Set a read deadline so we can check for TCP close periodically.
		_ = udpConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, raddr, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			s.logger.Debug("socks5 udp: read error", "err", err)
			return
		}

		if n < 4 {
			s.logger.Debug("socks5 udp: datagram too short", "len", n)
			continue
		}

		// Remember client address from first datagram.
		if clientAddr == nil {
			clientAddr = raddr
		}

		// Parse SOCKS5 UDP request header: RSV(2) + FRAG(1) + ATYP(1) + ADDR + PORT + DATA
		// RSV must be 0x0000
		frag := buf[2]
		if frag != 0 {
			s.logger.Debug("socks5 udp: dropping fragmented datagram", "frag", frag)
			continue
		}

		target, headerLen, err := parseSOCKS5UDPHeader(buf[:n])
		if err != nil {
			s.logger.Debug("socks5 udp: parse header failed", "err", err)
			continue
		}

		payload := buf[headerLen:n]

		// Get or create a stream for this target.
		streamsMu.Lock()
		se, ok := streams[target]
		if !ok {
			// Open a new transport stream for this target.
			remote, dialErr := s.dialer(ctx, "udp", target)
			if dialErr != nil {
				streamsMu.Unlock()
				s.logger.Debug("socks5 udp: dial failed", "target", target, "err", dialErr)
				continue
			}
			se = &udpStreamEntry{stream: remote}
			streams[target] = se

			// Start a goroutine to read responses from this stream and send back to client.
			go func(target string, se *udpStreamEntry, clientAddr *net.UDPAddr) {
				defer func() {
					streamsMu.Lock()
					delete(streams, target)
					streamsMu.Unlock()
					se.stream.Close()
				}()
				s.udpStreamReader(udpConn, se, target, clientAddr)
			}(target, se, clientAddr)
		}
		streamsMu.Unlock()

		// Write the payload as a UDP frame to the transport stream.
		se.mu.Lock()
		err = WriteUDPFrame(se.stream, target, payload)
		se.mu.Unlock()
		if err != nil {
			s.logger.Debug("socks5 udp: write frame failed", "target", target, "err", err)
		}
	}
}

// udpStreamReader reads UDP frames from a transport stream and sends them back
// to the SOCKS5 UDP client as SOCKS5 UDP response datagrams.
func (s *SOCKS5Server) udpStreamReader(udpConn *net.UDPConn, se *udpStreamEntry, target string, clientAddr *net.UDPAddr) {
	for {
		addr, payload, err := ReadUDPFrame(se.stream)
		if err != nil {
			if err != io.EOF {
				s.logger.Debug("socks5 udp: read frame failed", "target", target, "err", err)
			}
			return
		}

		// Build SOCKS5 UDP response header: RSV(2) + FRAG(1) + ATYP + ADDR + PORT + DATA
		resp, err := buildSOCKS5UDPResponse(addr, payload)
		if err != nil {
			s.logger.Debug("socks5 udp: build response failed", "err", err)
			continue
		}

		if _, err := udpConn.WriteToUDP(resp, clientAddr); err != nil {
			s.logger.Debug("socks5 udp: write to client failed", "err", err)
			return
		}
	}
}

// sendUDPReply sends a SOCKS5 reply with a UDP address (supports IPv4/IPv6).
func (s *SOCKS5Server) sendUDPReply(conn net.Conn, rep byte, addr *net.UDPAddr) error {
	if addr == nil {
		return s.sendReply(conn, rep, nil)
	}
	ip4 := addr.IP.To4()
	if ip4 != nil {
		reply := []byte{socks5Version, rep, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0}
		copy(reply[4:8], ip4)
		binary.BigEndian.PutUint16(reply[8:10], uint16(addr.Port))
		_, err := conn.Write(reply)
		return err
	}
	ip6 := addr.IP.To16()
	reply := make([]byte, 4+16+2)
	reply[0] = socks5Version
	reply[1] = rep
	reply[2] = 0x00
	reply[3] = atypIPv6
	copy(reply[4:20], ip6)
	binary.BigEndian.PutUint16(reply[20:22], uint16(addr.Port))
	_, err := conn.Write(reply)
	return err
}

// parseSOCKS5UDPHeader parses a SOCKS5 UDP request header and returns the
// target address and the offset where the payload begins.
// Header format: RSV(2) + FRAG(1) + ATYP(1) + ADDR(variable) + PORT(2)
func parseSOCKS5UDPHeader(data []byte) (addr string, headerLen int, err error) {
	if len(data) < 4 {
		return "", 0, fmt.Errorf("data too short")
	}

	// Skip RSV(2) + FRAG(1), start at ATYP (index 3)
	atyp := data[3]
	var host string
	var addrEnd int

	switch atyp {
	case atypIPv4:
		if len(data) < 4+4+2 {
			return "", 0, fmt.Errorf("data too short for IPv4")
		}
		host = net.IP(data[4:8]).String()
		addrEnd = 8
	case atypDomain:
		if len(data) < 5 {
			return "", 0, fmt.Errorf("data too short for domain length")
		}
		dLen := int(data[4])
		if len(data) < 5+dLen+2 {
			return "", 0, fmt.Errorf("data too short for domain")
		}
		host = string(data[5 : 5+dLen])
		addrEnd = 5 + dLen
	case atypIPv6:
		if len(data) < 4+16+2 {
			return "", 0, fmt.Errorf("data too short for IPv6")
		}
		host = net.IP(data[4:20]).String()
		addrEnd = 20
	default:
		return "", 0, fmt.Errorf("unsupported address type: %d", atyp)
	}

	if len(data) < addrEnd+2 {
		return "", 0, fmt.Errorf("data too short for port")
	}
	port := binary.BigEndian.Uint16(data[addrEnd : addrEnd+2])
	headerLen = addrEnd + 2

	return net.JoinHostPort(host, fmt.Sprintf("%d", port)), headerLen, nil
}

// buildSOCKS5UDPResponse builds a SOCKS5 UDP response datagram from an
// address and payload.
func buildSOCKS5UDPResponse(addr string, payload []byte) ([]byte, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid addr %q: %w", addr, err)
	}
	port, err := net.LookupPort("udp", portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	// RSV(2) + FRAG(1) + ATYP(1) + ADDR + PORT(2) + DATA
	var header []byte
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			header = make([]byte, 4+4+2)
			header[3] = atypIPv4
			copy(header[4:8], ip4)
			binary.BigEndian.PutUint16(header[8:10], uint16(port))
		} else {
			ip6 := ip.To16()
			header = make([]byte, 4+16+2)
			header[3] = atypIPv6
			copy(header[4:20], ip6)
			binary.BigEndian.PutUint16(header[20:22], uint16(port))
		}
	} else {
		// Domain
		header = make([]byte, 4+1+len(host)+2)
		header[3] = atypDomain
		header[4] = byte(len(host))
		copy(header[5:5+len(host)], host)
		binary.BigEndian.PutUint16(header[5+len(host):7+len(host)], uint16(port))
	}

	// RSV = 0x0000, FRAG = 0x00 (already zero from make)
	result := make([]byte, len(header)+len(payload))
	copy(result, header)
	copy(result[len(header):], payload)
	return result, nil
}

