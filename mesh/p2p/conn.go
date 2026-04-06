package p2p

import (
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

// P2P packet magic
var P2PMagic = []byte{'P', '2', 'P', '1'}

// P2P packet types
const (
	P2PData      byte = 0x01 // Data packet
	P2PKeepAlive byte = 0x02 // Keep-alive
	P2PClose     byte = 0x03 // Connection close
)

// Packet overhead: magic(4) + type(1) + nonce(12) + tag(16) = 33 bytes
const P2POverhead = 33

// MaxP2PPacketSize is the maximum UDP packet size for P2P.
const MaxP2PPacketSize = 1400

// P2PConn represents an encrypted P2P connection.
type P2PConn struct {
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr
	remoteVIP  net.IP
	localVIP   net.IP

	sendAEAD cipher.AEAD
	recvAEAD cipher.AEAD
	sendKey  [32]byte
	recvKey  [32]byte

	sendNonce uint64

	lastSend atomic.Int64
	lastRecv atomic.Int64

	mu     sync.Mutex
	closed bool
}

// NewP2PConn creates a new P2P connection.
func NewP2PConn(conn *net.UDPConn, remoteAddr *net.UDPAddr, remoteVIP, localVIP net.IP, sendKey, recvKey [32]byte) (*P2PConn, error) {
	sendAEAD, err := chacha20poly1305.NewX(sendKey[:])
	if err != nil {
		return nil, fmt.Errorf("p2p: create send cipher: %w", err)
	}

	recvAEAD, err := chacha20poly1305.NewX(recvKey[:])
	if err != nil {
		return nil, fmt.Errorf("p2p: create recv cipher: %w", err)
	}

	pc := &P2PConn{
		conn:       conn,
		remoteAddr: remoteAddr,
		remoteVIP:  remoteVIP,
		localVIP:   localVIP,
		sendAEAD:   sendAEAD,
		recvAEAD:   recvAEAD,
		sendKey:    sendKey,
		recvKey:    recvKey,
	}

	now := time.Now().Unix()
	pc.lastSend.Store(now)
	pc.lastRecv.Store(now)

	return pc, nil
}

// Send sends encrypted data to the peer.
func (pc *P2PConn) Send(data []byte) error {
	pc.mu.Lock()
	if pc.closed {
		pc.mu.Unlock()
		return errors.New("p2p: connection closed")
	}
	pc.mu.Unlock()

	return pc.sendPacket(P2PData, data)
}

// sendPacket sends an encrypted packet.
func (pc *P2PConn) sendPacket(typ byte, data []byte) error {
	// Build packet: magic(4) + type(1) + nonce(12) + encrypted_data
	nonce := make([]byte, 24)
	binary.LittleEndian.PutUint64(nonce, atomic.AddUint64(&pc.sendNonce, 1)-1)
	// Add randomness to remaining bytes for XChaCha20
	if _, err := rand.Read(nonce[8:]); err != nil {
		return fmt.Errorf("p2p: generate nonce: %w", err)
	}

	// Encrypt
	ciphertext := pc.sendAEAD.Seal(nil, nonce, data, nil)

	// Build final packet
	pkt := make([]byte, 4+1+24+len(ciphertext))
	copy(pkt[0:4], P2PMagic)
	pkt[4] = typ
	copy(pkt[5:29], nonce)
	copy(pkt[29:], ciphertext)

	_, err := pc.conn.WriteToUDP(pkt, pc.remoteAddr)
	if err != nil {
		return fmt.Errorf("p2p: send: %w", err)
	}

	pc.lastSend.Store(time.Now().Unix())
	return nil
}

// ReceiveFrom decrypts a packet received from the peer.
// Returns nil if packet is not from this connection or invalid.
func (pc *P2PConn) ReceiveFrom(data []byte, from *net.UDPAddr) ([]byte, byte, error) {
	// Verify it's from our peer
	if !from.IP.Equal(pc.remoteAddr.IP) || from.Port != pc.remoteAddr.Port {
		return nil, 0, errors.New("p2p: wrong source")
	}

	return pc.Decrypt(data)
}

// Decrypt decrypts a P2P packet.
func (pc *P2PConn) Decrypt(data []byte) ([]byte, byte, error) {
	// Minimum size: magic(4) + type(1) + nonce(24) + tag(16)
	if len(data) < 4+1+24+16 {
		return nil, 0, errors.New("p2p: packet too short")
	}

	// Verify magic
	if data[0] != 'P' || data[1] != '2' || data[2] != 'P' || data[3] != '1' {
		return nil, 0, errors.New("p2p: invalid magic")
	}

	typ := data[4]
	nonce := data[5:29]
	ciphertext := data[29:]

	// Decrypt
	plaintext, err := pc.recvAEAD.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("p2p: decrypt: %w", err)
	}

	pc.lastRecv.Store(time.Now().Unix())
	return plaintext, typ, nil
}

// SendKeepAlive sends a keep-alive packet.
func (pc *P2PConn) SendKeepAlive() error {
	return pc.sendPacket(P2PKeepAlive, nil)
}

// Close closes the connection.
func (pc *P2PConn) Close() error {
	pc.mu.Lock()
	if pc.closed {
		pc.mu.Unlock()
		return nil
	}
	pc.closed = true
	pc.mu.Unlock()

	// Send close packet (best effort)
	_ = pc.sendPacket(P2PClose, nil)
	return nil
}

// RemoteAddr returns the remote UDP address.
func (pc *P2PConn) RemoteAddr() *net.UDPAddr { return pc.remoteAddr }

// RemoteVIP returns the remote virtual IP.
func (pc *P2PConn) RemoteVIP() net.IP { return pc.remoteVIP }

// LastSend returns the timestamp of last send.
func (pc *P2PConn) LastSend() time.Time {
	return time.Unix(pc.lastSend.Load(), 0)
}

// LastRecv returns the timestamp of last receive.
func (pc *P2PConn) LastRecv() time.Time {
	return time.Unix(pc.lastRecv.Load(), 0)
}

// IsP2PPacket checks if the data starts with P2P magic.
func IsP2PPacket(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	return data[0] == 'P' && data[1] == '2' && data[2] == 'P' && data[3] == '1'
}

// DeriveP2PKeys derives send and receive keys from shared secret using HKDF-SHA256.
// The initiator→responder and responder→initiator keys use different HKDF info strings.
func DeriveP2PKeys(sharedSecret []byte, isInitiator bool) (sendKey, recvKey [32]byte, err error) {
	salt := []byte("shuttle-p2p-v1")
	i2r := hkdf.New(sha256.New, sharedSecret, salt, []byte("i2r"))
	r2i := hkdf.New(sha256.New, sharedSecret, salt, []byte("r2i"))

	var i2rKey, r2iKey [32]byte
	if _, err = io.ReadFull(i2r, i2rKey[:]); err != nil {
		return sendKey, recvKey, fmt.Errorf("hkdf i2r: %w", err)
	}
	if _, err = io.ReadFull(r2i, r2iKey[:]); err != nil {
		return sendKey, recvKey, fmt.Errorf("hkdf r2i: %w", err)
	}

	if isInitiator {
		sendKey = i2rKey
		recvKey = r2iKey
	} else {
		sendKey = r2iKey
		recvKey = i2rKey
	}
	return sendKey, recvKey, nil
}

// P2PHandshake performs a Noise-like handshake to establish keys.
type P2PHandshake struct {
	localPriv  [32]byte
	localPub   [32]byte
	remotePub  [32]byte
	sharedKey  [32]byte
	completed  bool
	isInitiator bool
}

// NewP2PHandshake creates a new handshake.
func NewP2PHandshake(localPriv, localPub, remotePub [32]byte, isInitiator bool) *P2PHandshake {
	return &P2PHandshake{
		localPriv:   localPriv,
		localPub:    localPub,
		remotePub:   remotePub,
		isInitiator: isInitiator,
	}
}

// Complete marks the handshake as complete and derives keys.
func (h *P2PHandshake) Complete(sharedSecret []byte) (sendKey, recvKey [32]byte, err error) {
	copy(h.sharedKey[:], sharedSecret)
	h.completed = true
	return DeriveP2PKeys(sharedSecret, h.isInitiator)
}

// IsCompleted returns whether handshake is complete.
func (h *P2PHandshake) IsCompleted() bool {
	return h.completed
}

// HandshakeMessage represents a P2P handshake message.
type HandshakeMessage struct {
	PublicKey [32]byte
	Nonce     [32]byte // Random nonce for replay protection
}

// EncodeHandshakeMessage encodes a handshake message.
func EncodeHandshakeMessage(msg *HandshakeMessage) []byte {
	buf := make([]byte, 64)
	copy(buf[0:32], msg.PublicKey[:])
	copy(buf[32:64], msg.Nonce[:])
	return buf
}

// DecodeHandshakeMessage decodes a handshake message.
func DecodeHandshakeMessage(data []byte) (*HandshakeMessage, error) {
	if len(data) < 64 {
		return nil, errors.New("p2p: handshake message too short")
	}
	msg := &HandshakeMessage{}
	copy(msg.PublicKey[:], data[0:32])
	copy(msg.Nonce[:], data[32:64])
	return msg, nil
}

// GenerateNonce generates a random nonce.
func GenerateNonce() ([32]byte, error) {
	var nonce [32]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nonce, err
	}
	return nonce, nil
}
