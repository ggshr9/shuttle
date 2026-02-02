package mesh

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

const (
	// MeshMagic is the handshake identifier sent by clients to request a mesh stream.
	MeshMagic = "MESH\n"

	// ProtocolVersion is the mesh protocol version. Bump this when the
	// frame format or handshake layout changes.
	ProtocolVersion = byte(1)

	// HandshakeSize is the size of the server's handshake response:
	// version(1) + reserved(1) + reserved(2) + IP(4) + mask(4) + gateway(4).
	HandshakeSize = 16

	// MaxFrameSize is the maximum payload size of a single mesh frame.
	MaxFrameSize = 65535
)

// WriteFrame writes a length-prefixed frame: [2B big-endian length][payload].
func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) > MaxFrameSize {
		return fmt.Errorf("mesh: frame too large: %d > %d", len(payload), MaxFrameSize)
	}
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// ReadFrame reads a length-prefixed frame and returns the payload.
func ReadFrame(r io.Reader) ([]byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint16(hdr[:])
	if length == 0 {
		return nil, fmt.Errorf("mesh: zero-length frame")
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// EncodeHandshake encodes the server handshake response.
// Layout: [version(1)][reserved(3)][IP(4)][mask(4)][gateway(4)] = 16 bytes.
func EncodeHandshake(ip, mask, gateway net.IP) []byte {
	buf := make([]byte, HandshakeSize)
	buf[0] = ProtocolVersion
	// buf[1..3] reserved
	copy(buf[4:8], ip.To4())
	copy(buf[8:12], mask.To4())
	copy(buf[12:16], gateway.To4())
	return buf
}

// DecodeHandshake decodes the server handshake response.
func DecodeHandshake(data []byte) (ip, mask, gateway net.IP, err error) {
	if len(data) < HandshakeSize {
		return nil, nil, nil, fmt.Errorf("mesh: handshake too short: %d", len(data))
	}
	version := data[0]
	if version != ProtocolVersion {
		return nil, nil, nil, fmt.Errorf("mesh: unsupported protocol version %d (want %d)", version, ProtocolVersion)
	}
	ip = net.IP(make([]byte, 4))
	mask = net.IP(make([]byte, 4))
	gateway = net.IP(make([]byte, 4))
	copy(ip, data[4:8])
	copy(mask, data[8:12])
	copy(gateway, data[12:16])
	return
}
