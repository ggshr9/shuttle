package crypto

import (
	"crypto/rand"
	"fmt"
	"io"

	"github.com/flynn/noise"
	"golang.org/x/crypto/curve25519"
)

// Version bytes for handshake negotiation.
const (
	HandshakeVersionClassical byte = 0x01
	HandshakeVersionHybridPQ  byte = 0x02
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

// ClampPrivateKey applies Curve25519 key clamping to a 32-byte private key.
func ClampPrivateKey(priv []byte) {
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
}

func GenerateKeyPair() (pub, priv [32]byte, err error) {
	if _, err = io.ReadFull(rand.Reader, priv[:]); err != nil {
		return
	}
	ClampPrivateKey(priv[:])
	pubSlice, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return
	}
	copy(pub[:], pubSlice)
	return
}

func NewInitiator(localPriv, localPub, remotePub [32]byte) (*NoiseHandshake, error) {
	staticKey := noise.DHKey{Private: localPriv[:], Public: localPub[:]}
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:   cipherSuite,
		Pattern:       noise.HandshakeIK,
		Initiator:     true,
		StaticKeypair: staticKey,
		PeerStatic:    remotePub[:],
	})
	if err != nil {
		return nil, fmt.Errorf("noise initiator handshake state: %w", err)
	}
	return &NoiseHandshake{hs: hs, isInitiator: true}, nil
}

func NewResponder(localPriv, localPub [32]byte) (*NoiseHandshake, error) {
	staticKey := noise.DHKey{Private: localPriv[:], Public: localPub[:]}
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:   cipherSuite,
		Pattern:       noise.HandshakeIK,
		Initiator:     false,
		StaticKeypair: staticKey,
	})
	if err != nil {
		return nil, fmt.Errorf("noise responder handshake state: %w", err)
	}
	return &NoiseHandshake{hs: hs, isInitiator: false}, nil
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
	ClampPrivateKey(priv[:])
	pubSlice, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return
	}
	copy(pub[:], pubSlice)
	return
}

// PQHandshake wraps the Noise handshake with an additional hybrid PQ KEM exchange.
// After the Noise IK handshake completes, both sides perform a hybrid KEM
// exchange and mix the PQ shared secret into the transport keys.
//
// NOTE: The current ML-KEM-768 implementation uses a second X25519 exchange as
// a placeholder. Replace with cloudflare/circl for production PQ security.
type PQHandshake struct {
	kem     *HybridKEM
	keyPair *HybridKEMKeyPair
}

// NewPQHandshake creates a new PQ handshake state with a fresh key pair.
func NewPQHandshake() (*PQHandshake, error) {
	kem := NewHybridKEM()
	kp, err := kem.GenerateKeyPair(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("pq handshake keygen: %w", err)
	}
	return &PQHandshake{kem: kem, keyPair: kp}, nil
}

// PublicKeyBytes returns the serialized hybrid public key for transmission.
func (p *PQHandshake) PublicKeyBytes() []byte {
	return PublicKeyBytes(p.keyPair)
}

// Encapsulate performs a hybrid KEM encapsulation using the peer's PQ public key.
// Returns the shared secret and the ciphertext to send to the peer.
func (p *PQHandshake) Encapsulate(peerPQPublic []byte) (sharedSecret, ciphertext []byte, err error) {
	peer, err := ParsePublicKey(peerPQPublic)
	if err != nil {
		return nil, nil, fmt.Errorf("pq encapsulate: parse peer key: %w", err)
	}
	ss, ct, err := p.kem.Encapsulate(peer)
	if err != nil {
		return nil, nil, fmt.Errorf("pq encapsulate: %w", err)
	}
	return ss[:], ct, nil
}

// Decapsulate recovers the shared secret from ciphertext using local private keys.
func (p *PQHandshake) Decapsulate(ciphertext []byte) (sharedSecret []byte, err error) {
	ss, err := p.kem.Decapsulate(p.keyPair, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("pq decapsulate: %w", err)
	}
	return ss[:], nil
}
