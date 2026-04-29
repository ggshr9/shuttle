package reality

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
)

// classifyReason maps a handshake error into one of the metric reason
// categories: "timeout", "auth", or "protocol". Reality handles auth
// failures inline (noise rejection) and uses this helper for raw I/O
// errors during the handshake — connection drops, deadlines, framing
// errors that are not classifiable as auth.
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
