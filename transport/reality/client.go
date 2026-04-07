package reality

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/crypto"
	"github.com/shuttleX/shuttle/transport"
	ymux "github.com/shuttleX/shuttle/transport/mux/yamux"
)

// ClientConfig holds configuration for a Reality client transport.
type ClientConfig struct {
	ServerAddr  string
	ServerName  string
	ShortID     string
	PublicKey   string
	Password    string
	PostQuantum bool                // Enable hybrid X25519 + ML-KEM-768 key exchange
	Yamux       *config.YamuxConfig // optional yamux tuning
}

// Client implements transport.ClientTransport using Reality (TLS + Noise IK + yamux).
type Client struct {
	config *ClientConfig
	closed atomic.Bool
}

// NewClient creates a new Reality client transport.
func NewClient(cfg *ClientConfig) *Client {
	return &Client{config: cfg}
}

// Type returns the transport type identifier.
func (c *Client) Type() string { return "reality" }

// Dial establishes a Reality connection to the given address.
func (c *Client) Dial(ctx context.Context, addr string) (transport.Connection, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("reality client closed")
	}
	if addr == "" {
		addr = c.config.ServerAddr
	}

	// Step 1: TLS dial with SNI impersonation
	tlsConf := &tls.Config{
		ServerName:         c.config.ServerName,
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2", "http/1.1"},
		MinVersion:         tls.VersionTLS13,
	}
	dialer := &tls.Dialer{Config: tlsConf}
	raw, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("reality dial: %w", err)
	}

	// Ensure raw is closed on any error below.
	success := false
	defer func() {
		if !success {
			raw.Close()
		}
	}()

	// Step 2: Noise IK handshake
	localPub, localPriv, err := crypto.DeriveKeysFromPassword(c.config.Password)
	if err != nil {
		return nil, fmt.Errorf("derive keys: %w", err)
	}
	var remotePub [32]byte
	if c.config.PublicKey != "" {
		pubBytes, err := hex.DecodeString(c.config.PublicKey)
		if err != nil || len(pubBytes) != 32 {
			return nil, fmt.Errorf("invalid server public key: expected 64-char hex, got %d chars", len(c.config.PublicKey))
		}
		copy(remotePub[:], pubBytes)
	}

	hs, err := crypto.NewInitiator(localPriv, localPub, remotePub)
	if err != nil {
		return nil, fmt.Errorf("noise init: %w", err)
	}

	// Send handshake message 1 (-> e, es, s, ss)
	msg1, err := hs.WriteMessage(nil)
	if err != nil {
		return nil, fmt.Errorf("noise write msg1: %w", err)
	}
	if err := writeFrame(raw, msg1); err != nil {
		return nil, fmt.Errorf("send msg1: %w", err)
	}

	// Read handshake message 2 (<- e, ee, se)
	msg2, err := readFrame(raw)
	if err != nil {
		return nil, fmt.Errorf("read msg2: %w", err)
	}
	_, err = hs.ReadMessage(msg2)
	if err != nil {
		return nil, fmt.Errorf("noise read msg2: %w", err)
	}

	if !hs.Completed() {
		return nil, fmt.Errorf("noise handshake incomplete")
	}

	// Step 2.5: Post-quantum hybrid KEM exchange (optional, after Noise IK)
	// This is backward compatible: classical clients skip this step entirely.
	if c.config.PostQuantum {
		pq, err := crypto.NewPQHandshake()
		if err != nil {
			return nil, fmt.Errorf("pq handshake init: %w", err)
		}

		// Send version byte + PQ public key to server
		pqPub := pq.PublicKeyBytes()
		pqMsg := make([]byte, 1+len(pqPub))
		pqMsg[0] = crypto.HandshakeVersionHybridPQ
		copy(pqMsg[1:], pqPub)
		if err := writeFrame(raw, pqMsg); err != nil {
			return nil, fmt.Errorf("send pq public key: %w", err)
		}

		// Read PQ ciphertext from server
		pqCiphertext, err := readFrame(raw)
		if err != nil {
			return nil, fmt.Errorf("read pq ciphertext: %w", err)
		}

		// Decapsulate to derive PQ shared secret
		pqSecret, err := pq.Decapsulate(pqCiphertext)
		if err != nil {
			return nil, fmt.Errorf("pq decapsulate: %w", err)
		}

		// Mix PQ shared secret into connection by wrapping with HKDF-derived key.
		// This ensures harvest-now-decrypt-later attacks require breaking BOTH
		// X25519 AND the PQ exchange.
		raw, err = wrapConnWithPQ(raw, pqSecret)
		if err != nil {
			return nil, fmt.Errorf("pq wrap: %w", err)
		}
	}

	// Step 3: yamux multiplexed session over the TLS connection
	mux := ymux.New(c.config.Yamux)
	muxConn, err := mux.Client(raw)
	if err != nil {
		return nil, fmt.Errorf("yamux client: %w", err)
	}

	success = true
	return muxConn, nil
}

// Close shuts down the client transport.
func (c *Client) Close() error {
	c.closed.Store(true)
	return nil
}

// writeFrame writes a length-prefixed frame: [2-byte big-endian length][payload].
func writeFrame(w io.Writer, data []byte) error {
	header := make([]byte, 2)
	binary.BigEndian.PutUint16(header, uint16(len(data)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// readFrame reads a length-prefixed frame from the reader.
func readFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint16(header)
	const maxFrameSize = 64 * 1024 // 64 KB
	if int(length) > maxFrameSize {
		return nil, fmt.Errorf("reality frame too large: %d bytes (max %d)", length, maxFrameSize)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return data, nil
}

// Compile-time interface check.
var _ transport.ClientTransport = (*Client)(nil)
