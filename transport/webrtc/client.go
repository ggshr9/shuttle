package webrtc

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	"github.com/shuttleX/shuttle/transport"
	ymux "github.com/shuttleX/shuttle/transport/mux/yamux"
	"nhooyr.io/websocket"
)

// ClientConfig holds configuration for a WebRTC client transport.
type ClientConfig struct {
	SignalURL    string
	Password     string
	STUNServers  []string
	TURNServers  []string
	TURNUser     string
	TURNPass     string
	ICEPolicy    string // "all", "relay", "public" (default "all")
	LoopbackOnly bool   // restrict ICE to 127.0.0.1, disable mDNS (for testing)
}

// Client implements transport.ClientTransport using WebRTC DataChannels.
type Client struct {
	config *ClientConfig
	mu     sync.Mutex
	closed atomic.Bool
}

// NewClient creates a new WebRTC client transport.
func NewClient(cfg *ClientConfig) *Client {
	// Only fill defaults when STUNServers is nil (not explicitly set).
	// An explicit empty slice []string{} means "no STUN servers".
	if cfg.STUNServers == nil {
		cfg.STUNServers = []string{
			"stun:stun.l.google.com:19302",
			"stun:stun1.l.google.com:19302",
		}
	}
	return &Client{config: cfg}
}

// Type returns the transport type identifier.
func (c *Client) Type() string { return "webrtc" }

// isWSSignalURL returns true if the signal URL uses WebSocket protocol.
func isWSSignalURL(url string) bool {
	return strings.HasPrefix(url, "ws://") || strings.HasPrefix(url, "wss://")
}

// newPeerConnection creates a configured PeerConnection with detached DataChannels.
func (c *Client) newPeerConnection() (*webrtc.PeerConnection, error) {
	var iceServers []webrtc.ICEServer
	if len(c.config.STUNServers) > 0 {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs: c.config.STUNServers,
		})
	}
	if len(c.config.TURNServers) > 0 {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:           c.config.TURNServers,
			Username:       c.config.TURNUser,
			Credential:     c.config.TURNPass,
			CredentialType: webrtc.ICECredentialTypePassword,
		})
	}

	se := webrtc.SettingEngine{}
	se.DetachDataChannels()
	if c.config.LoopbackOnly {
		se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
		se.SetNAT1To1IPs([]string{"127.0.0.1"}, webrtc.ICECandidateTypeHost)
		se.SetIncludeLoopbackCandidate(true)
	}

	api := webrtc.NewAPI(webrtc.WithSettingEngine(se))
	return api.NewPeerConnection(webrtc.Configuration{
		ICEServers:         iceServers,
		ICETransportPolicy: mapICEPolicy(c.config.ICEPolicy),
	})
}

// createDataChannel creates a reliable, ordered DataChannel and returns channels
// that signal when it opens or errors.
func (c *Client) createDataChannel(pc *webrtc.PeerConnection) (*webrtc.DataChannel, <-chan datachanelRWC, <-chan error) {
	dcOpenCh := make(chan datachanelRWC, 1)
	dcErrCh := make(chan error, 1)

	ordered := true
	dc, err := pc.CreateDataChannel("shuttle", &webrtc.DataChannelInit{
		Ordered: &ordered,
	})
	if err != nil {
		dcErrCh <- err
		return nil, dcOpenCh, dcErrCh
	}

	dc.OnOpen(func() {
		raw, dErr := dc.Detach()
		if dErr != nil {
			dcErrCh <- dErr
			return
		}
		dcOpenCh <- raw
	})

	dc.OnError(func(err error) {
		select {
		case dcErrCh <- err:
		default:
		}
	})

	return dc, dcOpenCh, dcErrCh
}

// Dial establishes a WebRTC connection. It auto-detects ws/wss vs http/https
// signal URLs and uses the appropriate signaling path.
func (c *Client) Dial(ctx context.Context, addr string) (transport.Connection, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("webrtc client closed")
	}

	if isWSSignalURL(c.config.SignalURL) {
		return c.dialWS(ctx)
	}
	return c.dialHTTP(ctx)
}

// dialWS establishes a connection via WebSocket signaling with Trickle ICE.
func (c *Client) dialWS(ctx context.Context) (transport.Connection, error) {
	// Connect to WebSocket
	wsConn, _, err := websocket.Dial(ctx, c.config.SignalURL, &websocket.DialOptions{
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("webrtc ws dial: %w", err)
	}
	wsConn.SetReadLimit(wsReadLimit)

	// Step 1: Send auth
	authReq, err := GenerateAuth(c.config.Password, "")
	if err != nil {
		wsConn.Close(websocket.StatusInternalError, "")
		return nil, fmt.Errorf("webrtc ws auth gen: %w", err)
	}
	if err := sendWSMessage(ctx, wsConn, &SignalMessage{
		Type:  SignalTypeAuth,
		Nonce: authReq.Nonce,
		HMAC:  authReq.HMAC,
	}); err != nil {
		wsConn.Close(websocket.StatusInternalError, "")
		return nil, fmt.Errorf("webrtc ws send auth: %w", err)
	}

	// Step 2: Create PeerConnection + DataChannel
	pc, err := c.newPeerConnection()
	if err != nil {
		wsConn.Close(websocket.StatusInternalError, "")
		return nil, fmt.Errorf("webrtc ws new peer: %w", err)
	}

	_, dcOpenCh, dcErrCh := c.createDataChannel(pc)

	// Trickle ICE: send candidates as they are discovered
	pc.OnICECandidate(func(cand *webrtc.ICECandidate) {
		if cand == nil {
			_ = sendWSMessage(ctx, wsConn, &SignalMessage{Type: SignalTypeCandidateDone})
			return
		}
		init := cand.ToJSON()
		_ = sendWSMessage(ctx, wsConn, &SignalMessage{
			Type: SignalTypeCandidate,
			Candidate: &ICECandidateMsg{
				Candidate:        init.Candidate,
				SDPMid:           init.SDPMid,
				SDPMLineIndex:    init.SDPMLineIndex,
				UsernameFragment: init.UsernameFragment,
			},
		})
	})

	// Step 3: Create offer and send immediately (trickle ICE — don't wait for gathering)
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		pc.Close()
		wsConn.Close(websocket.StatusInternalError, "")
		return nil, fmt.Errorf("webrtc ws create offer: %w", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		pc.Close()
		wsConn.Close(websocket.StatusInternalError, "")
		return nil, fmt.Errorf("webrtc ws set local desc: %w", err)
	}

	// Send offer to server
	if err := sendWSMessage(ctx, wsConn, &SignalMessage{
		Type: SignalTypeOffer,
		SDP:  offer.SDP,
	}); err != nil {
		pc.Close()
		wsConn.Close(websocket.StatusInternalError, "")
		return nil, fmt.Errorf("webrtc ws send offer: %w", err)
	}

	// Step 4: Read answer
	answerMsg, err := readWSMessage(ctx, wsConn)
	if err != nil {
		pc.Close()
		wsConn.Close(websocket.StatusInternalError, "")
		return nil, fmt.Errorf("webrtc ws read answer: %w", err)
	}
	if answerMsg.Type == SignalTypeError {
		pc.Close()
		wsConn.Close(websocket.StatusPolicyViolation, "")
		return nil, fmt.Errorf("webrtc ws signal error: %s", answerMsg.Error)
	}
	if answerMsg.Type != SignalTypeAnswer {
		pc.Close()
		wsConn.Close(websocket.StatusPolicyViolation, "")
		return nil, fmt.Errorf("webrtc ws unexpected message: %s", answerMsg.Type)
	}

	answer := webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answerMsg.SDP}
	if err := pc.SetRemoteDescription(answer); err != nil {
		pc.Close()
		wsConn.Close(websocket.StatusInternalError, "")
		return nil, fmt.Errorf("webrtc ws set remote desc: %w", err)
	}

	// Step 5: Read server's trickle candidates
	go func() {
		for {
			msg, err := readWSMessage(ctx, wsConn)
			if err != nil {
				return
			}
			switch msg.Type {
			case SignalTypeCandidate:
				if msg.Candidate != nil {
					_ = pc.AddICECandidate(webrtc.ICECandidateInit{
						Candidate:        msg.Candidate.Candidate,
						SDPMid:           msg.Candidate.SDPMid,
						SDPMLineIndex:    msg.Candidate.SDPMLineIndex,
						UsernameFragment: msg.Candidate.UsernameFragment,
					})
				}
			case SignalTypeCandidateDone:
				return
			default:
				return
			}
		}
	}()

	// Step 6: Wait for DataChannel
	dcCtx, dcCancel := context.WithTimeout(ctx, 15*time.Second)
	defer dcCancel()
	var raw datachanelRWC
	select {
	case raw = <-dcOpenCh:
	case err := <-dcErrCh:
		pc.Close()
		wsConn.Close(websocket.StatusInternalError, "")
		return nil, fmt.Errorf("webrtc ws dc error: %w", err)
	case <-dcCtx.Done():
		pc.Close()
		wsConn.Close(websocket.StatusInternalError, "")
		return nil, fmt.Errorf("webrtc ws dc open timeout")
	}

	// Create yamux via shared Mux
	rwc := &dcReadWriteCloser{rwc: raw, pc: pc}
	mux := ymux.New(nil)
	muxConn, err := mux.ClientRWC(rwc)
	if err != nil {
		pc.Close()
		wsConn.Close(websocket.StatusInternalError, "")
		return nil, fmt.Errorf("yamux client: %w", err)
	}

	go func() {
		<-muxConn.CloseChan()
		pc.Close()
	}()

	conn := &webrtcConnection{
		pc:      pc,
		muxConn: muxConn,
		local:   &webrtcAddr{addr: "local"},
		remote:  &webrtcAddr{addr: c.config.SignalURL},
		sc:      newStatsCollector(pc),
		wsConn: &wsCloser{closeFn: func() error {
			return wsConn.Close(websocket.StatusNormalClosure, "closing")
		}},
	}

	// Set up reconnection for WS path
	rc := newReconnector(c, conn, wsConn)
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateFailed:
			go rc.trigger()
		case webrtc.PeerConnectionStateDisconnected:
			go func() {
				time.Sleep(5 * time.Second)
				if pc.ConnectionState() == webrtc.PeerConnectionStateDisconnected {
					rc.trigger()
				}
			}()
		}
	})

	return conn, nil
}

// dialHTTP establishes a connection via HTTP POST signaling (legacy, full ICE gather).
func (c *Client) dialHTTP(ctx context.Context) (transport.Connection, error) {
	pc, err := c.newPeerConnection()
	if err != nil {
		return nil, fmt.Errorf("webrtc new peer: %w", err)
	}

	_, dcOpenCh, dcErrCh := c.createDataChannel(pc)

	// Create SDP offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("webrtc create offer: %w", err)
	}

	// Wait for ICE gathering to complete (full ICE, not trickle)
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		pc.Close()
		return nil, fmt.Errorf("webrtc set local desc: %w", err)
	}

	gatherCtx, gatherCancel := context.WithTimeout(ctx, 10*time.Second)
	defer gatherCancel()
	select {
	case <-gatherComplete:
	case <-gatherCtx.Done():
		pc.Close()
		return nil, fmt.Errorf("webrtc ICE gather timeout")
	}

	// Send signaling request
	localDesc := pc.LocalDescription()
	req, err := GenerateAuth(c.config.Password, localDesc.SDP)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("webrtc auth: %w", err)
	}
	reqBody, err := json.Marshal(req)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("webrtc marshal: %w", err)
	}

	httpCtx, httpCancel := context.WithTimeout(ctx, 30*time.Second)
	defer httpCancel()
	httpReq, err := http.NewRequestWithContext(httpCtx, "POST", c.config.SignalURL, bytes.NewReader(reqBody))
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("webrtc signal request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("webrtc signal: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		pc.Close()
		return nil, fmt.Errorf("webrtc signal status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("webrtc read response: %w", err)
	}

	var sigResp SignalResponse
	if err := json.Unmarshal(body, &sigResp); err != nil {
		pc.Close()
		return nil, fmt.Errorf("webrtc parse response: %w", err)
	}
	if sigResp.Error != "" {
		pc.Close()
		return nil, fmt.Errorf("webrtc signal error: %s", sigResp.Error)
	}

	// Set remote description (server's answer)
	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sigResp.SDP,
	}
	if err := pc.SetRemoteDescription(answer); err != nil {
		pc.Close()
		return nil, fmt.Errorf("webrtc set remote desc: %w", err)
	}

	// Wait for DataChannel to open
	dcCtx, dcCancel := context.WithTimeout(ctx, 15*time.Second)
	defer dcCancel()
	var raw datachanelRWC
	select {
	case raw = <-dcOpenCh:
	case err := <-dcErrCh:
		pc.Close()
		return nil, fmt.Errorf("webrtc dc error: %w", err)
	case <-dcCtx.Done():
		pc.Close()
		return nil, fmt.Errorf("webrtc dc open timeout")
	}

	// Wrap in dcReadWriteCloser and create yamux via shared Mux
	rwc := &dcReadWriteCloser{rwc: raw, pc: pc}
	mux := ymux.New(nil)
	muxConn, err := mux.ClientRWC(rwc)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("yamux client: %w", err)
	}

	// Monitor PeerConnection state for cleanup
	go func() {
		<-muxConn.CloseChan()
		pc.Close()
	}()

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateDisconnected {
			muxConn.Close()
		}
	})

	return &webrtcConnection{
		pc:      pc,
		muxConn: muxConn,
		local:   &webrtcAddr{addr: "local"},
		remote:  &webrtcAddr{addr: c.config.SignalURL},
		sc:      newStatsCollector(pc),
	}, nil
}

// Close shuts down the client transport.
func (c *Client) Close() error {
	c.closed.Store(true)
	return nil
}

// Compile-time interface check.
var _ transport.ClientTransport = (*Client)(nil)
