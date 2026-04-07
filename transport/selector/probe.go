package selector

import (
	"context"
	"time"

	"github.com/shuttleX/shuttle/transport"
)

// Probe tests a transport's availability and performance.
// timeout controls how long the probe dial may take; 0 uses the default of 5s.
func Probe(ctx context.Context, t transport.ClientTransport, timeout time.Duration) *ProbeResult {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	result := &ProbeResult{
		Transport: t,
		LastCheck: time.Now(),
	}

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	conn, err := t.Dial(probeCtx, "")
	if err != nil {
		result.Available = false
		result.Latency = timeout
		result.Loss = 1.0
		return result
	}
	defer conn.Close()

	result.Available = true
	result.Latency = time.Since(start)
	result.Loss = 0
	return result
}
