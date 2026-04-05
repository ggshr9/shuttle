package engine

import (
	"log/slog"
	"time"
)

const eventChannelBuffer = 64

// Subscribe returns a channel that receives real-time engine events.
// The channel is buffered. Slow consumers will miss events.
func (e *Engine) Subscribe() chan Event {
	ch := make(chan Event, eventChannelBuffer)
	e.subMu.Lock()
	e.subs[ch] = struct{}{}
	e.subMu.Unlock()
	return ch
}

// Unsubscribe removes and closes a previously subscribed channel.
func (e *Engine) Unsubscribe(ch chan Event) {
	e.subMu.Lock()
	defer e.subMu.Unlock()
	if _, ok := e.subs[ch]; ok {
		delete(e.subs, ch)
		close(ch)
	}
}

func (e *Engine) emit(ev Event) {
	ev.Timestamp = time.Now()
	e.logger.Debug("emitting event", slog.String("type", ev.Type.String()))

	// Snapshot subscribers under read lock, then release before sending.
	// This prevents deadlock when a subscriber callback triggers Unsubscribe.
	e.subMu.RLock()
	snapshot := make([]chan Event, 0, len(e.subs))
	for ch := range e.subs {
		snapshot = append(snapshot, ch)
	}
	e.subMu.RUnlock()

	for _, ch := range snapshot {
		// The channel may have been closed by Unsubscribe after the
		// snapshot was taken. Recover from the resulting panic rather
		// than holding the lock during send (which risks deadlock).
		func() {
			defer func() { recover() }()
			select {
			case ch <- ev:
			default:
			}
		}()
	}
}

// EmitConnectionEvent emits a connection event to all subscribers.
// This is used by plugins to report connection open/close events.
func (e *Engine) EmitConnectionEvent(connID, state, target, rule, protocol, processName string, bytesIn, bytesOut, durationMs int64) {
	e.logger.Debug("connection state change", "conn_id", connID, "state", state, "target", target)
	e.emit(Event{
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
