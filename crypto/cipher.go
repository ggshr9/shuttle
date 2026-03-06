package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"runtime"
	"sync"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

// CipherType selects the AEAD cipher.
type CipherType int

const (
	CipherChaChaPoly CipherType = iota
	CipherAESGCM
)

// AutoSelectCipher picks the fastest cipher for this platform.
// AES-GCM is faster on x86 with AES-NI; ChaCha20 is faster on ARM/other.
func AutoSelectCipher() CipherType {
	arch := runtime.GOARCH
	if arch == "amd64" || arch == "arm64" {
		// Modern x86/ARM64 have hardware AES support
		return CipherAESGCM
	}
	return CipherChaChaPoly
}

// NewAEAD creates an AEAD cipher from a key.
func NewAEAD(key []byte, ct CipherType) (cipher.AEAD, error) {
	switch ct {
	case CipherAESGCM:
		block, err := aes.NewCipher(key[:32])
		if err != nil {
			return nil, fmt.Errorf("aes: %w", err)
		}
		return cipher.NewGCM(block)
	case CipherChaChaPoly:
		return chacha20poly1305.New(key[:32])
	default:
		return nil, fmt.Errorf("unknown cipher type: %d", ct)
	}
}

// Encrypt encrypts plaintext with the given key using the auto-selected AEAD.
func Encrypt(key, nonce, plaintext []byte) ([]byte, error) {
	aead, err := NewAEAD(key, AutoSelectCipher())
	if err != nil {
		return nil, err
	}

	if nonce == nil {
		nonce = make([]byte, aead.NonceSize())
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return nil, fmt.Errorf("generate nonce: %w", err)
		}
	}

	ciphertext := aead.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext with the given key.
func Decrypt(key, nonce, ciphertext []byte) ([]byte, error) {
	aead, err := NewAEAD(key, AutoSelectCipher())
	if err != nil {
		return nil, err
	}

	nonceSize := aead.NonceSize()
	if nonce == nil {
		if len(ciphertext) < nonceSize {
			return nil, fmt.Errorf("ciphertext too short")
		}
		nonce = ciphertext[:nonceSize]
		ciphertext = ciphertext[nonceSize:]
	}

	return aead.Open(nil, nonce, ciphertext, nil)
}

// DeriveKeys derives key material using HKDF-SHA256.
func DeriveKeys(ikm []byte, length int) ([]byte, error) {
	r := hkdf.New(sha256.New, ikm, []byte("shuttle-v1"), []byte("session-keys"))
	keys := make([]byte, length)
	if _, err := io.ReadFull(r, keys); err != nil {
		return nil, fmt.Errorf("hkdf expand: %w", err)
	}
	return keys, nil
}

// Argon2Key derives a key from a password using Argon2id.
func Argon2Key(password, salt []byte, keyLen uint32) []byte {
	return argon2.IDKey(password, salt, 3, 64*1024, 4, keyLen)
}

// StreamCipher wraps an AEAD for stream encryption with automatic nonce management.
type StreamCipher struct {
	mu    sync.Mutex
	aead  cipher.AEAD
	nonce uint64
	key   [32]byte
}

// NewStreamCipher creates a new stream cipher for encrypting/decrypting frames.
func NewStreamCipher(key [32]byte, ct CipherType) (*StreamCipher, error) {
	aead, err := NewAEAD(key[:], ct)
	if err != nil {
		return nil, err
	}
	return &StreamCipher{aead: aead, key: key}, nil
}

// Seal encrypts a frame, auto-incrementing the nonce.
func (sc *StreamCipher) Seal(plaintext []byte) []byte {
	sc.mu.Lock()
	nonce := make([]byte, sc.aead.NonceSize())
	binary.LittleEndian.PutUint64(nonce, sc.nonce)
	sc.nonce++
	sc.mu.Unlock()
	return sc.aead.Seal(nil, nonce, plaintext, nil)
}

// Open decrypts a frame with the expected nonce.
func (sc *StreamCipher) Open(ciphertext []byte) ([]byte, error) {
	sc.mu.Lock()
	nonce := make([]byte, sc.aead.NonceSize())
	binary.LittleEndian.PutUint64(nonce, sc.nonce)
	sc.nonce++
	sc.mu.Unlock()
	return sc.aead.Open(nil, nonce, ciphertext, nil)
}
