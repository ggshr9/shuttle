package webrtc

import (
	"context"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	pionwebrtc "github.com/pion/webrtc/v4"
	"nhooyr.io/websocket"
)

const (
	reconnectBaseDelay = 1 * time.Second
	reconnectMaxDelay  = 30 * time.Second
	reconnectMaxRetry  = 5
)

// reconnector handles automatic reconnection for WebSocket-signaled WebRTC connections.
type reconnector struct {
	client *Client
	conn   *webrtcConnection
	wsConn *websocket.Conn

	mu       sync.Mutex
	attempts int
	active   bool
}

func newReconnector(client *Client, conn *webrtcConnection, wsConn *websocket.Conn) *reconnector {
	return &reconnector{
		client: client,
		conn:   conn,
		wsConn: wsConn,
	}
}

// trigger initiates a reconnection attempt with exponential backoff.
func (r *reconnector) trigger() {
	r.mu.Lock()
	if r.active || r.attempts >= reconnectMaxRetry {
		r.mu.Unlock()
		return
	}
	r.active = true
	attempt := r.attempts
	r.attempts++
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.active = false
		r.mu.Unlock()
	}()

	// Exponential backoff
	delay := reconnectBaseDelay << uint(attempt)
	if delay > reconnectMaxDelay {
		delay = reconnectMaxDelay
	}
	time.Sleep(delay)

	if err := r.attemptReconnect(); err != nil {
		return
	}

	// Reset attempts on success
	r.mu.Lock()
	r.attempts = 0
	r.mu.Unlock()
}

func (r *reconnector) attemptReconnect() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Signal reconnect intent over the existing WebSocket
	if err := sendWSMessage(ctx, r.wsConn, &SignalMessage{Type: SignalTypeReconnect}); err != nil {
		return err
	}

	// Create new PeerConnection
	pc, err := r.client.newPeerConnection()
	if err != nil {
		return err
	}

	_, dcOpenCh, dcErrCh := r.client.createDataChannel(pc)

	// Trickle ICE
	pc.OnICECandidate(func(cand *pionwebrtc.ICECandidate) {
		if cand == nil {
			sendWSMessage(ctx, r.wsConn, &SignalMessage{Type: SignalTypeCandidateDone})
			return
		}
		init := cand.ToJSON()
		sendWSMessage(ctx, r.wsConn, &SignalMessage{
			Type: SignalTypeCandidate,
			Candidate: &ICECandidateMsg{
				Candidate:        init.Candidate,
				SDPMid:           init.SDPMid,
				SDPMLineIndex:    init.SDPMLineIndex,
				UsernameFragment: init.UsernameFragment,
			},
		})
	})

	// Create & send offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		pc.Close()
		return err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		pc.Close()
		return err
	}

	if err := sendWSMessage(ctx, r.wsConn, &SignalMessage{Type: SignalTypeOffer, SDP: offer.SDP}); err != nil {
		pc.Close()
		return err
	}

	// Read answer
	answerMsg, err := readWSMessage(ctx, r.wsConn)
	if err != nil || answerMsg.Type != SignalTypeAnswer {
		pc.Close()
		return err
	}

	answer := pionwebrtc.SessionDescription{Type: pionwebrtc.SDPTypeAnswer, SDP: answerMsg.SDP}
	if err := pc.SetRemoteDescription(answer); err != nil {
		pc.Close()
		return err
	}

	// Read server candidates
	go func() {
		for {
			msg, err := readWSMessage(ctx, r.wsConn)
			if err != nil {
				return
			}
			if msg.Type == SignalTypeCandidate && msg.Candidate != nil {
				pc.AddICECandidate(pionwebrtc.ICECandidateInit{
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

	// Wait for DataChannel
	dcTimer := time.NewTimer(15 * time.Second)
	defer dcTimer.Stop()

	var raw datachanelRWC
	select {
	case raw = <-dcOpenCh:
	case err := <-dcErrCh:
		pc.Close()
		return err
	case <-dcTimer.C:
		pc.Close()
		return context.DeadlineExceeded
	}

	// Create new yamux session
	rwc := &dcReadWriteCloser{rwc: raw, pc: pc}
	sess, err := yamux.Client(rwc, yamux.DefaultConfig())
	if err != nil {
		pc.Close()
		return err
	}

	go func() {
		<-sess.CloseChan()
		pc.Close()
	}()

	// Atomically swap session and PC in the connection
	r.conn.mu.Lock()
	oldPC := r.conn.pc
	oldSess := r.conn.session
	oldSC := r.conn.sc
	r.conn.pc = pc
	r.conn.session = sess
	r.conn.sc = newStatsCollector(pc)
	r.conn.mu.Unlock()

	// Cleanup old resources
	if oldSC != nil {
		oldSC.Close()
	}
	oldSess.Close()
	oldPC.Close()

	// Re-register reconnection handler on the new PC
	pc.OnConnectionStateChange(func(state pionwebrtc.PeerConnectionState) {
		switch state {
		case pionwebrtc.PeerConnectionStateFailed:
			go r.trigger()
		case pionwebrtc.PeerConnectionStateDisconnected:
			go func() {
				time.Sleep(5 * time.Second)
				if pc.ConnectionState() == pionwebrtc.PeerConnectionStateDisconnected {
					r.trigger()
				}
			}()
		}
	})

	return nil
}
