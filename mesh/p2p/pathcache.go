package p2p

import (
	"net"
	"sync"
	"time"
)

// PathCache remembers successful connection paths to peers
// This enables faster reconnection by trying the last working method first
type PathCache struct {
	mu      sync.RWMutex
	entries map[string]*PathEntry
	maxAge  time.Duration
}

// PathEntry stores information about a successful connection path
type PathEntry struct {
	PeerVIP      net.IP
	RemoteAddr   *net.UDPAddr
	Method       ConnectionMethod
	RTT          time.Duration
	SuccessCount int
	FailCount    int
	LastSuccess  time.Time
	LastAttempt  time.Time
}

// ConnectionMethod indicates how the connection was established
type ConnectionMethod int

const (
	MethodUnknown ConnectionMethod = iota
	MethodUPnP                     // Both sides used UPnP
	MethodNATPMP                   // Both sides used NAT-PMP
	MethodSTUN                     // STUN-based hole punching
	MethodDirect                   // Direct connection (same LAN)
	MethodRelay                    // Server relay
)

func (m ConnectionMethod) String() string {
	switch m {
	case MethodUPnP:
		return "upnp"
	case MethodNATPMP:
		return "nat-pmp"
	case MethodSTUN:
		return "stun"
	case MethodDirect:
		return "direct"
	case MethodRelay:
		return "relay"
	default:
		return "unknown"
	}
}

// NewPathCache creates a new path cache
func NewPathCache(maxAge time.Duration) *PathCache {
	if maxAge == 0 {
		maxAge = 24 * time.Hour
	}
	return &PathCache{
		entries: make(map[string]*PathEntry),
		maxAge:  maxAge,
	}
}

// Get retrieves the cached path for a peer
func (c *PathCache) Get(peerVIP net.IP) *PathEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := peerVIP.String()
	entry, ok := c.entries[key]
	if !ok {
		return nil
	}

	// Check if entry is too old
	if time.Since(entry.LastSuccess) > c.maxAge {
		return nil
	}

	return entry
}

// RecordSuccess records a successful connection
func (c *PathCache) RecordSuccess(peerVIP net.IP, remoteAddr *net.UDPAddr, method ConnectionMethod, rtt time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := peerVIP.String()
	entry, ok := c.entries[key]
	if !ok {
		entry = &PathEntry{
			PeerVIP: peerVIP,
		}
		c.entries[key] = entry
	}

	entry.RemoteAddr = remoteAddr
	entry.Method = method
	entry.RTT = rtt
	entry.SuccessCount++
	entry.LastSuccess = time.Now()
	entry.LastAttempt = time.Now()
}

// RecordFailure records a failed connection attempt
func (c *PathCache) RecordFailure(peerVIP net.IP) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := peerVIP.String()
	entry, ok := c.entries[key]
	if !ok {
		entry = &PathEntry{
			PeerVIP: peerVIP,
		}
		c.entries[key] = entry
	}

	entry.FailCount++
	entry.LastAttempt = time.Now()
}

// GetBestMethod returns the best connection method based on history
func (c *PathCache) GetBestMethod(peerVIP net.IP) ConnectionMethod {
	entry := c.Get(peerVIP)
	if entry == nil {
		return MethodUnknown
	}

	// If success rate is too low, don't recommend
	total := entry.SuccessCount + entry.FailCount
	if total > 3 && float64(entry.SuccessCount)/float64(total) < 0.5 {
		return MethodUnknown
	}

	return entry.Method
}

// Clear removes all entries
func (c *PathCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*PathEntry)
}

// Cleanup removes expired entries
func (c *PathCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.Sub(entry.LastSuccess) > c.maxAge {
			delete(c.entries, key)
		}
	}
}

// Stats returns cache statistics
func (c *PathCache) Stats() PathCacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := PathCacheStats{
		TotalEntries: len(c.entries),
		ByMethod:     make(map[ConnectionMethod]int),
	}

	for _, entry := range c.entries {
		stats.ByMethod[entry.Method]++
	}

	return stats
}

// PathCacheStats contains cache statistics
type PathCacheStats struct {
	TotalEntries int
	ByMethod     map[ConnectionMethod]int
}

// ConnectionQuality tracks connection quality metrics
type ConnectionQuality struct {
	mu sync.RWMutex

	// Sliding window for RTT measurements
	rttSamples []time.Duration
	maxSamples int

	// Packet statistics
	packetsSent     uint64
	packetsReceived uint64
	packetsLost     uint64

	// Jitter tracking
	lastRTT time.Duration
	jitter  time.Duration
}

// NewConnectionQuality creates a new quality tracker
func NewConnectionQuality() *ConnectionQuality {
	return &ConnectionQuality{
		rttSamples: make([]time.Duration, 0, 100),
		maxSamples: 100,
	}
}

// RecordRTT adds an RTT sample
func (q *ConnectionQuality) RecordRTT(rtt time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Calculate jitter (variation from last RTT)
	if q.lastRTT > 0 {
		diff := rtt - q.lastRTT
		if diff < 0 {
			diff = -diff
		}
		// Exponential moving average for jitter
		q.jitter = (q.jitter*15 + diff) / 16
	}
	q.lastRTT = rtt

	// Add to sliding window
	if len(q.rttSamples) >= q.maxSamples {
		q.rttSamples = q.rttSamples[1:]
	}
	q.rttSamples = append(q.rttSamples, rtt)
}

// RecordPacketSent records a sent packet
func (q *ConnectionQuality) RecordPacketSent() {
	q.mu.Lock()
	q.packetsSent++
	q.mu.Unlock()
}

// RecordPacketReceived records a received packet
func (q *ConnectionQuality) RecordPacketReceived() {
	q.mu.Lock()
	q.packetsReceived++
	q.mu.Unlock()
}

// RecordPacketLost records a lost packet
func (q *ConnectionQuality) RecordPacketLost() {
	q.mu.Lock()
	q.packetsLost++
	q.mu.Unlock()
}

// GetMetrics returns current quality metrics
func (q *ConnectionQuality) GetMetrics() QualityMetrics {
	q.mu.RLock()
	defer q.mu.RUnlock()

	metrics := QualityMetrics{
		PacketsSent:     q.packetsSent,
		PacketsReceived: q.packetsReceived,
		PacketsLost:     q.packetsLost,
		Jitter:          q.jitter,
	}

	// Calculate average RTT
	if len(q.rttSamples) > 0 {
		var total time.Duration
		for _, rtt := range q.rttSamples {
			total += rtt
		}
		metrics.AvgRTT = total / time.Duration(len(q.rttSamples))

		// Find min and max
		metrics.MinRTT = q.rttSamples[0]
		metrics.MaxRTT = q.rttSamples[0]
		for _, rtt := range q.rttSamples[1:] {
			if rtt < metrics.MinRTT {
				metrics.MinRTT = rtt
			}
			if rtt > metrics.MaxRTT {
				metrics.MaxRTT = rtt
			}
		}
	}

	// Calculate loss rate
	if q.packetsSent > 0 {
		metrics.LossRate = float64(q.packetsLost) / float64(q.packetsSent)
	}

	// Calculate quality score (0-100)
	metrics.Score = q.calculateScore(metrics)

	return metrics
}

// calculateScore computes a quality score from 0-100
func (q *ConnectionQuality) calculateScore(m QualityMetrics) int {
	score := 100

	// RTT penalty (each 50ms over 50ms costs 10 points)
	if m.AvgRTT > 50*time.Millisecond {
		excess := (m.AvgRTT - 50*time.Millisecond) / (50 * time.Millisecond)
		score -= int(excess) * 10
	}

	// Jitter penalty (each 20ms of jitter costs 5 points)
	jitterPenalty := int(m.Jitter/(20*time.Millisecond)) * 5
	score -= jitterPenalty

	// Loss rate penalty (each 1% loss costs 10 points)
	lossPenalty := int(m.LossRate * 1000)
	score -= lossPenalty

	if score < 0 {
		score = 0
	}
	return score
}

// QualityMetrics contains connection quality information
type QualityMetrics struct {
	AvgRTT          time.Duration
	MinRTT          time.Duration
	MaxRTT          time.Duration
	Jitter          time.Duration
	PacketsSent     uint64
	PacketsReceived uint64
	PacketsLost     uint64
	LossRate        float64
	Score           int // 0-100, higher is better
}

// IsGood returns true if the connection quality is acceptable
func (m QualityMetrics) IsGood() bool {
	return m.Score >= 50 && m.LossRate < 0.05
}
