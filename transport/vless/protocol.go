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
	Flow    string // XTLS flow, e.g. "xtls-rprx-vision" (empty = no addons)
}

// EncodeRequest writes a VLESS request header to w.
// Format: [version(1)=0][uuid(16)][addon_len(1)][addons(addon_len)][cmd(1)][addr(SOCKS5)]
//
// When h.Flow is non-empty the addons field encodes the flow string using
// protobuf wire format: tag 0x0a (field 1, wire type 2) + varint length + bytes.
// This is the format used by XTLS Vision (xtls-rprx-vision).
func EncodeRequest(w io.Writer, h *RequestHeader) error {
	// Build optional addons bytes for the flow field.
	var addons []byte
	if h.Flow != "" {
		flowBytes := []byte(h.Flow)
		// Protobuf field 1, wire type 2 (length-delimited): tag = (1 << 3) | 2 = 0x0a
		addons = append(addons, 0x0a)
		addons = append(addons, byte(len(flowBytes)))
		addons = append(addons, flowBytes...)
	}

	// version + uuid + addon_len + [addons] + cmd = 1+16+1+len(addons)+1
	buf := make([]byte, 0, 19+len(addons))
	buf = append(buf, Version)
	buf = append(buf, h.UUID[:]...)
	buf = append(buf, byte(len(addons)))
	buf = append(buf, addons...)
	buf = append(buf, h.Cmd)

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
	// Read: version(1) + uuid(16) + addon_len(1) = 18 bytes
	buf := make([]byte, 18)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("vless: read request header: %w", err)
	}

	if buf[0] != Version {
		return nil, fmt.Errorf("vless: unsupported version %d", buf[0])
	}

	h := &RequestHeader{}
	copy(h.UUID[:], buf[1:17])
	addonLen := buf[17]

	if addonLen > 0 {
		// Skip addon bytes.
		if _, err := io.ReadFull(r, make([]byte, addonLen)); err != nil {
			return nil, fmt.Errorf("vless: skip addons: %w", err)
		}
	}

	// Read cmd byte (comes after addons).
	cmdBuf := make([]byte, 1)
	if _, err := io.ReadFull(r, cmdBuf); err != nil {
		return nil, fmt.Errorf("vless: read cmd: %w", err)
	}
	h.Cmd = cmdBuf[0]

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
