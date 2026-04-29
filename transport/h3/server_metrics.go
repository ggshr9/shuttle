package h3

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
)

// classifyReason maps a handshake error into one of the metric reason
// categories: "timeout", "auth", or "protocol". For h3, auth errors are
// surfaced as explicit string failures inline (replay/HMAC) rather than
// errors flowing through here, so this helper covers stream/transport
// errors that occur before or during the auth read.
func classifyReason(err error) string {
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
