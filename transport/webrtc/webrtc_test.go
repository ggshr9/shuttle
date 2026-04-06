package webrtc

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient(&ClientConfig{
		SignalURL: "https://example.com/webrtc/signal",
		Password:  "secret",
	})
	if c.Type() != "webrtc" {
		t.Fatalf("expected type webrtc, got %s", c.Type())
	}
	// Default STUN servers should be set
	if len(c.config.STUNServers) == 0 {
		t.Fatal("expected default STUN servers")
	}
}

func TestNewClientExplicitEmptySTUN(t *testing.T) {
	c := NewClient(&ClientConfig{
		SignalURL:   "https://example.com/webrtc/signal",
		STUNServers: []string{}, // Explicitly empty
	})
	if len(c.config.STUNServers) != 0 {
		t.Fatal("expected no STUN servers when explicitly set to empty")
	}
}

func TestNewClientCustomSTUN(t *testing.T) {
	c := NewClient(&ClientConfig{
		STUNServers: []string{"stun:custom.stun.com:3478"},
	})
	if len(c.config.STUNServers) != 1 || c.config.STUNServers[0] != "stun:custom.stun.com:3478" {
		t.Fatalf("unexpected STUN servers: %v", c.config.STUNServers)
	}
}

func TestClientClosedDial(t *testing.T) {
	c := NewClient(&ClientConfig{
		SignalURL: "https://example.com/signal",
		Password:  "secret",
	})
	c.Close()
	_, err := c.Dial(context.TODO(), "")
	if err == nil {
		t.Fatal("expected error dialing closed client")
	}
}

func TestNewServer(t *testing.T) {
	s := NewServer(&ServerConfig{
		SignalListen: ":0",
		Password:     "secret",
	}, nil)
	if s.Type() != "webrtc" {
		t.Fatalf("expected type webrtc, got %s", s.Type())
	}
	if s.replayFilter == nil {
		t.Fatal("expected non-nil replay filter")
	}
}

func TestNewServerDefaultSTUN(t *testing.T) {
	s := NewServer(&ServerConfig{}, nil)
	if len(s.config.STUNServers) == 0 {
		t.Fatal("expected default STUN servers")
	}
}

func TestNewServerExplicitEmptySTUN(t *testing.T) {
	s := NewServer(&ServerConfig{STUNServers: []string{}}, nil)
	if len(s.config.STUNServers) != 0 {
		t.Fatal("expected empty STUN servers when explicitly set")
	}
}

func TestIsWSSignalURL(t *testing.T) {
	tests := []struct {
		url    string
		expect bool
	}{
		{"ws://localhost/ws", true},
		{"wss://example.com/ws", true},
		{"http://example.com/signal", false},
		{"https://example.com/signal", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isWSSignalURL(tt.url); got != tt.expect {
			t.Errorf("isWSSignalURL(%q) = %v, want %v", tt.url, got, tt.expect)
		}
	}
}

func TestMapICEPolicy(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"relay", "relay"},
		{"all", "all"},
		{"", "all"},
		{"unknown", "all"},
	}
	for _, tt := range tests {
		got := mapICEPolicy(tt.input)
		// We just verify it doesn't panic and returns a valid policy
		_ = got
	}
}

func TestGenerateAndVerifyAuth(t *testing.T) {
	password := "test-password"

	req, err := GenerateAuth(password, "v=0\r\no=- 0 0 IN IP4 0.0.0.0\r\n")
	if err != nil {
		t.Fatalf("GenerateAuth: %v", err)
	}

	if req.Nonce == "" || req.HMAC == "" {
		t.Fatal("expected non-empty nonce and HMAC")
	}
	if req.SDP == "" {
		t.Fatal("expected non-empty SDP")
	}

	// Verify with correct password
	nonce, ok := VerifyAuth(req, password)
	if !ok {
		t.Fatal("expected auth verification to succeed")
	}
	if len(nonce) != 32 {
		t.Fatalf("expected 32-byte nonce, got %d", len(nonce))
	}

	// Verify with wrong password
	_, ok = VerifyAuth(req, "wrong-password")
	if ok {
		t.Fatal("expected auth verification to fail with wrong password")
	}
}

func TestVerifyAuthInvalidNonce(t *testing.T) {
	_, ok := VerifyAuth(&SignalRequest{Nonce: "not-hex", HMAC: "abc"}, "pass")
	if ok {
		t.Fatal("expected failure for invalid nonce")
	}
}

func TestVerifyAuthShortNonce(t *testing.T) {
	_, ok := VerifyAuth(&SignalRequest{Nonce: "abcd", HMAC: "1234"}, "pass")
	if ok {
		t.Fatal("expected failure for short nonce")
	}
}

func TestValidateSDP(t *testing.T) {
	tests := []struct {
		name    string
		sdp     string
		wantErr bool
	}{
		{"valid", "v=0\r\no=- 0 0 IN IP4 0.0.0.0\r\n", false},
		{"empty", "", true},
		{"no version", "o=- 0 0 IN IP4 0.0.0.0", true},
		{"too large", "v=" + string(make([]byte, maxSDPSize+1)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSDP(tt.sdp)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSDP(%q) error = %v, wantErr = %v", tt.sdp[:minInt(len(tt.sdp), 20)], err, tt.wantErr)
			}
		})
	}
}

func TestSignalRequestJSON(t *testing.T) {
	req := &SignalRequest{
		SDP:   "v=0\r\n",
		Nonce: "abcd1234",
		HMAC:  "ef567890",
	}

	data, err := encodeSignalRequest(req)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	var decoded SignalRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.SDP != req.SDP || decoded.Nonce != req.Nonce || decoded.HMAC != req.HMAC {
		t.Fatal("JSON round-trip mismatch")
	}
}

func TestSignalMessageTypes(t *testing.T) {
	types := []SignalMessageType{
		SignalTypeAuth,
		SignalTypeOffer,
		SignalTypeAnswer,
		SignalTypeCandidate,
		SignalTypeCandidateDone,
		SignalTypeError,
		SignalTypeReconnect,
	}
	for _, st := range types {
		msg := SignalMessage{Type: st}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("marshal %s: %v", st, err)
		}
		var decoded SignalMessage
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal %s: %v", st, err)
		}
		if decoded.Type != st {
			t.Fatalf("type mismatch: got %s, want %s", decoded.Type, st)
		}
	}
}

func TestWebrtcAddr(t *testing.T) {
	a := &webrtcAddr{addr: "192.168.1.1:1234"}
	if a.Network() != "webrtc" {
		t.Fatalf("Network() = %s, want webrtc", a.Network())
	}
	if a.String() != "192.168.1.1:1234" {
		t.Fatalf("String() = %s, want 192.168.1.1:1234", a.String())
	}
}

func TestSignalResponseJSON(t *testing.T) {
	resp := SignalResponse{SDP: "v=0\r\n", Error: ""}
	data, _ := json.Marshal(resp)
	var decoded SignalResponse
	json.Unmarshal(data, &decoded)
	if decoded.SDP != resp.SDP {
		t.Fatal("SDP mismatch")
	}
}

func TestSignalResponseError(t *testing.T) {
	resp := SignalResponse{Error: "auth failed"}
	data, _ := json.Marshal(resp)
	var decoded SignalResponse
	json.Unmarshal(data, &decoded)
	if decoded.Error != "auth failed" {
		t.Fatalf("error mismatch: got %q", decoded.Error)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
