package obfs

import (
	"crypto/rand"
	"io"
)

const (
	// DefaultPaddingTarget is the QUIC recommended MTU.
	DefaultPaddingTarget = 1200
)

// Padder pads packets to a fixed size to eliminate packet-size fingerprinting.
type Padder struct {
	target int
}

// NewPadder creates a new padder with the given target size.
func NewPadder(target int) *Padder {
	if target <= 0 {
		target = DefaultPaddingTarget
	}
	return &Padder{target: target}
}

// Pad pads data to the next multiple of the target size.
// Format: [2-byte original length][data][random padding]
func (p *Padder) Pad(data []byte) []byte {
	origLen := len(data)
	// Calculate padded size: next multiple of target
	paddedSize := ((origLen + 2) / p.target + 1) * p.target
	if paddedSize < p.target {
		paddedSize = p.target
	}

	buf := make([]byte, paddedSize)
	// Store original length (big-endian)
	buf[0] = byte(origLen >> 8)
	buf[1] = byte(origLen)
	copy(buf[2:], data)

	// Fill remaining with random bytes
	if paddedSize > origLen+2 {
		io.ReadFull(rand.Reader, buf[origLen+2:])
	}
	return buf
}

// Unpad removes padding and returns the original data.
func (p *Padder) Unpad(data []byte) ([]byte, error) {
	if len(data) < 2 {
		return nil, io.ErrUnexpectedEOF
	}
	origLen := int(data[0])<<8 | int(data[1])
	if origLen+2 > len(data) {
		return nil, io.ErrUnexpectedEOF
	}
	return data[2 : 2+origLen], nil
}
