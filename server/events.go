package server

import (
	"sync"
	"time"
)

const eventChannelBuffer = 64

// ServerEvent represents a real-time event emitted by the server.
type ServerEvent struct {
	Type      string    `json:"type"`
	Message   string    `json:"message,omitempty"`
	ConnID    string    `json:"conn_id,omitempty"`
	Target    string    `json:"target,omitempty"`
	User      string    `json:"user,omitempty"`
	BytesIn   int64     `json:"bytes_in,omitempty"`
	BytesOut  int64     `json:"bytes_out,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// ServerEventBus provides a publish-subscribe mechanism for server events.
// It follows the same pattern as the client's ObservabilityManager:
// non-blocking send, snapshot subscribers under read lock, recover from
// closed channel panics.
type ServerEventBus struct {
	mu   sync.RWMutex
	subs map[chan ServerEvent]struct{}
}

// NewServerEventBus creates a new event bus ready for subscriptions.
func NewServerEventBus() *ServerEventBus {
	return &ServerEventBus{
		subs: make(map[chan ServerEvent]struct{}),
	}
}

// Subscribe returns a receive-only channel that receives real-time events.
// The channel is buffered; slow consumers will miss events (non-blocking send).
func (eb *ServerEventBus) Subscribe() <-chan ServerEvent {
	ch := make(chan ServerEvent, eventChannelBuffer)
	eb.mu.Lock()
	eb.subs[ch] = struct{}{}
	eb.mu.Unlock()
	return ch
}

// Unsubscribe removes and closes a previously subscribed channel. It accepts
// the receive-only channel returned by Subscribe and iterates the map to find
// the matching bidirectional channel.
func (eb *ServerEventBus) Unsubscribe(ch <-chan ServerEvent) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	for bidi := range eb.subs {
		if (<-chan ServerEvent)(bidi) == ch {
			delete(eb.subs, bidi)
			close(bidi)
			return
		}
	}
}

// Emit sends an event to all subscribers. It is non-blocking: if a subscriber's
// channel buffer is full the event is dropped for that subscriber. Closed
// channels (from a racing Unsubscribe) are handled gracefully via recover.
func (eb *ServerEventBus) Emit(ev ServerEvent) { //nolint:gocritic // hugeParam: hot path event emission
	ev.Timestamp = time.Now()

	// Snapshot subscribers under read lock, then release before sending.
	eb.mu.RLock()
	snapshot := make([]chan ServerEvent, 0, len(eb.subs))
	for ch := range eb.subs {
		snapshot = append(snapshot, ch)
	}
	eb.mu.RUnlock()

	for _, ch := range snapshot {
		func() {
			defer func() { _ = recover() }()
			select {
			case ch <- ev:
			default:
			}
		}()
	}
}
