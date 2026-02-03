package signal

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Client handles signaling communication on the client side.
type Client struct {
	localVIP net.IP
	stream   io.ReadWriter
	logger   *slog.Logger

	mu        sync.Mutex
	callbacks map[byte][]MessageCallback
	pending   map[string]chan *Message // key: dstVIP string, for waiting responses
}

// MessageCallback is called when a message of the registered type is received.
type MessageCallback func(*Message)

// NewClient creates a new signaling client.
func NewClient(localVIP net.IP, stream io.ReadWriter, logger *slog.Logger) *Client {
	return &Client{
		localVIP:  localVIP,
		stream:    stream,
		logger:    logger,
		callbacks: make(map[byte][]MessageCallback),
		pending:   make(map[string]chan *Message),
	}
}

// OnMessage registers a callback for a specific message type.
func (c *Client) OnMessage(msgType byte, callback MessageCallback) {
	c.mu.Lock()
	c.callbacks[msgType] = append(c.callbacks[msgType], callback)
	c.mu.Unlock()
}

// SendCandidates sends ICE candidates to a peer.
func (c *Client) SendCandidates(dstVIP net.IP, candidates []*CandidateInfo) error {
	msg := NewCandidateMessage(c.localVIP, dstVIP, candidates)
	return c.send(msg)
}

// SendConnect sends a connection request to a peer.
func (c *Client) SendConnect(dstVIP net.IP, publicKey [32]byte) error {
	msg := NewConnectMessage(c.localVIP, dstVIP, publicKey)
	return c.send(msg)
}

// SendConnectAck sends a connection acknowledgment to a peer.
func (c *Client) SendConnectAck(dstVIP net.IP, publicKey [32]byte) error {
	msg := NewConnectAckMessage(c.localVIP, dstVIP, publicKey)
	return c.send(msg)
}

// SendDisconnect notifies a peer of disconnection.
func (c *Client) SendDisconnect(dstVIP net.IP) error {
	msg := NewDisconnectMessage(c.localVIP, dstVIP)
	return c.send(msg)
}

// SendPing sends a ping to a peer.
func (c *Client) SendPing(dstVIP net.IP) error {
	msg := NewPingMessage(c.localVIP, dstVIP)
	return c.send(msg)
}

// send writes a message to the stream.
func (c *Client) send(msg *Message) error {
	data := msg.Encode()

	c.mu.Lock()
	defer c.mu.Unlock()

	_, err := c.stream.Write(data)
	if err != nil {
		return fmt.Errorf("signal: send: %w", err)
	}

	c.logger.Debug("signal: sent",
		"type", msg.Type,
		"dst", msg.DstVIP,
		"len", len(msg.Payload))

	return nil
}

// WaitForConnect waits for a ConnectAck from a specific peer.
func (c *Client) WaitForConnect(ctx context.Context, dstVIP net.IP, timeout time.Duration) (*Message, error) {
	key := dstVIP.String()

	ch := make(chan *Message, 1)
	c.mu.Lock()
	c.pending[key] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, key)
		c.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case msg := <-ch:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Run starts the message receive loop.
func (c *Client) Run(ctx context.Context) error {
	buf := make([]byte, HeaderSize+MaxSignalPayload)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Read header first
		n, err := io.ReadFull(c.stream, buf[:HeaderSize])
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("signal: read header: %w", err)
		}

		// Get payload length
		payloadLen := int(buf[9])<<8 | int(buf[10])
		if payloadLen > MaxSignalPayload {
			c.logger.Warn("signal: payload too large", "len", payloadLen)
			continue
		}

		// Read payload
		if payloadLen > 0 {
			_, err := io.ReadFull(c.stream, buf[HeaderSize:HeaderSize+payloadLen])
			if err != nil {
				return fmt.Errorf("signal: read payload: %w", err)
			}
			n += payloadLen
		}

		// Decode message
		msg, err := Decode(buf[:n])
		if err != nil {
			c.logger.Debug("signal: decode error", "err", err)
			continue
		}

		c.logger.Debug("signal: received",
			"type", msg.Type,
			"src", msg.SrcVIP,
			"len", len(msg.Payload))

		// Handle pending responses
		if msg.Type == SignalConnectAck {
			key := msg.SrcVIP.String()
			c.mu.Lock()
			if ch, ok := c.pending[key]; ok {
				select {
				case ch <- msg:
				default:
				}
			}
			c.mu.Unlock()
		}

		// Call registered callbacks
		c.mu.Lock()
		callbacks := c.callbacks[msg.Type]
		c.mu.Unlock()

		for _, cb := range callbacks {
			go cb(msg)
		}
	}
}

// ExchangeCandidates performs a candidate exchange with a peer.
// Returns the remote candidates.
func (c *Client) ExchangeCandidates(ctx context.Context, dstVIP net.IP, localCandidates []*CandidateInfo, timeout time.Duration) ([]*CandidateInfo, error) {
	// Set up callback to receive remote candidates
	remoteCh := make(chan []*CandidateInfo, 1)

	c.OnMessage(SignalCandidate, func(msg *Message) {
		if !msg.SrcVIP.Equal(dstVIP) {
			return
		}

		candidates, err := DecodeCandidates(msg.Payload)
		if err != nil {
			c.logger.Debug("signal: decode candidates failed", "err", err)
			return
		}

		select {
		case remoteCh <- candidates:
		default:
		}
	})

	// Send our candidates
	if err := c.SendCandidates(dstVIP, localCandidates); err != nil {
		return nil, err
	}

	// Wait for remote candidates
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case remote := <-remoteCh:
		return remote, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Handshake performs the full connection handshake with a peer.
// This includes candidate exchange and connection establishment.
func (c *Client) Handshake(ctx context.Context, dstVIP net.IP, localPublicKey [32]byte, localCandidates []*CandidateInfo, timeout time.Duration) (*HandshakeResult, error) {
	result := &HandshakeResult{}

	// Exchange candidates
	remoteCandidates, err := c.ExchangeCandidates(ctx, dstVIP, localCandidates, timeout)
	if err != nil {
		return nil, fmt.Errorf("signal: candidate exchange: %w", err)
	}
	result.RemoteCandidates = remoteCandidates

	// Send connect request
	if err := c.SendConnect(dstVIP, localPublicKey); err != nil {
		return nil, fmt.Errorf("signal: send connect: %w", err)
	}

	// Wait for ConnectAck
	ackMsg, err := c.WaitForConnect(ctx, dstVIP, timeout)
	if err != nil {
		return nil, fmt.Errorf("signal: wait connect ack: %w", err)
	}

	// Decode remote public key
	connectInfo, err := DecodeConnectInfo(ackMsg.Payload)
	if err != nil {
		return nil, fmt.Errorf("signal: decode connect info: %w", err)
	}
	result.RemotePublicKey = connectInfo.PublicKey

	return result, nil
}

// HandshakeResult contains the result of a signaling handshake.
type HandshakeResult struct {
	RemoteCandidates []*CandidateInfo
	RemotePublicKey  [32]byte
}

// RespondToHandshake responds to an incoming connection request.
func (c *Client) RespondToHandshake(ctx context.Context, srcVIP net.IP, localPublicKey [32]byte, localCandidates []*CandidateInfo) error {
	// Send our candidates
	if err := c.SendCandidates(srcVIP, localCandidates); err != nil {
		return fmt.Errorf("signal: send candidates: %w", err)
	}

	// Send ConnectAck
	if err := c.SendConnectAck(srcVIP, localPublicKey); err != nil {
		return fmt.Errorf("signal: send connect ack: %w", err)
	}

	return nil
}
