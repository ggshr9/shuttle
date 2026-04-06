// Package vmess implements a VMess AEAD-inspired transport with UUID authentication
// and AES-128-GCM encrypted headers and body. This implementation follows VMess AEAD
// conventions (alterId=0) but is not wire-compatible with V2Ray; it can be upgraded
// to full wire compatibility later while preserving the Dialer/InboundHandler API.
//
// Wire format (client -> server):
//
//	Request header:
//	  [auth_id (16)]  — HMAC-MD5(uuid, timestamp) for user lookup
//	  [nonce (12)]    — random GCM nonce
//	  [encrypted_header (variable)] — AES-128-GCM encrypted RequestHeader
//
//	RequestHeader (plaintext, before encryption):
//	  [version (1)]   — protocol version (0x01)
//	  [cmd (1)]       — 0x01=TCP, 0x02=UDP
//	  [security (1)]  — 0x03=AES-128-GCM, 0x04=ChaCha20-Poly1305, 0x05=None
//	  [addr ...]      — SOCKS5-style address (atype + host + port)
//	  [data_iv (8)]   — random IV seed for body encryption
//	  [data_key (8)]  — random key seed for body encryption
//
//	Body:
//	  [length (2, big-endian)] [nonce (12)] [encrypted_chunk (length)] [tag (16)]
//	  ...repeated until close
//
// Response header:
//
//	[response_key (1)] — 0x00 = OK, nonzero = error
package vmess

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/shuttleX/shuttle/transport/shared"
)

const (
	Version = 0x01

	CmdTCP byte = 0x01
	CmdUDP byte = 0x02

	SecurityAES128GCM byte = 0x03
	SecurityNone      byte = 0x05

	AuthIDLen = 16
	NonceLen  = 12

	ResponseOK    byte = 0x00
	ResponseError byte = 0x01
)

// RequestHeader is the decrypted VMess request metadata.
type RequestHeader struct {
	Version  byte
	Command  byte
	Security byte
	Address  string // "host:port"
	DataIV   [8]byte
	DataKey  [8]byte
}

// deriveKey produces a 16-byte AES key from UUID bytes.
func deriveKey(uuid [16]byte) []byte {
	h := md5.New()
	h.Write(uuid[:])
	h.Write(uuid[:])
	return h.Sum(nil)
}

// computeAuthID computes the 16-byte auth_id = HMAC-MD5(uuid, timestamp_be64).
func computeAuthID(uuid [16]byte, timestamp int64) [AuthIDLen]byte {
	var tsBuf [8]byte
	binary.BigEndian.PutUint64(tsBuf[:], uint64(timestamp))

	mac := hmac.New(md5.New, uuid[:])
	mac.Write(tsBuf[:])
	sum := mac.Sum(nil)

	var out [AuthIDLen]byte
	copy(out[:], sum[:AuthIDLen])
	return out
}

// encryptHeader encrypts a serialised request header using AES-128-GCM keyed by the UUID.
// Returns nonce (12) + ciphertext (includes GCM tag).
func encryptHeader(key []byte, plaintext []byte) (nonce [NonceLen]byte, ciphertext []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nonce, nil, fmt.Errorf("vmess: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nonce, nil, fmt.Errorf("vmess: new gcm: %w", err)
	}
	if _, err := rand.Read(nonce[:]); err != nil {
		return nonce, nil, fmt.Errorf("vmess: random nonce: %w", err)
	}
	ciphertext = gcm.Seal(nil, nonce[:], plaintext, nil)
	return nonce, ciphertext, nil
}

// decryptHeader decrypts a GCM-encrypted request header.
func decryptHeader(key []byte, nonce []byte, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("vmess: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("vmess: new gcm: %w", err)
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// EncodeRequest writes a complete VMess request header to w.
func EncodeRequest(w io.Writer, uuid [16]byte, timestamp int64, hdr *RequestHeader) error {
	// 1. Write auth_id
	authID := computeAuthID(uuid, timestamp)
	if _, err := w.Write(authID[:]); err != nil {
		return fmt.Errorf("vmess: write auth_id: %w", err)
	}

	// 2. Serialise the header
	plain, err := marshalHeader(hdr)
	if err != nil {
		return fmt.Errorf("vmess: marshal header: %w", err)
	}

	// 3. Encrypt
	key := deriveKey(uuid)
	nonce, ct, err := encryptHeader(key, plain)
	if err != nil {
		return err
	}

	// 4. Write nonce + length(2) + ciphertext
	if _, err := w.Write(nonce[:]); err != nil {
		return fmt.Errorf("vmess: write nonce: %w", err)
	}
	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(ct)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("vmess: write header length: %w", err)
	}
	if _, err := w.Write(ct); err != nil {
		return fmt.Errorf("vmess: write encrypted header: %w", err)
	}
	return nil
}

// DecodeRequest reads a VMess request header from r. The caller provides a
// lookup function to resolve auth_id -> UUID.
func DecodeRequest(r io.Reader, lookup func([AuthIDLen]byte) ([16]byte, bool)) (*RequestHeader, error) {
	// 1. Read auth_id
	var authID [AuthIDLen]byte
	if _, err := io.ReadFull(r, authID[:]); err != nil {
		return nil, fmt.Errorf("vmess: read auth_id: %w", err)
	}

	uuid, ok := lookup(authID)
	if !ok {
		return nil, fmt.Errorf("vmess: unknown auth_id")
	}

	// 2. Read nonce
	var nonce [NonceLen]byte
	if _, err := io.ReadFull(r, nonce[:]); err != nil {
		return nil, fmt.Errorf("vmess: read nonce: %w", err)
	}

	// 3. Read header length + ciphertext
	var lenBuf [2]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, fmt.Errorf("vmess: read header length: %w", err)
	}
	ctLen := binary.BigEndian.Uint16(lenBuf[:])
	ct := make([]byte, ctLen)
	if _, err := io.ReadFull(r, ct); err != nil {
		return nil, fmt.Errorf("vmess: read encrypted header: %w", err)
	}

	// 4. Decrypt
	key := deriveKey(uuid)
	plain, err := decryptHeader(key, nonce[:], ct)
	if err != nil {
		return nil, fmt.Errorf("vmess: decrypt header: %w", err)
	}

	return unmarshalHeader(plain)
}

// marshalHeader serialises a RequestHeader to bytes.
func marshalHeader(hdr *RequestHeader) ([]byte, error) {
	// version(1) + cmd(1) + security(1) + addr(variable) + dataIV(8) + dataKey(8)
	buf := make([]byte, 0, 64)
	buf = append(buf, hdr.Version, hdr.Command, hdr.Security)

	// Encode SOCKS5-style address into a temporary buffer.
	addrBuf := &appendWriter{}
	network := "tcp"
	if hdr.Command == CmdUDP {
		network = "udp"
	}
	if err := shared.EncodeAddr(addrBuf, network, hdr.Address); err != nil {
		return nil, fmt.Errorf("vmess: encode addr: %w", err)
	}
	buf = append(buf, addrBuf.data...)
	buf = append(buf, hdr.DataIV[:]...)
	buf = append(buf, hdr.DataKey[:]...)
	return buf, nil
}

// unmarshalHeader deserialises a RequestHeader from bytes.
func unmarshalHeader(data []byte) (*RequestHeader, error) {
	if len(data) < 3+1+2+8+8 { // min: version+cmd+security + IPv4(1+4+2) + IV + key
		return nil, fmt.Errorf("vmess: header too short (%d bytes)", len(data))
	}

	hdr := &RequestHeader{
		Version:  data[0],
		Command:  data[1],
		Security: data[2],
	}

	// Decode SOCKS5-style address starting at offset 3.
	r := &byteReader{data: data[3:]}
	_, addr, err := shared.DecodeAddr(r)
	if err != nil {
		return nil, fmt.Errorf("vmess: decode addr: %w", err)
	}
	hdr.Address = addr

	// Remaining bytes: dataIV(8) + dataKey(8)
	remaining := r.data
	if len(remaining) < 16 {
		return nil, fmt.Errorf("vmess: missing data IV/key (%d bytes left)", len(remaining))
	}
	copy(hdr.DataIV[:], remaining[:8])
	copy(hdr.DataKey[:], remaining[8:16])
	return hdr, nil
}

// appendWriter is a simple io.Writer that appends to a byte slice.
type appendWriter struct {
	data []byte
}

func (w *appendWriter) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

// byteReader wraps a byte slice as an io.Reader, advancing as bytes are consumed.
type byteReader struct {
	data []byte
}

func (r *byteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}
