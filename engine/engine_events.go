package engine

// eventChannelBuffer is kept for backward compatibility with tests and
// external code. It mirrors observabilityChannelBuffer.
const eventChannelBuffer = observabilityChannelBuffer

// Subscribe returns a channel that receives real-time engine events.
// The channel is buffered. Slow consumers will miss events.
func (e *Engine) Subscribe() <-chan Event {
	return e.obs.Subscribe()
}

// Unsubscribe removes and closes a previously subscribed channel.
func (e *Engine) Unsubscribe(ch <-chan Event) {
	e.obs.Unsubscribe(ch)
}

func (e *Engine) emit(ev Event) {
	e.obs.Emit(ev)
}

// EmitConnectionEvent emits a connection event to all subscribers.
// This is used by plugins to report connection open/close events.
func (e *Engine) EmitConnectionEvent(connID, state, target, rule, protocol, processName string, bytesIn, bytesOut, durationMs int64) {
	e.obs.EmitConnectionEvent(connID, state, target, rule, protocol, processName, bytesIn, bytesOut, durationMs)
}
