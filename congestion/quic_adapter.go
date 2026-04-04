package congestion

import (
	"sync"
	"time"

	quic "github.com/quic-go/quic-go"
)

// QUICAdapter wraps a CongestionController to implement quic.CongestionControl,
// bridging shuttle's congestion algorithms into quic-go's QUIC stack.
type QUICAdapter struct {
	mu   sync.Mutex
	cc   CongestionController
	cwnd int64 // bytes
	mds  int64 // max datagram size

	// Track loss for the adaptive controller.
	totalSent uint64
	totalLost uint64

	// Track RTT from ack timing (rough estimate).
	lastSendTimes map[int64]int64 // packetNumber → sentTimeNano
}

// NewQUICAdapter creates a quic.CongestionControl from a shuttle CongestionController.
func NewQUICAdapter(cc CongestionController) quic.CongestionControl {
	return &QUICAdapter{
		cc:            cc,
		cwnd:          int64(cc.GetCwnd()),
		mds:           1200,
		lastSendTimes: make(map[int64]int64),
	}
}

func (a *QUICAdapter) TimeUntilSend(bytesInFlight int64) int64 {
	// 0 means send immediately. We rely on CanSend for gating.
	return 0
}

func (a *QUICAdapter) HasPacingBudget(nowUnixNano int64) bool {
	return true // Pacing handled by CanSend/cwnd
}

func (a *QUICAdapter) OnPacketSent(sentTimeUnixNano int64, bytesInFlight int64, packetNumber int64, bytes int64, isRetransmittable bool) {
	if !isRetransmittable {
		return
	}
	a.mu.Lock()
	a.totalSent += uint64(bytes)
	a.lastSendTimes[packetNumber] = sentTimeUnixNano
	// Limit map size to prevent unbounded growth.
	if len(a.lastSendTimes) > 10000 {
		for k := range a.lastSendTimes {
			delete(a.lastSendTimes, k)
			if len(a.lastSendTimes) <= 5000 {
				break
			}
		}
	}
	a.mu.Unlock()

	a.cc.OnPacketSent(uint64(bytes))
}

func (a *QUICAdapter) CanSend(bytesInFlight int64) bool {
	return bytesInFlight < int64(a.cc.GetCwnd())
}

func (a *QUICAdapter) MaybeExitSlowStart() {
	// No-op: our controllers don't have a slow start concept.
}

func (a *QUICAdapter) OnPacketAcked(number int64, ackedBytes int64, priorInFlight int64, eventTimeUnixNano int64) {
	a.mu.Lock()
	sentTime, ok := a.lastSendTimes[number]
	delete(a.lastSendTimes, number)
	a.mu.Unlock()

	var rtt time.Duration
	if ok && eventTimeUnixNano > sentTime {
		rtt = time.Duration(eventTimeUnixNano - sentTime)
	}
	a.cc.OnAck(uint64(ackedBytes), rtt)
}

func (a *QUICAdapter) OnCongestionEvent(number int64, lostBytes int64, priorInFlight int64) {
	a.mu.Lock()
	a.totalLost += uint64(lostBytes)
	delete(a.lastSendTimes, number)
	a.mu.Unlock()

	a.cc.OnPacketLoss(uint64(lostBytes))
}

func (a *QUICAdapter) OnRetransmissionTimeout(packetsRetransmitted bool) {
	// Our controllers handle loss implicitly; no special RTO behavior needed.
}

func (a *QUICAdapter) SetMaxDatagramSize(size int64) {
	a.mu.Lock()
	a.mds = size
	a.mu.Unlock()
}

func (a *QUICAdapter) InSlowStart() bool {
	if bbr, ok := a.cc.(*BBRController); ok {
		return bbr.InStartup()
	}
	if ac, ok := a.cc.(*AdaptiveCongestion); ok {
		ac.mu.Lock()
		active := ac.active
		ac.mu.Unlock()
		if bbr, ok := active.(*BBRController); ok {
			return bbr.InStartup()
		}
	}
	return false
}

func (a *QUICAdapter) InRecovery() bool {
	return false // Our controllers don't track recovery state explicitly.
}

func (a *QUICAdapter) GetCongestionWindow() int64 {
	return int64(a.cc.GetCwnd())
}

var _ quic.CongestionControl = (*QUICAdapter)(nil)
