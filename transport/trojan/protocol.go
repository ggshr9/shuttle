// Package trojan implements the Trojan protocol with SHA224 authentication
// and cover-site fallback for censorship resistance.
package trojan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/shuttleX/shuttle/transport/shared"
)

const (
	// HashLen is the length of the SHA224 hex string (56 characters).
	HashLen = 56
	// CRLF is the Trojan header line terminator.
	crlf = "\r\n"
)

// HashPassword returns the lowercase hex SHA224 digest of password.
func HashPassword(password string) string {
	h := sha256.New224()
	h.Write([]byte(password))
	return hex.EncodeToString(h.Sum(nil))
}

// EncodeRequest writes a Trojan request header to w:
//
//	[SHA224 hex (56 chars)][CRLF][cmd (1 byte)][SOCKS5 addr][CRLF]
func EncodeRequest(w io.Writer, passwordHash string, cmd byte, address string) error {
	// Write hash + CRLF
	if _, err := io.WriteString(w, passwordHash+crlf); err != nil {
		return fmt.Errorf("trojan: write hash: %w", err)
	}
	// Write cmd
	if _, err := w.Write([]byte{cmd}); err != nil {
		return fmt.Errorf("trojan: write cmd: %w", err)
	}
	// Write SOCKS5-style address
	if err := shared.EncodeAddr(w, "tcp", address); err != nil {
		return fmt.Errorf("trojan: write addr: %w", err)
	}
	// Write trailing CRLF
	if _, err := io.WriteString(w, crlf); err != nil {
		return fmt.Errorf("trojan: write trailing CRLF: %w", err)
	}
	return nil
}

// DecodeRequest reads a Trojan request header from r and returns the
// password hash, command byte, and target address.
func DecodeRequest(r io.Reader) (hash string, cmd byte, address string, err error) {
	// Read 56 bytes of hex hash + 2 bytes CRLF
	buf := make([]byte, HashLen+2)
	if _, err = io.ReadFull(r, buf); err != nil {
		return "", 0, "", fmt.Errorf("trojan: read hash+CRLF: %w", err)
	}
	hash = string(buf[:HashLen])
	if string(buf[HashLen:]) != crlf {
		return "", 0, "", fmt.Errorf("trojan: expected CRLF after hash, got %q", buf[HashLen:])
	}

	// Read 1-byte command
	cmdBuf := [1]byte{}
	if _, err = io.ReadFull(r, cmdBuf[:]); err != nil {
		return "", 0, "", fmt.Errorf("trojan: read cmd: %w", err)
	}
	cmd = cmdBuf[0]

	// Read SOCKS5-style address
	_, address, err = shared.DecodeAddr(r)
	if err != nil {
		return "", 0, "", fmt.Errorf("trojan: read addr: %w", err)
	}

	// Read trailing CRLF
	crlfBuf := [2]byte{}
	if _, err = io.ReadFull(r, crlfBuf[:]); err != nil {
		return "", 0, "", fmt.Errorf("trojan: read trailing CRLF: %w", err)
	}
	if string(crlfBuf[:]) != crlf {
		return "", 0, "", fmt.Errorf("trojan: expected trailing CRLF, got %q", crlfBuf[:])
	}

	return hash, cmd, address, nil
}
