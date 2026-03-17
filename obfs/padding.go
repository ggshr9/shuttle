package obfs

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
)

const (
	// DefaultPaddingMinTarget is the minimum padding target size.
	DefaultPaddingMinTarget = 1100
	// headerSize is the overhead: 2-byte frame length + 2-byte original data length.
	headerSize = 4
)

// Padder pads packets to a randomized size to eliminate packet-size fingerprinting.
//
// Wire format: [2-byte totalFrameLen][2-byte origDataLen][data][random padding]
// totalFrameLen includes everything after itself (i.e. totalFrameLen = origDataLen header + data + padding).
type Padder struct {
	minTarget int
	maxTarget int
}

// NewPadder creates a new padder. If minTarget <= 0, defaults are used.
func NewPadder(minTarget int, maxTarget ...int) *Padder {
	mn := minTarget
	mx := 0
	if len(maxTarget) > 0 {
		mx = maxTarget[0]
	}
	if mn <= 0 {
		mn = DefaultPaddingMinTarget
	}
	if mx <= 0 || mx < mn {
		mx = mn + 300
	}
	return &Padder{minTarget: mn, maxTarget: mx}
}

// randomTarget returns a random target between minTarget and maxTarget.
func (p *Padder) randomTarget() int {
	diff := p.maxTarget - p.minTarget
	if diff <= 0 {
		return p.minTarget
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(diff+1)))
	if err != nil {
		return p.minTarget
	}
	return p.minTarget + int(n.Int64())
}

// Pad pads data to the next multiple of a random target size.
// Format: [2-byte totalFrameLen][2-byte origDataLen][data][random padding]
func (p *Padder) Pad(data []byte) ([]byte, error) {
	origLen := len(data)
	target := p.randomTarget()
	// Total frame = headerSize + origLen + padding. Round up to next multiple of target.
	totalSize := ((headerSize + origLen) / target + 1) * target

	buf := make([]byte, totalSize)
	// Frame length = everything after the first 2 bytes
	frameLen := totalSize - 2
	binary.BigEndian.PutUint16(buf[0:2], uint16(frameLen)) //nolint:gosec // G115: frameLen bounded by max padding target (~1400) + data; well within uint16 range
	binary.BigEndian.PutUint16(buf[2:4], uint16(origLen))
	copy(buf[headerSize:], data)

	// Fill remaining with random bytes
	if padStart := headerSize + origLen; padStart < totalSize {
		if _, err := io.ReadFull(rand.Reader, buf[padStart:]); err != nil {
			return nil, fmt.Errorf("obfs: random padding: %w", err)
		}
	}
	return buf, nil
}

// Unpad removes padding and returns the original data from a complete frame.
func (p *Padder) Unpad(frame []byte) ([]byte, error) {
	if len(frame) < headerSize {
		return nil, io.ErrUnexpectedEOF
	}
	frameLen := int(binary.BigEndian.Uint16(frame[0:2]))
	if 2+frameLen > len(frame) {
		return nil, io.ErrUnexpectedEOF
	}
	origLen := int(binary.BigEndian.Uint16(frame[2:4]))
	if headerSize+origLen > 2+frameLen {
		return nil, fmt.Errorf("obfs: origLen %d exceeds frame", origLen)
	}
	return frame[headerSize : headerSize+origLen], nil
}

// ReadFrame reads one padded frame from r and returns the original data.
func (p *Padder) ReadFrame(r io.Reader) ([]byte, error) {
	// Read 2-byte frame length.
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	frameLen := int(binary.BigEndian.Uint16(hdr[:]))
	if frameLen < 2 {
		return nil, fmt.Errorf("obfs: frame too short (%d)", frameLen)
	}
	const maxFrameLen = 65535 // uint16 max, explicit cap
	if frameLen > maxFrameLen {
		return nil, fmt.Errorf("obfs: frame too large (%d)", frameLen)
	}

	// Read the rest of the frame.
	frameBuf := make([]byte, frameLen)
	if _, err := io.ReadFull(r, frameBuf); err != nil {
		return nil, err
	}

	origLen := int(binary.BigEndian.Uint16(frameBuf[0:2]))
	if 2+origLen > frameLen {
		return nil, fmt.Errorf("obfs: origLen %d exceeds frame payload %d", origLen, frameLen)
	}
	return frameBuf[2 : 2+origLen], nil
}
