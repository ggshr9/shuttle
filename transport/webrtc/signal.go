package webrtc

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"

	"github.com/pion/webrtc/v4"
)

// SignalRequest is the client's POST body for WebRTC signaling.
type SignalRequest struct {
	SDP   string `json:"sdp"`
	Nonce string `json:"nonce"` // hex-encoded 32-byte nonce
	HMAC  string `json:"hmac"`  // hex-encoded HMAC-SHA256(password, nonce)
}

// SignalResponse is the server's response containing the SDP answer.
type SignalResponse struct {
	SDP   string `json:"sdp"`
	Error string `json:"error,omitempty"`
}

// GenerateAuth creates a SignalRequest with HMAC authentication.
func GenerateAuth(password string, sdp string) (*SignalRequest, error) {
	nonce := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write(nonce)
	sig := mac.Sum(nil)

	return &SignalRequest{
		SDP:   sdp,
		Nonce: hex.EncodeToString(nonce),
		HMAC:  hex.EncodeToString(sig),
	}, nil
}

// VerifyAuth checks the HMAC in a SignalRequest against the given password.
// Returns false if the nonce or HMAC is invalid.
func VerifyAuth(req *SignalRequest, password string) ([]byte, bool) {
	nonce, err := hex.DecodeString(req.Nonce)
	if err != nil || len(nonce) != 32 {
		return nil, false
	}
	clientMAC, err := hex.DecodeString(req.HMAC)
	if err != nil {
		return nil, false
	}
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write(nonce)
	expected := mac.Sum(nil)
	if !hmac.Equal(clientMAC, expected) {
		return nil, false
	}
	return nonce, true
}

// mapICEPolicy converts a config string to a webrtc.ICETransportPolicy.
func mapICEPolicy(policy string) webrtc.ICETransportPolicy {
	switch policy {
	case "relay":
		return webrtc.ICETransportPolicyRelay
	default:
		return webrtc.ICETransportPolicyAll
	}
}

// encodeSignalRequest marshals a SignalRequest to JSON.
func encodeSignalRequest(req *SignalRequest) ([]byte, error) {
	return json.Marshal(req)
}

// decodeSignalRequest unmarshals a SignalRequest from JSON.
func decodeSignalRequest(data []byte) (*SignalRequest, error) {
	var req SignalRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}
