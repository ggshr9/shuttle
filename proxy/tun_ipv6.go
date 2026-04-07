package proxy

import (
	"context"
	"encoding/binary"
	"net/netip"
)

// ---------------------------------------------------------------------------
// IPv6 packet handling
// ---------------------------------------------------------------------------

const ipv6HeaderLen = 40

func (t *TUNServer) handleIPv6(ctx context.Context, pkt []byte) {
	if len(pkt) < ipv6HeaderLen {
		return
	}

	payloadLen := int(binary.BigEndian.Uint16(pkt[4:6]))
	nextHeader := pkt[6]

	if ipv6HeaderLen+payloadLen > len(pkt) {
		payloadLen = len(pkt) - ipv6HeaderLen
	}

	var src16, dst16 [16]byte
	copy(src16[:], pkt[8:24])
	copy(dst16[:], pkt[24:40])
	srcIP := netip.AddrFrom16(src16)
	dstIP := netip.AddrFrom16(dst16)

	payload := pkt[ipv6HeaderLen : ipv6HeaderLen+payloadLen]

	switch nextHeader {
	case protoTCP:
		t.handleTCP(ctx, srcIP, dstIP, payload)
	case protoUDP:
		t.handleUDP(ctx, srcIP, dstIP, payload)
	}
}

// ---------------------------------------------------------------------------
// IPv6 packet construction helpers
// ---------------------------------------------------------------------------

// buildTCPPacketV6WithTOS constructs a raw IPv6+TCP packet with traffic class marking.
func buildTCPPacketV6WithTOS(srcIP, dstIP [16]byte, srcPort, dstPort uint16, seq, ack uint32, flags byte, payload []byte, trafficClass uint8) []byte {
	tcpLen := 20 + len(payload)
	totalLen := ipv6HeaderLen + tcpLen
	pkt := getTUNPacket(totalLen)

	// IPv6 header (40 bytes)
	// Byte 0: version(4)=6, traffic_class_high(4)
	// Byte 1: traffic_class_low(4), flow_label_high(4)
	// Bytes 2-3: flow_label_low(16)
	pkt[0] = 0x60 | (trafficClass >> 4)
	pkt[1] = (trafficClass << 4)
	pkt[2] = 0
	pkt[3] = 0
	binary.BigEndian.PutUint16(pkt[4:6], uint16(tcpLen)) // payload length
	pkt[6] = protoTCP                                     // next header
	pkt[7] = 64                                           // hop limit
	copy(pkt[8:24], srcIP[:])
	copy(pkt[24:40], dstIP[:])

	// TCP header at offset 40
	tcp := pkt[ipv6HeaderLen:]
	binary.BigEndian.PutUint16(tcp[0:2], srcPort)
	binary.BigEndian.PutUint16(tcp[2:4], dstPort)
	binary.BigEndian.PutUint32(tcp[4:8], seq)
	binary.BigEndian.PutUint32(tcp[8:12], ack)
	tcp[12] = 5 << 4 // data offset = 5 (20 bytes)
	tcp[13] = flags
	binary.BigEndian.PutUint16(tcp[14:16], 65535) // window

	if len(payload) > 0 {
		copy(tcp[20:], payload)
	}

	// TCP checksum (using IPv6 pseudo-header)
	binary.BigEndian.PutUint16(tcp[16:18], 0)
	binary.BigEndian.PutUint16(tcp[16:18], tcpChecksumV6(srcIP, dstIP, tcp[:tcpLen]))

	return pkt
}

// buildUDPPacketV6WithTOS constructs a raw IPv6+UDP packet with traffic class marking.
func buildUDPPacketV6WithTOS(srcIP, dstIP [16]byte, srcPort, dstPort uint16, payload []byte, trafficClass uint8) []byte {
	udpLen := 8 + len(payload)
	totalLen := ipv6HeaderLen + udpLen
	pkt := getTUNPacket(totalLen)

	// IPv6 header
	pkt[0] = 0x60 | (trafficClass >> 4)
	pkt[1] = (trafficClass << 4)
	pkt[2] = 0
	pkt[3] = 0
	binary.BigEndian.PutUint16(pkt[4:6], uint16(udpLen))
	pkt[6] = protoUDP
	pkt[7] = 64
	copy(pkt[8:24], srcIP[:])
	copy(pkt[24:40], dstIP[:])

	// UDP header at offset 40
	udp := pkt[ipv6HeaderLen:]
	binary.BigEndian.PutUint16(udp[0:2], srcPort)
	binary.BigEndian.PutUint16(udp[2:4], dstPort)
	binary.BigEndian.PutUint16(udp[4:6], uint16(udpLen))

	if len(payload) > 0 {
		copy(udp[8:], payload)
	}

	// UDP checksum — mandatory for IPv6 (unlike IPv4 where it's optional)
	binary.BigEndian.PutUint16(udp[6:8], 0)
	csum := udpChecksumV6(srcIP, dstIP, udp[:udpLen])
	if csum == 0 {
		csum = 0xffff // RFC 2460: zero checksum transmitted as all ones
	}
	binary.BigEndian.PutUint16(udp[6:8], csum)

	return pkt
}

// ---------------------------------------------------------------------------
// IPv6 checksum helpers
// ---------------------------------------------------------------------------

// tcpChecksumV6 computes the TCP checksum using the IPv6 pseudo-header.
// Pseudo-header: src(16) + dst(16) + upper-layer-length(4) + zeros(3) + next-header(1) = 40 bytes.
func tcpChecksumV6(srcIP, dstIP [16]byte, tcpSeg []byte) uint16 {
	var pseudo [40]byte
	copy(pseudo[0:16], srcIP[:])
	copy(pseudo[16:32], dstIP[:])
	binary.BigEndian.PutUint32(pseudo[32:36], uint32(len(tcpSeg)))
	pseudo[39] = protoTCP
	sum := checksumData(0, pseudo[:])
	sum = checksumData(sum, tcpSeg)
	return ^checksumFold(sum)
}

// udpChecksumV6 computes the UDP checksum using the IPv6 pseudo-header.
// Pseudo-header: src(16) + dst(16) + upper-layer-length(4) + zeros(3) + next-header(1) = 40 bytes.
func udpChecksumV6(srcIP, dstIP [16]byte, udpSeg []byte) uint16 {
	var pseudo [40]byte
	copy(pseudo[0:16], srcIP[:])
	copy(pseudo[16:32], dstIP[:])
	binary.BigEndian.PutUint32(pseudo[32:36], uint32(len(udpSeg)))
	pseudo[39] = protoUDP
	sum := checksumData(0, pseudo[:])
	sum = checksumData(sum, udpSeg)
	return ^checksumFold(sum)
}
