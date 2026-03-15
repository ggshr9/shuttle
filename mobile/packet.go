package mobile

import "sync"

// PacketResult holds a single packet returned from the engine to the native layer.
// Uses simple types only for gomobile compatibility.
type PacketResult struct {
	Data  []byte
	Proto int32 // 4 = IPv4, 6 = IPv6
}

var (
	pktMu      sync.Mutex
	pktInbound chan *PacketResult
)

func initPacketPipe() {
	pktMu.Lock()
	pktInbound = make(chan *PacketResult, 256)
	pktMu.Unlock()
}

func closePacketPipe() {
	pktMu.Lock()
	if pktInbound != nil {
		close(pktInbound)
		pktInbound = nil
	}
	pktMu.Unlock()
}

// WritePacket sends a raw IP packet from the native TUN to the Go engine for processing.
// proto is the address family (AF_INET=2 for IPv4, AF_INET6=30 for IPv6 on Darwin).
func WritePacket(data []byte, proto int32) {
	// The engine processes outbound packets through the proxy pipeline.
	// In the current architecture, the TUN listener in the engine handles
	// packet processing when a TUN fd is provided. For iOS where we use
	// packetFlow, outbound packets are processed via the SOCKS5/HTTP proxy
	// that the engine already runs. This function is a hook for future
	// direct packet injection when the engine supports it.
	if len(data) == 0 {
		return
	}
	mobileLogger.Debug("outbound packet", "size", len(data), "proto", proto)
}

// ReadPacket blocks until a processed packet is available from the Go engine,
// then returns it. Returns nil when the engine is stopped.
func ReadPacket() *PacketResult {
	pktMu.Lock()
	ch := pktInbound
	pktMu.Unlock()

	if ch == nil {
		return nil
	}

	pkt, ok := <-ch
	if !ok {
		return nil
	}
	return pkt
}

// enqueueInboundPacket sends a processed packet to the native layer.
// Called by the engine when a response packet needs to be delivered to the TUN.
func enqueueInboundPacket(data []byte, proto int32) {
	pktMu.Lock()
	ch := pktInbound
	pktMu.Unlock()

	if ch == nil {
		return
	}

	select {
	case ch <- &PacketResult{Data: data, Proto: proto}:
	default:
		// Drop packet if buffer is full — UDP-like behaviour
		mobileLogger.Warn("inbound packet buffer full, dropping packet", "size", len(data))
	}
}
