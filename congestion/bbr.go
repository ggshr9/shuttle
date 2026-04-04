package congestion

import (
	"math"
	"sync"
	"time"
)

// BBRState represents the current BBR state machine phase.
type BBRState int

const (
	BBRStartup BBRState = iota
	BBRDrain
	BBRProbeBW
	BBRProbeRTT
)

// BBRController implements Google's BBR congestion control algorithm.
type BBRController struct {
	mu sync.Mutex

	// Estimated bottleneck bandwidth (bytes per second).
	btlBw uint64
	// Minimum RTT observed.
	rtProp time.Duration
	// Current pacing rate.
	pacingRate uint64
	// Congestion window in bytes.
	cwnd uint64

	state BBRState

	// Bandwidth filter: max over last 10 RTTs.
	bwFilter    []uint64
	bwFilterIdx int

	// RTT probe timer.
	rtPropExpiry  time.Time
	rtPropStamp   time.Time
	probeRTTDone  bool
	probeRTTRound uint64

	// ProbeBW pacing gain cycle index (0-7).
	cycleIdx   int
	cycleStart time.Time

	// Round counting.
	roundCount   uint64
	roundStart   bool
	nextRoundDel uint64

	// Startup growth tracking.
	filledPipe bool
	fullBwCnt  int
	fullBw     uint64

	// Stats.
	totalSent  uint64
	totalAcked uint64
	totalLost  uint64

	// Config.
	initialCwnd uint64
	minCwnd     uint64
	maxCwnd     uint64
}

const (
	bbrHighGain         = 2.885 // 2/ln(2)
	bbrDrainGain        = 1.0 / bbrHighGain
	bbrCwndGain         = 2.0
	bbrProbeBWGain      = 1.25
	bbrProbeBWDrainGain = 1.0 / bbrProbeBWGain // 0.8
	bbrProbeBWCycleLen  = 8
	bbrBWFilterLen      = 10
	bbrMinCwnd          = 4 * 1200 // 4 packets
	bbrProbeRTTTime     = 200 * time.Millisecond
	bbrRTPropExpiry     = 10 * time.Second
)

// NewBBR creates a new BBR congestion controller.
func NewBBR(initialCwnd uint64) *BBRController {
	if initialCwnd == 0 {
		initialCwnd = 32 * 1200 // 32 packets
	}
	return &BBRController{
		state:        BBRStartup,
		cwnd:         initialCwnd,
		initialCwnd:  initialCwnd,
		minCwnd:      bbrMinCwnd,
		maxCwnd:      100 * 1024 * 1024, // 100MB
		rtProp:       time.Duration(math.MaxInt64),
		bwFilter:     make([]uint64, bbrBWFilterLen),
		rtPropExpiry: time.Now().Add(bbrRTPropExpiry),
	}
}

// OnPacketSent records a packet being sent.
func (b *BBRController) OnPacketSent(bytes uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.totalSent += bytes
}

// OnAck processes an acknowledgment.
func (b *BBRController) OnAck(ackedBytes uint64, rtt time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.totalAcked += ackedBytes
	b.updateRTT(rtt)
	b.updateBandwidth(ackedBytes, rtt)
	b.updateState()
	b.updateCwnd()
	b.updatePacingRate()
}

// OnPacketLoss processes a packet loss event.
func (b *BBRController) OnPacketLoss(lostBytes uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.totalLost += lostBytes
}

func (b *BBRController) updateRTT(rtt time.Duration) {
	if rtt < b.rtProp || time.Now().After(b.rtPropExpiry) {
		b.rtProp = rtt
		b.rtPropExpiry = time.Now().Add(bbrRTPropExpiry)
	}
}

func (b *BBRController) updateBandwidth(ackedBytes uint64, rtt time.Duration) {
	if rtt <= 0 {
		return
	}
	// Delivery rate = acked bytes / RTT
	deliveryRate := ackedBytes * uint64(time.Second) / uint64(rtt)

	b.bwFilter[b.bwFilterIdx%bbrBWFilterLen] = deliveryRate
	b.bwFilterIdx++

	// btlBw = max over filter window
	maxBw := uint64(0)
	for _, bw := range b.bwFilter {
		if bw > maxBw {
			maxBw = bw
		}
	}
	b.btlBw = maxBw
}

func (b *BBRController) updateState() {
	switch b.state {
	case BBRStartup:
		if b.filledPipe {
			b.state = BBRDrain
		}
		b.checkFilledPipe()
	case BBRDrain:
		inflight := b.totalSent - b.totalAcked
		target := b.bdp()
		if inflight <= target {
			b.state = BBRProbeBW
			b.cycleStart = time.Now()
		}
	case BBRProbeBW:
		// Advance pacing gain cycle every RTT
		if b.rtProp > 0 && time.Since(b.cycleStart) > b.rtProp {
			b.cycleIdx = (b.cycleIdx + 1) % bbrProbeBWCycleLen
			b.cycleStart = time.Now()
		}
		if time.Now().After(b.rtPropExpiry) {
			b.state = BBRProbeRTT
			b.probeRTTDone = false
		}
	case BBRProbeRTT:
		if b.probeRTTDone {
			b.state = BBRProbeBW
		}
	}
}

func (b *BBRController) checkFilledPipe() {
	if b.btlBw >= b.fullBw*5/4 { // 25% growth
		b.fullBw = b.btlBw
		b.fullBwCnt = 0
		return
	}
	b.fullBwCnt++
	if b.fullBwCnt >= 3 {
		b.filledPipe = true
	}
}

func (b *BBRController) bdp() uint64 {
	if b.rtProp == time.Duration(math.MaxInt64) {
		return b.initialCwnd
	}
	return b.btlBw * uint64(b.rtProp) / uint64(time.Second)
}

func (b *BBRController) updateCwnd() {
	target := b.bdp()
	gain := b.cwndGain()
	b.cwnd = uint64(float64(target) * gain)
	if b.cwnd < b.minCwnd {
		b.cwnd = b.minCwnd
	}
	if b.cwnd > b.maxCwnd {
		b.cwnd = b.maxCwnd
	}
}

func (b *BBRController) cwndGain() float64 {
	switch b.state {
	case BBRStartup:
		return bbrHighGain
	case BBRDrain:
		// During Drain, cwnd should shrink to BDP so the queue drains.
		// Using 1/bbrHighGain matches the Linux kernel BBR implementation.
		return bbrDrainGain
	case BBRProbeRTT:
		return 1.0
	default:
		// ProbeBW: keep 2x BDP headroom for bandwidth probing bursts.
		return bbrCwndGain
	}
}

func (b *BBRController) updatePacingRate() {
	gain := b.pacingGain()
	b.pacingRate = uint64(float64(b.btlBw) * gain)
}

func (b *BBRController) pacingGain() float64 {
	switch b.state {
	case BBRStartup:
		return bbrHighGain
	case BBRDrain:
		return bbrDrainGain
	case BBRProbeBW:
		switch b.cycleIdx {
		case 0:
			return bbrProbeBWGain
		case 1:
			return bbrProbeBWDrainGain
		default:
			return 1.0
		}
	default:
		return 1.0
	}
}

// GetCwnd returns the current congestion window.
func (b *BBRController) GetCwnd() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cwnd
}

// GetPacingRate returns the current pacing rate in bytes/sec.
func (b *BBRController) GetPacingRate() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pacingRate
}

// Stats returns current BBR statistics.
func (b *BBRController) Stats() map[string]interface{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	return map[string]interface{}{
		"state":      b.state,
		"btlBw":      b.btlBw,
		"rtProp":     b.rtProp,
		"cwnd":       b.cwnd,
		"pacingRate": b.pacingRate,
		"totalSent":  b.totalSent,
		"totalAcked": b.totalAcked,
		"totalLost":  b.totalLost,
	}
}

// InStartup returns true if BBR is in the Startup phase.
// Thread-safe — acquires the mutex.
func (b *BBRController) InStartup() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state == BBRStartup
}
