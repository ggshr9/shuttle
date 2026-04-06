package engine

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/shuttleX/shuttle/plugin"
)

const observabilityChannelBuffer = 64

// ObservabilityManager owns metrics collection, event emission, the plugin
// chain, speed sampling, and background goroutine tracking. It extracts these
// concerns from Engine so they can be tested and reasoned about independently.
type ObservabilityManager struct {
	metrics *plugin.Metrics
	chain   *plugin.Chain
	logger  *slog.Logger

	subMu sync.RWMutex
	subs  map[chan Event]struct{}

	bgWg sync.WaitGroup
}

// NewObservabilityManager creates a new ObservabilityManager with fresh metrics.
// If logger is nil, slog.Default() is used.
func NewObservabilityManager(logger *slog.Logger) *ObservabilityManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &ObservabilityManager{
		metrics: plugin.NewMetrics(),
		logger:  logger,
		subs:    make(map[chan Event]struct{}),
	}
}

// Metrics returns the metrics collector.
func (om *ObservabilityManager) Metrics() *plugin.Metrics {
	return om.metrics
}

// Chain returns the current plugin chain, or nil if not built.
func (om *ObservabilityManager) Chain() *plugin.Chain {
	return om.chain
}

// BuildChain constructs and initialises the plugin chain (metrics, connection
// tracker, logger). The emitter is used by the connection tracker to emit
// lifecycle events — typically the Engine or ObservabilityManager itself.
func (om *ObservabilityManager) BuildChain(ctx context.Context, emitter plugin.ConnEmitter) error {
	connTracker := plugin.NewConnTracker(emitter)
	chain := plugin.NewChain(
		om.metrics,
		connTracker,
		plugin.NewLogger(om.logger),
	)
	if err := chain.Init(ctx); err != nil {
		return err
	}
	om.chain = chain
	return nil
}

// CloseChain closes the plugin chain if one exists and nils the reference.
func (om *ObservabilityManager) CloseChain() {
	if om.chain != nil {
		om.chain.Close()
		om.chain = nil
	}
}

// Subscribe returns a receive-only channel that receives real-time events.
// The channel is buffered; slow consumers will miss events (non-blocking send).
func (om *ObservabilityManager) Subscribe() <-chan Event {
	ch := make(chan Event, observabilityChannelBuffer)
	om.subMu.Lock()
	om.subs[ch] = struct{}{}
	om.subMu.Unlock()
	return ch
}

// Unsubscribe removes and closes a previously subscribed channel. It accepts
// the receive-only channel returned by Subscribe and iterates the map to find
// the matching bidirectional channel.
func (om *ObservabilityManager) Unsubscribe(ch <-chan Event) {
	om.subMu.Lock()
	defer om.subMu.Unlock()
	for bidi := range om.subs {
		// A receive-only channel derived from a bidirectional channel compares
		// equal to that bidirectional channel when both are cast to <-chan.
		if (<-chan Event)(bidi) == ch {
			delete(om.subs, bidi)
			close(bidi)
			return
		}
	}
}

// Emit sends an event to all subscribers. It is non-blocking: if a subscriber's
// channel buffer is full the event is dropped for that subscriber.
func (om *ObservabilityManager) Emit(ev Event) { //nolint:gocritic // hugeParam: Event is stack-allocated, hot path
	ev.Timestamp = time.Now()
	om.logger.Debug("emitting event", slog.String("type", ev.Type.String()))

	// Hold RLock for the entire send loop. Since sends are non-blocking
	// (select with default), this cannot deadlock. Unsubscribe acquires a
	// write lock and will wait until Emit finishes, ensuring close(ch) never
	// races with a send.
	om.subMu.RLock()
	defer om.subMu.RUnlock()

	for ch := range om.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// EmitConnectionEvent is a convenience method that constructs and emits a
// connection lifecycle event. It satisfies the plugin.ConnEmitter interface.
func (om *ObservabilityManager) EmitConnectionEvent(connID, state, target, rule, protocol, processName string, bytesIn, bytesOut, durationMs int64) {
	om.logger.Debug("connection state change", "conn_id", connID, "state", state, "target", target)
	om.Emit(Event{
		Type:        EventConnection,
		ConnID:      connID,
		ConnState:   state,
		Target:      target,
		Rule:        rule,
		Protocol:    protocol,
		ProcessName: processName,
		BytesIn:     bytesIn,
		BytesOut:    bytesOut,
		DurationMs:  durationMs,
	})
}

// StartSpeedLoop launches a background goroutine that samples upload/download
// speed every second and emits EventSpeedTick events. It stops when ctx is
// cancelled. The goroutine is tracked by the internal WaitGroup.
func (om *ObservabilityManager) StartSpeedLoop(ctx context.Context) {
	om.bgWg.Add(1)
	go func() {
		defer om.bgWg.Done()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				up, down := om.metrics.SampleSpeed()
				om.Emit(Event{
					Type:     EventSpeedTick,
					Upload:   up,
					Download: down,
				})
			}
		}
	}()
}

// WaitBackground blocks until all background goroutines tracked by this
// manager have exited.
func (om *ObservabilityManager) WaitBackground() {
	om.bgWg.Wait()
}

// Verify ObservabilityManager satisfies plugin.ConnEmitter at compile time.
var _ plugin.ConnEmitter = (*ObservabilityManager)(nil)
