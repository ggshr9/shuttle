package proxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shuttle-proxy/shuttle/internal/procnet"
)

// TUNConfig configures the TUN device for system-wide proxying.
type TUNConfig struct {
	DeviceName string
	CIDR       string // e.g., "198.18.0.0/16"
	MTU        int
	AutoRoute  bool
	TunFD      int // externally provided fd; if > 0, skip createTUN
}

// TUNServer manages a TUN device for transparent proxying.
type TUNServer struct {
	config       *TUNConfig
	dialer       Dialer
	closed       atomic.Bool
	logger       *slog.Logger
	ProcResolver *procnet.Resolver

	tunFile  *os.File
	natTable natTable
	wmu      sync.Mutex // serialises writes to TUN
}

// NewTUNServer creates a new TUN proxy server.
func NewTUNServer(cfg *TUNConfig, dialer Dialer, logger *slog.Logger) *TUNServer {
	if cfg.DeviceName == "" {
		cfg.DeviceName = "shuttle0"
	}
	if cfg.CIDR == "" {
		cfg.CIDR = "198.18.0.0/16"
	}
	if cfg.MTU == 0 {
		cfg.MTU = 1500
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &TUNServer{
		config: cfg,
		dialer: dialer,
		logger: logger,
		natTable: natTable{
			tcp: make(map[natKey]*natEntry),
			udp: make(map[natKey]*natEntry),
		},
	}
}

// ---------------------------------------------------------------------------
// NAT table — maps (srcIP, srcPort, dstIP, dstPort) to a proxied connection
// ---------------------------------------------------------------------------

type natKey struct {
	srcIP   [4]byte
	dstIP   [4]byte
	srcPort uint16
	dstPort uint16
}

type natEntry struct {
	conn   net.Conn
	cancel context.CancelFunc
}

type natTable struct {
	mu  sync.RWMutex
	tcp map[natKey]*natEntry
	udp map[natKey]*natEntry
}

func (n *natTable) getTCP(k natKey) *natEntry {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.tcp[k]
}

func (n *natTable) putTCP(k natKey, e *natEntry) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.tcp[k] = e
}

func (n *natTable) deleteTCP(k natKey) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.tcp, k)
}

func (n *natTable) getUDP(k natKey) *natEntry {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.udp[k]
}

func (n *natTable) putUDP(k natKey, e *natEntry) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.udp[k] = e
}

func (n *natTable) deleteUDP(k natKey) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.udp, k)
}

func (n *natTable) closeAll() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for k, e := range n.tcp {
		e.cancel()
		e.conn.Close()
		delete(n.tcp, k)
	}
	for k, e := range n.udp {
		e.cancel()
		e.conn.Close()
		delete(n.udp, k)
	}
}

// ---------------------------------------------------------------------------
// Start / Close
// ---------------------------------------------------------------------------

// Start creates the TUN device, configures routes and begins packet processing.
func (t *TUNServer) Start(ctx context.Context) error {
	if t.config.TunFD > 0 {
		// Use externally provided file descriptor (e.g. from Android VpnService)
		t.tunFile = os.NewFile(uintptr(t.config.TunFD), "tun")
	} else {
		f, err := t.createTUN()
		if err != nil {
			return fmt.Errorf("tun: create device: %w", err)
		}
		t.tunFile = f

		if err := t.configureTUN(); err != nil {
			t.tunFile.Close()
			return fmt.Errorf("tun: configure device: %w", err)
		}

		if t.config.AutoRoute {
			if err := t.setupRoutes(); err != nil {
				t.tunFile.Close()
				return fmt.Errorf("tun: setup routes: %w", err)
			}
		}
	}

	t.logger.Info("tun device started",
		"name", t.config.DeviceName,
		"cidr", t.config.CIDR,
		"mtu", t.config.MTU,
	)

	go t.readLoop(ctx)
	return nil
}

// Close shuts down the TUN device and tears down routes.
func (t *TUNServer) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}
	t.natTable.closeAll()
	if t.config.AutoRoute {
		t.teardownRoutes()
	}
	if t.tunFile != nil {
		return t.tunFile.Close()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Packet read loop
// ---------------------------------------------------------------------------

func (t *TUNServer) readLoop(ctx context.Context) {
	buf := make([]byte, t.config.MTU+4)
	for {
		if t.closed.Load() {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := t.tunFile.Read(buf)
		if err != nil {
			if t.closed.Load() {
				return
			}
			t.logger.Error("tun read error", "err", err)
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if n == 0 {
			continue
		}

		pkt := buf[:n]
		if len(pkt) < 20 {
			continue
		}
		version := pkt[0] >> 4
		if version != 4 {
			continue // IPv4 only for now
		}

		t.handleIPv4(ctx, pkt)
	}
}

// ---------------------------------------------------------------------------
// IPv4 packet handling
// ---------------------------------------------------------------------------

const (
	protoTCP = 6
	protoUDP = 17
)

func (t *TUNServer) handleIPv4(ctx context.Context, pkt []byte) {
	if len(pkt) < 20 {
		return
	}
	ihl := int(pkt[0]&0x0f) * 4
	if ihl < 20 || len(pkt) < ihl {
		return
	}
	totalLen := int(binary.BigEndian.Uint16(pkt[2:4]))
	if totalLen > len(pkt) {
		totalLen = len(pkt)
	}
	if totalLen < ihl {
		return
	}
	proto := pkt[9]

	var srcIP, dstIP [4]byte
	copy(srcIP[:], pkt[12:16])
	copy(dstIP[:], pkt[16:20])

	payload := pkt[ihl:totalLen]

	switch proto {
	case protoTCP:
		t.handleTCP(ctx, srcIP, dstIP, payload)
	case protoUDP:
		t.handleUDP(ctx, srcIP, dstIP, payload)
	}
}

// ---------------------------------------------------------------------------
// TCP handling — SYN triggers dial, data is relayed
// ---------------------------------------------------------------------------

const (
	tcpFlagFIN = 0x01
	tcpFlagSYN = 0x02
	tcpFlagRST = 0x04
	tcpFlagACK = 0x10
)

func (t *TUNServer) handleTCP(ctx context.Context, srcIP, dstIP [4]byte, tcpData []byte) {
	if len(tcpData) < 20 {
		return
	}
	srcPort := binary.BigEndian.Uint16(tcpData[0:2])
	dstPort := binary.BigEndian.Uint16(tcpData[2:4])
	flags := tcpData[13]
	dataOff := int(tcpData[12]>>4) * 4
	seq := binary.BigEndian.Uint32(tcpData[4:8])

	key := natKey{srcIP: srcIP, dstIP: dstIP, srcPort: srcPort, dstPort: dstPort}

	// RST or FIN — tear down
	if flags&tcpFlagRST != 0 || flags&tcpFlagFIN != 0 {
		if e := t.natTable.getTCP(key); e != nil {
			e.cancel()
			e.conn.Close()
			t.natTable.deleteTCP(key)
		}
		if flags&tcpFlagFIN != 0 {
			t.sendTCPReset(dstIP, srcIP, dstPort, srcPort, seq+1)
		}
		return
	}

	// SYN — new connection
	if flags&tcpFlagSYN != 0 && flags&tcpFlagACK == 0 {
		if t.natTable.getTCP(key) != nil {
			return
		}
		go t.dialAndProxyTCP(ctx, key, seq)
		return
	}

	// Data — forward to existing connection
	if e := t.natTable.getTCP(key); e != nil && dataOff < len(tcpData) {
		payload := tcpData[dataOff:]
		if len(payload) > 0 {
			e.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
			if _, err := e.conn.Write(payload); err != nil {
				t.logger.Debug("tcp write to proxy error", "err", err)
				e.cancel()
				e.conn.Close()
				t.natTable.deleteTCP(key)
			}
		}
	}
}

func (t *TUNServer) dialAndProxyTCP(ctx context.Context, key natKey, clientISN uint32) {
	addr := net.JoinHostPort(
		net.IP(key.dstIP[:]).String(),
		fmt.Sprintf("%d", key.dstPort),
	)

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	if t.ProcResolver != nil {
		if procName := t.ProcResolver.Resolve(key.srcPort); procName != "" {
			dialCtx = WithProcess(dialCtx, procName)
		}
	}
	conn, err := t.dialer(dialCtx, "tcp", addr)
	if err != nil {
		cancel()
		t.logger.Debug("tun: tcp dial failed", "addr", addr, "err", err)
		t.sendTCPReset(key.dstIP, key.srcIP, key.dstPort, key.srcPort, clientISN+1)
		return
	}

	entry := &natEntry{conn: conn, cancel: cancel}
	t.natTable.putTCP(key, entry)

	// Send SYN-ACK back to TUN.
	t.sendTCPSynAck(key.dstIP, key.srcIP, key.dstPort, key.srcPort, clientISN)

	// Read responses from proxy and inject back as IP packets.
	go func() {
		defer func() {
			conn.Close()
			cancel()
			t.natTable.deleteTCP(key)
		}()
		buf := make([]byte, t.config.MTU-40) // IP(20)+TCP(20)
		seq := uint32(1)                      // our ISN=0, first data byte seq=1
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				t.injectTCPData(key.dstIP, key.srcIP, key.dstPort, key.srcPort, seq, buf[:n])
				seq += uint32(n)
			}
			if err != nil {
				return
			}
		}
	}()
}

// ---------------------------------------------------------------------------
// UDP handling
// ---------------------------------------------------------------------------

func (t *TUNServer) handleUDP(ctx context.Context, srcIP, dstIP [4]byte, udpData []byte) {
	if len(udpData) < 8 {
		return
	}
	srcPort := binary.BigEndian.Uint16(udpData[0:2])
	dstPort := binary.BigEndian.Uint16(udpData[2:4])
	payload := udpData[8:]

	key := natKey{srcIP: srcIP, dstIP: dstIP, srcPort: srcPort, dstPort: dstPort}

	if e := t.natTable.getUDP(key); e != nil {
		e.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
		e.conn.Write(payload)
		return
	}

	go t.dialAndProxyUDP(ctx, key, payload)
}

func (t *TUNServer) dialAndProxyUDP(ctx context.Context, key natKey, initialPayload []byte) {
	addr := net.JoinHostPort(
		net.IP(key.dstIP[:]).String(),
		fmt.Sprintf("%d", key.dstPort),
	)

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	if t.ProcResolver != nil {
		if procName := t.ProcResolver.Resolve(key.srcPort); procName != "" {
			dialCtx = WithProcess(dialCtx, procName)
		}
	}
	conn, err := t.dialer(dialCtx, "udp", addr)
	if err != nil {
		cancel()
		t.logger.Debug("tun: udp dial failed", "addr", addr, "err", err)
		return
	}

	entry := &natEntry{conn: conn, cancel: cancel}
	t.natTable.putUDP(key, entry)

	conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	conn.Write(initialPayload)

	go func() {
		defer func() {
			conn.Close()
			cancel()
			t.natTable.deleteUDP(key)
		}()
		buf := make([]byte, t.config.MTU-28) // IP(20)+UDP(8)
		for {
			conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
			n, err := conn.Read(buf)
			if n > 0 {
				t.injectUDPPacket(key.dstIP, key.srcIP, key.dstPort, key.srcPort, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
}

// ---------------------------------------------------------------------------
// Packet construction helpers
// ---------------------------------------------------------------------------

func (t *TUNServer) writeTUN(pkt []byte) {
	t.wmu.Lock()
	defer t.wmu.Unlock()
	if t.tunFile != nil {
		t.tunFile.Write(pkt)
	}
}

func (t *TUNServer) sendTCPSynAck(srcIP, dstIP [4]byte, srcPort, dstPort uint16, clientISN uint32) {
	pkt := buildTCPPacket(srcIP, dstIP, srcPort, dstPort, 0, clientISN+1, tcpFlagSYN|tcpFlagACK, nil)
	t.writeTUN(pkt)
}

func (t *TUNServer) sendTCPReset(srcIP, dstIP [4]byte, srcPort, dstPort uint16, ackNum uint32) {
	pkt := buildTCPPacket(srcIP, dstIP, srcPort, dstPort, 0, ackNum, tcpFlagRST|tcpFlagACK, nil)
	t.writeTUN(pkt)
}

func (t *TUNServer) injectTCPData(srcIP, dstIP [4]byte, srcPort, dstPort uint16, seq uint32, data []byte) {
	pkt := buildTCPPacket(srcIP, dstIP, srcPort, dstPort, seq, 0, tcpFlagACK, data)
	t.writeTUN(pkt)
}

func (t *TUNServer) injectUDPPacket(srcIP, dstIP [4]byte, srcPort, dstPort uint16, data []byte) {
	pkt := buildUDPPacket(srcIP, dstIP, srcPort, dstPort, data)
	t.writeTUN(pkt)
}

// buildTCPPacket constructs a raw IPv4+TCP packet.
func buildTCPPacket(srcIP, dstIP [4]byte, srcPort, dstPort uint16, seq, ack uint32, flags byte, payload []byte) []byte {
	tcpLen := 20 + len(payload)
	totalLen := 20 + tcpLen
	pkt := make([]byte, totalLen)

	// IPv4 header
	pkt[0] = 0x45 // version=4, IHL=5
	binary.BigEndian.PutUint16(pkt[2:4], uint16(totalLen))
	pkt[8] = 64 // TTL
	pkt[9] = protoTCP
	copy(pkt[12:16], srcIP[:])
	copy(pkt[16:20], dstIP[:])

	// TCP header at offset 20
	tcp := pkt[20:]
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

	// TCP checksum
	binary.BigEndian.PutUint16(tcp[16:18], 0)
	binary.BigEndian.PutUint16(tcp[16:18], tcpChecksum(srcIP, dstIP, tcp[:tcpLen]))

	// IPv4 header checksum
	binary.BigEndian.PutUint16(pkt[10:12], 0)
	binary.BigEndian.PutUint16(pkt[10:12], ipChecksum(pkt[:20]))

	return pkt
}

// buildUDPPacket constructs a raw IPv4+UDP packet.
func buildUDPPacket(srcIP, dstIP [4]byte, srcPort, dstPort uint16, payload []byte) []byte {
	udpLen := 8 + len(payload)
	totalLen := 20 + udpLen
	pkt := make([]byte, totalLen)

	// IPv4 header
	pkt[0] = 0x45
	binary.BigEndian.PutUint16(pkt[2:4], uint16(totalLen))
	pkt[8] = 64
	pkt[9] = protoUDP
	copy(pkt[12:16], srcIP[:])
	copy(pkt[16:20], dstIP[:])

	// UDP header at offset 20
	udp := pkt[20:]
	binary.BigEndian.PutUint16(udp[0:2], srcPort)
	binary.BigEndian.PutUint16(udp[2:4], dstPort)
	binary.BigEndian.PutUint16(udp[4:6], uint16(udpLen))

	if len(payload) > 0 {
		copy(udp[8:], payload)
	}

	// UDP checksum
	binary.BigEndian.PutUint16(udp[6:8], 0)
	csum := udpChecksum(srcIP, dstIP, udp[:udpLen])
	if csum == 0 {
		csum = 0xffff
	}
	binary.BigEndian.PutUint16(udp[6:8], csum)

	// IPv4 header checksum
	binary.BigEndian.PutUint16(pkt[10:12], 0)
	binary.BigEndian.PutUint16(pkt[10:12], ipChecksum(pkt[:20]))

	return pkt
}

// ---------------------------------------------------------------------------
// Checksum helpers
// ---------------------------------------------------------------------------

func ipChecksum(header []byte) uint16 {
	return ^checksumFold(checksumData(0, header))
}

func tcpChecksum(srcIP, dstIP [4]byte, tcpSeg []byte) uint16 {
	var pseudo [12]byte
	copy(pseudo[0:4], srcIP[:])
	copy(pseudo[4:8], dstIP[:])
	pseudo[9] = protoTCP
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(len(tcpSeg)))
	sum := checksumData(0, pseudo[:])
	sum = checksumData(sum, tcpSeg)
	return ^checksumFold(sum)
}

func udpChecksum(srcIP, dstIP [4]byte, udpSeg []byte) uint16 {
	var pseudo [12]byte
	copy(pseudo[0:4], srcIP[:])
	copy(pseudo[4:8], dstIP[:])
	pseudo[9] = protoUDP
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(len(udpSeg)))
	sum := checksumData(0, pseudo[:])
	sum = checksumData(sum, udpSeg)
	return ^checksumFold(sum)
}

func checksumData(initial uint32, data []byte) uint32 {
	sum := initial
	for i := 0; i+1 < len(data); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	if len(data)%2 != 0 {
		sum += uint32(data[len(data)-1]) << 8
	}
	return sum
}

func checksumFold(sum uint32) uint16 {
	for sum > 0xffff {
		sum = (sum >> 16) + (sum & 0xffff)
	}
	return uint16(sum)
}

// maskBits returns the prefix length of an IP mask.
func maskBits(m net.IPMask) int {
	ones, _ := m.Size()
	return ones
}
