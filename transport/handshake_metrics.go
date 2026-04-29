package transport

import "time"

// HandshakeMetrics is the optional hook called by server-side transports
// after each completed (or failed) accept. It is set once at server startup
// and passed through transport-specific factories/options.
//
// OnSuccess fires after a fully authenticated handshake (Noise/HMAC verified
// and any post-quantum exchange complete). The duration is measured from
// the start of accept handling on a single raw connection.
//
// OnFailure fires when the handshake terminates for any reason other than
// success — auth failure, protocol error, timeout, etc. The reason should
// be one of "timeout", "auth", "protocol".
type HandshakeMetrics struct {
	OnSuccess func(transport string, duration time.Duration)
	OnFailure func(transport string, reason string)
}
