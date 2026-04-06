package webrtc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/coder/websocket"
)

// SignalMessageType identifies the type of a WebSocket signaling message.
type SignalMessageType string

const (
	SignalTypeAuth          SignalMessageType = "auth"
	SignalTypeOffer         SignalMessageType = "offer"
	SignalTypeAnswer        SignalMessageType = "answer"
	SignalTypeCandidate     SignalMessageType = "candidate"
	SignalTypeCandidateDone SignalMessageType = "candidate_done"
	SignalTypeError         SignalMessageType = "error"
	SignalTypeReconnect     SignalMessageType = "reconnect"
)

// SignalMessage is the JSON envelope for all WebSocket signaling messages.
type SignalMessage struct {
	Type      SignalMessageType `json:"type"`
	SDP       string           `json:"sdp,omitempty"`
	Candidate *ICECandidateMsg `json:"candidate,omitempty"`
	Nonce     string           `json:"nonce,omitempty"`
	HMAC      string           `json:"hmac,omitempty"`
	Error     string           `json:"error,omitempty"`
}

// ICECandidateMsg wraps an ICE candidate for signaling.
type ICECandidateMsg struct {
	Candidate        string  `json:"candidate"`
	SDPMid           *string `json:"sdpMid,omitempty"`
	SDPMLineIndex    *uint16 `json:"sdpMLineIndex,omitempty"`
	UsernameFragment *string `json:"usernameFragment,omitempty"`
}

const wsReadLimit = 1 << 20 // 1 MB

// sendWSMessage marshals and sends a SignalMessage over a WebSocket.
func sendWSMessage(ctx context.Context, conn *websocket.Conn, msg *SignalMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal signal: %w", err)
	}
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, data)
}

// readWSMessage reads and unmarshals a SignalMessage from a WebSocket.
func readWSMessage(ctx context.Context, conn *websocket.Conn) (*SignalMessage, error) {
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, data, err := conn.Read(readCtx)
	if err != nil {
		return nil, fmt.Errorf("read ws: %w", err)
	}
	var msg SignalMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal signal: %w", err)
	}
	return &msg, nil
}
