package webrtc

import (
	"context"
	"net/http"
	"time"

	"github.com/pion/webrtc/v4"
	ymux "github.com/shuttleX/shuttle/transport/mux/yamux"
	"github.com/coder/websocket"
)

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Origin checking is not needed — connections are authenticated
		// via HMAC in the first message, and WebSocket is not cookie-based.
		InsecureSkipVerify: true, //nolint:gosec // auth via HMAC, not origin
	})
	if err != nil {
		s.logger.Error("webrtc ws accept failed", "err", err)
		return
	}
	wsConn.SetReadLimit(wsReadLimit)

	ctx := r.Context()
	remoteAddr := r.RemoteAddr

	// Step 1: Read auth message
	authMsg, err := readWSMessage(ctx, wsConn)
	if err != nil || authMsg.Type != SignalTypeAuth {
		s.logger.Debug("webrtc ws: expected auth message", "remote", remoteAddr)
		_ = sendWSMessage(ctx, wsConn, &SignalMessage{Type: SignalTypeError, Error: "expected auth"})
		_ = wsConn.Close(websocket.StatusPolicyViolation, "expected auth")
		return
	}

	// Verify HMAC
	sigReq := &SignalRequest{Nonce: authMsg.Nonce, HMAC: authMsg.HMAC}
	nonce, ok := VerifyAuth(sigReq, s.config.Password)
	if !ok {
		s.logger.Debug("webrtc ws auth failed", "remote", remoteAddr)
		_ = sendWSMessage(ctx, wsConn, &SignalMessage{Type: SignalTypeError, Error: "auth failed"})
		_ = wsConn.Close(websocket.StatusPolicyViolation, "auth failed")
		return
	}
	if s.replayFilter.CheckBytes(nonce) {
		s.logger.Warn("webrtc ws replay detected", "remote", remoteAddr)
		_ = sendWSMessage(ctx, wsConn, &SignalMessage{Type: SignalTypeError, Error: "replay"})
		_ = wsConn.Close(websocket.StatusPolicyViolation, "replay")
		return
	}

	// Step 2: Read offer
	offerMsg, err := readWSMessage(ctx, wsConn)
	if err != nil || offerMsg.Type != SignalTypeOffer {
		s.logger.Debug("webrtc ws: expected offer", "remote", remoteAddr)
		_ = sendWSMessage(ctx, wsConn, &SignalMessage{Type: SignalTypeError, Error: "expected offer"})
		wsConn.Close(websocket.StatusPolicyViolation, "expected offer")
		return
	}

	// Create PeerConnection
	pc, err := s.newPeerConnection()
	if err != nil {
		s.logger.Error("webrtc ws: new peer failed", "err", err)
		_ = sendWSMessage(ctx, wsConn, &SignalMessage{Type: SignalTypeError, Error: "internal error"})
		wsConn.Close(websocket.StatusInternalError, "internal error")
		return
	}

	// DataChannel handler
	dcCh := make(chan datachanelRWC, 1)
	dcErrCh := make(chan error, 1)
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnOpen(func() {
			raw, dErr := dc.Detach()
			if dErr != nil {
				dcErrCh <- dErr
				return
			}
			dcCh <- raw
		})
		dc.OnError(func(err error) {
			select {
			case dcErrCh <- err:
			default:
			}
		})
	})

	// Trickle ICE: send server candidates as they are discovered
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			// Gathering complete
			_ = sendWSMessage(ctx, wsConn, &SignalMessage{Type: SignalTypeCandidateDone})
			return
		}
		init := c.ToJSON()
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

	// Set remote description (offer)
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerMsg.SDP,
	}
	if err := pc.SetRemoteDescription(offer); err != nil {
		pc.Close()
		s.logger.Error("webrtc ws: set remote desc failed", "err", err)
		_ = sendWSMessage(ctx, wsConn, &SignalMessage{Type: SignalTypeError, Error: "invalid offer"})
		wsConn.Close(websocket.StatusPolicyViolation, "invalid offer")
		return
	}

	// Create answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		s.logger.Error("webrtc ws: create answer failed", "err", err)
		_ = sendWSMessage(ctx, wsConn, &SignalMessage{Type: SignalTypeError, Error: "internal error"})
		wsConn.Close(websocket.StatusInternalError, "internal error")
		return
	}

	// Set local description — this starts ICE gathering (trickle candidates sent via OnICECandidate)
	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		s.logger.Error("webrtc ws: set local desc failed", "err", err)
		_ = sendWSMessage(ctx, wsConn, &SignalMessage{Type: SignalTypeError, Error: "internal error"})
		wsConn.Close(websocket.StatusInternalError, "internal error")
		return
	}

	// Send answer
	if err := sendWSMessage(ctx, wsConn, &SignalMessage{
		Type: SignalTypeAnswer,
		SDP:  pc.LocalDescription().SDP,
	}); err != nil {
		pc.Close()
		s.logger.Error("webrtc ws: send answer failed", "err", err)
		wsConn.Close(websocket.StatusInternalError, "send answer failed")
		return
	}

	// Read incoming trickle candidates from client in a goroutine
	candidateDone := make(chan struct{})
	go func() {
		defer close(candidateDone)
		for {
			msg, err := readWSMessage(ctx, wsConn)
			if err != nil {
				return
			}
			switch msg.Type {
			case SignalTypeCandidate:
				if msg.Candidate != nil {
					init := webrtc.ICECandidateInit{
						Candidate:        msg.Candidate.Candidate,
						SDPMid:           msg.Candidate.SDPMid,
						SDPMLineIndex:    msg.Candidate.SDPMLineIndex,
						UsernameFragment: msg.Candidate.UsernameFragment,
					}
					if addErr := pc.AddICECandidate(init); addErr != nil {
						s.logger.Debug("webrtc ws: add candidate failed", "err", addErr)
					}
				}
			case SignalTypeCandidateDone:
				return
			case SignalTypeReconnect:
				s.handleWSReconnect(ctx, wsConn, pc, remoteAddr)
				return
			default:
				return
			}
		}
	}()

	// Wait for DataChannel
	dcTimer := time.NewTimer(15 * time.Second)
	defer dcTimer.Stop()

	var raw datachanelRWC
	select {
	case raw = <-dcCh:
	case err := <-dcErrCh:
		pc.Close()
		s.logger.Error("webrtc ws dc error", "err", err, "remote", remoteAddr)
		wsConn.Close(websocket.StatusInternalError, "dc error")
		return
	case <-dcTimer.C:
		pc.Close()
		s.logger.Warn("webrtc ws dc open timeout", "remote", remoteAddr)
		wsConn.Close(websocket.StatusInternalError, "dc timeout")
		return
	}

	// Create yamux session via shared Mux
	rwc := &dcReadWriteCloser{rwc: raw, pc: pc}
	mux := ymux.New(nil)
	muxConn, err := mux.ServerRWC(rwc)
	if err != nil {
		pc.Close()
		s.logger.Error("webrtc ws yamux error", "err", err)
		wsConn.Close(websocket.StatusInternalError, "yamux error")
		return
	}

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateDisconnected {
			muxConn.Close()
		}
	})

	conn := &webrtcConnection{
		pc:      pc,
		muxConn: muxConn,
		local:   &webrtcAddr{addr: "server"},
		remote:  &webrtcAddr{addr: remoteAddr},
		sc:      newStatsCollector(pc),
		wsConn: &wsCloser{closeFn: func() error {
			return wsConn.Close(websocket.StatusNormalClosure, "closing")
		}},
	}

	select {
	case s.connCh <- conn:
		s.logger.Debug("webrtc ws connection accepted", "remote", remoteAddr)
	default:
		s.logger.Warn("webrtc ws connection channel full, dropping", "remote", remoteAddr)
		conn.Close()
	}
}

// handleWSReconnect handles a reconnection request on an existing WebSocket.
func (s *Server) handleWSReconnect(ctx context.Context, wsConn *websocket.Conn, oldPC *webrtc.PeerConnection, remoteAddr string) {
	// Read the new offer
	offerMsg, err := readWSMessage(ctx, wsConn)
	if err != nil || offerMsg.Type != SignalTypeOffer {
		s.logger.Debug("webrtc ws reconnect: expected offer", "remote", remoteAddr)
		return
	}

	pc, err := s.newPeerConnection()
	if err != nil {
		s.logger.Error("webrtc ws reconnect: new peer failed", "err", err)
		return
	}

	dcCh := make(chan datachanelRWC, 1)
	dcErrCh := make(chan error, 1)
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnOpen(func() {
			raw, dErr := dc.Detach()
			if dErr != nil {
				dcErrCh <- dErr
				return
			}
			dcCh <- raw
		})
		dc.OnError(func(err error) {
			select {
			case dcErrCh <- err:
			default:
			}
		})
	})

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			_ = sendWSMessage(ctx, wsConn, &SignalMessage{Type: SignalTypeCandidateDone})
			return
		}
		init := c.ToJSON()
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

	offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerMsg.SDP}
	if err := pc.SetRemoteDescription(offer); err != nil {
		pc.Close()
		return
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		return
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		return
	}

	_ = sendWSMessage(ctx, wsConn, &SignalMessage{Type: SignalTypeAnswer, SDP: pc.LocalDescription().SDP})

	// Read client candidates
	go func() {
		for {
			msg, err := readWSMessage(ctx, wsConn)
			if err != nil {
				return
			}
			if msg.Type == SignalTypeCandidate && msg.Candidate != nil {
				_ = pc.AddICECandidate(webrtc.ICECandidateInit{
					Candidate:        msg.Candidate.Candidate,
					SDPMid:           msg.Candidate.SDPMid,
					SDPMLineIndex:    msg.Candidate.SDPMLineIndex,
					UsernameFragment: msg.Candidate.UsernameFragment,
				})
			} else if msg.Type == SignalTypeCandidateDone {
				return
			}
		}
	}()

	dcTimer := time.NewTimer(15 * time.Second)
	defer dcTimer.Stop()

	var raw datachanelRWC
	select {
	case raw = <-dcCh:
	case err := <-dcErrCh:
		pc.Close()
		s.logger.Error("webrtc ws reconnect dc error", "err", err)
		return
	case <-dcTimer.C:
		pc.Close()
		s.logger.Warn("webrtc ws reconnect dc timeout")
		return
	}

	rwc := &dcReadWriteCloser{rwc: raw, pc: pc}
	mux := ymux.New(nil)
	muxConn, err := mux.ServerRWC(rwc)
	if err != nil {
		pc.Close()
		return
	}

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateDisconnected {
			muxConn.Close()
		}
	})

	conn := &webrtcConnection{
		pc:      pc,
		muxConn: muxConn,
		local:   &webrtcAddr{addr: "server"},
		remote:  &webrtcAddr{addr: remoteAddr},
		sc:      newStatsCollector(pc),
		wsConn: &wsCloser{closeFn: func() error {
			return wsConn.Close(websocket.StatusNormalClosure, "closing")
		}},
	}

	select {
	case s.connCh <- conn:
		s.logger.Debug("webrtc ws reconnect accepted", "remote", remoteAddr)
	default:
		conn.Close()
	}
}
