package crypto

import (
	"crypto/rand"
	"fmt"
	"io"

	"github.com/flynn/noise"
	"golang.org/x/crypto/curve25519"
)

var cipherSuite = noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256)

type NoiseHandshake struct {
	hs          *noise.HandshakeState
	sendCipher  *noise.CipherState
	recvCipher  *noise.CipherState
	isInitiator bool
	completed   bool

	// Keep raw keys for backward compatibility
	sendKey [32]byte
	recvKey [32]byte
}

func GenerateKeyPair() (pub, priv [32]byte, err error) {
	if _, err = io.ReadFull(rand.Reader, priv[:]); err != nil {
		return
	}
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	pubSlice, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return
	}
	copy(pub[:], pubSlice)
	return
}

func NewInitiator(localPriv, localPub, remotePub [32]byte) *NoiseHandshake {
	staticKey := noise.DHKey{Private: localPriv[:], Public: localPub[:]}
	hs, _ := noise.NewHandshakeState(noise.Config{
		CipherSuite:   cipherSuite,
		Pattern:       noise.HandshakeIK,
		Initiator:     true,
		StaticKeypair: staticKey,
		PeerStatic:    remotePub[:],
	})
	return &NoiseHandshake{hs: hs, isInitiator: true}
}

func NewResponder(localPriv, localPub [32]byte) *NoiseHandshake {
	staticKey := noise.DHKey{Private: localPriv[:], Public: localPub[:]}
	hs, _ := noise.NewHandshakeState(noise.Config{
		CipherSuite:   cipherSuite,
		Pattern:       noise.HandshakeIK,
		Initiator:     false,
		StaticKeypair: staticKey,
	})
	return &NoiseHandshake{hs: hs, isInitiator: false}
}

// WriteMessage writes the next handshake message, optionally with payload.
// Returns the message bytes.
func (h *NoiseHandshake) WriteMessage(payload []byte) ([]byte, error) {
	msg, cs0, cs1, err := h.hs.WriteMessage(nil, payload)
	if err != nil {
		return nil, fmt.Errorf("noise write: %w", err)
	}
	if cs0 != nil && cs1 != nil {
		h.finalize(cs0, cs1)
	}
	return msg, nil
}

// ReadMessage reads and processes a handshake message.
// Returns the decrypted payload.
func (h *NoiseHandshake) ReadMessage(msg []byte) ([]byte, error) {
	payload, cs0, cs1, err := h.hs.ReadMessage(nil, msg)
	if err != nil {
		return nil, fmt.Errorf("noise read: %w", err)
	}
	if cs0 != nil && cs1 != nil {
		h.finalize(cs0, cs1)
	}
	return payload, nil
}

func (h *NoiseHandshake) finalize(cs0, cs1 *noise.CipherState) {
	if h.isInitiator {
		h.sendCipher = cs0
		h.recvCipher = cs1
	} else {
		h.sendCipher = cs1
		h.recvCipher = cs0
	}
	h.completed = true
}

func (h *NoiseHandshake) SendCipher() *noise.CipherState { return h.sendCipher }
func (h *NoiseHandshake) RecvCipher() *noise.CipherState { return h.recvCipher }

// SendKey returns a 32-byte key derived from the send cipher for backward compat.
func (h *NoiseHandshake) SendKey() [32]byte { return h.sendKey }
func (h *NoiseHandshake) RecvKey() [32]byte { return h.recvKey }
func (h *NoiseHandshake) Completed() bool   { return h.completed }

// PeerPublicKey returns the remote party's static public key (available after handshake).
func (h *NoiseHandshake) PeerPublicKey() []byte {
	return h.hs.PeerStatic()
}

// DeriveKeysFromPassword derives a Curve25519 key pair from a password using Argon2id.
func DeriveKeysFromPassword(password string) (pub, priv [32]byte, err error) {
	salt := []byte("shuttle-noise-ik-v1")
	keyMaterial := Argon2Key([]byte(password), salt, 32)
	copy(priv[:], keyMaterial)
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	pubSlice, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return
	}
	copy(pub[:], pubSlice)
	return
}
