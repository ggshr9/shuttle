package webrtc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Stats tests
// ---------------------------------------------------------------------------

func TestConnStatsZeroValue(t *testing.T) {
	var s ConnStats
	if s.RTT != 0 {
		t.Fatalf("expected zero RTT, got %v", s.RTT)
	}
	if s.PacketsSent != 0 || s.PacketsRecv != 0 {
		t.Fatal("expected zero packet counts")
	}
	if s.BytesSent != 0 || s.BytesRecv != 0 {
		t.Fatal("expected zero byte counts")
	}
	if s.PacketsLost != 0 {
		t.Fatalf("expected zero packets lost, got %d", s.PacketsLost)
	}
	if s.CandidateLocal != "" || s.CandidateType != "" {
		t.Fatal("expected empty candidate fields")
	}
}

func TestStatsCollectorCloseIdempotent(t *testing.T) {
	done := make(chan struct{})
	sc := &statsCollector{
		done: done,
	}

	// Close should not panic even when called multiple times.
	sc.Close()
	sc.Close()
	sc.Close()

	// Verify the done channel is closed.
	select {
	case <-sc.done:
	default:
		t.Fatal("expected done channel to be closed")
	}
}

func TestStatsCollectorStatsReturnsLastCollected(t *testing.T) {
	sc := &statsCollector{
		done: make(chan struct{}),
		stats: ConnStats{
			RTT:            42 * time.Millisecond,
			PacketsSent:    100,
			PacketsRecv:    200,
			BytesSent:      3000,
			BytesRecv:      4000,
			PacketsLost:    5,
			CandidateLocal: "192.168.1.1:5000",
			CandidateType:  "host",
		},
	}

	got := sc.Stats()
	if got.RTT != 42*time.Millisecond {
		t.Fatalf("RTT = %v, want 42ms", got.RTT)
	}
	if got.PacketsSent != 100 {
		t.Fatalf("PacketsSent = %d, want 100", got.PacketsSent)
	}
	if got.PacketsRecv != 200 {
		t.Fatalf("PacketsRecv = %d, want 200", got.PacketsRecv)
	}
	if got.BytesSent != 3000 {
		t.Fatalf("BytesSent = %d, want 3000", got.BytesSent)
	}
	if got.BytesRecv != 4000 {
		t.Fatalf("BytesRecv = %d, want 4000", got.BytesRecv)
	}
	if got.PacketsLost != 5 {
		t.Fatalf("PacketsLost = %d, want 5", got.PacketsLost)
	}
	if got.CandidateLocal != "192.168.1.1:5000" {
		t.Fatalf("CandidateLocal = %q", got.CandidateLocal)
	}
	if got.CandidateType != "host" {
		t.Fatalf("CandidateType = %q", got.CandidateType)
	}
}

func TestStatsCollectorConcurrentAccess(t *testing.T) {
	sc := &statsCollector{
		done: make(chan struct{}),
	}

	var wg sync.WaitGroup
	// Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			sc.mu.Lock()
			sc.stats = ConnStats{
				PacketsSent: uint64(n),
				PacketsRecv: uint64(n * 2),
			}
			sc.mu.Unlock()
		}(i)
	}
	// Readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sc.Stats()
		}()
	}
	wg.Wait()
}

func TestStatsCollectorLoopStopsOnClose(t *testing.T) {
	done := make(chan struct{})
	sc := &statsCollector{
		done: done,
	}

	// Run a goroutine that mimics the loop select but exits on done.
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-sc.done:
				return
			case <-ticker.C:
			}
		}
	}()

	sc.Close()

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not stop after Close")
	}
}

// ---------------------------------------------------------------------------
// Signal message tests (ws_signal.go types)
// ---------------------------------------------------------------------------

func TestSignalMessageAuthJSON(t *testing.T) {
	msg := SignalMessage{
		Type:  SignalTypeAuth,
		Nonce: "aabbccdd",
		HMAC:  "11223344",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded SignalMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Type != SignalTypeAuth {
		t.Fatalf("type = %s, want auth", decoded.Type)
	}
	if decoded.Nonce != "aabbccdd" {
		t.Fatalf("nonce = %q", decoded.Nonce)
	}
	if decoded.HMAC != "11223344" {
		t.Fatalf("hmac = %q", decoded.HMAC)
	}
	// Optional fields should be empty/nil
	if decoded.SDP != "" {
		t.Fatalf("sdp should be empty, got %q", decoded.SDP)
	}
	if decoded.Candidate != nil {
		t.Fatal("candidate should be nil")
	}
}

func TestSignalMessageOfferJSON(t *testing.T) {
	msg := SignalMessage{
		Type: SignalTypeOffer,
		SDP:  "v=0\r\nsome-sdp-data\r\n",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded SignalMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Type != SignalTypeOffer {
		t.Fatalf("type = %s, want offer", decoded.Type)
	}
	if decoded.SDP != "v=0\r\nsome-sdp-data\r\n" {
		t.Fatalf("sdp mismatch")
	}
}

func TestSignalMessageCandidateJSON(t *testing.T) {
	mid := "0"
	idx := uint16(0)
	frag := "ufrag"
	msg := SignalMessage{
		Type: SignalTypeCandidate,
		Candidate: &ICECandidateMsg{
			Candidate:        "candidate:1 1 UDP 2130706431 192.168.1.1 5000 typ host",
			SDPMid:           &mid,
			SDPMLineIndex:    &idx,
			UsernameFragment: &frag,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded SignalMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Type != SignalTypeCandidate {
		t.Fatalf("type = %s, want candidate", decoded.Type)
	}
	if decoded.Candidate == nil {
		t.Fatal("candidate should not be nil")
	}
	if decoded.Candidate.Candidate != msg.Candidate.Candidate {
		t.Fatalf("candidate string mismatch")
	}
	if *decoded.Candidate.SDPMid != "0" {
		t.Fatalf("SDPMid = %q, want 0", *decoded.Candidate.SDPMid)
	}
	if *decoded.Candidate.SDPMLineIndex != 0 {
		t.Fatalf("SDPMLineIndex = %d, want 0", *decoded.Candidate.SDPMLineIndex)
	}
	if *decoded.Candidate.UsernameFragment != "ufrag" {
		t.Fatalf("UsernameFragment = %q, want ufrag", *decoded.Candidate.UsernameFragment)
	}
}

func TestSignalMessageCandidateDoneJSON(t *testing.T) {
	msg := SignalMessage{Type: SignalTypeCandidateDone}
	data, _ := json.Marshal(msg)
	var decoded SignalMessage
	json.Unmarshal(data, &decoded)
	if decoded.Type != SignalTypeCandidateDone {
		t.Fatalf("type = %s, want candidate_done", decoded.Type)
	}
}

func TestSignalMessageErrorJSON(t *testing.T) {
	msg := SignalMessage{
		Type:  SignalTypeError,
		Error: "something went wrong",
	}
	data, _ := json.Marshal(msg)
	var decoded SignalMessage
	json.Unmarshal(data, &decoded)
	if decoded.Type != SignalTypeError {
		t.Fatalf("type = %s, want error", decoded.Type)
	}
	if decoded.Error != "something went wrong" {
		t.Fatalf("error = %q", decoded.Error)
	}
}

func TestSignalMessageReconnectJSON(t *testing.T) {
	msg := SignalMessage{Type: SignalTypeReconnect}
	data, _ := json.Marshal(msg)
	var decoded SignalMessage
	json.Unmarshal(data, &decoded)
	if decoded.Type != SignalTypeReconnect {
		t.Fatalf("type = %s, want reconnect", decoded.Type)
	}
}

func TestSignalMessageOmitsEmptyFields(t *testing.T) {
	msg := SignalMessage{Type: SignalTypeCandidateDone}
	data, _ := json.Marshal(msg)

	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	// Only "type" should be present; sdp, candidate, nonce, hmac, error should be omitted.
	if _, ok := raw["sdp"]; ok {
		t.Fatal("sdp should be omitted when empty")
	}
	if _, ok := raw["candidate"]; ok {
		t.Fatal("candidate should be omitted when nil")
	}
	if _, ok := raw["nonce"]; ok {
		t.Fatal("nonce should be omitted when empty")
	}
	if _, ok := raw["hmac"]; ok {
		t.Fatal("hmac should be omitted when empty")
	}
	if _, ok := raw["error"]; ok {
		t.Fatal("error should be omitted when empty")
	}
}

func TestICECandidateMsgNilOptionalFields(t *testing.T) {
	msg := ICECandidateMsg{
		Candidate: "candidate:1 1 UDP 2130706431 10.0.0.1 5000 typ host",
	}
	data, _ := json.Marshal(msg)
	var decoded ICECandidateMsg
	json.Unmarshal(data, &decoded)
	if decoded.SDPMid != nil {
		t.Fatal("SDPMid should be nil")
	}
	if decoded.SDPMLineIndex != nil {
		t.Fatal("SDPMLineIndex should be nil")
	}
	if decoded.UsernameFragment != nil {
		t.Fatal("UsernameFragment should be nil")
	}
}

// ---------------------------------------------------------------------------
// Connection wrapper tests (connection.go)
// ---------------------------------------------------------------------------

func TestWebrtcAddrNetwork(t *testing.T) {
	a := &webrtcAddr{addr: "10.0.0.1:9999"}
	if a.Network() != "webrtc" {
		t.Fatalf("Network() = %s, want webrtc", a.Network())
	}
}

func TestWebrtcAddrStringEmpty(t *testing.T) {
	a := &webrtcAddr{addr: ""}
	if a.String() != "" {
		t.Fatalf("String() = %q, want empty", a.String())
	}
}

func TestWsCloserDelegates(t *testing.T) {
	var called bool
	wsc := &wsCloser{closeFn: func() error {
		called = true
		return nil
	}}
	if err := wsc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !called {
		t.Fatal("expected closeFn to be called")
	}
}

func TestWsCloserPropagatesError(t *testing.T) {
	expected := errors.New("close error")
	wsc := &wsCloser{closeFn: func() error {
		return expected
	}}
	if err := wsc.Close(); !errors.Is(err, expected) {
		t.Fatalf("Close error = %v, want %v", err, expected)
	}
}

// mockRWC implements datachanelRWC for testing dcReadWriteCloser.
type mockRWC struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	closed   atomic.Bool
	closeErr error
}

func newMockRWC(readData []byte) *mockRWC {
	return &mockRWC{
		readBuf:  bytes.NewBuffer(readData),
		writeBuf: &bytes.Buffer{},
	}
}

func (m *mockRWC) Read(p []byte) (int, error) {
	return m.readBuf.Read(p)
}

func (m *mockRWC) Write(p []byte) (int, error) {
	return m.writeBuf.Write(p)
}

func (m *mockRWC) Close() error {
	m.closed.Store(true)
	return m.closeErr
}

func TestDCReadWriteCloserRead(t *testing.T) {
	mock := newMockRWC([]byte("hello"))
	dc := &dcReadWriteCloser{rwc: mock}

	buf := make([]byte, 5)
	n, err := dc.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n != 5 || string(buf) != "hello" {
		t.Fatalf("Read = %d %q, want 5 hello", n, buf)
	}
}

func TestDCReadWriteCloserWrite(t *testing.T) {
	mock := newMockRWC(nil)
	dc := &dcReadWriteCloser{rwc: mock}

	n, err := dc.Write([]byte("world"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 5 {
		t.Fatalf("Write = %d, want 5", n)
	}
	if mock.writeBuf.String() != "world" {
		t.Fatalf("writeBuf = %q, want world", mock.writeBuf.String())
	}
}

func TestDCReadWriteCloserCloseOnce(t *testing.T) {
	mock := newMockRWC(nil)
	dc := &dcReadWriteCloser{rwc: mock}

	if err := dc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !mock.closed.Load() {
		t.Fatal("expected underlying rwc to be closed")
	}
}

func TestDCReadWriteCloserCloseIdempotent(t *testing.T) {
	closeCount := 0
	mock := &mockRWC{
		readBuf:  &bytes.Buffer{},
		writeBuf: &bytes.Buffer{},
	}
	// Override Close to count calls.
	origClose := mock.Close
	_ = origClose
	// Use a wrapper to count.
	countingRWC := &countingCloseRWC{inner: mock}
	dc := &dcReadWriteCloser{rwc: countingRWC}

	dc.Close()
	dc.Close()
	dc.Close()

	if countingRWC.count != 1 {
		t.Fatalf("Close called %d times, want 1", closeCount)
	}
}

type countingCloseRWC struct {
	inner datachanelRWC
	count int
}

func (c *countingCloseRWC) Read(p []byte) (int, error)  { return c.inner.Read(p) }
func (c *countingCloseRWC) Write(p []byte) (int, error) { return c.inner.Write(p) }
func (c *countingCloseRWC) Close() error {
	c.count++
	return c.inner.Close()
}

func TestDCReadWriteCloserClosePropagatesError(t *testing.T) {
	expected := errors.New("dc close error")
	mock := newMockRWC(nil)
	mock.closeErr = expected
	dc := &dcReadWriteCloser{rwc: mock}

	err := dc.Close()
	if !errors.Is(err, expected) {
		t.Fatalf("Close error = %v, want %v", err, expected)
	}
}

func TestWebrtcConnectionStatsNilCollector(t *testing.T) {
	conn := &webrtcConnection{
		sc: nil,
	}
	stats := conn.Stats()
	if stats.RTT != 0 || stats.PacketsSent != 0 {
		t.Fatal("expected zero stats when collector is nil")
	}
}

func TestWebrtcConnectionLocalRemoteAddr(t *testing.T) {
	local := &webrtcAddr{addr: "local-addr"}
	remote := &webrtcAddr{addr: "remote-addr"}
	conn := &webrtcConnection{
		local:  local,
		remote: remote,
	}
	if conn.LocalAddr().String() != "local-addr" {
		t.Fatalf("LocalAddr = %s", conn.LocalAddr())
	}
	if conn.RemoteAddr().String() != "remote-addr" {
		t.Fatalf("RemoteAddr = %s", conn.RemoteAddr())
	}
	if conn.LocalAddr().Network() != "webrtc" {
		t.Fatalf("LocalAddr.Network = %s", conn.LocalAddr().Network())
	}
}

// ---------------------------------------------------------------------------
// Reconnect tests
// ---------------------------------------------------------------------------

func TestReconnectConstants(t *testing.T) {
	if reconnectBaseDelay != 1*time.Second {
		t.Fatalf("reconnectBaseDelay = %v, want 1s", reconnectBaseDelay)
	}
	if reconnectMaxDelay != 30*time.Second {
		t.Fatalf("reconnectMaxDelay = %v, want 30s", reconnectMaxDelay)
	}
	if reconnectMaxRetry != 5 {
		t.Fatalf("reconnectMaxRetry = %d, want 5", reconnectMaxRetry)
	}
}

func TestReconnectorExponentialBackoff(t *testing.T) {
	// Verify backoff delay calculation: baseDelay << attempt, capped at maxDelay.
	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 30 * time.Second}, // capped at maxDelay
		{10, 30 * time.Second},
	}

	for _, tt := range tests {
		delay := reconnectBaseDelay << uint(tt.attempt)
		if delay > reconnectMaxDelay {
			delay = reconnectMaxDelay
		}
		if delay != tt.expected {
			t.Errorf("backoff(attempt=%d) = %v, want %v", tt.attempt, delay, tt.expected)
		}
	}
}

func TestReconnectorDoesNotExceedMaxRetries(t *testing.T) {
	r := &reconnector{
		attempts: reconnectMaxRetry,
	}

	// trigger should return immediately without setting active.
	r.trigger()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active {
		t.Fatal("should not be active when max retries exceeded")
	}
	if r.attempts != reconnectMaxRetry {
		t.Fatalf("attempts = %d, want %d", r.attempts, reconnectMaxRetry)
	}
}

func TestReconnectorSkipsIfAlreadyActive(t *testing.T) {
	r := &reconnector{
		active: true,
	}

	r.trigger()

	r.mu.Lock()
	defer r.mu.Unlock()
	// attempts should not have been incremented since the active guard prevented entry.
	if r.attempts != 0 {
		t.Fatalf("attempts = %d, want 0", r.attempts)
	}
}

func TestNewReconnector(t *testing.T) {
	client := &Client{config: &ClientConfig{}}
	conn := &webrtcConnection{}

	r := newReconnector(client, conn, nil)
	if r.client != client {
		t.Fatal("client not set")
	}
	if r.conn != conn {
		t.Fatal("conn not set")
	}
	if r.active {
		t.Fatal("should not be active initially")
	}
	if r.attempts != 0 {
		t.Fatalf("attempts = %d, want 0", r.attempts)
	}
}

// ---------------------------------------------------------------------------
// Signal encoding/decoding (signal.go)
// ---------------------------------------------------------------------------

func TestDecodeSignalRequestValid(t *testing.T) {
	req := &SignalRequest{
		SDP:   "v=0\r\ntest sdp\r\n",
		Nonce: "aabb",
		HMAC:  "ccdd",
	}
	data, _ := json.Marshal(req)

	decoded, err := decodeSignalRequest(data)
	if err != nil {
		t.Fatalf("decodeSignalRequest: %v", err)
	}
	if decoded.SDP != req.SDP {
		t.Fatalf("SDP = %q, want %q", decoded.SDP, req.SDP)
	}
	if decoded.Nonce != "aabb" || decoded.HMAC != "ccdd" {
		t.Fatal("nonce/hmac mismatch")
	}
}

func TestDecodeSignalRequestInvalidJSON(t *testing.T) {
	_, err := decodeSignalRequest([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDecodeSignalRequestEmptySDP(t *testing.T) {
	data, _ := json.Marshal(&SignalRequest{SDP: "", Nonce: "aa", HMAC: "bb"})
	_, err := decodeSignalRequest(data)
	if err == nil {
		t.Fatal("expected error for empty SDP")
	}
}

func TestDecodeSignalRequestInvalidSDP(t *testing.T) {
	data, _ := json.Marshal(&SignalRequest{SDP: "no-version-line", Nonce: "aa", HMAC: "bb"})
	_, err := decodeSignalRequest(data)
	if err == nil {
		t.Fatal("expected error for SDP missing version line")
	}
}

func TestDecodeSignalRequestOversizedSDP(t *testing.T) {
	bigSDP := "v=" + string(make([]byte, maxSDPSize+1))
	data, _ := json.Marshal(&SignalRequest{SDP: bigSDP, Nonce: "aa", HMAC: "bb"})
	_, err := decodeSignalRequest(data)
	if err == nil {
		t.Fatal("expected error for oversized SDP")
	}
}

func TestEncodeSignalRequest(t *testing.T) {
	req := &SignalRequest{
		SDP:   "v=0\r\n",
		Nonce: "nonce123",
		HMAC:  "hmac456",
	}
	data, err := encodeSignalRequest(req)
	if err != nil {
		t.Fatalf("encodeSignalRequest: %v", err)
	}

	var decoded SignalRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.SDP != req.SDP || decoded.Nonce != req.Nonce || decoded.HMAC != req.HMAC {
		t.Fatal("round-trip mismatch")
	}
}

// ---------------------------------------------------------------------------
// WS Signal message type constants
// ---------------------------------------------------------------------------

func TestSignalMessageTypeValues(t *testing.T) {
	// Verify the string values of signal message types match expected protocol values.
	expectations := map[SignalMessageType]string{
		SignalTypeAuth:          "auth",
		SignalTypeOffer:         "offer",
		SignalTypeAnswer:        "answer",
		SignalTypeCandidate:     "candidate",
		SignalTypeCandidateDone: "candidate_done",
		SignalTypeError:         "error",
		SignalTypeReconnect:     "reconnect",
	}
	for typ, want := range expectations {
		if string(typ) != want {
			t.Errorf("SignalMessageType %v = %q, want %q", typ, string(typ), want)
		}
	}
}

func TestWsReadLimitConstant(t *testing.T) {
	if wsReadLimit != 1<<20 {
		t.Fatalf("wsReadLimit = %d, want %d", wsReadLimit, 1<<20)
	}
}

// ---------------------------------------------------------------------------
// Server buildICEServers tests
// ---------------------------------------------------------------------------

func TestBuildICEServersNoServers(t *testing.T) {
	servers := buildICEServers(&ICEConfig{
		STUNServers: []string{},
		TURNServers: []string{},
	})
	if len(servers) != 0 {
		t.Fatalf("expected 0 ICE servers, got %d", len(servers))
	}
}

func TestBuildICEServersSTUNOnly(t *testing.T) {
	servers := buildICEServers(&ICEConfig{
		STUNServers: []string{"stun:stun.example.com:3478"},
	})
	if len(servers) != 1 {
		t.Fatalf("expected 1 ICE server, got %d", len(servers))
	}
	if len(servers[0].URLs) != 1 || servers[0].URLs[0] != "stun:stun.example.com:3478" {
		t.Fatalf("unexpected STUN server: %v", servers[0].URLs)
	}
}

func TestBuildICEServersTURNOnly(t *testing.T) {
	servers := buildICEServers(&ICEConfig{
		STUNServers: []string{},
		TURNServers: []string{"turn:turn.example.com:3478"},
		TURNUser:    "user",
		TURNPass:    "pass",
	})
	if len(servers) != 1 {
		t.Fatalf("expected 1 ICE server, got %d", len(servers))
	}
	if servers[0].Username != "user" || servers[0].Credential != "pass" {
		t.Fatal("TURN credentials mismatch")
	}
}

func TestBuildICEServersSTUNAndTURN(t *testing.T) {
	servers := buildICEServers(&ICEConfig{
		STUNServers: []string{"stun:s.example.com:3478"},
		TURNServers: []string{"turn:t.example.com:3478"},
		TURNUser:    "u",
		TURNPass:    "p",
	})
	if len(servers) != 2 {
		t.Fatalf("expected 2 ICE servers, got %d", len(servers))
	}
}

// ---------------------------------------------------------------------------
// Client close / type tests
// ---------------------------------------------------------------------------

func TestClientCloseAndType(t *testing.T) {
	c := NewClient(&ClientConfig{SignalURL: "https://example.com/signal"})
	if c.Type() != "webrtc" {
		t.Fatalf("Type = %s, want webrtc", c.Type())
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify closed state is set.
	if !c.closed.Load() {
		t.Fatal("expected closed to be true after Close()")
	}
}

// ---------------------------------------------------------------------------
// Server close / type / accept tests
// ---------------------------------------------------------------------------

func TestServerTypeAndClose(t *testing.T) {
	s := NewServer(&ServerConfig{Password: "p"}, nil)
	if s.Type() != "webrtc" {
		t.Fatalf("Type = %s, want webrtc", s.Type())
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !s.closed.Load() {
		t.Fatal("expected closed to be true")
	}
}

func TestServerAcceptContextCanceled(t *testing.T) {
	s := NewServer(&ServerConfig{Password: "p"}, nil)
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Accept(ctx)
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}

// ---------------------------------------------------------------------------
// GenerateAuth with empty SDP
// ---------------------------------------------------------------------------

func TestGenerateAuthEmptySDP(t *testing.T) {
	req, err := GenerateAuth("password", "")
	if err != nil {
		t.Fatalf("GenerateAuth: %v", err)
	}
	if req.SDP != "" {
		t.Fatalf("SDP = %q, want empty", req.SDP)
	}
	if req.Nonce == "" || req.HMAC == "" {
		t.Fatal("expected non-empty nonce and HMAC")
	}
}

// ---------------------------------------------------------------------------
// ConnStats JSON serialization
// ---------------------------------------------------------------------------

func TestConnStatsJSONRoundTrip(t *testing.T) {
	original := ConnStats{
		RTT:            150 * time.Millisecond,
		PacketsSent:    1000,
		PacketsRecv:    2000,
		BytesSent:      50000,
		BytesRecv:      60000,
		PacketsLost:    10,
		CandidateLocal: "10.0.0.1:5000",
		CandidateType:  "srflx",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded ConnStats
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.RTT != original.RTT {
		t.Fatalf("RTT = %v, want %v", decoded.RTT, original.RTT)
	}
	if decoded.PacketsSent != original.PacketsSent {
		t.Fatalf("PacketsSent = %d, want %d", decoded.PacketsSent, original.PacketsSent)
	}
	if decoded.CandidateType != "srflx" {
		t.Fatalf("CandidateType = %q, want srflx", decoded.CandidateType)
	}
}
