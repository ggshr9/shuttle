// Package hysteria2 implements the Hysteria2 protocol over QUIC with
// stream multiplexing and custom authentication.
package hysteria2

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"

	"github.com/quic-go/quic-go"
)

// Auth protocol constants.
const (
	AuthVersion byte = 1
	AuthOK      byte = 0
	AuthFail    byte = 1
)

// EncodeStreamHeader writes a Hysteria2 stream header:
//
//	[request_id(4 big-endian)][addr_len(2 big-endian)][addr_bytes][padding_len(2 big-endian)][padding_bytes]
func EncodeStreamHeader(w io.Writer, requestID uint32, address string) error {
	addrBytes := []byte(address)
	if len(addrBytes) > 65535 {
		return fmt.Errorf("hysteria2: address too long (%d bytes)", len(addrBytes))
	}

	// Random padding 0-64 bytes
	paddingLen := rand.Intn(65) //nolint:gosec // non-crypto padding
	padding := make([]byte, paddingLen)
	rand.Read(padding) //nolint:gosec,errcheck

	// Header: 4 + 2 + len(addr) + 2 + paddingLen
	buf := make([]byte, 4+2+len(addrBytes)+2+paddingLen)
	binary.BigEndian.PutUint32(buf[0:4], requestID)
	binary.BigEndian.PutUint16(buf[4:6], uint16(len(addrBytes)))
	copy(buf[6:6+len(addrBytes)], addrBytes)
	off := 6 + len(addrBytes)
	binary.BigEndian.PutUint16(buf[off:off+2], uint16(paddingLen))
	copy(buf[off+2:], padding)

	_, err := w.Write(buf)
	return err
}

// DecodeStreamHeader reads a Hysteria2 stream header and returns the request ID
// and target address.
func DecodeStreamHeader(r io.Reader) (requestID uint32, address string, err error) {
	// Read request_id (4 bytes) + addr_len (2 bytes)
	hdr := [6]byte{}
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return 0, "", fmt.Errorf("hysteria2: read header: %w", err)
	}
	requestID = binary.BigEndian.Uint32(hdr[0:4])
	addrLen := binary.BigEndian.Uint16(hdr[4:6])

	// Read address
	addrBuf := make([]byte, addrLen)
	if _, err = io.ReadFull(r, addrBuf); err != nil {
		return 0, "", fmt.Errorf("hysteria2: read address: %w", err)
	}
	address = string(addrBuf)

	// Read padding_len (2 bytes)
	padLenBuf := [2]byte{}
	if _, err = io.ReadFull(r, padLenBuf[:]); err != nil {
		return 0, "", fmt.Errorf("hysteria2: read padding length: %w", err)
	}
	paddingLen := binary.BigEndian.Uint16(padLenBuf[:])

	// Discard padding
	if paddingLen > 0 {
		if _, err = io.CopyN(io.Discard, r, int64(paddingLen)); err != nil {
			return 0, "", fmt.Errorf("hysteria2: read padding: %w", err)
		}
	}

	return requestID, address, nil
}

// EncodeAuth writes the client auth message:
//
//	[auth_version(1)][password_len(2 big-endian)][password_bytes]
func EncodeAuth(w io.Writer, password string) error {
	pwBytes := []byte(password)
	if len(pwBytes) > 65535 {
		return fmt.Errorf("hysteria2: password too long")
	}
	buf := make([]byte, 1+2+len(pwBytes))
	buf[0] = AuthVersion
	binary.BigEndian.PutUint16(buf[1:3], uint16(len(pwBytes)))
	copy(buf[3:], pwBytes)
	_, err := w.Write(buf)
	return err
}

// DecodeAuth reads the client auth message and returns the password.
func DecodeAuth(r io.Reader) (password string, err error) {
	hdr := [3]byte{}
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return "", fmt.Errorf("hysteria2: read auth header: %w", err)
	}
	if hdr[0] != AuthVersion {
		return "", fmt.Errorf("hysteria2: unsupported auth version %d", hdr[0])
	}
	pwLen := binary.BigEndian.Uint16(hdr[1:3])
	pwBuf := make([]byte, pwLen)
	if _, err = io.ReadFull(r, pwBuf); err != nil {
		return "", fmt.Errorf("hysteria2: read password: %w", err)
	}
	return string(pwBuf), nil
}

// WriteAuthResult writes a single status byte to the auth stream.
func WriteAuthResult(w io.Writer, status byte) error {
	_, err := w.Write([]byte{status})
	return err
}

// ReadAuthResult reads the single-byte auth response.
func ReadAuthResult(r io.Reader) (byte, error) {
	buf := [1]byte{}
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, fmt.Errorf("hysteria2: read auth result: %w", err)
	}
	return buf[0], nil
}

// streamConn wraps a *quic.Stream as a net.Conn.
type streamConn struct {
	*quic.Stream
	local, remote net.Addr
}

func (c *streamConn) LocalAddr() net.Addr  { return c.local }
func (c *streamConn) RemoteAddr() net.Addr { return c.remote }
