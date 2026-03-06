package webrtc

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pion/webrtc/v4"
	"github.com/shuttle-proxy/shuttle/transport/auth"
)

const maxSDPSize = 64 * 1024 // 64 KB

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
	payload, err := auth.GenerateHMAC(password)
	if err != nil {
		return nil, err
	}
	return &SignalRequest{
		SDP:   sdp,
		Nonce: hex.EncodeToString(payload[:32]),
		HMAC:  hex.EncodeToString(payload[32:]),
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
	if !auth.VerifyHMAC(nonce, clientMAC, password) {
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

// decodeSignalRequest unmarshals a SignalRequest from JSON and validates the SDP.
func decodeSignalRequest(data []byte) (*SignalRequest, error) {
	var req SignalRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	if err := validateSDP(req.SDP); err != nil {
		return nil, fmt.Errorf("invalid SDP: %w", err)
	}
	return &req, nil
}

// validateSDP performs basic validation on an SDP string.
func validateSDP(sdp string) error {
	if len(sdp) > maxSDPSize {
		return fmt.Errorf("SDP too large: %d bytes (max %d)", len(sdp), maxSDPSize)
	}
	if sdp == "" {
		return fmt.Errorf("SDP is empty")
	}
	if !strings.HasPrefix(sdp, "v=") {
		return fmt.Errorf("SDP missing version line")
	}
	return nil
}
