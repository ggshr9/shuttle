package subscription

import (
	"context"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/shuttleX/shuttle/config"
)

// SpeedResult holds the result of a speed test for one server.
type SpeedResult struct {
	Server  config.ServerEndpoint
	Latency time.Duration
	Error   error
}

// SpeedTestAll tests all servers in parallel with limited concurrency.
// Returns results sorted by latency ascending; entries with errors are placed last.
// If concurrency is <= 0, it defaults to 10.
func SpeedTestAll(ctx context.Context, servers []config.ServerEndpoint, timeout time.Duration, concurrency int) []SpeedResult {
	if concurrency <= 0 {
		concurrency = 10
	}

	results := make([]SpeedResult, len(servers))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, srv := range servers {
		wg.Add(1)
		go func(idx int, s config.ServerEndpoint) {
			defer wg.Done()

			// Acquire semaphore slot.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[idx] = SpeedResult{Server: s, Error: ctx.Err()}
				return
			}
			defer func() { <-sem }()

			start := time.Now()
			conn, err := net.DialTimeout("tcp", s.Addr, timeout)
			latency := time.Since(start)
			if err != nil {
				results[idx] = SpeedResult{Server: s, Error: err}
				return
			}
			conn.Close()
			results[idx] = SpeedResult{Server: s, Latency: latency}
		}(i, srv)
	}

	wg.Wait()

	sort.SliceStable(results, func(i, j int) bool {
		iErr := results[i].Error != nil
		jErr := results[j].Error != nil
		if iErr != jErr {
			// Errors go last.
			return !iErr
		}
		if iErr {
			// Both errors — preserve original order.
			return false
		}
		return results[i].Latency < results[j].Latency
	})

	return results
}
