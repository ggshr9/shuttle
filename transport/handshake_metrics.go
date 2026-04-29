package transport

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"time"
)

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

// ClassifyHandshakeReason maps a handshake error into one of the metric
// reason categories: "timeout" or "protocol". Auth failures across all
// transports (HMAC verify, Noise rejection) are detected inline and
// reported with reason="auth" by the caller without going through this
// helper, so the result here is "timeout" for deadline-derived errors
// and "protocol" for everything else (including io.EOF and unexpected
// stream termination).
func ClassifyHandshakeReason(err error) string {
	if err == nil {
		return "protocol"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "protocol"
	}
	return "protocol"
}
