// Package p2p implements ICE Restart functionality per RFC 8445.
// ICE Restart allows re-establishing connectivity when network conditions change.
package p2p

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// ICEGatheringState represents the gathering state of an ICE agent.
type ICEGatheringState int

const (
	ICEGatheringNew ICEGatheringState = iota
	ICEGatheringGathering
	ICEGatheringComplete
)

func (s ICEGatheringState) String() string {
	switch s {
	case ICEGatheringNew:
		return "new"
	case ICEGatheringGathering:
		return "gathering"
	case ICEGatheringComplete:
		return "complete"
	default:
		return "unknown"
	}
}

// ICEConnectionState represents the connection state of an ICE agent.
type ICEConnectionState int

const (
	ICEConnectionNew ICEConnectionState = iota
	ICEConnectionChecking
	ICEConnectionConnected
	ICEConnectionCompleted
	ICEConnectionFailed
	ICEConnectionDisconnected
	ICEConnectionClosed
)

func (s ICEConnectionState) String() string {
	switch s {
	case ICEConnectionNew:
		return "new"
	case ICEConnectionChecking:
		return "checking"
	case ICEConnectionConnected:
		return "connected"
	case ICEConnectionCompleted:
		return "completed"
	case ICEConnectionFailed:
		return "failed"
	case ICEConnectionDisconnected:
		return "disconnected"
	case ICEConnectionClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// ICERestartReason indicates why an ICE restart was triggered.
type ICERestartReason int

const (
	ICERestartReasonManual ICERestartReason = iota // Explicit restart request
	ICERestartReasonNetworkChange                  // Network interface changed
	ICERestartReasonQualityDegraded                // Connection quality dropped
	ICERestartReasonAllPairsFailed                 // All candidate pairs failed
	ICERestartReasonTimeout                        // Connection timeout
	ICERestartReasonRemoteRequest                  // Remote peer requested restart
)

func (r ICERestartReason) String() string {
	switch r {
	case ICERestartReasonManual:
		return "manual"
	case ICERestartReasonNetworkChange:
		return "network_change"
	case ICERestartReasonQualityDegraded:
		return "quality_degraded"
	case ICERestartReasonAllPairsFailed:
		return "all_pairs_failed"
	case ICERestartReasonTimeout:
		return "timeout"
	case ICERestartReasonRemoteRequest:
		return "remote_request"
	default:
		return "unknown"
	}
}

// ICECredentials holds the username fragment and password for an ICE session.
type ICECredentials struct {
	UsernameFragment string // 4+ chars, ice-ufrag
	Password         string // 22+ chars, ice-pwd
	Generation       int    // Incremented on each restart
}

// NewICECredentials generates new random ICE credentials.
func NewICECredentials() *ICECredentials {
	return &ICECredentials{
		UsernameFragment: generateICEString(8),
		Password:         generateICEString(24),
		Generation:       0,
	}
}

// Regenerate creates new credentials for ICE restart, incrementing generation.
func (c *ICECredentials) Regenerate() {
	c.UsernameFragment = generateICEString(8)
	c.Password = generateICEString(24)
	c.Generation++
}

// generateICEString generates a random string for ICE credentials.
func generateICEString(length int) string {
	bytes := make([]byte, length/2+1)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
}

// ICEAgent manages ICE connectivity with support for restarts and Trickle ICE.
type ICEAgent struct {
	mu sync.RWMutex

	// Credentials
	localCredentials  *ICECredentials
	remoteCredentials *ICECredentials

	// State
	gatheringState  ICEGatheringState
	connectionState ICEConnectionState
	isControlling   bool

	// Candidates
	localCandidates  []*Candidate
	remoteCandidates []*Candidate
	candidatePairs   []*CandidatePair
	selectedPair     *CandidatePair

	// Connection
	conn        *net.UDPConn
	localVIP    net.IP
	gatherer    *ICEGatherer
	checkTimeout time.Duration
	logger      *slog.Logger

	// Trickle ICE support
	trickleEnabled     bool
	trickleGatherer    *TrickleICEGatherer
	remoteGatheringDone bool // True when remote signals end-of-candidates
	checksStarted      bool  // True when connectivity checks have started

	// Restart tracking
	restartCount    int
	lastRestart     time.Time
	restartCooldown time.Duration
	pendingRestart  bool

	// Quality monitoring
	qualityMonitor  *ConnectionQuality
	qualityThreshold float64 // Restart if quality drops below this

	// Callbacks
	onStateChange       func(ICEConnectionState)
	onGatheringComplete func([]*Candidate)
	onSelectedPair      func(*CandidatePair)
	onRestartNeeded     func(ICERestartReason)
	onLocalCandidate    func(*Candidate) // Trickle: called for each discovered candidate
	onEndOfCandidates   func()           // Trickle: called when local gathering completes

	// Shutdown
	done chan struct{}
}

// ICEAgentConfig holds configuration for ICE agent.
type ICEAgentConfig struct {
	STUNServers      []string
	IsControlling    bool
	GatherTimeout    time.Duration
	CheckTimeout     time.Duration
	RestartCooldown  time.Duration
	QualityThreshold float64 // 0-100, restart if below this
	TrickleEnabled   bool    // Enable Trickle ICE (RFC 8838)
	Logger           *slog.Logger
}

// DefaultICEAgentConfig returns default configuration.
func DefaultICEAgentConfig() *ICEAgentConfig {
	return &ICEAgentConfig{
		STUNServers:      DefaultSTUNServers(),
		IsControlling:    true,
		GatherTimeout:    5 * time.Second,
		CheckTimeout:     30 * time.Second,
		RestartCooldown:  10 * time.Second,
		QualityThreshold: 30.0,
	}
}

// NewICEAgent creates a new ICE agent.
func NewICEAgent(cfg *ICEAgentConfig) *ICEAgent {
	if cfg == nil {
		cfg = DefaultICEAgentConfig()
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	agent := &ICEAgent{
		localCredentials:  NewICECredentials(),
		gatheringState:    ICEGatheringNew,
		connectionState:   ICEConnectionNew,
		isControlling:     cfg.IsControlling,
		localCandidates:   make([]*Candidate, 0),
		remoteCandidates:  make([]*Candidate, 0),
		candidatePairs:    make([]*CandidatePair, 0),
		gatherer:          NewICEGatherer(cfg.STUNServers, cfg.GatherTimeout),
		checkTimeout:      cfg.CheckTimeout,
		logger:            cfg.Logger,
		trickleEnabled:    cfg.TrickleEnabled,
		restartCooldown:   cfg.RestartCooldown,
		qualityThreshold:  cfg.QualityThreshold,
		qualityMonitor:    NewConnectionQuality(),
		done:              make(chan struct{}),
	}

	// Initialize trickle gatherer if enabled
	if cfg.TrickleEnabled {
		agent.trickleGatherer = NewTrickleICEGatherer(&TrickleGathererConfig{
			STUNServers: cfg.STUNServers,
			Timeout:     cfg.GatherTimeout,
			Logger:      cfg.Logger,
		})
	}

	return agent
}

// GetLocalCredentials returns the local ICE credentials.
func (a *ICEAgent) GetLocalCredentials() *ICECredentials {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.localCredentials
}

// SetRemoteCredentials sets the remote peer's credentials.
func (a *ICEAgent) SetRemoteCredentials(creds *ICECredentials) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.remoteCredentials = creds
}

// GetGatheringState returns the current gathering state.
func (a *ICEAgent) GetGatheringState() ICEGatheringState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.gatheringState
}

// GetConnectionState returns the current connection state.
func (a *ICEAgent) GetConnectionState() ICEConnectionState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connectionState
}

// GatherCandidates starts gathering ICE candidates.
func (a *ICEAgent) GatherCandidates() error {
	a.mu.Lock()
	if a.gatheringState == ICEGatheringGathering {
		a.mu.Unlock()
		return nil // Already gathering
	}
	a.gatheringState = ICEGatheringGathering
	a.mu.Unlock()

	a.logger.Debug("ICE gathering started",
		"generation", a.localCredentials.Generation)

	// Gather candidates
	result, err := a.gatherer.Gather()
	if err != nil {
		a.setGatheringState(ICEGatheringNew)
		return fmt.Errorf("ice: gather failed: %w", err)
	}

	a.mu.Lock()
	a.conn = result.LocalConn
	a.localCandidates = result.Candidates
	a.gatheringState = ICEGatheringComplete
	candidates := a.localCandidates
	a.mu.Unlock()

	a.logger.Debug("ICE gathering complete",
		"candidates", len(candidates))

	// Notify callback
	if a.onGatheringComplete != nil {
		a.onGatheringComplete(candidates)
	}

	return nil
}

// AddRemoteCandidate adds a remote candidate.
func (a *ICEAgent) AddRemoteCandidate(candidate *Candidate) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check for duplicates
	for _, c := range a.remoteCandidates {
		if c.Addr.String() == candidate.Addr.String() {
			return
		}
	}

	a.remoteCandidates = append(a.remoteCandidates, candidate)

	// Create pairs with new candidate
	for _, local := range a.localCandidates {
		pair := NewCandidatePair(local, candidate, a.isControlling)
		a.candidatePairs = append(a.candidatePairs, pair)
	}

	// Sort pairs by priority
	SortCandidatePairs(a.candidatePairs)
}

// StartConnectivityChecks begins checking candidate pairs.
func (a *ICEAgent) StartConnectivityChecks() error {
	a.mu.Lock()
	if a.connectionState != ICEConnectionNew && a.connectionState != ICEConnectionDisconnected {
		a.mu.Unlock()
		return fmt.Errorf("ice: invalid state for checks: %s", a.connectionState)
	}
	a.setConnectionStateLocked(ICEConnectionChecking)
	conn := a.conn
	pairs := make([]*CandidatePair, len(a.candidatePairs))
	copy(pairs, a.candidatePairs)
	a.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("ice: no connection available")
	}

	a.logger.Debug("ICE connectivity checks started",
		"pairs", len(pairs))

	// Check pairs in priority order
	go a.runConnectivityChecks(conn, pairs)

	return nil
}

// runConnectivityChecks performs connectivity checks on candidate pairs.
func (a *ICEAgent) runConnectivityChecks(conn *net.UDPConn, pairs []*CandidatePair) {
	a.mu.RLock()
	localVIP := a.localVIP
	checkTimeout := a.checkTimeout
	logger := a.logger
	a.mu.RUnlock()

	// Create hole puncher for this check session
	holePuncher := NewHolePuncher(conn, localVIP, checkTimeout, logger)

	for _, pair := range pairs {
		select {
		case <-a.done:
			return
		default:
		}

		// Update pair state
		a.mu.Lock()
		pair.State = CandidatePairInProgress
		a.mu.Unlock()

		// Try to establish connectivity
		ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
		result, err := holePuncher.SimultaneousPunch(ctx, pair.Remote.Addr.IP, []*Candidate{pair.Remote})
		cancel()

		if err == nil && result != nil {
			// Success!
			a.mu.Lock()
			pair.State = CandidatePairSucceeded
			a.selectedPair = pair
			a.setConnectionStateLocked(ICEConnectionConnected)
			a.mu.Unlock()

			a.logger.Info("ICE connectivity check succeeded",
				"local", pair.Local.Addr,
				"remote", pair.Remote.Addr,
				"rtt", result.RTT)

			if a.onSelectedPair != nil {
				a.onSelectedPair(pair)
			}

			// Start quality monitoring
			go a.monitorConnectionQuality()

			return
		}

		// Mark as failed
		a.mu.Lock()
		pair.State = CandidatePairFailed
		a.mu.Unlock()
	}

	// All pairs failed
	a.mu.Lock()
	a.setConnectionStateLocked(ICEConnectionFailed)
	a.mu.Unlock()

	a.logger.Warn("ICE all connectivity checks failed")

	// Trigger restart if all pairs failed
	if a.onRestartNeeded != nil {
		a.onRestartNeeded(ICERestartReasonAllPairsFailed)
	}
}

// Restart initiates an ICE restart.
func (a *ICEAgent) Restart(reason ICERestartReason) error {
	a.mu.Lock()

	// Check cooldown
	if time.Since(a.lastRestart) < a.restartCooldown {
		a.mu.Unlock()
		return fmt.Errorf("ice: restart cooldown not elapsed")
	}

	// Check if already restarting
	if a.pendingRestart {
		a.mu.Unlock()
		return fmt.Errorf("ice: restart already pending")
	}

	a.pendingRestart = true
	a.lastRestart = time.Now()
	a.restartCount++

	a.logger.Info("ICE restart initiated",
		"reason", reason,
		"restart_count", a.restartCount)

	// Generate new credentials
	a.localCredentials.Regenerate()

	// Clear previous state
	a.remoteCandidates = make([]*Candidate, 0)
	a.candidatePairs = make([]*CandidatePair, 0)
	a.selectedPair = nil
	a.gatheringState = ICEGatheringNew
	a.setConnectionStateLocked(ICEConnectionNew)

	// Keep connection if still valid, otherwise will recreate during gathering
	oldConn := a.conn

	a.mu.Unlock()

	// Re-gather candidates
	var err error
	if oldConn != nil {
		// Try to reuse existing connection
		result, gatherErr := a.gatherer.GatherWithConnection(oldConn)
		if gatherErr == nil {
			a.mu.Lock()
			a.localCandidates = result.Candidates
			a.gatheringState = ICEGatheringComplete
			a.mu.Unlock()
		} else {
			err = gatherErr
		}
	}

	if err != nil || oldConn == nil {
		// Gather with new connection
		err = a.GatherCandidates()
	}

	a.mu.Lock()
	a.pendingRestart = false
	a.mu.Unlock()

	if err != nil {
		return fmt.Errorf("ice: restart gather failed: %w", err)
	}

	a.logger.Debug("ICE restart gathering complete",
		"generation", a.localCredentials.Generation,
		"candidates", len(a.localCandidates))

	return nil
}

// NeedsRestart checks if an ICE restart is needed based on current conditions.
func (a *ICEAgent) NeedsRestart() (bool, ICERestartReason) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Check if connection is failed
	if a.connectionState == ICEConnectionFailed {
		return true, ICERestartReasonAllPairsFailed
	}

	// Check if disconnected for too long
	if a.connectionState == ICEConnectionDisconnected {
		return true, ICERestartReasonTimeout
	}

	// Check connection quality
	if a.selectedPair != nil && a.qualityMonitor != nil {
		metrics := a.qualityMonitor.GetMetrics()
		if float64(metrics.Score) < a.qualityThreshold {
			return true, ICERestartReasonQualityDegraded
		}
	}

	return false, 0
}

// monitorConnectionQuality monitors the connection and triggers restart if needed.
func (a *ICEAgent) monitorConnectionQuality() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.done:
			return
		case <-ticker.C:
			needs, reason := a.NeedsRestart()
			if needs {
				if a.onRestartNeeded != nil {
					a.onRestartNeeded(reason)
				}
			}
		}
	}
}

// RecordRTT records an RTT sample for quality monitoring.
func (a *ICEAgent) RecordRTT(rtt time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.qualityMonitor != nil {
		a.qualityMonitor.RecordRTT(rtt)
	}
}

// RecordPacketLoss records packet loss for quality monitoring.
func (a *ICEAgent) RecordPacketLoss() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.qualityMonitor != nil {
		a.qualityMonitor.RecordPacketLost()
	}
}

// GetSelectedPair returns the currently selected candidate pair.
func (a *ICEAgent) GetSelectedPair() *CandidatePair {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.selectedPair
}

// GetConnection returns the UDP connection.
func (a *ICEAgent) GetConnection() *net.UDPConn {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.conn
}

// GetRestartCount returns the number of restarts.
func (a *ICEAgent) GetRestartCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.restartCount
}

// OnStateChange sets the callback for connection state changes.
func (a *ICEAgent) OnStateChange(cb func(ICEConnectionState)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onStateChange = cb
}

// OnGatheringComplete sets the callback for gathering completion.
func (a *ICEAgent) OnGatheringComplete(cb func([]*Candidate)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onGatheringComplete = cb
}

// OnSelectedPair sets the callback for pair selection.
func (a *ICEAgent) OnSelectedPair(cb func(*CandidatePair)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onSelectedPair = cb
}

// OnRestartNeeded sets the callback for restart needed events.
func (a *ICEAgent) OnRestartNeeded(cb func(ICERestartReason)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onRestartNeeded = cb
}

// Close shuts down the ICE agent.
func (a *ICEAgent) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	select {
	case <-a.done:
		return nil // Already closed
	default:
		close(a.done)
	}

	a.setConnectionStateLocked(ICEConnectionClosed)

	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}

	return nil
}

// setGatheringState sets the gathering state and logs.
func (a *ICEAgent) setGatheringState(state ICEGatheringState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.gatheringState = state
}

// setConnectionStateLocked sets connection state (must hold lock).
func (a *ICEAgent) setConnectionStateLocked(state ICEConnectionState) {
	old := a.connectionState
	a.connectionState = state

	if old != state {
		a.logger.Debug("ICE connection state changed",
			"old", old,
			"new", state)

		if a.onStateChange != nil {
			go a.onStateChange(state)
		}
	}
}

// HandleNetworkChange should be called when network conditions change.
func (a *ICEAgent) HandleNetworkChange() {
	a.mu.RLock()
	state := a.connectionState
	a.mu.RUnlock()

	if state == ICEConnectionConnected || state == ICEConnectionCompleted {
		a.logger.Info("Network change detected, checking if restart needed")

		// Check if current path is still valid
		a.mu.Lock()
		if a.selectedPair != nil {
			// Mark as disconnected until we verify connectivity
			a.setConnectionStateLocked(ICEConnectionDisconnected)
		}
		a.mu.Unlock()

		// Trigger restart evaluation
		if a.onRestartNeeded != nil {
			a.onRestartNeeded(ICERestartReasonNetworkChange)
		}
	}
}

// GetLocalCandidates returns the local candidates.
func (a *ICEAgent) GetLocalCandidates() []*Candidate {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]*Candidate, len(a.localCandidates))
	copy(result, a.localCandidates)
	return result
}

// GetRemoteCandidates returns the remote candidates.
func (a *ICEAgent) GetRemoteCandidates() []*Candidate {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]*Candidate, len(a.remoteCandidates))
	copy(result, a.remoteCandidates)
	return result
}

// GetCandidatePairs returns all candidate pairs.
func (a *ICEAgent) GetCandidatePairs() []*CandidatePair {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]*CandidatePair, len(a.candidatePairs))
	copy(result, a.candidatePairs)
	return result
}

// GetQualityMetrics returns current connection quality metrics.
func (a *ICEAgent) GetQualityMetrics() QualityMetrics {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.qualityMonitor == nil {
		return QualityMetrics{}
	}
	return a.qualityMonitor.GetMetrics()
}

// =============================================================================
// Trickle ICE Support (RFC 8838)
// =============================================================================

// IsTrickleEnabled returns whether Trickle ICE is enabled.
func (a *ICEAgent) IsTrickleEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.trickleEnabled
}

// OnLocalCandidate sets the callback for trickle candidates.
// Called whenever a new local candidate is discovered.
func (a *ICEAgent) OnLocalCandidate(cb func(*Candidate)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onLocalCandidate = cb
}

// OnEndOfCandidates sets the callback for end-of-candidates.
// Called when local gathering is complete.
func (a *ICEAgent) OnEndOfCandidates(cb func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onEndOfCandidates = cb
}

// GatherCandidatesTrickle starts gathering candidates with trickle support.
// Candidates are delivered via OnLocalCandidate callback as discovered.
// Gathering completion is signaled via OnEndOfCandidates callback.
func (a *ICEAgent) GatherCandidatesTrickle() error {
	a.mu.Lock()
	if !a.trickleEnabled {
		a.mu.Unlock()
		return fmt.Errorf("ice: trickle mode not enabled")
	}
	if a.gatheringState == ICEGatheringGathering {
		a.mu.Unlock()
		return nil // Already gathering
	}
	if a.trickleGatherer == nil {
		a.mu.Unlock()
		return fmt.Errorf("ice: trickle gatherer not initialized")
	}
	a.gatheringState = ICEGatheringGathering
	gatherer := a.trickleGatherer
	a.mu.Unlock()

	a.logger.Debug("ICE trickle gathering started",
		"generation", a.localCredentials.Generation)

	// Set up callbacks
	gatherer.OnCandidate(func(candidate *Candidate) {
		a.addLocalCandidate(candidate)
	})

	gatherer.OnGatheringComplete(func() {
		a.onLocalGatheringComplete()
	})

	// Start gathering
	return gatherer.Gather()
}

// GatherCandidatesTrickleWithConnection starts trickle gathering with existing connection.
func (a *ICEAgent) GatherCandidatesTrickleWithConnection(conn *net.UDPConn) error {
	a.mu.Lock()
	if !a.trickleEnabled {
		a.mu.Unlock()
		return fmt.Errorf("ice: trickle mode not enabled")
	}
	if a.gatheringState == ICEGatheringGathering {
		a.mu.Unlock()
		return nil
	}
	if a.trickleGatherer == nil {
		a.mu.Unlock()
		return fmt.Errorf("ice: trickle gatherer not initialized")
	}
	a.gatheringState = ICEGatheringGathering
	a.conn = conn
	gatherer := a.trickleGatherer
	a.mu.Unlock()

	a.logger.Debug("ICE trickle gathering started with existing connection",
		"generation", a.localCredentials.Generation)

	// Set up callbacks
	gatherer.OnCandidate(func(candidate *Candidate) {
		a.addLocalCandidate(candidate)
	})

	gatherer.OnGatheringComplete(func() {
		a.onLocalGatheringComplete()
	})

	return gatherer.GatherWithConnection(conn)
}

// addLocalCandidate adds a locally discovered candidate.
func (a *ICEAgent) addLocalCandidate(candidate *Candidate) {
	a.mu.Lock()
	// Check for duplicates
	for _, c := range a.localCandidates {
		if c.Addr.String() == candidate.Addr.String() && c.Type == candidate.Type {
			a.mu.Unlock()
			return
		}
	}
	a.localCandidates = append(a.localCandidates, candidate)

	// Create pairs with existing remote candidates
	for _, remote := range a.remoteCandidates {
		pair := NewCandidatePair(candidate, remote, a.isControlling)
		a.candidatePairs = append(a.candidatePairs, pair)
	}
	SortCandidatePairs(a.candidatePairs)

	cb := a.onLocalCandidate
	checksStarted := a.checksStarted
	a.mu.Unlock()

	a.logger.Debug("ICE local candidate discovered",
		"type", candidate.Type,
		"addr", candidate.Addr)

	// Notify callback
	if cb != nil {
		cb(candidate)
	}

	// If checks already started, try new pair immediately
	if checksStarted {
		go a.checkNewPairs()
	}
}

// onLocalGatheringComplete handles local gathering completion.
func (a *ICEAgent) onLocalGatheringComplete() {
	a.mu.Lock()
	a.gatheringState = ICEGatheringComplete

	// Get connection from trickle gatherer if we don't have one
	if a.conn == nil && a.trickleGatherer != nil {
		a.conn = a.trickleGatherer.GetLocalConn()
	}

	candidates := a.localCandidates
	cb := a.onEndOfCandidates
	gcb := a.onGatheringComplete
	a.mu.Unlock()

	a.logger.Debug("ICE trickle gathering complete",
		"candidates", len(candidates))

	// Notify callbacks
	if cb != nil {
		cb()
	}
	if gcb != nil {
		gcb(candidates)
	}
}

// AddRemoteCandidateTrickle adds a remote candidate received via trickle.
// Can be called during connectivity checks.
func (a *ICEAgent) AddRemoteCandidateTrickle(candidate *Candidate) {
	a.mu.Lock()

	// Check for duplicates
	for _, c := range a.remoteCandidates {
		if c.Addr.String() == candidate.Addr.String() && c.Type == candidate.Type {
			a.mu.Unlock()
			return
		}
	}

	a.remoteCandidates = append(a.remoteCandidates, candidate)

	// Create pairs with existing local candidates
	for _, local := range a.localCandidates {
		pair := NewCandidatePair(local, candidate, a.isControlling)
		a.candidatePairs = append(a.candidatePairs, pair)
	}
	SortCandidatePairs(a.candidatePairs)

	checksStarted := a.checksStarted
	a.mu.Unlock()

	a.logger.Debug("ICE remote trickle candidate added",
		"type", candidate.Type,
		"addr", candidate.Addr)

	// If checks already started, try new pair immediately
	if checksStarted {
		go a.checkNewPairs()
	}
}

// SetRemoteGatheringDone marks remote gathering as complete.
// Called when receiving end-of-candidates signal from remote.
func (a *ICEAgent) SetRemoteGatheringDone() {
	a.mu.Lock()
	a.remoteGatheringDone = true
	a.mu.Unlock()

	a.logger.Debug("ICE remote gathering marked as done")
}

// StartConnectivityChecksTrickle starts connectivity checks in trickle mode.
// Can be called before all candidates are gathered.
func (a *ICEAgent) StartConnectivityChecksTrickle() error {
	a.mu.Lock()
	if !a.trickleEnabled {
		a.mu.Unlock()
		return fmt.Errorf("ice: trickle mode not enabled")
	}
	if a.checksStarted {
		a.mu.Unlock()
		return nil // Already started
	}
	if a.connectionState != ICEConnectionNew && a.connectionState != ICEConnectionDisconnected {
		a.mu.Unlock()
		return fmt.Errorf("ice: invalid state for checks: %s", a.connectionState)
	}

	a.checksStarted = true
	a.setConnectionStateLocked(ICEConnectionChecking)
	conn := a.conn
	pairs := make([]*CandidatePair, len(a.candidatePairs))
	copy(pairs, a.candidatePairs)
	a.mu.Unlock()

	if conn == nil {
		// Wait for connection to be available
		a.logger.Debug("ICE trickle checks waiting for connection")
		go a.waitForConnectionAndCheck()
		return nil
	}

	a.logger.Debug("ICE trickle connectivity checks started",
		"pairs", len(pairs))

	go a.runConnectivityChecksTrickle()

	return nil
}

// waitForConnectionAndCheck waits for connection and starts checks.
func (a *ICEAgent) waitForConnectionAndCheck() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(10 * time.Second)

	for {
		select {
		case <-a.done:
			return
		case <-timeout:
			a.logger.Warn("ICE trickle: timeout waiting for connection")
			a.mu.Lock()
			a.setConnectionStateLocked(ICEConnectionFailed)
			a.mu.Unlock()
			return
		case <-ticker.C:
			a.mu.RLock()
			conn := a.conn
			a.mu.RUnlock()

			if conn != nil {
				go a.runConnectivityChecksTrickle()
				return
			}
		}
	}
}

// runConnectivityChecksTrickle runs connectivity checks in trickle mode.
func (a *ICEAgent) runConnectivityChecksTrickle() {
	for {
		select {
		case <-a.done:
			return
		default:
		}

		a.mu.RLock()
		if a.connectionState == ICEConnectionConnected ||
			a.connectionState == ICEConnectionCompleted ||
			a.connectionState == ICEConnectionFailed ||
			a.connectionState == ICEConnectionClosed {
			a.mu.RUnlock()
			return
		}

		// Find next pair to check
		var pairToCheck *CandidatePair
		for _, pair := range a.candidatePairs {
			if pair.State == CandidatePairFrozen || pair.State == CandidatePairWaiting {
				pairToCheck = pair
				break
			}
		}
		conn := a.conn
		localVIP := a.localVIP
		checkTimeout := a.checkTimeout
		a.mu.RUnlock()

		if pairToCheck == nil {
			// No pairs to check, wait for more candidates or check if we're done
			a.mu.RLock()
			allFailed := true
			for _, pair := range a.candidatePairs {
				if pair.State != CandidatePairFailed {
					allFailed = false
					break
				}
			}
			localDone := a.gatheringState == ICEGatheringComplete
			remoteDone := a.remoteGatheringDone
			a.mu.RUnlock()

			if allFailed && localDone && remoteDone {
				// All pairs failed and gathering is complete
				a.mu.Lock()
				a.setConnectionStateLocked(ICEConnectionFailed)
				a.mu.Unlock()

				a.logger.Warn("ICE trickle: all pairs failed")
				if a.onRestartNeeded != nil {
					a.onRestartNeeded(ICERestartReasonAllPairsFailed)
				}
				return
			}

			// Wait for new candidates
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if conn == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Mark pair as in progress
		a.mu.Lock()
		pairToCheck.State = CandidatePairInProgress
		a.mu.Unlock()

		// Try to establish connectivity
		holePuncher := NewHolePuncher(conn, localVIP, checkTimeout, a.logger)
		ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
		result, err := holePuncher.SimultaneousPunch(ctx, pairToCheck.Remote.Addr.IP, []*Candidate{pairToCheck.Remote})
		cancel()

		if err == nil && result != nil {
			// Success!
			a.mu.Lock()
			pairToCheck.State = CandidatePairSucceeded
			a.selectedPair = pairToCheck
			a.setConnectionStateLocked(ICEConnectionConnected)
			a.mu.Unlock()

			a.logger.Info("ICE trickle connectivity check succeeded",
				"local", pairToCheck.Local.Addr,
				"remote", pairToCheck.Remote.Addr,
				"rtt", result.RTT)

			if a.onSelectedPair != nil {
				a.onSelectedPair(pairToCheck)
			}

			go a.monitorConnectionQuality()
			return
		}

		// Mark as failed
		a.mu.Lock()
		pairToCheck.State = CandidatePairFailed
		a.mu.Unlock()
	}
}

// checkNewPairs attempts connectivity checks on newly added pairs.
func (a *ICEAgent) checkNewPairs() {
	a.mu.RLock()
	if a.connectionState == ICEConnectionConnected ||
		a.connectionState == ICEConnectionCompleted {
		a.mu.RUnlock()
		return // Already connected
	}
	a.mu.RUnlock()

	// The main trickle check loop will pick up new pairs
	// This function exists to potentially trigger immediate checking
}
