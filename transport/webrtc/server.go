package webrtc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	"github.com/shuttleX/shuttle/crypto"
	"github.com/shuttleX/shuttle/transport"
	ymux "github.com/shuttleX/shuttle/transport/mux/yamux"
	"nhooyr.io/websocket"
)

// ServerConfig holds configuration for a WebRTC server transport.
type ServerConfig struct {
	SignalListen string
	CertFile     string
	KeyFile      string
	Password     string
	STUNServers  []string
	TURNServers  []string
	TURNUser     string
	TURNPass     string
	ICEPolicy    string // "all", "relay", "public" (default "all")
	LoopbackOnly bool   // restrict ICE to 127.0.0.1, disable mDNS (for testing)
}

// Server implements transport.ServerTransport using WebRTC DataChannels.
type Server struct {
	config       *ServerConfig
	httpServer   *http.Server
	connCh       chan transport.Connection
	closed       atomic.Bool
	logger       *slog.Logger
	replayFilter *crypto.ReplayFilter
}

// NewServer creates a new WebRTC server transport.
func NewServer(cfg *ServerConfig, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	// Only fill defaults when STUNServers is nil (not explicitly set).
	// An explicit empty slice []string{} means "no STUN servers".
	if cfg.STUNServers == nil {
		cfg.STUNServers = []string{
			"stun:stun.l.google.com:19302",
			"stun:stun1.l.google.com:19302",
		}
	}
	return &Server{
		config:       cfg,
		connCh:       make(chan transport.Connection, 64),
		logger:       logger,
		replayFilter: crypto.NewReplayFilter(120 * time.Second),
	}
}

// Type returns the transport type identifier.
func (s *Server) Type() string { return "webrtc" }

// Listen starts the HTTPS signaling server.
func (s *Server) Listen(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webrtc/signal", s.handleSignal)
	mux.HandleFunc("GET /webrtc/ws", s.handleWebSocket)
	// Cover behavior: non-POST or wrong path returns 404
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	addr := s.config.SignalListen
	if addr == "" {
		addr = ":8443"
	}

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Load TLS config if certs provided
	if s.config.CertFile != "" && s.config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.config.CertFile, s.config.KeyFile)
		if err != nil {
			return fmt.Errorf("webrtc load tls: %w", err)
		}
		s.httpServer.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	}

	s.logger.Info("webrtc signaling server listening", "addr", addr)

	go func() {
		var err error
		if s.httpServer.TLSConfig != nil {
			err = s.httpServer.ListenAndServeTLS("", "")
		} else {
			err = s.httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			s.logger.Error("webrtc signal server error", "err", err)
		}
	}()

	return nil
}

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

// buildICEServers creates the ICE server configuration from server config.
func (s *Server) buildICEServers() []webrtc.ICEServer {
	var iceServers []webrtc.ICEServer
	if len(s.config.STUNServers) > 0 {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs: s.config.STUNServers,
		})
	}
	if len(s.config.TURNServers) > 0 {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:           s.config.TURNServers,
			Username:       s.config.TURNUser,
			Credential:     s.config.TURNPass,
			CredentialType: webrtc.ICECredentialTypePassword,
		})
	}
	return iceServers
}

// newPeerConnection creates a configured PeerConnection with detached DataChannels.
func (s *Server) newPeerConnection() (*webrtc.PeerConnection, error) {
	se := webrtc.SettingEngine{}
	se.DetachDataChannels()
	if s.config.LoopbackOnly {
		se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
		se.SetNAT1To1IPs([]string{"127.0.0.1"}, webrtc.ICECandidateTypeHost)
		se.SetIncludeLoopbackCandidate(true)
	}
	api := webrtc.NewAPI(webrtc.WithSettingEngine(se))
	return api.NewPeerConnection(webrtc.Configuration{
		ICEServers:         s.buildICEServers(),
		ICETransportPolicy: mapICEPolicy(s.config.ICEPolicy),
	})
}

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

// Accept returns the next authenticated WebRTC connection.
func (s *Server) Accept(ctx context.Context) (transport.Connection, error) {
	select {
	case conn := <-s.connCh:
		return conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close shuts down the server transport.
func (s *Server) Close() error {
	s.closed.Store(true)
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func writeSignalError(w http.ResponseWriter, msg string) {
	resp := SignalResponse{Error: msg}
	body, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write(body)
}

// Compile-time interface check.
var _ transport.ServerTransport = (*Server)(nil)
