package webrtc

import (
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// ConnStats holds connection-level statistics collected from the WebRTC peer connection.
type ConnStats struct {
	RTT            time.Duration
	PacketsSent    uint64
	PacketsRecv    uint64
	BytesSent      uint64
	BytesRecv      uint64
	PacketsLost    uint32
	CandidateLocal string
	CandidateType  string // "host", "srflx", "prflx", "relay"
}

// statsCollector periodically polls the pion Stats API.
type statsCollector struct {
	mu    sync.RWMutex
	pc    *webrtc.PeerConnection
	stats ConnStats
	done  chan struct{}
}

func newStatsCollector(pc *webrtc.PeerConnection) *statsCollector {
	sc := &statsCollector{
		pc:   pc,
		done: make(chan struct{}),
	}
	go sc.loop()
	return sc
}

func (sc *statsCollector) loop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-sc.done:
			return
		case <-ticker.C:
			sc.collect()
		}
	}
}

func (sc *statsCollector) collect() {
	report := sc.pc.GetStats()

	var stats ConnStats
	for _, s := range report {
		switch v := s.(type) {
		case webrtc.ICECandidatePairStats:
			if v.Nominated {
				stats.RTT = time.Duration(v.CurrentRoundTripTime * float64(time.Second))
				stats.BytesSent = uint64(v.BytesSent)
				stats.BytesRecv = uint64(v.BytesReceived)
				stats.PacketsSent = uint64(v.PacketsSent)
				stats.PacketsRecv = uint64(v.PacketsReceived)
			}
		case webrtc.ICECandidateStats:
			if v.Type == webrtc.StatsTypeLocalCandidate {
				stats.CandidateLocal = v.IP + ":" + fmt.Sprint(v.Port)
				stats.CandidateType = string(v.CandidateType)
			}
		}
	}

	sc.mu.Lock()
	sc.stats = stats
	sc.mu.Unlock()
}

func (sc *statsCollector) Stats() ConnStats {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.stats
}

func (sc *statsCollector) Close() {
	select {
	case <-sc.done:
	default:
		close(sc.done)
	}
}
