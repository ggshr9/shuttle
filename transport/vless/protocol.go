package vless

import (
	"fmt"
	"io"

	"github.com/shuttleX/shuttle/transport/shared"
)

const (
	Version = 0

	CmdTCP = 0x01
	CmdUDP = 0x02
)

// RequestHeader is the VLESS request sent from client to server.
type RequestHeader struct {
	UUID    [16]byte
	Cmd     byte   // CmdTCP or CmdUDP
	Network string // "tcp" or "udp"
	Address string // "host:port"
}

// EncodeRequest writes a VLESS request header to w.
// Format: [version(1)=0][uuid(16)][addon_len(1)=0][cmd(1)][addr(SOCKS5)]
func EncodeRequest(w io.Writer, h *RequestHeader) error {
	// version + uuid + addon_len + cmd = 1+16+1+1 = 19 bytes
	buf := make([]byte, 19)
	buf[0] = Version
	copy(buf[1:17], h.UUID[:])
	buf[17] = 0 // addon_len
	buf[18] = h.Cmd

	if _, err := w.Write(buf); err != nil {
		return fmt.Errorf("vless: write request header: %w", err)
	}

	if err := shared.EncodeAddr(w, h.Network, h.Address); err != nil {
		return fmt.Errorf("vless: encode addr: %w", err)
	}

	return nil
}

// DecodeRequest reads a VLESS request header from r.
func DecodeRequest(r io.Reader) (*RequestHeader, error) {
	buf := make([]byte, 19)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("vless: read request header: %w", err)
	}

	if buf[0] != Version {
		return nil, fmt.Errorf("vless: unsupported version %d", buf[0])
	}

	addonLen := buf[17]
	if addonLen > 0 {
		// Skip addon bytes.
		if _, err := io.ReadFull(r, make([]byte, addonLen)); err != nil {
			return nil, fmt.Errorf("vless: skip addons: %w", err)
		}
	}

	h := &RequestHeader{
		Cmd: buf[18],
	}
	copy(h.UUID[:], buf[1:17])

	network, addr, err := shared.DecodeAddr(r)
	if err != nil {
		return nil, fmt.Errorf("vless: decode addr: %w", err)
	}
	h.Network = network
	h.Address = addr

	return h, nil
}

// EncodeResponse writes a VLESS response header to w.
// Format: [version(1)=0][addon_len(1)=0]
func EncodeResponse(w io.Writer) error {
	_, err := w.Write([]byte{Version, 0})
	if err != nil {
		return fmt.Errorf("vless: write response header: %w", err)
	}
	return nil
}

// DecodeResponse reads a VLESS response header from r.
func DecodeResponse(r io.Reader) error {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(r, buf); err != nil {
		return fmt.Errorf("vless: read response header: %w", err)
	}

	if buf[0] != Version {
		return fmt.Errorf("vless: unsupported response version %d", buf[0])
	}

	addonLen := buf[1]
	if addonLen > 0 {
		if _, err := io.ReadFull(r, make([]byte, addonLen)); err != nil {
			return fmt.Errorf("vless: skip response addons: %w", err)
		}
	}

	return nil
}
