package selector

import (
	"context"
	"time"

	"github.com/shuttle-proxy/shuttle/transport"
)

// Probe tests a transport's availability and performance.
func Probe(ctx context.Context, t transport.ClientTransport) *ProbeResult {
	result := &ProbeResult{
		Transport: t,
		LastCheck: time.Now(),
	}

	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := time.Now()
	conn, err := t.Dial(probeCtx, "")
	if err != nil {
		result.Available = false
		result.Latency = 5 * time.Second
		result.Loss = 1.0
		return result
	}
	defer conn.Close()

	result.Available = true
	result.Latency = time.Since(start)
	result.Loss = 0
	return result
}
