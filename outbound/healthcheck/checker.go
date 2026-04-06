package healthcheck

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

// DialFunc creates a connection through a specific outbound.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// DirectDialer returns a DialFunc that dials directly (for testing).
func DirectDialer() DialFunc {
	d := &net.Dialer{}
	return d.DialContext
}

// Config configures the health checker.
type Config struct {
	URL       string        // default: "http://www.gstatic.com/generate_204"
	Interval  time.Duration // default: 300s
	Timeout   time.Duration // default: 5s
	Tolerance int           // consecutive failures before marking down, default: 3
	Lazy      bool          // only check when used
}

// Result is the outcome of a single health check.
type Result struct {
	Latency   time.Duration
	Available bool
	UpdatedAt time.Time
}

// nodeState tracks the health state of a single outbound node.
type nodeState struct {
	mu              sync.Mutex
	lastResult      Result
	consecutiveFail int
}

// Checker performs HTTP health checks against outbounds.
type Checker struct {
	cfg  Config
	mu   sync.RWMutex
	nodes map[string]*nodeState // tag → state
}

const (
	defaultURL       = "http://www.gstatic.com/generate_204"
	defaultInterval  = 300 * time.Second
	defaultTimeout   = 5 * time.Second
	defaultTolerance = 3
)

// New creates a Checker, applying defaults to any zero-value fields.
func New(cfg *Config) *Checker {
	c := &Checker{
		nodes: make(map[string]*nodeState),
	}
	if cfg != nil {
		c.cfg = *cfg
	}
	if c.cfg.URL == "" {
		c.cfg.URL = defaultURL
	}
	if c.cfg.Interval == 0 {
		c.cfg.Interval = defaultInterval
	}
	if c.cfg.Timeout == 0 {
		c.cfg.Timeout = defaultTimeout
	}
	if c.cfg.Tolerance == 0 {
		c.cfg.Tolerance = defaultTolerance
	}
	return c
}

// getOrCreate retrieves (or lazily creates) the nodeState for a tag.
func (c *Checker) getOrCreate(tag string) *nodeState {
	c.mu.RLock()
	ns, ok := c.nodes[tag]
	c.mu.RUnlock()
	if ok {
		return ns
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// Double-check after acquiring write lock.
	if ns, ok = c.nodes[tag]; ok {
		return ns
	}
	ns = &nodeState{}
	c.nodes[tag] = ns
	return ns
}

// Check performs a single HTTP health check for the given tag using the provided
// DialFunc, records the result, and returns it.
func (c *Checker) Check(ctx context.Context, tag string, dial DialFunc) Result {
	transport := &http.Transport{
		DialContext: dial,
	}
	client := &http.Client{
		Transport: transport,
		// Disable redirects — any redirect counts as a failure.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: c.cfg.Timeout,
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.URL, nil)
	available := false
	var latency time.Duration

	if err == nil {
		var resp *http.Response
		resp, err = client.Do(req)
		latency = time.Since(start)
		if err == nil {
			resp.Body.Close()
			available = resp.StatusCode >= 200 && resp.StatusCode < 400
		}
	}

	if err != nil {
		latency = time.Since(start)
	}

	result := Result{
		Latency:   latency,
		Available: available,
		UpdatedAt: time.Now(),
	}

	// Record in nodeState.
	ns := c.getOrCreate(tag)
	ns.mu.Lock()
	ns.lastResult = result
	if available {
		ns.consecutiveFail = 0
	} else {
		ns.consecutiveFail++
	}
	ns.mu.Unlock()

	return result
}

// Results returns all recorded results. The Available field is set to false
// if the node's consecutive failure count has reached the configured tolerance.
func (c *Checker) Results() map[string]Result {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make(map[string]Result, len(c.nodes))
	for tag, ns := range c.nodes {
		ns.mu.Lock()
		r := ns.lastResult
		if ns.consecutiveFail >= c.cfg.Tolerance {
			r.Available = false
		}
		ns.mu.Unlock()
		out[tag] = r
	}
	return out
}

// Result returns the recorded result for a single tag, applying the same
// tolerance logic as Results(). Returns false if the tag is unknown.
func (c *Checker) Result(tag string) (Result, bool) {
	c.mu.RLock()
	ns, ok := c.nodes[tag]
	c.mu.RUnlock()
	if !ok {
		return Result{}, false
	}

	ns.mu.Lock()
	r := ns.lastResult
	if ns.consecutiveFail >= c.cfg.Tolerance {
		r.Available = false
	}
	ns.mu.Unlock()
	return r, true
}

// Cfg returns the effective configuration.
func (c *Checker) Cfg() Config {
	return c.cfg
}
