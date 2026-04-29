package reality

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	// pqHKDFInfo is the HKDF info string for deriving the PQ session key.
	pqHKDFInfo = "shuttle-reality-pq-v1"

	// pqFrameMaxPlaintext is the maximum plaintext size per frame.
	// 16384 bytes of plaintext + 16 bytes Poly1305 tag = 16400 ciphertext.
	// The 2-byte length prefix encodes the ciphertext length, so max is 65535.
	pqFrameMaxPlaintext = 16384
)

// wrapConnWithPQ derives an XChaCha20-Poly1305 key from pqSecret via HKDF-SHA256
// and wraps conn in bidirectional AEAD encryption with length-prefixed framing.
// Each direction (read/write) maintains an independent monotonic nonce counter.
func wrapConnWithPQ(conn net.Conn, pqSecret []byte) (net.Conn, error) {
	if len(pqSecret) == 0 {
		return nil, fmt.Errorf("pqwrap: empty shared secret")
	}

	// Derive 32-byte key via HKDF-SHA256.
	r := hkdf.New(sha256.New, pqSecret, nil, []byte(pqHKDFInfo))
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, fmt.Errorf("pqwrap: hkdf derive: %w", err)
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("pqwrap: new xchacha20: %w", err)
	}

	return &pqConn{
		Conn: conn,
		aead: aead,
		// readBuf / decBuf lazily allocated on first Read
	}, nil
}

// pqConn wraps a net.Conn with XChaCha20-Poly1305 AEAD encryption.
// Writes are framed as [2-byte big-endian ciphertext length][ciphertext].
// Reads reverse the process. Each direction has an independent nonce counter.
type pqConn struct {
	net.Conn

	aead interface {
		Seal(dst, nonce, plaintext, additionalData []byte) []byte
		Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
		NonceSize() int
		Overhead() int
	}

	writeMu    sync.Mutex
	writeNonce uint64

	readMu    sync.Mutex
	readNonce uint64
	readBuf   []byte // decrypted plaintext buffer for partial reads
	readOff   int    // offset into readBuf
}

// makeNonce writes the counter into an XChaCha20-Poly1305 nonce (24 bytes).
func makeNonce(buf []byte, counter uint64) {
	// Zero out the nonce, then write counter in first 8 bytes (little-endian).
	for i := range buf {
		buf[i] = 0
	}
	binary.LittleEndian.PutUint64(buf, counter)
}

// Write encrypts p and writes a length-prefixed ciphertext frame to the underlying conn.
//
// Per-frame allocations: previously each call allocated a new
// nonce slice (24 B) and a new 2-byte header, both on the heap.
// Since chacha20poly1305.NonceSizeX is a constant, the nonce now
// lives in a stack array; the same goes for the length prefix.
// Hot-path data plane sees zero allocations beyond the AEAD
// ciphertext that Seal returns.
func (c *pqConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	var nonce [chacha20poly1305.NonceSizeX]byte
	var header [2]byte

	written := 0
	for len(p) > 0 {
		chunk := p
		if len(chunk) > pqFrameMaxPlaintext {
			chunk = chunk[:pqFrameMaxPlaintext]
		}

		makeNonce(nonce[:], c.writeNonce)
		c.writeNonce++

		ciphertext := c.aead.Seal(nil, nonce[:], chunk, nil)

		binary.BigEndian.PutUint16(header[:], uint16(len(ciphertext)))
		if _, err := c.Conn.Write(header[:]); err != nil {
			return written, err
		}
		if _, err := c.Conn.Write(ciphertext); err != nil {
			return written, err
		}

		written += len(chunk)
		p = p[len(chunk):]
	}
	return written, nil
}

// Read decrypts data from the underlying conn. It reads one frame at a time
// and buffers decrypted plaintext for partial reads.
func (c *pqConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	// Return buffered data first.
	if c.readOff < len(c.readBuf) {
		n := copy(p, c.readBuf[c.readOff:])
		c.readOff += n
		if c.readOff >= len(c.readBuf) {
			c.readBuf = nil
			c.readOff = 0
		}
		return n, nil
	}

	// Read next frame: 2-byte length prefix.
	var header [2]byte
	if _, err := io.ReadFull(c.Conn, header[:]); err != nil {
		return 0, err
	}
	ctLen := binary.BigEndian.Uint16(header[:])
	if ctLen == 0 {
		return 0, fmt.Errorf("pqwrap: zero-length ciphertext frame")
	}

	ciphertext := make([]byte, ctLen)
	if _, err := io.ReadFull(c.Conn, ciphertext); err != nil {
		return 0, err
	}

	var nonce [chacha20poly1305.NonceSizeX]byte
	makeNonce(nonce[:], c.readNonce)
	c.readNonce++

	plaintext, err := c.aead.Open(nil, nonce[:], ciphertext, nil)
	if err != nil {
		return 0, fmt.Errorf("pqwrap: decrypt: %w", err)
	}

	n := copy(p, plaintext)
	if n < len(plaintext) {
		c.readBuf = plaintext
		c.readOff = n
	}
	return n, nil
}
