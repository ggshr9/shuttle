package p2p

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Hole punch packet magic
var HolePunchMagic = []byte{'H', 'O', 'L', 'E'}

// Hole punch packet types
const (
	HolePunchRequest byte = 0x01
	HolePunchResponse byte = 0x02
	HolePunchAck byte = 0x03
)

// HolePunchPacket represents a UDP hole punch packet.
// Format: [4B "HOLE"][1B type][4B srcVIP][4B dstVIP][8B timestamp][4B seq]
const HolePunchPacketSize = 25

type HolePunchPacket struct {
	Type      byte
	SrcVIP    net.IP
	DstVIP    net.IP
	Timestamp int64
	Seq       uint32
}

// Encode encodes the hole punch packet.
func (p *HolePunchPacket) Encode() []byte {
	buf := make([]byte, HolePunchPacketSize)
	copy(buf[0:4], HolePunchMagic)
	buf[4] = p.Type
	copy(buf[5:9], p.SrcVIP.To4())
	copy(buf[9:13], p.DstVIP.To4())
	binary.BigEndian.PutUint64(buf[13:21], uint64(p.Timestamp)) //nolint:gosec // G115: Unix timestamp, non-negative in practice
	binary.BigEndian.PutUint32(buf[21:25], p.Seq)
	return buf
}

// DecodeHolePunchPacket decodes a hole punch packet.
func DecodeHolePunchPacket(data []byte) (*HolePunchPacket, error) {
	if len(data) < HolePunchPacketSize {
		return nil, errors.New("holepunch: packet too short")
	}

	// Verify magic
	if data[0] != 'H' || data[1] != 'O' || data[2] != 'L' || data[3] != 'E' {
		return nil, errors.New("holepunch: invalid magic")
	}

	p := &HolePunchPacket{
		Type:      data[4],
		SrcVIP:    net.IP(make([]byte, 4)),
		DstVIP:    net.IP(make([]byte, 4)),
		Timestamp: int64(binary.BigEndian.Uint64(data[13:21])), //nolint:gosec // G115: timestamp round-trips through uint64 encoding
		Seq:       binary.BigEndian.Uint32(data[21:25]),
	}
	copy(p.SrcVIP, data[5:9])
	copy(p.DstVIP, data[9:13])

	return p, nil
}

// IsHolePunchPacket checks if the data starts with hole punch magic.
func IsHolePunchPacket(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	return data[0] == 'H' && data[1] == 'O' && data[2] == 'L' && data[3] == 'E'
}

// HolePuncher performs UDP hole punching.
type HolePuncher struct {
	conn       *net.UDPConn
	localVIP   net.IP
	timeout    time.Duration
	interval   time.Duration
	logger     *slog.Logger
}

// NewHolePuncher creates a new hole puncher.
func NewHolePuncher(conn *net.UDPConn, localVIP net.IP, timeout time.Duration, logger *slog.Logger) *HolePuncher {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &HolePuncher{
		conn:     conn,
		localVIP: localVIP,
		timeout:  timeout,
		interval: 100 * time.Millisecond,
		logger:   logger,
	}
}

// HolePunchResult contains the result of hole punching.
type HolePunchResult struct {
	RemoteAddr *net.UDPAddr // Actual remote address we can communicate with
	RTT        time.Duration
	Candidate  *Candidate   // Which candidate succeeded
}

// Punch attempts to punch a hole to the remote peer.
// It tries all candidate addresses simultaneously.
func (hp *HolePuncher) Punch(ctx context.Context, remoteVIP net.IP, candidates []*Candidate) (*HolePunchResult, error) {
	if len(candidates) == 0 {
		return nil, errors.New("holepunch: no candidates")
	}

	ctx, cancel := context.WithTimeout(ctx, hp.timeout)
	defer cancel()

	resultCh := make(chan *HolePunchResult, 1)
	errCh := make(chan error, 1)

	// Start receiver goroutine
	go hp.receiveLoop(ctx, remoteVIP, resultCh)

	// Start sender goroutines for each candidate
	var wg sync.WaitGroup
	for _, candidate := range candidates {
		wg.Add(1)
		go func(c *Candidate) {
			defer wg.Done()
			hp.sendLoop(ctx, remoteVIP, c)
		}(candidate)
	}

	// Wait for result or timeout
	select {
	case result := <-resultCh:
		cancel() // Stop all senders
		return result, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("holepunch: timeout after %v", hp.timeout)
	case err := <-errCh:
		return nil, err
	}
}

// sendLoop sends hole punch packets to a candidate.
func (hp *HolePuncher) sendLoop(ctx context.Context, remoteVIP net.IP, candidate *Candidate) {
	seq := uint32(0)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		pkt := &HolePunchPacket{
			Type:      HolePunchRequest,
			SrcVIP:    hp.localVIP,
			DstVIP:    remoteVIP,
			Timestamp: time.Now().UnixNano(),
			Seq:       seq,
		}

		data := pkt.Encode()
		_, err := hp.conn.WriteToUDP(data, candidate.Addr)
		if err != nil {
			hp.logger.Debug("holepunch: send failed",
				"addr", candidate.Addr,
				"err", err)
		} else {
			hp.logger.Debug("holepunch: sent request",
				"addr", candidate.Addr,
				"seq", seq)
		}

		seq++
		time.Sleep(hp.interval)
	}
}

// receiveLoop receives hole punch responses.
func (hp *HolePuncher) receiveLoop(ctx context.Context, remoteVIP net.IP, resultCh chan<- *HolePunchResult) {
	buf := make([]byte, 1500)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_ = hp.conn.SetReadDeadline(time.Now().Add(hp.interval * 2))
		n, addr, err := hp.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			hp.logger.Debug("holepunch: receive error", "err", err)
			continue
		}

		// Check if it's a hole punch packet
		if !IsHolePunchPacket(buf[:n]) {
			continue
		}

		pkt, err := DecodeHolePunchPacket(buf[:n])
		if err != nil {
			continue
		}

		// Verify it's from the expected peer
		if !pkt.SrcVIP.Equal(remoteVIP) {
			continue
		}

		hp.logger.Debug("holepunch: received",
			"type", pkt.Type,
			"from", addr,
			"seq", pkt.Seq)

		switch pkt.Type {
		case HolePunchRequest:
			// Send response
			resp := &HolePunchPacket{
				Type:      HolePunchResponse,
				SrcVIP:    hp.localVIP,
				DstVIP:    remoteVIP,
				Timestamp: pkt.Timestamp,
				Seq:       pkt.Seq,
			}
			_, _ = hp.conn.WriteToUDP(resp.Encode(), addr)

		case HolePunchResponse:
			// Calculate RTT
			rtt := time.Duration(time.Now().UnixNano() - pkt.Timestamp)

			// Send ACK
			ack := &HolePunchPacket{
				Type:      HolePunchAck,
				SrcVIP:    hp.localVIP,
				DstVIP:    remoteVIP,
				Timestamp: time.Now().UnixNano(),
				Seq:       pkt.Seq,
			}
			_, _ = hp.conn.WriteToUDP(ack.Encode(), addr)

			// Report success
			result := &HolePunchResult{
				RemoteAddr: addr,
				RTT:        rtt,
			}

			select {
			case resultCh <- result:
			default:
			}
			return

		case HolePunchAck:
			// ACK received - connection confirmed
			result := &HolePunchResult{
				RemoteAddr: addr,
				RTT:        0,
			}

			select {
			case resultCh <- result:
			default:
			}
			return
		}
	}
}

// SimultaneousPunch performs simultaneous hole punching.
// Both peers call this at approximately the same time.
func (hp *HolePuncher) SimultaneousPunch(ctx context.Context, remoteVIP net.IP, candidates []*Candidate) (*HolePunchResult, error) {
	return hp.Punch(ctx, remoteVIP, candidates)
}

// PunchWithRetry performs hole punching with retry logic.
func (hp *HolePuncher) PunchWithRetry(ctx context.Context, remoteVIP net.IP, candidates []*Candidate, maxRetries int) (*HolePunchResult, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		result, err := hp.Punch(ctx, remoteVIP, candidates)
		if err == nil {
			return result, nil
		}

		lastErr = err
		hp.logger.Debug("holepunch: retry",
			"attempt", i+1,
			"err", err)

		// Wait before retry
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}

	return nil, fmt.Errorf("holepunch: failed after %d retries: %w", maxRetries, lastErr)
}

// CandidatesToAddrs converts candidates to UDP addresses.
func CandidatesToAddrs(candidates []*Candidate) []*net.UDPAddr {
	addrs := make([]*net.UDPAddr, len(candidates))
	for i, c := range candidates {
		addrs[i] = c.Addr
	}
	return addrs
}

// AddrsToCandidates converts UDP addresses to host candidates.
func AddrsToCandidates(addrs []*net.UDPAddr) []*Candidate {
	candidates := make([]*Candidate, len(addrs))
	for i, addr := range addrs {
		candidates[i] = NewCandidate(CandidateHost, addr)
	}
	return candidates
}
