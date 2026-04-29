package webrtc

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/pion/webrtc/v4"
	ymux "github.com/ggshr9/shuttle/transport/mux/yamux"
)

func (s *Server) handleSignal(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var req SignalRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Verify HMAC auth
	nonce, ok := VerifyAuth(&req, s.config.Password)
	if !ok {
		s.logger.Debug("webrtc auth failed", "remote", r.RemoteAddr)
		http.NotFound(w, r)
		return
	}

	// Check replay
	if s.replayFilter.CheckBytes(nonce) {
		s.logger.Warn("webrtc replay detected", "remote", r.RemoteAddr)
		http.NotFound(w, r)
		return
	}

	// Create PeerConnection
	pc, err := s.newPeerConnection()
	if err != nil {
		s.logger.Error("webrtc new peer failed", "err", err)
		writeSignalError(w, "internal error")
		return
	}

	// Register OnDataChannel BEFORE setting remote description to avoid
	// a race where the DataChannel opens before the handler is set.
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

	// Set remote description (client's offer)
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  req.SDP,
	}
	if err := pc.SetRemoteDescription(offer); err != nil {
		pc.Close()
		s.logger.Error("webrtc set remote desc failed", "err", err)
		writeSignalError(w, "invalid offer")
		return
	}

	// Create answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		s.logger.Error("webrtc create answer failed", "err", err)
		writeSignalError(w, "internal error")
		return
	}

	// Wait for ICE gathering to complete
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		s.logger.Error("webrtc set local desc failed", "err", err)
		writeSignalError(w, "internal error")
		return
	}

	gatherTimer := time.NewTimer(10 * time.Second)
	defer gatherTimer.Stop()
	select {
	case <-gatherComplete:
	case <-gatherTimer.C:
		pc.Close()
		s.logger.Warn("webrtc ICE gather timeout")
		writeSignalError(w, "ICE gather timeout")
		return
	}

	// Return SDP answer
	localDesc := pc.LocalDescription()
	resp := SignalResponse{SDP: localDesc.SDP}
	respBody, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(respBody)

	// Async: wait for DataChannel to open, then establish yamux and push connection
	go s.awaitDataChannel(pc, dcCh, dcErrCh, r.RemoteAddr)
}

func (s *Server) awaitDataChannel(pc *webrtc.PeerConnection, dcCh <-chan datachanelRWC, dcErrCh <-chan error, remoteAddr string) {
	// Wait for DataChannel with timeout
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()

	var raw datachanelRWC
	select {
	case raw = <-dcCh:
	case err := <-dcErrCh:
		s.logger.Error("webrtc dc error", "err", err, "remote", remoteAddr)
		pc.Close()
		return
	case <-timer.C:
		s.logger.Warn("webrtc dc open timeout", "remote", remoteAddr)
		pc.Close()
		return
	}

	// Wrap and create yamux server session via shared Mux
	rwc := &dcReadWriteCloser{rwc: raw, pc: pc}
	mux := ymux.New(nil)
	muxConn, err := mux.ServerRWC(rwc)
	if err != nil {
		s.logger.Error("webrtc yamux server error", "err", err)
		pc.Close()
		return
	}

	// Monitor PeerConnection state for cleanup
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
	}

	select {
	case s.connCh <- conn:
		s.logger.Debug("webrtc connection accepted", "remote", remoteAddr)
	default:
		s.logger.Warn("webrtc connection channel full, dropping", "remote", remoteAddr)
		conn.Close()
	}
}

// iceConfig returns the shared ICE configuration derived from ServerConfig.
func (s *Server) iceConfig() *ICEConfig {
	return &ICEConfig{
		STUNServers:  s.config.STUNServers,
		TURNServers:  s.config.TURNServers,
		TURNUser:     s.config.TURNUser,
		TURNPass:     s.config.TURNPass,
		ICEPolicy:    s.config.ICEPolicy,
		LoopbackOnly: s.config.LoopbackOnly,
	}
}

// newPeerConnection creates a configured PeerConnection with detached DataChannels.
func (s *Server) newPeerConnection() (*webrtc.PeerConnection, error) {
	return newPeerConnectionFromConfig(s.iceConfig())
}

func writeSignalError(w http.ResponseWriter, msg string) {
	resp := SignalResponse{Error: msg}
	body, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write(body)
}
