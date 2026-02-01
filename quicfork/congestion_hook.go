package quic

import (
	"github.com/quic-go/quic-go/internal/congestion"
	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/protocol"
)

// CongestionControl is the public interface for injecting custom congestion
// control algorithms into QUIC connections. It mirrors the internal
// SendAlgorithmWithDebugInfos but uses only public/stdlib types.
type CongestionControl interface {
	// TimeUntilSend returns when the next packet may be sent (as UnixNano).
	// Return 0 to send immediately.
	TimeUntilSend(bytesInFlight int64) int64
	// HasPacingBudget reports whether a packet can be sent now (nowUnixNano).
	HasPacingBudget(nowUnixNano int64) bool
	// OnPacketSent is called when a packet is sent.
	OnPacketSent(sentTimeUnixNano int64, bytesInFlight int64, packetNumber int64, bytes int64, isRetransmittable bool)
	// CanSend reports whether data can be sent given the bytes in flight.
	CanSend(bytesInFlight int64) bool
	// MaybeExitSlowStart transitions out of slow start if appropriate.
	MaybeExitSlowStart()
	// OnPacketAcked is called when a packet is acknowledged.
	OnPacketAcked(number int64, ackedBytes int64, priorInFlight int64, eventTimeUnixNano int64)
	// OnCongestionEvent is called on packet loss.
	OnCongestionEvent(number int64, lostBytes int64, priorInFlight int64)
	// OnRetransmissionTimeout is called on RTO.
	OnRetransmissionTimeout(packetsRetransmitted bool)
	// SetMaxDatagramSize updates the maximum datagram size.
	SetMaxDatagramSize(size int64)
	// InSlowStart reports whether the algorithm is in slow start.
	InSlowStart() bool
	// InRecovery reports whether the algorithm is in recovery.
	InRecovery() bool
	// GetCongestionWindow returns the current congestion window in bytes.
	GetCongestionWindow() int64
}

// ccAdapter wraps a public CongestionControl into the internal interface.
type ccAdapter struct {
	cc CongestionControl
}

func (a *ccAdapter) TimeUntilSend(bytesInFlight protocol.ByteCount) monotime.Time {
	return monotime.Time(a.cc.TimeUntilSend(int64(bytesInFlight)))
}

func (a *ccAdapter) HasPacingBudget(now monotime.Time) bool {
	return a.cc.HasPacingBudget(int64(now))
}

func (a *ccAdapter) OnPacketSent(sentTime monotime.Time, bytesInFlight protocol.ByteCount, packetNumber protocol.PacketNumber, bytes protocol.ByteCount, isRetransmittable bool) {
	a.cc.OnPacketSent(int64(sentTime), int64(bytesInFlight), int64(packetNumber), int64(bytes), isRetransmittable)
}

func (a *ccAdapter) CanSend(bytesInFlight protocol.ByteCount) bool {
	return a.cc.CanSend(int64(bytesInFlight))
}

func (a *ccAdapter) MaybeExitSlowStart() {
	a.cc.MaybeExitSlowStart()
}

func (a *ccAdapter) OnPacketAcked(number protocol.PacketNumber, ackedBytes protocol.ByteCount, priorInFlight protocol.ByteCount, eventTime monotime.Time) {
	a.cc.OnPacketAcked(int64(number), int64(ackedBytes), int64(priorInFlight), int64(eventTime))
}

func (a *ccAdapter) OnCongestionEvent(number protocol.PacketNumber, lostBytes protocol.ByteCount, priorInFlight protocol.ByteCount) {
	a.cc.OnCongestionEvent(int64(number), int64(lostBytes), int64(priorInFlight))
}

func (a *ccAdapter) OnRetransmissionTimeout(packetsRetransmitted bool) {
	a.cc.OnRetransmissionTimeout(packetsRetransmitted)
}

func (a *ccAdapter) SetMaxDatagramSize(s protocol.ByteCount) {
	a.cc.SetMaxDatagramSize(int64(s))
}

func (a *ccAdapter) InSlowStart() bool    { return a.cc.InSlowStart() }
func (a *ccAdapter) InRecovery() bool      { return a.cc.InRecovery() }
func (a *ccAdapter) GetCongestionWindow() protocol.ByteCount {
	return protocol.ByteCount(a.cc.GetCongestionWindow())
}

var _ congestion.SendAlgorithmWithDebugInfos = (*ccAdapter)(nil)

// wrapCongestionControl converts a public CongestionControl to the internal interface.
// Returns nil if cc is nil.
func wrapCongestionControl(cc CongestionControl) congestion.SendAlgorithmWithDebugInfos {
	if cc == nil {
		return nil
	}
	return &ccAdapter{cc: cc}
}

